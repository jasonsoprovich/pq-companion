package spelltimer

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Spell action evidence from the Zeal pipe.
//
// The log's spell landed lines ("A gnoll's movements slow.") name the target
// but carry no spawn id, so a detrimental landing on one of several
// same-named mobs can't be tied to the right one, and the pipe's death event
// can't clear it. Zeal's spell message (one per OP_Action spell packet) does
// carry the ids. Each land line is matched against the recent actions once,
// from the parsed land itself, before the batch's raw lines reach the trigger
// engine (PrimeLands); the parsed pipeline and every trigger timer for that
// line then read the same answer.
//
// The pipe write normally beats the log line (Zeal sends it from the packet
// handler, before the client prints the chat text, and the tailer polls every
// 250ms), but that isn't guaranteed. A land with no matching action keeps the
// existing heuristics, exactly as before.

const (
	// spellActionTTL is how long an action stays eligible to match a land.
	spellActionTTL = 6 * time.Second
	// spellActionMergeWindow is how close the success packet follows its
	// attempt for the two to count as one cast.
	spellActionMergeWindow = 1 * time.Second
	// spellActionCap bounds the buffer during heavy raid casting.
	spellActionCap = 256
)

// spellAction is one spell action the pipe reported.
type spellAction struct {
	spellID  int
	sourceID int
	targetID int
	target   string // normalizeNPCName(target name)
	at       time.Time
	success  bool
	consumed bool
}

// landCorrelation is what the actions say about one land line. Each field is
// set only when every matching action agrees on it; zero means unknown, and
// the log-based path decides it.
type landCorrelation struct {
	target    string // normalizeNPCName of the land's target
	targetID  int
	spellID   int
	spellName string
	sourceID  int
	// candidateIDs is every spell id the land text could be (the parsed
	// SpellID and Candidates), for matching a trigger's configured spell when
	// spellID is unknown.
	candidateIDs map[int]bool
}

// HandlePipeSpellAction records one spell action from the pipe. A success
// packet arriving within spellActionMergeWindow of its attempt upgrades that
// entry, even one already consumed, so it never becomes fresh evidence.
func (e *Engine) HandlePipeSpellAction(spellID, sourceID, targetID int, targetName string, success bool) {
	if spellID <= 0 || targetID <= 0 {
		return
	}
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastActionAt = now
	e.pruneSpellActionsLocked(now)
	for i := len(e.spellActions) - 1; i >= 0; i-- {
		a := e.spellActions[i]
		if now.Sub(a.at) > spellActionMergeWindow {
			break
		}
		if a.spellID == spellID && a.sourceID == sourceID && a.targetID == targetID {
			if success {
				a.success = true
			}
			return
		}
	}
	e.spellActions = append(e.spellActions, &spellAction{
		spellID:  spellID,
		sourceID: sourceID,
		targetID: targetID,
		target:   normalizeNPCName(targetName),
		at:       now,
		success:  success,
	})
	if n := len(e.spellActions); n > spellActionCap {
		e.spellActions = e.spellActions[n-spellActionCap:]
	}
}

// SetSelfSpawnID records the active character's own spawn id from the pipe's
// player message, so a matched action's source can say whether the cast was
// ours. nil (an older Zeal, or no pipe) clears it.
func (e *Engine) SetSelfSpawnID(id *int) {
	e.mu.Lock()
	e.selfSpawnID = 0
	if id != nil {
		e.selfSpawnID = *id
	}
	e.mu.Unlock()
}

func (e *Engine) pruneSpellActionsLocked(now time.Time) {
	i := 0
	for i < len(e.spellActions) && now.Sub(e.spellActions[i].at) > spellActionTTL {
		i++
	}
	if i > 0 {
		e.spellActions = e.spellActions[i:]
	}
}

// PrimeLands correlates every spell land in a log batch with the recent
// actions, before the batch's raw lines are dispatched to triggers. It walks
// the batch in log order: a zone change clears the actions before any later
// land is correlated, so an old zone's action can't identify a land in the
// new one. Lines with Seq 0 (replay) are skipped. The results stay until
// EndBatch.
func (e *Engine) PrimeLands(events []logparser.LogEvent) {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.landCorr = nil
	for _, ev := range events {
		switch ev.Type {
		case logparser.EventZone:
			e.spellActions = nil
		case logparser.EventSpellLanded:
			data, ok := ev.Data.(logparser.SpellLandedData)
			if !ok || ev.Seq == 0 || data.Kind == logparser.SpellLandedKindYou || data.TargetName == "" {
				continue
			}
			c, found := e.correlateLandLocked(data, now)
			if !found {
				if !e.lastActionAt.IsZero() && now.Sub(e.lastActionAt) < time.Minute {
					e.unmatchedLands++
					slog.Debug("timer-debug: land with no matching action evidence",
						"target", data.TargetName, "spell", data.SpellName, "count", e.unmatchedLands)
				}
				continue
			}
			if e.landCorr == nil {
				e.landCorr = make(map[uint64]landCorrelation)
			}
			e.landCorr[ev.Seq] = c
		}
	}
}

// SetLineSeq records the Seq of the raw line about to be matched by the
// trigger engine (0 once the batch's raw lines are done), so a trigger timer
// started while matching it reads that line's correlation.
func (e *Engine) SetLineSeq(seq uint64) {
	e.mu.Lock()
	e.curLineSeq = seq
	e.mu.Unlock()
}

// EndBatch drops the batch's correlations once both its raw-line and parsed
// dispatch phases have finished.
func (e *Engine) EndBatch() {
	e.mu.Lock()
	e.landCorr = nil
	e.curLineSeq = 0
	e.mu.Unlock()
}

// correlateLandLocked matches one land against the unconsumed recent actions
// with the same target name and a spell the land text could be. The target id
// is assigned when the matches agree on it, or when exactly one of them is a
// confirmed success; otherwise the land is ambiguous and nothing is consumed.
// Spell and source are then each set only if the chosen target's actions
// agree on them. found is false when no action matched at all.
func (e *Engine) correlateLandLocked(data logparser.SpellLandedData, now time.Time) (landCorrelation, bool) {
	c := landCorrelation{target: normalizeNPCName(data.TargetName), candidateIDs: map[int]bool{}}
	names := map[int]string{}
	if data.SpellID > 0 {
		c.candidateIDs[data.SpellID] = true
		names[data.SpellID] = data.SpellName
	}
	for _, cand := range data.Candidates {
		if cand.SpellID > 0 {
			c.candidateIDs[cand.SpellID] = true
			names[cand.SpellID] = cand.SpellName
		}
	}
	if len(c.candidateIDs) == 0 || c.target == "" {
		return c, false
	}
	e.pruneSpellActionsLocked(now)
	var matches []*spellAction
	for _, a := range e.spellActions {
		if !a.consumed && a.target == c.target && c.candidateIDs[a.spellID] {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		return c, false
	}
	targets := map[int]bool{}
	successTarget, successes := 0, 0
	for _, a := range matches {
		targets[a.targetID] = true
		if a.success {
			successes++
			successTarget = a.targetID
		}
	}
	switch {
	case len(targets) == 1:
		c.targetID = matches[0].targetID
	case successes == 1:
		c.targetID = successTarget
	default:
		return c, true // ambiguous: no tag, nothing consumed
	}
	spells, sources := map[int]bool{}, map[int]bool{}
	for _, a := range matches {
		if a.targetID != c.targetID {
			continue
		}
		a.consumed = true
		spells[a.spellID] = true
		sources[a.sourceID] = true
	}
	if len(spells) == 1 {
		for id := range spells {
			c.spellID = id
			c.spellName = names[id]
		}
	}
	if len(sources) == 1 {
		for id := range sources {
			c.sourceID = id
		}
	}
	return c, true
}

// landCorrFor returns the correlation PrimeLands stored for a line.
func (e *Engine) landCorrFor(seq uint64) landCorrelation {
	if seq == 0 {
		return landCorrelation{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.landCorr[seq]
}

// castByMeFor says whether a landed spell was the active character's own
// cast: from the matched action's source when it's known and the pipe gave our
// spawn id, else the cast-history heuristic.
func (e *Engine) castByMeFor(c landCorrelation, spellName, target string, spell *db.Spell) bool {
	if c.sourceID != 0 {
		e.mu.Lock()
		self := e.selfSpawnID
		e.mu.Unlock()
		if self != 0 {
			return c.sourceID == self
		}
	}
	return e.recentSelfCastMatches(spellName, target, isSingleTargetSpellType(spell.TargetType))
}

// selfLandIsMine reports whether a spell that landed on the player came from
// the player's own cast. The log line never names the caster, so this uses,
// in order: the Zeal pipe's spell action source; for a recourse, a recent
// local cast of its parent spell (Aura of Darkness for Aura of Darkness
// Recourse); otherwise a recent local cast of the spell itself. The target
// isn't compared: a self-land is on the player by definition, and the target
// recorded at cast time may be whoever the player had selected. Buffs with no
// "You begin casting" line (item procs) count as someone else's without the
// pipe.
func (e *Engine) selfLandIsMine(c landCorrelation, spellName string, spell *db.Spell) bool {
	if c.sourceID != 0 {
		e.mu.Lock()
		self := e.selfSpawnID
		e.mu.Unlock()
		if self != 0 {
			return c.sourceID == self
		}
	}
	if e.db != nil {
		if parent := e.db.RecourseParent(spell.ID); parent != "" {
			return e.recentSelfCastMatches(parent, "", false)
		}
	}
	return e.recentSelfCastMatches(spellName, "", false)
}

// correlatedToken returns targetName tagged with the spawn id the actions
// identified, or "" when they didn't. Beneficial spells are never tagged (see
// keyTargetTokenLocked).
func correlatedToken(c landCorrelation, spell *db.Spell, targetName string) string {
	if c.targetID == 0 || spell == nil || spell.GoodEffect != 0 || targetName == "" ||
		normalizeNPCName(targetName) != c.target {
		return ""
	}
	return targetName + targetIDKeySep + strconv.Itoa(c.targetID)
}

// triggerCorrelatedTokenLocked is correlatedToken for a trigger timer started
// while the trigger engine matches the current raw line: it applies only when
// the trigger's spell is the correlated spell, or one the land text could be
// when the spell wasn't settled. Caller must hold e.mu.
func (e *Engine) triggerCorrelatedTokenLocked(spell *db.Spell, targetName string) string {
	if e.curLineSeq == 0 || spell == nil {
		return ""
	}
	c, ok := e.landCorr[e.curLineSeq]
	if !ok {
		return ""
	}
	if c.spellID != 0 {
		if spell.ID != c.spellID {
			return ""
		}
	} else if !c.candidateIDs[spell.ID] {
		return ""
	}
	return correlatedToken(c, spell, targetName)
}

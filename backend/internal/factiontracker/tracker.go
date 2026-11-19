package factiontracker

import (
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// NPCFactionHit is one faction's point delta for an NPC's npc_faction_id, as
// looked up in quarm.db (npc_faction_entries joined to faction_list).
type NPCFactionHit struct {
	FactionID   int
	FactionName string
	Value       int
}

// NPCFactionResolver resolves a killed mob's display name (as it appears in
// the kill log line) to its quarm.db faction hits, best-effort. Returns
// ok=false when the name can't be resolved to an NPC or the NPC carries no
// faction hits. Injected so the tracker stays decoupled from the game DB.
type NPCFactionResolver func(mobName string) (hits []NPCFactionHit, ok bool)

// NPCPrimaryFactionResolver resolves a /con'd NPC's display name to its
// primary faction (npc_faction.primaryfaction) — the faction /con's
// disposition message actually reflects. Returns ok=false when the name
// can't be resolved or the NPC has no faction.
type NPCPrimaryFactionResolver func(npcName string) (factionID int, factionName string, ok bool)

// FactionIDResolver resolves a faction name (as named in a "Your faction
// standing with X…" line) to its quarm.db faction_list id, best-effort. Used
// only to fill in FactionID the first time a faction is seen via a kill-driven
// change with no prior /con reading to have already established it. Returns
// ok=false when the name can't be resolved (FactionID is left 0 in that case
// — cosmetic only, doesn't affect tallying).
type FactionIDResolver func(name string) (id int, ok bool)

// QuestDialogueResolver resolves an NPC's spoken log line (from
// EventNPCDialogue) to the exact faction deltas its matching quest-script
// dialogue branch applies, best-effort. Returns ok=false when the NPC has no
// faction-bearing quest dialogue at all, or none of it matches sayText — the
// overwhelming majority of ordinary NPC hails and all player chat, since
// EventNPCDialogue fires on any "<Name> says, '...'" line, not just quest
// turn-ins.
type QuestDialogueResolver func(npcName, sayText string) (hits []NPCFactionHit, ok bool)

// IsIllusionedProvider reports whether the active character currently has an
// illusion effect active. Illusions change how NPCs perceive the player, so
// a /con reading taken while illusioned is flagged as suspect rather than
// trusted at face value.
type IsIllusionedProvider func() bool

// ConsiderContext describes one /con reading to the suspect and pet checks.
type ConsiderContext struct {
	NPCName string
	// SpawnID is the considered spawn's id when the reading came from the Zeal
	// pipe, 0 for a log reading.
	SpawnID int
	Bucket  logparser.FactionBucket
}

// ConsiderSuspectProvider reports whether a /con reading isn't a clean read
// of standing because of the player's state at the time: invisible to
// anything, hidden, feigned, a faction buff active, or engaged with the NPC.
// The server answers all of those with an override (indifferent,
// threatening) or an adjusted value, not the standing itself.
type ConsiderSuspectProvider func(c ConsiderContext) bool

// ConsiderPetProvider reports whether the considered NPC is a pet. A pet
// cons amiable to its owner and indifferent to everyone else, so its reading
// says nothing about its faction and is dropped.
type ConsiderPetProvider func(c ConsiderContext) bool

// considerDedupeWindow is how far apart the log line and the pipe event for
// one /con can arrive.
const considerDedupeWindow = 2 * time.Second

// PersistFunc is called after every tally mutation with the full current
// state of that one faction's tally, so the caller can write it to durable
// storage. Called outside the engine's lock.
type PersistFunc func(characterID int, tally Tally)

// ClearPersistedFunc is called by Reset to wipe durable storage for the
// character — Reset means "discard this character's faction tracking
// history," not "start a fresh session," since tallies now persist across
// restarts.
type ClearPersistedFunc func(characterID int)

// pendingKill is a resolved kill's expected faction deltas, waiting to be
// matched against the EventFactionChanged lines that should follow within
// correlationWindow. hits is keyed by lowercased faction name; a matched
// entry is deleted so a multi-faction kill's lines each consume their own
// faction once.
type pendingKill struct {
	at   time.Time
	hits map[string]int
}

// Engine tracks per-character faction-standing changes for every faction the
// character has ever killed toward or /con'd — the same "record everything
// encountered" approach as the Lockout and Player trackers. Which factions
// are pinned to a wishlist is the caller's concern (character_faction_
// wishlist), resolved entirely outside the engine; it never gates what gets
// recorded here. Safe for concurrent use.
type Engine struct {
	hub              *ws.Hub
	resolve          NPCFactionResolver
	resolvePrimary   NPCPrimaryFactionResolver
	resolveFactionID FactionIDResolver
	resolveDialogue  QuestDialogueResolver
	isIllusioned     IsIllusionedProvider
	isSuspect        ConsiderSuspectProvider
	isPet            ConsiderPetProvider
	resolveInfo      FactionInfoResolver
	persist          PersistFunc
	clearPersisted   ClearPersistedFunc

	// now and after are the wall clock used to pair a log /con with its pipe
	// event (both arrive for one /con, the log's own timestamp only has
	// second resolution). Swapped in tests.
	now   func() time.Time
	after func(time.Duration, func())

	mu          sync.Mutex
	characterID int
	tallies     map[string]*Tally // key: strings.ToLower(FactionName)
	pending     []pendingKill

	// infoCache memoizes resolveInfo per faction id for the current
	// character (cleared on SetCharacter). recent holds recent attributed
	// changes per faction key and direction ("+"/"-" suffix) for inferring
	// the size of unattributed ones.
	infoCache map[int]*FactionInfo
	recent    map[string][]recentHit

	// pipeSeen is set by the first pipe consider this Zeal session; from then
	// on the pipe is the primary /con source and a log reading only counts
	// if no pipe event for that NPC turns up within considerDedupeWindow.
	// pipeConsiders maps a lowercased NPC name to its last pipe consider's
	// arrival.
	pipeSeen      bool
	pipeConsiders map[string]time.Time
}

// NewEngine returns an initialized Engine tracking no character yet. Call
// SetCharacter once the active character is known.
func NewEngine(hub *ws.Hub, resolve NPCFactionResolver) *Engine {
	return &Engine{
		hub:           hub,
		resolve:       resolve,
		tallies:       make(map[string]*Tally),
		infoCache:     make(map[int]*FactionInfo),
		recent:        make(map[string][]recentHit),
		now:           time.Now,
		after:         func(d time.Duration, fn func()) { time.AfterFunc(d, fn) },
		pipeConsiders: make(map[string]time.Time),
	}
}

// SetPrimaryFactionResolver registers the resolver used to correlate /con
// readings to a faction. Optional — /con correlation is skipped entirely if
// never set.
func (e *Engine) SetPrimaryFactionResolver(fn NPCPrimaryFactionResolver) {
	e.mu.Lock()
	e.resolvePrimary = fn
	e.mu.Unlock()
}

// SetFactionIDResolver registers the resolver used to fill in FactionID the
// first time a faction is seen via a kill-driven change. Optional — FactionID
// stays 0 (cosmetic only) if never set or the name can't be resolved.
func (e *Engine) SetFactionIDResolver(fn FactionIDResolver) {
	e.mu.Lock()
	e.resolveFactionID = fn
	e.mu.Unlock()
}

// SetQuestDialogueResolver registers the resolver used to correlate an NPC's
// spoken log line to a quest-script dialogue branch's exact faction deltas.
// Optional — quest-turn-in faction changes fall back to direction-only
// (Unresolved) tallying if never set, same as any other unresolvable change.
func (e *Engine) SetQuestDialogueResolver(fn QuestDialogueResolver) {
	e.mu.Lock()
	e.resolveDialogue = fn
	e.mu.Unlock()
}

// SetIllusionProvider registers the callback used to flag /con readings
// taken while illusioned. Optional — readings are never flagged suspect if
// never set.
func (e *Engine) SetIllusionProvider(fn IsIllusionedProvider) {
	e.mu.Lock()
	e.isIllusioned = fn
	e.mu.Unlock()
}

// SetConsiderSuspectProvider registers the callback used to flag /con
// readings taken while the player's state overrides the server's answer.
// Optional, and checked in addition to the illusion provider.
func (e *Engine) SetConsiderSuspectProvider(fn ConsiderSuspectProvider) {
	e.mu.Lock()
	e.isSuspect = fn
	e.mu.Unlock()
}

// SetConsiderPetProvider registers the callback used to drop /con readings
// of pets. Optional — no reading is dropped as a pet if never set.
func (e *Engine) SetConsiderPetProvider(fn ConsiderPetProvider) {
	e.mu.Lock()
	e.isPet = fn
	e.mu.Unlock()
}

// HandlePipeConsider records a consider reading from the Zeal pipe (the
// proposed consider message): the considered spawn's id and name and the
// faction level, 1 ally .. 9 scowls. A level outside 1..9 is ignored. It
// takes over from the log /con line for the same consider, which is dropped
// (see Handle).
func (e *Engine) HandlePipeConsider(spawnID int, npcName string, level int, ts time.Time) {
	bucket := logparser.FactionBucketForLevel(level)
	if bucket == "" || npcName == "" {
		return
	}
	key := strings.ToLower(npcName)
	e.mu.Lock()
	arrived := e.now()
	e.pipeSeen = true
	for k, at := range e.pipeConsiders {
		if arrived.Sub(at) > considerDedupeWindow {
			delete(e.pipeConsiders, k)
		}
	}
	e.pipeConsiders[key] = arrived
	e.mu.Unlock()

	c := ConsiderContext{NPCName: npcName, SpawnID: spawnID, Bucket: bucket}
	if suspect, ok := e.checkConsider(c); ok {
		e.applyConsidered(c, suspect, ts)
	}
}

// ResetPipe forgets that the pipe has been sending considers, so log /con
// readings count straight away again. Called when Zeal disconnects.
func (e *Engine) ResetPipe() {
	e.mu.Lock()
	e.pipeSeen = false
	e.pipeConsiders = make(map[string]time.Time)
	e.mu.Unlock()
}

// SetFactionInfoResolver registers the resolver for a faction's caps and the
// active character's modifier. Optional — without it no standing range is
// estimated. Clears the cache, so call it again (or SetCharacter) when the
// character's race, class or deity becomes known.
func (e *Engine) SetFactionInfoResolver(fn FactionInfoResolver) {
	e.mu.Lock()
	e.resolveInfo = fn
	e.infoCache = make(map[int]*FactionInfo)
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// infoLocked returns the faction's info, or nil when unknown. Must be
// called with mu held.
func (e *Engine) infoLocked(factionID int) *FactionInfo {
	if factionID == 0 || e.resolveInfo == nil {
		return nil
	}
	if fi, ok := e.infoCache[factionID]; ok {
		return fi
	}
	var fi *FactionInfo
	if info, ok := e.resolveInfo(e.characterID, factionID); ok {
		fi = &info
	}
	e.infoCache[factionID] = fi
	return fi
}

// FillInfo sets Info on tallies read straight from storage (a non-active
// character's history), uncached.
func (e *Engine) FillInfo(characterID int, tallies []Tally) {
	e.mu.Lock()
	fn := e.resolveInfo
	e.mu.Unlock()
	if fn == nil {
		return
	}
	for i := range tallies {
		if tallies[i].FactionID == 0 {
			continue
		}
		if info, ok := fn(characterID, tallies[i].FactionID); ok {
			tallies[i].Info = &info
		}
	}
}

// SetPersistFunc registers the callback invoked after every tally mutation.
// Optional — the engine works in-memory-only if never set (e.g. tests).
func (e *Engine) SetPersistFunc(fn PersistFunc) {
	e.mu.Lock()
	e.persist = fn
	e.mu.Unlock()
}

// SetClearPersistedFunc registers the callback invoked by Reset to wipe
// durable storage. Optional.
func (e *Engine) SetClearPersistedFunc(fn ClearPersistedFunc) {
	e.mu.Lock()
	e.clearPersisted = fn
	e.mu.Unlock()
}

// SetCharacter switches the engine to a new active character, seeding its
// in-memory tallies from every persisted row the caller loaded for that
// character — not filtered to a wishlist; every faction with any recorded
// history comes back. Called on startup and whenever the active character
// changes. Pending kill correlations are dropped: they're specific to
// whichever character's log produced them.
func (e *Engine) SetCharacter(characterID int, tallies []Tally) {
	e.mu.Lock()
	e.characterID = characterID
	newTallies := make(map[string]*Tally, len(tallies))
	for _, t := range tallies {
		tc := t
		newTallies[strings.ToLower(t.FactionName)] = &tc
	}
	e.tallies = newTallies
	e.pending = nil
	e.infoCache = make(map[int]*FactionInfo)
	e.recent = make(map[string][]recentHit)
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// Reset zeroes every recorded faction's tally (including the /con reading)
// and drops pending kill correlations, then wipes durable storage for the
// character via ClearPersistedFunc — an explicit, user-initiated "discard my
// faction tracking history," not something that happens automatically on
// restart or character switch.
func (e *Engine) Reset() {
	e.mu.Lock()
	charID := e.characterID
	for _, t := range e.tallies {
		t.Better, t.Worse, t.EstimatedNet, t.Unresolved = 0, 0, 0, 0
		t.LastBucket, t.LastConsideredAt, t.LastConsiderSuspect = "", nil, false
		t.Inferred, t.MaxStep = 0, 0
		t.clearRange()
	}
	e.pending = nil
	e.recent = make(map[string][]recentHit)
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.clearPersisted != nil {
		e.clearPersisted(charID)
	}
}

// State returns a snapshot of every faction with recorded activity for the
// current character — not just pinned ones. Callers (the API/frontend) merge
// this against the wishlist to know which are pinned.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stateLocked()
}

// Handle processes a parsed log event, updating tallies and pending-kill
// correlations as needed.
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventKill:
		data, ok := ev.Data.(logparser.KillData)
		if !ok || data.Target == "" {
			return
		}
		e.handleKill(data.Target, ev.Timestamp)
	case logparser.EventFactionChanged:
		data, ok := ev.Data.(logparser.FactionChangedData)
		if !ok {
			return
		}
		e.handleFactionChanged(data.Faction, data.Direction, data.AtCap, ev.Timestamp)
	case logparser.EventConsidered:
		data, ok := ev.Data.(logparser.ConsideredData)
		if !ok || data.Bucket == "" {
			return
		}
		e.handleConsidered(data.TargetName, data.Bucket, ev.Timestamp)
	case logparser.EventNPCDialogue:
		data, ok := ev.Data.(logparser.NPCDialogueData)
		if !ok {
			return
		}
		e.handleNPCDialogue(data.NPCName, data.Text, ev.Timestamp)
	}
}

// handleKill resolves the slain mob to its DB faction hits and stashes them
// as a pending correlation. No-op (and no lock taken) if the resolver can't
// place the mob at all, since the resolver call is the expensive part.
func (e *Engine) handleKill(target string, ts time.Time) {
	if e.resolve == nil {
		return
	}
	hits, ok := e.resolve(target)
	if !ok || len(hits) == 0 {
		return
	}
	m := make(map[string]int, len(hits))
	for _, h := range hits {
		// A zero-value entry never produces a "got better/worse" line (the
		// server doesn't log a no-op change), so it can never be matched —
		// skip it rather than let it dilute the pending map.
		if h.Value == 0 {
			continue
		}
		m[strings.ToLower(h.FactionName)] = h.Value
	}
	if len(m) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, pendingKill{at: ts, hits: m})
	e.gcPendingLocked(ts)
}

// handleNPCDialogue resolves an NPC's spoken line to a matching quest-script
// dialogue branch and stashes its exact faction deltas as a pending
// correlation — the same mechanism handleKill uses for DB-resolved kill
// hits, so handleFactionChanged's matching loop picks these up unchanged.
// No-op if the resolver can't place the line at all (the overwhelming
// majority of say-lines: ordinary hails, player chat, non-quest dialogue).
func (e *Engine) handleNPCDialogue(npcName, text string, ts time.Time) {
	if e.resolveDialogue == nil {
		return
	}
	hits, ok := e.resolveDialogue(npcName, text)
	if !ok || len(hits) == 0 {
		return
	}
	m := make(map[string]int, len(hits))
	for _, h := range hits {
		if h.Value == 0 {
			continue
		}
		m[strings.ToLower(h.FactionName)] = h.Value
	}
	if len(m) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, pendingKill{at: ts, hits: m})
	e.gcPendingLocked(ts)
}

// tallyLocked returns the tally for key, creating a zero-value entry (with
// the given display name/id) the first time this faction is ever seen. Must
// be called with mu held.
func (e *Engine) tallyLocked(key, factionName string, factionID int) *Tally {
	if t, ok := e.tallies[key]; ok {
		return t
	}
	t := &Tally{FactionID: factionID, FactionName: factionName}
	e.tallies[key] = t
	return t
}

// handleFactionChanged tallies a "got better/worse" line for whichever
// faction it names — creating a new tally entry the first time this faction
// is seen — and, if a pending kill within correlationWindow has a matching
// signed hit for it, attributes its point value as an estimate. With no
// match, recent attributed changes to the same faction may still supply the
// size (Inferred). A capped line ("could not possibly get any…") changes
// nothing but pins the standing estimate to the faction's cap.
func (e *Engine) handleFactionChanged(factionName, direction string, atCap bool, ts time.Time) {
	key := strings.ToLower(factionName)
	better := direction == "better"

	e.mu.Lock()
	factionID := 0
	if t, exists := e.tallies[key]; (!exists || t.FactionID == 0) && e.resolveFactionID != nil {
		factionID, _ = e.resolveFactionID(factionName)
	}
	tally := e.tallyLocked(key, factionName, factionID)
	if tally.FactionID == 0 && factionID != 0 {
		tally.FactionID = factionID
	}
	info := e.infoLocked(tally.FactionID)

	e.gcPendingLocked(ts)
	// Newest-first so a burst of identical rapid kills (e.g. a fast-respawning
	// script encounter) consumes its own most recent hit rather than an older
	// one that a later line might still need.
	val, matched := 0, false
	for i := len(e.pending) - 1; i >= 0; i-- {
		v, ok := e.pending[i].hits[key]
		if !ok {
			continue
		}
		if (better && v > 0) || (!better && v < 0) {
			val, matched = v, true
			delete(e.pending[i].hits, key)
			break
		}
	}

	switch {
	case atCap:
		// The hit was clamped to nothing: personal faction sits exactly at
		// the cap. Not a change, so better/worse are left alone.
		if info != nil {
			c := info.MinCap
			if better {
				c = info.MaxCap
			}
			tally.setRange(c, c, RangeSourceCap, ts)
		}
	case matched:
		e.countLocked(tally, better)
		tally.EstimatedNet += val
		if a := abs(val); a > tally.MaxStep {
			tally.MaxStep = a
		}
		e.rememberLocked(key, val, ts)
		if info != nil {
			tally.shiftRange(val, *info)
		}
	default:
		e.countLocked(tally, better)
		if v, ok := e.inferLocked(key, better, ts); ok {
			tally.EstimatedNet += v
			tally.Inferred++
			if info != nil {
				tally.shiftRange(v, *info)
			}
		} else {
			tally.Unresolved++
			if info != nil {
				tally.widenRange(better, *info)
			}
		}
	}
	snapshot := *tally
	charID := e.characterID
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.persist != nil {
		e.persist(charID, snapshot)
	}
}

func (e *Engine) countLocked(t *Tally, better bool) {
	if better {
		t.Better++
	} else {
		t.Worse++
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func recentKey(key string, positive bool) string {
	if positive {
		return key + "+"
	}
	return key + "-"
}

// rememberLocked records an attributed change for later inference. Must be
// called with mu held.
func (e *Engine) rememberLocked(key string, v int, ts time.Time) {
	rk := recentKey(key, v > 0)
	hits := append(e.recent[rk], recentHit{value: v, at: ts})
	if len(hits) > inferKeep {
		hits = hits[len(hits)-inferKeep:]
	}
	e.recent[rk] = hits
}

// inferLocked returns the size of an unattributed change when every
// attributed change to the same faction and direction within inferWindow
// had the same value — e.g. a raid clearing one kind of mob, most of
// whose deaths the player doesn't see logged. Must be called with mu held.
func (e *Engine) inferLocked(key string, better bool, ts time.Time) (int, bool) {
	hits := e.recent[recentKey(key, better)]
	val, n := 0, 0
	for _, h := range hits {
		if ts.Sub(h.at) > inferWindow || h.at.After(ts.Add(correlationWindow)) {
			continue
		}
		if n > 0 && h.value != val {
			return 0, false
		}
		val = h.value
		n++
	}
	return val, n > 0
}

// handleConsidered takes a log /con reading. Before any pipe consider has
// arrived it's recorded straight away. Once the pipe is sending considers the
// matching pipe event is the better reading, so the log one is dropped if a
// pipe event for the same NPC arrived within considerDedupeWindow, and
// otherwise held that long in case it's still coming. The player's state is
// checked on arrival either way.
func (e *Engine) handleConsidered(npcName string, bucket logparser.FactionBucket, ts time.Time) {
	key := strings.ToLower(npcName)
	e.mu.Lock()
	arrived := e.now()
	pipeSeen := e.pipeSeen
	at, dup := e.pipeConsiders[key]
	e.mu.Unlock()
	if dup && arrived.Sub(at) <= considerDedupeWindow {
		return
	}

	c := ConsiderContext{NPCName: npcName, Bucket: bucket}
	suspect, ok := e.checkConsider(c)
	if !ok {
		return
	}
	if !pipeSeen {
		e.applyConsidered(c, suspect, ts)
		return
	}
	e.after(considerDedupeWindow, func() {
		e.mu.Lock()
		at, dup := e.pipeConsiders[key]
		e.mu.Unlock()
		if dup && !at.Before(arrived.Add(-considerDedupeWindow)) {
			return
		}
		e.applyConsidered(c, suspect, ts)
	})
}

// checkConsider runs the pet and suspect checks for a reading. ok is false
// when the reading should be dropped (a pet).
func (e *Engine) checkConsider(c ConsiderContext) (suspect, ok bool) {
	e.mu.Lock()
	isPet, isSuspect, isIllusioned := e.isPet, e.isSuspect, e.isIllusioned
	e.mu.Unlock()
	if isPet != nil && isPet(c) {
		return false, false
	}
	if isIllusioned != nil && isIllusioned() {
		suspect = true
	}
	if !suspect && isSuspect != nil {
		suspect = isSuspect(c)
	}
	return suspect, true
}

// applyConsidered resolves a /con'd NPC to its primary faction — creating a
// new tally entry the first time this faction is seen — and records the
// disposition bucket as its latest reading, flagged suspect when the
// player's state at the time kept it from being a clean read.
func (e *Engine) applyConsidered(c ConsiderContext, suspect bool, ts time.Time) {
	if e.resolvePrimary == nil {
		return
	}
	factionID, factionName, ok := e.resolvePrimary(c.NPCName)
	if !ok {
		return
	}
	key := strings.ToLower(factionName)

	e.mu.Lock()
	tally := e.tallyLocked(key, factionName, factionID)
	if tally.FactionID == 0 && factionID != 0 {
		tally.FactionID = factionID
	}
	tally.LastBucket = string(c.Bucket)
	tally.LastConsideredAt = &ts
	tally.LastConsiderSuspect = suspect
	// Only a clean reading with an exact modifier says where personal
	// faction is.
	if info := e.infoLocked(tally.FactionID); !suspect && info != nil && info.ModifierExact {
		tally.applyCon(c.Bucket, *info, ts)
	}
	snapshot := *tally
	charID := e.characterID
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.persist != nil {
		e.persist(charID, snapshot)
	}
}

// gcPendingLocked drops pending kills older than correlationWindow relative
// to now, and caps the backlog at maxPendingKills. Must be called with mu
// held.
func (e *Engine) gcPendingLocked(now time.Time) {
	cutoff := now.Add(-correlationWindow)
	kept := e.pending[:0]
	for _, p := range e.pending {
		if p.at.After(cutoff) {
			kept = append(kept, p)
		}
	}
	e.pending = kept
	if len(e.pending) > maxPendingKills {
		e.pending = e.pending[len(e.pending)-maxPendingKills:]
	}
}

func (e *Engine) stateLocked() State {
	out := make([]Tally, 0, len(e.tallies))
	for _, t := range e.tallies {
		tc := *t
		if fi := e.infoLocked(t.FactionID); fi != nil {
			info := *fi
			tc.Info = &info
		}
		out = append(out, tc)
	}
	return State{Tallies: out}
}

func (e *Engine) broadcast(state State) {
	if e.hub == nil {
		return
	}
	e.hub.Broadcast(ws.Event{Type: WSEventFactions, Data: state})
}

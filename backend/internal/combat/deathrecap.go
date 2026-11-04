package combat

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// deathRecapWindow is how far back a death recap looks.
const deathRecapWindow = 20 * time.Second

// maxRecapEvents bounds the incoming-event buffer. A raid AE can put dozens
// of lines a second on the player; the recap keeps the newest ones.
const maxRecapEvents = 400

// Recap event kinds.
const (
	RecapHit  = "hit"  // damage taken
	RecapMiss = "miss" // an attack on the player that missed or was avoided
	RecapHeal = "heal" // healing received
	RecapCH   = "ch"   // a Complete Heal landed on the player
)

// completeHealSpells are the spells counted as a CH in a death recap. Both
// share the "You are completely healed." land text.
var completeHealSpells = map[string]bool{
	"Complete Healing": true,
	"Complete Heal":    true,
}

// RecapEvent is one incoming event in a death recap.
type RecapEvent struct {
	// Offset is seconds before the death (zero or negative).
	Offset float64 `json:"offset"`
	Kind   string  `json:"kind"`
	// Source is the attacker or healer. Empty for a CH, whose land line
	// doesn't name the caster.
	Source string `json:"source,omitempty"`
	// Ability is the attack verb, spell name or miss type.
	Ability string `json:"ability,omitempty"`
	Amount  int    `json:"amount,omitempty"`
}

// DeathRecap summarises the deathRecapWindow before a death.
type DeathRecap struct {
	WindowSeconds float64 `json:"window_seconds"`
	DamageTaken   int64   `json:"damage_taken"`
	Hits          int     `json:"hits"`
	Misses        int     `json:"misses"`
	BiggestHit    int     `json:"biggest_hit"`
	HealingTaken  int64   `json:"healing_taken"`
	Heals         int     `json:"heals"`
	CHs           int     `json:"chs"`
	// SinceLastCH is seconds from the last CH on the player to the death,
	// looking back past the window. Nil when no CH landed this session.
	SinceLastCH *float64 `json:"since_last_ch,omitempty"`
	// Killer is the slain-by name, or the last attacker for "You died.".
	Killer string       `json:"killer,omitempty"`
	Events []RecapEvent `json:"events"`
}

// noteIncomingLocked buffers an event that targets the player. Must be
// called with t.mu held.
func (t *Tracker) noteIncomingLocked(at time.Time, ev RecapEvent) {
	t.recapBuf = append(t.recapBuf, recapEntry{at: at, ev: ev})
	cut := 0
	for cut < len(t.recapBuf) && at.Sub(t.recapBuf[cut].at) > deathRecapWindow {
		cut++
	}
	if over := len(t.recapBuf) - cut - maxRecapEvents; over > 0 {
		cut += over
	}
	if cut > 0 {
		t.recapBuf = append(t.recapBuf[:0], t.recapBuf[cut:]...)
	}
}

type recapEntry struct {
	at time.Time
	ev RecapEvent
}

// noteRecapEvent feeds the death recap buffer from one log event.
func (t *Tracker) noteRecapEvent(ev logparser.LogEvent) {
	var re RecapEvent
	switch d := ev.Data.(type) {
	case logparser.CombatHitData:
		if d.Target != "You" {
			return
		}
		ability := d.Skill
		if d.SpellName != "" {
			ability = d.SpellName
		}
		re = RecapEvent{Kind: RecapHit, Source: canonicalNPCName(d.Actor), Ability: ability, Amount: d.Damage}
	case logparser.CombatMissData:
		if d.Target != "You" {
			return
		}
		re = RecapEvent{Kind: RecapMiss, Source: canonicalNPCName(d.Actor), Ability: d.MissType}
	case logparser.HealData:
		if d.Target != "You" {
			return
		}
		re = RecapEvent{Kind: RecapHeal, Source: d.Actor, Amount: d.Amount}
	case logparser.SpellLandedData:
		if d.Kind != logparser.SpellLandedKindYou || !isCompleteHeal(d) {
			return
		}
		re = RecapEvent{Kind: RecapCH, Ability: "Complete Healing"}
	default:
		return
	}
	t.mu.Lock()
	if re.Kind == RecapCH {
		t.lastCHAt = ev.Timestamp
	}
	t.noteIncomingLocked(ev.Timestamp, re)
	t.mu.Unlock()
}

func isCompleteHeal(d logparser.SpellLandedData) bool {
	if completeHealSpells[d.SpellName] {
		return true
	}
	for _, c := range d.Candidates {
		if completeHealSpells[c.SpellName] {
			return true
		}
	}
	return false
}

// buildDeathRecapLocked summarises the buffer for a death at `at` and
// clears it, so a quick second death doesn't repeat the first one's events.
// Must be called with t.mu held.
func (t *Tracker) buildDeathRecapLocked(at time.Time, slainBy string) *DeathRecap {
	r := &DeathRecap{
		WindowSeconds: deathRecapWindow.Seconds(),
		Killer:        slainBy,
		Events:        []RecapEvent{},
	}
	lastAttacker := ""
	for _, e := range t.recapBuf {
		d := at.Sub(e.at)
		if d > deathRecapWindow || d < 0 {
			continue
		}
		ev := e.ev
		ev.Offset = -d.Seconds()
		r.Events = append(r.Events, ev)
		switch ev.Kind {
		case RecapHit:
			r.DamageTaken += int64(ev.Amount)
			r.Hits++
			if ev.Amount > r.BiggestHit {
				r.BiggestHit = ev.Amount
			}
			lastAttacker = ev.Source
		case RecapMiss:
			r.Misses++
		case RecapHeal:
			r.HealingTaken += int64(ev.Amount)
			r.Heals++
		case RecapCH:
			r.CHs++
		}
	}
	if r.Killer == "" {
		r.Killer = lastAttacker
	}
	if !t.lastCHAt.IsZero() && !t.lastCHAt.After(at) {
		s := at.Sub(t.lastCHAt).Seconds()
		r.SinceLastCH = &s
	}
	t.recapBuf = t.recapBuf[:0]
	return r
}

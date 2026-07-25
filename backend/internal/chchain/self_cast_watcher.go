package chchain

import (
	"regexp"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// WSEventSelfCast is broadcast when the local player begins casting a
// recognized CH-chain heal spell, so the CH Metronome can show a confirmed
// "cast sent" state instead of just assuming the cast happened once its
// countdown elapses.
const WSEventSelfCast = "ch_metronome:self_cast"

// SelfCastEvent is the WSEventSelfCast payload.
type SelfCastEvent struct {
	SpellName string    `json:"spell_name"`
	CastAt    time.Time `json:"cast_at"`
}

// reBeginCastSelfNamed matches "You begin casting <SpellName>." and captures
// the spell name. Unlike reBeginCastSelf (cast_watcher.go), which only needs
// to know THAT the local player began a cast, this needs to know WHICH spell
// so it can be filtered to known CH-chain heals — the local player's own
// cast-begin line, unlike a bystander's, actually reveals the spell name.
var reBeginCastSelfNamed = regexp.MustCompile(`^You begin casting (.+)\.$`)

// chChainHealSpells are the known Complete-Heal-chain heal spells whose
// self-cast confirms a metronome cycle: Cleric Complete Healing and Druid
// Tunare's Renewal (Luclin) / Karana's Renewal (PoP) — the same spells
// CastWatcher/Matcher already document for chain-timer confirmation.
var chChainHealSpells = map[string]bool{
	"Complete Healing": true,
	"Tunare's Renewal": true,
	"Karana's Renewal": true,
}

// Broadcaster is the subset of ws.Hub SelfCastWatcher needs. (*ws.Hub).
// Broadcast satisfies this directly; tests supply a fake to observe events
// without a real Hub/client connection.
type Broadcaster interface {
	Broadcast(ws.Event)
}

// SelfCastWatcher watches raw log lines for the local player beginning to
// cast a CH-chain heal and broadcasts WSEventSelfCast so the CH Metronome can
// confirm the current cycle's cast instead of just assuming it once its
// countdown elapses. Purely additive/informational: it never creates,
// modifies, or removes a timer, and the frontend only ever uses it to turn an
// existing "cast sent" state into a confirmed one — never to flag a miss.
// That asymmetry is deliberate: an incomplete spell name here would
// otherwise show a false "missed" flag on every cycle for a guild whose
// macro casts something not in chChainHealSpells, unlike the chain matcher's
// possible-miss flag, which has independent grounding in a real per-timer
// expiry rather than a spell-name whitelist.
type SelfCastWatcher struct {
	hub Broadcaster
	cfg func() config.CHChainSettings
}

// NewSelfCastWatcher constructs a SelfCastWatcher broadcasting through hub,
// reading live settings via cfg.
func NewSelfCastWatcher(hub Broadcaster, cfg func() config.CHChainSettings) *SelfCastWatcher {
	return &SelfCastWatcher{hub: hub, cfg: cfg}
}

// HandleLine checks one raw log line for a self-cast of a known CH-chain
// heal and broadcasts a confirmation event on a match.
func (w *SelfCastWatcher) HandleLine(ts time.Time, msg string) {
	if !w.cfg().Enabled {
		return
	}
	m := reBeginCastSelfNamed.FindStringSubmatch(msg)
	if m == nil || !chChainHealSpells[m[1]] {
		return
	}
	w.hub.Broadcast(ws.Event{Type: WSEventSelfCast, Data: SelfCastEvent{SpellName: m[1], CastAt: ts}})
}

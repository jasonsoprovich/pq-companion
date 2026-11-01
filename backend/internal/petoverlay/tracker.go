// Package petoverlay tracks the active character's pet — charmed or summoned —
// for the Pet overlay: who it is, its HP, whether it is a charm, the npc_types
// row behind it (scaled stats, immunities, summon/gate), and the latest
// `/pet health` effect list. It broadcasts a pet:state WebSocket event on every
// meaningful change.
//
// Sources, best first:
//   - Zeal pipe: pet_id (MsgPlayer, v1.4.6+) is the authoritative "a pet
//     exists / it's gone" signal; label 68 is its name and label 69 its HP%.
//   - Log: "Attacking X Master." tells and "My leader is <you>." bind a pet
//     name; "Your charm spell has worn off." breaks a charm; `/pet health`
//     replies give HP and the full effect list.
//
// The npc_types row comes from the NPC overlay's spawn-id resolution cache (the
// pet was almost always targeted when it was charmed, and target_id == pet_id),
// falling back to a by-name lookup in the current zone.
package petoverlay

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
)

// WSEventPetState is the WebSocket event type carrying a State.
const WSEventPetState = "pet:state"

// Mode is the overlay's top-level state.
type Mode string

const (
	// ModeNone: no pet. The panel shows the charm finder (charm classes) or
	// an empty state.
	ModeNone Mode = "none"
	// ModePet: a pet is up.
	ModePet Mode = "pet"
	// ModeBroken: a charm just broke (or the pet just died). The panel keeps
	// the old pet's card in a BROKE state until SwapAt, then returns to none.
	ModeBroken Mode = "broken"
)

// Kind distinguishes a charmed pet from a summoned one.
type Kind string

const (
	KindUnknown  Kind = ""
	KindCharm    Kind = "charm"
	KindSummoned Kind = "summoned"
)

// HP sources.
const (
	HPSourceZeal      = "zeal"
	HPSourcePetHealth = "pet_health"
)

// DefaultSwapDelay is how long ModeBroken lasts before returning to ModeNone.
const DefaultSwapDelay = 10 * time.Second

// petLossMissThreshold mirrors spelltimer's debounce of the same name: Zeal
// omits pet_id for a single frame around zone lines and gates, so a pet only
// counts as gone after this many consecutive nil observations (~300 ms).
const petLossMissThreshold = 3

// zealHPFresh is how long a Zeal pet-HP reading outranks a `/pet health`
// reading. Labels resend at ~10 Hz while a pet is up, so anything older means
// the feed stopped.
const zealHPFresh = 2 * time.Second

// hpBroadcastInterval rate-limits HP-only broadcasts. Labels resend the same
// value continuously, so a skipped intermediate value is always followed by
// the settled one.
const hpBroadcastInterval = 250 * time.Millisecond

// kindSettle is how long a new pet with no charm evidence stays KindUnknown
// before it is called summoned. A charm's timer (trigger-driven) and the pipe's
// pet_id arrive within a beat of each other, in either order.
const kindSettle = 3 * time.Second

// nameFallbackDelay holds off the by-name lookup while the pipe is connected:
// label 68 (the name) lands a beat before pet_id, and the spawn-id cache gives
// a better pick. Zeal builds without pet_id still fall back after this.
const nameFallbackDelay = time.Second

// healthFlushDelay closes a `/pet health` block when no later line arrives to
// close it (quiet log).
const healthFlushDelay = 1500 * time.Millisecond

// State is the pet:state payload.
type State struct {
	Mode Mode `json:"mode"`
	Kind Kind `json:"kind"`

	// PetID is the Zeal spawn id (nil without Zeal v1.4.6+).
	PetID *int   `json:"pet_id,omitempty"`
	Name  string `json:"name,omitempty"`

	HPPercent *int       `json:"hp_percent,omitempty"`
	HPSource  string     `json:"hp_source,omitempty"`
	HPAt      *time.Time `json:"hp_at,omitempty"`

	// Stats is nil until the pet's npc_types row resolves (always nil for
	// summoned pets until their lookup path lands). Variants > 1 marks an
	// ambiguous same-name pick.
	Stats    *Stats `json:"stats,omitempty"`
	Variants int    `json:"variants,omitempty"`

	// CharmSpell is the active charm timer's spell, when one is running.
	CharmSpell string `json:"charm_spell,omitempty"`

	// Health is the latest `/pet health` reply for this pet.
	Health *HealthSnapshot `json:"health,omitempty"`

	BrokeAt *time.Time `json:"broke_at,omitempty"`
	SwapAt  *time.Time `json:"swap_at,omitempty"`

	// ZealLimited is true while no Zeal pipe is connected: pet death can't be
	// detected promptly and HP comes only from `/pet health`.
	ZealLimited bool `json:"zeal_limited"`

	UpdatedAt time.Time `json:"updated_at"`
}

// Resolver looks up the pet's npc_types row. *overlay.NPCTracker satisfies it.
type Resolver interface {
	ResolvedForID(id int) (overlay.Resolution, bool)
	ResolveByName(displayName string) overlay.Resolution
}

// Config wires a Tracker's collaborators. Every func is optional.
type Config struct {
	Resolver Resolver
	// Broadcast publishes a State (typically hub.Broadcast of WSEventPetState).
	Broadcast func(State)
	// ActiveChar returns the active character's name.
	ActiveChar func() string
	// CharmTimer reports the active charm timer's spell and bound target name.
	// Called on every pipe tick and log event while a pet is up, so it must
	// be cheap (spelltimer.Engine.ActiveCharm).
	CharmTimer func() (spell, target string, ok bool)
	// IsSpell reports whether a line is an exact spell name (for `/pet
	// health` member lines); IsCharmSpell whether a spell name is a charm.
	IsSpell      func(name string) bool
	IsCharmSpell func(name string) bool
}

// Tracker holds the pet state. Safe for concurrent use.
type Tracker struct {
	cfg Config
	now func() time.Time

	mu        sync.Mutex
	st        State
	swapDelay time.Duration

	pipeConnected bool
	petMisses     int
	labelName     string // last non-empty label 68 value ("" once the pet is gone)
	brokenID      *int   // spawn id of the pet that just entered ModeBroken
	startedAt     time.Time
	zealHPAt      time.Time
	charmSeen     bool   // a charm-only signal was seen for the current pet
	resolvedName  string // name the by-name fallback last ran for
	health        healthAssembler
	healthTimer   *time.Timer
	swapTimer     *time.Timer

	lastSentHP *int
	lastSentAt time.Time
}

// New returns a Tracker in ModeNone.
func New(cfg Config) *Tracker {
	t := &Tracker{
		cfg:       cfg,
		now:       time.Now,
		swapDelay: DefaultSwapDelay,
		st:        State{Mode: ModeNone, ZealLimited: true},
	}
	t.health.isSpell = cfg.IsSpell
	return t
}

// SetSwapDelay sets how long ModeBroken lasts. Non-positive restores the
// default.
func (t *Tracker) SetSwapDelay(d time.Duration) {
	if d <= 0 {
		d = DefaultSwapDelay
	}
	t.mu.Lock()
	t.swapDelay = d
	t.mu.Unlock()
}

// GetState returns the current state.
func (t *Tracker) GetState() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.st
}

// ── Zeal pipe inputs ─────────────────────────────────────────────────────────

// SetPipeConnected records the pipe's connection state. A disconnect drops the
// tracked pet id without ending the pet — the pet may well still be alive; we
// have only lost the feed (mirrors spelltimer.ResetPipePetID).
func (t *Tracker) SetPipeConnected(connected bool) {
	t.mu.Lock()
	t.pipeConnected = connected
	changed := t.st.ZealLimited == connected
	t.st.ZealLimited = !connected
	if !connected {
		t.st.PetID = nil
		t.petMisses = 0
		t.zealHPAt = time.Time{}
	}
	t.commitLocked(changed)
}

// SetPipePetID consumes MsgPlayer.pet_id (~10 Hz). A new id starts a new pet;
// a debounced loss ends the current one; every tick refreshes the npc_types
// resolution from the NPC overlay's cache (the player targeting their pet
// narrows it) and the charm/summoned kind.
func (t *Tracker) SetPipePetID(id *int) {
	t.mu.Lock()
	prev := t.st.PetID
	changed := false
	switch {
	case id == nil:
		t.brokenID = nil
		if prev == nil {
			break
		}
		t.petMisses++
		if t.petMisses < petLossMissThreshold {
			break
		}
		t.petMisses = 0
		t.endPetLocked()
		changed = true
	case t.st.Mode == ModeBroken && t.brokenID != nil && *t.brokenID == *id:
		// The charm broke on a log line but the snapshot still lists the
		// old pet for a few frames — not a new pet.
	case prev == nil || *prev != *id:
		t.petMisses = 0
		if prev != nil || t.st.Mode != ModePet {
			// A different spawn (re-charm, new summon) or a pet first seen
			// by id. Label 68 can land a beat before the id, so seed the
			// name from it.
			t.startPetLocked(t.labelName)
		}
		v := *id
		t.st.PetID = &v
		changed = true
	default:
		t.petMisses = 0
	}
	if t.st.Mode == ModePet {
		if t.resolveLocked() {
			changed = true
		}
		if t.refreshKindLocked() {
			changed = true
		}
	}
	t.commitLocked(changed)
}

// SetPipePetName consumes label 68 (the player's pet name; empty = no pet).
func (t *Tracker) SetPipePetName(name string) {
	name = strings.TrimSpace(name)
	t.mu.Lock()
	t.labelName = name
	if name == "" || name == t.st.Name {
		t.mu.Unlock()
		return
	}
	if t.st.Mode == ModePet && (t.st.PetID != nil || t.st.Name == "") {
		// The id (when present) already decides new-vs-same pet; an unnamed
		// pet (opened by a `/pet health` reply) just learns its name.
		t.st.Name = name
	} else {
		// No pet, a broken one, or (no id) a different name: a new pet.
		t.startPetLocked(name)
	}
	t.resolveLocked()
	t.refreshKindLocked()
	t.commitLocked(true)
}

// SetPipePetHPPercent consumes label 69 (the pet's HP %).
func (t *Tracker) SetPipePetHPPercent(raw string) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 || n > 100 {
		return
	}
	t.mu.Lock()
	if t.st.Mode != ModePet {
		t.mu.Unlock()
		return
	}
	now := t.now()
	t.zealHPAt = now
	t.setHPLocked(n, HPSourceZeal, now)
	t.commitLocked(false)
}

// ── Log inputs ───────────────────────────────────────────────────────────────

// Handle consumes parsed log events.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventCharmedPet:
		d, ok := ev.Data.(logparser.CharmedPetData)
		if !ok || d.Pet == "" {
			return
		}
		t.bindLogPet(d.Pet, true)
	case logparser.EventPetOwner:
		d, ok := ev.Data.(logparser.PetOwnerData)
		if !ok || d.Pet == "" || t.cfg.ActiveChar == nil ||
			!strings.EqualFold(d.Owner, t.cfg.ActiveChar()) {
			return
		}
		t.bindLogPet(d.Pet, false)
	case logparser.EventCharmBroken:
		t.mu.Lock()
		if t.st.Mode != ModePet {
			t.mu.Unlock()
			return
		}
		t.charmSeen = true
		t.st.Kind = KindCharm
		t.endPetLocked()
		t.commitLocked(true)
	case logparser.EventZone:
		t.mu.Lock()
		// Spawn ids and the NPC overlay's resolution cache are per zone. With
		// Zeal, pet_id re-reports the pet on the far side (a new id starts a
		// new pet). Without it, a charm doesn't survive the zone; a summoned
		// pet follows, so keep it but drop the zone-scoped row.
		t.resolvedName = ""
		changed := false
		if !t.pipeConnected && t.st.Mode == ModePet && t.st.Kind == KindCharm {
			t.resetLocked()
			changed = true
		} else if t.st.Stats != nil && t.st.Kind != KindCharm {
			t.st.Stats, t.st.Variants = nil, 0
			changed = true
		}
		t.commitLocked(changed)
	default:
		// A trigger-driven charm timer may have just started for the current
		// pet, or the summoned settle window may have elapsed.
		t.mu.Lock()
		changed := t.st.Mode == ModePet && t.refreshKindLocked()
		t.commitLocked(changed)
	}
}

// bindLogPet binds a pet name seen in the log to the active character.
func (t *Tracker) bindLogPet(name string, charmed bool) {
	t.mu.Lock()
	switch {
	case t.st.Mode != ModePet:
		t.startPetLocked("")
	case t.st.PetID == nil && t.st.Name != "" &&
		logparser.CanonicalNPCName(t.st.Name) != logparser.CanonicalNPCName(name):
		t.startPetLocked("")
	}
	if t.st.Name == "" || t.st.PetID == nil {
		t.st.Name = name
	}
	if charmed {
		t.charmSeen = true
	}
	t.refreshKindLocked()
	t.resolveLocked()
	t.commitLocked(true)
}

// HandleLine consumes raw log lines for `/pet health` blocks.
func (t *Tracker) HandleLine(ts time.Time, msg string) {
	t.mu.Lock()
	wasOpen := t.health.open
	snap := t.health.handleLine(ts, msg)
	if t.health.open && (!wasOpen || snap != nil) {
		// A header just opened a block: close it on a quiet log too.
		t.armHealthFlushLocked()
	}
	if snap == nil {
		t.mu.Unlock()
		return
	}
	t.applyHealthLocked(snap)
	t.commitLocked(true)
}

func (t *Tracker) armHealthFlushLocked() {
	if t.healthTimer != nil {
		t.healthTimer.Stop()
	}
	t.healthTimer = time.AfterFunc(healthFlushDelay, t.flushHealth)
}

func (t *Tracker) flushHealth() {
	t.mu.Lock()
	snap := t.health.flush()
	if snap == nil {
		t.mu.Unlock()
		return
	}
	t.applyHealthLocked(snap)
	t.commitLocked(true)
}

// applyHealthLocked records a `/pet health` reply. A reply proves a pet exists,
// so it (re)opens ModePet; a charm spell in the list proves the pet is charmed.
func (t *Tracker) applyHealthLocked(snap *HealthSnapshot) {
	if t.st.Mode != ModePet {
		t.startPetLocked("")
	}
	t.st.Health = snap
	if t.cfg.IsCharmSpell != nil {
		for _, s := range snap.Spells {
			if t.cfg.IsCharmSpell(s) {
				t.charmSeen = true
				break
			}
		}
	}
	t.refreshKindLocked()
	if t.zealHPAt.IsZero() || t.now().Sub(t.zealHPAt) > zealHPFresh {
		t.setHPLocked(snap.HPPercent, HPSourcePetHealth, snap.At)
	}
}

// Reset returns to ModeNone (character switch, log replay).
func (t *Tracker) Reset() {
	t.mu.Lock()
	t.resetLocked()
	t.commitLocked(true)
}

// ── internals (caller holds t.mu) ────────────────────────────────────────────

// startPetLocked replaces the state with a fresh pet.
func (t *Tracker) startPetLocked(name string) {
	t.stopSwapLocked()
	t.st = State{
		Mode:        ModePet,
		Name:        name,
		ZealLimited: !t.pipeConnected,
	}
	t.charmSeen = false
	t.resolvedName = ""
	t.brokenID = nil
	t.startedAt = t.now()
	t.zealHPAt = time.Time{}
	t.lastSentHP = nil
	t.refreshKindLocked()
}

// endPetLocked moves a live pet to ModeBroken and arms the swap back to none.
// The card keeps the old pet's name and stats for the re-charm view.
func (t *Tracker) endPetLocked() {
	if t.st.Mode != ModePet {
		return
	}
	now := t.now()
	swapAt := now.Add(t.swapDelay)
	t.st.Mode = ModeBroken
	t.brokenID = t.st.PetID
	t.st.PetID = nil
	t.st.HPPercent, t.st.HPSource, t.st.HPAt = nil, "", nil
	t.st.CharmSpell = ""
	t.st.BrokeAt = &now
	t.st.SwapAt = &swapAt
	t.stopSwapLocked()
	t.swapTimer = time.AfterFunc(t.swapDelay, t.expireBroken)
}

// expireBroken ends ModeBroken once SwapAt has passed.
func (t *Tracker) expireBroken() {
	t.mu.Lock()
	if t.st.Mode != ModeBroken || t.st.SwapAt == nil || t.now().Before(*t.st.SwapAt) {
		t.mu.Unlock()
		return
	}
	t.resetLocked()
	t.commitLocked(true)
}

func (t *Tracker) stopSwapLocked() {
	if t.swapTimer != nil {
		t.swapTimer.Stop()
		t.swapTimer = nil
	}
}

func (t *Tracker) resetLocked() {
	t.stopSwapLocked()
	t.st = State{Mode: ModeNone, ZealLimited: !t.pipeConnected}
	t.petMisses = 0
	t.charmSeen = false
	t.resolvedName = ""
	t.brokenID = nil
	t.zealHPAt = time.Time{}
	t.lastSentHP = nil
}

func (t *Tracker) setHPLocked(pct int, source string, at time.Time) {
	t.st.HPPercent = &pct
	t.st.HPSource = source
	t.st.HPAt = &at
}

// refreshKindLocked derives charm vs summoned and the charm spell. Reports
// whether anything changed.
func (t *Tracker) refreshKindLocked() bool {
	spell := ""
	if t.cfg.CharmTimer != nil {
		if s, target, ok := t.cfg.CharmTimer(); ok && t.charmTimerMatchesLocked(target) {
			spell = s
		}
	}
	kind := KindUnknown
	switch {
	case spell != "" || t.charmSeen:
		kind = KindCharm
	case t.now().Sub(t.startedAt) >= kindSettle:
		kind = KindSummoned
	}
	if kind == t.st.Kind && spell == t.st.CharmSpell {
		return false
	}
	t.st.Kind = kind
	t.st.CharmSpell = spell
	return true
}

// charmTimerMatchesLocked reports whether a charm timer bound to target
// belongs to the current pet. Only one charm runs per character, so an unbound
// timer (empty target) or an unnamed pet is accepted.
func (t *Tracker) charmTimerMatchesLocked(target string) bool {
	if target == "" || t.st.Name == "" {
		return true
	}
	return logparser.CanonicalNPCName(target) == logparser.CanonicalNPCName(t.st.Name)
}

// resolveLocked refreshes Stats. The spawn-id cache wins and may upgrade an
// earlier pick (narrower variants, or an exact level appearing); the by-name
// fallback runs once per name and only when nothing better exists. Reports
// whether Stats changed.
func (t *Tracker) resolveLocked() bool {
	if t.cfg.Resolver == nil || t.st.Mode != ModePet {
		return false
	}
	if t.st.PetID != nil {
		if r, ok := t.cfg.Resolver.ResolvedForID(*t.st.PetID); ok && t.betterLocked(r) {
			t.applyResolutionLocked(r)
			return true
		}
	}
	if t.st.Stats != nil || t.st.Name == "" || t.resolvedName == t.st.Name {
		return false
	}
	if t.pipeConnected && t.now().Sub(t.startedAt) < nameFallbackDelay {
		return false // retried on a later pipe tick
	}
	t.resolvedName = t.st.Name
	r := t.cfg.Resolver.ResolveByName(t.st.Name)
	if r.NPC == nil {
		return false
	}
	t.applyResolutionLocked(r)
	return true
}

// betterLocked reports whether r improves on the current Stats.
func (t *Tracker) betterLocked(r overlay.Resolution) bool {
	cur := t.st.Stats
	switch {
	case cur == nil:
		return true
	case r.NPC.ID != cur.NPCID:
		// Variants is 0 for a unique pick and >= 2 when ambiguous; only
		// swap rows for a pick at least as narrow.
		return r.Variants == 0 || (t.st.Variants != 0 && r.Variants <= t.st.Variants)
	case r.Level > 0 && cur.Level != r.Level:
		return true
	}
	return false
}

func (t *Tracker) applyResolutionLocked(r overlay.Resolution) {
	t.st.Stats = BuildStats(r.NPC, r.Abilities, r.Summary, r.Level)
	t.st.Variants = r.Variants
}

// commitLocked stamps, unlocks, and broadcasts when changed is true or the HP
// moved and the HP rate limit allows (a pet's first reading goes out at once).
// Always releases t.mu.
func (t *Tracker) commitLocked(changed bool) {
	now := t.now()
	hpMoved := !eqIntPtr(t.st.HPPercent, t.lastSentHP)
	hpDue := t.lastSentHP == nil || now.Sub(t.lastSentAt) >= hpBroadcastInterval
	if !changed && !(hpMoved && hpDue) {
		t.mu.Unlock()
		return
	}
	t.st.UpdatedAt = now
	t.lastSentAt = now
	if t.st.HPPercent != nil {
		v := *t.st.HPPercent
		t.lastSentHP = &v
	} else {
		t.lastSentHP = nil
	}
	snap := t.st
	t.mu.Unlock()
	if t.cfg.Broadcast != nil {
		t.cfg.Broadcast(snap)
	}
}

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

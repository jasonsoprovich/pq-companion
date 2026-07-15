package raidthreat

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// defaultClassMods are the built-in per-class hate adjustments (signed percent)
// applied when the user hasn't overridden a class. Empty by default: every
// player's hate is honest observed damage. The tank-undercount problem is now
// handled by modelling taunt directly (see applyTaunt) rather than a blanket
// boost; the per-class config knob remains for users who want to approximate a
// known aggro-mod gear/AA setup. An explicit config entry always wins.
var defaultClassMods = map[string]int{}

// tauntBump is the hate a successful taunt adds above the current top of the
// list (EQMacEmu Mob::Taunt: myHate = topHate + 10).
const tauntBump = 10

// dotClasses / healClasses drive the per-row confidence flags: their DoT ticks
// (resp. heals on others) are never in the local log, so their hate reads low.
var (
	dotClasses  = map[string]bool{"Necromancer": true, "Shaman": true, "Druid": true}
	healClasses = map[string]bool{"Cleric": true, "Druid": true, "Shaman": true}
)

// DamageSource supplies per-mob, per-attacker damage (implemented by
// combat.Tracker).
type DamageSource interface {
	RaidThreatDamage() []combat.MobDamage
}

// PersonalSource supplies the active character's per-mob hate (implemented by
// threat.Tracker).
type PersonalSource interface {
	PersonalHate() map[string]int64
}

// Assembler combines observed per-attacker damage with the personal hate
// estimate into a ranked, per-mob raid threat view. It owns no log state — it
// reads its inputs live at snapshot time, so it is always consistent with the
// DPS and personal meters.
type Assembler struct {
	hub      *ws.Hub
	dmg      DamageSource
	personal PersonalSource

	// Config closures read live so a settings change takes effect immediately.
	enabledFn    func() bool           // raid_threat_enabled
	classModsFn  func() map[string]int // raid_threat_class_mods (class → signed %)
	playerModsFn func() map[string]int // raid_threat_player_mods (player → signed %)
	// selfNameFn returns the active character's name, so a taunt emote naming
	// "Borg" maps onto the "You" row when Borg is the player. May be nil.
	selfNameFn func() string

	mu         sync.Mutex
	pipeTarget string
	// taunt holds a per-mob, per-player additive hate offset set when that
	// player taunts (so their displayed hate becomes top+10) and carried until
	// the mob dies / the player zones. Observed damage accrues on top of it.
	taunt map[string]map[string]int64
	// left holds a per-mob, per-player additive (negative) hate offset set when
	// that player is seen leaving the zone (Evacuate/Succor/Call of the
	// Hero/a Circle spell/...), zeroing their displayed hate on every mob they
	// were on — the server clears a departing player's hate list entirely, but
	// combat's per-attacker damage (our proxy for others' hate) is a cumulative
	// total that never resets on its own. See applyDeparture. Cleared on the
	// same kill / zone / death lifecycle as taunt.
	left map[string]map[string]int64
	// dismissed holds mobs the user manually removed via the overlay's per-card
	// "x". The view is otherwise stateless (rebuilt from live combat each
	// snapshot), so without this a removed mob would reappear on the next tick.
	// Cleared on the same kill / zone / death lifecycle events as taunt.
	dismissed map[string]bool
}

// NewAssembler returns a raid threat Assembler. Any of the closures may be nil
// (treated as disabled / empty / no self name).
func NewAssembler(hub *ws.Hub, dmg DamageSource, personal PersonalSource,
	enabledFn func() bool, classModsFn, playerModsFn func() map[string]int,
	selfNameFn func() string) *Assembler {
	return &Assembler{
		hub:          hub,
		dmg:          dmg,
		personal:     personal,
		enabledFn:    enabledFn,
		classModsFn:  classModsFn,
		playerModsFn: playerModsFn,
		selfNameFn:   selfNameFn,
		taunt:        make(map[string]map[string]int64),
		left:         make(map[string]map[string]int64),
		dismissed:    make(map[string]bool),
	}
}

func (a *Assembler) enabled() bool {
	return a.enabledFn != nil && a.enabledFn()
}

func (a *Assembler) classModsCfg() map[string]int {
	if a.classModsFn == nil {
		return nil
	}
	return a.classModsFn()
}

func (a *Assembler) playerModsCfg() map[string]int {
	if a.playerModsFn == nil {
		return nil
	}
	return a.playerModsFn()
}

// SetPipeTarget records the player's current Zeal target so the highlighted mob
// matches the DPS/personal meters. Mirrors combat/threat SetPipeTarget.
func (a *Assembler) SetPipeTarget(name string) {
	name = logparser.CanonicalNPCName(name)
	a.mu.Lock()
	a.pipeTarget = name
	a.mu.Unlock()
}

// classMod returns the effective signed-percent adjustment for a class: the
// user override when present, else the built-in default (currently 0 for every
// class — see defaultClassMods).
func classMod(class string, userMods map[string]int) int {
	if v, ok := userMods[class]; ok {
		return v
	}
	return defaultClassMods[class]
}

// collectBase builds the per-mob, per-player BASE hate (observed damage × hate
// mods, plus the high-fidelity personal "You" row) before any taunt offset.
// Stateless — derived fresh from the combat and personal meters. Keyed
// mob → player.
func (a *Assembler) collectBase() map[string]map[string]*RaidEntry {
	classMods := a.classModsCfg()
	playerMods := a.playerModsCfg()

	out := make(map[string]map[string]*RaidEntry)
	ensure := func(mob string) map[string]*RaidEntry {
		m := out[mob]
		if m == nil {
			m = make(map[string]*RaidEntry)
			out[mob] = m
		}
		return m
	}

	for _, md := range a.dmg.RaidThreatDamage() {
		m := ensure(md.Mob)
		for _, atk := range md.Attackers {
			if atk.Name == "You" {
				// The high-fidelity "You" row comes from the personal meter
				// (it includes spell/heal/miss hate beyond raw damage).
				continue
			}
			mod := 0
			if !atk.IsPet {
				// Pets don't benefit from their owner's +hate gear, so they get
				// a neutral adjustment regardless of (owner's) class.
				mod = classMod(atk.Class, classMods) + playerMods[atk.Name]
			}
			hate := atk.Damage * int64(100+mod) / 100
			if hate < 0 {
				hate = 0
			}
			m[atk.Name] = &RaidEntry{
				Name:       atk.Name,
				Class:      atk.Class,
				OwnerName:  atk.OwnerName,
				IsPet:      atk.IsPet,
				Hate:       hate,
				Confidence: confidenceFor(atk.Class, atk.IsPet),
			}
		}
	}
	for mob, hate := range a.personal.PersonalHate() {
		ensure(mob)["You"] = &RaidEntry{Name: "You", IsYou: true, Hate: hate}
	}
	return out
}

// GetState builds a point-in-time raid threat snapshot: base hate plus any
// active taunt offsets, ranked per mob. Returns an empty (not-in-combat) state
// when the feature is disabled.
func (a *Assembler) GetState() RaidThreatState {
	now := time.Now()
	if !a.enabled() {
		return RaidThreatState{Mobs: []RaidMob{}, LastUpdated: now}
	}

	base := a.collectBase()

	a.mu.Lock()
	pipe := a.pipeTarget
	// Snapshot the dismissed set so we can drop those mobs after releasing the
	// lock (the final assembly loop runs unlocked).
	var dismissed map[string]bool
	if len(a.dismissed) > 0 {
		dismissed = make(map[string]bool, len(a.dismissed))
		for k := range a.dismissed {
			dismissed[k] = true
		}
	}
	// Layer taunt offsets on top of base, materialising a taunt-only player
	// (one who taunted but hasn't been seen doing damage yet) as a bare row.
	for mob, offs := range a.taunt {
		m := base[mob]
		if m == nil {
			m = make(map[string]*RaidEntry)
			base[mob] = m
		}
		for player, off := range offs {
			e := m[player]
			if e == nil {
				e = &RaidEntry{Name: player, IsYou: player == "You"}
				m[player] = e
			}
			e.Hate += off
			if e.Hate < 0 {
				e.Hate = 0
			}
		}
	}
	// Layer departure offsets the same way, zeroing a player who was seen
	// leaving the zone (see applyDeparture) — their cumulative damage keeps
	// climbing in the combat tracker, but their real hate is gone.
	for mob, offs := range a.left {
		m := base[mob]
		if m == nil {
			continue
		}
		for player, off := range offs {
			e := m[player]
			if e == nil {
				continue
			}
			e.Hate += off
			if e.Hate < 0 {
				e.Hate = 0
			}
		}
	}
	a.mu.Unlock()

	state := RaidThreatState{Mobs: make([]RaidMob, 0, len(base)), LastUpdated: now}
	for mob, players := range base {
		if len(players) == 0 || dismissed[mob] {
			continue
		}
		rows := make([]RaidEntry, 0, len(players))
		for _, e := range players {
			rows = append(rows, *e)
		}
		// Sort by hate desc, name asc as a stable tiebreaker so equal-hate rows
		// don't jitter between snapshots.
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Hate != rows[j].Hate {
				return rows[i].Hate > rows[j].Hate
			}
			return rows[i].Name < rows[j].Name
		})
		top := rows[0].Hate
		if top > 0 {
			for i := range rows {
				rows[i].HatePct = float64(rows[i].Hate) / float64(top)
			}
		}
		state.Mobs = append(state.Mobs, RaidMob{
			Name:     mob,
			IsTarget: mob == pipe && pipe != "",
			TopHate:  top,
			Players:  rows,
		})
	}
	sort.Slice(state.Mobs, func(i, j int) bool {
		if state.Mobs[i].TopHate != state.Mobs[j].TopHate {
			return state.Mobs[i].TopHate > state.Mobs[j].TopHate
		}
		return state.Mobs[i].Name < state.Mobs[j].Name
	})
	state.InCombat = len(state.Mobs) > 0
	return state
}

// applyTaunt records a successful taunt: the taunter's displayed hate on the
// mob becomes the current top + tauntBump, stored as an additive offset so
// subsequent observed damage accrues on top of it. A taunter already at the top
// is left unchanged (matches the server's no-op when you are already top hater).
// Note: the offset is fixed until the mob dies / the player zones, so a tank who
// taunts once then stops can read high until others overtake — re-taunts (the
// normal case) re-pin it to the live top.
func (a *Assembler) applyTaunt(mob, taunter string) {
	// The taunt emote names the mob in subject position, whose article casing
	// EQ varies ("a fetid fiend says ..." but also "A shadow reaver says ...").
	// Fold it so the offset keys match the canonical base/damage mob keys —
	// otherwise the offset is recorded but never merged into displayed hate.
	mob = logparser.CanonicalNPCName(mob)
	if a.selfNameFn != nil {
		if sn := a.selfNameFn(); sn != "" && taunter == sn {
			taunter = "You" // the emote names the character; our row is keyed "You"
		}
	}
	base := a.collectBase()

	a.mu.Lock()
	defer a.mu.Unlock()

	// Current displayed hate (base + existing offset) for every player on the
	// mob, including earlier taunt-only players.
	displayed := make(map[string]int64)
	if m := base[mob]; m != nil {
		for p, e := range m {
			displayed[p] = e.Hate
		}
	}
	for p, off := range a.taunt[mob] {
		displayed[p] += off
	}

	var top int64
	for _, h := range displayed {
		if h > top {
			top = h
		}
	}
	if top > 0 && displayed[taunter] >= top {
		return // already the top hater — taunt is a no-op
	}

	var taunterBase int64
	if m := base[mob]; m != nil {
		if e := m[taunter]; e != nil {
			taunterBase = e.Hate
		}
	}
	if a.taunt[mob] == nil {
		a.taunt[mob] = make(map[string]int64)
	}
	// Offset chosen so base + offset == top + tauntBump right now.
	a.taunt[mob][taunter] = top + tauntBump - taunterBase
}

// isDepartureSpell reports whether spellName is one of the zone-departure
// spells whose cast_on_other text ("<Name> creates a mystic portal.", "<Name>
// creates a shimmering portal.", "<Name> steps into a mystic portal.") is
// visible to everyone nearby — Evacuate/Succor (and every zone-specific
// variant, e.g. "Evacuate: North"), Call of the Hero, and the druid/wizard
// Circle line. All of them mean the same thing for raid threat purposes: this
// player is about to leave (or has just left) the zone, so the server clears
// their hate on every mob. Matched by name rather than SPA/effect, since
// these are simple teleport spells with no hate-related SPA in common.
func isDepartureSpell(name string) bool {
	switch {
	case name == "":
		return false
	case strings.HasPrefix(name, "Evacuate"):
		return true // Evacuate, Lesser Evacuate, Evacuate: <zone>
	case strings.HasPrefix(name, "Succor"):
		return true // Succor, Lesser Succor, Succor: <zone>
	case strings.HasPrefix(name, "Circle of "):
		return true // druid/wizard self-teleport line
	case strings.HasPrefix(name, "Allure of the Pool"):
		return true
	case strings.HasPrefix(name, "Random Teleport"):
		return true
	}
	switch name {
	case "Call of the Hero", "Guide Evacuation", "Trakanon's Touch",
		"BurningTouch", "BurningTouch2", "Word of Passage", "Exodus", "Call of Alarm":
		return true
	}
	return false
}

// candidateSpellNames returns the spell name(s) a landed-spell event may refer
// to: the resolved name when the cast text is unambiguous, else every
// candidate that shares the ambiguous text — mirrors threat.candidateNames.
func candidateSpellNames(data logparser.SpellLandedData) []string {
	if data.SpellName != "" {
		return []string{data.SpellName}
	}
	names := make([]string, 0, len(data.Candidates))
	for _, c := range data.Candidates {
		names = append(names, c.SpellName)
	}
	return names
}

// applyDeparture zeroes target's displayed hate on every mob it currently
// holds hate on, recording the offset needed to cancel its current base (+
// any existing taunt/departure offset). Future damage from a player who
// returns and re-engages accrues on top of this new baseline, same as taunt.
func (a *Assembler) applyDeparture(target string) {
	if target == "" {
		return
	}
	if a.selfNameFn != nil {
		if sn := a.selfNameFn(); sn != "" && target == sn {
			target = "You" // the emote/land text names the character
		}
	}
	base := a.collectBase()

	a.mu.Lock()
	defer a.mu.Unlock()
	for mob, players := range base {
		e, ok := players[target]
		if !ok {
			continue
		}
		displayed := e.Hate + a.taunt[mob][target] + a.left[mob][target]
		if displayed == 0 {
			continue
		}
		if a.left[mob] == nil {
			a.left[mob] = make(map[string]int64)
		}
		a.left[mob][target] -= displayed
	}
}

// DismissMob suppresses a mob from the raid threat view (the overlay's per-card
// "x"). The view is rebuilt from live combat each snapshot, so the mob is held
// in a dismissed set until its fight lifecycle resets (kill / zone / death)
// rather than reappearing on the next tick. Returns false if the named mob
// isn't currently shown.
func (a *Assembler) DismissMob(name string) bool {
	name = logparser.CanonicalNPCName(name)
	if name == "" {
		return false
	}
	present := false
	for _, m := range a.GetState().Mobs {
		if m.Name == name {
			present = true
			break
		}
	}
	if !present {
		return false
	}
	a.mu.Lock()
	a.dismissed[name] = true
	delete(a.taunt, name)
	delete(a.left, name)
	a.mu.Unlock()
	a.broadcast(a.GetState())
	return true
}

// DismissAll suppresses every mob currently shown in the raid threat view (the
// overlay's header "clear" button). Like DismissMob, each mob is held in the
// dismissed set until its own fight lifecycle resets (kill / zone / death)
// rather than reappearing on the next tick. Returns the number of mobs cleared.
func (a *Assembler) DismissAll() int {
	shown := a.GetState().Mobs
	if len(shown) == 0 {
		return 0
	}
	a.mu.Lock()
	for _, m := range shown {
		a.dismissed[m.Name] = true
		delete(a.taunt, m.Name)
		delete(a.left, m.Name)
	}
	a.mu.Unlock()
	a.broadcast(a.GetState())
	return len(shown)
}

// Handle processes the parsed log events the assembler reacts to: taunts (which
// drive the taunt model) and the lifecycle events that clear it. No-op while
// the feature is disabled.
func (a *Assembler) Handle(ev logparser.LogEvent) {
	if !a.enabled() {
		return
	}
	switch ev.Type {
	case logparser.EventTaunt:
		data, ok := ev.Data.(logparser.TauntData)
		if !ok || data.Mob == "" || data.Taunter == "" {
			return
		}
		a.applyTaunt(data.Mob, data.Taunter)
		a.broadcast(a.GetState()) // reflect the jump immediately, not on the next tick

	case logparser.EventSpellLanded:
		data, ok := ev.Data.(logparser.SpellLandedData)
		if !ok || data.Kind != logparser.SpellLandedKindOther || data.TargetName == "" {
			return
		}
		for _, name := range candidateSpellNames(data) {
			if isDepartureSpell(name) {
				a.applyDeparture(data.TargetName)
				a.broadcast(a.GetState()) // reflect the drop immediately
				break
			}
		}

	case logparser.EventKill:
		if data, ok := ev.Data.(logparser.KillData); ok {
			a.mu.Lock()
			delete(a.taunt, logparser.CanonicalNPCName(data.Target))
			delete(a.left, logparser.CanonicalNPCName(data.Target))
			delete(a.dismissed, logparser.CanonicalNPCName(data.Target))
			a.mu.Unlock()
		}

	case logparser.EventZone, logparser.EventDeath:
		a.mu.Lock()
		a.taunt = make(map[string]map[string]int64)
		a.left = make(map[string]map[string]int64)
		a.dismissed = make(map[string]bool)
		a.mu.Unlock()

	case logparser.EventFeignDeath:
		// A successful feign drops YOU from every hate list — clear only your
		// taunt holds, leaving other players' intact.
		a.mu.Lock()
		for _, offs := range a.taunt {
			delete(offs, "You")
		}
		a.mu.Unlock()
	}
}

// confidenceFor returns the caveat flags for a non-You player row.
func confidenceFor(class string, isPet bool) []string {
	if isPet {
		return nil
	}
	if class == "" {
		return []string{ConfClassUnknown}
	}
	var c []string
	if dotClasses[class] {
		c = append(c, ConfDoTUndercount)
	}
	if healClasses[class] {
		c = append(c, ConfHealUndercount)
	}
	return c
}

func (a *Assembler) broadcast(state RaidThreatState) {
	if a.hub != nil {
		a.hub.Broadcast(ws.Event{Type: WSEventRaidThreat, Data: state})
	}
}

// Broadcast pushes the current state immediately (e.g. after a target change).
// No-op while the feature is disabled.
func (a *Assembler) Broadcast() {
	if a.enabled() {
		a.broadcast(a.GetState())
	}
}

// RunTicker rebroadcasts the snapshot on a fixed interval while the feature is
// enabled and combat is active, so the live estimate refreshes as other
// players' damage lands. Idle ticks broadcast nothing. Blocks until ctx is
// cancelled; run it in its own goroutine.
func (a *Assembler) RunTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.enabled() {
				continue
			}
			if state := a.GetState(); state.InCombat {
				a.broadcast(state)
			}
		}
	}
}

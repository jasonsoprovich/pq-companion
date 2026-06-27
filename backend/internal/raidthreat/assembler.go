package raidthreat

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// defaultClassMods are the built-in per-class hate adjustments (signed percent)
// applied when the user hasn't overridden a class. Tanks default high because
// their taunt / disciplines / +hate gear are invisible in the log, so raw
// damage badly understates their hate. Users tune these in settings; an
// explicit entry (including 0) always wins over the default.
var defaultClassMods = map[string]int{
	"Warrior":       30,
	"Shadow Knight": 30,
	"Paladin":       30,
}

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

	mu         sync.Mutex
	pipeTarget string
}

// NewAssembler returns a raid threat Assembler. Any of the config closures may
// be nil (treated as disabled / empty).
func NewAssembler(hub *ws.Hub, dmg DamageSource, personal PersonalSource,
	enabledFn func() bool, classModsFn, playerModsFn func() map[string]int) *Assembler {
	return &Assembler{
		hub:          hub,
		dmg:          dmg,
		personal:     personal,
		enabledFn:    enabledFn,
		classModsFn:  classModsFn,
		playerModsFn: playerModsFn,
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
	a.mu.Lock()
	a.pipeTarget = name
	a.mu.Unlock()
}

func (a *Assembler) pipe() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pipeTarget
}

// classMod returns the effective signed-percent adjustment for a class: the
// user override when present, else the built-in default (0 for non-tanks).
func classMod(class string, userMods map[string]int) int {
	if v, ok := userMods[class]; ok {
		return v
	}
	return defaultClassMods[class]
}

// GetState builds a point-in-time raid threat snapshot. Returns an empty
// (not-in-combat) state when the feature is disabled.
func (a *Assembler) GetState() RaidThreatState {
	now := time.Now()
	if !a.enabled() {
		return RaidThreatState{Mobs: []RaidMob{}, LastUpdated: now}
	}

	classMods := a.classModsCfg()
	playerMods := a.playerModsCfg()
	pipe := a.pipe()

	dmgByMob := a.dmg.RaidThreatDamage()
	personal := a.personal.PersonalHate()

	// Union of mobs seen in combat and mobs we personally hold hate on, in a
	// stable first-seen order before the final sort.
	entries := make(map[string][]RaidEntry)
	order := make([]string, 0, len(dmgByMob)+len(personal))
	ensure := func(name string) {
		if _, ok := entries[name]; !ok {
			entries[name] = nil
			order = append(order, name)
		}
	}

	for _, md := range dmgByMob {
		ensure(md.Mob)
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
			entries[md.Mob] = append(entries[md.Mob], RaidEntry{
				Name:       atk.Name,
				Class:      atk.Class,
				OwnerName:  atk.OwnerName,
				IsPet:      atk.IsPet,
				Hate:       hate,
				Confidence: confidenceFor(atk.Class, atk.IsPet),
			})
		}
	}

	for mob, hate := range personal {
		ensure(mob)
		entries[mob] = append(entries[mob], RaidEntry{Name: "You", IsYou: true, Hate: hate})
	}

	state := RaidThreatState{Mobs: make([]RaidMob, 0, len(order)), LastUpdated: now}
	for _, mob := range order {
		rows := entries[mob]
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Hate > rows[j].Hate })
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
	sort.SliceStable(state.Mobs, func(i, j int) bool {
		return state.Mobs[i].TopHate > state.Mobs[j].TopHate
	})
	state.InCombat = len(state.Mobs) > 0
	return state
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

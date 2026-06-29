package main

import (
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/threat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// meleeHateRefreshTTL bounds how often the per-swing melee hate is recomputed.
// It changes only when the character, their level, or their equipped primary
// weapon changes, so a few seconds of staleness during a fight is harmless and
// keeps the per-swing lookup off the DB on a busy combat log.
const meleeHateRefreshTTL = 5 * time.Second

// meleeSwingHateProvider computes the active character's flat per-swing melee
// hate (equipped primary weapon damage + primary-hand bonus) for the threat
// meter, memoising the result for meleeHateRefreshTTL. Returns 0 when the
// character, level, or equipped weapon can't be resolved, so the meter falls
// back to observed damage.
type meleeSwingHateProvider struct {
	activeName func() string
	inventory  func() *zeal.Inventory
	getItem    func(id int) (*db.Item, error)
	getChar    func(name string) (character.Character, bool, error)

	mu        sync.Mutex
	cachedAt  time.Time
	cachedVal int
}

func (p *meleeSwingHateProvider) value() int {
	now := time.Now()
	p.mu.Lock()
	if !p.cachedAt.IsZero() && now.Sub(p.cachedAt) < meleeHateRefreshTTL {
		v := p.cachedVal
		p.mu.Unlock()
		return v
	}
	p.mu.Unlock()

	v := p.compute()

	p.mu.Lock()
	p.cachedAt = now
	p.cachedVal = v
	p.mu.Unlock()
	return v
}

func (p *meleeSwingHateProvider) compute() int {
	name := p.activeName()
	if name == "" {
		return 0
	}
	// The watcher holds one parsed inventory; only use it when it's the active
	// character's, never another character's stale export.
	inv := p.inventory()
	if inv == nil || !strings.EqualFold(inv.Character, name) {
		return 0
	}
	primaryID := 0
	for _, e := range inv.Entries {
		if e.Location == "Primary" {
			primaryID = e.ID
			break
		}
	}
	if primaryID <= 0 {
		return 0
	}
	item, err := p.getItem(primaryID)
	if err != nil || item == nil || !isMeleeWeaponType(item.ItemType) {
		return 0
	}
	ch, ok, err := p.getChar(name)
	if err != nil || !ok || ch.Level <= 0 || ch.Class < 0 {
		return 0
	}
	return threat.MeleeSwingHate(item.Damage, item.Delay, item.ItemType, ch.Level, ch.Class)
}

// isMeleeWeaponType reports whether an items.itemtype is a melee weapon (1H/2H
// slash, blunt, pierce, or hand-to-hand) — the EQMacEmu item_data.h enum, also
// in internal/db/enums/item_type.go. Bows, shields, and non-weapons are excluded.
func isMeleeWeaponType(t int) bool {
	switch t {
	case 0, 1, 2, 3, 4, 35, 45:
		return true
	}
	return false
}

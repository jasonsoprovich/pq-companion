package main

import (
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// hatemodRefreshTTL bounds how often the AA-derived hate modifier is recomputed.
// It changes only when the active character or their trained AAs change, so a few
// seconds of staleness is harmless and keeps the lookup off the DB on a busy log.
const hatemodRefreshTTL = 5 * time.Second

// staticHatemodProvider supplies the threat meter's static hate modifier as a
// signed percentage: the auto-detected hate AA (Spell Casting Subtlety — the
// only one in Quarm) plus the user's manual gear modifier from settings.
//
// The manual part is read live so a settings change takes effect immediately;
// the AA part is memoised for hatemodRefreshTTL.
type staticHatemodProvider struct {
	activeName func() string
	manualPct  func() int
	getChar    func(name string) (character.Character, bool, error)
	listAAs    func(characterID int) ([]character.AAEntry, error)
	aaBonuses  func(trained []db.TrainedAA) (db.AABonuses, error)

	mu        sync.Mutex
	cachedAt  time.Time
	cachedVal int
}

func (p *staticHatemodProvider) value() int {
	return p.manualPct() + p.aaHatemod()
}

func (p *staticHatemodProvider) aaHatemod() int {
	now := time.Now()
	p.mu.Lock()
	if !p.cachedAt.IsZero() && now.Sub(p.cachedAt) < hatemodRefreshTTL {
		v := p.cachedVal
		p.mu.Unlock()
		return v
	}
	p.mu.Unlock()

	v := p.computeAAHatemod()

	p.mu.Lock()
	p.cachedAt = now
	p.cachedVal = v
	p.mu.Unlock()
	return v
}

func (p *staticHatemodProvider) computeAAHatemod() int {
	name := p.activeName()
	if name == "" {
		return 0
	}
	ch, ok, err := p.getChar(name)
	if err != nil || !ok || ch.ID <= 0 {
		return 0
	}
	trained, err := p.listAAs(ch.ID)
	if err != nil || len(trained) == 0 {
		return 0
	}
	conv := make([]db.TrainedAA, 0, len(trained))
	for _, t := range trained {
		conv = append(conv, db.TrainedAA{AAID: t.AAID, Rank: t.Rank})
	}
	b, err := p.aaBonuses(conv)
	if err != nil {
		return 0
	}
	return b.Hatemod
}

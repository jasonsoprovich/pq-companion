package spelltimer

import (
	"encoding/json"
	"math"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Length-delimited JSON tuple identity cannot collide with legacy name@target
// rows, or another trigger's identically named countdown.
func ownedTimerKey(owner, name, target string) string {
	b, _ := json.Marshal([]string{owner, name, target})
	return "\x00owned:" + string(b)
}

func (e *Engine) StartOwnedExternal(owner, name string, duration, threshold float64, at time.Time, alerts json.RawMessage, spellID int, target, color string, pinned bool, group string, ifIdle, clearContext bool) bool {
	if owner == "" || name == "" || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return false
	}
	expires := at.Add(time.Duration(duration * float64(time.Second)))
	caster := e.activePlayerName()
	icon := 0
	if spellID > 0 && e.db != nil {
		if spell, err := e.db.GetSpell(spellID); err == nil && spell != nil {
			icon = spell.NewIcon
		}
	}
	key := ownedTimerKey(owner, name, target)
	e.mu.Lock()
	now := time.Now()
	if !expires.After(now) {
		e.mu.Unlock()
		return false
	}
	if old := e.timers[key]; ifIdle && old != nil && old.ExpiresAt.After(now) {
		e.mu.Unlock()
		return false
	}
	e.timers[key] = &ActiveTimer{
		ID:                   key,
		SpellName:            name,
		SpellID:              spellID,
		TargetName:           target,
		Icon:                 icon,
		Category:             CategoryCustom,
		CastAt:               at,
		StartsAt:             at,
		ExpiresAt:            expires,
		DurationSeconds:      duration,
		DisplayThresholdSecs: threshold,
		TimerAlerts:          append(json.RawMessage(nil), alerts...),
		BarColor:             color,
		Pinned:               pinned,
		CustomGroup:          group,
		CasterCharacter:      caster,
		ownerID:              owner,
		clearOnContext:       clearContext,
	}
	snap := e.snapshot(now)
	e.mu.Unlock()
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	return true
}
func (e *Engine) StopOwnedExternal(owner, name, target string) {
	e.mu.Lock()
	key := ownedTimerKey(owner, name, target)
	if _, found := e.timers[key]; !found {
		e.mu.Unlock()
		return
	}
	delete(e.timers, key)
	snap := e.snapshot(time.Now())
	e.mu.Unlock()
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

// Empty owner is used only for context retirement; ordinary definition
// retirement must supply a specific trigger ID.
func (e *Engine) RetireOwnedExternal(owner string, contextOnly bool) {
	e.mu.Lock()
	changed := false
	for key, row := range e.timers {
		if row.ownerID == "" || (contextOnly && !row.clearOnContext) {
			continue
		}
		if owner != "" {
			if row.ownerID != owner {
				continue
			}
		} else if !contextOnly {
			continue
		}
		delete(e.timers, key)
		changed = true
	}
	if !changed {
		e.mu.Unlock()
		return
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

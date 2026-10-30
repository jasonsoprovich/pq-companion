package trigger

import (
	"encoding/json"
	"time"
)

// EncounterTimerEligible is shared by persistence, API and execution. Empty
// source is the historical spelling of log, not a new source kind.
func (t *Trigger) EncounterTimerEligible() bool {
	return (t.Source == "" || t.Source == SourceLog) && t.TimerType == TimerTypeCustom && !t.TimerStack && !t.TimerRepeat
}
func (t *Trigger) NormalizeEncounterTimer() {
	if !t.EncounterTimerEligible() {
		t.TimerIfIdle = false
		t.TimerClearOnContextChange = false
	}
}
func (t *Trigger) managedTimer() bool {
	return t.EncounterTimerEligible() && (t.TimerIfIdle || t.TimerClearOnContextChange)
}

// OwnedTimerSink admission must atomically inspect/insert the REAL timer row.
// A sink without this capability cannot execute opted-in timers or actions.
type OwnedTimerSink interface {
	StartOwnedExternal(owner, name string, duration, threshold float64, at time.Time, alerts json.RawMessage, spellID int, target, color string, pinned bool, group string, ifIdle, clearContext bool) bool
	StopOwnedExternal(owner, name, target string)
	RetireOwnedExternal(owner string, contextOnly bool)
}

// SourceChanged is called with the Tailer's monotonic source generation. An
// older notification cannot regain authority after a newer one has retired it.
func (e *Engine) SourceChanged(generation uint64) {
	e.managedMu.Lock()
	if generation <= e.sourceGeneration {
		e.managedMu.Unlock()
		return
	}
	e.sourceGeneration = generation
	e.retireContextLocked()
	e.managedMu.Unlock()
	// Character tokens are compiled against the selected source. Reload only
	// on a new source generation, never on an ordinary polling tick.
	if e.store != nil {
		e.Reload()
	}
}
func (e *Engine) retireContextLocked() {
	e.managedEpoch++
	if sink, ok := e.sink.(OwnedTimerSink); ok {
		sink.RetireOwnedExternal("", true)
	}
}
func (e *Engine) RetireContext() {
	e.managedMu.Lock()
	defer e.managedMu.Unlock()
	e.retireContextLocked()
}

// Harmless category/order/presentation reloads preserve the original deadline.
// Marshal compares deep values, not aliases to mutable editor-owned slices.
func encounterSemantics(t *Trigger) string {
	c := *t
	c.PackName = ""
	c.CategoryID = ""
	c.SortOrder = 0
	c.SourcePack = ""
	c.PackKey = ""
	c.CreatedAt = time.Time{}
	c.BarColor = ""
	c.Pinned = false
	c.CustomGroupID = ""
	c.DisplayThresholdSecs = 0
	c.TimerAlerts = nil
	b, _ := json.Marshal(c)
	return string(b)
}

// These are planning metadata, not timer admission state. Lifetimes are owned
// by current compiled definitions and any in-flight snapshots; resolved keys
// exist only while their Handle call is in flight, never as historical epochs.
// Lifetime/cancellation state is protected by Engine.managedMu. Captured
// matches and their extra-pattern index are immutable for the Handle call.
type managedLifetime struct {
	retired bool
	pending map[*managedPlan]struct{}
}
type managedPlan struct {
	lifetime    *managedLifetime
	key, target string
	revoked     bool
	match       []string
	extra       int // -1 for the primary pattern
}

func (e *Engine) captureManagedPlansLocked(cs []compiled, message string, stale bool) []*managedPlan {
	if stale {
		return nil
	}
	var plans []*managedPlan
	for i, c := range cs {
		if c.lifetime == nil || c.timerKey == "" || matchesAny(c.excludes, message) {
			continue
		}
		m, names := c.re.FindStringSubmatch(message), c.re.SubexpNames()
		extra := -1
		for j := 0; m == nil && j < len(c.extras); j++ {
			m, names = c.extras[j].re.FindStringSubmatch(message), c.extras[j].re.SubexpNames()
			if m != nil {
				extra = j
			}
		}
		if m == nil {
			continue
		}
		if plans == nil {
			plans = make([]*managedPlan, len(cs))
		}
		p := &managedPlan{lifetime: c.lifetime, key: resolveTimerKey(c.trigger, c.timerKey, m, names), target: resolveTimerTarget(c.trigger, m, names), match: m, extra: extra}
		if c.lifetime.pending == nil {
			c.lifetime.pending = make(map[*managedPlan]struct{})
		}
		c.lifetime.pending[p] = struct{}{}
		plans[i] = p
	}
	return plans
}
func (e *Engine) releaseManagedPlans(plans []*managedPlan) {
	e.managedMu.Lock()
	defer e.managedMu.Unlock()
	for _, p := range plans {
		if p != nil {
			delete(p.lifetime.pending, p)
		}
	}
}

func (e *Engine) stopOwned(c compiled, key, target string, epoch uint64) {
	e.managedMu.Lock()
	defer e.managedMu.Unlock()
	if epoch != e.managedEpoch || c.lifetime == nil || c.lifetime.retired {
		return
	}
	e.stopOwnedLocked(c.trigger.ID, key, target, c.lifetime)
}

// Effectful drafts may own rows without a saved definition. Bind any matching
// live planning lifetime at the stop, without invalidating the whole context.
func (e *Engine) stopOwnedDraft(owner, key, target string) bool {
	e.managedMu.Lock()
	defer e.managedMu.Unlock()
	var lifetime *managedLifetime
	e.mu.RLock()
	for _, c := range e.compiled {
		if c.trigger.ID == owner {
			lifetime = c.lifetime
			break
		}
	}
	e.mu.RUnlock()
	return e.stopOwnedLocked(owner, key, target, lifetime)
}

// Caller holds managedMu; identity-scoped cancellation is shared by normal
// log stops and explicitly effectful previews. Silent tests never call here.
func (e *Engine) stopOwnedLocked(owner, key, target string, lifetime *managedLifetime) bool {
	if lifetime != nil {
		for p := range lifetime.pending {
			if p.key == key && p.target == target {
				p.revoked = true
			}
		}
	}
	if sink, ok := e.sink.(OwnedTimerSink); ok {
		sink.StopOwnedExternal(owner, key, target)
		return true
	}
	return false
}

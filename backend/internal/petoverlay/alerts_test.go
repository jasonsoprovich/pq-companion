package petoverlay

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

// overlayOn is an alert with only its overlay action enabled.
func overlayOn(threshold int) config.PetAlertSettings {
	return config.PetAlertSettings{Enabled: true, Threshold: threshold, OverlayEnabled: true}
}

// charmedPet brings up a Zeal-tracked charmed pet.
func charmedPet(h *harness) {
	h.charm, h.charmTgt = "Boltran`s Agacerie", "a Shissar Revenant"
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))
}

func (h *harness) takeFired() []trigger.TriggerFired {
	f := h.fired
	h.fired = nil
	return f
}

func TestAlerts_CharmExpiring(t *testing.T) {
	h := newHarness()
	h.prefs.CharmExpiring = overlayOn(30)
	charmedPet(h)

	h.charmExp = h.now.Add(60 * time.Second)
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired above the threshold: %+v", f)
	}

	h.advance(31 * time.Second) // 29s left
	h.tr.Tick()
	f := h.takeFired()
	if len(f) != 1 || f[0].MatchedLine != "Charm on a Shissar Revenant breaks in 29s" {
		t.Fatalf("fired = %+v", f)
	}
	if f[0].TriggerID != "system:pet:charm" || len(f[0].Actions) != 1 || f[0].Actions[0].Type != trigger.ActionOverlayText {
		t.Errorf("alert shape = %+v", f[0])
	}

	h.advance(5 * time.Second)
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired twice for one charm: %+v", f)
	}

	// Recast: a new expiry re-arms the alert.
	h.charmExp = h.now.Add(60 * time.Second)
	h.tr.Tick()
	h.advance(35 * time.Second)
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 1 {
		t.Fatalf("recast charm fired %d alerts, want 1", len(f))
	}
}

func TestAlerts_CharmShorterThanThresholdNeverFires(t *testing.T) {
	h := newHarness()
	h.prefs.CharmExpiring = overlayOn(30)
	charmedPet(h)
	// A bard charm whose whole duration is under the threshold: never seen
	// above it, so never armed — no alert the moment it lands.
	h.charmExp = h.now.Add(18 * time.Second)
	for i := 0; i < 20; i++ {
		h.tr.Tick()
		h.advance(time.Second)
	}
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired for a short charm: %+v", f)
	}
}

func TestAlerts_LowHPRearms(t *testing.T) {
	h := newHarness()
	h.prefs.LowHP = overlayOn(30)
	charmedPet(h)

	steps := []struct {
		hp   string
		fire bool
	}{
		{"50", false},
		{"28", true},  // crossed the threshold
		{"25", false}, // still low: once only
		{"35", false}, // recovered, but not 10 points above
		{"29", false}, // so not re-armed
		{"41", false}, // re-armed
		{"30", true},  // at the threshold counts
	}
	for i, st := range steps {
		h.advance(time.Second) // clear the HP broadcast rate limit
		h.tr.SetPipePetHPPercent(st.hp)
		h.tr.Tick()
		f := h.takeFired()
		if (len(f) == 1) != st.fire {
			t.Fatalf("step %d (hp %s): fired %d alerts, want fire=%v", i, st.hp, len(f), st.fire)
		}
		if st.fire && f[0].MatchedLine != "a Shissar Revenant at "+st.hp+"% HP" {
			t.Errorf("step %d text = %q", i, f[0].MatchedLine)
		}
	}
}

func TestAlerts_BuffMissing(t *testing.T) {
	h := newHarness()
	h.prefs.BuffMissing = overlayOn(0)
	h.prefs.WatchLists = map[string]config.PetWatchList{
		"osui": {
			Charm:    []string{"Speed of the Shissar", "Primal Essence"},
			Summoned: []string{"Burnout IV"},
		},
	}
	charmedPet(h)
	h.petTimers = []string{"Speed of the Shissar"}

	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired inside the grace period: %+v", f)
	}

	h.advance(buffMissingGrace)
	h.tr.Tick()
	f := h.takeFired()
	if len(f) != 1 || f[0].MatchedLine != "a Shissar Revenant is missing Primal Essence" {
		t.Fatalf("fired = %+v", f)
	}
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("repeated for the same missing buff: %+v", f)
	}

	// Another player's Primal Essence shows up in /pet health: present, so
	// the alert re-arms.
	h.tr.HandleLine(h.now, "I have 90 percent of my hit points left.")
	h.tr.HandleLine(h.now, "Primal Essence")
	h.tr.flushHealth()
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired while /pet health lists it: %+v", f)
	}

	// The reply goes stale with no timer behind it: missing again.
	h.advance(healthFreshForWatch + time.Second)
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 1 {
		t.Fatalf("stale reply: fired %d alerts, want 1", len(f))
	}
}

func TestAlerts_BuffMissingUsesKindList(t *testing.T) {
	h := newHarness()
	h.prefs.BuffMissing = overlayOn(0)
	h.prefs.WatchLists = map[string]config.PetWatchList{
		"osui": {Charm: []string{"Speed of the Shissar"}, Summoned: []string{"Burnout IV"}},
	}
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(9))
	h.advance(buffMissingGrace)
	h.tr.SetPipePetID(ip(9)) // settles to summoned
	h.tr.Tick()
	f := h.takeFired()
	if len(f) != 1 || f[0].MatchedLine != "Gobeker is missing Burnout IV" {
		t.Fatalf("fired = %+v", f)
	}
}

func TestAlerts_NoActionsNoFire(t *testing.T) {
	h := newHarness()
	h.prefs.LowHP = config.PetAlertSettings{Enabled: true, Threshold: 30}
	charmedPet(h)
	h.tr.SetPipePetHPPercent("10")
	h.tr.Tick()
	if f := h.takeFired(); len(f) != 0 {
		t.Fatalf("fired with every action off: %+v", f)
	}
}

func TestSwapDelayFromSettings(t *testing.T) {
	h := newHarness()
	h.prefs.SwapDelaySecs = 3
	charmedPet(h)
	h.tr.Handle(charmBroken())
	st := h.state()
	if st.SwapAt == nil || !st.SwapAt.Equal(h.now.Add(3*time.Second)) {
		t.Fatalf("swap at %v, want now+3s", st.SwapAt)
	}
}

func charmBroken() logparser.LogEvent {
	return logparser.LogEvent{Type: logparser.EventCharmBroken}
}

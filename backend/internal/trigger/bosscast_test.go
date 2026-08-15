package trigger

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func TestBossCastTracker_ResolveWithinWindow(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(5*time.Second), []string{"Aten Ha Ra"})
	if got != "Aten Ha Ra" {
		t.Errorf("resolveCaster = %q, want %q", got, "Aten Ha Ra")
	}
}

func TestBossCastTracker_ExpiresOutsideWindow(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(bossCastWindow+time.Second), []string{"Aten Ha Ra"})
	if got != "" {
		t.Errorf("resolveCaster after window expiry = %q, want empty", got)
	}
}

func TestBossCastTracker_IgnoresUnknownCasters(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	// A raid member's own bystander cast line — same log format, not a
	// known boss — must not be recorded at all.
	b.observe(now, "Healbot begins to cast a spell.")

	got := b.resolveCaster(now, []string{"Aten Ha Ra"})
	if got != "" {
		t.Errorf("resolveCaster = %q, want empty (unrelated caster shouldn't be tracked)", got)
	}
}

func TestBossCastTracker_PicksMostRecentAmongCandidates(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Diabo Xi Xin Thall begins to cast a spell.")
	b.observe(now.Add(2*time.Second), "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(3*time.Second), signatureSpellCasters["Silence of the Shadows"])
	if got != "Aten Ha Ra" {
		t.Errorf("resolveCaster = %q, want most recent caster %q", got, "Aten Ha Ra")
	}
}

// TestEngine_SignatureSpellBindsToActualCaster_NotLiveTarget is the core
// scenario this feature exists for: the player is off-tanking an add (or
// has nothing targeted) when the boss's signature spell lands. The old
// combat-target fallback would bind the timer to the add (or leave it
// target-less); the caster-based correlator should bind it to the boss that
// actually cast it, using the "<boss> begins to cast a spell." line that
// preceded the land text.
func TestEngine_SignatureSpellBindsToActualCaster_NotLiveTarget(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)
	e.SetTargetProvider(func() string { return "an alligator" }) // off-tanking an add

	fling := findSignatureTrigger(t, "Fling")
	fling.ID = "test-fling-caster"
	if err := s.Insert(fling); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	base := time.Now()
	e.Handle(base, "Aten Ha Ra begins to cast a spell.")
	e.Handle(base.Add(2*time.Second), "Bob is knocked into the air by a massive force.")

	if sink.calls != 1 {
		t.Fatalf("expected 1 StartExternal call, got %d", sink.calls)
	}
	if sink.target != "Aten Ha Ra" {
		t.Errorf("timer target_name = %q, want %q (actual caster, not the live target %q)", sink.target, "Aten Ha Ra", "an alligator")
	}
}

// TestEngine_SignatureSpellFallsBackToLiveTargetWhenNoCastObserved preserves
// the old behavior when no boss cast-start line was seen — e.g. the app
// started mid-fight, or the log line scrolled past before this session's log
// tailer opened the file.
func TestEngine_SignatureSpellFallsBackToLiveTargetWhenNoCastObserved(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)
	e.SetTargetProvider(func() string { return "Aten Ha Ra" })

	fling := findSignatureTrigger(t, "Fling")
	fling.ID = "test-fling-fallback"
	if err := s.Insert(fling); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Bob is knocked into the air by a massive force.")

	if sink.calls != 1 || sink.target != "Aten Ha Ra" {
		t.Errorf("StartExternal target = %+v, want target Aten Ha Ra (live-target fallback)", sink)
	}
}

// TestEngine_CausticMistDisambiguatesCasterWithoutTarget verifies the
// Caustic Mist / Putrefy Flesh text-collision case (see the trigger's own
// doc comment): the caster-based correlator can tell the two fights apart
// from the "begins to cast" line alone, with no help from the live target at
// all — something the old target-fallback could never do since both spells'
// land text is byte-identical.
func TestEngine_CausticMistDisambiguatesCasterWithoutTarget(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)
	e.SetTargetProvider(func() string { return "" }) // nothing targeted

	tr := findSignatureTrigger(t, "Caustic Mist / Putrefy Flesh")
	tr.ID = "test-caustic-zlandicar"
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	base := time.Now()
	e.Handle(base, "Zlandicar begins to cast a spell.")
	e.Handle(base.Add(2*time.Second), "Larcen's flesh begins to liquefy.")

	if sink.calls != 1 {
		t.Fatalf("expected 1 StartExternal call, got %d", sink.calls)
	}
	if sink.target != "Zlandicar" {
		t.Errorf("timer target_name = %q, want %q", sink.target, "Zlandicar")
	}
}

func findSignatureTrigger(t *testing.T, name string) *Trigger {
	t.Helper()
	for _, tr := range raidSignatureSpellAlerts() {
		if tr.Name == name {
			cp := tr
			return &cp
		}
	}
	t.Fatalf("signature trigger %q not found in raidSignatureSpellAlerts()", name)
	return nil
}

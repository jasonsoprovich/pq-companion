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

	got := b.resolveCaster(now.Add(5*time.Second), []string{"Aten Ha Ra"}, "")
	if got != "Aten Ha Ra" {
		t.Errorf("resolveCaster = %q, want %q", got, "Aten Ha Ra")
	}
}

func TestBossCastTracker_ExpiresOutsideWindow(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(bossCastWindow+time.Second), []string{"Aten Ha Ra"}, "")
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

	got := b.resolveCaster(now, []string{"Aten Ha Ra"}, "")
	if got != "" {
		t.Errorf("resolveCaster = %q, want empty (unrelated caster shouldn't be tracked)", got)
	}
}

func TestBossCastTracker_PicksMostRecentAmongCandidates(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Diabo Xi Xin Thall begins to cast a spell.")
	b.observe(now.Add(2*time.Second), "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(3*time.Second), signatureSpellCasters["Silence of the Shadows"], "")
	if got != "Aten Ha Ra" {
		t.Errorf("resolveCaster = %q, want most recent caster %q", got, "Aten Ha Ra")
	}
}

// TestBossCastTracker_PrefersLiveTargetOverMostRecent covers the real-world
// case a Vex Thal Ring War report surfaced (2026-08-18): the player is
// targeting "Kaas Thox Xi Ans Dyek", which really is casting Fling, but
// another Fling candidate from the same Aten Ha Ra family ("Kaas Thox Xi
// Aten Ha Ra") logged some other cast-start line a moment later — e.g. a
// second add up at the same time, or overlap from a prior phase. Picking
// "most recent among all candidates" would bind the timer to the wrong add,
// which the general Detrimental panel wouldn't visibly flag (it just prints
// whatever name it's given) but which silently drops the row from the NPC
// overlay's Timers tab, since that view exact-matches against the live
// target. The live target, once confirmed to itself be casting within the
// window, should win regardless of which candidate cast most recently.
func TestBossCastTracker_PrefersLiveTargetOverMostRecent(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Kaas Thox Xi Ans Dyek begins to cast a spell.")
	b.observe(now.Add(2*time.Second), "Kaas Thox Xi Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(3*time.Second), signatureSpellCasters["Fling"], "Kaas Thox Xi Ans Dyek")
	if got != "Kaas Thox Xi Ans Dyek" {
		t.Errorf("resolveCaster = %q, want live target %q even though it wasn't the most recent observed cast", got, "Kaas Thox Xi Ans Dyek")
	}
}

// TestBossCastTracker_LiveTargetIgnoredIfNotObservedCasting ensures the live
// target only wins when it was itself independently confirmed casting within
// the window — otherwise it's just a guess, no better than the old
// live-target fallback this feature was built to avoid (off-tanking an add,
// or a stale/irrelevant target left over from before the current cast).
func TestBossCastTracker_LiveTargetIgnoredIfNotObservedCasting(t *testing.T) {
	b := newBossCastTracker()
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got := b.resolveCaster(now.Add(2*time.Second), signatureSpellCasters["Fling"], "Kaas Thox Xi Ans Dyek")
	if got != "Aten Ha Ra" {
		t.Errorf("resolveCaster = %q, want most-recent fallback %q since the live target was never observed casting", got, "Aten Ha Ra")
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
	if !sink.targetIsCaster {
		t.Error("targetIsCaster = false, want true (Fling is a known signature-spell trigger)")
	}
}

// TestEngine_OrdinaryDetrimentalDoesNotSetTargetIsCaster verifies the badge
// flag stays false for a normal detrimental trigger (not in
// signatureSpellCasters) that falls back to the live combat target — e.g. a
// user's own Tash/slow tracking trigger. Only the known raid-boss
// signature-spell triggers should ever badge as "caster-bound".
func TestEngine_OrdinaryDetrimentalDoesNotSetTargetIsCaster(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)
	e.SetTargetProvider(func() string { return "a gnoll" })

	tr := &Trigger{
		ID:                "ordinary-debuff",
		Name:              "Tash Landed",
		Enabled:           true,
		Pattern:           `^.+ looks weaker\.$`,
		TimerType:         TimerTypeDetrimental,
		TimerDurationSecs: 30,
		Actions:           []Action{},
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "A gnoll pup looks weaker.")

	if sink.calls != 1 || sink.target != "a gnoll" {
		t.Fatalf("StartExternal target = %+v, want target %q", sink, "a gnoll")
	}
	if sink.targetIsCaster {
		t.Error("targetIsCaster = true, want false (not a known signature-spell trigger)")
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

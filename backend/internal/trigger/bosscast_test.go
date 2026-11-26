package trigger

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// bossTrig builds a boss-caster trigger for tracker unit tests.
func bossTrig(id string, casters ...string) *Trigger {
	t := &Trigger{ID: id, TimerType: TimerTypeDetrimental}
	for _, c := range casters {
		t.BossCasters = append(t.BossCasters, BossCaster{Name: c})
	}
	return t
}

// trackerFor returns a tracker that records casts for every caster of ts.
func trackerFor(ts ...*Trigger) *bossCastTracker {
	b := newBossCastTracker()
	b.setKnown(ts)
	return b
}

func TestBossCastTracker_BindsWithinWindow(t *testing.T) {
	tr := bossTrig("fling", "Aten Ha Ra")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got, ok := b.bind(tr, now.Add(5*time.Second), "")
	if !ok || got != "Aten Ha Ra" {
		t.Errorf("bind = %q, %v; want Aten Ha Ra, true", got, ok)
	}
}

func TestBossCastTracker_ExpiresOutsideWindow(t *testing.T) {
	tr := bossTrig("fling", "Aten Ha Ra")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	got, ok := b.bind(tr, now.Add(bossCastWindow+time.Second), "")
	if !ok || got != "" {
		t.Errorf("bind after expiry = %q, %v; want live-target fallback (\"\", true)", got, ok)
	}
}

func TestBossCastTracker_IgnoresUnknownCasters(t *testing.T) {
	tr := bossTrig("fling", "Aten Ha Ra")
	b := trackerFor(tr)
	now := time.Now()
	// A raid member's own bystander cast line — same log format, not a
	// known boss — must not be recorded at all.
	b.observe(now, "Healbot begins to cast a spell.")

	if got, _ := b.bind(tr, now, ""); got != "" {
		t.Errorf("bind = %q, want empty (unrelated caster shouldn't be tracked)", got)
	}
}

func TestBossCastTracker_OldestUnconsumedWins(t *testing.T) {
	tr := bossTrig("silence", "Aten Ha Ra", "Diabo Xi Xin Thall", "Thall Va Xakra")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Diabo Xi Xin Thall begins to cast a spell.")
	b.observe(now.Add(2*time.Second), "Aten Ha Ra begins to cast a spell.")

	if got, _ := b.bind(tr, now.Add(3*time.Second), ""); got != "Diabo Xi Xin Thall" {
		t.Errorf("first land bound to %q, want oldest caster Diabo Xi Xin Thall", got)
	}
	if got, _ := b.bind(tr, now.Add(4*time.Second), ""); got != "Aten Ha Ra" {
		t.Errorf("second land bound to %q, want the remaining caster Aten Ha Ra", got)
	}
}

// TestBossCastTracker_PrefersLiveTarget covers the Vex Thal Ring War report
// (2026-08-18): the player targets "Kaas Thox Xi Ans Dyek", which really is
// casting Fling, while another Fling candidate logged a cast-start too. The
// live target, independently confirmed casting, wins over the oldest.
func TestBossCastTracker_PrefersLiveTarget(t *testing.T) {
	tr := bossTrig("fling", "Kaas Thox Xi Ans Dyek", "Kaas Thox Xi Aten Ha Ra")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Kaas Thox Xi Aten Ha Ra begins to cast a spell.")
	b.observe(now.Add(2*time.Second), "Kaas Thox Xi Ans Dyek begins to cast a spell.")

	if got, _ := b.bind(tr, now.Add(3*time.Second), "Kaas Thox Xi Ans Dyek"); got != "Kaas Thox Xi Ans Dyek" {
		t.Errorf("bind = %q, want live target Kaas Thox Xi Ans Dyek", got)
	}
}

// TestBossCastTracker_LiveTargetIgnoredIfNotObservedCasting: the live target
// only wins when it was itself seen casting within the window.
func TestBossCastTracker_LiveTargetIgnoredIfNotObservedCasting(t *testing.T) {
	tr := bossTrig("fling", "Aten Ha Ra", "Kaas Thox Xi Ans Dyek")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")

	if got, _ := b.bind(tr, now.Add(2*time.Second), "Kaas Thox Xi Ans Dyek"); got != "Aten Ha Ra" {
		t.Errorf("bind = %q, want Aten Ha Ra (live target never observed casting)", got)
	}
}

// TestBossCastTracker_AEDuplicatesCollapse: one boss's PBAE logs a land line
// per raid member hit. The first binds; the rest are duplicates.
func TestBossCastTracker_AEDuplicatesCollapse(t *testing.T) {
	tr := bossTrig("fling", "Aten Ha Ra")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Aten Ha Ra begins to cast a spell.")
	land := now.Add(3 * time.Second)

	if _, ok := b.bind(tr, land, ""); !ok {
		t.Fatal("first land line suppressed, want fire")
	}
	for i := 0; i < 5; i++ {
		if _, ok := b.bind(tr, land, ""); ok {
			t.Fatalf("duplicate line %d fired, want suppressed", i+1)
		}
	}
	// A recast well after the duplicate window fires again (fallback).
	if _, ok := b.bind(tr, land.Add(30*time.Second), ""); !ok {
		t.Error("next cast after the window suppressed, want fire")
	}
}

// TestBossCastTracker_SameSecondOverlap is the worked case from bind's doc:
// A and B start in the same second, then A's land + duplicate and B's land +
// duplicate, all with one-second timestamps. One binding per boss.
func TestBossCastTracker_SameSecondOverlap(t *testing.T) {
	tr := bossTrig("gravel", "War Chieftan Awisano", "War Chieftan Birak")
	b := trackerFor(tr)
	t0 := time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC)
	b.observe(t0, "War Chieftan Awisano begins to cast a spell.")
	b.observe(t0, "War Chieftan Birak begins to cast a spell.")

	land := t0.Add(3 * time.Second)
	var bound []string
	fires := 0
	for i := 0; i < 4; i++ { // A land, A dup, B land, B dup
		if c, ok := b.bind(tr, land, ""); ok {
			fires++
			bound = append(bound, c)
		}
	}
	if fires != 2 || bound[0] != "War Chieftan Awisano" || bound[1] != "War Chieftan Birak" {
		t.Errorf("fires=%d bound=%v; want 2 fires bound to Awisano then Birak", fires, bound)
	}
}

// TestBossCastTracker_PerTriggerConsumption: a cast-start line doesn't say
// which spell, so one boss's observation feeds each of its tracked spells'
// triggers once. Casting spell X never uses up spell Y's evidence.
func TestBossCastTracker_PerTriggerConsumption(t *testing.T) {
	x := bossTrig("comet", "Warlord Gintolaken")
	y := bossTrig("gravel", "Warlord Gintolaken")
	b := trackerFor(x, y)
	now := time.Now()
	b.observe(now, "Warlord Gintolaken begins to cast a spell.")

	if got, _ := b.bind(x, now.Add(2*time.Second), ""); got != "Warlord Gintolaken" {
		t.Fatalf("spell X bound to %q, want Warlord Gintolaken", got)
	}
	if got, _ := b.bind(y, now.Add(3*time.Second), ""); got != "Warlord Gintolaken" {
		t.Errorf("spell Y bound to %q, want Warlord Gintolaken (X must not consume Y's evidence)", got)
	}
}

// TestBossCastTracker_InterruptedCastExpires: a cast that never produced a
// land line must not bind the boss's next cast once the window has passed.
func TestBossCastTracker_InterruptedCastExpires(t *testing.T) {
	tr := bossTrig("comet", "Warlord Gintolaken")
	b := trackerFor(tr)
	now := time.Now()
	b.observe(now, "Warlord Gintolaken begins to cast a spell.") // interrupted

	later := now.Add(bossCastWindow + 10*time.Second)
	b.observe(later, "Warlord Gintolaken begins to cast a spell.")
	if got, _ := b.bind(tr, later.Add(2*time.Second), ""); got != "Warlord Gintolaken" {
		t.Fatalf("bind = %q, want Warlord Gintolaken", got)
	}
	// The stale observation is gone, so an immediate second land line is a
	// duplicate rather than a second binding.
	if _, ok := b.bind(tr, later.Add(2*time.Second), ""); ok {
		t.Error("stale interrupted cast produced a second binding")
	}
}

// recordSink records every timer start, for tests that expect several.
type recordSink struct {
	starts []recordedStart
}

type recordedStart struct {
	target   string
	duration float64
	isCaster bool
}

func (s *recordSink) StartExternal(name, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string, pinned bool, customGroup string, targetIsCaster bool, stack ...bool) {
	s.starts = append(s.starts, recordedStart{target: targetName, duration: durationSecs, isCaster: targetIsCaster})
}
func (s *recordSink) StopExternal(name string, spellID int) {}

// TestEngine_TwoBossesSameSpellGetTwoTimers: Gravel Rain cast by two war
// chieftains inside the duplicate window — each gets its own countdown.
func TestEngine_TwoBossesSameSpellGetTwoTimers(t *testing.T) {
	s := openTestStore(t)
	sink := &recordSink{}
	e := NewEngine(s, ws.NewHub(), sink, nil)
	tr := &Trigger{
		ID: "gravel", Name: "Gravel Rain", Enabled: true,
		Pattern:   `^(?:A cloud of gravel pelts you and covers the ground\.|\S+ is pelted by a cloud of gravel\.)$`,
		TimerType: TimerTypeDetrimental, TimerDurationSecs: 10, Actions: []Action{},
		BossCasters: []BossCaster{{Name: "War Chieftan Awisano"}, {Name: "War Chieftan Birak"}},
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	t0 := time.Now()
	e.Handle(t0, "War Chieftan Awisano begins to cast a spell.")
	e.Handle(t0.Add(time.Second), "War Chieftan Birak begins to cast a spell.")
	e.Handle(t0.Add(3*time.Second), "Bob is pelted by a cloud of gravel.")
	e.Handle(t0.Add(3*time.Second), "Sue is pelted by a cloud of gravel.")
	e.Handle(t0.Add(4*time.Second), "Bob is pelted by a cloud of gravel.")

	if len(sink.starts) != 2 {
		t.Fatalf("got %d timers, want 2: %+v", len(sink.starts), sink.starts)
	}
	if sink.starts[0].target != "War Chieftan Awisano" || sink.starts[1].target != "War Chieftan Birak" {
		t.Errorf("timer targets = %q, %q; want Awisano then Birak", sink.starts[0].target, sink.starts[1].target)
	}
	for _, st := range sink.starts {
		if !st.isCaster {
			t.Error("targetIsCaster = false, want true for a boss-caster trigger")
		}
	}
}

// TestEngine_PerCasterDurationOverride: one shared land line, two bosses
// with different recasts (Caustic Mist 24s, Putrefy Flesh 30s).
func TestEngine_PerCasterDurationOverride(t *testing.T) {
	s := openTestStore(t)
	sink := &recordSink{}
	e := NewEngine(s, ws.NewHub(), sink, nil)
	tr := findSignatureTrigger(t, "Caustic Mist / Putrefy Flesh")
	tr.ID = "caustic"
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	t0 := time.Now()
	e.Handle(t0, "Zlandicar begins to cast a spell.")
	e.Handle(t0.Add(2*time.Second), "Larcen's flesh begins to liquefy.")
	e.Handle(t0.Add(60*time.Second), "Vyzh`dra the Cursed begins to cast a spell.")
	e.Handle(t0.Add(62*time.Second), "Larcen's flesh begins to liquefy.")

	if len(sink.starts) != 2 {
		t.Fatalf("got %d timers, want 2", len(sink.starts))
	}
	if sink.starts[0].duration != 30 || sink.starts[1].duration != 24 {
		t.Errorf("durations = %v, %v; want Zlandicar 30, Vyzh`dra 24", sink.starts[0].duration, sink.starts[1].duration)
	}
}

// TestEngine_RenamedBossTriggerKeepsBinding: binding comes from the
// trigger's BossCasters, not its display name.
func TestEngine_RenamedBossTriggerKeepsBinding(t *testing.T) {
	s := openTestStore(t)
	sink := &captureSink{}
	e := NewEngine(s, ws.NewHub(), sink, nil)
	tr := findSignatureTrigger(t, "Fling")
	tr.ID = "renamed-fling"
	tr.Name = "My Fling Warning"
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	t0 := time.Now()
	e.Handle(t0, "Aten Ha Ra begins to cast a spell.")
	e.Handle(t0.Add(2*time.Second), "Bob is knocked into the air by a massive force.")
	if sink.target != "Aten Ha Ra" || !sink.targetIsCaster {
		t.Errorf("target=%q isCaster=%v; want Aten Ha Ra, true", sink.target, sink.targetIsCaster)
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

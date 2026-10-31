package killtracker

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type fakeHub struct {
	mu     sync.Mutex
	events []ws.Event
}

func (f *fakeHub) Broadcast(ev ws.Event) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
}

// alerts returns the text of every synthetic trigger:fired event.
func (f *fakeHub) alerts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, ev := range f.events {
		if ev.Type == trigger.WSEventTriggerFired {
			out = append(out, ev.Data.(trigger.TriggerFired).MatchedLine)
		}
	}
	return out
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var t0 = time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)

func killEv(at time.Time, target, killer string) logparser.LogEvent {
	return logparser.LogEvent{Type: logparser.EventKill, Timestamp: at, Data: logparser.KillData{Target: target, Killer: killer}}
}

func zoneEv(at time.Time, zone string) logparser.LogEvent {
	return logparser.LogEvent{Type: logparser.EventZone, Timestamp: at, Data: logparser.ZoneData{ZoneName: zone}}
}

func TestIsProperName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Darwol Adan", true},
		{"Grummus", true},
		{"a gnoll", false},
		{"an owlbear", false},
		{"A Shissar Wrath", false},
		{"An Ancient Wyvern", false},
		{"Knight of Decay", true}, // known false positive, documented
		{"", false},
	}
	for _, tt := range tests {
		if got := isProperName(tt.name); got != tt.want {
			t.Errorf("isProperName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestKillLogDedupAndSeq(t *testing.T) {
	s := openTestStore(t)
	players := map[string]bool{"osui": true}
	classify := func(n string) bool { return !players[strings.ToLower(n)] }

	feed := func(h interface{ HandleEvent(logparser.LogEvent) }) {
		h.HandleEvent(zoneEv(t0, "The Crypt of Decay"))
		// AE pull: two same-name kills by the same killer in one second.
		h.HandleEvent(killEv(t0.Add(time.Second), "Knight of Decay", "Ineka"))
		h.HandleEvent(killEv(t0.Add(time.Second), "Knight of Decay", "Ineka"))
		h.HandleEvent(killEv(t0.Add(2*time.Second), "Darwol Adan", "You"))
		h.HandleEvent(killEv(t0.Add(3*time.Second), "Osui", "Darwol Adan"))
	}

	live := NewLogger(s, classify, func() string { return "Osui" })
	var got []Entry
	live.SetOnInsert(func(e Entry) { got = append(got, e) })
	feed(live)
	if len(got) != 4 {
		t.Fatalf("live inserts = %d, want 4", len(got))
	}

	// A backfill of the same log must not duplicate anything.
	bf := NewBackfillHandler(s, classify, "Osui")
	feed(bf)
	if bf.Inserted() != 0 {
		t.Fatalf("backfill inserted %d duplicates", bf.Inserted())
	}

	npc, err := s.List(Filters{Character: "Osui"})
	if err != nil {
		t.Fatal(err)
	}
	if len(npc) != 3 {
		t.Fatalf("npc scope rows = %d, want 3 (player death excluded)", len(npc))
	}
	all, _ := s.List(Filters{Character: "Osui", Scope: ScopeAll})
	if len(all) != 4 {
		t.Fatalf("all scope rows = %d, want 4", len(all))
	}
	named, _ := s.List(Filters{Character: "Osui", Scope: ScopeNamed, SortDesc: true})
	if len(named) != 3 || named[0].Target != "Darwol Adan" || named[0].Killer != "Osui" {
		t.Fatalf("named rows = %+v", named)
	}
	if named[0].Zone != "The Crypt of Decay" {
		t.Errorf("zone = %q", named[0].Zone)
	}
}

func newTestCounter(t *testing.T, e *Engine, def CounterDef) Counter {
	t.Helper()
	def, err := Validate(def)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	c, err := e.Create(def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c
}

func countOf(e *Engine, id string) Counter {
	for _, c := range e.State().Counters {
		if c.Def.ID == id {
			return c
		}
	}
	return Counter{}
}

func TestCounterKillsZoneGate(t *testing.T) {
	e := NewEngine(nil, &fakeHub{})
	c := newTestCounter(t, e, CounterDef{
		Name: "Knights", Enabled: true, Mobs: []string{"knight of decay"}, Zone: "The Crypt of Decay",
	})

	// Unknown zone still counts (app restarted mid-raid).
	e.Handle(killEv(t0, "Knight of Decay", "x"))
	e.Handle(zoneEv(t0, "The Plane of Knowledge"))
	e.Handle(killEv(t0, "Knight of Decay", "x"))
	e.Handle(zoneEv(t0, "The Crypt of Decay"))
	e.Handle(killEv(t0, "Knight of Decay", "x"))
	e.Handle(killEv(t0, "a gnoll", "x"))

	if got := countOf(e, c.Def.ID).Count; got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestCounterPatternSource(t *testing.T) {
	e := NewEngine(nil, nil)
	c := newTestCounter(t, e, CounterDef{Name: "XP", Enabled: true, Pattern: `^You gain (party |raid )?experience`})
	e.HandleLine(t0, "You gain raid experience!!")
	e.HandleLine(t0, "You gain experience!!")
	e.HandleLine(t0, "Osui tells the raid, 'You gain experience'")
	if got := countOf(e, c.Def.ID).Count; got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestCounterDisabledIgnored(t *testing.T) {
	e := NewEngine(nil, nil)
	c := newTestCounter(t, e, CounterDef{Name: "X", Enabled: false, Mobs: []string{"a gnoll"}})
	e.Handle(killEv(t0, "a gnoll", ""))
	if got := countOf(e, c.Def.ID).Count; got != 0 {
		t.Fatalf("disabled counter counted: %d", got)
	}
}

func TestValidateRejects(t *testing.T) {
	if _, err := Validate(CounterDef{Name: "", Mobs: []string{"x"}}); err == nil {
		t.Error("empty name accepted")
	}
	if _, err := Validate(CounterDef{Name: "x"}); err == nil {
		t.Error("counter with no source accepted")
	}
	if _, err := Validate(CounterDef{Name: "x", Pattern: "("}); err == nil {
		t.Error("bad regex accepted")
	}
	def, err := Validate(CounterDef{Name: " x ", Mobs: []string{"a", " A ", ""}, Milestones: []Milestone{{At: 9}, {At: 0}, {At: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "x" || len(def.Mobs) != 1 || len(def.Milestones) != 2 || def.Milestones[0].At != 3 {
		t.Errorf("normalized def = %+v", def)
	}
}

func installBertox(t *testing.T, e *Engine) (trash, kings Counter) {
	t.Helper()
	cs, err := e.InstallPreset("bertox")
	if err != nil {
		t.Fatalf("InstallPreset: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("preset counters = %d", len(cs))
	}
	return cs[0], cs[1]
}

func TestBertoxPresetFlow(t *testing.T) {
	hub := &fakeHub{}
	e := NewEngine(nil, hub)
	trash, kings := installBertox(t, e)
	if !e.HasPreset("bertox") {
		t.Fatal("HasPreset false after install")
	}

	e.Handle(zoneEv(t0, "The Crypt of Decay"))
	// Stale count from a previous attempt is wiped by the Spectre emote,
	// which also starts the clock.
	_ = e.SetCount(trash.Def.ID, 17)
	e.HandleLine(t0, "Crazed laughter is heard as you notice a foul creature standing before you. The creature then speaks saying, 'Violaters...'")
	got := countOf(e, trash.Def.ID)
	if got.Count != 0 || got.ClockStartedAt != t0.Unix() {
		t.Fatalf("after spectre: count=%d clock=%d", got.Count, got.ClockStartedAt)
	}

	// 42 visible kills → heads-up for Darwol at 44.
	for i := 0; i < 42; i++ {
		e.Handle(killEv(t0.Add(time.Duration(400+i)*time.Second), "Elite Priest of Decay", "x"))
	}
	if a := hub.alerts(); len(a) != 1 || a[0] != "Darwol Adan spawns in 2 kills" {
		t.Fatalf("alerts after 42 = %q", a)
	}

	// Missed a few kills in another wing; the Feig emote resyncs to 48 and
	// announces the highest milestone crossed.
	// Quarm's wording (SecretsOTheP/quests); upstream EQMacEmu uses "Feig Adan, your master".
	e.HandleLine(t0, "An unsettling feeling of fear passes through you as the summoners cry, 'We call to you, corrupted King of Lxanvom, Feig Adan! Your master has need of you!' A bestial squeak thunders through the crypt as a foul fiend of Bertoxxulous is summoned forth.")
	if got := countOf(e, trash.Def.ID).Count; got != 48 {
		t.Fatalf("after Feig emote count = %d, want 48", got)
	}
	a := hub.alerts()
	if a[len(a)-1] != "Feig Adan spawns (48)" {
		t.Fatalf("last alert = %q", a[len(a)-1])
	}

	// The 8th king death is announced; kings resync to 8.
	// Kings count from the zone-wide fall line, not kill lines.
	e.Handle(killEv(t0, "Darwol Adan", "x"))
	if got := countOf(e, kings.Def.ID).Count; got != 0 {
		t.Fatalf("kings counted a kill line: %d", got)
	}
	e.HandleLine(t0, "The corrupted king, Darwol Adan, falls. His dying cry echoes through the depths of Lxanvom.")
	if got := countOf(e, kings.Def.ID).Count; got != 1 {
		t.Fatalf("kings after fall line = %d, want 1", got)
	}
	e.HandleLine(t0, "An unsettling feeling of fear passes through you as you hear the summoners finish a dark incantation then cry out saying, 'We call to you the last corrupted Kings of Lxanvom. Meedo Adan! Qezzin Adan! Pzo Adan! Bhaly Adan! Your master has need of you!'")
	if got := countOf(e, kings.Def.ID).Count; got != 8 {
		t.Fatalf("kings = %d, want 8", got)
	}

	// The expiry warning pins the clock even if the start was missed.
	warnAt := t0.Add(2 * time.Hour)
	e.HandleLine(warnAt, "Dark voices whisper in your ear saying, 'Your time is close in coming to an end. Time to flee little ones!'")
	if got := countOf(e, trash.Def.ID).ClockStartedAt; got != warnAt.Add(-7080*time.Second).Unix() {
		t.Fatalf("clock after warning = %d", got)
	}

	// Bertox's death stops the clock without touching counts.
	e.HandleLine(warnAt, "A nimbus of light floods throughs the crypt in one magnificent wave as an earth shattering howl is heard.  The bringer of plagues, lord of all disease and decay, Bertoxxulous has been defeated.")
	got = countOf(e, trash.Def.ID)
	if got.ClockStartedAt != 0 || got.Count != 48 {
		t.Fatalf("after bertox death: %+v", got)
	}
}

func TestCountersPersist(t *testing.T) {
	s := openTestStore(t)
	e := NewEngine(s, nil)
	trash, _ := installBertox(t, e)
	e.Handle(killEv(t0, "Knight of Decay", ""))
	e.Handle(killEv(t0, "Knight of Decay", ""))

	e2 := NewEngine(s, nil)
	st := e2.State()
	if len(st.Counters) != 2 {
		t.Fatalf("reloaded %d counters", len(st.Counters))
	}
	if st.Counters[0].Def.ID != trash.Def.ID || st.Counters[0].Count != 2 {
		t.Fatalf("reloaded first counter = %+v", st.Counters[0])
	}
	if err := e2.Delete(trash.Def.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(NewEngine(s, nil).State().Counters); n != 1 {
		t.Fatalf("after delete reload = %d", n)
	}
}

func TestAllPresetsInstall(t *testing.T) {
	for _, p := range Presets() {
		e := NewEngine(nil, nil)
		cs, err := e.InstallPreset(p.Key)
		if err != nil {
			t.Fatalf("preset %s: %v", p.Key, err)
		}
		if len(cs) != len(p.Counters) || p.Zone == "" {
			t.Fatalf("preset %s: installed %d of %d, zone %q", p.Key, len(cs), len(p.Counters), p.Zone)
		}
		for _, c := range cs {
			if c.Def.Preset != p.Key {
				t.Errorf("preset %s: counter %q missing preset key", p.Key, c.Def.Name)
			}
		}
	}
}

func TestSlainPattern(t *testing.T) {
	re := regexp.MustCompile(slainPattern("Ston`Ruak, Ancient of the Trees"))
	for _, line := range []string{
		"Ston`Ruak, Ancient of the Trees has been slain by Osui!",
		"You have slain Ston`Ruak, Ancient of the Trees!",
		"Ineka has slain Ston`Ruak, Ancient of the Trees!",
		"Ston`Ruak, Ancient of the Trees died.",
	} {
		if !re.MatchString(line) {
			t.Errorf("no match: %q", line)
		}
	}
	for _, line := range []string{
		"Ston`Ruak, Ancient of the Trees hits YOU for 900 points of damage.",
		"Osui hits Ston`Ruak, Ancient of the Trees for 120 points of damage.",
	} {
		if re.MatchString(line) {
			t.Errorf("false match: %q", line)
		}
	}
}

func installOne(t *testing.T, e *Engine, key string) []Counter {
	t.Helper()
	cs, err := e.InstallPreset(key)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestCoirnavClockAndSync(t *testing.T) {
	e := NewEngine(nil, nil)
	c := installOne(t, e, "coirnav")[0]
	_ = e.SetCount(c.Def.ID, 30)
	e.HandleLine(t0, `Coirnav the Avatar of Water shouts, "Those that violate my domain will pay. I call upon the power imbued to me by Povar! Come forth my minions of vapor and destroy these intruders."`)
	got := countOf(e, c.Def.ID)
	if got.Count != 0 || got.ClockStartedAt != t0.Unix() {
		t.Fatalf("after start: %+v", got)
	}
	// The 12-minute warning pins the clock even if the start was missed.
	at := t0.Add(13 * time.Minute)
	e.HandleLine(at, `Ice spreads across the reef as vapor coils around Coirnav's towering form. His voice rolls through the depths. "Our power gathers, and your time dwindles. Struggle against the currents!"`)
	if got := countOf(e, c.Def.ID).ClockStartedAt; got != at.Add(-720*time.Second).Unix() {
		t.Fatalf("clock after warning 2 = %d", got)
	}
	// The first warning names all three gods; it must not reset or restart at 0.
	e.HandleLine(at, `The waters churn as three voices rise as one through Coirnav, echoing throughout the reef. "The powers of Povar, E`+"`"+`ci and Tarew Marr are bound within me. You will not break the will of the Triumvirate!"`)
	if got := countOf(e, c.Def.ID).ClockStartedAt; got != at.Add(-600*time.Second).Unix() {
		t.Fatalf("clock after warning 1 = %d", got)
	}
	e.HandleLine(at, "The currents wrench across the reef as Pwelon, Nrinda and Vamuil gather before Coirnav.")
	if got := countOf(e, c.Def.ID).Count; got != 75 {
		t.Fatalf("count after miniboss line = %d", got)
	}
}

func TestFenninStagesAndWarnings(t *testing.T) {
	e := NewEngine(nil, nil)
	c := installOne(t, e, "fennin")[0]
	e.HandleLine(t0, "The Guardian of Doomfire crashes to the scorched earth. Frenzied cries erupt as an Army of Flames burns across the scorched realm of Doomfire.")
	if got := countOf(e, c.Def.ID).ClockStartedAt; got != t0.Unix() {
		t.Fatalf("clock start = %d", got)
	}
	e.HandleLine(t0, "The commanders collapse beneath the onslaught. Columns of fire tear through the air as the Council of Fire advances.")
	if got := countOf(e, c.Def.ID).Count; got != 61 {
		t.Fatalf("count = %d, want 61", got)
	}
	at := t0.Add(3 * time.Hour)
	e.HandleLine(at, `All of Doomfire trembles as the voice of Fennin Ro booms, "Hear the roar of Doomfire! Flame and Chaos answer to me! We will see what remains when my fury meets your ambition!"`)
	if got := countOf(e, c.Def.ID).ClockStartedAt; got != at.Add(-4*2700*time.Second).Unix() {
		t.Fatalf("clock after warning 4 = %d", got)
	}
	e.HandleLine(at, "Fennin Ro, the Tyrant of Fire, falls! His colossal form crashes into the scorched earth.")
	if got := countOf(e, c.Def.ID).ClockStartedAt; got != 0 {
		t.Fatalf("clock not stopped: %d", got)
	}
}

func TestStoneRingZoneEmotes(t *testing.T) {
	e := NewEngine(nil, nil)
	cs := installOne(t, e, "stonering")
	forts, heaps := cs[0], cs[1]
	e.HandleLine(t0, "Massive slabs of stone wrench themselves from the earth and lock together. The living fortifications advance with crushing weight, each stride grinding rock beneath them.")
	if got := countOf(e, forts.Def.ID); got.ClockStartedAt != t0.Unix() {
		t.Fatalf("clock not started: %+v", got)
	}
	e.HandleLine(t0, "The last fortification shatters with a thunderous crack. The Rock Monstrosity's massive frame splits into four mounds of rubble, each grinding with the strength of the broken whole.")
	if got := countOf(e, forts.Def.ID).Count; got != 43 {
		t.Fatalf("fortification sync = %d", got)
	}
	_ = e.SetCount(heaps.Def.ID, 7)
	e.HandleLine(t0, "The Rock Monstrosity crashes apart, its weight shaking the temple. A towering stone figure rises and folds into stillness as heaps of broken rock grind forward to crush the intruders.")
	if got := countOf(e, heaps.Def.ID).Count; got != 0 {
		t.Fatalf("heaps not reset on first wave: %d", got)
	}
	e.HandleLine(t0, "The last stone heap shatters. The temple's remaining strength gathers into its final defender, a grinding mass of stone and packed earth that shakes the ground as it advances.")
	if got := countOf(e, heaps.Def.ID).Count; got != 24 {
		t.Fatalf("heap sync = %d", got)
	}
	e.HandleLine(t0, "The strength binding the stone temple's guardians falters unexpectedly. Their massive bodies buckle and crash into silent rubble. The Stone Ring has reset; you may attempt it again in 15 minutes.")
	for _, c := range cs {
		if got := countOf(e, c.Def.ID); got.Count != 0 || got.ClockStartedAt != 0 {
			t.Fatalf("%s not reset on failure: %+v", c.Def.Name, got)
		}
	}
}

func TestStoneRingUpgradesStalePatterns(t *testing.T) {
	s := openTestStore(t)
	e := NewEngine(s, nil)
	cs := installOne(t, e, "stonering")
	// Rewind the stored definitions to what pre-2026-10-25 builds installed,
	// plus a user edit that the upgrade must keep.
	old := cs[0].Def
	old.ResetPattern = ""
	old.Syncs = []SyncRule{{Pattern: `The Rock Monstrosity crumbles into mounds of rubble\.`, SetTo: 43}}
	old.Clock = &Clock{
		Label:        "Ring time left",
		DurationSecs: 5880,
		Marks:        []ClockMark{{Pattern: "The ground shudders as stone fortifications are summoned forth", IfIdle: true, Reset: true}},
		StopPatterns: []string{`(?i)(?:Peregrin Rockskull|an Encrusted Dirt Cloud) (?:has been slain by .+!|died\.)|slain (?:Peregrin Rockskull|an Encrusted Dirt Cloud)!`},
	}
	if _, err := e.Update(old); err != nil {
		t.Fatal(err)
	}
	edited := cs[1].Def
	edited.ResetPattern = "my own reset line"
	edited.Syncs = []SyncRule{{Pattern: "As the last of the stone heaps collapses", SetTo: 24}}
	if _, err := e.Update(edited); err != nil {
		t.Fatal(err)
	}

	e = NewEngine(s, nil)
	want := stoneRingPreset().Counters
	got := countOf(e, cs[0].Def.ID).Def
	if got.ResetPattern != want[0].ResetPattern || got.Syncs[0].Pattern != want[0].Syncs[0].Pattern ||
		got.Clock.Marks[0].Pattern != want[0].Clock.Marks[0].Pattern ||
		got.Clock.StopPatterns[0] != want[0].Clock.StopPatterns[0] {
		t.Fatalf("fortifications not upgraded: %+v", got)
	}
	got = countOf(e, cs[1].Def.ID).Def
	if got.ResetPattern != "my own reset line" || got.Syncs[0].Pattern != want[1].Syncs[0].Pattern {
		t.Fatalf("heaps upgrade wrong: %+v", got)
	}
	// Persisted: a third load sees the upgraded patterns with nothing stale.
	for _, c := range NewEngine(s, nil).State().Counters {
		if _, changed := upgradePresetDef(c.Def); changed {
			t.Fatalf("%s still stale after reload", c.Def.Name)
		}
	}
}

func TestStoneRingIdleOnlyStart(t *testing.T) {
	e := NewEngine(nil, nil)
	forts := installOne(t, e, "stonering")[0]
	const summon = "The ground shudders as stone fortifications are summoned forth."
	e.HandleLine(t0, summon)
	for i := 0; i < 12; i++ {
		e.Handle(killEv(t0, "A Stone Fortification", ""))
	}
	// A second trigger mob dies mid-ring: same line, must not reset or restart.
	later := t0.Add(5 * time.Minute)
	e.HandleLine(later, summon)
	got := countOf(e, forts.Def.ID)
	if got.Count != 12 || got.ClockStartedAt != t0.Unix() {
		t.Fatalf("mid-ring repeat changed state: %+v", got)
	}
	// The ring failed silently (no stop line); the next attempt still starts.
	next := t0.Add(2 * time.Hour)
	e.HandleLine(next, summon)
	got = countOf(e, forts.Def.ID)
	if got.Count != 0 || got.ClockStartedAt != next.Unix() {
		t.Fatalf("new attempt after silent failure: %+v", got)
	}
}

func TestCounterKillSource(t *testing.T) {
	type step struct {
		pipe    bool          // a Zeal death event (else a log kill line)
		reset   bool          // pipe disconnect
		advance time.Duration // wall time passing before this step
		mob     string
	}
	tests := []struct {
		name       string
		steps      []step
		want       int
		wantSource string
	}{
		{"log only", []step{{mob: "Knight of Decay"}, {mob: "Knight of Decay"}}, 2, KillSourceLog},
		{"pipe only", []step{{pipe: true, mob: "Knight of Decay"}, {pipe: true, mob: "Knight of Decay"}}, 2, KillSourceZeal},
		{"log line ignored once pipe deaths arrive", []step{
			{pipe: true, mob: "Knight of Decay"}, {mob: "Knight of Decay"}, {pipe: true, mob: "Knight of Decay"},
		}, 2, KillSourceZeal},
		{"first pipe death matching a just-counted log kill is skipped", []step{
			{mob: "Knight of Decay"}, {pipe: true, advance: time.Second, mob: "Knight of Decay"},
		}, 1, KillSourceZeal},
		{"pipe death long after a log kill counts", []step{
			{mob: "Knight of Decay"}, {pipe: true, advance: time.Minute, mob: "Knight of Decay"},
		}, 2, KillSourceZeal},
		{"pipe death of another mob counts", []step{
			{mob: "Priest of Decay"}, {pipe: true, mob: "Knight of Decay"},
		}, 2, KillSourceZeal},
		{"pipe disconnect falls back to log lines", []step{
			{pipe: true, mob: "Knight of Decay"}, {reset: true}, {mob: "Knight of Decay"},
		}, 2, KillSourceLog},
		{"unnamed pipe death still switches source", []step{
			{pipe: true, mob: ""}, {mob: "Knight of Decay"},
		}, 0, KillSourceZeal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine(nil, &fakeHub{})
			// now drives the engine's wall clock for the log/pipe dedup window.
			now := t0
			e.now = func() time.Time { return now }
			c := newTestCounter(t, e, CounterDef{
				Name: "Decay", Enabled: true, Mobs: []string{"knight of decay", "priest of decay"},
			})
			for _, s := range tc.steps {
				now = now.Add(s.advance)
				switch {
				case s.reset:
					e.HandlePipeReset()
				case s.pipe:
					e.HandlePipeDeath(now, s.mob)
				default:
					e.Handle(killEv(now, s.mob, "x"))
				}
			}
			if got := countOf(e, c.Def.ID).Count; got != tc.want {
				t.Errorf("count = %d, want %d", got, tc.want)
			}
			if got := e.State().KillSource; got != tc.wantSource {
				t.Errorf("kill source = %q, want %q", got, tc.wantSource)
			}
		})
	}
}

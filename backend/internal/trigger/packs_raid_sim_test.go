package trigger

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Raid pack simulations: every raid pack is installed (the way a raider
// would have them, next to Raid Alerts and Raid: Scripted Event Timers) into one real
// engine, and log lines are replayed through it, checking what actually
// fires rather than only that each regex matches.

type simStart struct {
	key      string
	target   string
	duration float64
	isCaster bool
}

type simSink struct{ starts []simStart }

func (s *simSink) StartExternal(name, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string, pinned bool, customGroup string, targetIsCaster bool, stack ...bool) {
	s.starts = append(s.starts, simStart{key: name, target: targetName, duration: durationSecs, isCaster: targetIsCaster})
}
func (s *simSink) StopExternal(name string, spellID int) {}

type raidSim struct {
	t      *testing.T
	e      *Engine
	sink   *simSink
	now    time.Time
	target string
	// byID maps installed trigger IDs to the trigger, for reading fires.
	byID map[string]*Trigger
	nst  int
}

func newRaidSim(t *testing.T, packs ...TriggerPack) *raidSim {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	s := openTestStore(t)
	sim := &raidSim{t: t, sink: &simSink{}, now: time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC)}
	sim.e = NewEngine(s, ws.NewHub(), sim.sink, nil)
	sim.e.SetTargetProvider(func() string { return sim.target })
	for _, p := range packs {
		if err := InstallPack(s, p); err != nil {
			t.Fatalf("InstallPack %s: %v", p.PackName, err)
		}
	}
	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sim.byID = map[string]*Trigger{}
	for _, tr := range all {
		sim.byID[tr.ID] = tr
	}
	sim.e.Reload()
	return sim
}

// gap moves the clock well past every binding window and cooldown, so
// scenarios don't leak into each other.
func (s *raidSim) gap() { s.now = s.now.Add(30 * time.Minute) }

func (s *raidSim) at(secs float64, line string) {
	s.e.Handle(s.now.Add(time.Duration(secs*float64(time.Second))), line)
}

// take returns the fires and timer starts since the last take.
func (s *raidSim) take() ([]TriggerFired, []simStart) {
	// History is capped, so it's emptied after each read rather than indexed.
	s.e.histMu.Lock()
	fires := s.e.history
	s.e.history = nil
	s.e.histMu.Unlock()
	starts := append([]simStart(nil), s.sink.starts[s.nst:]...)
	s.nst = len(s.sink.starts)
	return fires, starts
}

func (s *raidSim) isRaidTimer(f TriggerFired) bool {
	tr := s.byID[f.TriggerID]
	return tr != nil && tr.TimerType == TimerTypeDetrimental && len(tr.BossCasters) > 0
}

// simBars folds timer starts into the bars the timer engine would show: it
// keys rows by timer name and target, so the same-named timer from two packs
// (Tallon Zek in PoP Tier 3 and Plane of Time) is one bar. conflict is set
// when two starts for one bar disagree on its length.
func simBars(starts []simStart) (bars []simStart, conflict string) {
	idx := map[string]int{}
	for _, st := range starts {
		k := st.key + "\x00" + st.target
		if i, ok := idx[k]; ok {
			if bars[i].duration != st.duration {
				conflict = fmt.Sprintf("bar %q on %s started for both %gs and %gs", st.key, st.target, bars[i].duration, st.duration)
			}
			continue
		}
		idx[k] = len(bars)
		bars = append(bars, st)
	}
	return bars, conflict
}

func allRaidSimPacks() []TriggerPack {
	packs := []TriggerPack{RaidAlertsPack(), RaidEventTimersPack()}
	return append(packs, raidPacks()...)
}

// simLines are the land lines a spell can write: on another player, on you,
// and your resist.
func simLines(s raidSpell) []string {
	var out []string
	if s.OnOther != "" {
		out = append(out, "Larcen"+s.OnOther)
	}
	if s.OnYou != "" {
		out = append(out, s.OnYou)
	}
	return append(out, "You resist the "+s.Name+" spell!")
}

// raidSimCase is one boss spell with a timer.
type raidSimCase struct {
	pack  string
	boss  string
	spell raidSpell
	trig  Trigger
	dur   float64
}

// raidSimCases pairs every timed boss spell with the trigger that times it.
// Spells whose every land line is shared with another spell in the pack
// (so no timer can be built) are returned in skipped.
func raidSimCases(t *testing.T) (cases []raidSimCase, skipped []string) {
	for _, d := range raidPackDefs() {
		p := d.build()
		for _, b := range d.Bosses {
			for _, s := range b.Spells {
				if s.Buff || s.Proc || s.Scripted || s.Recast < raidMinTimerSecs {
					continue
				}
				var found *Trigger
				for i := range p.Triggers {
					tr := &p.Triggers[i]
					if !bossCasterListed(tr, b.Name) {
						continue
					}
					re := compileSimPattern(t, tr.Pattern)
					for _, l := range simLines(s) {
						if re(l) && (strings.Contains(tr.Name, s.Name) || tr.SpellID == s.ID) {
							found = tr
						}
					}
				}
				if found == nil {
					skipped = append(skipped, d.Name+" / "+b.Name+" / "+s.Name)
					continue
				}
				cases = append(cases, raidSimCase{
					pack: d.Name, boss: b.Name, spell: s, trig: *found,
					dur: simDuration(found, b.Name),
				})
			}
		}
	}
	return cases, skipped
}

// simDuration is the bar length expected for caster on t: its own
// override, else the trigger's duration.
func simDuration(t *Trigger, caster string) float64 {
	if d := casterDuration(t, caster); d > 0 {
		return d
	}
	return t.TimerDurationSecs
}

func bossCasterListed(t *Trigger, name string) bool {
	for _, c := range t.BossCasters {
		if c.Name == name {
			return true
		}
	}
	return false
}

func compileSimPattern(t *testing.T, pattern string) func(string) bool {
	t.Helper()
	p, err := compilePattern(pattern)
	if err != nil {
		t.Fatalf("compile %q: %v", pattern, err)
	}
	return p.MatchString
}

// TestRaidSim_BossCastsStartOneTimer: for every timed boss spell, the boss
// begins casting, the spell lands on three raiders at once (AE) and on you,
// and exactly one bar starts, on that boss, for that boss's recast.
func TestRaidSim_BossCastsStartOneTimer(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	cases, skipped := raidSimCases(t)
	if len(cases) < 100 {
		t.Fatalf("only %d sim cases; the case builder lost spells", len(cases))
	}
	for _, s := range skipped {
		t.Logf("no timer (every land line shared within its pack): %s", s)
	}
	for _, c := range cases {
		match := compileSimPattern(t, c.trig.Pattern)
		for _, line := range simLines(c.spell) {
			if !match(line) {
				continue // a line shared with another spell; not used
			}
			sim.gap()
			sim.at(0, c.boss+" begins to cast a spell.")
			sim.at(3, line)
			// AE: the same line for other raiders in the same second.
			if strings.HasPrefix(line, "Larcen") {
				sim.at(3, "Bob"+c.spell.OnOther)
				sim.at(3, "Sue"+c.spell.OnOther)
			}
			fires, raw := sim.take()
			starts, conflict := simBars(raw)
			if conflict != "" {
				t.Errorf("%s / %s: %s", c.pack, c.boss, conflict)
			}
			var timers []TriggerFired
			seen := map[string]bool{}
			for _, f := range fires {
				if sim.isRaidTimer(f) && !seen[f.TriggerName] {
					seen[f.TriggerName] = true
					timers = append(timers, f)
				}
			}
			where := c.pack + " / " + c.boss + " / " + c.spell.Name + " on " + line
			if len(timers) != 1 || len(starts) != 1 {
				names := []string{}
				for _, f := range timers {
					names = append(names, f.TriggerName)
				}
				t.Errorf("%s: %d raid timer fires %v, %d bars; want 1", where, len(timers), names, len(starts))
				continue
			}
			if timers[0].TriggerName != c.trig.Name {
				t.Errorf("%s: fired %q, want %q", where, timers[0].TriggerName, c.trig.Name)
			}
			st := starts[0]
			if st.target != c.boss || st.duration != c.dur || !st.isCaster {
				t.Errorf("%s: bar %+v; want on %s for %gs", where, st, c.boss, c.dur)
			}
		}
	}
}

// TestRaidSim_NoCastSeen: a land line with no "begins to cast" before it
// starts nothing unless you have one of the spell's bosses targeted, and
// then the bar goes on that boss.
func TestRaidSim_NoCastSeen(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	cases, _ := raidSimCases(t)
	for _, c := range cases {
		match := compileSimPattern(t, c.trig.Pattern)
		var line string
		for _, l := range simLines(c.spell) {
			if match(l) {
				line = l
				break
			}
		}
		where := c.pack + " / " + c.boss + " / " + c.spell.Name

		sim.gap()
		sim.target = ""
		sim.at(0, line)
		if _, starts := sim.take(); len(starts) != 0 {
			t.Errorf("%s: %d bars with no cast seen and nothing targeted; want 0", where, len(starts))
		}

		sim.gap()
		sim.target = "a gnoll"
		sim.at(0, line)
		if _, starts := sim.take(); len(starts) != 0 {
			t.Errorf("%s: %d bars with an unrelated target; want 0", where, len(starts))
		}

		sim.gap()
		sim.target = c.boss
		sim.at(0, line)
		_, raw := sim.take()
		starts, conflict := simBars(raw)
		if conflict != "" {
			t.Errorf("%s: %s", where, conflict)
		}
		if len(starts) != 1 || starts[0].target != c.boss || starts[0].duration != c.dur {
			t.Errorf("%s: with the boss targeted got bars %+v; want one on %s for %gs", where, starts, c.boss, c.dur)
		}
	}
	sim.target = ""
}

// TestRaidSim_CureAlerts: a curable boss debuff landing on you shows the
// cure alert once, with the cure type.
func TestRaidSim_CureAlerts(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	n := 0
	for _, d := range raidPackDefs() {
		for _, b := range d.Bosses {
			for _, s := range b.Spells {
				if s.Counter == "" {
					continue
				}
				n++
				sim.gap()
				sim.at(0, s.OnYou)
				fires, _ := sim.take()
				var cures []TriggerFired
				for _, f := range fires {
					if strings.HasSuffix(f.TriggerName, " on you") && sim.byID[f.TriggerID].PackName == d.Name {
						cures = append(cures, f)
					}
				}
				where := d.Name + " / " + b.Name + " / " + s.Name
				if len(cures) != 1 {
					t.Errorf("%s: %d cure alerts on %q; want 1", where, len(cures), s.OnYou)
					continue
				}
				text := cures[0].Actions[0].Text
				if !strings.Contains(text, strings.ToUpper(s.Name)) || !strings.HasSuffix(text, "cure "+strings.ToLower(s.Counter)) {
					t.Errorf("%s: cure alert text %q", where, text)
				}
			}
		}
	}
	if n < 20 {
		t.Fatalf("only %d curable spells simulated", n)
	}
}

// simTellNames are the ways a raider types a boss name in /tell: as the
// log writes it, with underscores, and shortened to each word's first three
// letters (the last word's first letter), e.g. "gry_the_cry_g".
func simTellNames(name string) (full []string, short string) {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '_' || r == '-' })
	full = []string{name, strings.Join(words, "_")}
	if len(words) == 1 {
		return full, ""
	}
	parts := make([]string, len(words))
	for i, w := range words {
		w = strings.NewReplacer(",", "", "`", "", "'", "").Replace(w)
		keep := 3
		if i == len(words)-1 {
			keep = 1
		}
		if len(w) > keep {
			w = w[:keep]
		}
		parts[i] = strings.ToLower(w)
	}
	return full, strings.Join(parts, "_")
}

// TestRaidSim_WhoCards: /tell <boss> shows that boss's card and no other,
// for the full name, its aliases and the shortened form.
func TestRaidSim_WhoCards(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	whoFires := func(line string) []string {
		sim.gap()
		sim.at(0, line)
		fires, starts := sim.take()
		if len(starts) != 0 {
			t.Errorf("%q started %d bars", line, len(starts))
		}
		var names []string
		for _, f := range fires {
			if strings.HasSuffix(f.TriggerName, " — Who") {
				names = append(names, f.TriggerName)
			}
		}
		return names
	}
	ambiguous := 0
	multi := raidMultiPackBosses()
	cardsFor := map[string]int{}
	for _, d := range raidPackDefs() {
		for _, b := range d.Bosses {
			if !b.NoWho {
				cardsFor[b.Name]++
			}
		}
	}
	for _, d := range raidPackDefs() {
		for _, b := range d.Bosses {
			if b.NoWho {
				continue
			}
			want := b.Name + " — Who"
			full, short := simTellNames(b.Name)
			for _, a := range b.Aliases {
				full = append(full, a)
			}
			for _, n := range full {
				got := whoFires(n + " is not online at this time.")
				// An alias ("CT") is typed for one pack's boss only.
				wantN := cardsFor[b.Name]
				if containsString(b.Aliases, n) {
					wantN = 1
				}
				ok := len(got) == wantN
				for _, g := range got {
					ok = ok && g == want
				}
				if !ok {
					t.Errorf("%s: /tell %q showed %v; want %d× %q", d.Name, n, got, wantN, want)
				}
			}
			if multi[b.Name] {
				card := raidWhoText(b, true)
				if !strings.HasPrefix(card, b.Name+" ("+b.Zone+") · ") {
					t.Errorf("%s: %s is in several packs but its card %q doesn't name the zone", d.Name, b.Name, card)
				}
			}
			if short == "" {
				continue
			}
			got := whoFires(short + " is not online at this time.")
			if !containsString(got, want) {
				t.Errorf("%s: /tell %q didn't show %q (got %v)", d.Name, short, want, got)
			} else if len(got) > cardsFor[b.Name] {
				ambiguous++
				t.Logf("shortened /tell %q shows %d cards: %v", short, len(got), got)
			}
		}
	}
	if ambiguous > 0 {
		t.Logf("%d shortened names show more than one card", ambiguous)
	}
}

// TestRaidSim_Events: every event sample line fires its trigger once, and
// starts its countdown when it has one.
func TestRaidSim_Events(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	events := map[string]Trigger{}
	for _, p := range raidPacks() {
		for _, tr := range p.Triggers {
			if _, ok := raidEventSamples[tr.Name]; ok && len(tr.BossCasters) == 0 && !strings.HasSuffix(tr.Name, " — Who") {
				events[tr.Name] = tr
			}
		}
	}
	for name, tr := range events {
		for _, line := range raidEventSamples[name] {
			sim.gap()
			sim.at(0, line)
			fires, starts := sim.take()
			n := 0
			var others []string
			for _, f := range fires {
				if f.TriggerName == name {
					n++
				} else if sim.byID[f.TriggerID].PackName != "" && strings.HasPrefix(sim.byID[f.TriggerID].PackName, "Raid") {
					others = append(others, f.TriggerName)
				}
			}
			if n != 1 {
				t.Errorf("%s: fired %d times on %q; want 1", name, n, line)
			}
			if len(others) > 0 {
				t.Logf("%s: %q also fired %v", name, line, others)
			}
			if tr.TimerType != TimerTypeNone && tr.TimerType != "" && tr.TimerDurationSecs > 0 {
				// The main line starts the trigger's duration; an extra
				// pattern (a phase warning, an aggro line) its own.
				durs := []float64{tr.TimerDurationSecs}
				for _, x := range tr.ExtraPatterns {
					if x.TimerDurationSecs > 0 {
						durs = append(durs, x.TimerDurationSecs)
					}
				}
				ok := false
				for _, st := range starts {
					if st.key == name {
						for _, d := range durs {
							ok = ok || st.duration == d
						}
					}
				}
				if !ok {
					t.Errorf("%s: no countdown (%v s) on %q (bars %+v)", name, durs, line, starts)
				}
			}
		}
	}
}

// TestRaidSim_Noise: ordinary raid log traffic fires no raid trigger.
func TestRaidSim_Noise(t *testing.T) {
	sim := newRaidSim(t, allRaidSimPacks()...)
	sim.target = "Vulak`Aerr"
	noise := []string{
		"Vulak`Aerr hits YOU for 1100 points of damage.",
		"Vulak`Aerr tries to hit YOU, but misses!",
		"You slash Vulak`Aerr for 150 points of damage.",
		"Larcen hits Vulak`Aerr for 98 points of damage.",
		"Vulak`Aerr begins to cast a spell.",
		"Larcen begins to cast a spell.",
		"You begin casting Complete Healing.",
		"You feel much better.",
		"Larcen has been slain by Vulak`Aerr!",
		"Vulak`Aerr has been slain by Larcen!",
		"You have been slain by Vulak`Aerr!",
		"Larcen tells the raid, 'CH on Bob 3'",
		"Bob tells the guild, 'ready check'",
		"You have entered Temple of Veeshan.",
		"Vulak`Aerr is enraged!",
		"Vulak`Aerr has become ENRAGED.",
		"Your target resisted the Malosini spell.",
		"Your Malosini spell has worn off.",
		"Larcen is no longer enraged.",
		"Bob is not online at this time.",
	}
	for _, l := range noise {
		sim.gap()
		sim.at(0, l)
		fires, starts := sim.take()
		var raid []string
		for _, f := range fires {
			if tr := sim.byID[f.TriggerID]; tr != nil && strings.HasPrefix(tr.PackName, "Raid: ") {
				raid = append(raid, f.TriggerName)
			}
		}
		if len(raid) > 0 || len(starts) > 0 {
			t.Errorf("noise %q fired raid triggers %v, %d bars", l, raid, len(starts))
		}
	}
	sim.target = ""
}

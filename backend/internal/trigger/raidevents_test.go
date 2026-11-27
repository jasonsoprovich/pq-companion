package trigger

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// recordingSink records every timer start/stop/repeat call by name.
type recordingSink struct {
	starts  map[string]float64 // name → last duration
	stops   map[string]int
	repeats map[string]int // name → max repeats requested
}

func newRecordingSink() *recordingSink {
	return &recordingSink{starts: map[string]float64{}, stops: map[string]int{}, repeats: map[string]int{}}
}

func (s *recordingSink) StartExternal(name, _ string, durationSecs, _ float64, _ time.Time, _ json.RawMessage, _ int, _, _ string, _ bool, _ string, _ bool, _ ...bool) {
	s.starts[name] = durationSecs
}
func (s *recordingSink) StopExternal(name string, _ int) { s.stops[name]++ }
func (s *recordingSink) SetExternalRepeat(name, _ string, max int) {
	s.repeats[name] = max
}

func raidEventsEngine(t *testing.T) (*Engine, *recordingSink) {
	t.Helper()
	store := openTestStore(t)
	if err := InstallPack(store, RaidEventTimersPack()); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}
	sink := newRecordingSink()
	e := NewEngine(store, ws.NewHub(), sink, nil)
	e.Reload()
	return e, sink
}

func TestRaidEventTimersPatternsCompile(t *testing.T) {
	p := RaidEventTimersPack()
	if len(p.Triggers) == 0 {
		t.Fatal("empty pack")
	}
	seen := map[string]bool{}
	for _, tr := range p.Triggers {
		if seen[tr.Name] {
			t.Errorf("duplicate trigger name %q (timer keys would collide)", tr.Name)
		}
		seen[tr.Name] = true
		for _, pat := range []string{tr.Pattern, tr.WornOffPattern} {
			if pat == "" {
				continue
			}
			if _, err := regexp.Compile(pat); err != nil {
				t.Errorf("%s: bad pattern %q: %v", tr.Name, pat, err)
			}
		}
		for _, x := range tr.ExtraPatterns {
			if _, err := regexp.Compile(x.Pattern); err != nil {
				t.Errorf("%s: bad extra pattern %q: %v", tr.Name, x.Pattern, err)
			}
		}
	}
}

func TestRaidEventTimersFire(t *testing.T) {
	e, sink := raidEventsEngine(t)
	now := time.Now()

	// Plane of Time: hourly and thirty-minute announcements pin the clock.
	e.Handle(now, "In the distance, an hourglass appears, the grains of sand falling methodically into place.  As quickly as the image was formed, it dissipates.  You have three hours left.")
	if got := sink.starts["Plane of Time: instance time left"]; got != 3*3600 {
		t.Errorf("PoTime clock after 'three hours' = %v", got)
	}
	e.Handle(now, "In the distance, an hourglass appears, the grains of sand falling methodically into place.  As quickly as the image was formed, it dissipates.  You have thirty minutes left.")
	if got := sink.starts["Plane of Time: instance time left"]; got != 1800 {
		t.Errorf("PoTime clock after 'thirty minutes' = %v", got)
	}

	// Coirnav's final phase starts a repeating 49s reinforcement timer.
	e.Handle(now, `Water surges, ice fractures and vapor coils through the reef as Coirnav summons the remaining defenders of the depths.`)
	if sink.starts["Coirnav: reinforcements"] != 49 || sink.repeats["Coirnav: reinforcements"] != 30 {
		t.Errorf("Coirnav reinforcements: start %v repeat %v", sink.starts["Coirnav: reinforcements"], sink.repeats["Coirnav: reinforcements"])
	}

	// Paffa: the opening line starts the wave cycle and the Overlord timer.
	e.Handle(now, "A deep voice bellows from just ahead saying, 'Come fools see if you can match the power of Banord and his puslings.  Puslings come your master has need of you!'")
	if sink.starts["Paffa: next pusling wave"] != 234 || sink.starts["Paffa: Overlord Banord Paffa"] != 702 {
		t.Errorf("Paffa starts = %v", sink.starts)
	}
	if _, ok := sink.repeats["Paffa: next pusling wave"]; !ok {
		t.Error("Paffa waves not marked repeating")
	}
	// A combat line naming the Overlord must not clear his timers.
	e.Handle(now, "Overlord Banord Paffa hits YOU for 400 points of damage.")
	if sink.stops["Paffa: next pusling wave"] != 0 {
		t.Error("combat line cleared the Paffa wave timer")
	}
	e.Handle(now, "Overlord Banord Paffa has been slain by Osui!")
	if sink.stops["Paffa: next pusling wave"] == 0 {
		t.Error("Overlord death did not clear the Paffa wave timer")
	}

	// Emperor room curse: the second room line re-pins the same timer.
	e.Handle(now, "You notice a splatter of oddly colored blood on the floor")
	if got := sink.starts["Emperor room: Curse of Ssraeshza"]; got != 1185 {
		t.Errorf("curse after splatter = %v", got)
	}
	e.Handle(now, "Something tells you that this room isn't safe anymore...")
	if got := sink.starts["Emperor room: Curse of Ssraeshza"]; got != 312 {
		t.Errorf("curse after warning = %v", got)
	}

	// Ring War: one shout starts every wave timer; an early charge clears them.
	e.Handle(now, "Seneschal Aldikar shouts, 'Good citizens of Thurgadin, hear me! Our city, our people, our very lives are in danger this day.'")
	if sink.starts["Ring War: wave 01"] != 125 || sink.starts["Ring War: wave 19"] != 5155 ||
		sink.starts["Ring War: Narandi the Wretched"] != 5475 {
		t.Errorf("Ring War schedule = %v / %v / %v", sink.starts["Ring War: wave 01"],
			sink.starts["Ring War: wave 19"], sink.starts["Ring War: Narandi the Wretched"])
	}
	e.Handle(now, "Zrelik the Scout shouts, 'Osui has spotted tha' next wave! Charge!'")
	if sink.stops["Ring War: wave 07"] == 0 || sink.stops["Ring War: Narandi the Wretched"] == 0 {
		t.Error("early charge did not clear the Ring War schedule")
	}

	// Hollowshade: any attacker's preparation emote starts the countdown.
	e.Handle(now, "Frenzied howling fills the air, the sound taking on every aspect of an angry wail. The sonic wolves are mounting an offensive.")
	if sink.starts["Hollowshade: invasion begins"] != 720 {
		t.Errorf("Hollowshade = %v", sink.starts["Hollowshade: invasion begins"])
	}
}

// TestMigrateRaidEventTimersRename installs the pack, rewinds it to its old
// name as a v0.34 database would hold it, and checks the migration moves
// every trace of it to the new name with no pending pack update.
func TestMigrateRaidEventTimersRename(t *testing.T) {
	store := openTestStore(t)
	if err := InstallPack(store, RaidEventTimersPack()); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}
	old, cur := legacyRaidEventTimersPackName, RaidEventTimersPackName
	for _, q := range []string{
		`UPDATE triggers SET pack_name = ?, source_pack = ? WHERE source_pack = ?`,
		`UPDATE trigger_categories SET name = ? WHERE parent_id = '' AND name = ?`,
		`UPDATE pack_baselines SET source_pack = ?, definition = json_set(definition, '$.pack_name', ?) WHERE source_pack = ?`,
		`DELETE FROM pack_default_updates WHERE key LIKE 'RaidEventTimers:RenameScripted:%'`,
	} {
		var err error
		switch {
		case strings.HasPrefix(q, "UPDATE triggers"):
			_, err = store.db.Exec(q, old, old, cur)
		case strings.HasPrefix(q, "UPDATE pack_baselines"):
			_, err = store.db.Exec(q, old, old, cur)
		case strings.HasPrefix(q, "DELETE"):
			_, err = store.db.Exec(q)
		default:
			_, err = store.db.Exec(q, old, cur)
		}
		if err != nil {
			t.Fatalf("rewind %q: %v", q, err)
		}
	}

	if err := store.MigrateRaidEventTimersRename(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.migrateRaidEventTimersBaselineCategory(); err != nil {
		t.Fatalf("migrate baselines: %v", err)
	}

	installed, err := store.InstalledPackNames()
	if err != nil {
		t.Fatal(err)
	}
	if !installed[cur] || installed[old] {
		t.Errorf("installed packs = %v, want %q only", installed, cur)
	}
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM triggers WHERE pack_name = ? OR source_pack = ?`, old, old).Scan(&n); err != nil || n != 0 {
		t.Errorf("triggers still on old name: %d (%v)", n, err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM trigger_categories WHERE parent_id = '' AND name = ?`, cur).Scan(&n); err != nil || n != 1 {
		t.Errorf("new category rows = %d (%v), want 1", n, err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM triggers t JOIN trigger_categories c ON c.id = t.category_id WHERE c.name = ?`, cur).Scan(&n); err != nil || n != len(RaidEventTimersPack().Triggers) {
		t.Errorf("triggers linked to new category = %d (%v), want %d", n, err, len(RaidEventTimersPack().Triggers))
	}
	diff, err := ComputePackDiff(store, RaidEventTimersPack())
	if err != nil {
		t.Fatal(err)
	}
	if diff.HasUpdates() {
		t.Errorf("pack shows updates after rename: %+v", diff)
	}
}

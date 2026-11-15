package raidtrack

import (
	"testing"
	"time"
)

func TestRosterJoinLeave(t *testing.T) {
	h := newHarness(t)
	base := []Member{{Name: "Osui", Class: 2, Level: 60}, {Name: "Larcen", Class: 9, Level: 60}}
	step := func(members []Member, d time.Duration) {
		for end := h.now.Add(d); h.now.Before(end); h.now = h.now.Add(time.Second) {
			h.rec.SetRaid("Osui", "poknowledge", members)
			h.rec.tick()
		}
	}
	start := h.now
	step(base, 10*time.Second)
	// Bertox joins, then Larcen drops for 5 s (a missed frame, not a leave),
	// then leaves for good.
	step(append(base, Member{Name: "Bertox", Class: 11, Level: 59}), 10*time.Second)
	step([]Member{base[0], {Name: "Bertox"}}, 5*time.Second)
	step(append(base, Member{Name: "Bertox"}), 5*time.Second)
	leftAt := h.now
	step([]Member{base[0], {Name: "Bertox"}}, 30*time.Second)

	got, err := h.store.RosterBetween("Osui", 0, h.now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ kind, name string }
	want := []row{{RosterPresent, "Osui"}, {RosterPresent, "Larcen"}, {RosterJoin, "Bertox"}, {RosterLeave, "Larcen"}}
	if len(got) != len(want) {
		t.Fatalf("roster = %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Name != w.name {
			t.Errorf("roster[%d] = %s %s, want %s %s", i, got[i].Kind, got[i].Name, w.kind, w.name)
		}
	}
	if got[0].T != start.UnixMilli() {
		t.Errorf("present at %d, want %d", got[0].T, start.UnixMilli())
	}
	// A leave is stamped when the raider was last seen.
	if l := got[3].T; l != leftAt.Add(-time.Second).UnixMilli() {
		t.Errorf("leave at %d, want %d", l, leftAt.Add(-time.Second).UnixMilli())
	}
}

func TestRosterResetAfterSilence(t *testing.T) {
	h := newHarness(t)
	h.rec.SetRaid("Osui", "pok", []Member{{Name: "Osui"}, {Name: "Larcen"}})
	h.rec.tick()
	// No roster for longer than rosterReset (camped): nobody is marked as
	// leaving, and the next roster is a fresh "present".
	h.now = h.now.Add(rosterReset + time.Minute)
	h.rec.tick()
	h.rec.SetRaid("Osui", "pok", []Member{{Name: "Osui"}})
	h.rec.tick()
	got, _ := h.store.RosterBetween("Osui", 0, h.now.UnixMilli())
	kinds := ""
	for _, ev := range got {
		kinds += ev.Kind + ":" + ev.Name + " "
	}
	if kinds != "present:Osui present:Larcen present:Osui " {
		t.Errorf("roster = %q", kinds)
	}
}

func TestGroupNights(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 4, h, m, 0, 0, time.Local) }
	ss := []Session{
		{ID: 1, Character: "Osui", Zone: "sebilis", StartedAt: at(19, 0), EndedAt: at(20, 0), PeakMembers: 30, Kills: 5},
		// Ported out for 20 minutes, then back in another zone: same night.
		{ID: 2, Character: "Osui", Zone: "trakanon", StartedAt: at(20, 20), EndedAt: at(21, 0), PeakMembers: 40, Kills: 1},
		// A boxed character's recording doesn't join Osui's night.
		{ID: 3, Character: "Larcen", Zone: "trakanon", StartedAt: at(20, 25), EndedAt: at(20, 50)},
		// Two hours later: a new night.
		{ID: 4, Character: "Osui", Zone: "veeshan", StartedAt: at(23, 0), EndedAt: at(23, 30)},
	}
	nights := GroupNights(ss)
	if len(nights) != 3 {
		t.Fatalf("got %d nights, want 3", len(nights))
	}
	if nights[0].ID != 4 || nights[1].ID != 3 || nights[2].ID != 1 {
		t.Errorf("order = %d %d %d, want 4 3 1", nights[0].ID, nights[1].ID, nights[2].ID)
	}
	n := nights[2]
	if len(n.Sessions) != 2 || n.PeakMembers != 40 || n.Kills != 6 || !n.EndedAt.Equal(at(21, 0)) {
		t.Errorf("night = %+v", n)
	}
	// The roster window stops short of the next night of the same character.
	if n.rosterTo >= at(23, 0).UnixMilli() || n.rosterTo != at(21, 45).UnixMilli() {
		t.Errorf("rosterTo = %v", time.UnixMilli(n.rosterTo))
	}
	if nights[0].rosterFrom != at(22, 0).UnixMilli() {
		t.Errorf("next night's rosterFrom = %v", time.UnixMilli(nights[0].rosterFrom))
	}
}

func TestSummarize(t *testing.T) {
	t0 := time.Date(2026, 11, 4, 20, 0, 0, 0, time.Local).UnixMilli()
	sec := int64(1000)
	rec := Recording{
		Members: []MemberInfo{{Index: 0, Name: "Osui", Class: 2, Level: 60}, {Index: 1, Name: "Larcen", Class: 9, Level: 60}},
	}
	// Osui stands still for 5 minutes, then walks. Larcen arrives a minute
	// in, leaves for a minute at 3:00 and is gone 30 s before the end.
	for s := int64(0); s <= 600; s++ {
		f := Frame{T: t0 + s*sec}
		x := 0
		if s > 300 {
			x = int(s-300) * 20
		}
		f.M = append(f.M, []int{0, x, 0, 0, 0, 1, 100})
		if s >= 60 && (s < 180 || s >= 240) && s <= 570 {
			f.M = append(f.M, []int{1, 50, 0, 0, 0, 1, 100})
		}
		rec.Frames = append(rec.Frames, f)
	}
	rec.Events = []Event{{T: t0 + 400*sec, Kind: EventDeath, Name: "Larcen"}}
	roster := []RosterEvent{
		{T: t0 - 600*sec, Kind: RosterPresent, Name: "Osui"},
		{T: t0 + 30*sec, Kind: RosterJoin, Name: "Larcen", Class: 9, Level: 60},
		{T: t0 + 700*sec, Kind: RosterLeave, Name: "Bertox", Class: 1, Level: 50},
	}
	sums, events := summarize([]Recording{rec}, roster)
	by := map[string]RaiderSummary{}
	for _, s := range sums {
		by[s.Name] = s
	}
	osui, larcen, bertox := by["Osui"], by["Larcen"], by["Bertox"]

	if osui.JoinedAt != 0 || osui.FirstSeen != t0-600*sec || osui.InZoneMs != 600*sec {
		t.Errorf("Osui = %+v", osui)
	}
	if osui.StillMs != 300*sec || osui.LongestStillMs != 300*sec {
		t.Errorf("Osui still = %d/%d, want 300 s", osui.StillMs, osui.LongestStillMs)
	}
	if larcen.JoinedAt != t0+30*sec || larcen.Away != 2 || larcen.Deaths != 1 {
		t.Errorf("Larcen = %+v", larcen)
	}
	// 60→179 and 240→570 in zone: 119 s + 330 s.
	if larcen.InZoneMs != 449*sec {
		t.Errorf("Larcen in zone = %d s, want 449", larcen.InZoneMs/sec)
	}
	// Only seen in the roster, never in the observer's zone.
	if bertox.LeftAt != t0+700*sec || bertox.InZoneMs != 0 || bertox.Class != 1 {
		t.Errorf("Bertox = %+v", bertox)
	}

	var kinds []string
	for _, ev := range events {
		kinds = append(kinds, ev.Kind+":"+ev.Name)
	}
	want := []string{
		"raid_join:Larcen", "zone_in:Larcen", "zone_out:Larcen", "zone_in:Larcen",
		"zone_out:Larcen", "raid_leave:Bertox",
	}
	if len(kinds) != len(want) {
		t.Fatalf("events = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("events[%d] = %s, want %s", i, kinds[i], want[i])
		}
	}
}

func TestGetNightEndToEnd(t *testing.T) {
	h := newHarness(t)
	h.run("sebilis", 2*time.Minute)
	h.run("trakanon", 2*time.Minute)
	nights, err := h.store.ListNights()
	if err != nil {
		t.Fatal(err)
	}
	if len(nights) != 1 || len(nights[0].Sessions) != 2 {
		t.Fatalf("nights = %+v", nights)
	}
	data, ok, err := h.store.GetNight(nights[0].ID)
	if err != nil || !ok {
		t.Fatalf("GetNight: ok=%v err=%v", ok, err)
	}
	if len(data.Recordings) != 2 || !data.RosterTracked {
		t.Errorf("recordings=%d tracked=%v", len(data.Recordings), data.RosterTracked)
	}
	// "Away" is in the roster but never in zone.
	names := map[string]bool{}
	for _, s := range data.Summary {
		names[s.Name] = true
	}
	if !names["Away"] || !names["Larcen"] || len(data.Summary) != 3 {
		t.Errorf("summary = %+v", data.Summary)
	}
}

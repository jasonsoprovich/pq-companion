package raidtrack

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type harness struct {
	t     *testing.T
	store *Store
	rec   *Recorder
	now   time.Time
	cfg   config.RaidReplaySettings
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := &harness{t: t, store: st, now: time.Date(2026, 11, 4, 20, 0, 0, 0, time.Local)}
	h.rec = NewRecorder(st, func() config.RaidReplaySettings { return h.cfg })
	h.rec.now = func() time.Time { return h.now }
	return h
}

func raid(x float64) []Member {
	return []Member{
		{Name: "Osui", Class: 2, Level: 60, Group: 1, HasLoc: true, X: x, Y: 10, Z: -5, Heading: 128, HPPct: -1},
		{Name: "Larcen", Class: 9, Level: 60, Group: 1, HasLoc: true, X: x + 20, Y: 15, Z: -5, HPPct: 80},
		{Name: "Away", Class: 1, Level: 60, Group: 2},
	}
}

// run feeds the raid roster and ticks once a second for d.
func (h *harness) run(zone string, d time.Duration) {
	for end := h.now.Add(d); h.now.Before(end); h.now = h.now.Add(time.Second) {
		h.rec.SetRaid("Osui", zone, raid(float64(h.now.Second())))
		h.rec.tick()
	}
}

func (h *harness) sessions() []Session {
	h.t.Helper()
	ss, err := h.store.ListSessions()
	if err != nil {
		h.t.Fatal(err)
	}
	return ss
}

func TestRecorderSessionAndPlayback(t *testing.T) {
	h := newHarness(t)
	h.run("templeveeshan", 3*time.Minute)

	h.rec.Handle(logparser.LogEvent{Timestamp: h.now, Data: logparser.KillData{Killer: "You", Target: "Lord Vyemm"}})
	h.rec.Handle(logparser.LogEvent{Timestamp: h.now, Data: logparser.KillData{Killer: "Lord Yelinak", Target: "Larcen"}})
	h.rec.Handle(logparser.LogEvent{Timestamp: h.now, Data: logparser.DeathData{SlainBy: "Lord Yelinak"}})

	// The roster stops: the session ends after the gap.
	h.now = h.now.Add(sessionGap + 2*time.Second)
	h.rec.tick()
	h.rec.mu.Lock()
	if h.rec.sess != nil {
		t.Fatal("session still open after the gap")
	}
	h.rec.mu.Unlock()

	ss := h.sessions()
	if len(ss) != 1 {
		t.Fatalf("sessions = %d, want 1", len(ss))
	}
	s := ss[0]
	// 180 s at one frame per 3 s.
	if s.Zone != "templeveeshan" || s.Frames != 60 || s.PeakMembers != 2 || s.Kills != 1 || s.Deaths != 2 {
		t.Fatalf("session = %+v", s)
	}

	rec, ok, err := h.store.GetRecording(s.ID)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if len(rec.Frames) != 60 || len(rec.Members) != 3 {
		t.Fatalf("frames %d members %d", len(rec.Frames), len(rec.Members))
	}
	for i := 1; i < len(rec.Frames); i++ {
		if d := rec.Frames[i].T - rec.Frames[i-1].T; d != frameInterval.Milliseconds() {
			t.Fatalf("frame %d spacing %d ms", i, d)
		}
	}
	row := rec.Frames[0].M[1]
	if row[0] != 1 || row[2] != 15 || row[5] != 1 || row[6] != 80 {
		t.Fatalf("frame row = %v", row)
	}
	if rec.Members[2].Name != "Away" {
		t.Fatalf("out-of-zone raider not in roster: %+v", rec.Members)
	}
	if rec.Events[1].Kind != EventDeath || rec.Events[1].Name != "Larcen" ||
		rec.Events[2].Name != "Osui" || rec.Events[0].Other != "Osui" {
		t.Fatalf("events = %+v", rec.Events)
	}
}

// Boxing: the pipe is Osui's client but the active log is Larcen's, so the
// log's "You" is Larcen.
func TestRecorderLogCharacter(t *testing.T) {
	h := newHarness(t)
	h.rec.SetLogCharacter(func() string { return "Larcen" })
	h.run("templeveeshan", 3*time.Minute)
	h.rec.Handle(logparser.LogEvent{Timestamp: h.now, Data: logparser.DeathData{SlainBy: "Lord Yelinak"}})
	h.rec.Handle(logparser.LogEvent{Timestamp: h.now, Data: logparser.KillData{Killer: "You", Target: "Lord Vyemm"}})
	h.now = h.now.Add(sessionGap + 2*time.Second)
	h.rec.tick()

	ss := h.sessions()
	if len(ss) != 1 {
		t.Fatalf("sessions = %d, want 1", len(ss))
	}
	rec, _, err := h.store.GetRecording(ss[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Events[0].Name != "Larcen" || rec.Events[1].Other != "Larcen" {
		t.Fatalf("events = %+v", rec.Events)
	}
}

func TestRecorderTargetAndCHMarks(t *testing.T) {
	h := newHarness(t)
	h.rec.SetLogCharacter(func() string { return "Osui" })
	// One minute with no target, then a target with a position, then one
	// without (out of range).
	h.run("templeveeshan", time.Minute)
	for end := h.now.Add(time.Minute); h.now.Before(end); h.now = h.now.Add(time.Second) {
		h.rec.SetTargetName("Lord Vyemm")
		h.rec.SetTargetLoc("Lord Vyemm", 100, -200, 5)
		h.rec.SetRaid("Osui", "templeveeshan", raid(0))
		h.rec.tick()
	}
	// A position for some other mob is ignored.
	for end := h.now.Add(time.Minute); h.now.Before(end); h.now = h.now.Add(time.Second) {
		h.rec.SetTargetName("Larcen")
		h.rec.SetTargetLoc("a drake", 1, 2, 3)
		h.rec.SetRaid("Osui", "templeveeshan", raid(0))
		h.rec.tick()
	}
	h.rec.HandleCHCall(h.now, "Clerica", "Larcen")
	h.rec.HandleLine(h.now, "Larcen is completely healed.")
	h.rec.HandleLine(h.now, "You are completely healed.")
	h.rec.HandleLine(h.now, "Larcen is healed.")
	h.now = h.now.Add(sessionGap + 2*time.Second)
	h.rec.tick()

	ss := h.sessions()
	if len(ss) != 1 {
		t.Fatalf("sessions = %d", len(ss))
	}
	rec, _, err := h.store.GetRecording(ss[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var none, withLoc, nameOnly int
	for _, f := range rec.Frames {
		switch {
		case f.Tg == nil:
			none++
		case f.Tg.N == "Lord Vyemm" && len(f.Tg.L) == 3 && f.Tg.L[1] == -200:
			withLoc++
		case f.Tg.N == "Larcen" && f.Tg.L == nil:
			nameOnly++
		default:
			t.Fatalf("frame target = %+v", f.Tg)
		}
	}
	if none != 20 || withLoc != 20 || nameOnly != 20 {
		t.Fatalf("frames none/loc/name = %d/%d/%d", none, withLoc, nameOnly)
	}
	if len(rec.Events) != 3 ||
		rec.Events[0] != (Event{T: rec.Events[0].T, Kind: EventCH, Name: "Larcen", Other: "Clerica"}) ||
		rec.Events[1].Kind != EventCHLand || rec.Events[1].Name != "Larcen" ||
		rec.Events[2].Name != "Osui" {
		t.Fatalf("events = %+v", rec.Events)
	}
	if ss[0].Kills != 0 || ss[0].Deaths != 0 {
		t.Fatalf("CH marks counted as kills/deaths: %+v", ss[0])
	}
}

func TestRecorderZoneChangeAndShortSessions(t *testing.T) {
	h := newHarness(t)
	h.run("templeveeshan", 30*time.Second) // too short to keep
	h.run("westwastes", 2*time.Minute)
	h.rec.mu.Lock()
	h.rec.endLocked()
	h.rec.mu.Unlock()
	ss := h.sessions()
	if len(ss) != 1 || ss[0].Zone != "westwastes" {
		t.Fatalf("sessions = %+v", ss)
	}
}

func TestRecorderDisabled(t *testing.T) {
	h := newHarness(t)
	h.cfg.Disabled = true
	h.run("templeveeshan", 2*time.Minute)
	if ss := h.sessions(); len(ss) != 0 {
		t.Fatalf("recorded while disabled: %+v", ss)
	}
}

func TestStorePrune(t *testing.T) {
	h := newHarness(t)
	h.run("templeveeshan", 2*time.Minute)
	h.rec.mu.Lock()
	h.rec.endLocked()
	h.rec.mu.Unlock()
	if n, err := h.store.PruneOlderThan(h.now.Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("pruned recent: %d %v", n, err)
	}
	if n, err := h.store.PruneOlderThan(h.now.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	var left int
	h.store.db.QueryRow(`SELECT (SELECT COUNT(*) FROM raid_track_chunks) + (SELECT COUNT(*) FROM raid_track_members)`).Scan(&left) //nolint:errcheck
	if left != 0 || len(h.sessions()) != 0 {
		t.Fatalf("rows left after prune: %d", left)
	}
}

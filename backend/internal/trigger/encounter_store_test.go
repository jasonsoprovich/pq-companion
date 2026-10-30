package trigger

import (
	"encoding/json"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"path/filepath"
	"testing"
	"time"
)

func TestEncounterSQLiteMigrationAndCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	old := &Trigger{ID: "old", Name: "Old", Pattern: "x", TimerType: TimerTypeCustom}
	if err = s.Insert(old); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"timer_if_idle", "timer_clear_on_context_change"} {
		if _, err = s.db.Exec("ALTER TABLE triggers DROP COLUMN " + column); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get("old")
	if err != nil || got.TimerIfIdle || got.TimerClearOnContextChange {
		t.Fatal("old schema defaults changed", got, err)
	}
	got.TimerIfIdle = true
	got.TimerClearOnContextChange = true
	got.SourcePack = "Fixture"
	got.PackName = "Fixture"
	if err = s.Update(got); err != nil {
		t.Fatal(err)
	}
	for _, list := range []func() ([]*Trigger, error){s.List, func() ([]*Trigger, error) { return s.ListBySourcePack("Fixture") }, func() ([]*Trigger, error) { return s.ListByCategory("Fixture") }, func() ([]*Trigger, error) { return s.ListByCategoryIDs([]string{got.CategoryID}) }} {
		rows, err := list()
		if err != nil || len(rows) != 1 || !rows[0].TimerIfIdle || !rows[0].TimerClearOnContextChange {
			t.Fatal("query projection lost flags", rows, err)
		}
	}
	copy := *got
	copy.ID = "copy"
	if err = s.Insert(&copy); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(TriggerPack{PackName: "Export", Triggers: []Trigger{copy}})
	preview, err := DetectAndParse("export.json", b)
	if err != nil || len(preview.Triggers) != 1 || !preview.Triggers[0].Trigger.TimerIfIdle || !preview.Triggers[0].Trigger.TimerClearOnContextChange {
		t.Fatal("native import lost flags", preview, err)
	}
	if err = s.UpsertPackBaseline("Fixture", &copy); err != nil {
		t.Fatal(err)
	}
}

type unsupportedEncounterSink struct{ starts, stops int }

func (s *unsupportedEncounterSink) StartExternal(_, _ string, _, _ float64, _ time.Time, _ json.RawMessage, _ int, _, _ string, _ bool, _ string, _ bool, _ ...bool) {
	s.starts++
}
func (s *unsupportedEncounterSink) StopExternal(_ string, _ int) { s.stops++ }
func TestEncounterUnsupportedSinkFailsClosedAndSilent(t *testing.T) {
	s := openTestStore(t)
	sink := &unsupportedEncounterSink{}
	e := NewEngine(s, ws.NewHub(), sink, nil)
	tr := &Trigger{ID: "one", Name: "Encounter", Enabled: true, Pattern: "^START$", WornOffPattern: "^STOP$", TimerType: TimerTypeCustom, TimerDurationSecs: 10, TimerIfIdle: true}
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(time.Now(), "START")
	e.Handle(time.Now(), "STOP")
	if sink.starts != 0 || sink.stops != 0 || len(e.GetHistory()) != 0 {
		t.Fatal("unsupported sink fell back to legacy")
	}
	report := e.RunTest(TestRequest{Trigger: tr, Lines: "START\nSTOP", FireEffects: false})
	if report.Matched != 2 || report.Fired != 0 || sink.starts != 0 || sink.stops != 0 {
		t.Fatal("silent tester consulted live sink", report)
	}
	report = e.RunTest(TestRequest{Trigger: tr, Lines: "START", FireEffects: true})
	if report.Fired != 0 {
		t.Fatal("unsupported tester claimed fire")
	}
	tr.TimerIfIdle = false
	if err := s.Update(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(time.Now(), "START")
	if sink.starts != 1 || len(e.GetHistory()) != 1 {
		t.Fatal("legacy sink behavior changed")
	}
}

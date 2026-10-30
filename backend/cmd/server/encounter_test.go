package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func awaitEncounter(t *testing.T, fn func() bool) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for !fn() {
		if time.Now().After(until) {
			t.Fatal("physical Tailer transition not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type encounterFixture struct {
	cfg    *config.Manager
	tail   *logparser.Tailer
	e      *trigger.Engine
	timers *spelltimer.Engine
	path   string
	cancel context.CancelFunc
	done   chan struct{}
	raw    chan string
	zone   chan struct{}
}

func physicalEncounter(t *testing.T) *encounterFixture {
	t.Helper()
	dir := t.TempDir()
	mgr, err := config.LoadFrom(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := mgr.Get()
	c.EQPath = dir
	c.Character = "Tester"
	c.Preferences.ParseCombatLog = true
	if err = mgr.Update(c); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "eqlog_Tester_pq.proj.txt")
	if err = os.WriteFile(path, []byte("historical bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := trigger.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	hub := ws.NewHub()
	timers := spelltimer.NewEngine(hub, nil, nil, nil, nil, nil, nil, nil, nil)
	e := trigger.NewEngine(store, hub, timers, func() string { return mgr.Get().Character })
	tr := &trigger.Trigger{ID: "owned", Name: "Encounter", Enabled: true, Pattern: "^START$", WornOffPattern: "^STOP$", TimerType: trigger.TimerTypeCustom, TimerDurationSecs: 7380, TimerIfIdle: true, TimerClearOnContextChange: true}
	if err = store.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	f := &encounterFixture{cfg: mgr, e: e, timers: timers, path: path, done: make(chan struct{}), raw: make(chan string, 32), zone: make(chan struct{}, 8)}
	f.tail = logparser.NewTailer(mgr, func(ev logparser.LogEvent) {
		timers.Handle(ev)
		if ev.Type == logparser.EventZone {
			f.zone <- struct{}{}
		}
	}, func(_ time.Time, msg string) { f.raw <- msg }, nil)
	wireEncounterTimers(f.tail, e, newConsumerGuard())
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() { defer close(f.done); f.tail.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-f.done:
		case <-time.After(4 * time.Second):
			t.Error("Tailer did not join")
		}
	})
	awaitEncounter(t, func() bool { return f.tail.Status().Tailing })
	timers.StartExternal("Encounter", "custom", 900, 0, time.Now(), nil, 0, "", "", false, "", false)
	return f
}
func (f *encounterFixture) append(t *testing.T, msgs ...string) {
	t.Helper()
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stamp := time.Now().Format("[Mon Jan _2 15:04:05 2006] ")
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(stamp + msg + "\n")
	}
	if _, err = file.WriteString(b.String()); err != nil {
		t.Fatal(err)
	}
}
func (f *encounterFixture) mark(t *testing.T) {
	t.Helper()
	for {
		select {
		case s := <-f.raw:
			if s == "MARK" {
				return
			}
		case <-time.After(4 * time.Second):
			t.Fatal("batch not dispatched")
		}
	}
}
func TestEncounterProductionPhysicalZoneBatch(t *testing.T) {
	f := physicalEncounter(t)
	f.append(t, "START", "MARK")
	f.mark(t)
	if len(f.timers.GetState().Timers) != 2 {
		t.Fatal("start not admitted")
	}
	f.append(t, "You have entered The North Karana.", "START", "MARK")
	f.mark(t)
	select {
	case <-f.zone:
	case <-time.After(4 * time.Second):
		t.Fatal("parsed zone not dispatched")
	}
	if len(f.timers.GetState().Timers) != 2 {
		t.Fatal("late parsed zone erased valid same-append start", f.timers.GetState())
	}
	rows := f.timers.GetState().Timers
	found := false
	for _, row := range rows {
		if row.DurationSeconds == 7380 {
			found = true
		}
	}
	if !found {
		t.Fatal("wrong timer duration")
	}
	if len(f.e.GetHistory()) != 2 {
		t.Fatal("zone failed to retire/rearm before the same-append new start")
	}
}
func TestEncounterProductionPhysicalTransitions(t *testing.T) {
	for _, transition := range []string{"replace", "truncate", "character", "eq_source", "parsing_disable", "pause_resume", "shutdown"} {
		t.Run(transition, func(t *testing.T) {
			f := physicalEncounter(t)
			f.append(t, "START", "MARK")
			f.mark(t)
			switch transition {
			case "replace":
				if err := os.Remove(f.path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.path, []byte("replacement history\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "truncate":
				if err := os.WriteFile(f.path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "character":
				if err := f.cfg.Modify(func(c *config.Config) { c.Character = "Other" }); err != nil {
					t.Fatal(err)
				}
			case "eq_source":
				if err := f.cfg.Modify(func(c *config.Config) { c.EQPath = t.TempDir() }); err != nil {
					t.Fatal(err)
				}
			case "parsing_disable":
				if err := f.cfg.Modify(func(c *config.Config) { c.Preferences.ParseCombatLog = false }); err != nil {
					t.Fatal(err)
				}
			case "pause_resume":
				f.tail.SetPaused(true)
				f.tail.SetPaused(false)
			case "shutdown":
				f.cancel()
				<-f.done
			}
			awaitEncounter(t, func() bool { return len(f.timers.GetState().Timers) == 1 })
			if f.timers.GetState().Timers[0].DurationSeconds != 900 {
				t.Fatal("context touched legacy timer")
			}
			if transition != "shutdown" {
				c := f.cfg.Get()
				f.path = filepath.Join(c.EQPath, "eqlog_"+c.Character+"_pq.proj.txt")
				if transition == "character" || transition == "eq_source" {
					if err := os.WriteFile(f.path, []byte("new source history\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if transition == "parsing_disable" {
					if err := f.cfg.Modify(func(c *config.Config) { c.Preferences.ParseCombatLog = true }); err != nil {
						t.Fatal(err)
					}
				}
				awaitEncounter(t, func() bool { return f.tail.Status().Tailing })
				f.append(t, "START", "MARK")
				f.mark(t)
				if len(f.timers.GetState().Timers) != 2 || len(f.e.GetHistory()) != 2 {
					t.Fatal("fresh physical source failed to admit next attempt")
				}
			}
		})
	}
}
func TestEncounterProductionRetiredInflightLine(t *testing.T) {
	f := physicalEncounter(t)
	arrived := make(chan struct{})
	release := make(chan struct{})
	f.e.SetTargetProvider(func() string { close(arrived); <-release; return "" })
	f.append(t, "START", "MARK")
	select {
	case <-arrived:
	case <-time.After(4 * time.Second):
		close(release)
		t.Fatal("old source did not reach barrier")
	}
	if err := f.cfg.Modify(func(c *config.Config) { c.Preferences.ParseCombatLog = false }); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	f.mark(t)
	if len(f.e.GetHistory()) != 0 || len(f.timers.GetState().Timers) != 1 {
		t.Fatal("old physical source recreated retired row/actions")
	}
}

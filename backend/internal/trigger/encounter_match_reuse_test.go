package trigger_test

import (
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"testing"
	"time"
)

func TestEncounterCapturedMatches(t *testing.T) {
	for _, tc := range []struct {
		name, line             string
		duration               float64
		excluded, stop, legacy bool
	}{
		{name: "primary_before_extra", line: "PRIMARY Main Alpha", duration: 350},
		{name: "first_extra_and_named_captures", line: "EXTRA Alpha Main", duration: 80},
		{name: "excluded_extra", line: "EXTRA Alpha Main", excluded: true},
		{name: "negative", line: "unrelated activity"},
		{name: "excluded_start_still_stops", line: "STOP Main Alpha", excluded: true, stop: true},
		{name: "missed_start_still_stops", line: "STOP Main Alpha", stop: true},
		{name: "legacy_match_without_managed_plan", line: "LEGACY START", duration: 90, legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, e, timers := encounterOwners(t)
			tr := &trigger.Trigger{ID: "reuse", Name: "Reusable", Enabled: true, Pattern: `^PRIMARY (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$`, WornOffPattern: `^STOP (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$`, TimerType: trigger.TimerTypeCustom, TimerDurationSecs: 350, TimerKeyCapture: "key", TimerTargetCapture: "target", TimerClearOnContextChange: true, Actions: []trigger.Action{{Type: trigger.ActionTextToSpeech, Text: "{key}/{target}"}}, ExtraPatterns: []trigger.ExtraPattern{
				{Enabled: true, Pattern: `^EXTRA (?P<target>[A-Za-z]+) (?P<key>[A-Za-z]+)$`, TimerDurationSecs: 80},
				{Enabled: true, Pattern: `^EXTRA (?P<target>[A-Za-z]+) (?P<key>[A-Za-z]+)$`, TimerDurationSecs: 999},
				{Enabled: true, Pattern: `^PRIMARY (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$`, TimerDurationSecs: 999},
			}}
			if tc.excluded {
				tr.ExcludePatterns = []string{`^(EXTRA|STOP) `}
			}
			if tc.stop && tc.excluded {
				tr.ExtraPatterns = append(tr.ExtraPatterns, trigger.ExtraPattern{Enabled: true, Pattern: `^STOP (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$`})
			}
			if err := s.Insert(tr); err != nil {
				t.Fatal(err)
			}
			if tc.legacy {
				if err := s.Insert(&trigger.Trigger{ID: "legacy", Name: "Legacy", Enabled: true, Pattern: "^LEGACY START$", TimerType: trigger.TimerTypeCustom, TimerDurationSecs: 90}); err != nil {
					t.Fatal(err)
				}
			}
			e.Reload()
			at := time.Now()
			if tc.stop {
				e.Handle(at, "PRIMARY Main Alpha")
				if len(timers.GetState().Timers) != 1 {
					t.Fatal("seed did not start")
				}
			}
			e.Handle(at, tc.line)
			rows := timers.GetState().Timers
			history := e.GetHistory()
			if tc.stop {
				if len(rows) != 0 || len(history) != 1 {
					t.Fatal("worn-off path was lost or excluded start fired", rows, history)
				}
				return
			}
			if tc.duration == 0 {
				if len(rows) != 0 || len(history) != 0 {
					t.Fatal("negative/excluded line fired", rows, history)
				}
				return
			}
			if len(rows) != 1 || len(history) != 1 {
				t.Fatal("expected one real row and initial action", rows, history)
			}
			row := rows[0]
			if row.DurationSeconds != tc.duration || !row.ExpiresAt.Equal(at.Add(time.Duration(tc.duration)*time.Second)) {
				t.Fatal("wrong selected-pattern deadline", row)
			}
			if !tc.legacy && (row.SpellName != "Main" || row.TargetName != "Alpha" || history[0].Actions[0].Text != "Main/Alpha") {
				t.Fatal("captured identity/actions not preserved", row, history)
			}
		})
	}
}

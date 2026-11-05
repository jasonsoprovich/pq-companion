package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// decode is a fixture helper: payloads here are copied in the exact shape
// Zeal writes them into the `data` field of a type-3 envelope.
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	return m
}

func TestFormatPlayerNoTarget(t *testing.T) {
	p := decode(t, `{"zone":151,"location":{"x":-102.0,"y":-784.0,"z":3.1},
		"heading":128.0,"autoattack":false,"spawn_id":812}`)
	if got := formatPlayer(p); got != "no target" {
		t.Errorf("formatPlayer() = %q, want %q", got, "no target")
	}
}

func TestFormatPlayerFullDescriptors(t *testing.T) {
	// A patched Zeal targeting the northern Kaas Thox in Vex Thal.
	p := decode(t, `{"zone":152,"location":{"x":0.0,"y":0.0,"z":0.0},
		"heading":0.0,"autoattack":true,"spawn_id":812,
		"target_id":1234,"target_name":"Kaas Thox Xi Aten Ha Ra","target_type":1,
		"target_level":66,"target_class":9,"target_race":145,
		"target_loc":{"x":318.0,"y":141.0,"z":-80.5}}`)
	got := formatPlayer(p)
	for _, want := range []string{
		"id=1234",
		"name=Kaas Thox Xi Aten Ha Ra",
		"type=1 (NPC)",
		"level=66",
		"class=9",
		"race=145",
		"loc=(318.0,141.0,-80.5)",
		"dist≈355",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatPlayer() = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "MISSING") {
		t.Errorf("formatPlayer() = %q, should not report anything missing", got)
	}
}

// A stock v1.4.7 Zeal emits target_id and nothing else. The tool has to say so
// loudly, otherwise a failed .asi swap looks identical to a working one.
func TestFormatPlayerPreChangeZealReportsMissing(t *testing.T) {
	p := decode(t, `{"zone":152,"location":{"x":0.0,"y":0.0,"z":0.0},
		"heading":0.0,"autoattack":false,"spawn_id":812,"target_id":1234}`)
	got := formatPlayer(p)
	if !strings.Contains(got, "MISSING[") {
		t.Fatalf("formatPlayer() = %q, want a MISSING warning", got)
	}
	for _, want := range []string{"name", "type", "level", "class", "race", "loc"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatPlayer() = %q, MISSING list should name %q", got, want)
		}
	}
}

func TestFormatPlayerSpawnTypeLabels(t *testing.T) {
	tests := map[int]string{0: "type=0 (player)", 1: "type=1 (NPC)", 2: "type=2 (NPC corpse)", 3: "type=3 (player corpse)"}
	for code, want := range tests {
		p := map[string]any{"target_id": 1.0, "target_type": float64(code)}
		if got := formatPlayer(p); !strings.Contains(got, want) {
			t.Errorf("target_type=%d: formatPlayer() = %q, want %q", code, got, want)
		}
	}
}

// Zeal's location x/y are transposed relative to the game's /loc order, so the
// tool prints both readings. Guard the conversion.
func TestFormatLocPrintsGameLocOrder(t *testing.T) {
	got := formatLoc(map[string]any{"x": 318.0, "y": 141.0, "z": -80.5})
	if !strings.Contains(got, "game /loc 141.0,318.0") {
		t.Errorf("formatLoc() = %q, want the transposed /loc reading", got)
	}
}

func TestCoverageReportTracksConditions(t *testing.T) {
	c := newCoverage()
	c.observe(decode(t, `{"spawn_id":1}`))
	c.observe(decode(t, `{"target_id":5,"target_name":"a gnoll pup","target_type":1,
		"target_level":12,"target_class":1,"target_race":58,"target_loc":{"x":1,"y":2,"z":3}}`))
	rep := c.report()
	if strings.Contains(rep, "[ ] saw a tick with no target") {
		t.Error("report should mark the no-target condition seen")
	}
	if strings.Contains(rep, "[ ] saw target_level") {
		t.Error("report should mark target_level seen")
	}
	if !strings.Contains(rep, "[ ] saw target_type=0") {
		t.Error("report should still show player-target as unseen")
	}
	if !strings.Contains(rep, "a gnoll pup") {
		t.Error("report should list distinct target names seen")
	}
}

// A patched Zeal withholds target_loc beyond 250 units. That's expected and
// must not be reported as a missing key.
func TestFormatPlayerLocWithheldIsNotMissing(t *testing.T) {
	p := decode(t, `{"zone":152,"location":{"x":0.0,"y":0.0,"z":0.0},
		"heading":0.0,"autoattack":false,"spawn_id":812,
		"target_id":1234,"target_name":"a gnoll","target_type":1,
		"target_level":12,"target_class":1,"target_race":39}`)
	got := formatPlayer(p)
	if strings.Contains(got, "MISSING") {
		t.Errorf("formatPlayer() = %q, withheld loc reported as missing", got)
	}
	if !strings.Contains(got, "loc=withheld") {
		t.Errorf("formatPlayer() = %q, want loc=withheld", got)
	}
}

func TestFormatConsider(t *testing.T) {
	for _, typ := range []zealpipe.PipeMessageType{7, 9} {
		got, ok := formatConsider(zealpipe.Envelope{Type: typ,
			Data: `{"spawn_id":1234,"name":"a gnoll pup","faction":9}`})
		want := fmt.Sprintf("type=%d spawn_id=1234 name=a gnoll pup faction=9 (scowls)", typ)
		if !ok || got != want {
			t.Errorf("formatConsider(type %d) = %q, %v; want %q", typ, got, ok, want)
		}
	}
	for _, env := range []zealpipe.Envelope{
		{Type: 7, Data: `{"spawn_id":1234,"killer_id":0}`},             // a death
		{Type: 8, Data: `{"item_id":13073,"name":"Bone Chips"}`},       // an item
		{Type: 6, Data: `{"spawn_id":1,"name":"a gnoll","faction":9}`}, // below 7
	} {
		if got, ok := formatConsider(env); ok {
			t.Errorf("formatConsider(%+v) = %q, want not a consider", env, got)
		}
	}
	got, ok := formatConsider(zealpipe.Envelope{Type: 9, Data: `{"spawn_id":1,"name":"a gnoll","faction":0}`})
	if !ok || !strings.Contains(got, "UNUSABLE") {
		t.Errorf("bad faction: %q, %v", got, ok)
	}
}

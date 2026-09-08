package zealpipe

import (
	"encoding/json"
	"testing"
)

// jsonString returns s wrapped as a JSON string literal (with the escaping
// Zeal's serializer applies), so a test fixture can embed a payload as the
// double-encoded `data` field the wire format actually uses.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The real wire format has `data` as a JSON-encoded string of JSON — see
// Zeal/named_pipe.h:21. Fixtures below use the literal double-encoded form
// that Zeal actually writes.

func TestDecodeEnvelopeLabel(t *testing.T) {
	line := []byte(`{"type":1,"data_len":58,"data":"[{\"type\":28,\"value\":\"a gnoll pup\"},{\"type\":29,\"value\":\"73\"}]","character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgLabel {
		t.Fatalf("Type = %v, want MsgLabel", env.Type)
	}
	if env.Character != "Osui" {
		t.Fatalf("Character = %q, want Osui", env.Character)
	}
	labels, err := DecodeLabels(env.Data)
	if err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("len(labels) = %d, want 2", len(labels))
	}
	if labels[0].Type != LabelTargetName || labels[0].Value != "a gnoll pup" {
		t.Errorf("label[0] = %+v", labels[0])
	}
	if labels[1].Type != LabelTargetHPPerc || labels[1].Value != "73" {
		t.Errorf("label[1] = %+v", labels[1])
	}
}

func TestDecodeEnvelopePlayer(t *testing.T) {
	line := []byte(`{"type":3,"data":"{\"zone\":24,\"location\":{\"x\":1.5,\"y\":-200,\"z\":3.0},\"heading\":128,\"autoattack\":true}","character":"Nariana"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgPlayer {
		t.Fatalf("Type = %v, want MsgPlayer", env.Type)
	}
	p, err := DecodePlayer(env.Data)
	if err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if p.Zone != 24 || !p.AutoAttack || p.Location.Y != -200 {
		t.Errorf("player = %+v", p)
	}
}

func TestDecodePlayerSpawnIDsV146(t *testing.T) {
	// Zeal v1.4.6 (PR #229): player snapshot carries spawn_id (self), and
	// target_id / pet_id when a target / pet exists.
	line := []byte(`{"type":3,"data":"{\"zone\":24,\"location\":{\"x\":1,\"y\":-2,\"z\":3},\"heading\":10,\"autoattack\":false,\"spawn_id\":1234,\"target_id\":5678,\"pet_id\":4321}","character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	p, err := DecodePlayer(env.Data)
	if err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if p.SpawnID == nil || *p.SpawnID != 1234 {
		t.Errorf("SpawnID = %v, want 1234", p.SpawnID)
	}
	if p.TargetID == nil || *p.TargetID != 5678 {
		t.Errorf("TargetID = %v, want 5678", p.TargetID)
	}
	if p.PetID == nil || *p.PetID != 4321 {
		t.Errorf("PetID = %v, want 4321", p.PetID)
	}
}

func TestDecodePlayerNoTargetNoPet(t *testing.T) {
	// Zeal omits target_id / pet_id entirely (not 0, not -1) when there is no
	// target or no pet — a consumer must test for presence.
	line := []byte(`{"type":3,"data":"{\"zone\":24,\"location\":{\"x\":1,\"y\":-2,\"z\":3},\"heading\":10,\"autoattack\":false,\"spawn_id\":1234}","character":"Osui"}`)
	env, _ := DecodeEnvelope(line)
	p, err := DecodePlayer(env.Data)
	if err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if p.SpawnID == nil || *p.SpawnID != 1234 {
		t.Errorf("SpawnID = %v, want 1234", p.SpawnID)
	}
	if p.TargetID != nil {
		t.Errorf("TargetID = %v, want nil (no target)", *p.TargetID)
	}
	if p.PetID != nil {
		t.Errorf("PetID = %v, want nil (no pet)", *p.PetID)
	}
}

func TestDecodePlayerPre146NoIDs(t *testing.T) {
	// Any Zeal before v1.4.6 sends no id keys at all. Decode must succeed and
	// leave every id pointer nil so consumers stay on the name-based path.
	line := []byte(`{"type":3,"data":"{\"zone\":24,\"location\":{\"x\":1,\"y\":-2,\"z\":3},\"heading\":10,\"autoattack\":true}","character":"Osui"}`)
	env, _ := DecodeEnvelope(line)
	p, err := DecodePlayer(env.Data)
	if err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if p.SpawnID != nil || p.TargetID != nil || p.PetID != nil {
		t.Errorf("expected all id pointers nil, got %v/%v/%v", p.SpawnID, p.TargetID, p.PetID)
	}
	if !p.AutoAttack {
		t.Errorf("AutoAttack lost")
	}
}

func TestDecodeRaidRosterNoVerbose(t *testing.T) {
	// MsgRaid with PipeVerbose off: name/level/class/group/rank always present;
	// spawn_id/loc/heading only for the member in your zone; no hp fields.
	payload := `[` +
		`{"name":"Tank","level":60,"class":1,"group":"1","rank":"Raid Leader","spawn_id":100,"loc":{"x":-784,"y":-102,"z":5},"heading":128},` +
		`{"name":"Healer","level":60,"class":2,"group":"1","rank":"Group Leader"},` +
		`{"name":"Rogue","level":59,"class":9,"group":"2","rank":""}` +
		`]`
	line := []byte(`{"type":5,"data":` + jsonString(payload) + `,"character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgRaid {
		t.Fatalf("Type = %v, want MsgRaid", env.Type)
	}
	members, err := DecodeRaid(env.Data)
	if err != nil {
		t.Fatalf("decode raid: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(members))
	}
	if members[0].Name != "Tank" || members[0].Level != 60 || members[0].Class != 1 ||
		members[0].Group != "1" || members[0].Rank != "Raid Leader" {
		t.Errorf("members[0] = %+v", members[0])
	}
	if members[0].SpawnID == nil || *members[0].SpawnID != 100 {
		t.Errorf("members[0].SpawnID = %v, want 100", members[0].SpawnID)
	}
	if members[0].Loc == nil || members[0].Loc.GameY() != -784 {
		t.Errorf("members[0].Loc = %+v (GameY want -784)", members[0].Loc)
	}
	// Out-of-zone member: roster fields present, entity fields nil.
	if members[1].SpawnID != nil || members[1].Loc != nil {
		t.Errorf("members[1] out-of-zone should have nil spawn/loc: %+v", members[1])
	}
	if members[1].Name != "Healer" || members[1].Class != 2 {
		t.Errorf("members[1] roster fields lost: %+v", members[1])
	}
	if members[0].HPCur != nil {
		t.Errorf("HPCur set without PipeVerbose: %v", *members[0].HPCur)
	}
}

func TestDecodeRaidRosterVerbose(t *testing.T) {
	// PipeVerbose on: in-zone members also carry hp_current/hp_max/zone_id.
	payload := `[{"name":"Tank","level":60,"class":1,"group":"1","rank":"","spawn_id":100,` +
		`"loc":{"x":0,"y":0,"z":0},"heading":0,"hp_current":8000,"hp_max":9000,"zone_id":24}]`
	line := []byte(`{"type":5,"data":` + jsonString(payload) + `,"character":"Osui"}`)
	env, _ := DecodeEnvelope(line)
	members, err := DecodeRaid(env.Data)
	if err != nil {
		t.Fatalf("decode raid: %v", err)
	}
	if members[0].HPCur == nil || *members[0].HPCur != 8000 ||
		members[0].HPMax == nil || *members[0].HPMax != 9000 ||
		members[0].ZoneID == nil || *members[0].ZoneID != 24 {
		t.Errorf("verbose fields = %+v", members[0])
	}
}

func TestDecodeGroupRoster(t *testing.T) {
	payload := `[{"name":"You","spawn_id":1,"loc":{"x":10,"y":20,"z":0},"heading":90},` +
		`{"name":"Friend","spawn_id":2,"loc":{"x":11,"y":21,"z":0},"heading":95,` +
		`"hp_current":500,"hp_max":600,"class":5,"level":58,"zone_id":24}]`
	line := []byte(`{"type":6,"data":` + jsonString(payload) + `,"character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgGroup {
		t.Fatalf("Type = %v, want MsgGroup", env.Type)
	}
	members, err := DecodeGroup(env.Data)
	if err != nil {
		t.Fatalf("decode group: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("len = %d, want 2", len(members))
	}
	if members[0].Name != "You" || members[0].SpawnID == nil || *members[0].SpawnID != 1 {
		t.Errorf("members[0] = %+v", members[0])
	}
	if members[0].Class != nil {
		t.Errorf("members[0].Class set without verbose: %v", *members[0].Class)
	}
	if members[1].Class == nil || *members[1].Class != 5 || members[1].Level == nil || *members[1].Level != 58 {
		t.Errorf("members[1] verbose fields = %+v", members[1])
	}
}

func TestDecodeRaidGroupEmpty(t *testing.T) {
	if m, err := DecodeRaid(""); err != nil || m != nil {
		t.Errorf("empty raid: m=%v err=%v", m, err)
	}
	if m, err := DecodeGroup("null"); err != nil || m != nil {
		t.Errorf("null group: m=%v err=%v", m, err)
	}
}

func TestDecodeEnvelopePipeCmd(t *testing.T) {
	line := []byte(`{"type":4,"data":"{\"text\":\"pull\"}","character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgCmd {
		t.Fatalf("Type = %v, want MsgCmd", env.Type)
	}
	c, err := DecodePipeCmd(env.Data)
	if err != nil {
		t.Fatalf("decode cmd: %v", err)
	}
	if c.Text != "pull" {
		t.Errorf("cmd text = %q, want pull", c.Text)
	}
}

func TestDecodeLabelsEmpty(t *testing.T) {
	labels, err := DecodeLabels("")
	if err != nil || labels != nil {
		t.Errorf("empty payload: labels=%v err=%v", labels, err)
	}
	labels, err = DecodeLabels("null")
	if err != nil || labels != nil {
		t.Errorf("null payload: labels=%v err=%v", labels, err)
	}
}

func TestDecodeEnvelopeInvalid(t *testing.T) {
	if _, err := DecodeEnvelope([]byte("not json")); err == nil {
		t.Error("expected error for non-JSON")
	}
}

func TestUnknownLabelTypeAccepted(t *testing.T) {
	// Schema is best-effort — unknown IDs decode as their numeric value rather
	// than fail. Consumers ignore via a switch default.
	line := []byte(`{"type":1,"data":"[{\"type\":9999,\"value\":\"x\"}]","character":"X"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	labels, err := DecodeLabels(env.Data)
	if err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	if labels[0].Type != LabelType(9999) {
		t.Errorf("unknown label type lost: %v", labels[0].Type)
	}
}

func TestLocationGameAccessorsTranspose(t *testing.T) {
	// Zeal's JSON "x" carries the game's Y. Verified in game 2026-07-30 by
	// standing in the Bazaar at /loc -784, -102 and measuring where the arrow
	// drew; see the Location doc comment.
	//
	// This test exists because the raw fields are named exactly what a reader
	// expects them to mean, so the bug reappears the moment someone "tidies"
	// GameX() away.
	l := Location{X: -784, Y: -102, Z: 5}
	if got := l.GameX(); got != -102 {
		t.Errorf("GameX() = %v, want -102 (Zeal's y)", got)
	}
	if got := l.GameY(); got != -784 {
		t.Errorf("GameY() = %v, want -784 (Zeal's x)", got)
	}
}

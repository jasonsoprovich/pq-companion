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

func TestDecodePlayerTargetDescriptors(t *testing.T) {
	// Zeal PR #239 shape. target_loc is in Zeal's (y, x, z) order like
	// location: the JSON "x" is the game's Y.
	full := `{"zone":158,"location":{"x":300,"y":141,"z":130},"heading":0,"autoattack":false,` +
		`"spawn_id":1,"target_id":77,"target_name":"Kaas Thox Xi Aten Ha Ra","target_type":1,` +
		`"target_level":66,"target_class":9,"target_race":145,"target_loc":{"x":318,"y":141,"z":130}}`
	noLoc := `{"zone":158,"location":{"x":-300,"y":141,"z":130},"heading":0,"autoattack":false,` +
		`"target_id":77,"target_name":"a gnoll","target_type":2,"target_level":12,"target_class":1,"target_race":39}`
	appearance := `{"zone":211,"location":{"x":1,"y":2,"z":3},"heading":0,"autoattack":false,` +
		`"target_id":9,"target_name":"A Valiant Spirit","target_type":1,"target_level":62,"target_class":1,` +
		`"target_race":323,"target_texture":2,"target_gender":2,"target_size":12,` +
		`"target_equipment":[0,0,0,0,0,0,0,0,0]}`
	partialAppearance := `{"zone":211,"location":{"x":1,"y":2,"z":3},"heading":0,"autoattack":false,` +
		`"target_id":9,"target_name":"x","target_type":1,"target_level":62,"target_class":1,"target_race":323,` +
		`"target_texture":2}`
	stock147 := `{"zone":158,"location":{"x":1,"y":2,"z":3},"heading":0,"autoattack":false,"target_id":77}`

	tests := []struct {
		name     string
		payload  string
		wantDesc bool
		want     TargetDescriptors
		wantDist float64
		distOK   bool
	}{
		{"full", full, true, TargetDescriptors{
			Name: "Kaas Thox Xi Aten Ha Ra", Type: TargetTypeNPC, Level: 66, Class: 9, Race: 145,
			LocKnown: true, GameX: 141, GameY: 318, Z: 130,
		}, 18, true},
		{"loc withheld beyond 250", noLoc, true, TargetDescriptors{
			Name: "a gnoll", Type: TargetTypeNPCCorpse, Level: 12, Class: 1, Race: 39,
		}, 0, false},
		{"appearance", appearance, true, TargetDescriptors{
			Name: "A Valiant Spirit", Type: TargetTypeNPC, Level: 62, Class: 1, Race: 323,
			HasAppearance: true, Texture: 2, Gender: 2, Size: 12,
		}, 0, false},
		{"partial appearance is ignored", partialAppearance, true, TargetDescriptors{
			Name: "x", Type: TargetTypeNPC, Level: 62, Class: 1, Race: 323,
		}, 0, false},
		{"stock v1.4.7", stock147, false, TargetDescriptors{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := DecodePlayer(tt.payload)
			if err != nil {
				t.Fatalf("decode player: %v", err)
			}
			d := p.TargetDescriptors()
			if (d != nil) != tt.wantDesc {
				t.Fatalf("TargetDescriptors() = %+v, want present=%v", d, tt.wantDesc)
			}
			if d != nil && *d != tt.want {
				t.Errorf("TargetDescriptors() = %+v, want %+v", *d, tt.want)
			}
			dist, ok := p.DistanceToTarget()
			if ok != tt.distOK || dist != tt.wantDist {
				t.Errorf("DistanceToTarget() = %v, %v; want %v, %v", dist, ok, tt.wantDist, tt.distOK)
			}
		})
	}
}

func TestDecodeRaidRosterLiveCapture(t *testing.T) {
	// Live capture 2026-09-14 (Zeal v1.4.6, real raid in Kael Drakkel):
	// class arrives as the display NAME ("Necromancer"), level as a STRING
	// ("60") — the opposite of what the pre-live fixtures assumed. This test
	// pins the real wire shape so a decoder regression can't silently drop
	// the whole roster again (the original bug: every MsgRaid failed to
	// unmarshal into Class int / Level int and was debug-logged away).
	payload := `[` +
		`{"class":"Wizard","group":"1","level":"60","name":"Tenchi","rank":"Raid Leader"},` +
		`{"class":"Necromancer","group":"0","heading":238.19,"hp_current":5151,"hp_max":5151,"level":"60","loc":{"x":-122.64,"y":3163.95,"z":-395.78},"name":"Kravija","rank":"","spawn_id":287,"zone_id":113}` +
		`]`
	line := []byte(`{"type":5,"data":` + jsonString(payload) + `,"character":"Kravija"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	members, err := DecodeRaid(env.Data)
	if err != nil {
		t.Fatalf("decode raid: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}
	// "Wizard" → Zeal id 12; "60" (string) → 60.
	if members[0].Name != "Tenchi" || int(members[0].Class) != 12 || int(members[0].Level) != 60 ||
		members[0].Group != "1" || members[0].Rank != "Raid Leader" {
		t.Errorf("members[0] = %+v", members[0])
	}
	// "Necromancer" → 11; string level; in-zone entity fields intact.
	if int(members[1].Class) != 11 || int(members[1].Level) != 60 || members[1].SpawnID == nil ||
		members[1].ZoneID == nil || *members[1].ZoneID != 113 {
		t.Errorf("members[1] = %+v", members[1])
	}
	if members[1].HPCur == nil || *members[1].HPCur != 5151 {
		t.Errorf("members[1].HPCur = %v, want 5151", members[1].HPCur)
	}
}

func TestDecodeRaidUnknownClassName(t *testing.T) {
	// An unrecognized class name must not drop the envelope — the member
	// decodes with Class 0 (unknown) and the rest of the roster survives.
	payload := `[` +
		`{"class":"Frobnicator","group":"1","level":"60","name":"Odd","rank":""},` +
		`{"class":"Cleric","group":"1","level":"60","name":"Heals","rank":""}` +
		`]`
	line := []byte(`{"type":5,"data":` + jsonString(payload) + `,"character":"Osui"}`)
	env, _ := DecodeEnvelope(line)
	members, err := DecodeRaid(env.Data)
	if err != nil {
		t.Fatalf("decode raid: %v", err)
	}
	if len(members) != 2 || int(members[0].Class) != 0 || int(members[1].Class) != 2 {
		t.Errorf("members = %+v", members)
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

func TestCleanTargetName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "a gnoll pup", "a gnoll pup"},
		{"spawn id appended", "a gnoll pup (1234)", "a gnoll pup"},
		{"no space before id", "a gnoll pup(1234)", "a gnoll pup"},
		{"boss name with spawn id", "Kaas Thox Xi Aten Ha Ra (881)", "Kaas Thox Xi Aten Ha Ra"},
		{"corpse suffix preserved", "a gnoll pup's corpse (881)", "a gnoll pup's corpse"},
		{"trailing digits are not an id", "Animation1", "Animation1"},
		{"parens mid-name are left alone", "a (test) gnoll", "a (test) gnoll"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanTargetName(tt.in); got != tt.want {
				t.Errorf("CleanTargetName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeDeath(t *testing.T) {
	d, err := DecodeDeath(`{"spawn_id":1234,"name":"a gnoll pup","type":1,"level":12,"killer_id":567,"killer_name":"Soandso"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.SpawnID != 1234 || d.Name != "a gnoll pup" || d.Type == nil || *d.Type != 1 ||
		d.Level == nil || *d.Level != 12 || d.KillerID != 567 || d.KillerName != "Soandso" {
		t.Errorf("unexpected decode: %+v", d)
	}
	// Unknown spawn, no killer: optional fields absent.
	d, err = DecodeDeath(`{"spawn_id":9,"killer_id":0}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.SpawnID != 9 || d.Name != "" || d.Type != nil || d.KillerID != 0 {
		t.Errorf("unexpected decode: %+v", d)
	}
}

// Payloads that aren't shaped like a death are rejected, so a different
// message that ever takes type 7 can't delete timers on living mobs.
func TestDecodeDeath_RejectsOtherShapes(t *testing.T) {
	for _, payload := range []string{
		`[{"spawn_id":5,"killer_id":1}]`,  // array, like the roster messages
		`{"spawn_id":5,"name":"a gnoll"}`, // spawn-style object, no killer_id
		`{"killer_id":3}`,                 // no spawn_id
		`{"spawn_id":0,"killer_id":3}`,    // invalid id
		`not json`,
	} {
		if _, err := DecodeDeath(payload); err == nil {
			t.Errorf("expected %s to be rejected", payload)
		}
	}
}

func TestDecodeItemDisplay(t *testing.T) {
	it, err := DecodeItemDisplay(`{"item_id":13073,"name":"Bone Chips"}`)
	if err != nil {
		t.Fatal(err)
	}
	if it.ItemID != 13073 || it.Name != "Bone Chips" {
		t.Errorf("unexpected decode: %+v", it)
	}
	for _, payload := range []string{
		`{"spawn_id":5,"killer_id":1}`, // a death
		`{"item_id":13073}`,            // no name
		`{"item_id":0,"name":"x"}`,     // invalid id
		`[{"item_id":1,"name":"x"}]`,   // array
		`not json`,
	} {
		if _, err := DecodeItemDisplay(payload); err == nil {
			t.Errorf("expected %s to be rejected", payload)
		}
	}
}

func TestLabelBuffTicks(t *testing.T) {
	// Shape Zeal writes: long-buff slots carry meta.ticks, the song-window
	// slots and ordinary labels an empty meta object.
	line := []byte(`{"type":1,"data":"[{\"type\":45,\"value\":\"Aegolism\",\"meta\":{\"ticks\":118}},{\"type\":135,\"value\":\"Selo's Accelerando\",\"meta\":{}},{\"type\":28,\"value\":\"a gnoll\",\"meta\":{}},{\"type\":46,\"value\":\"Clarity\"}]","character":"Osui"}`)
	env, err := DecodeEnvelope(line)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	labels, err := DecodeLabels(env.Data)
	if err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	want := []struct {
		ticks int
		ok    bool
	}{{118, true}, {0, false}, {0, false}, {0, false}}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels, want %d", len(labels), len(want))
	}
	for i, w := range want {
		if n, ok := labels[i].BuffTicks(); n != w.ticks || ok != w.ok {
			t.Errorf("label %d (%s): BuffTicks = %d,%v want %d,%v",
				i, labels[i].Value, n, ok, w.ticks, w.ok)
		}
	}
}

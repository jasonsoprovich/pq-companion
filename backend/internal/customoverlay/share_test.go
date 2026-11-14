package customoverlay

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	e, _ := newTestEngine(t, nil, "Lootmaster")
	def := dkpDef()
	def.Characters = []string{"Lootmaster"}
	o, err := e.Create(def)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := e.Export(o.Def.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sh.Format != ShareFormat || sh.Version != ShareVersion {
		t.Fatalf("envelope = %q v%d", sh.Format, sh.Version)
	}
	if sh.Overlay.ID != "" || sh.Overlay.Characters != nil {
		t.Fatalf("personal fields exported: id=%q chars=%v", sh.Overlay.ID, sh.Overlay.Characters)
	}
	raw, _ := json.Marshal(sh)
	got, err := ParseShare(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != def.Name || len(got.Widgets) != len(def.Widgets) || len(got.Rules) != len(def.Rules) {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if _, err := e.Export("nope"); err != ErrNotFound {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestParseShare(t *testing.T) {
	share, _ := json.Marshal(Share{Format: ShareFormat, Version: ShareVersion, Overlay: dkpDef()})
	bare, _ := json.Marshal(dkpDef())
	newer, _ := json.Marshal(Share{Format: ShareFormat, Version: ShareVersion + 1, Overlay: dkpDef()})
	tests := []struct {
		name, in, wantErr string
	}{
		{"share", string(share), ""},
		{"bare def", string(bare), ""},
		{"fenced", "```json\n" + string(share) + "\n```", ""},
		{"fenced no info", "```" + string(share) + "```", ""},
		{"empty", "  ", "nothing to import"},
		{"not json", "hello", "invalid JSON"},
		{"other format", `{"format":"pq-triggers","version":1}`, "format"},
		{"newer", string(newer), "newer version"},
		{"no overlay", `{"format":"` + ShareFormat + `","version":1}`, "missing"},
		{"random object", `{"a":1}`, "not a custom overlay"},
		{"invalid def", `{"format":"` + ShareFormat + `","version":1,"overlay":{"name":"x","widgets":[]}}`, "at least one widget"},
		{"too big", strings.Repeat(" ", maxShareSize+1), "too big"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, err := ParseShare([]byte(tc.in))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if def.ID != "" || def.Name == "" {
					t.Fatalf("bad def: id=%q name=%q", def.ID, def.Name)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

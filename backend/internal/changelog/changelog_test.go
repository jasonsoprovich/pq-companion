package changelog

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []Entry
	}{
		{
			name: "two versions",
			md: "# Changelog\n\nIntro text, not a version.\n\n" +
				"## v0.17.6 — 2026-07-19\n\nFixes stuff.\n\n### Fixes\n- one\n- two\n\n" +
				"## v0.17.5 — 2026-07-18\n\nFixes other stuff.\n",
			want: []Entry{
				{Version: "0.17.6", Date: "2026-07-19", Body: "Fixes stuff.\n\n### Fixes\n- one\n- two"},
				{Version: "0.17.5", Date: "2026-07-18", Body: "Fixes other stuff."},
			},
		},
		{
			name: "no version headers",
			md:   "# Changelog\n\nNothing here yet.\n",
			want: []Entry{},
		},
		{
			name: "empty input",
			md:   "",
			want: []Entry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.md)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, e := range got {
				if e != tt.want[i] {
					t.Errorf("entry %d = %+v, want %+v", i, e, tt.want[i])
				}
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	entries, err := Load("/nonexistent/CHANGELOG.md")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for missing file", err)
	}
	if len(entries) != 0 {
		t.Errorf("Load() entries = %v, want empty for missing file", entries)
	}
}

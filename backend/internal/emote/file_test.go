package emote

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	_ "modernc.org/sqlite"
)

const fixturePath = "../../../testdata/TAKPv22/spells_en.txt"

// newTestGameDB builds a small synthetic quarm.db seeded with just the three
// spells these tests exercise, with emote text matching the checked-in
// spells_en.txt fixture exactly — deliberately NOT the real bundled
// backend/data/quarm.db, whose spell text is periodically refreshed by the
// data-release pipeline and can legitimately drift from the frozen fixture
// over time (that drift is exactly what bootstrapDefault's pending-import
// detection is *for*, but it would make these tests nondeterministic).
// Every other spell in the fixture is intentionally absent, so
// deriveDefaultContent leaves those lines untouched, same as before this
// package depended on the game DB at all.
func newTestGameDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-quarm.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open test game db: %v", err)
	}

	if _, err := raw.Exec(`
		CREATE TABLE spells_new (
			id INTEGER PRIMARY KEY,
			name TEXT,
			you_cast TEXT,
			other_casts TEXT,
			cast_on_you TEXT,
			cast_on_other TEXT,
			spell_fades TEXT
		)
	`); err != nil {
		t.Fatalf("create spells_new: %v", err)
	}

	type row struct {
		id                                                            int
		name, youCast, otherCasts, castOnYou, castOnOther, spellFades string
	}
	rows := []row{
		{1588, "Turgur's Insects", "", "", "You feel drowsy.", " yawns.", "You feel less drowsy."},
		{207, "Divine Aura", "", "", "The gods have rendered you invulnerable.", "", "Your invulnerability fades."},
		{2267, "Curse of Turgur", "", "", "You feel drowsy.", " yawns.", "You feel less drowsy."},
	}
	for _, r := range rows {
		if _, err := raw.Exec(
			`INSERT INTO spells_new (id, name, you_cast, other_casts, cast_on_you, cast_on_other, spell_fades) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.name, r.youCast, r.otherCasts, r.castOnYou, r.castOnOther, r.spellFades,
		); err != nil {
			t.Fatalf("seed spell %d: %v", r.id, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close seed connection: %v", err)
	}

	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open test game db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func readFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// copyFixture copies the real spells_en.txt fixture into a temp dir so tests
// can write to it freely without ever mutating the checked-in file.
func copyFixture(t *testing.T) string {
	t.Helper()
	content := readFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "spells_en.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	return dir
}

func TestSplitJoinLinesRoundTrip(t *testing.T) {
	content := readFixture(t)
	nl := newlineOf(content)
	if nl != "\r\n" {
		t.Fatalf("expected CRLF fixture, got %q", nl)
	}
	lines := splitLines(content)
	rejoined := strings.Join(lines, nl)
	if rejoined != content {
		t.Fatalf("split/join round-trip not byte-identical (got %d bytes, want %d)", len(rejoined), len(content))
	}
}

func TestLineSpellIDAndEmoteText(t *testing.T) {
	content := readFixture(t)
	fields := findSpellLine(content, 1588) // Turgur's Insects
	if fields == nil {
		t.Fatal("expected to find spell 1588")
	}
	if fields[1] != "Turgur's Insects" {
		t.Fatalf("unexpected name: %q", fields[1])
	}
	et := lineEmoteText(fields)
	if et.CastOnOther != " yawns." {
		t.Fatalf("unexpected cast_on_other: %q", et.CastOnOther)
	}
}

func TestApplyOverridesDivineAuraEmptyEmote(t *testing.T) {
	content := readFixture(t)
	before := findSpellLine(content, 207)
	if before == nil {
		t.Fatal("expected to find Divine Aura (207)")
	}
	if before[idxCastOnOther] != "" {
		t.Fatalf("expected Divine Aura to ship with no cast_on_other emote, got %q", before[idxCastOnOther])
	}

	newEmote := " has had Divine Aura cast on them."
	overrides := map[int]OverrideRow{
		207: {SpellID: 207, CastOnOther: &newEmote},
	}
	rewritten := applyOverrides(content, overrides)

	after := findSpellLine(rewritten, 207)
	if after == nil {
		t.Fatal("spell 207 missing after rewrite")
	}
	if after[idxCastOnOther] != newEmote {
		t.Fatalf("cast_on_other not applied: got %q", after[idxCastOnOther])
	}
	// Every other field on that line must be untouched.
	for i, f := range before {
		if i == idxCastOnOther {
			continue
		}
		if after[i] != f {
			t.Fatalf("field %d changed unexpectedly: before %q after %q", i, f, after[i])
		}
	}
}

func TestApplyOverridesTurgursInsectsDisambiguation(t *testing.T) {
	content := readFixture(t)
	newEmote := " yawns. (Turgur's Insects)"
	overrides := map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: &newEmote},
	}
	rewritten := applyOverrides(content, overrides)

	after := findSpellLine(rewritten, 1588)
	if after[idxCastOnOther] != newEmote {
		t.Fatalf("cast_on_other not applied: got %q", after[idxCastOnOther])
	}

	// A different spell sharing the original " yawns." emote (Curse of
	// Turgur, 2267) must be untouched by the targeted rewrite.
	other := findSpellLine(rewritten, 2267)
	if other == nil {
		t.Fatal("expected to find Curse of Turgur (2267)")
	}
	if other[idxCastOnOther] != " yawns." {
		t.Fatalf("unrelated spell's emote changed: %q", other[idxCastOnOther])
	}
}

func TestApplyOverridesPreservesLineEndingAndByteCount(t *testing.T) {
	content := readFixture(t)
	newEmote := " yawns. (Turgur's Insects)"
	overrides := map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: &newEmote},
	}
	rewritten := applyOverrides(content, overrides)

	if !strings.Contains(rewritten, "\r\n") {
		t.Fatal("expected CRLF preserved in rewritten content")
	}
	if strings.Count(rewritten, "\n") != strings.Count(content, "\n") {
		t.Fatalf("line count changed: got %d want %d", strings.Count(rewritten, "\n"), strings.Count(content, "\n"))
	}

	// Every line except the targeted spell's must be byte-identical.
	origLines := splitLines(content)
	newLines := splitLines(rewritten)
	if len(origLines) != len(newLines) {
		t.Fatalf("line count mismatch: %d vs %d", len(origLines), len(newLines))
	}
	changed := 0
	for i := range origLines {
		if origLines[i] != newLines[i] {
			changed++
			fields := parseFields(origLines[i])
			if id, ok := lineSpellID(fields); !ok || id != 1588 {
				t.Fatalf("unexpected line changed at index %d: %q", i, origLines[i])
			}
		}
	}
	if changed != 1 {
		t.Fatalf("expected exactly 1 changed line, got %d", changed)
	}
}

func TestApplyOverridesNoOverridesIsNoOp(t *testing.T) {
	content := readFixture(t)
	rewritten := applyOverrides(content, map[int]OverrideRow{})
	if rewritten != content {
		t.Fatal("applying no overrides must be a byte-identical no-op")
	}
}

func TestHashContentStable(t *testing.T) {
	content := readFixture(t)
	if hashContent(content) != hashContent(content) {
		t.Fatal("hash must be deterministic")
	}
	if hashContent(content) == hashContent(content+" ") {
		t.Fatal("hash must change when content changes")
	}
}

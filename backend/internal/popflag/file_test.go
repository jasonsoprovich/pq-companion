package popflag

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// exportText builds a Zeal PoPFlags export as outputfile.cpp writes it.
func exportText(char, stamp string, body ...string) string {
	return "Character\t" + char + "\nTimestamp\t" + stamp + "\nCommand\t#popflags, #popflags 1-5\n---\n" +
		strings.Join(body, "\n") + "\n"
}

var tier1Body = []string{
	"=== Planes of Power Progression ===",
	"Tier 1: Complete",
	"Tier 2: In progress",
	"=== Tier 3 Progression ===",
	"--- Halls of Honor ---",
	"Rydda`Dar trial: Complete",
	"Village trial: Incomplete",
	"Nomad trial: Incomplete",
}

func TestParsePopFlagsFile(t *testing.T) {
	data := []byte(exportText("Osui", "2026-11-15_18-30-05", tier1Body...))
	f, err := ParsePopFlagsFile(data, "/eq/Osui-PoPFlags.txt", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Character != "Osui" || len(f.Reports) != 2 {
		t.Fatalf("got %q with %d sections", f.Character, len(f.Reports))
	}
	if f.Reports[0].Section != PopFlagsOverview || f.Reports[1].Section != PopFlagsTier3 {
		t.Errorf("sections %q, %q", f.Reports[0].Section, f.Reports[1].Section)
	}
	if f.Reports[1].Exact["hohtrials"] != "100" {
		t.Errorf("hohtrials = %q", f.Reports[1].Exact["hohtrials"])
	}
	if f.SHA == "" || f.Raw == "" {
		t.Error("expected content hash and raw text")
	}
}

// Zeal stamps the file in local time with no zone, so it must be read as
// local time, not UTC.
func TestParsePopFlagsFile_LocalTimestamp(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	old := time.Local
	time.Local = la
	defer func() { time.Local = old }()
	f, err := ParsePopFlagsFile([]byte(exportText("Osui", "2026-11-15_18-30-05", tier1Body...)), "x", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 15, 18, 30, 5, 0, la)
	if !f.Timestamp.Equal(want) {
		t.Errorf("timestamp %v, want %v", f.Timestamp, want)
	}
}

// The format 1 name gives the character when the header lacks one; a file
// cut off before its sections, or before its separator, is rejected.
func TestParsePopFlagsFile_NamesAndTruncation(t *testing.T) {
	noChar := strings.Replace(exportText("Osui", "2026-11-15_18-30-05", tier1Body...), "Character\tOsui\n", "", 1)
	f, err := ParsePopFlagsFile([]byte(noChar), "/eq/Osui-PoPFlags_pq.proj.txt", time.Time{})
	if err != nil || f.Character != "Osui" {
		t.Errorf("format 1 name: %q %v", f.Character, err)
	}
	for name, text := range map[string]string{
		"header only":  "Character\tOsui\nTimestamp\t2026-11-15_18-30-05\n",
		"no sections":  exportText("Osui", "2026-11-15_18-30-05"),
		"no timestamp": "Character\tOsui\n---\n=== Tier 1 Progression ===\n",
	} {
		if _, err := ParsePopFlagsFile([]byte(text), "/eq/Osui-PoPFlags.txt", time.Time{}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A file changed within the settle window is left for the next poll.
func TestReadPopFlagsFile_Settling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Osui-PoPFlags.txt")
	if err := os.WriteFile(path, []byte(exportText("Osui", "2026-11-15_18-30-05", tier1Body...)), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if _, err := ReadPopFlagsFile(path, st.ModTime().Add(time.Second)); !errors.Is(err, ErrPopFlagsFileSettling) {
		t.Errorf("fresh file: err = %v", err)
	}
	if _, err := ReadPopFlagsFile(path, st.ModTime().Add(3*time.Second)); err != nil {
		t.Errorf("settled file: %v", err)
	}
}

func fileAt(char string, ts time.Time, reports ...PopFlagsReport) PopFlagsFile {
	raw := ts.String()
	for _, r := range reports {
		for k, v := range r.Exact {
			raw += k + "=" + v
		}
	}
	return PopFlagsFile{Character: char, Timestamp: ts, Reports: reports, Raw: raw, SHA: raw}
}

func exactReport(kv ...string) PopFlagsReport {
	r := newPopFlagsReport()
	for i := 0; i+1 < len(kv); i += 2 {
		r.Exact[kv[i]] = kv[i+1]
	}
	return r
}

func snapQ(t *testing.T, s *Store, char string) map[string]string {
	t.Helper()
	snap, err := s.GetSnapshot(char)
	if err != nil || snap == nil {
		t.Fatalf("snapshot: %v %v", snap, err)
	}
	return snap.Qglobals
}

// The same export is applied once; an older one is skipped; a rewrite at the
// same timestamp fills in what the first read lacked and lowers nothing.
func TestApplyPopFlagsFile_Watermark(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	t0 := time.Now().Add(time.Minute).Truncate(time.Second)
	first := fileAt(char, t0, exactReport("hohtrials", "100"))
	if ok, err := s.ApplyPopFlagsFile(first); !ok || err != nil {
		t.Fatalf("first: %v %v", ok, err)
	}
	if ok, _ := s.ApplyPopFlagsFile(first); ok {
		t.Error("the same export must apply once")
	}
	if ok, _ := s.ApplyPopFlagsFile(fileAt(char, t0.Add(-time.Hour), exactReport("hohtrials", "111"))); ok {
		t.Error("an older export must be skipped")
	}
	rewrite := fileAt(char, t0, exactReport("hohtrials", "010", "zeks", "2"))
	if ok, err := s.ApplyPopFlagsFile(rewrite); !ok || err != nil {
		t.Fatalf("rewrite: %v %v", ok, err)
	}
	q := snapQ(t, s, char)
	if q["hohtrials"] != "110" || q["zeks"] != "2" {
		t.Errorf("after rewrite: hohtrials=%q zeks=%q, want 110 and 2", q["hohtrials"], q["zeks"])
	}
}

// Evidence observed before a live grant can only add, in either order and
// across a restart: the grant survives, trials combine and the Zek branches
// join.
func TestStaleReportCannotRetractGrant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	const char = "Osui"
	now := time.Now().Truncate(time.Second)
	// A newer file, then a newer hail (Tallon, zeks 4).
	if ok, err := s.ApplyPopFlagsFile(fileAt(char, now.Add(-2*time.Minute), exactReport("hohtrials", "100"))); !ok || err != nil {
		t.Fatalf("file: %v %v", ok, err)
	}
	if _, err := s.ApplyHailGrant(char, "potac_tallon", now); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// A delayed log report, observed before the hail, that knows nothing of
	// Tallon (zeks 3, Vallon only) and shows a different trial.
	delayed := exactReport("zeks", "3", "hohtrials", "010")
	if _, err := s.ApplyPopFlagsReport(char, delayed, "late", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	q := snapQ(t, s, char)
	if q["zeks"] != "5" || q["hohtrials"] != "110" {
		t.Errorf("zeks=%q hohtrials=%q, want 5 (branches joined) and 110", q["zeks"], q["hohtrials"])
	}
	rows := doneByID(t, s, char)
	for _, id := range []string{"potac_tallon", "potac_vallon"} {
		if !rows[id].Done {
			t.Errorf("%s should be done", id)
		}
	}
	// A confirmed absence from the stale report clears nothing either.
	if _, err := s.ApplyPopFlagsReport(char, exactReport("zeks", ""), "late", now.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if q := snapQ(t, s, char); q["zeks"] != "5" {
		t.Errorf("zeks = %q after a stale absence, want 5", q["zeks"])
	}
	// taken_at follows the newest reading and never goes back: an even older
	// report leaves it where it is.
	snap, _ := s.GetSnapshot(char)
	if snap.TakenAt != now.Add(-30*time.Second).Unix() {
		t.Errorf("taken_at = %d, want the newest reading's", snap.TakenAt)
	}
	if _, err := s.ApplyPopFlagsReport(char, exactReport("mavuin", "1"), "older", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if snap2, _ := s.GetSnapshot(char); snap2.TakenAt != snap.TakenAt || snap2.RawText != snap.RawText {
		t.Errorf("an older report moved taken_at/raw text: %d %q", snap2.TakenAt, snap2.RawText)
	}
}

// A report observed after everything stored still applies exactly, so a real
// new reading can correct the snapshot.
func TestFreshReportStillExact(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	now := time.Now().Truncate(time.Second)
	if _, err := s.ApplyPopFlagsReport(char, exactReport("hohtrials", "110"), "a", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPopFlagsReport(char, exactReport("hohtrials", "100"), "b", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if q := snapQ(t, s, char); q["hohtrials"] != "100" {
		t.Errorf("hohtrials = %q, want the newer exact 100", q["hohtrials"])
	}
}

func TestMergeAdditive(t *testing.T) {
	for _, c := range []struct{ q, cur, in, want string }{
		{"hohtrials", "100", "010", "110"},
		{"sol_room", "10", "00101", "10101"},
		{"zeks", "3", "4", "5"},
		{"zeks", "4", "3", "5"},
		{"zeks", "5", "3", "5"},
		{"mavuin", "2", "3", "3"},
		{"mavuin", "3", "2", "3"},
		{"mavuin", "", "2", "2"},
		{"mavuin", "2", "", "2"},
		{"mavuin", "2", "x", "2"},
	} {
		if got := mergeAdditive(c.q, c.cur, c.in); got != c.want {
			t.Errorf("mergeAdditive(%s, %q, %q) = %q, want %q", c.q, c.cur, c.in, got, c.want)
		}
	}
}

// With Zeal's hide option off the same replies reach the log and the file
// (stamped a few seconds later). Either order ends in the same state.
func TestHideOffLogAndFileAgree(t *testing.T) {
	body := []string{
		"=== Tier 3 Progression ===",
		"--- Halls of Honor ---",
		"Rydda`Dar trial: Complete",
		"Village trial: Incomplete",
		"Nomad trial: Complete",
	}
	report := ParsePopFlagsReport(body)
	logAt := time.Now().Add(time.Minute).Truncate(time.Second)
	file := fileAt("Osui", logAt.Add(3*time.Second), report)

	results := make([]map[string]string, 2)
	for i, logFirst := range []bool{true, false} {
		s := openTempStore(t)
		if logFirst {
			s.ApplyPopFlagsReport("Osui", report, "log", logAt) //nolint:errcheck
			s.ApplyPopFlagsFile(file)                           //nolint:errcheck
		} else {
			s.ApplyPopFlagsFile(file)                           //nolint:errcheck
			s.ApplyPopFlagsReport("Osui", report, "log", logAt) //nolint:errcheck
		}
		results[i] = snapQ(t, s, "Osui")
	}
	if results[0]["hohtrials"] != "101" || results[1]["hohtrials"] != "101" {
		t.Errorf("hohtrials log-first %q, file-first %q, want 101 both", results[0]["hohtrials"], results[1]["hohtrials"])
	}
}

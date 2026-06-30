package zeal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findButton(mf *MacroFile, page, button int) *MacroButton {
	for i := range mf.Buttons {
		if mf.Buttons[i].Page == page && mf.Buttons[i].Button == button {
			return &mf.Buttons[i]
		}
	}
	return nil
}

func TestParseMacros(t *testing.T) {
	// Out-of-order buttons and a non-contiguous button (Line2 appears far from
	// Line1) — both real patterns from the EQ client.
	content := strings.Join([]string{
		"[HotButtons]",
		"Page1Button1=F16",
		"[Socials]",
		"Page2Button1Name=/targ1",
		"Page2Button1Color=0",
		"Page2Button1Line1=/target Zel Zoszh",
		"Page2Button6Name=Pet Assi",
		"Page2Button6Color=13",
		"Page2Button6Line1=/assist tippy",
		"Page2Button1Line2=/tag local ^s^", // non-contiguous: button1's 2nd line
		"Page2Button6Line3=/pet attack",    // gap: line3 with no line2
		"[InspectText]",
		"",
	}, "\n")

	path := writeTemp(t, "Tester_pq.proj.ini", content)
	mf, err := ParseMacros(path, "Tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Character != "Tester" {
		t.Errorf("character = %q, want Tester", mf.Character)
	}
	if len(mf.Buttons) != 2 {
		t.Fatalf("buttons = %d, want 2", len(mf.Buttons))
	}
	// Sorted page-then-button: button1 first.
	b1 := findButton(mf, 2, 1)
	if b1 == nil {
		t.Fatal("Page2Button1 missing")
	}
	if b1.Name != "/targ1" || b1.Color != 0 {
		t.Errorf("b1 name/color = %q/%d", b1.Name, b1.Color)
	}
	if b1.Lines[0] != "/target Zel Zoszh" || b1.Lines[1] != "/tag local ^s^" {
		t.Errorf("b1 lines = %#v", b1.Lines)
	}
	if len(b1.Lines) != MacroLineCount {
		t.Errorf("b1 line count = %d, want %d", len(b1.Lines), MacroLineCount)
	}

	b6 := findButton(mf, 2, 6)
	if b6 == nil {
		t.Fatal("Page2Button6 missing")
	}
	// Line3 set with no Line2 — the gap must be preserved by index.
	if b6.Lines[0] != "/assist tippy" || b6.Lines[1] != "" || b6.Lines[2] != "/pet attack" {
		t.Errorf("b6 lines = %#v", b6.Lines)
	}
}

func TestParseMacrosFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "TAKPv22", "Osui_pq.proj.ini")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	mf, err := ParseMacros(path, "Osui")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(mf.Buttons) == 0 {
		t.Fatal("expected macro buttons in fixture")
	}
	// Trailing whitespace in a command must be preserved verbatim.
	b := findButton(mf, 2, 2)
	if b == nil {
		t.Fatal("Page2Button2 missing in fixture")
	}
	if b.Lines[0] != "/target Zlakas  " {
		t.Errorf("trailing spaces not preserved: %q", b.Lines[0])
	}
}

// TestWriteMacrosPreservesOtherSections is the load-bearing test for the
// surgical writer: editing macros must not alter a single byte outside the
// [Socials] section of the live client config.
func TestWriteMacrosPreservesOtherSections(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "testdata", "TAKPv22", "Osui_pq.proj.ini")
	orig, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "Osui_pq.proj.ini")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	mf, err := ParseMacros(path, "Osui")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Mutate one macro and add a new one.
	if b := findButton(mf, 2, 1); b != nil {
		b.Name = "RENAMED"
		b.Lines[0] = "/target SomethingElse"
	}
	mf.Buttons = append(mf.Buttons, MacroButton{
		Page: 5, Button: 4, Name: "New One", Color: 7,
		Lines: []string{"/say hi", "", "", "", ""},
	})

	if err := WriteMacros(path, mf); err != nil {
		t.Fatalf("write: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}

	origSecs := splitSections(t, string(orig))
	newSecs := splitSections(t, string(after))
	for name, body := range origSecs {
		if name == macroSection {
			continue // expected to change
		}
		if newSecs[name] != body {
			t.Errorf("section [%s] changed:\n--- before ---\n%q\n--- after ---\n%q", name, body, newSecs[name])
		}
	}

	// The reparsed macros must reflect the edits.
	reparsed, err := ParseMacros(path, "Osui")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if b := findButton(reparsed, 2, 1); b == nil || b.Name != "RENAMED" || b.Lines[0] != "/target SomethingElse" {
		t.Errorf("edit not persisted: %+v", b)
	}
	if b := findButton(reparsed, 5, 4); b == nil || b.Name != "New One" || b.Color != 7 || b.Lines[0] != "/say hi" {
		t.Errorf("new button not persisted: %+v", b)
	}
}

func TestWriteMacrosRoundTrip(t *testing.T) {
	content := strings.Join([]string{
		"[Socials]",
		"Page1Button1Name=Test",
		"Page1Button1Color=3",
		"Page1Button1Line1=/say one",
		"Page1Button1Line3=/say three",
		"[InspectText]",
		"",
	}, "\n")
	path := writeTemp(t, "Tester_pq.proj.ini", content)

	mf, err := ParseMacros(path, "Tester")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := WriteMacros(path, mf); err != nil {
		t.Fatalf("write: %v", err)
	}
	reparsed, err := ParseMacros(path, "Tester")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	b := findButton(reparsed, 1, 1)
	if b == nil {
		t.Fatal("button lost on round trip")
	}
	if b.Lines[0] != "/say one" || b.Lines[1] != "" || b.Lines[2] != "/say three" {
		t.Errorf("line positions not preserved: %#v", b.Lines)
	}
}

func TestWriteMacrosAppendsSectionWhenMissing(t *testing.T) {
	content := "[Friends]\nFriend0=bob\n[InspectText]\n"
	path := writeTemp(t, "Tester_pq.proj.ini", content)

	mf := &MacroFile{
		Character: "Tester",
		Buttons: []MacroButton{
			{Page: 1, Button: 1, Name: "Hi", Color: 0, Lines: []string{"/say hi", "", "", "", ""}},
		},
	}
	if err := WriteMacros(path, mf); err != nil {
		t.Fatalf("write: %v", err)
	}
	reparsed, err := ParseMacros(path, "Tester")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if findButton(reparsed, 1, 1) == nil {
		t.Fatal("appended button not found")
	}
	// Friends section must be intact.
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "Friend0=bob") {
		t.Errorf("Friends section lost: %s", after)
	}
}

// TestWriteMacrosPreservesCRLF ensures the surgical writer keeps Windows CRLF
// line endings — the real client config is CRLF, and a silent LF rewrite would
// noisily churn the whole file and could confuse the client.
func TestWriteMacrosPreservesCRLF(t *testing.T) {
	content := "[Friends]\r\nFriend0=bob\r\n[Socials]\r\n" +
		"Page1Button1Name=Old\r\nPage1Button1Color=0\r\nPage1Button1Line1=/say hi\r\n" +
		"[InspectText]\r\n"
	path := writeTemp(t, "Tester_pq.proj.ini", content)

	mf, err := ParseMacros(path, "Tester")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b := findButton(mf, 1, 1); b != nil {
		b.Name = "New"
	}
	if err := WriteMacros(path, mf); err != nil {
		t.Fatalf("write: %v", err)
	}
	after, _ := os.ReadFile(path)
	s := string(after)
	if strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
		t.Errorf("found a bare LF — CRLF not preserved:\n%q", s)
	}
	if !strings.Contains(s, "Page1Button1Name=New\r\n") {
		t.Errorf("edited macro not written with CRLF:\n%q", s)
	}
	if !strings.Contains(s, "[Friends]\r\nFriend0=bob\r\n") {
		t.Errorf("Friends section CRLF not preserved:\n%q", s)
	}
}

// splitSections parses an INI string into a map of section name → body (the raw
// text between the header and the next header), for comparison in tests.
func splitSections(t *testing.T, content string) map[string]string {
	t.Helper()
	out := map[string]string{}
	cur := ""
	var b strings.Builder
	flush := func() {
		if cur != "" {
			out[cur] = b.String()
		}
		b.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		tr := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(tr, "[") && strings.HasSuffix(tr, "]") {
			flush()
			cur = tr[1 : len(tr)-1]
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	flush()
	return out
}

package zeal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MacroPath returns the EQ client config path that holds a character's socials.
// The client (and Zeal) read/write: <eq_path>/<CharName>_pq.proj.ini
func MacroPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s_pq.proj.ini", character))
}

const macroSection = "Socials"

// macroKeyRe matches a [Socials] macro key: Page<N>Button<M> followed by Name,
// Color, or Line<L>. Captures: 1=page, 2=button, 3=field, 4=line number (when
// the field is LineN).
var macroKeyRe = regexp.MustCompile(`^Page(\d+)Button(\d+)(Name|Color|Line(\d+))$`)

// ParseMacros reads the [Socials] section of a <CharName>_pq.proj.ini file and
// returns the macro buttons it defines. Keys may appear in any order and a
// single button's lines may be non-contiguous, so everything is collected by
// (page, button) and the result is sorted page-then-button for a stable view.
// Only buttons with a name or at least one command line are returned.
func ParseMacros(path, character string) (*MacroFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	out := &MacroFile{
		Character:  character,
		ExportedAt: info.ModTime(),
		Buttons:    []MacroButton{},
	}

	// Accumulate per button, keyed by page*1000+button.
	type acc struct {
		name    string
		color   int
		lines   []string
		page    int
		button  int
		present bool
	}
	byKey := map[int]*acc{}
	get := func(page, button int) *acc {
		k := page*1000 + button
		a := byKey[k]
		if a == nil {
			a = &acc{page: page, button: button, lines: make([]string, MacroLineCount)}
			byKey[k] = a
		}
		return a
	}

	inSection := false
	scanner := bufio.NewScanner(f)
	// Allow long macro lines; the default 64KB token size is plenty but be safe.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = strings.EqualFold(strings.TrimSpace(trimmed[1:len(trimmed)-1]), macroSection)
			continue
		}
		if !inSection {
			continue
		}
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(raw[:eq])
		// Preserve the value verbatim (trailing spaces in commands are
		// meaningful), minus the line ending the scanner already stripped.
		val := raw[eq+1:]

		m := macroKeyRe.FindStringSubmatch(key)
		if m == nil {
			continue // unknown key inside [Socials] — ignored on read
		}
		page, _ := strconv.Atoi(m[1])
		button, _ := strconv.Atoi(m[2])
		if page < 1 || page > MacroPageCount || button < 1 || button > MacroButtonsPerPage {
			continue
		}
		a := get(page, button)
		switch {
		case m[3] == "Name":
			a.name = val
			a.present = true
		case m[3] == "Color":
			a.color, _ = strconv.Atoi(strings.TrimSpace(val))
		default: // Line<N>
			ln, _ := strconv.Atoi(m[4])
			if ln >= 1 && ln <= MacroLineCount {
				a.lines[ln-1] = val
				if strings.TrimSpace(val) != "" {
					a.present = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	accs := make([]*acc, 0, len(byKey))
	for _, a := range byKey {
		if a.present {
			accs = append(accs, a)
		}
	}
	sort.Slice(accs, func(i, j int) bool {
		if accs[i].page != accs[j].page {
			return accs[i].page < accs[j].page
		}
		return accs[i].button < accs[j].button
	})
	for _, a := range accs {
		out.Buttons = append(out.Buttons, MacroButton{
			Page:   a.page,
			Button: a.button,
			Name:   a.name,
			Color:  a.color,
			Lines:  a.lines,
		})
	}
	return out, nil
}

// serializeMacroButtons renders the [Socials] macro key/value lines for a file,
// sorted page-then-button, joined with nl and terminated by nl. Only buttons
// with a name or a non-empty line are written; empty trailing lines are skipped
// but interior positions are preserved (Line3 stays Line3 even if Line2 blank).
func serializeMacroButtons(buttons []MacroButton, nl string) string {
	sorted := append([]MacroButton(nil), buttons...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Page != sorted[j].Page {
			return sorted[i].Page < sorted[j].Page
		}
		return sorted[i].Button < sorted[j].Button
	})

	var b strings.Builder
	for _, mb := range sorted {
		hasLine := false
		for _, l := range mb.Lines {
			if l != "" {
				hasLine = true
				break
			}
		}
		if mb.Name == "" && !hasLine {
			continue // not a real button
		}
		prefix := fmt.Sprintf("Page%dButton%d", mb.Page, mb.Button)
		fmt.Fprintf(&b, "%sName=%s%s", prefix, mb.Name, nl)
		fmt.Fprintf(&b, "%sColor=%d%s", prefix, mb.Color, nl)
		for i, l := range mb.Lines {
			if l != "" {
				fmt.Fprintf(&b, "%sLine%d=%s%s", prefix, i+1, l, nl)
			}
		}
	}
	return b.String()
}

// WriteMacros writes the macro buttons back into the [Socials] section of an
// existing <CharName>_pq.proj.ini, atomically (temp + rename). It is SURGICAL:
// only the [Socials] section's macro keys are replaced — every other byte of the
// file (other sections, the [Socials] header line itself, line endings, unknown
// keys) is preserved exactly. The file must already exist; this never creates a
// partial client config from scratch.
func WriteMacros(path string, mf *MacroFile) error {
	if mf == nil {
		return fmt.Errorf("nil macro file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err // includes "file does not exist" — we never fabricate the config
	}
	content := string(raw)

	// Preserve the file's existing line-ending style.
	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}

	body := serializeMacroButtons(mf.Buttons, nl)

	hdrStart, hdrEnd := findSectionHeader(content, macroSection)
	if hdrStart < 0 {
		// No [Socials] yet: append a fresh section, preserving all prior bytes.
		next := content
		if len(next) > 0 && !strings.HasSuffix(next, "\n") {
			next += nl
		}
		next += "[" + macroSection + "]" + nl + body
		return writeFileAtomic(path, next)
	}

	bodyStart := hdrEnd
	bodyEnd := findNextSectionHeader(content, bodyStart)
	if bodyEnd < 0 {
		bodyEnd = len(content)
	}

	// Preserve any non-macro lines that were inside the old [Socials] body
	// (blank lines, comments, keys we don't recognize), appended after the
	// freshly serialized macros.
	preserved := preserveNonMacroLines(content[bodyStart:bodyEnd], nl)

	next := content[:bodyStart] + body + preserved + content[bodyEnd:]
	return writeFileAtomic(path, next)
}

// findSectionHeader returns the byte offsets [start, afterNewline) of the
// "[name]" header line in content, or (-1, -1) if absent. start is the offset of
// the '['; afterNewline is the offset just past the header line's line ending
// (i.e. where the section body begins).
func findSectionHeader(content, name string) (int, int) {
	target := "[" + name + "]"
	for off := 0; off < len(content); {
		end := strings.IndexByte(content[off:], '\n')
		var line string
		var lineEnd int
		if end < 0 {
			line = content[off:]
			lineEnd = len(content)
		} else {
			line = content[off : off+end]
			lineEnd = off + end + 1 // include the '\n'
		}
		if strings.EqualFold(strings.TrimSpace(line), target) {
			return off, lineEnd
		}
		if end < 0 {
			break
		}
		off = lineEnd
	}
	return -1, -1
}

// findNextSectionHeader returns the byte offset of the next "[...]" section
// header at or after from, or -1 if there is none (body runs to EOF).
func findNextSectionHeader(content string, from int) int {
	for off := from; off < len(content); {
		end := strings.IndexByte(content[off:], '\n')
		var line string
		var lineEnd int
		if end < 0 {
			line = content[off:]
			lineEnd = len(content)
		} else {
			line = content[off : off+end]
			lineEnd = off + end + 1
		}
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			return off
		}
		if end < 0 {
			break
		}
		off = lineEnd
	}
	return -1
}

// preserveNonMacroLines returns the lines of an old [Socials] body that are NOT
// recognized macro keys (so unknown keys/comments survive a round-trip), each
// terminated by nl. Returns "" when there is nothing to preserve.
func preserveNonMacroLines(oldBody, nl string) string {
	var b strings.Builder
	for _, line := range strings.Split(oldBody, "\n") {
		trimmedNL := strings.TrimRight(line, "\r")
		t := strings.TrimSpace(trimmedNL)
		if t == "" {
			continue
		}
		eq := strings.IndexByte(trimmedNL, '=')
		if eq >= 0 {
			key := strings.TrimSpace(trimmedNL[:eq])
			if macroKeyRe.MatchString(key) {
				continue // a macro key — replaced by the fresh serialization
			}
		}
		b.WriteString(trimmedNL)
		b.WriteString(nl)
	}
	return b.String()
}

// writeFileAtomic writes content to path via a temp file + rename in the same
// directory, matching the durability pattern used by WriteSpellsets.
func writeFileAtomic(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".macros-*.ini")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

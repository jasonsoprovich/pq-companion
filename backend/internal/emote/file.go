package emote

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

const fileName = "spells_en.txt"

// newlineOf returns the dominant line ending used by content, so a rewrite
// matches the original file (EQ ships spells_en.txt with CRLF).
func newlineOf(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits content into lines, dropping a trailing \r from each so
// the caller works with bare text; the chosen newline is reapplied on join.
// A trailing blank element (from a final newline) is preserved so joining
// reproduces the original file exactly.
func splitLines(content string) []string {
	raw := strings.Split(content, "\n")
	for i := range raw {
		raw[i] = strings.TrimRight(raw[i], "\r")
	}
	return raw
}

// parseFields splits one line on '^'.
func parseFields(line string) []string {
	return strings.Split(line, "^")
}

// lineSpellID returns the line's leading spell id and true, or 0/false if
// the line doesn't start with an integer (blank/malformed lines pass through
// untouched).
func lineSpellID(fields []string) (int, bool) {
	if len(fields) == 0 {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return 0, false
	}
	return id, true
}

// lineEmoteText reads the five emote columns from a parsed line. Returns the
// zero value if the line is too short to have them.
func lineEmoteText(fields []string) EmoteText {
	if len(fields) < minEmoteFields {
		return EmoteText{}
	}
	return EmoteText{
		YouCast:     fields[idxYouCast],
		OtherCasts:  fields[idxOtherCasts],
		CastOnYou:   fields[idxCastOnYou],
		CastOnOther: fields[idxCastOnOther],
		SpellFades:  fields[idxSpellFades],
	}
}

// applyOverrides rewrites content, replacing the emote columns of any line
// whose spell id has a stored override. Only fields idxYouCast..idxSpellFades
// are ever touched, and only for overridden columns (nil entries in the
// override row leave that column untouched) — every other byte of every
// other line is preserved exactly, including the file's line ending and any
// trailing blank line.
func applyOverrides(content string, overrides map[int]OverrideRow) string {
	nl := newlineOf(content)
	lines := splitLines(content)
	for i, line := range lines {
		fields := parseFields(line)
		if len(fields) < minEmoteFields {
			continue
		}
		id, ok := lineSpellID(fields)
		if !ok {
			continue
		}
		ov, ok := overrides[id]
		if !ok {
			continue
		}
		ptrs := ov.fieldPtrs()
		idxs := [5]int{idxYouCast, idxOtherCasts, idxCastOnYou, idxCastOnOther, idxSpellFades}
		changed := false
		for j, p := range ptrs {
			if p != nil {
				fields[idxs[j]] = *p
				changed = true
			}
		}
		if changed {
			lines[i] = strings.Join(fields, "^")
		}
	}
	return strings.Join(lines, nl)
}

// findSpellLine returns the parsed fields for spellID's line in content, or
// nil if not found.
func findSpellLine(content string, spellID int) []string {
	for _, line := range splitLines(content) {
		fields := parseFields(line)
		if len(fields) < minEmoteFields {
			continue
		}
		if id, ok := lineSpellID(fields); ok && id == spellID {
			return fields
		}
	}
	return nil
}

// hashContent returns a stable content fingerprint used to tell the app's own
// writes apart from an external (server patch) overwrite.
func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

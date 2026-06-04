// Package eqconfig reads and writes the EverQuest client config files
// (eqclient.ini and zeal.ini). It is the only place in the app that MODIFIES
// game-owned config files, so all writes are line-preserving (comments,
// ordering, and CRLF endings are kept) and callers are expected to snapshot the
// file via the backup manager first.
package eqconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	eqClientINI = "eqclient.ini"
	zealINI     = "zeal.ini"

	logSection      = "Defaults" // [Defaults] Log=TRUE in eqclient.ini
	logKey          = "Log"
	zealSection     = "Zeal" // [Zeal] ExportOnCamp=true in zeal.ini
	exportOnCampKey = "ExportOnCamp"
)

// newlineOf returns the dominant line ending used by content so rewrites match
// the original file (EQ writes CRLF on Windows).
func newlineOf(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits content into lines, dropping a trailing \r from each so the
// caller works with bare text; the chosen newline is reapplied on join.
func splitLines(content string) []string {
	raw := strings.Split(content, "\n")
	for i := range raw {
		raw[i] = strings.TrimRight(raw[i], "\r")
	}
	return raw
}

// isTruthy accepts the boolean spellings EQ/Zeal emit.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// getINIValue returns the first non-comment value for key (case-insensitive),
// scanning the whole file regardless of section.
func getINIValue(content, key string) (string, bool) {
	for _, line := range splitLines(content) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(line[:eq]), key) {
			return strings.TrimSpace(line[eq+1:]), true
		}
	}
	return "", false
}

// setINIKey returns content with key set to value. If the key already exists
// anywhere (non-comment), its value is replaced in place to avoid creating a
// duplicate. Otherwise the key is inserted at the end of section, creating the
// section (appended) if it doesn't exist. Comments, ordering, and the original
// line ending are preserved.
func setINIKey(content, section, key, value string) string {
	nl := newlineOf(content)
	lines := splitLines(content)

	// 1. In-place replace wherever the key already lives.
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "[") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq >= 0 && strings.EqualFold(strings.TrimSpace(line[:eq]), key) {
			lines[i] = line[:eq+1] + value
			return strings.Join(lines, nl)
		}
	}

	// 2. Insert under the target section (or append the section).
	out := make([]string, 0, len(lines)+3)
	inTarget := false
	inserted := false
	sectionExists := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inTarget && !inserted {
				out = append(out, key+"="+value)
				inserted = true
			}
			sec := strings.TrimSpace(t[1 : len(t)-1])
			inTarget = strings.EqualFold(sec, section)
			if inTarget {
				sectionExists = true
			}
		}
		out = append(out, line)
	}
	if inTarget && !inserted { // target section ran to EOF
		out = append(out, key+"="+value)
		inserted = true
	}
	if !sectionExists {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, "["+section+"]", key+"="+value)
	}
	return strings.Join(out, nl)
}

// ── eqclient.ini logging ──────────────────────────────────────────────────────

// LogStatus describes the eqclient.ini Log setting. Found is false when the
// file or key is absent (treated as "unknown / not enabled").
type LogStatus struct {
	Found   bool `json:"found"`
	Enabled bool `json:"enabled"`
}

// ReadLog reports the eqclient.ini Log setting for the given EQ directory.
func ReadLog(eqPath string) LogStatus {
	if strings.TrimSpace(eqPath) == "" {
		return LogStatus{}
	}
	b, err := os.ReadFile(filepath.Join(eqPath, eqClientINI))
	if err != nil {
		return LogStatus{}
	}
	v, ok := getINIValue(string(b), logKey)
	if !ok {
		return LogStatus{}
	}
	return LogStatus{Found: true, Enabled: isTruthy(v)}
}

// SetLog writes the eqclient.ini Log setting (TRUE/FALSE) under [Defaults],
// preserving the rest of the file. eqclient.ini must already exist (it's
// created by the EQ client); a missing file is an error rather than silently
// creating a partial config.
func SetLog(eqPath string, enabled bool) error {
	return writeINI(filepath.Join(eqPath, eqClientINI), logSection, logKey, boolStr(enabled, "TRUE", "FALSE"))
}

// ── zeal.ini ExportOnCamp ──────────────────────────────────────────────────────

// SetExportOnCamp writes the zeal.ini ExportOnCamp setting under [Zeal]. The
// file is created if absent (Zeal tolerates a minimal config it later merges).
func SetExportOnCamp(eqPath string, enabled bool) error {
	path := filepath.Join(eqPath, zealINI)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Seed a minimal file so the toggle works before Zeal has written one.
		content := "[" + zealSection + "]\r\n" + exportOnCampKey + "=" + boolStr(enabled, "true", "false") + "\r\n"
		return os.WriteFile(path, []byte(content), 0o644)
	}
	return writeINI(path, zealSection, exportOnCampKey, boolStr(enabled, "true", "false"))
}

// writeINI reads path, sets the key, and writes it back (file must exist).
func writeINI(path, section, key, value string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	updated := setINIKey(string(b), section, key, value)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

package zeal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ZealINIFilename is the configuration file Zeal writes next to eqgame.exe.
// All settings exposed by Zeal's in-game UI persist here as plain INI lines,
// and pq-companion reads a small handful (currently just ExportOnCamp) to
// warn the user when a critical setting is disabled.
const ZealINIFilename = "zeal.ini"

// ExportOnCampStatus describes whether Zeal's ExportOnCamp setting is enabled.
// When the file is missing or the key is absent we report Found=false rather
// than guessing — callers treat that as "unknown" and stay quiet rather than
// false-alarming on a fresh install where Zeal hasn't written its config yet.
type ExportOnCampStatus struct {
	Found   bool
	Enabled bool
}

// ReadExportOnCamp scans <eqPath>/zeal.ini for the ExportOnCamp= line under
// the [Zeal] section. We do a forgiving scan (case-insensitive key match,
// trim spaces around `=`, accept TRUE/True/true/1) because Zeal writes the
// value with mixed casing depending on which code path saved it.
func ReadExportOnCamp(eqPath string) ExportOnCampStatus {
	if strings.TrimSpace(eqPath) == "" {
		return ExportOnCampStatus{}
	}
	path := filepath.Join(eqPath, ZealINIFilename)
	f, err := os.Open(path)
	if err != nil {
		return ExportOnCampStatus{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Zeal.ini lines are short; the default 64 KiB buffer is plenty.
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		// We don't enforce that the key lives under [Zeal] specifically —
		// the file's other sections are per-character ([Osui], [Nariana])
		// and don't define ExportOnCamp, so a first-match scan is safe.
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !strings.EqualFold(key, "ExportOnCamp") {
			continue
		}
		val := strings.TrimSpace(line[eq+1:])
		return ExportOnCampStatus{Found: true, Enabled: isTruthyINI(val)}
	}
	return ExportOnCampStatus{}
}

// isTruthyINI accepts the various boolean spellings Zeal emits. Anything
// other than a recognized true value (including blank) reads as false.
func isTruthyINI(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

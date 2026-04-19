package logparser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredCharacter describes a character found on disk via their EQ log file.
type DiscoveredCharacter struct {
	// Name is the character name (no server suffix).
	Name string `json:"name"`
	// ModTime is the Unix seconds timestamp of the log file's last modification.
	// Used by clients to present "most recent" first.
	ModTime int64 `json:"mod_time"`
}

// DiscoverCharacters scans eqPath for eqlog_*_pq.proj.txt files and returns
// one entry per character, sorted by most-recently-modified first.
// Returns an empty slice if eqPath is empty or no log files are found.
func DiscoverCharacters(eqPath string) []DiscoveredCharacter {
	if eqPath == "" {
		return []DiscoveredCharacter{}
	}
	pattern := filepath.Join(eqPath, "eqlog_*_pq.proj.txt")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return []DiscoveredCharacter{}
	}

	out := make([]DiscoveredCharacter, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		base := filepath.Base(path)
		name := strings.TrimPrefix(base, "eqlog_")
		name = strings.TrimSuffix(name, "_pq.proj.txt")
		if name == "" {
			continue
		}
		out = append(out, DiscoveredCharacter{
			Name:    name,
			ModTime: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime > out[j].ModTime
	})
	return out
}

// ResolveActiveCharacter returns the character name from the most recently
// modified eqlog_*_pq.proj.txt file in eqPath, or "" if none exist.
func ResolveActiveCharacter(eqPath string) string {
	chars := DiscoverCharacters(eqPath)
	if len(chars) == 0 {
		return ""
	}
	return chars[0].Name
}

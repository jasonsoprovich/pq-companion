// Package changelog parses CHANGELOG.md into structured version entries for
// the /api/changelog endpoint, which feeds the in-app "what's new" popup and
// the Settings > Changelog tab.
package changelog

import (
	"os"
	"regexp"
	"strings"
)

// Entry is one "## vX.Y.Z — YYYY-MM-DD" section of CHANGELOG.md.
type Entry struct {
	Version string `json:"version"` // without the leading "v", e.g. "0.17.6"
	Date    string `json:"date"`    // YYYY-MM-DD
	Body    string `json:"body"`    // raw markdown between this header and the next
}

// headerRe matches a release header line, e.g. "## v0.17.6 — 2026-07-19".
var headerRe = regexp.MustCompile(`(?m)^## v(\S+) — (\S+)\s*$`)

// Load reads and parses the changelog file at path. A missing file returns
// an empty slice rather than an error — the popup and Settings tab both
// treat "no entries" as "nothing to show" instead of a hard failure, since
// this data is presentational, never load-bearing for the rest of the app.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return Parse(string(data)), nil
}

// Parse splits raw changelog markdown into version entries, preserving the
// file's own newest-first ordering.
func Parse(md string) []Entry {
	locs := headerRe.FindAllStringSubmatchIndex(md, -1)
	entries := make([]Entry, 0, len(locs))
	for i, loc := range locs {
		bodyEnd := len(md)
		if i+1 < len(locs) {
			bodyEnd = locs[i+1][0]
		}
		entries = append(entries, Entry{
			Version: md[loc[2]:loc[3]],
			Date:    md[loc[4]:loc[5]],
			Body:    strings.TrimSpace(md[loc[1]:bodyEnd]),
		})
	}
	return entries
}

package popflag

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Zeal's PoP flags export (CoastalRedwood/Zeal #253): '/outputfile popflags',
// or automatically on camp with "Export data on /camp" and the PoP flags
// option, sends '#popflags' and '#popflags 1'..'5' and writes the replies to
// <Char>-PoPFlags.txt in the EQ folder (<Char>-PoPFlags_pq.proj.txt with
// export format 1):
//
//	Character	Osui
//	Timestamp	2026-11-15_18-30-00
//	Command	#popflags, #popflags 1-5
//	---
//	=== Planes of Power Progression ===
//	...
//
// With Zeal's "hide popflags" option on, those replies never reach the chat
// log, so this file is the only place PQ Companion can read them from.

// popFlagsFileRe matches an export's file name in either format.
var popFlagsFileRe = regexp.MustCompile(`(?i)^(.+?)-PoPFlags(?:_pq\.proj)?\.txt$`)

// PopFlagsFileGlob finds every character's export in the EQ folder.
const PopFlagsFileGlob = "*-PoPFlags*.txt"

// popFlagsFileTimeLayout is Zeal's generateTimestamp(): local time, no zone.
const popFlagsFileTimeLayout = "2006-01-02_15-04-05"

// popFlagsFileSettle is how long the file must have been left alone before
// it's read: Zeal writes it with a plain ofstream, so a fresh file may still
// be mid-write.
const popFlagsFileSettle = 2 * time.Second

// ErrPopFlagsFileSettling means the file changed too recently to read yet;
// try again on the next poll.
var ErrPopFlagsFileSettling = errors.New("popflag: export file still being written")

// PopFlagsFile is one parsed export.
type PopFlagsFile struct {
	Character string
	Timestamp time.Time
	Reports   []PopFlagsReport
	Raw       string
	SHA       string
}

// ParsePopFlagsFile parses an export. It's rejected unless it has the
// Character and Timestamp header lines, the '---' separator and at least one
// '#popflags' section. A Timestamp that doesn't parse falls back to mtime.
func ParsePopFlagsFile(data []byte, name string, mtime time.Time) (PopFlagsFile, error) {
	f := PopFlagsFile{Raw: string(data)}
	sum := sha256.Sum256(data)
	f.SHA = hex.EncodeToString(sum[:])

	var header = true
	var block []string
	var stamp string
	flush := func() {
		if len(block) > 0 {
			f.Reports = append(f.Reports, ParsePopFlagsReport(block))
		}
		block = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sawSeparator := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if header {
			if line == "---" {
				header, sawSeparator = false, true
				continue
			}
			if k, v, ok := strings.Cut(line, "\t"); ok {
				switch k {
				case "Character":
					f.Character = strings.TrimSpace(v)
				case "Timestamp":
					stamp = strings.TrimSpace(v)
				}
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if _, ok := popFlagsHeaders[trimmed]; ok {
			flush()
		}
		if trimmed == "" || (len(block) == 0 && !isPopFlagsHeader(trimmed)) {
			continue // nothing before the first section header counts
		}
		block = append(block, trimmed)
	}
	if err := sc.Err(); err != nil {
		return f, fmt.Errorf("popflag: read export: %w", err)
	}
	flush()

	if f.Character == "" {
		if m := popFlagsFileRe.FindStringSubmatch(filepath.Base(name)); m != nil {
			f.Character = m[1]
		}
	}
	switch {
	case !sawSeparator || stamp == "" || f.Character == "":
		return f, fmt.Errorf("popflag: export %s has no complete header", filepath.Base(name))
	case len(f.Reports) == 0:
		return f, fmt.Errorf("popflag: export %s has no #popflags sections", filepath.Base(name))
	}
	if ts, err := time.ParseInLocation(popFlagsFileTimeLayout, stamp, time.Local); err == nil {
		f.Timestamp = ts
	} else {
		f.Timestamp = mtime
	}
	return f, nil
}

func isPopFlagsHeader(line string) bool {
	_, ok := popFlagsHeaders[line]
	return ok
}

// ReadPopFlagsFile reads and parses the export at path, refusing one changed
// within popFlagsFileSettle of now.
func ReadPopFlagsFile(path string, now time.Time) (PopFlagsFile, error) {
	st, err := os.Stat(path)
	if err != nil {
		return PopFlagsFile{}, err
	}
	if now.Sub(st.ModTime()) < popFlagsFileSettle {
		return PopFlagsFile{}, ErrPopFlagsFileSettling
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PopFlagsFile{}, err
	}
	return ParsePopFlagsFile(data, path, st.ModTime())
}

// ApplyPopFlagsFile applies every section of an export in one transaction,
// all or nothing, and records it as the character's last applied export.
//
//   - An export older than the last one applied is skipped, and so is the
//     same export again (same timestamp and content).
//   - The same timestamp with different content (a complete rewrite of a
//     file first read part-written) is applied additively: it fills in what
//     was missing and never lowers anything.
//   - Otherwise it applies like an in-game '#popflags' report, additively if
//     it's older than the stored reading or the last live grant.
//
// Reports whether it was applied.
func (s *Store) ApplyPopFlagsFile(f PopFlagsFile) (bool, error) {
	if f.Character == "" || len(f.Reports) == 0 {
		return false, fmt.Errorf("popflag: empty export")
	}
	at := f.Timestamp.Unix()
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastTS int64
	var lastSHA string
	err := s.db.QueryRow(`SELECT file_ts, content_sha FROM pop_flag_file_import WHERE character = ?`, f.Character).
		Scan(&lastTS, &lastSHA)
	seen := err == nil
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("load export watermark for %q: %w", f.Character, err)
	}
	if seen && (at < lastTS || (at == lastTS && f.SHA == lastSHA)) {
		return false, nil
	}
	prev, stale, err := s.staleAgainstLocked(f.Character, at)
	if err != nil {
		return false, err
	}
	additive := stale || (seen && at == lastTS)
	mark := func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO pop_flag_file_import (character, file_ts, content_sha) VALUES (?, ?, ?)
			ON CONFLICT(character) DO UPDATE SET file_ts = excluded.file_ts, content_sha = excluded.content_sha
		`, f.Character, at, f.SHA); err != nil {
			return fmt.Errorf("record export watermark for %q: %w", f.Character, err)
		}
		return nil
	}
	if _, err := s.applyMergedLocked(f.Character, SourcePopflags, prev, f.Reports, f.Raw, at, additive, mark); err != nil {
		return false, err
	}
	return true, nil
}

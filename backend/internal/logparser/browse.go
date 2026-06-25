package logparser

import (
	"bytes"
	"context"
	"os"
	"strings"
)

// browseChunk is how many bytes are read per backward step when browsing a
// log file. EQ log lines are short, so a 64 KiB chunk holds hundreds of lines
// — the common "show me the tail" case finishes in a single read.
const browseChunk = 64 * 1024

// BrowseLine is one line returned by BrowseLines: the parsed event plus the
// byte offset of the line's start in the file. The offset is the pagination
// cursor — pass the oldest line's offset back as beforeOffset to page older.
type BrowseLine struct {
	LogEvent
	Offset int64 `json:"offset"`
}

// BrowseResult is a page of browsed log lines, newest-first.
type BrowseResult struct {
	Lines []BrowseLine `json:"lines"`
	// NextOffset is the beforeOffset to pass for the next (older) page, or nil
	// when the start of the file has been reached.
	NextOffset *int64 `json:"next_offset"`
}

// BrowseLines returns up to limit log lines from path, newest-first, ending
// strictly before beforeOffset (pass 0 or a value >= file size to start at the
// end). query filters case-insensitively on the line message; eventType, when
// non-empty, keeps only lines whose classified type matches exactly (use
// "log:raw" for lines that match no known pattern).
//
// The file is read strictly read-only, backward in chunks. With no filter the
// scan stops as soon as limit lines are collected — typically a single chunk —
// so a multi-hundred-MB log costs only a few KB of I/O for the recent tail. A
// filter that matches sparsely may scan further, up to the whole file; ctx is
// checked between chunks so an abandoned request (the user kept typing) stops
// scanning instead of running the whole file to completion.
func BrowseLines(ctx context.Context, path string, beforeOffset int64, query, eventType string, limit int) (*BrowseResult, error) {
	if limit <= 0 {
		limit = 300
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()

	hi := beforeOffset
	if hi <= 0 || hi > size {
		hi = size
	}
	q := strings.ToLower(strings.TrimSpace(query))

	res := &BrowseResult{Lines: make([]BrowseLine, 0, limit)}

	// partial holds the leading (lowest-offset) line fragment carried from the
	// previous chunk: its true start is below the current read position, so it
	// can't be finalized until we read further down (or reach offset 0).
	var partial []byte
	lo := hi

	// keep emits a line into the result. Returns true once the page is full.
	keep := func(off int64, raw string) bool {
		text := strings.TrimRight(raw, "\r")
		ts, msg, ok := ParseRawLine(text)
		if !ok {
			return false // no EQ timestamp — continuation/garbage, skip
		}
		// Apply the cheap substring filter before the expensive classifier.
		// A text search over a multi-hundred-thousand-line file otherwise
		// pays full ~45-regex classification on every line, including the
		// vast majority that can't match the query at all.
		if q != "" && !strings.Contains(strings.ToLower(msg), q) {
			return false
		}
		ev, classified := ClassifyMessage(msg)
		if !classified {
			ev = LogEvent{Type: EventRaw}
		}
		ev.Timestamp = ts
		ev.Message = msg
		if eventType != "" && string(ev.Type) != eventType {
			return false
		}
		res.Lines = append(res.Lines, BrowseLine{LogEvent: ev, Offset: off})
		return len(res.Lines) >= limit
	}

	full := false
	for lo > 0 {
		// Stop scanning if the caller has gone away (e.g. the user kept
		// typing and this search was superseded). Without this an abandoned
		// filtered request keeps churning the whole file, and several stack
		// up under fast typing.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunkSize := int64(browseChunk)
		if chunkSize > lo {
			chunkSize = lo
		}
		start := lo - chunkSize
		buf := make([]byte, chunkSize)
		if _, err := f.ReadAt(buf, start); err != nil {
			return nil, err
		}
		buf = append(buf, partial...) // covers [start, prevTop)

		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			// No line boundary yet — the whole buffer is one continuing line.
			partial = buf
			lo = start
			continue
		}
		// buf[:i] is the new leading partial (its start is below `start`,
		// unless start == 0, handled after the loop). Everything after the
		// first newline is complete lines with known offsets.
		region := buf[i+1:]
		regionBase := start + int64(i+1)

		// Collect complete lines with offsets, then emit newest-first.
		type ln struct {
			off int64
			s   string
		}
		var lines []ln
		off := regionBase
		for len(region) > 0 {
			j := bytes.IndexByte(region, '\n')
			if j < 0 {
				lines = append(lines, ln{off, string(region)})
				break
			}
			lines = append(lines, ln{off, string(region[:j])})
			off += int64(j) + 1
			region = region[j+1:]
		}
		for k := len(lines) - 1; k >= 0; k-- {
			if keep(lines[k].off, lines[k].s) {
				full = true
				break
			}
		}

		partial = buf[:i]
		lo = start
		if full {
			break
		}
	}

	// Reached the start of the file: the carried partial is a complete line at
	// offset 0.
	if !full && lo == 0 && len(partial) > 0 {
		full = keep(0, string(partial))
	}

	// More lines may remain below the oldest one returned. Offset 0 is the
	// start of the file, so there is nothing older — and 0 doubles as the
	// "start at end" sentinel for beforeOffset, so never hand it back.
	if full {
		if next := res.Lines[len(res.Lines)-1].Offset; next > 0 {
			res.NextOffset = &next
		}
	}
	return res, nil
}

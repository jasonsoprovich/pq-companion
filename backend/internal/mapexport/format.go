// Package mapexport writes our POIs into Zeal's external map files, so the
// markers show on the in-game map rather than only in the app.
//
// This is Grokii's original request fully realised, and it is a different thing
// from the "Pin in game" button: that copies one marker at a time through the
// clipboard and lasts until the map is cleared, whereas this writes every marker
// for a zone once and they persist.
//
// Only P (point) lines are emitted, never L (line) geometry. In `data_mode both`
// Zeal draws its own internal geometry *and* the external file, so shipping
// geometry would draw a second copy of the zone outline on top of the one
// already there, for no gain — and it would expose us to whatever the renderer's
// line limit turns out to be. Markers are the part Zeal does not already have.
package mapexport

import (
	"fmt"
	"strings"
	"unicode"
)

// Colours per category, matching the app's own palette so a marker means the
// same thing on both maps. RGB is cast to uint8 by Zeal's parser, so these must
// stay in 0-255.
var categoryColor = map[string][3]int{
	"trap":         {245, 158, 11},
	"locked":       {251, 113, 133},
	"teleport":     {34, 211, 238},
	"switch":       {163, 230, 53},
	"vendor":       {201, 168, 76},
	"raid_target":  {239, 68, 68},
	"zone_line":    {34, 197, 94},
	"succor":       {94, 234, 212},
	"ground_spawn": {59, 130, 246},
	"tradeskill":   {167, 139, 250},
	"door":         {148, 163, 184},
	"wall":         {244, 114, 182},
	"hazard":       {220, 38, 38},
	"note":         {250, 204, 21},
}

// DefaultCategories are the ones exported unless the caller says otherwise:
// what the in-game map does not already show.
//
// Vendors, zone lines and raid targets are deliberately absent by default —
// existing map packs mark those, so exporting them adds clutter on top of
// information the player already has. Traps, locked doors, in-zone teleports and
// hand-placed markers are the categories no map set carries.
var DefaultCategories = []string{
	"trap", "locked", "teleport", "switch", "wall", "hazard", "note",
}

// labelMaxBytes is Zeal's label buffer minus the null terminator
// (char buffer[64] with buffer[63] forced to 0).
const labelMaxBytes = 63

// Point is one marker to write.
type Point struct {
	// X, Y, Z are map-space coordinates — the same convention the external file
	// format uses, so no transform is applied here.
	X, Y, Z  int
	Category string
	Label    string
}

// sanitizeLabel makes a label safe for Zeal's parser.
//
// The parser reads the label with %s, which stops at the first whitespace, so a
// label with spaces would be silently truncated at the first one — "Locked"
// instead of "Locked — needs Golden Crescent Key". The external format's
// convention is underscores for spaces, which is why every existing map pack
// looks like that.
//
// Non-ASCII is transliterated rather than passed through: the buffer is bytes,
// and an em dash costs three of them while rendering as mojibake in the EQ
// client's font.
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '—' || r == '–':
			b.WriteByte('-')
		case r == '’' || r == '\'':
			b.WriteByte('\'')
		case r == ' ' || r == '\t':
			b.WriteByte('_')
		case r == ',':
			// Commas separate fields. One inside a label would not break the
			// parse (the label is last) but would read as a field boundary to
			// anything else reading the file, including us.
			b.WriteByte(';')
		case r < 128 && (unicode.IsPrint(r)):
			b.WriteRune(r)
		default:
			// Anything else: drop rather than emit a byte the client cannot draw.
		}
	}
	out := b.String()
	if len(out) > labelMaxBytes {
		// Trim on a byte boundary — everything here is ASCII by now.
		out = out[:labelMaxBytes]
	}
	if out == "" {
		out = "marker"
	}
	return out
}

// FormatFile renders points as a Zeal external map file.
//
// Returns an empty string when there is nothing to write, so callers can skip
// the zone rather than leave an empty file behind — an empty base file parses to
// zero entries, and a base file that fails to load disables external data for
// the whole zone.
func FormatFile(points []Point) string {
	var b strings.Builder
	for _, p := range points {
		c, ok := categoryColor[p.Category]
		if !ok {
			c = [3]int{200, 200, 200}
		}
		// P x, y, z, r, g, b, size, label — matching add_map_data_from_file's
		// "P %f, %f, %f, %u, %u, %u, %i, %s".
		fmt.Fprintf(&b, "P %d.0000, %d.0000, %d.0000, %d, %d, %d, 2, %s\n",
			p.X, p.Y, p.Z, c[0], c[1], c[2], sanitizeLabel(p.Label))
	}
	return b.String()
}

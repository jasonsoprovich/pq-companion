// Package mapfiles reads classic EverQuest .txt map packs from the player's own
// game directory.
//
// This is a *read* path, deliberately. Packs like Brewall's are published with
// no licence, no copyright statement and no redistribution terms
// (docs/maps-feasibility.md §5.1), which makes shipping them the most exposed
// possible position and is why the app builds its own maps.db instead. Reading
// files a user chose to install is a different act entirely: nothing of anyone
// else's is copied, bundled, or served — the app renders what is already on the
// player's disk, the same way the game client does.
//
// So this adds a third render mode that appears only when such a pack is
// detected, and disappears with it.
package mapfiles

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Segment is one drawn line, in map space with the pack's own colour.
//
// No coordinate transform is applied or needed: the .txt format stores
// f1 = -game_x, f2 = -game_y, z unchanged, which is exactly the map space the
// renderer already works in. Verified empirically against three landmarks in
// qeynos2 (docs/maps-feasibility.md §3).
type Segment struct {
	X1, Y1, Z1 int
	X2, Y2, Z2 int
	R, G, B    uint8
}

// Point is one labelled marker from a pack.
type Point struct {
	X, Y, Z int
	R, G, B uint8
	Label   string
}

// Zone is one zone's drawable content, gathered across its layer files.
type Zone struct {
	Segments []Segment
	Points   []Point
}

// candidateDirs are where a pack is looked for, relative to the EQ directory,
// in the order they are tried.
//
// The published instructions ("create a folder called Brewall under
// Everquest's map folder") describe the modern client, which has a map-pack
// dropdown. Project Quarm's client has no such thing — its external map data
// goes through Zeal's map_files. So both conventions are searched, plus the
// bare maps folder, because a user following any of the guides out there will
// have landed in one of them.
var candidateDirs = []string{
	filepath.Join("maps", "Brewall"),
	"maps",
	"map_files",
}

// Pack is a detected map pack.
type Pack struct {
	// Dir is the absolute directory the files were found in.
	Dir string
	// Name is what to call it in the UI, taken from the directory name when it
	// is meaningful ("Brewall") and generic otherwise.
	Name string
	// Zones is the set of zone short names with at least one file.
	Zones map[string]bool
}

// packMinZones is how many zones a directory needs before it counts as a pack.
//
// Guards against pointing at a folder that merely happens to contain a .txt
// file — including, importantly, the map_files directory this app writes its
// own marker exports into, which holds a handful of files and is not a pack.
const packMinZones = 25

// Detect finds an installed map pack under eqPath, or returns nil.
func Detect(eqPath string) *Pack {
	if eqPath == "" {
		return nil
	}
	for _, rel := range candidateDirs {
		dir := filepath.Join(eqPath, rel)
		zones := scanZones(dir)
		if len(zones) < packMinZones {
			continue
		}
		return &Pack{Dir: dir, Name: packName(dir), Zones: zones}
	}
	return nil
}

// packName labels the pack by its folder, since that is the only self-
// description these packs carry. A folder called "maps" says nothing, so it
// gets a generic name rather than a guessed attribution — crediting the wrong
// author would be worse than crediting none.
func packName(dir string) string {
	base := filepath.Base(dir)
	switch strings.ToLower(base) {
	case "maps", "map_files", "":
		return "Map pack"
	default:
		return base
	}
}

// scanZones lists the zone short names present in a directory.
//
// A zone is any <name>.txt; the numbered <name>_1.txt files are its extra
// layers and are folded into the same zone rather than counted separately.
func scanZones(dir string) map[string]bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	zones := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".txt") {
			continue
		}
		base := name[:len(name)-4]
		if i := strings.LastIndex(base, "_"); i > 0 {
			if _, err := strconv.Atoi(base[i+1:]); err == nil {
				base = base[:i]
			}
		}
		zones[strings.ToLower(base)] = true
	}
	return zones
}

// maxLayer is how many numbered layer files are read past the base file.
//
// The client itself reads a contiguous chain, and packs in the wild use up to
// three or four. Ten is well past anything observed and bounds the work.
const maxLayer = 10

// Load reads one zone's files.
//
// Every layer is read and merged rather than being assigned a fixed meaning.
// Layer numbering is a per-zone convention, not a standard: in Brewall's set
// oasis_1 is labels and oasis_2 is the legend block, but nothing guarantees
// that ordering elsewhere, so inferring roles from the number would misread
// other packs.
func (p *Pack) Load(zone string) (*Zone, error) {
	zone = strings.ToLower(zone)
	if !p.Zones[zone] {
		return nil, nil
	}
	out := &Zone{Segments: []Segment{}, Points: []Point{}}
	names := []string{zone + ".txt"}
	for i := 1; i <= maxLayer; i++ {
		names = append(names, fmt.Sprintf("%s_%d.txt", zone, i))
	}
	for _, name := range names {
		f, err := os.Open(filepath.Join(p.Dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		err = parseInto(f, out)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
	}
	return out, nil
}

// parseInto reads the two line types the format defines.
//
//	L x1, y1, z1, x2, y2, z2, R, G, B
//	P x,  y,  z,  R, G, B, size, label
//
// Malformed lines are skipped rather than failing the file. These are
// third-party text files edited by hand over two decades; one bad line in a
// zone should cost that line, not the map.
func parseInto(r *os.File, out *Zone) error {
	sc := bufio.NewScanner(r)
	// Some packs carry very long label lines; the default 64KB token limit is
	// ample but the buffer must be set explicitly to grow at all.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) < 2 {
			continue
		}
		kind := line[0]
		if kind != 'L' && kind != 'P' {
			continue
		}
		f := strings.Split(line[1:], ",")
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		switch kind {
		case 'L':
			if len(f) < 9 {
				continue
			}
			n, ok := numbers(f[:6])
			if !ok {
				continue
			}
			c, ok := colour(f[6:9])
			if !ok {
				continue
			}
			out.Segments = append(out.Segments, Segment{
				X1: n[0], Y1: n[1], Z1: n[2], X2: n[3], Y2: n[4], Z2: n[5],
				R: c[0], G: c[1], B: c[2],
			})
		case 'P':
			if len(f) < 8 {
				continue
			}
			n, ok := numbers(f[:3])
			if !ok {
				continue
			}
			c, ok := colour(f[3:6])
			if !ok {
				continue
			}
			// The label is the last field and may itself contain commas in a
			// malformed file, so take everything from field 7 on.
			label := strings.Join(f[7:], ",")
			out.Points = append(out.Points, Point{
				X: n[0], Y: n[1], Z: n[2],
				R: c[0], G: c[1], B: c[2],
				// Underscores are the format's spaces.
				Label: strings.ReplaceAll(label, "_", " "),
			})
		}
	}
	return sc.Err()
}

// coordLimit matches the renderer's int16 packing. A value past this is
// corrupt rather than distant, and would wrap to the opposite side of the map.
const coordLimit = 32767

func numbers(f []string) ([]int, bool) {
	out := make([]int, len(f))
	for i, s := range f {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v > coordLimit || v < -coordLimit {
			return nil, false
		}
		out[i] = int(v)
	}
	return out, true
}

func colour(f []string) ([3]uint8, bool) {
	var out [3]uint8
	for i, s := range f {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 || v > 255 {
			return out, false
		}
		out[i] = uint8(v)
	}
	return out, true
}

package mapexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// MapFilesDir is the directory Zeal reads, relative to the EQ install root.
const MapFilesDir = "map_files"

// maxOptionalSlot is Zeal's cap: <zone>.txt plus _1 through _10.
const maxOptionalSlot = 10

// Manifest records exactly which files we wrote and what they contained.
//
// Kept in the app's own directory rather than inside map_files, because the file
// format permits no comments — a marker line would fail the parse, and a base
// file that fails to parse disables external map data for the whole zone. So
// ownership cannot be recorded in-band.
//
// The hash is what makes removal and re-export safe: it distinguishes "this is
// still the file we wrote" from "something replaced it", and we only ever
// overwrite or delete the former.
type Manifest struct {
	Files map[string]FileRecord `json:"files"`
}

// FileRecord is one written file, keyed in the manifest by its path relative to
// map_files.
type FileRecord struct {
	Zone   string `json:"zone"`
	SHA256 string `json:"sha256"`
	Points int    `json:"points"`
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadManifest reads the manifest, treating a missing one as empty.
func LoadManifest(path string) (*Manifest, error) {
	m := &Manifest{Files: map[string]FileRecord{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(b, m); err != nil {
		// A corrupt manifest must not authorise deleting anything. Start empty:
		// the worst case is orphaned files we no longer claim, which is far
		// better than deleting a map pack we never wrote.
		return &Manifest{Files: map[string]FileRecord{}}, nil
	}
	if m.Files == nil {
		m.Files = map[string]FileRecord{}
	}
	return m, nil
}

// Save writes the manifest.
func (m *Manifest) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// ownsFile reports whether a path on disk is one we wrote and still matches.
func (m *Manifest) ownsFile(mapDir, rel string) bool {
	rec, ok := m.Files[rel]
	if !ok {
		return false
	}
	b, err := os.ReadFile(filepath.Join(mapDir, rel))
	if err != nil {
		return false
	}
	return hashOf(b) == rec.SHA256
}

// Plan is what an export would do to one zone, decided before anything is
// written so the caller can show it and the user can decline.
type Plan struct {
	Zone   string `json:"zone"`
	File   string `json:"file"`
	Points int    `json:"points"`
	// Action is "create", "replace" (a file we wrote and still own) or "skip".
	Action string `json:"action"`
	// Reason explains a skip.
	Reason string `json:"reason,omitempty"`
}

// PlanZone decides which file a zone's markers should go in.
//
// The rules exist to make one guarantee: we never write over a file we did not
// create. Zeal's loader forces the shape of this — the numbered files must be
// contiguous and the base file is mandatory, so "just use a high slot to stay
// clear of other packs" does not work; a file at _5 beside a pack ending at _2
// is never read, silently.
//
//   - No base file: we create it. On a plain Quarm install there is no
//     map_files directory at all, so this is the normal path.
//   - Base file is ours (hash matches the manifest): replace it.
//   - Base file is someone else's: walk the contiguous numbered chain and take
//     the first slot that is free or already ours. That appends to their pack
//     without touching it.
//   - Chain already full to _10: skip, and say so.
func PlanZone(mapDir, zone string, points int, m *Manifest) Plan {
	base := zone + ".txt"
	basePath := filepath.Join(mapDir, base)

	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return Plan{Zone: zone, File: base, Points: points, Action: "create"}
	}
	if m.ownsFile(mapDir, base) {
		return Plan{Zone: zone, File: base, Points: points, Action: "replace"}
	}

	// Someone else's base file. Find the first slot in the contiguous chain we
	// can take without displacing anything.
	for i := 1; i <= maxOptionalSlot; i++ {
		rel := fmt.Sprintf("%s_%d.txt", zone, i)
		if _, err := os.Stat(filepath.Join(mapDir, rel)); os.IsNotExist(err) {
			return Plan{Zone: zone, File: rel, Points: points, Action: "create"}
		}
		if m.ownsFile(mapDir, rel) {
			return Plan{Zone: zone, File: rel, Points: points, Action: "replace"}
		}
	}
	return Plan{
		Zone: zone, Points: points, Action: "skip",
		Reason: "all 11 map file slots for this zone are in use by other map data",
	}
}

// Result summarises a completed export.
type Result struct {
	Written int    `json:"written"`
	Skipped int    `json:"skipped"`
	Points  int    `json:"points"`
	Dir     string `json:"dir"`
	Plans   []Plan `json:"plans"`
}

// Write exports markers for every zone in points, keyed by zone short name.
//
// Files are written atomically via a temp file and rename. A half-written base
// file is not merely a bad zone: a base file that fails to parse makes Zeal fall
// back to internal data and skip every numbered file after it, so a torn write
// would silently disable external map data for that zone.
func Write(eqPath, manifestPath string, byZone map[string][]Point) (*Result, error) {
	mapDir := filepath.Join(eqPath, MapFilesDir)
	if err := os.MkdirAll(mapDir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", mapDir, err)
	}
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	zones := make([]string, 0, len(byZone))
	for z := range byZone {
		zones = append(zones, z)
	}
	sort.Strings(zones)

	res := &Result{Dir: mapDir}
	for _, zone := range zones {
		pts := byZone[zone]
		if len(pts) == 0 {
			continue
		}
		body := FormatFile(pts)
		if body == "" {
			continue
		}
		plan := PlanZone(mapDir, zone, len(pts), m)
		if plan.Action == "skip" {
			res.Skipped++
			res.Plans = append(res.Plans, plan)
			continue
		}
		if err := writeAtomic(filepath.Join(mapDir, plan.File), []byte(body)); err != nil {
			return nil, err
		}
		m.Files[plan.File] = FileRecord{
			Zone: zone, SHA256: hashOf([]byte(body)), Points: len(pts),
		}
		res.Written++
		res.Points += len(pts)
		res.Plans = append(res.Plans, plan)
	}

	if err := m.Save(manifestPath); err != nil {
		return nil, err
	}
	return res, nil
}

// Remove deletes every file we still own and clears the manifest.
//
// "Still own" is the whole point: a file whose contents changed since we wrote
// it has been taken over by something else, and is left alone. Removing a map
// pack because it happened to occupy a path we once used would be a far worse
// bug than leaving a stale file behind.
func Remove(eqPath, manifestPath string) (removed int, kept int, err error) {
	mapDir := filepath.Join(eqPath, MapFilesDir)
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return 0, 0, err
	}
	for rel := range m.Files {
		if !m.ownsFile(mapDir, rel) {
			kept++
			continue
		}
		if err := os.Remove(filepath.Join(mapDir, rel)); err != nil && !os.IsNotExist(err) {
			return removed, kept, fmt.Errorf("remove %s: %w", rel, err)
		}
		removed++
	}
	m.Files = map[string]FileRecord{}
	if err := m.Save(manifestPath); err != nil {
		return removed, kept, err
	}
	return removed, kept, nil
}

func writeAtomic(path string, body []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

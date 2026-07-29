// Command mapgen builds backend/data/maps.db from an EverQuest client's zone
// geometry.
//
// This runs offline, on a machine that has the client. The app never reads
// client files — it ships the ~4 MB maps.db this produces. Re-run it only when
// the extraction pipeline changes; the client geometry itself does not move.
//
//	go run ./cmd/mapgen -client /Volumes/T7/EQ/TAKPv22
//	go run ./cmd/mapgen -client <dir> -zones unrest,akheva -out /tmp/maps.db
//	go run ./cmd/mapgen -client <dir> -report      # classify only, write nothing
//
// See docs/maps-feasibility.md for the format research and the technique
// selection rules.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
)

func main() {
	clientDir := flag.String("client", "", "path to the EQ client directory (required)")
	dbPath := flag.String("db", "data/quarm.db", "path to quarm.db, for the zone list")
	outPath := flag.String("out", "data/maps.db", "output maps.db path")
	zoneList := flag.String("zones", "", "comma-separated zone short names (default: all)")
	report := flag.Bool("report", false, "print the classification table and write nothing")
	compareDir := flag.String("compare", "", "write per-zone side-by-side SVGs of all three techniques here, and write no maps.db")
	flag.Parse()

	if *clientDir == "" {
		log.Fatal("-client is required")
	}
	if _, err := os.Stat(*clientDir); err != nil {
		log.Fatalf("client dir %s: %v", *clientDir, err)
	}

	zones, err := resolveZones(*dbPath, *clientDir, *zoneList)
	if err != nil {
		log.Fatalf("resolve zones: %v", err)
	}
	if len(zones) == 0 {
		log.Fatal("no zones to process")
	}
	log.Printf("processing %d zones from %s", len(zones), *clientDir)

	start := time.Now()
	outputs := make([]mapgen.ZoneOutput, 0, len(zones))
	var failed []string

	fmt.Printf("%-16s %9s %9s %7s %7s  %-11s %9s\n",
		"zone", "tris", "walkable", "occ%", "bnd_d", "technique", "segments")
	for _, name := range zones {
		z, err := mapgen.LoadZone(*clientDir, name)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, err))
			fmt.Printf("%-16s %s\n", name, "FAILED")
			continue
		}
		c := mapgen.Classify(z)
		segs := mapgen.Extract(z, c)
		detail := mapgen.ExtractDetail(z, c)
		minX, minY, maxX, maxY := z.Bounds()

		if *compareDir != "" {
			if err := os.MkdirAll(*compareDir, 0o755); err != nil {
				log.Fatalf("mkdir %s: %v", *compareDir, err)
			}
			title := fmt.Sprintf("%s   occ=%.2f  bnd_d=%.2f  z_span=%.0f  -> %s",
				name, c.Occupancy, c.BoundaryDensit, c.ZSpan, c.Technique)
			out := filepath.Join(*compareDir, name+".svg")
			if err := mapgen.RenderComparison(out, title, mapgen.AllTechniques(z, c), 520); err != nil {
				log.Fatalf("render %s: %v", name, err)
			}
		}

		note := ""
		if c.Overridden {
			note = " (override)"
		}
		fmt.Printf("%-16s %9d %9d %6.1f%% %7.2f  %-11s %9d%s\n",
			name, c.Triangles, c.WalkableFaces, 100*c.Occupancy, c.BoundaryDensit,
			c.Technique, len(segs), note)

		outputs = append(outputs, mapgen.ZoneOutput{
			Zone: name, Segments: segs, Detail: detail,
			MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
			Technique: c.Technique, Occupancy: c.Occupancy,
			BoundaryDensity: c.BoundaryDensit, ZSpan: c.ZSpan,
		})
	}

	byTechnique := map[mapgen.Technique]int{}
	totalSegs := 0
	for _, o := range outputs {
		byTechnique[o.Technique]++
		totalSegs += len(o.Segments)
	}
	fmt.Printf("\n%d zones, %d failed, %d segments, %s\n",
		len(outputs), len(failed), totalSegs, time.Since(start).Round(time.Second))
	for _, t := range []mapgen.Technique{
		mapgen.TechniqueContours, mapgen.TechniqueSilhouette, mapgen.TechniqueBoundary,
	} {
		fmt.Printf("   %-11s %4d\n", t, byTechnique[t])
	}
	for _, f := range failed {
		fmt.Printf("   FAILED %s\n", f)
	}

	if *report || *compareDir != "" {
		return
	}

	// POIs come from quarm.db, not the client: everything here is source="db:*"
	// and is rebuilt on every run.
	poiDB, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open %s for POIs: %v", *dbPath, err)
	}
	pois, err := mapgen.GeneratePOIs(poiDB.DB)
	poiDB.Close()
	if err != nil {
		log.Fatalf("generate POIs: %v", err)
	}
	byCategory := map[string]int{}
	for _, p := range pois {
		byCategory[p.Category]++
	}
	fmt.Printf("\n%d POIs\n", len(pois))
	for _, c := range []string{
		mapgen.CategoryVendor, mapgen.CategoryDoor, mapgen.CategoryGroundSpawn,
		mapgen.CategoryZoneLine, mapgen.CategoryRaidTarget, mapgen.CategoryTradeskill,
		mapgen.CategorySuccor, mapgen.CategoryTrap,
	} {
		fmt.Printf("   %-14s %5d\n", c, byCategory[c])
	}

	if err := mapgen.WriteMapsDB(*outPath, outputs, pois); err != nil {
		log.Fatalf("write %s: %v", *outPath, err)
	}
	info, err := os.Stat(*outPath)
	if err != nil {
		log.Fatalf("stat %s: %v", *outPath, err)
	}
	fmt.Printf("\nwrote %s (%.1f MB)\n", *outPath, float64(info.Size())/(1<<20))
}

// resolveZones returns the zone short names to process: the explicit -zones
// list, or every zone in quarm.db that has a matching .s3d in the client.
func resolveZones(dbPath, clientDir, explicit string) ([]string, error) {
	if explicit != "" {
		var out []string
		for _, z := range strings.Split(explicit, ",") {
			if z = strings.TrimSpace(z); z != "" {
				out = append(out, z)
			}
		}
		return out, nil
	}

	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer database.Close()

	rows, err := database.Query(`SELECT DISTINCT short_name FROM zone ORDER BY short_name`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		// Skip zones with no client geometry: instanced copies, loading
		// screens, and long-dead zones still listed in the DB.
		if _, err := os.Stat(filepath.Join(clientDir, name+".s3d")); err != nil {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate zones: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

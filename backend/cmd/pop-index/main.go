// Command pop-index regenerates backend/internal/db/pop_gated.json — the
// precomputed set of Planes-of-Power-gated item IDs the gear-upgrade finder
// uses to hide not-yet-available gear while the pop_enabled flag is off.
//
// Building this set live is a multi-second pass over the loot/spawn join
// tables, so the server reads this embedded result instead of recomputing it
// at runtime (see internal/db/pop_index.go). Re-run this whenever quarm.db is
// regenerated, the same as cmd/quest-sources:
//
//	go run ./cmd/pop-index            # uses backend/data/quarm.db
//	go run ./cmd/pop-index -db /path/to/quarm.db -out path/to/pop_gated.json
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"sort"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func main() {
	dbPath := flag.String("db", "data/quarm.db", "path to quarm.db")
	outPath := flag.String("out", "internal/db/pop_gated.json", "output JSON path")
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db %s: %v", *dbPath, err)
	}
	defer database.Close()

	gated, err := database.ComputePoPGated()
	if err != nil {
		log.Fatalf("compute PoP gated set: %v", err)
	}

	ids := make([]int, 0, len(gated))
	for id := range gated {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	buf, err := json.Marshal(ids)
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(*outPath, buf, 0o644); err != nil {
		log.Fatalf("write %s: %v", *outPath, err)
	}
	log.Printf("wrote %d PoP-gated item IDs to %s", len(ids), *outPath)
}

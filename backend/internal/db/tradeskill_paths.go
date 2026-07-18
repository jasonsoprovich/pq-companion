package db

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
)

//go:embed tradeskill_paths.json
var tradeskillPathsJSON []byte

// PathStep is one recipe in a curated Recommended leveling path, in the order
// the guide it was derived from teaches it. Skill windows are NOT stored here
// — they're computed the same way a Custom path's are (grind to
// min(trivial, cap, target)), so a faithful recipe list reproduces the
// guide's breakpoints without duplicating that logic.
type PathStep struct {
	RecipeID int `json:"recipe_id"`
	// Note is an optional curator annotation surfaced on the stage (e.g.
	// "bulk-buy water flasks", "masks/wrists take 1 part — cheapest variant").
	Note string `json:"note,omitempty"`
}

// recommendedPath is one tradeskill discipline's curated data.
type recommendedPath struct {
	// Source documents where this path was derived from, for maintainers (not
	// surfaced to players). Empty means the discipline has no curated path yet.
	Source string     `json:"source,omitempty"`
	Steps  []PathStep `json:"steps"`
}

var (
	tradeskillPathsOnce sync.Once
	tradeskillPathsByTS map[int]recommendedPath
)

// loadTradeskillPaths parses tradeskill_paths.json once, on first use. A
// parse error leaves every discipline's Recommended path empty rather than
// panicking — the UI treats that the same as "not curated yet."
func loadTradeskillPaths() {
	tradeskillPathsOnce.Do(func() {
		tradeskillPathsByTS = map[int]recommendedPath{}
		var raw map[string]recommendedPath
		if err := json.Unmarshal(tradeskillPathsJSON, &raw); err != nil {
			slog.Error("parse tradeskill_paths.json", "err", err)
			return
		}
		for k, v := range raw {
			id, err := strconv.Atoi(k)
			if err != nil {
				slog.Error("tradeskill_paths.json: non-numeric tradeskill key", "key", k)
				continue
			}
			tradeskillPathsByTS[id] = v
		}
	})
}

// RecommendedPathFor returns the curated Recommended leveling path's recipe
// ids, in guide order, for a tradeskill discipline. Returns nil when no path
// has been authored for that discipline yet (curation is per-discipline and
// starts with the trades that have a solid, era-checkable community guide —
// see internal/db/tradeskill_paths.json for the current coverage).
func RecommendedPathFor(tradeskill int) []PathStep {
	loadTradeskillPaths()
	return tradeskillPathsByTS[tradeskill].Steps
}

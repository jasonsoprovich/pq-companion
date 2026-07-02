package db

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

// The EQMacEmu / Al'Kabor (TAKP) content dump that quarm.db is built from
// ships many same-name rows with different ids — e.g. item "Spell: Color Flux"
// as 15290 and 16204, spell "Acumen" as 1575 and 2248. Some are byte-identical
// copies, some are sparse/orphaned, and some are genuinely referenced by
// different merchant/loot rows. They clutter the item and spell explorers.
//
// Rather than mutate the read-only, regenerated quarm.db (which would orphan
// the loot/merchant/recipe rows that reference these ids, and be wiped by the
// next dump anyway), we collapse same-name rows to a single "canonical" row in
// list/search views and expose the rest as "variants": still fetchable by id,
// surfaced as links on the canonical row's detail. The whole grouping is
// derived from whatever quarm.db is loaded, so a future dump recomputes it
// automatically with nothing to hand-maintain.

// itemRefSources lists the tables/columns that point at an item by id. The
// canonical row in a duplicate group is the one the live game data references
// most — it carries the real merchant inventory, loot, and recipe links while
// the duplicates are sparse or orphaned. Each source is queried defensively:
// a dump missing one of these tables simply contributes zero references.
var itemRefSources = []struct{ table, col string }{
	{"merchantlist", "item"},
	{"lootdrop_entries", "item_id"},
	{"tradeskill_recipe_entries", "item_id"},
	{"starting_items", "itemid"},
	{"ground_spawns", "item"},
	{"forage", "Itemid"},
}

// variantIndex is the canonical/variant grouping for one entity table.
// A nil *variantIndex is safe to call any method on (treated as "no
// duplicates"), so callers needn't nil-check.
type variantIndex struct {
	// canonicalOf maps every id that belongs to a multi-row name group to its
	// group's canonical id (the canonical id maps to itself).
	canonicalOf map[int]int
	// groupOf maps a canonical id to the full sorted id list of its group,
	// including the canonical id.
	groupOf map[int][]int
	// notInBody is the comma-joined list of non-canonical ids for inlining
	// into "id NOT IN (...)"; empty when there are no duplicates.
	notInBody string
}

// excludeNonCanonical returns a SQL boolean clause that filters out every
// non-canonical duplicate row, e.g. "id NOT IN (16204,16236,...)". Returns ""
// when there are no duplicates. The ids are integers we computed (never user
// input), so inlining them is injection-safe and sidesteps SQLite's bound-
// parameter limit (a single dump can have well over 999 duplicate rows).
func (vi *variantIndex) excludeNonCanonical(idExpr string) string {
	if vi == nil || vi.notInBody == "" {
		return ""
	}
	return idExpr + " NOT IN (" + vi.notInBody + ")"
}

// siblings returns the other ids that share this id's name (the collapsed
// variants), sorted ascending, or nil if the id isn't part of a duplicate
// group. For a canonical id this is the list of hidden variants; for a variant
// id it's the canonical plus any other variants.
func (vi *variantIndex) siblings(id int) []int {
	if vi == nil {
		return nil
	}
	canon, ok := vi.canonicalOf[id]
	if !ok {
		return nil
	}
	full := vi.groupOf[canon]
	out := make([]int, 0, len(full)-1)
	for _, x := range full {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// canonicalID returns the canonical id for a variant id, or 0 when id is itself
// the canonical row or not part of a duplicate group. Lets a variant's detail
// view link back to the "main" entry.
func (vi *variantIndex) canonicalID(id int) int {
	if vi == nil {
		return 0
	}
	canon, ok := vi.canonicalOf[id]
	if !ok || canon == id {
		return 0
	}
	return canon
}

// variantFields returns the (siblings, canonicalID) pair for an id, for
// populating the Item/Spell detail fields in one call. List rows are always
// canonical, so there the siblings are the hidden variants and canonical is 0.
func (vi *variantIndex) variantFields(id int) (siblings []int, canonical int) {
	return vi.siblings(id), vi.canonicalID(id)
}

// ensureVariants builds both variant indexes once, on first use. On failure it
// logs and installs an empty index so the feature degrades to "no collapse"
// rather than breaking item/spell queries.
func (db *DB) ensureVariants() {
	db.variantsOnce.Do(func() {
		if iv, err := db.buildItemVariants(); err != nil {
			slog.Warn("db: failed to build item variant index; duplicates will not be collapsed", "err", err)
			db.itemVariants = &variantIndex{}
		} else {
			db.itemVariants = iv
		}
		if sv, err := db.buildSpellVariants(); err != nil {
			slog.Warn("db: failed to build spell variant index; duplicates will not be collapsed", "err", err)
			db.spellVariants = &variantIndex{}
		} else {
			db.spellVariants = sv
		}
	})
}

// buildVariantIndex turns name→ids groups plus a per-id score into a
// variantIndex. The canonical row per group is the highest-scoring id,
// tie-broken by lowest id (so identical copies resolve deterministically to
// the earliest id). Single-row "groups" are ignored.
func buildVariantIndex(groups map[string][]int, score map[int]int) *variantIndex {
	vi := &variantIndex{
		canonicalOf: make(map[int]int),
		groupOf:     make(map[int][]int),
	}
	var nonCanon []int
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		sort.Ints(ids)
		canon := ids[0]
		best := score[canon]
		for _, id := range ids[1:] {
			// strictly-greater keeps the lowest id on ties (ids ascending).
			if score[id] > best {
				best = score[id]
				canon = id
			}
		}
		vi.groupOf[canon] = ids
		for _, id := range ids {
			vi.canonicalOf[id] = canon
			if id != canon {
				nonCanon = append(nonCanon, id)
			}
		}
	}
	sort.Ints(nonCanon)
	parts := make([]string, len(nonCanon))
	for i, id := range nonCanon {
		parts[i] = strconv.Itoa(id)
	}
	vi.notInBody = strings.Join(parts, ",")
	return vi
}

// buildItemVariants groups items by name and scores each duplicate row by how
// many live game-data rows reference it (see itemRefSources).
func (db *DB) buildItemVariants() (*variantIndex, error) {
	groups, dupIDs, err := dupNameGroups(db,
		"SELECT id, Name FROM items WHERE Name IN "+
			"(SELECT Name FROM items GROUP BY Name HAVING COUNT(*) > 1)")
	if err != nil {
		return nil, fmt.Errorf("query duplicate items: %w", err)
	}

	score := make(map[int]int)
	if len(dupIDs) > 0 {
		inBody := intList(dupIDs)
		for _, src := range itemRefSources {
			q := fmt.Sprintf(
				"SELECT %s, COUNT(*) FROM %s WHERE %s IN (%s) GROUP BY %s",
				src.col, src.table, src.col, inBody, src.col)
			rows, qerr := db.Query(q)
			if qerr != nil {
				// Table/column absent in this dump — contributes no references.
				continue
			}
			for rows.Next() {
				var id, n int
				if scanErr := rows.Scan(&id, &n); scanErr != nil {
					slog.Warn("variant index: scan reference row", "table", src.table, "err", scanErr)
					continue
				}
				score[id] += n
			}
			if rerr := rows.Err(); rerr != nil {
				slog.Warn("variant index: iterate reference rows", "table", src.table, "err", rerr)
			}
			rows.Close()
		}
	}
	return buildVariantIndex(groups, score), nil
}

// buildSpellVariants groups spells by name and scores each duplicate row by how
// many of the 15 class columns can cast it (classesN < 255). Spells aren't
// referenced by merchant/loot tables, so the most broadly-castable row is the
// most complete; byte-identical copies tie to the lowest id. This keeps the
// "real" player-castable entry (e.g. Bind Affinity 35) over a stripped clicky
// duplicate (2049, all classes 255).
func (db *DB) buildSpellVariants() (*variantIndex, error) {
	const q = `SELECT id, name,
		(classes1<255)+(classes2<255)+(classes3<255)+(classes4<255)+(classes5<255)+
		(classes6<255)+(classes7<255)+(classes8<255)+(classes9<255)+(classes10<255)+
		(classes11<255)+(classes12<255)+(classes13<255)+(classes14<255)+(classes15<255)
		AS castable
		FROM spells_new
		WHERE name IN (SELECT name FROM spells_new GROUP BY name HAVING COUNT(*) > 1)`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query duplicate spells: %w", err)
	}
	defer rows.Close()

	groups := make(map[string][]int)
	score := make(map[int]int)
	for rows.Next() {
		var id, castable int
		var name string
		if err := rows.Scan(&id, &name, &castable); err != nil {
			return nil, fmt.Errorf("scan duplicate spell: %w", err)
		}
		groups[name] = append(groups[name], id)
		score[id] = castable
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildVariantIndex(groups, score), nil
}

// dupNameGroups runs a "SELECT id, Name ..." query and returns the rows grouped
// by name plus a flat list of every id seen.
func dupNameGroups(db *DB, query string) (map[string][]int, []int, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	groups := make(map[string][]int)
	var all []int
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		groups[name] = append(groups[name], id)
		all = append(all, id)
	}
	return groups, all, rows.Err()
}

// intList joins ids into a comma-separated literal for inlining into SQL.
func intList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

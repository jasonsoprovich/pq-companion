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

// itemVariantRow is one duplicate-name item row with the two derived facts the
// item collapse needs: whether it is a "substantial" equippable item (real gear
// a player equips, as opposed to a quest note or sparse stub) and its gameplay
// identity signature (equal signatures ⇒ effectively the same item).
type itemVariantRow struct {
	id          int
	name        string
	substantial bool
	sig         string
}

// itemSignatureCols are the gameplay-identity columns that decide whether two
// same-name rows are the same item or genuinely different ones. Cosmetic and
// economic fields (icon, price, weight, reclevel, size) are deliberately left
// out so trivially-differing copies still collapse. Effect-id columns are
// normalized (any value ≤ 0 → 0) in code, since "no effect" is stored as both
// 0 and -1 across the dump.
var itemSignatureCols = []string{
	"itemtype", "slots", "classes", "races",
	"ac", "hp", "mana",
	"astr", "asta", "aagi", "adex", "awis", "aint", "acha",
	"mr", "cr", "dr", "fr", "pr",
	"focustype", "damage", "delay", `"range"`,
	// effect ids (normalized): a distinct clicky/proc/worn/focus is a distinct item.
	"focuseffect", "clickeffect", "proceffect", "worneffect", "scrolleffect",
}

// itemEffectCols is the subset of the signature that stores an effect id, where
// both 0 and -1 mean "none" and must be normalized so they don't spuriously
// split otherwise-identical rows.
var itemEffectCols = map[string]bool{
	"focuseffect": true, "clickeffect": true, "proceffect": true,
	"worneffect": true, "scrolleffect": true,
}

// buildItemVariants groups items by name and scores each duplicate row by how
// many live game-data rows reference it (see itemRefSources). Unlike a plain
// name group, genuinely distinct equippable gear that happens to share a name
// (e.g. the Chardok vs. Aten Ha Ra "Mask of Secrets") is kept visible; only
// junk, sparse orphans, and byte-identical copies collapse — see
// buildItemVariantIndex.
func (db *DB) buildItemVariants() (*variantIndex, error) {
	cols := append([]string{"id", "Name"}, itemSignatureCols...)
	q := fmt.Sprintf(
		"SELECT %s FROM items WHERE Name IN "+
			"(SELECT Name FROM items GROUP BY Name HAVING COUNT(*) > 1)",
		strings.Join(cols, ", "))
	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query duplicate items: %w", err)
	}
	defer rows.Close()

	byName := make(map[string][]itemVariantRow)
	substantialSet := make(map[int]bool)
	var dupIDs []int
	for rows.Next() {
		var id int
		var name string
		vals := make([]int, len(itemSignatureCols))
		dest := make([]any, 0, len(itemSignatureCols)+2)
		dest = append(dest, &id, &name)
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan duplicate item: %w", err)
		}

		var sig strings.Builder
		substantial := false
		var slots int
		for i, col := range itemSignatureCols {
			v := vals[i]
			if itemEffectCols[col] && v < 0 {
				v = 0 // "none" is stored as both 0 and -1; treat alike.
			}
			if col == "slots" {
				slots = v
			}
			// A row is substantial if it is equippable and carries any real
			// stat or effect — the mark of gear a player would treat as its own
			// item rather than an orphaned/placeholder copy.
			if col != "itemtype" && col != "slots" && col != "classes" &&
				col != "races" && col != "focustype" && col != "delay" &&
				col != `"range"` && v > 0 {
				substantial = true
			}
			fmt.Fprintf(&sig, "%d|", v)
		}
		substantial = substantial && slots != 0

		r := itemVariantRow{id: id, name: name, substantial: substantial, sig: sig.String()}
		byName[name] = append(byName[name], r)
		if substantial {
			substantialSet[id] = true
		}
		dupIDs = append(dupIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
	return buildItemVariantIndex(byName, substantialSet, score), nil
}

// buildItemVariantIndex collapses duplicate-name item rows, but — unlike the
// generic buildVariantIndex — keeps genuinely distinct equippable gear visible.
//
// Within a name group: if two or more substantial rows have differing identity
// signatures, they are different items (a quality tier, focus effect, or slot
// apart), so each distinct signature is kept as its own canonical row. Junk,
// sparse orphans, and byte-identical copies collapse into whichever kept row
// they belong to. When a group has at most one distinct substantial signature
// (spell scrolls, quest notes, plain orphans) it collapses to a single
// canonical exactly as before.
func buildItemVariantIndex(byName map[string][]itemVariantRow, substantialSet map[int]bool, score map[int]int) *variantIndex {
	vi := &variantIndex{
		canonicalOf: make(map[int]int),
		groupOf:     make(map[int][]int),
	}
	var nonCanon []int

	for _, group := range byName {
		if len(group) < 2 {
			continue
		}

		// Distinct signatures among the substantial (real-gear) rows.
		subSigs := make(map[string]bool)
		for _, r := range group {
			if r.substantial {
				subSigs[r.sig] = true
			}
		}

		// Partition the group into buckets that will each collapse to one
		// canonical. With fewer than two distinct substantial signatures the
		// whole group is one bucket (legacy behaviour); otherwise each distinct
		// substantial signature is its own bucket and the leftover junk rows
		// ride along on the primary (best-referenced) bucket.
		buckets := make(map[string][]int)
		if len(subSigs) < 2 {
			ids := make([]int, len(group))
			for i, r := range group {
				ids[i] = r.id
			}
			buckets["*"] = ids
		} else {
			for _, r := range group {
				if r.substantial {
					buckets[r.sig] = append(buckets[r.sig], r.id)
				}
			}
			primary := primaryBucketKey(buckets, score)
			for _, r := range group {
				if !r.substantial {
					buckets[primary] = append(buckets[primary], r.id)
				}
			}
		}

		for _, ids := range buckets {
			sort.Ints(ids)
			canon := pickItemCanonical(ids, substantialSet, score)
			vi.groupOf[canon] = ids
			for _, id := range ids {
				vi.canonicalOf[id] = canon
				if id != canon {
					nonCanon = append(nonCanon, id)
				}
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

// pickItemCanonical returns the canonical id for one bucket: the highest-scoring
// row, tie-broken by lowest id (ids arrive sorted ascending). When the bucket
// contains any substantial row, non-substantial rows are never eligible, so a
// junk copy can never be chosen over the real gear it shadows.
func pickItemCanonical(ids []int, substantialSet map[int]bool, score map[int]int) int {
	hasSub := false
	for _, id := range ids {
		if substantialSet[id] {
			hasSub = true
			break
		}
	}
	canon, best := -1, -1
	for _, id := range ids {
		if hasSub && !substantialSet[id] {
			continue
		}
		if canon == -1 || score[id] > best {
			best = score[id]
			canon = id
		}
	}
	return canon
}

// primaryBucketKey returns the bucket whose canonical is best-referenced (tie:
// lowest canonical id). Leftover junk rows attach here so they surface as
// variants of the group's most prominent item.
func primaryBucketKey(buckets map[string][]int, score map[int]int) string {
	bestKey, bestScore, bestID := "", -1, 0
	for key, ids := range buckets {
		canon, cScore := 0, -1
		for _, id := range ids {
			if score[id] > cScore || canon == 0 {
				cScore = score[id]
				canon = id
			} else if score[id] == cScore && id < canon {
				canon = id
			}
		}
		if bestKey == "" || cScore > bestScore || (cScore == bestScore && canon < bestID) {
			bestKey, bestScore, bestID = key, cScore, canon
		}
	}
	return bestKey
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

// intList joins ids into a comma-separated literal for inlining into SQL.
func intList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

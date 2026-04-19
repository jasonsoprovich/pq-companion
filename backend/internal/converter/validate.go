package converter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ValidationReport summarises the outcome of a post-conversion data check.
type ValidationReport struct {
	Checks   []CheckResult
	Errors   int
	Warnings int
}

// CheckResult records the outcome of a single validation rule.
type CheckResult struct {
	Name     string
	Severity Severity
	Message  string
	Count    int64
}

// Severity categorises a check outcome.
type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warn"
	SeverityError   Severity = "error"
)

// rowCountExpectation describes the minimum row count for a table to be considered healthy.
// Numbers are conservative lower bounds drawn from the 2026-03-20 dump; a dump that
// falls well below these figures almost certainly failed to import a table.
type rowCountExpectation struct {
	table   string
	minRows int64
}

// coreRowCounts enumerates tables that every PQ Companion build must have.
// Missing or empty tables here break core UI features (item search, spawn lookup, etc.).
// Thresholds are conservative — they're sized so a truncated/partial import trips
// the check, not so a dump with a slightly different mix of data does.
var coreRowCounts = []rowCountExpectation{
	{"items", 20000},
	{"spells_new", 3000},
	{"npc_types", 5000},
	{"zone", 150},
	{"spawn2", 15000},
	{"spawnentry", 15000},
	{"spawngroup", 3000},
	{"lootdrop", 3000},
	{"lootdrop_entries", 30000},
	{"loottable", 3000},
	{"loottable_entries", 10000},
	{"npc_spells", 500},
	{"npc_spells_entries", 3000},
	{"skill_caps", 10000},
}

// referentialCheck describes an orphan lookup between two tables.
// A child row is an "orphan" when its foreign-key column points at a parent
// that does not exist. Some FKs legitimately use 0 or NULL as a sentinel for
// "no parent", so those values are excluded from the orphan count.
//
// Real EQEmu dumps routinely contain a trickle of orphan references (deprecated
// items, retired spell IDs, etc.) that the app handles gracefully. These checks
// report orphans as warnings up to errorThreshold — beyond that a partial import
// is the likely cause, so the check escalates to an error.
type referentialCheck struct {
	name           string
	child          string
	childCol       string
	parent         string
	parentCol      string
	ignoreZero     bool  // treat child value 0 as "no reference"
	errorThreshold int64 // orphan count at which the warning becomes an error
	description    string
}

// Default: warn up to 500 orphans, error above. Individual checks can override.
const defaultOrphanErrorThreshold = 500

var referentialChecks = []referentialCheck{
	{
		name:        "spawnentry.npcID → npc_types.id",
		child:       "spawnentry",
		childCol:    "npcID",
		parent:      "npc_types",
		parentCol:   "id",
		description: "spawn entries whose NPC does not exist",
	},
	{
		name:        "spawnentry.spawngroupID → spawngroup.id",
		child:       "spawnentry",
		childCol:    "spawngroupID",
		parent:      "spawngroup",
		parentCol:   "id",
		description: "spawn entries referencing a missing spawn group",
	},
	{
		name:        "loottable_entries.loottable_id → loottable.id",
		child:       "loottable_entries",
		childCol:    "loottable_id",
		parent:      "loottable",
		parentCol:   "id",
		description: "loot table entries referencing a missing loot table",
	},
	{
		name:        "loottable_entries.lootdrop_id → lootdrop.id",
		child:       "loottable_entries",
		childCol:    "lootdrop_id",
		parent:      "lootdrop",
		parentCol:   "id",
		description: "loot table entries referencing a missing loot drop",
	},
	{
		name:        "lootdrop_entries.item_id → items.id",
		child:       "lootdrop_entries",
		childCol:    "item_id",
		parent:      "items",
		parentCol:   "id",
		description: "loot drop entries referencing a missing item",
	},
	{
		name:        "lootdrop_entries.lootdrop_id → lootdrop.id",
		child:       "lootdrop_entries",
		childCol:    "lootdrop_id",
		parent:      "lootdrop",
		parentCol:   "id",
		description: "loot drop entries referencing a missing loot drop",
	},
	{
		name:        "npc_spells_entries.npc_spells_id → npc_spells.id",
		child:       "npc_spells_entries",
		childCol:    "npc_spells_id",
		parent:      "npc_spells",
		parentCol:   "id",
		description: "NPC spell entries referencing a missing spell list",
	},
	{
		name:        "npc_spells_entries.spellid → spells_new.id",
		child:       "npc_spells_entries",
		childCol:    "spellid",
		parent:      "spells_new",
		parentCol:   "id",
		description: "NPC spell entries referencing a missing spell",
	},
	{
		name:        "npc_types.loottable_id → loottable.id",
		child:       "npc_types",
		childCol:    "loottable_id",
		parent:      "loottable",
		parentCol:   "id",
		ignoreZero:  true, // 0 == no loot
		description: "NPCs pointing at a missing loot table",
	},
	{
		name:        "npc_types.npc_spells_id → npc_spells.id",
		child:       "npc_types",
		childCol:    "npc_spells_id",
		parent:      "npc_spells",
		parentCol:   "id",
		ignoreZero:  true, // 0 == no spells
		description: "NPCs pointing at a missing spell list",
	},
}

// spotCheck verifies a specific well-known record exists.
// These catch silent partial imports that still hit the row-count minimums.
type spotCheck struct {
	name  string
	query string
	args  []interface{}
}

var spotChecks = []spotCheck{
	// Cloth Cap is the canonical starter armor in every EQEmu dump.
	{
		name:  "item 1001 is Cloth Cap",
		query: "SELECT COUNT(*) FROM items WHERE id = 1001 AND lower(Name) = 'cloth cap'",
	},
	// Zones: North Karana (short_name = 'northkarana') is a bellwether zone.
	{
		name:  "zone 'northkarana' exists",
		query: "SELECT COUNT(*) FROM zone WHERE short_name = 'northkarana'",
	},
	// Spells: 'Complete Healing' is spell id 13 and is a classic EQ priest spell.
	{
		name:  "spell 13 is Complete Healing",
		query: "SELECT COUNT(*) FROM spells_new WHERE id = 13 AND lower(name) = 'complete healing'",
	},
	// 'Minor Healing' is present in every classic-era dump — not pinned to an ID here
	// because a single-record spot check is sufficient; we mostly want to catch a
	// spells_new table that imported metadata but dropped rows.
	{
		name:  "spells_new contains 'Minor Healing'",
		query: "SELECT COUNT(*) FROM spells_new WHERE lower(name) = 'minor healing'",
	},
}

// Validate runs the full validation suite against an already-populated SQLite
// database. It returns a report describing each check and any issues.
//
// The caller should treat report.Errors > 0 as a conversion failure.
func Validate(ctx context.Context, cfg Config, db *sql.DB) (*ValidationReport, error) {
	rep := &ValidationReport{}

	if err := runRowCountChecks(ctx, rep, db); err != nil {
		return rep, err
	}
	if err := runReferentialChecks(ctx, rep, db); err != nil {
		return rep, err
	}
	if err := runSpotChecks(ctx, rep, db); err != nil {
		return rep, err
	}

	logReport(cfg, rep)
	return rep, nil
}

func runRowCountChecks(ctx context.Context, rep *ValidationReport, db *sql.DB) error {
	for _, exp := range coreRowCounts {
		var n int64
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(exp.table))
		if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			rep.add(CheckResult{
				Name:     "row count " + exp.table,
				Severity: SeverityError,
				Message:  fmt.Sprintf("query failed: %v", err),
			})
			continue
		}

		switch {
		case n < exp.minRows:
			rep.add(CheckResult{
				Name:     "row count " + exp.table,
				Severity: SeverityError,
				Message:  fmt.Sprintf("%d rows, expected at least %d", n, exp.minRows),
				Count:    n,
			})
		default:
			rep.add(CheckResult{
				Name:     "row count " + exp.table,
				Severity: SeverityOK,
				Message:  fmt.Sprintf("%d rows", n),
				Count:    n,
			})
		}
	}
	return nil
}

func runReferentialChecks(ctx context.Context, rep *ValidationReport, db *sql.DB) error {
	for _, rc := range referentialChecks {
		var filter strings.Builder
		filter.WriteString("c.")
		filter.WriteString(quoteIdent(rc.childCol))
		filter.WriteString(" IS NOT NULL")
		if rc.ignoreZero {
			filter.WriteString(" AND c.")
			filter.WriteString(quoteIdent(rc.childCol))
			filter.WriteString(" != 0")
		}

		q := fmt.Sprintf(
			`SELECT COUNT(*) FROM %s c LEFT JOIN %s p ON p.%s = c.%s WHERE %s AND p.%s IS NULL`,
			quoteIdent(rc.child), quoteIdent(rc.parent),
			quoteIdent(rc.parentCol), quoteIdent(rc.childCol),
			filter.String(), quoteIdent(rc.parentCol),
		)
		var orphans int64
		if err := db.QueryRowContext(ctx, q).Scan(&orphans); err != nil {
			rep.add(CheckResult{
				Name:     rc.name,
				Severity: SeverityError,
				Message:  fmt.Sprintf("query failed: %v", err),
			})
			continue
		}

		threshold := rc.errorThreshold
		if threshold == 0 {
			threshold = defaultOrphanErrorThreshold
		}

		switch {
		case orphans == 0:
			rep.add(CheckResult{
				Name:     rc.name,
				Severity: SeverityOK,
				Message:  "no orphans",
			})
		case orphans >= threshold:
			rep.add(CheckResult{
				Name:     rc.name,
				Severity: SeverityError,
				Message:  fmt.Sprintf("%d %s (threshold %d)", orphans, rc.description, threshold),
				Count:    orphans,
			})
		default:
			rep.add(CheckResult{
				Name:     rc.name,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("%d %s", orphans, rc.description),
				Count:    orphans,
			})
		}
	}
	return nil
}

func runSpotChecks(ctx context.Context, rep *ValidationReport, db *sql.DB) error {
	for _, sc := range spotChecks {
		var n int64
		if err := db.QueryRowContext(ctx, sc.query, sc.args...).Scan(&n); err != nil {
			rep.add(CheckResult{
				Name:     sc.name,
				Severity: SeverityError,
				Message:  fmt.Sprintf("query failed: %v", err),
			})
			continue
		}
		if n == 0 {
			rep.add(CheckResult{
				Name:     sc.name,
				Severity: SeverityError,
				Message:  "expected record not found",
			})
			continue
		}
		rep.add(CheckResult{
			Name:     sc.name,
			Severity: SeverityOK,
			Message:  "found",
		})
	}
	return nil
}

func (r *ValidationReport) add(c CheckResult) {
	r.Checks = append(r.Checks, c)
	switch c.Severity {
	case SeverityError:
		r.Errors++
	case SeverityWarning:
		r.Warnings++
	}
}

func logReport(cfg Config, rep *ValidationReport) {
	log := cfg.log()
	for _, c := range rep.Checks {
		switch c.Severity {
		case SeverityOK:
			log.Info("validation check passed", "check", c.Name, "detail", c.Message)
		case SeverityWarning:
			log.Warn("validation warning", "check", c.Name, "detail", c.Message)
		case SeverityError:
			log.Error("validation error", "check", c.Name, "detail", c.Message)
		}
	}
	log.Info("validation summary",
		"total", len(rep.Checks),
		"errors", rep.Errors,
		"warnings", rep.Warnings,
	)
}

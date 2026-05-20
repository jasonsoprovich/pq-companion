package enums

import (
	"database/sql"
	"fmt"
	"strings"
)

// sampleRows runs `SELECT id, name FROM <table> WHERE <column> = ? ORDER BY id LIMIT ?`
// and returns the rows. Used by per-enum AuditDef.Sample funcs to pull
// example records for visual verification.
func sampleRows(db *sql.DB, table, column string, code, limit int) ([]SampleRow, error) {
	return sampleRowsNamed(db, table, column, "name", code, limit)
}

// sampleRowsNamed is sampleRows with an explicit name-column override
// (e.g. zone.short_name instead of the canonical "name").
func sampleRowsNamed(db *sql.DB, table, column, nameCol string, code, limit int) ([]SampleRow, error) {
	q := fmt.Sprintf(`SELECT id, COALESCE(%s, '') FROM %s WHERE %s = ? ORDER BY id LIMIT ?`, nameCol, table, column)
	rows, err := db.Query(q, code, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// sampleSpellEffect returns spells whose effectid1..effectid12 includes
// the given code. Slightly more involved than sampleRows because the
// 12 effect slots are separate columns.
func sampleSpellEffect(db *sql.DB, code, limit int) ([]SampleRow, error) {
	conds := make([]string, 12)
	args := make([]any, 0, 13)
	for i := 0; i < 12; i++ {
		conds[i] = fmt.Sprintf("effectid%d = ?", i+1)
		args = append(args, code)
	}
	args = append(args, limit)
	q := fmt.Sprintf(`SELECT id, COALESCE(name, '') FROM spells_new WHERE %s ORDER BY id LIMIT ?`, strings.Join(conds, " OR "))
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// sampleSpecialAbility returns NPCs whose special_abilities string
// carries the given code. The column is a caret-delimited list of
// "code,value" pairs (e.g. "1,1^18,1^19,1"), so we LIKE-match on the
// code boundary.
func sampleSpecialAbility(db *sql.DB, code, limit int) ([]SampleRow, error) {
	// Match either "<code>," at the very start, or "^<code>," after a separator.
	prefix := fmt.Sprintf("%d,", code)
	infix := "%^" + prefix + "%"
	startPattern := prefix + "%"
	q := `SELECT id, COALESCE(name, '') FROM npc_types
	      WHERE special_abilities LIKE ? OR special_abilities LIKE ?
	      ORDER BY id LIMIT ?`
	rows, err := db.Query(q, startPattern, infix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

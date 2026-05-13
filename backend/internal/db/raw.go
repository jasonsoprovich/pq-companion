package db

import (
	"database/sql"
	"fmt"
)

// RawField is a single column from a database row, preserving the original
// column order so the UI can render them the same way pqdi.cc does.
type RawField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// RawRow is the ordered list of columns + values for a single row, plus the
// table name it came from (useful as a debug header in the UI).
type RawRow struct {
	Table  string     `json:"table"`
	Fields []RawField `json:"fields"`
}

// GetRawRow runs `SELECT * FROM <table> WHERE <idCol> = ?` and returns every
// column in declaration order. Returns sql.ErrNoRows if the row isn't found.
//
// table and idCol are caller-supplied identifiers (never user input) so we
// can safely interpolate them; the id value is bound as a parameter.
func (db *DB) GetRawRow(table, idCol string, id int) (*RawRow, error) {
	q := fmt.Sprintf("SELECT * FROM %s WHERE %s = ? LIMIT 1", table, idCol)
	rows, err := db.Query(q, id)
	if err != nil {
		return nil, fmt.Errorf("query raw row: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}

	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	fields := make([]RawField, len(cols))
	for i, name := range cols {
		fields[i] = RawField{Name: name, Value: normalizeRawValue(values[i])}
	}
	return &RawRow{Table: table, Fields: fields}, nil
}

// normalizeRawValue converts SQLite driver values into types the JSON encoder
// can serialize naturally. []byte → string, everything else passes through.
func normalizeRawValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return v
	}
}

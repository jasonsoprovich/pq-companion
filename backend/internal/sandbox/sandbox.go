// Package sandbox provides a guarded, read-only SQL query interface against
// quarm.db for the Settings → Developer tab. It is gated by the
// Preferences.DeveloperMode flag and intentionally exposes only SELECT/WITH/
// EXPLAIN statements through a separate database connection so a runaway
// query in the sandbox cannot block reads from the rest of the app.
package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

const (
	// MaxRows is the hard cap on rows returned by Query. Anything beyond
	// this is silently dropped and the caller is told via Truncated=true.
	// Keeps the JSON payload bounded so a careless `SELECT * FROM items`
	// doesn't try to ship hundreds of MB to the renderer.
	MaxRows = 10_000

	// DefaultTimeout is the per-query deadline. modernc.org/sqlite honours
	// context cancellation cooperatively (between row reads), so a runaway
	// query gets interrupted instead of pinning the connection forever.
	DefaultTimeout = 8 * time.Second
)

// Sandbox owns a dedicated read-only connection pool to quarm.db. Open the
// sandbox once at startup; the underlying *sql.DB handles concurrency for
// the (rare) case of overlapping requests from the renderer.
type Sandbox struct {
	db   *sql.DB
	path string
}

// Open returns a Sandbox backed by the same quarm.db file the rest of the
// app reads. The connection is opened with mode=ro&immutable=1 — these are
// the same flags the main read-only pool uses — and is independent of that
// pool so a long-running sandbox query can't starve item/NPC/zone reads.
func Open(path string) (*Sandbox, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sandbox sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sandbox sqlite: %w", err)
	}
	// Two connections is plenty: one for an in-flight query, one for the
	// schema introspection endpoint the UI calls in parallel. Capping
	// avoids unbounded growth if the user spams the Run button.
	db.SetMaxOpenConns(2)
	return &Sandbox{db: db, path: path}, nil
}

// Close releases the underlying *sql.DB.
func (s *Sandbox) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Result is the structured response of a sandbox Query. Rows are returned
// as []any so JSON encoding preserves SQLite's native types (number/string/
// nil/[]byte). Columns is kept in declaration order; the slice indexes
// into Rows[i].
type Result struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"row_count"`
	DurationMS int64    `json:"duration_ms"`
	Truncated  bool     `json:"truncated"`
}

// ErrStatementNotAllowed is returned by Query when the user-supplied SQL
// doesn't start with a permitted statement keyword. The check is a coarse
// safety net — the real guarantee that nothing mutates the database is
// the mode=ro&immutable=1 connection — but rejecting obvious writes early
// gives a clearer error than the SQLite "attempt to write a readonly
// database" wall.
var ErrStatementNotAllowed = errors.New("sandbox: only SELECT, WITH, and EXPLAIN statements are allowed")

// ErrEmpty is returned when the input contains no statement after comment
// and whitespace stripping.
var ErrEmpty = errors.New("sandbox: query is empty")

// allowedPrefixes is the set of first keywords (case-insensitive) we accept.
// PRAGMA is intentionally excluded — even read-only PRAGMAs are noise here,
// and some (writable_schema) could be misused.
var allowedPrefixes = []string{"SELECT", "WITH", "EXPLAIN"}

// Query runs sql against the read-only quarm.db connection with a deadline
// and row cap. The statement must begin with SELECT, WITH, or EXPLAIN;
// anything else returns ErrStatementNotAllowed without touching the DB.
func (s *Sandbox) Query(ctx context.Context, sqlText string) (*Result, error) {
	stripped := stripLeadingCommentsAndSpace(sqlText)
	if stripped == "" {
		return nil, ErrEmpty
	}
	if !hasAllowedPrefix(stripped) {
		return nil, ErrStatementNotAllowed
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	start := time.Now()
	rows, err := s.db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := &Result{Columns: cols, Rows: make([][]any, 0, 256)}
	truncated := false
	for rows.Next() {
		if len(out.Rows) >= MaxRows {
			truncated = true
			break
		}
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// modernc.org/sqlite returns []byte for TEXT in some paths; coerce
		// to string so the JSON wire format is stable and readable.
		for i, v := range dest {
			if b, ok := v.([]byte); ok {
				dest[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, dest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out.RowCount = len(out.Rows)
	out.DurationMS = time.Since(start).Milliseconds()
	out.Truncated = truncated
	return out, nil
}

// Column is one column entry returned by the schema introspection endpoint.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	NotNull  bool   `json:"notnull"`
	PK       bool   `json:"pk"`
}

// Table is one table (or view) entry returned by Schema. Columns are in
// declared order so the UI can render them naturally.
type Table struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // "table" or "view"
	Columns []Column `json:"columns"`
}

// Schema returns every user table and view in quarm.db along with their
// columns. Internal sqlite_* tables are filtered out. The result is sorted
// alphabetically by name so the UI can render directly without sorting.
func (s *Sandbox) Schema(ctx context.Context) ([]Table, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Name, &t.Kind); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// PRAGMA table_info can't take a bind parameter, so we splice in the
	// table name. Names come from sqlite_master so they're safe — but
	// quote them defensively in case any table ever ships with a quoted
	// identifier.
	for i := range tables {
		cols, err := s.tableInfo(ctx, tables[i].Name)
		if err != nil {
			return nil, err
		}
		tables[i].Columns = cols
	}
	return tables, nil
}

func (s *Sandbox) tableInfo(ctx context.Context, name string) ([]Column, error) {
	q := fmt.Sprintf(`PRAGMA table_info("%s")`, strings.ReplaceAll(name, `"`, `""`))
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Column
	for rows.Next() {
		var (
			cid     int
			c       Column
			dflt    sql.NullString
			notnull int
			pk      int
		)
		if err := rows.Scan(&cid, &c.Name, &c.Type, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		c.NotNull = notnull != 0
		c.PK = pk != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// stripLeadingCommentsAndSpace removes -- line comments, /* block comments */,
// and whitespace from the start of s. It does NOT try to parse the full
// statement — it only walks enough to find the first non-comment, non-space
// token so hasAllowedPrefix can inspect it.
func stripLeadingCommentsAndSpace(s string) string {
	for {
		// Trim leading whitespace.
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		if len(s) == 0 {
			return ""
		}
		// Skip a -- line comment.
		if strings.HasPrefix(s, "--") {
			if nl := strings.IndexByte(s, '\n'); nl >= 0 {
				s = s[nl+1:]
				continue
			}
			return ""
		}
		// Skip a /* ... */ block comment.
		if strings.HasPrefix(s, "/*") {
			if end := strings.Index(s, "*/"); end >= 0 {
				s = s[end+2:]
				continue
			}
			return ""
		}
		return s
	}
}

// hasAllowedPrefix returns true when s starts with one of the allowed
// statement keywords, comparing case-insensitively against the first word.
func hasAllowedPrefix(s string) bool {
	// Pluck the first whitespace-delimited word.
	end := 0
	for end < len(s) && !unicode.IsSpace(rune(s[end])) && s[end] != '(' && s[end] != ';' {
		end++
	}
	word := strings.ToUpper(s[:end])
	for _, p := range allowedPrefixes {
		if word == p {
			return true
		}
	}
	return false
}

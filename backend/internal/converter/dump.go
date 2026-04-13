package converter

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds configuration for the converter.
type Config struct {
	Verbose bool
	Logger  *slog.Logger
}

func (c Config) log() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// ConvertFromDump reads the given MySQL .sql dump files and populates the SQLite db.
func ConvertFromDump(ctx context.Context, cfg Config, files []string, db *sql.DB) error {
	if err := configureSQLite(db); err != nil {
		return fmt.Errorf("configure sqlite: %w", err)
	}

	for _, path := range files {
		cfg.log().Info("processing dump file", "file", path)
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		err = processDumpFile(ctx, cfg, f, db)
		f.Close()
		if err != nil {
			return fmt.Errorf("process %s: %w", path, err)
		}
	}
	return nil
}

// configureSQLite applies performance pragmas for bulk loading.
func configureSQLite(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=OFF",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-64000", // 64 MB
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// processDumpFile reads and processes all SQL statements from a dump file.
func processDumpFile(ctx context.Context, cfg Config, r io.Reader, db *sql.DB) error {
	scanner := newStatementScanner(r)
	tableCount := 0
	rowCount := 0

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stmt := strings.TrimSpace(scanner.Statement())
		if stmt == "" {
			continue
		}

		// Only uppercase the first few chars to identify the statement type —
		// avoiding O(n) copy of potentially-MB INSERT statements on every call.
		prefix := stmt
		if len(prefix) > 30 {
			prefix = prefix[:30]
		}
		upper := strings.ToUpper(prefix)

		switch {
		case strings.HasPrefix(upper, "CREATE TABLE"):
			if err := execCreateTable(cfg, stmt, db); err != nil {
				return fmt.Errorf("CREATE TABLE: %w", err)
			}
			tableCount++
			if cfg.Verbose {
				cfg.log().Info("created table", "n", tableCount)
			}

		case strings.HasPrefix(upper, "INSERT INTO"):
			n, err := execInsert(ctx, cfg, stmt, db)
			if err != nil {
				return fmt.Errorf("INSERT: %w", err)
			}
			rowCount += n
			if cfg.Verbose {
				cfg.log().Debug("inserted rows", "count", n, "total", rowCount)
			}

		case strings.HasPrefix(upper, "DROP TABLE"):
			// Convert MySQL DROP TABLE to SQLite
			sqliteStmt := convertDropTable(stmt)
			if _, err := db.ExecContext(ctx, sqliteStmt); err != nil {
				cfg.log().Warn("drop table failed", "err", err)
			}
		// skip SET, LOCK, UNLOCK, ALTER, etc.
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	cfg.log().Info("dump processing complete",
		"tables", tableCount,
		"rows", rowCount)
	return nil
}

// execCreateTable converts and executes a MySQL CREATE TABLE statement.
func execCreateTable(cfg Config, stmt string, db *sql.DB) error {
	create, indexes, err := ConvertCreateTable(stmt)
	if err != nil {
		return fmt.Errorf("convert schema: %w", err)
	}

	if cfg.Verbose {
		cfg.log().Debug("CREATE TABLE", "sql", create)
	}

	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("exec create: %w\nsql: %s", err, create)
	}

	for _, idx := range indexes {
		if cfg.Verbose {
			cfg.log().Debug("CREATE INDEX", "sql", idx)
		}
		if _, err := db.Exec(idx); err != nil {
			// Non-fatal: log and continue
			cfg.log().Warn("create index failed", "sql", idx, "err", err)
		}
	}
	return nil
}

// execInsert parses and executes a MySQL INSERT statement into SQLite.
// Uses batched multi-row INSERT for performance. Returns the number of rows inserted.
func execInsert(ctx context.Context, cfg Config, stmt string, db *sql.DB) (int, error) {
	tableName, rows, err := parseInsert(stmt)
	if err != nil {
		return 0, fmt.Errorf("parse insert: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	colCount := len(rows[0])

	// Batch size: keep total parameters well within SQLite's 32766 limit.
	const maxParams = 9999
	batchSize := maxParams / colCount
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 500 {
		batchSize = 500
	}

	rowPlaceholder := "(" + strings.Repeat("?,", colCount-1) + "?)"

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}

	inserted := 0
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		// Build a multi-row INSERT: VALUES (?,?,...),(?,?,...), ...
		placeholders := make([]string, len(batch))
		for i := range batch {
			placeholders[i] = rowPlaceholder
		}
		query := "INSERT OR REPLACE INTO " + quoteIdent(tableName) +
			" VALUES " + strings.Join(placeholders, ",")

		// Flatten all row values into a single args slice
		args := make([]interface{}, len(batch)*colCount)
		for i, row := range batch {
			copy(args[i*colCount:], row)
		}

		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("batch insert rows %d-%d: %w", start, end, err)
		}
		inserted += len(batch)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}

// convertDropTable converts a MySQL DROP TABLE to SQLite-compatible syntax.
func convertDropTable(stmt string) string {
	// Ensure IF EXISTS is present
	upper := strings.ToUpper(stmt)
	if !strings.Contains(upper, "IF EXISTS") {
		stmt = strings.Replace(stmt, "DROP TABLE ", "DROP TABLE IF EXISTS ", 1)
		stmt = strings.Replace(stmt, "drop table ", "DROP TABLE IF EXISTS ", 1)
	}
	// Remove backticks → standard ident (SQLite handles backticks but let's normalize)
	return stmt
}

// parseInsert parses a MySQL INSERT INTO ... VALUES (...),(...); statement.
// Returns the table name and a slice of rows, each row being []interface{}.
func parseInsert(stmt string) (tableName string, rows [][]interface{}, err error) {
	s := stmt

	// Skip "INSERT INTO" — compare only the needed prefix, no full ToUpper.
	if len(s) < 11 || !strings.EqualFold(s[:11], "INSERT INTO") {
		return "", nil, fmt.Errorf("not an INSERT statement")
	}
	s = s[11:]
	s = skipWhitespace(s)

	// Extract table name
	tableName, s, err = extractQuotedIdent(s)
	if err != nil {
		return "", nil, fmt.Errorf("table name: %w", err)
	}
	s = skipWhitespace(s)

	// Skip to VALUES keyword.
	// The remainder after the table name is short (column list or just "VALUES"),
	// so ToUpper here is fine — it's O(small constant), not O(data size).
	if len(s) > 0 && s[0] == '(' {
		// Has explicit column list — skip it, then find VALUES
		_, s2, err2 := extractBalancedParens(s)
		if err2 != nil {
			return "", nil, fmt.Errorf("skip column list: %w", err2)
		}
		s = skipWhitespace(s2)
	}
	// Now expect VALUES
	if len(s) >= 6 && strings.EqualFold(s[:6], "VALUES") {
		s = s[6:]
	} else {
		return "", nil, fmt.Errorf("VALUES keyword not found, got: %q", s[:min(20, len(s))])
	}

	s = skipWhitespace(s)

	// Parse value tuples
	for len(s) > 0 {
		s = skipWhitespace(s)
		if len(s) == 0 || s[0] == ';' {
			break
		}
		if s[0] != '(' {
			break
		}

		row, rest, err := parseTuple(s)
		if err != nil {
			return "", nil, fmt.Errorf("parse tuple: %w", err)
		}
		rows = append(rows, row)
		s = skipWhitespace(rest)

		// Expect comma or semicolon/end
		if len(s) > 0 && s[0] == ',' {
			s = s[1:]
		}
	}

	return tableName, rows, nil
}

// parseTuple parses a MySQL VALUES tuple like (val1, val2, ...).
// Returns the values as []interface{} and the remaining string.
func parseTuple(s string) ([]interface{}, string, error) {
	if len(s) == 0 || s[0] != '(' {
		return nil, s, fmt.Errorf("expected '('")
	}
	s = s[1:] // skip opening (

	var values []interface{}

	for {
		s = skipWhitespace(s)
		if len(s) == 0 {
			return nil, "", fmt.Errorf("unterminated tuple")
		}
		if s[0] == ')' {
			return values, s[1:], nil
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}

		val, rest, err := parseValue(s)
		if err != nil {
			return nil, "", fmt.Errorf("parse value: %w", err)
		}
		values = append(values, val)
		s = rest
	}
}

// parseValue parses a single MySQL value (NULL, number, or string).
// Returns the Go value and the remaining string.
func parseValue(s string) (interface{}, string, error) {
	s = skipWhitespace(s)
	if len(s) == 0 {
		return nil, "", fmt.Errorf("empty value")
	}

	// NULL — check individual bytes to avoid ToUpper on the entire remaining string.
	if len(s) >= 4 &&
		(s[0] == 'N' || s[0] == 'n') &&
		(s[1] == 'U' || s[1] == 'u') &&
		(s[2] == 'L' || s[2] == 'l') &&
		(s[3] == 'L' || s[3] == 'l') &&
		(len(s) == 4 || !isIdentChar(rune(s[4]))) {
		return nil, s[4:], nil
	}

	// Single-quoted string
	if s[0] == '\'' {
		return parseString(s)
	}

	// Number (integer or float, possibly negative)
	if s[0] == '-' || s[0] == '+' || (s[0] >= '0' && s[0] <= '9') {
		return parseNumber(s)
	}

	// Unquoted keyword or identifier value (e.g. CURRENT_TIMESTAMP)
	i := 0
	for i < len(s) && !isValueTerminator(rune(s[i])) {
		i++
	}
	return s[:i], s[i:], nil
}

// parseString parses a MySQL single-quoted string literal.
// Handles MySQL escape sequences: \', \\, \n, \r, \t, \0, \b, \Z, \"
func parseString(s string) (string, string, error) {
	if len(s) == 0 || s[0] != '\'' {
		return "", s, fmt.Errorf("expected single quote")
	}
	s = s[1:] // skip opening quote

	var sb strings.Builder
	for {
		if len(s) == 0 {
			return "", "", fmt.Errorf("unterminated string")
		}
		ch := s[0]
		if ch == '\'' {
			// Check for '' escape (SQL standard)
			if len(s) > 1 && s[1] == '\'' {
				sb.WriteByte('\'')
				s = s[2:]
				continue
			}
			// End of string
			return sb.String(), s[1:], nil
		}
		if ch == '\\' {
			if len(s) < 2 {
				return "", "", fmt.Errorf("trailing backslash in string")
			}
			esc := s[1]
			s = s[2:]
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '0':
				sb.WriteByte(0)
			case 'b':
				sb.WriteByte('\b')
			case 'Z':
				sb.WriteByte(26) // ^Z
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				// Any other \x → x
				sb.WriteByte(esc)
			}
			continue
		}
		sb.WriteByte(ch)
		s = s[1:]
	}
}

// parseNumber parses an integer or float value using fast strconv functions.
func parseNumber(s string) (interface{}, string, error) {
	i := 0
	isFloat := false

	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		isFloat = true
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		isFloat = true
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}

	numStr := s[:i]
	rest := s[i:]

	if isFloat {
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return numStr, rest, nil
		}
		return f, rest, nil
	}

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		// Try as unsigned 64-bit (MySQL uses unsigned bigint sometimes)
		u, err2 := strconv.ParseUint(numStr, 10, 64)
		if err2 != nil {
			return numStr, rest, nil
		}
		return int64(u), rest, nil
	}
	return n, rest, nil
}

// extractBalancedParens extracts a balanced parenthesized group from s.
// Returns the content (including parens) and the remaining string.
func extractBalancedParens(s string) (string, string, error) {
	if len(s) == 0 || s[0] != '(' {
		return "", s, fmt.Errorf("expected '('")
	}
	depth := 0
	inStr := false
	i := 0
	for i < len(s) {
		ch := s[i]
		if inStr {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == '\'' {
				inStr = false
			}
			i++
			continue
		}
		switch ch {
		case '\'':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i+1], s[i+1:], nil
			}
		}
		i++
	}
	return "", "", fmt.Errorf("unmatched '('")
}

// isIdentChar reports whether r is a valid SQL identifier character.
func isIdentChar(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// isValueTerminator reports whether r terminates a bare value token.
func isValueTerminator(r rune) bool {
	return r == ',' || r == ')' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// statementScanner reads SQL statements from a Reader, yielding one at a time.
type statementScanner struct {
	r    *bufio.Reader
	stmt strings.Builder
	err  error
}

func newStatementScanner(r io.Reader) *statementScanner {
	return &statementScanner{r: bufio.NewReaderSize(r, 64*1024)}
}

// Scan advances to the next complete SQL statement.
// Returns true if a statement was found.
func (s *statementScanner) Scan() bool {
	s.stmt.Reset()

	type scanState int
	const (
		stateNormal scanState = iota
		stateString           // inside '...'
		stateLineComment      // after --
		stateBlockComment     // inside /* */
	)

	state := stateNormal
	var prev byte

	for {
		b, err := s.r.ReadByte()
		if err != nil {
			if err == io.EOF {
				// flush any trailing content (no final semicolon)
				stmt := strings.TrimSpace(s.stmt.String())
				if stmt != "" {
					s.stmt.Reset()
					s.stmt.WriteString(stmt)
					return true
				}
			}
			s.err = err
			return false
		}

		switch state {
		case stateNormal:
			switch b {
			case '\'':
				state = stateString
				s.stmt.WriteByte(b)
			case '-':
				if prev == '-' {
					// start line comment — remove the '-' we already wrote
					raw := s.stmt.String()
					s.stmt.Reset()
					s.stmt.WriteString(raw[:len(raw)-1])
					state = stateLineComment
				} else {
					s.stmt.WriteByte(b)
				}
			case '/':
				// might be /* */
				s.stmt.WriteByte(b)
			case '*':
				if prev == '/' {
					// block comment — remove the '/' we already wrote
					raw := s.stmt.String()
					s.stmt.Reset()
					s.stmt.WriteString(raw[:len(raw)-1])
					state = stateBlockComment
				} else {
					s.stmt.WriteByte(b)
				}
			case ';':
				stmt := strings.TrimSpace(s.stmt.String())
				s.stmt.Reset()
				prev = b
				if stmt != "" {
					s.stmt.WriteString(stmt)
					return true
				}
			default:
				s.stmt.WriteByte(b)
			}

		case stateString:
			s.stmt.WriteByte(b)
			switch b {
			case '\\':
				// escape sequence — next byte is literal
				next, err := s.r.ReadByte()
				if err == nil {
					s.stmt.WriteByte(next)
				}
			case '\'':
				// check for '' (doubled quote)
				next, err := s.r.ReadByte()
				if err == nil {
					if next == '\'' {
						s.stmt.WriteByte(next)
					} else {
						s.r.UnreadByte()
						state = stateNormal
					}
				} else {
					state = stateNormal
				}
			}

		case stateLineComment:
			if b == '\n' {
				state = stateNormal
				s.stmt.WriteByte('\n')
			}
			// discard comment content

		case stateBlockComment:
			if b == '/' && prev == '*' {
				state = stateNormal
			}
			// discard comment content
		}

		prev = b
	}
}

// Statement returns the most recently scanned SQL statement.
func (s *statementScanner) Statement() string {
	return s.stmt.String()
}

// Err returns the first non-EOF error encountered.
func (s *statementScanner) Err() error {
	if s.err == io.EOF {
		return nil
	}
	return s.err
}

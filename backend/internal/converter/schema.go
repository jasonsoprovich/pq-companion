// Package converter handles MySQL→SQLite schema and data conversion.
package converter

import (
	"fmt"
	"strings"
	"unicode"
)

// mysqlTypeMap maps lowercase MySQL type names to SQLite affinity types.
var mysqlTypeMap = map[string]string{
	"tinyint":    "INTEGER",
	"smallint":   "INTEGER",
	"mediumint":  "INTEGER",
	"int":        "INTEGER",
	"integer":    "INTEGER",
	"bigint":     "INTEGER",
	"bool":       "INTEGER",
	"boolean":    "INTEGER",
	"bit":        "INTEGER",
	"year":       "INTEGER",
	"float":      "REAL",
	"double":     "REAL",
	"decimal":    "REAL",
	"numeric":    "REAL",
	"varchar":    "TEXT",
	"char":       "TEXT",
	"tinytext":   "TEXT",
	"text":       "TEXT",
	"mediumtext": "TEXT",
	"longtext":   "TEXT",
	"enum":       "TEXT",
	"set":        "TEXT",
	"date":       "TEXT",
	"datetime":   "TEXT",
	"timestamp":  "TEXT",
	"time":       "TEXT",
	"tinyblob":   "BLOB",
	"blob":       "BLOB",
	"mediumblob": "BLOB",
	"longblob":   "BLOB",
	"binary":     "BLOB",
	"varbinary":  "BLOB",
	"json":       "TEXT",
}

// mapMySQLType converts a MySQL type name (lowercase) to a SQLite type.
// Returns "TEXT" as default for unknown types.
func mapMySQLType(mysqlType string) string {
	if t, ok := mysqlTypeMap[strings.ToLower(mysqlType)]; ok {
		return t
	}
	return "TEXT"
}

// ConvertCreateTable converts a MySQL CREATE TABLE statement to SQLite DDL.
// Returns the SQLite CREATE TABLE statement and a slice of CREATE INDEX statements.
func ConvertCreateTable(stmt string) (create string, indexes []string, err error) {
	tableName, body, err := extractTableParts(stmt)
	if err != nil {
		return "", nil, fmt.Errorf("extractTableParts: %w", err)
	}

	lines := splitBodyLines(body)

	var colDefs []string
	var inlineConstraints []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// strip trailing comma
		line = strings.TrimRight(line, ",")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "PRIMARY KEY"):
			c := convertPrimaryKey(line)
			inlineConstraints = append(inlineConstraints, c)

		case strings.HasPrefix(upper, "UNIQUE KEY") || strings.HasPrefix(upper, "UNIQUE INDEX"):
			idx := convertUniqueIndex(line, tableName)
			if idx != "" {
				indexes = append(indexes, idx)
			}

		case strings.HasPrefix(upper, "KEY ") || strings.HasPrefix(upper, "INDEX "):
			idx := convertIndex(line, tableName)
			if idx != "" {
				indexes = append(indexes, idx)
			}

		case line[0] == '`':
			col, err := convertColumnDef(line)
			if err != nil {
				return "", nil, fmt.Errorf("column %q: %w", line, err)
			}
			colDefs = append(colDefs, col)

		default:
			// skip unknown lines (CONSTRAINT, etc.)
		}
	}

	// Build CREATE TABLE
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(quoteIdent(tableName))
	sb.WriteString(" (\n")

	all := append(colDefs, inlineConstraints...)
	for i, def := range all {
		sb.WriteString("  ")
		sb.WriteString(def)
		if i < len(all)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(")")

	return sb.String(), indexes, nil
}

// extractTableParts extracts the table name and body content from a CREATE TABLE statement.
func extractTableParts(stmt string) (tableName string, body string, err error) {
	// Find table name in backticks after CREATE TABLE
	s := stmt
	upper := strings.ToUpper(s)
	pos := strings.Index(upper, "CREATE TABLE")
	if pos < 0 {
		return "", "", fmt.Errorf("not a CREATE TABLE statement")
	}
	s = s[pos+len("CREATE TABLE"):]
	s = skipWhitespace(s)

	// Optional IF NOT EXISTS
	if strings.HasPrefix(strings.ToUpper(s), "IF NOT EXISTS") {
		s = s[len("IF NOT EXISTS"):]
		s = skipWhitespace(s)
	}

	// Extract table name
	tableName, s, err = extractQuotedIdent(s)
	if err != nil {
		return "", "", fmt.Errorf("table name: %w", err)
	}

	s = skipWhitespace(s)

	// Find opening paren
	if len(s) == 0 || s[0] != '(' {
		return "", "", fmt.Errorf("expected '(' after table name, got %q", s[:min(20, len(s))])
	}

	// Extract content between the outermost parens
	body, err = extractParenContent(s)
	if err != nil {
		return "", "", fmt.Errorf("extractParenContent: %w", err)
	}

	return tableName, body, nil
}

// extractParenContent extracts the content between the outermost ( and ),
// handling nested parens and string literals.
func extractParenContent(s string) (string, error) {
	if len(s) == 0 || s[0] != '(' {
		return "", fmt.Errorf("expected '('")
	}
	depth := 0
	inStr := false
	var body strings.Builder

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == '\\' {
				// skip next char
				body.WriteByte(ch)
				i++
				if i < len(s) {
					body.WriteByte(s[i])
				}
				continue
			}
			if ch == '\'' {
				inStr = false
			}
			body.WriteByte(ch)
			continue
		}
		switch ch {
		case '\'':
			inStr = true
			body.WriteByte(ch)
		case '(':
			depth++
			if depth > 1 {
				body.WriteByte(ch)
			}
		case ')':
			depth--
			if depth == 0 {
				return body.String(), nil
			}
			body.WriteByte(ch)
		default:
			if depth > 0 {
				body.WriteByte(ch)
			}
		}
	}
	return "", fmt.Errorf("unmatched '('")
}

// splitBodyLines splits the CREATE TABLE body into individual definition lines.
// Each column/constraint is on its own line in MySQL dumps.
func splitBodyLines(body string) []string {
	return strings.Split(body, "\n")
}

// convertColumnDef converts a MySQL column definition line to SQLite.
// Input example: `aaid` mediumint(8) unsigned NOT NULL DEFAULT 0
func convertColumnDef(line string) (string, error) {
	tokens := tokenizeLine(line)
	if len(tokens) == 0 {
		return "", fmt.Errorf("empty line")
	}

	// Token 0: column name in backticks
	name := unquoteIdent(tokens[0])
	if name == "" {
		return "", fmt.Errorf("empty column name from %q", tokens[0])
	}

	if len(tokens) < 2 {
		return "", fmt.Errorf("missing type for column %q", name)
	}

	// Token 1: MySQL type (possibly with (args))
	typeToken := tokens[1]
	baseType := extractBaseType(typeToken)
	sqliteType := mapMySQLType(baseType)

	// Build the rest: process modifiers
	var parts []string
	parts = append(parts, quoteIdent(name))
	parts = append(parts, sqliteType)

	i := 2
	for i < len(tokens) {
		tok := tokens[i]
		upper := strings.ToUpper(tok)

		switch upper {
		case "UNSIGNED", "ZEROFILL":
			i++
		case "AUTO_INCREMENT":
			i++
		case "CHARACTER":
			// CHARACTER SET <charset> — skip 3 tokens
			i++
			if i < len(tokens) && strings.ToUpper(tokens[i]) == "SET" {
				i += 2 // skip SET and charset value
			}
		case "COLLATE":
			i += 2 // skip COLLATE and value
		case "COMMENT":
			// COMMENT 'string' — skip both
			i += 2
		case "ON":
			// ON UPDATE CURRENT_TIMESTAMP — skip 3
			i++
			if i < len(tokens) && strings.ToUpper(tokens[i]) == "UPDATE" {
				i += 2
			}
		case "NOT":
			parts = append(parts, "NOT")
			i++
		case "NULL":
			parts = append(parts, "NULL")
			i++
		case "DEFAULT":
			parts = append(parts, "DEFAULT")
			i++
			if i < len(tokens) {
				defVal := convertDefaultValue(tokens[i])
				parts = append(parts, defVal)
				i++
			}
		default:
			// Unknown modifier — skip
			i++
		}
	}

	return strings.Join(parts, " "), nil
}

// convertDefaultValue converts a MySQL DEFAULT value to SQLite-compatible form.
func convertDefaultValue(val string) string {
	upper := strings.ToUpper(val)
	// b'0' or b'1' binary literals → integer
	if strings.HasPrefix(upper, "B'") {
		bits := val[2 : len(val)-1]
		if bits == "0" || bits == "" {
			return "0"
		}
		// Convert binary string to integer
		n := int64(0)
		for _, ch := range bits {
			n = n*2 + int64(ch-'0')
		}
		return fmt.Sprintf("%d", n)
	}
	return val
}

// convertPrimaryKey converts a MySQL PRIMARY KEY definition to SQLite.
// Input: PRIMARY KEY (`col1`,`col2`) USING BTREE
// Output: PRIMARY KEY ("col1","col2")
func convertPrimaryKey(line string) string {
	// Find the ( ... ) part
	start := strings.Index(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end < 0 || end <= start {
		return line
	}
	cols := line[start+1 : end]
	converted := convertColList(cols)
	return "PRIMARY KEY (" + converted + ")"
}

// convertIndex converts a MySQL KEY/INDEX line to a CREATE INDEX statement.
// Input: KEY `idx_name` (`col1`,`col2`)
// Output: CREATE INDEX IF NOT EXISTS "table__idx_name" ON "table" ("col1","col2");
//
// Index names are prefixed with the table name to avoid collisions: SQLite shares
// the namespace for tables and indexes, so a bare index name like "zone" would
// conflict with a table also named "zone".
func convertIndex(line string, tableName string) string {
	name, cols := extractIndexParts(line)
	if name == "" {
		return ""
	}
	qualifiedName := tableName + "__" + name
	return fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (%s)`,
		quoteIdent(qualifiedName), quoteIdent(tableName), cols)
}

// convertUniqueIndex converts a MySQL UNIQUE KEY line to a CREATE UNIQUE INDEX statement.
func convertUniqueIndex(line string, tableName string) string {
	name, cols := extractIndexParts(line)
	if name == "" {
		return ""
	}
	qualifiedName := tableName + "__" + name
	return fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)`,
		quoteIdent(qualifiedName), quoteIdent(tableName), cols)
}

// extractIndexParts extracts the index name and column list from a KEY/INDEX line.
func extractIndexParts(line string) (name string, cols string) {
	tokens := tokenizeLine(line)
	// Find the backtick-quoted name (first backtick token after KEY/UNIQUE KEY/INDEX)
	nameIdx := -1
	for i, t := range tokens {
		upper := strings.ToUpper(t)
		if upper == "KEY" || upper == "INDEX" || upper == "UNIQUE" {
			continue
		}
		if len(t) > 0 && t[0] == '`' {
			nameIdx = i
			break
		}
	}
	if nameIdx < 0 {
		return "", ""
	}
	name = unquoteIdent(tokens[nameIdx])

	// Find the column list in parens
	for i := nameIdx + 1; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) > 0 && t[0] == '(' {
			inner := t[1 : len(t)-1]
			cols = convertColList(inner)
			return name, cols
		}
	}
	return name, ""
}

// convertColList converts a MySQL comma-separated column list to SQLite.
// Converts backtick identifiers to double-quoted, strips length hints like (10).
func convertColList(cols string) string {
	// tokenize the column list
	parts := strings.Split(cols, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// may have (length) suffix: `col`(10) → just the col name
		if idx := strings.Index(p, "("); idx >= 0 {
			p = p[:idx]
		}
		p = strings.TrimSpace(p)
		name := unquoteIdent(p)
		if name != "" {
			result = append(result, quoteIdent(name))
		}
	}
	return strings.Join(result, ", ")
}

// tokenizeLine tokenizes a MySQL column/index definition line into tokens.
// Preserves quoted strings and parenthesized groups as single tokens.
func tokenizeLine(s string) []string {
	var tokens []string
	var cur strings.Builder
	i := 0

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for i < len(s) {
		ch := s[i]

		switch {
		case ch == '`':
			flush()
			cur.WriteByte('`')
			i++
			for i < len(s) && s[i] != '`' {
				cur.WriteByte(s[i])
				i++
			}
			if i < len(s) {
				cur.WriteByte('`')
				i++
			}
			flush()

		case ch == '\'':
			flush()
			cur.WriteByte('\'')
			i++
			for i < len(s) {
				c := s[i]
				cur.WriteByte(c)
				i++
				if c == '\\' && i < len(s) {
					cur.WriteByte(s[i])
					i++
				} else if c == '\'' {
					// check for '' escape
					if i < len(s) && s[i] == '\'' {
						cur.WriteByte('\'')
						i++
					} else {
						break
					}
				}
			}
			flush()

		case ch == '"':
			flush()
			cur.WriteByte('"')
			i++
			for i < len(s) && s[i] != '"' {
				cur.WriteByte(s[i])
				i++
			}
			if i < len(s) {
				cur.WriteByte('"')
				i++
			}
			flush()

		case ch == '(':
			flush()
			depth := 0
			for i < len(s) {
				c := s[i]
				cur.WriteByte(c)
				i++
				if c == '(' {
					depth++
				} else if c == ')' {
					depth--
					if depth == 0 {
						break
					}
				} else if c == '\'' {
					for i < len(s) {
						c2 := s[i]
						cur.WriteByte(c2)
						i++
						if c2 == '\\' && i < len(s) {
							cur.WriteByte(s[i])
							i++
						} else if c2 == '\'' {
							break
						}
					}
				}
			}
			flush()

		case unicode.IsSpace(rune(ch)):
			flush()
			i++

		default:
			cur.WriteByte(ch)
			i++
		}
	}
	flush()
	return tokens
}

// extractBaseType extracts the base type name from a MySQL type token like "mediumint(8)" → "mediumint".
func extractBaseType(typeToken string) string {
	if idx := strings.Index(typeToken, "("); idx >= 0 {
		return typeToken[:idx]
	}
	return typeToken
}

// extractQuotedIdent extracts a backtick-quoted or unquoted identifier from the start of s.
// Returns the identifier, the remaining string, and any error.
func extractQuotedIdent(s string) (ident string, rest string, err error) {
	s = skipWhitespace(s)
	if len(s) == 0 {
		return "", "", fmt.Errorf("empty string")
	}
	if s[0] == '`' {
		end := strings.Index(s[1:], "`")
		if end < 0 {
			return "", "", fmt.Errorf("unclosed backtick identifier")
		}
		return s[1 : end+1], s[end+2:], nil
	}
	// unquoted identifier — read until whitespace or (
	i := 0
	for i < len(s) && !unicode.IsSpace(rune(s[i])) && s[i] != '(' {
		i++
	}
	return s[:i], s[i:], nil
}

// unquoteIdent removes backtick or double-quote wrapping from an identifier.
func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '`' && s[len(s)-1] == '`') ||
			(s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// quoteIdent wraps an identifier in double quotes for SQLite.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// skipWhitespace returns s with leading whitespace removed.
func skipWhitespace(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

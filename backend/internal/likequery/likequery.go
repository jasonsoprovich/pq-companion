// Package likequery turns what a user types in a search box into a SQL LIKE
// pattern: a case-insensitive substring match where * is any run of
// characters and ? is one character, and literal % and _ match themselves.
// Use the pattern with the Escape clause, e.g. `name LIKE ?` + Escape.
package likequery

import (
	"regexp"
	"strings"
)

// escape escapes LIKE wildcards so a user-typed search term is matched
// literally (paired with ESCAPE '\\' in the query).
func escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// searchWildcards turns the search box's wildcards into LIKE ones after the
// text has been escaped: * is any run of characters and ? is one character,
// so "Elemental * Mold" finds every Elemental mold. Literal % and _ stay
// escaped. A few item names end in a literal * (the "Backpack*" variants);
// a search for one still finds it, since * also matches nothing.
var searchWildcards = strings.NewReplacer(`*`, `%`, `?`, `_`)

// starSpaces is a * with the spaces around it. The * takes them in, so
// "Spirit * Wolf" also finds "Spirit Wolf" (a * may match nothing).
var starSpaces = regexp.MustCompile(`\s*\*\s*`)

// Contains is the LIKE pattern (with ESCAPE '\') for a search box
// query: a case-insensitive substring match with * and ? wildcards.
func Contains(q string) string {
	q = starSpaces.ReplaceAllString(q, "*")
	return "%" + searchWildcards.Replace(escape(q)) + "%"
}

// Escape is the clause that goes after `LIKE ?` for a Contains pattern.
const Escape = ` ESCAPE '\'`

// Matcher is Contains for text already in memory: it reports whether s holds
// q as a case-insensitive substring, with the same * and ? wildcards.
func Matcher(q string) func(s string) bool {
	q = strings.ToLower(starSpaces.ReplaceAllString(q, "*"))
	if !strings.ContainsAny(q, "*?") {
		return func(s string) bool { return strings.Contains(strings.ToLower(s), q) }
	}
	var b strings.Builder
	b.WriteString("(?is)")
	for _, r := range q {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re := regexp.MustCompile(b.String())
	return re.MatchString
}

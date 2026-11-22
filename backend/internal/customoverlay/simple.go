package customoverlay

import "github.com/jasonsoprovich/pq-companion/backend/internal/linematch"

// Simple-mode match kinds (SimpleMatch.Kind); see linematch.Simple.
const (
	SimpleStarts   = linematch.SimpleStarts
	SimpleContains = linematch.SimpleContains
	SimpleExact    = linematch.SimpleExact
)

// SimpleMatch is the beginner-friendly way to write a rule's pattern
// (linematch.Simple). Validate turns it into Rule.Pattern, so the engine only
// ever sees a regex, and the editor round-trips what the author typed.
// Custom Overlays use no Simple extensions: {c} captures like any other
// placeholder and * is literal.
type SimpleMatch = linematch.Simple

// simplePattern builds the regex for a SimpleMatch.
func simplePattern(m SimpleMatch) (string, error) {
	return linematch.SimplePattern(m)
}

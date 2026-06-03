package tells

import "regexp"

// reIncoming matches "Peer tells you, 'message'". The name is a single word
// (EQ player names are a single alphabetic token); channel tells such as
// "X tells the raid," or "X tells General:2," never match because they lack
// the literal "tells you," and so are excluded for free.
var reIncoming = regexp.MustCompile(`^([A-Za-z]+) tells you, '(.*)'$`)

// reOutgoing matches "You told Peer, 'message'".
var reOutgoing = regexp.MustCompile(`^You told ([A-Za-z]+), '(.*)'$`)

// incomingExcludes filters NPC merchant/banker/trainer replies and charmed-pet
// command echoes out of incoming tells, so only genuine player conversations
// are stored. Mirrors the "Incoming Tell" trigger's ExcludePatterns in
// internal/trigger/packs.go — keep the two lists in sync.
var incomingExcludes = func() []*regexp.Regexp {
	pats := []string{
		`\b[Mm]aster[.!]`,               // pet command responses (Attacking ... Master.)
		`tells you, '[Tt]hat'll be `,    // NPC merchant: selling price
		`tells you, '[Ii]'ll give you `, // NPC merchant: buying offer
		`tells you, 'I'?m not interested in buying`,
		`tells you, 'Welcome to my bank`,
		`tells you, 'Come back soon`,
		`tells you, 'You cannot afford `,
		`tells you, '?Hold your horses`,
		`tells you, 'I'?m busy`,
		`tells you, 'You have learned the basics`,
		`tells you, 'You have increased your `,
		`tells you, 'You are already browsing`,
		`tells you, 'I charge `,
		`tells you, 'I am unable to wake `,
	}
	out := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}()

// Parsed is the result of matching one log message as a tell.
type Parsed struct {
	Peer      string
	Direction string
	Message   string
}

// ParseTell classifies a bare log message (timestamp already stripped) as a
// direct player tell. Returns (Parsed, true) for genuine incoming/outgoing
// tells, or (zero, false) for channel chatter, NPC replies, pet echoes, and
// any non-tell line.
func ParseTell(msg string) (Parsed, bool) {
	if m := reIncoming.FindStringSubmatch(msg); m != nil {
		for _, ex := range incomingExcludes {
			if ex.MatchString(msg) {
				return Parsed{}, false
			}
		}
		return Parsed{Peer: m[1], Direction: DirectionIn, Message: m[2]}, true
	}
	if m := reOutgoing.FindStringSubmatch(msg); m != nil {
		return Parsed{Peer: m[1], Direction: DirectionOut, Message: m[2]}, true
	}
	return Parsed{}, false
}

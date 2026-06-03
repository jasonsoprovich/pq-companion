package chat

import (
	"regexp"
	"strings"
)

// Channel keys. Named/custom channels (General, Lfg, guild-private channels…)
// are stored as their lowercased base name with the per-user slot number
// dropped, so "General:1" and "General:4" group together as "general".
const (
	ChannelTell    = "tell"
	ChannelGuild   = "guild"
	ChannelRaid    = "raid"
	ChannelGroup   = "group"
	ChannelOOC     = "ooc"
	ChannelAuction = "auction"
	ChannelShout   = "shout"
)

// Direction values.
const (
	DirectionIn  = "in"  // someone else spoke
	DirectionOut = "out" // the local character spoke
)

// Each comma is followed by one OR MORE spaces in EQ logs — raid lines in
// particular use a double space ("tells the raid,  '…'"), so every body match
// uses ,\s+' rather than a literal ", '".
var (
	reTellIn    = regexp.MustCompile(`^([A-Za-z]+) tells you,\s+'(.*)'$`)
	reGuildIn   = regexp.MustCompile(`^([A-Za-z]+) tells the guild,\s+'(.*)'$`)
	reRaidIn    = regexp.MustCompile(`^([A-Za-z]+) tells the raid,\s+'(.*)'$`)
	reGroupIn   = regexp.MustCompile(`^([A-Za-z]+) tells the group,\s+'(.*)'$`)
	reOOCIn     = regexp.MustCompile(`^([A-Za-z]+) says out of character,\s+'(.*)'$`)
	reAuctionIn = regexp.MustCompile(`^([A-Za-z]+) auctions,\s+'(.*)'$`)
	reShoutIn   = regexp.MustCompile(`^([A-Za-z]+) shouts,\s+'(.*)'$`)
	// Named/custom channel, e.g. "Eyden tells General:1, 'msg'".
	reNamedIn = regexp.MustCompile(`^([A-Za-z]+) tells ([A-Za-z][A-Za-z0-9\-]*):\d+,\s+'(.*)'$`)

	reTellOut    = regexp.MustCompile(`^You told ([A-Za-z]+),\s+'(.*)'$`)
	reGuildOut   = regexp.MustCompile(`^You say to your guild,\s+'(.*)'$`)
	reGroupOut   = regexp.MustCompile(`^You tell your party,\s+'(.*)'$`)
	reRaidOut    = regexp.MustCompile(`^You tell the raid,\s+'(.*)'$`)
	reOOCOut     = regexp.MustCompile(`^You say out of character,\s+'(.*)'$`)
	reAuctionOut = regexp.MustCompile(`^You auction,\s+'(.*)'$`)
	reShoutOut   = regexp.MustCompile(`^You shout,\s+'(.*)'$`)
	reNamedOut   = regexp.MustCompile(`^You tell ([A-Za-z][A-Za-z0-9\-]*):\d+,\s+'(.*)'$`)
)

// incomingTellExcludes filters NPC merchant/banker/trainer replies and
// charmed-pet command echoes out of incoming TELLS only (channel chatter is
// never an NPC reply). Mirrors the "Incoming Tell" trigger's ExcludePatterns
// in internal/trigger/packs.go — keep the two lists in sync.
var incomingTellExcludes = func() []*regexp.Regexp {
	pats := []string{
		`\b[Mm]aster[.!]`,
		`tells you, '[Tt]hat'll be `,
		`tells you, '[Ii]'ll give you `,
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

// Parsed is one matched chat line.
type Parsed struct {
	Channel   string
	Direction string
	Peer      string // tell: the other player; channel-in: the speaker; channel-out: ""
	Message   string
}

// CapitalizeName normalizes an EQ player name to canonical casing (first letter
// upper, rest lower). EQ accepts "/tell soandso" but the name is really
// "Soandso"; storing it canonically also merges lowercase variants into one
// conversation.
func CapitalizeName(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

// ParseChat classifies a bare log message (timestamp stripped) as a chat line
// on a tracked channel. Returns (Parsed, true) for genuine player chat, or
// (zero, false) for non-chat lines and NPC/merchant/pet tell replies.
func ParseChat(msg string) (Parsed, bool) {
	// --- Incoming ---
	if m := reTellIn.FindStringSubmatch(msg); m != nil {
		for _, ex := range incomingTellExcludes {
			if ex.MatchString(msg) {
				return Parsed{}, false
			}
		}
		return Parsed{Channel: ChannelTell, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reGuildIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelGuild, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reRaidIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelRaid, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reGroupIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelGroup, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reOOCIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelOOC, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reAuctionIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelAuction, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reShoutIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelShout, Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reNamedIn.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: strings.ToLower(m[2]), Direction: DirectionIn, Peer: CapitalizeName(m[1]), Message: m[3]}, true
	}

	// --- Outgoing (the local character) ---
	if m := reTellOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelTell, Direction: DirectionOut, Peer: CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reGuildOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelGuild, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reGroupOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelGroup, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reRaidOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelRaid, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reOOCOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelOOC, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reAuctionOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelAuction, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reShoutOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: ChannelShout, Direction: DirectionOut, Message: m[1]}, true
	}
	if m := reNamedOut.FindStringSubmatch(msg); m != nil {
		return Parsed{Channel: strings.ToLower(m[1]), Direction: DirectionOut, Message: m[2]}, true
	}

	return Parsed{}, false
}

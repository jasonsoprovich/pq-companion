package linematch

import (
	"regexp"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
)

// ChannelSay is /say, which chat history doesn't store but line rules can
// match. The other channel keys are chat's (chat.ChannelGuild, …), and a
// named channel is its lowercased base name ("general" for General:1).
const ChannelSay = "say"

var (
	reSayIn  = regexp.MustCompile(`^([A-Za-z]+) says,\s+'(.*)'$`)
	reSayOut = regexp.MustCompile(`^You say,\s+'(.*)'$`)
)

// Chat is a log line classified as chat, with the speaker resolved.
type Chat struct {
	Channel   string
	Direction string // chat.DirectionIn or chat.DirectionOut
	Speaker   string // the local character's name for outgoing lines
	// Peer is chat history's peer: for a tell, the other player (so an
	// outgoing tell keeps its recipient); for incoming channel chat, the
	// speaker; "" for outgoing channel chat.
	Peer    string
	Message string
}

// ClassifyChat reports whether a bare log message (timestamp stripped) is a
// chat line, on any channel chat history tracks or /say. self is the active
// character, used as the speaker of outgoing lines.
func ClassifyChat(msg, self string) (Chat, bool) {
	if p, ok := chat.ParseChat(msg); ok {
		c := Chat{Channel: p.Channel, Direction: p.Direction, Message: p.Message, Speaker: p.Peer, Peer: p.Peer}
		if p.Direction == chat.DirectionOut {
			c.Speaker = self
		}
		return c, true
	}
	if m := reSayIn.FindStringSubmatch(msg); m != nil {
		return Chat{Channel: ChannelSay, Direction: chat.DirectionIn, Speaker: chat.CapitalizeName(m[1]), Peer: chat.CapitalizeName(m[1]), Message: m[2]}, true
	}
	if m := reSayOut.FindStringSubmatch(msg); m != nil {
		return Chat{Channel: ChannelSay, Direction: chat.DirectionOut, Speaker: self, Message: m[1]}, true
	}
	return Chat{}, false
}

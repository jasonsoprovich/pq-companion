package chatwindow

import (
	"fmt"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/linematch"
)

// lineFromLog builds a Line from a live log message (timestamp stripped).
// self is the active character.
func lineFromLog(ts time.Time, msg, self string) Line {
	msg = strings.TrimRight(msg, "\r\n")
	if c, ok := linematch.ClassifyChat(msg, self); ok {
		return chatLine(ts.UnixMilli(), self, c.Channel, c.Direction, c.Speaker, c.Peer, c.Message)
	}
	return Line{TS: ts.UnixMilli(), Character: self, Channel: ChannelOther, Text: msg}
}

// lineFromHistory rebuilds a Line from a chat history row (whose ts is in
// seconds). It goes through the same builder as a live line, so the two
// match and dedupe alike.
func lineFromHistory(m chat.Message) Line {
	speaker := m.Peer
	if m.Direction == chat.DirectionOut {
		speaker = m.Character
	}
	return chatLine(m.TS*1000, m.Character, m.Channel, m.Direction, speaker, m.Peer, m.Message)
}

func chatLine(tsMs int64, character, channel, direction, speaker, peer, message string) Line {
	l := Line{
		TS:        tsMs,
		Character: character,
		Channel:   channel,
		Direction: direction,
		Speaker:   speaker,
		Message:   message,
	}
	if channel == ChannelTell {
		l.Peer = peer
	}
	l.Text = chatText(l)
	return l
}

// chatText is the standard text of a chat line, the same however the line
// arrived: one space after the comma (raid lines log two), a named channel's
// slot number dropped and its name capitalized ("General:1" → "General").
func chatText(l Line) string {
	out := l.Direction == chat.DirectionOut
	q := "'" + l.Message + "'"
	switch l.Channel {
	case ChannelTell:
		if out {
			return fmt.Sprintf("You told %s, %s", l.Peer, q)
		}
		return fmt.Sprintf("%s tells you, %s", l.Speaker, q)
	case ChannelGuild:
		if out {
			return "You say to your guild, " + q
		}
		return l.Speaker + " tells the guild, " + q
	case ChannelRaid:
		if out {
			return "You tell your raid, " + q
		}
		return l.Speaker + " tells the raid, " + q
	case ChannelGroup:
		if out {
			return "You tell your party, " + q
		}
		return l.Speaker + " tells the group, " + q
	case ChannelOOC:
		if out {
			return "You say out of character, " + q
		}
		return l.Speaker + " says out of character, " + q
	case ChannelAuction:
		if out {
			return "You auction, " + q
		}
		return l.Speaker + " auctions, " + q
	case ChannelShout:
		if out {
			return "You shout, " + q
		}
		return l.Speaker + " shouts, " + q
	case ChannelSay:
		if out {
			return "You say, " + q
		}
		return l.Speaker + " says, " + q
	}
	name := chat.CapitalizeName(l.Channel)
	if out {
		return fmt.Sprintf("You tell %s, %s", name, q)
	}
	return fmt.Sprintf("%s tells %s, %s", l.Speaker, name, q)
}

// lineKey identifies a line for deduplication where chat history and live
// lines meet. History only keeps whole seconds.
type lineKey struct {
	sec                                                   int64
	character, channel, direction, peer, speaker, message string
}

func keyOf(l Line) lineKey {
	return lineKey{
		sec:       l.TS / 1000,
		character: strings.ToLower(l.Character),
		channel:   l.Channel,
		direction: l.Direction,
		peer:      strings.ToLower(l.Peer),
		speaker:   strings.ToLower(l.Speaker),
		message:   l.Message,
	}
}

// isChat reports whether a line is chat (not ChannelOther).
func (l Line) isChat() bool { return l.Channel != ChannelOther }

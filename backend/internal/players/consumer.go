package players

import (
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// reGroupJoined matches "Soandso has joined the group." — someone joining the
// local character's group. "You have joined the group." is deliberately not
// captured (no other player named on that line).
var reGroupJoined = regexp.MustCompile(`^(\w+) has joined the group\.$`)

// pvpAlertCooldown suppresses repeat warnings for the same player — spamming
// /who shouldn't re-announce a name the user was just warned about.
const pvpAlertCooldown = 60 * time.Second

// Consumer turns log events into Sighting upserts. /who entries are buffered
// until the trailing summary line ("There are N players in <Zone>.") so each
// entry gets stamped with the zone the /who block reported — more reliable
// than tracking zone state from EventZone alone (the backend may have started
// mid-session without yet seeing a zone change).
type Consumer struct {
	store *Store

	mu      sync.Mutex
	zone    string // last-known zone from EventZone / EventWhoSummary
	pending []SightingInput

	onPVP        func(name, zone, source string)
	lastPVPAlert map[string]time.Time // lowercased name → wall-clock time of last alert
}

// NewConsumer constructs a consumer wired to the given store.
func NewConsumer(store *Store) *Consumer {
	return &Consumer{store: store, lastPVPAlert: map[string]time.Time{}}
}

// SetOnPVPSighting registers a callback fired (outside any replay/backfill
// path) when a PVP-flagged player shows up live. source is "who" for /who
// entries; future sources (group joins…) pass their own tag so the alert
// text can say what happened.
func (c *Consumer) SetOnPVPSighting(fn func(name, zone, source string)) {
	c.mu.Lock()
	c.onPVP = fn
	c.mu.Unlock()
}

// Handle is the entry point for the shared logparser event stream.
func (c *Consumer) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventZone:
		zd, ok := ev.Data.(logparser.ZoneData)
		if !ok {
			return
		}
		c.mu.Lock()
		// A zone change while /who entries are still buffered means those
		// entries came from the prior zone — flush them with the old zone
		// before switching state.
		c.flushLocked(c.zone)
		c.zone = zd.ZoneName
		c.mu.Unlock()

	case logparser.EventWhoEntry:
		wd, ok := ev.Data.(logparser.WhoEntryData)
		if !ok {
			return
		}
		c.mu.Lock()
		c.pending = append(c.pending, SightingInput{
			Name:       wd.Name,
			Level:      wd.Level,
			Class:      wd.Class,
			Race:       wd.Race,
			Guild:      wd.Guild,
			Anonymous:  wd.Anonymous,
			Zone:       c.zone, // fallback if summary never arrives
			ObservedAt: ev.Timestamp,
		})
		c.mu.Unlock()

	case logparser.EventWhoSummary:
		ws, ok := ev.Data.(logparser.WhoSummaryData)
		if !ok {
			return
		}
		c.mu.Lock()
		c.zone = ws.Zone
		c.flushLocked(ws.Zone)
		c.mu.Unlock()

	case logparser.EventGuildStat:
		gs, ok := ev.Data.(logparser.GuildStatData)
		if !ok || gs.Player == "" || gs.Guild == "" {
			return
		}
		c.mu.Lock()
		zone := c.zone
		c.mu.Unlock()
		// Guild-only update so we don't blank out class/race/level when the
		// player is already known from a prior /who.
		if err := c.store.UpdateGuild(gs.Player, gs.Guild, zone, ev.Timestamp); err != nil {
			slog.Warn("players: guildstat update failed", "name", gs.Player, "err", err)
		}
	}
}

// HandleLine watches the raw log line stream for group-join messages and
// records them as interactions, so groupmates show up in the tracker even if
// they never appear in a /who. A flagged player joining the group also fires
// the PVP warning.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	m := reGroupJoined.FindStringSubmatch(strings.TrimRight(msg, "\r\n"))
	if m == nil {
		return
	}
	name := m[1]
	if name == "You" {
		return
	}
	if err := c.store.TouchInteraction(name, InteractionGroup, ts); err != nil {
		slog.Warn("players: group interaction failed", "name", name, "err", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.onPVP == nil {
		return
	}
	flagged, err := c.store.PVPNames()
	if err != nil {
		slog.Warn("players: pvp flag lookup failed", "err", err)
		return
	}
	if flagged[strings.ToLower(name)] {
		c.alertPVPLocked(name, c.zone, "group")
	}
}

// RecordTell records a direct tell exchanged with another player (either
// direction). Wired from the chat consumer's insert hook in main.
func (c *Consumer) RecordTell(peer string, ts time.Time) {
	if peer == "" {
		return
	}
	if err := c.store.TouchInteraction(peer, InteractionTell, ts); err != nil {
		slog.Warn("players: tell interaction failed", "name", peer, "err", err)
	}
}

// flushLocked drains the pending buffer, upserting each entry under the
// supplied zone, then fires PVP warnings for any flagged names in the batch.
// Caller must hold c.mu.
func (c *Consumer) flushLocked(zone string) {
	if len(c.pending) == 0 {
		return
	}
	for _, in := range c.pending {
		in.Zone = zone
		if err := c.store.Upsert(in); err != nil {
			slog.Warn("players: upsert failed", "name", in.Name, "err", err)
		}
	}
	if c.onPVP != nil {
		flagged, err := c.store.PVPNames()
		if err != nil {
			slog.Warn("players: pvp flag lookup failed", "err", err)
		} else if len(flagged) > 0 {
			for _, in := range c.pending {
				if flagged[strings.ToLower(in.Name)] {
					c.alertPVPLocked(in.Name, zone, "who")
				}
			}
		}
	}
	c.pending = c.pending[:0]
}

// alertPVPLocked fires the PVP callback for a flagged name unless one was
// already fired for that name inside the cooldown window. Caller must hold
// c.mu.
func (c *Consumer) alertPVPLocked(name, zone, source string) {
	key := strings.ToLower(name)
	now := time.Now()
	if last, ok := c.lastPVPAlert[key]; ok && now.Sub(last) < pvpAlertCooldown {
		return
	}
	c.lastPVPAlert[key] = now
	c.onPVP(name, zone, source)
}

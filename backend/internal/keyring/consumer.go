package keyring

import (
	"log/slog"
	"sync"
	"time"
)

// FlushIdle is the duration of inactivity after which a buffered /keys burst
// is committed even if no further log lines arrive. /keys output drops in
// one tight burst (every line stamped to the same second in observed logs),
// so 1.5s is plenty of headroom over the natural burst spacing while still
// committing quickly enough to feel responsive.
const FlushIdle = 1500 * time.Millisecond

// Consumer turns raw log lines into per-character keyring snapshots.
//
// Detection model: every incoming log line is exact-matched against the
// keyring_data.key_name set. Matching lines are buffered; the buffer is
// flushed (and committed as a snapshot replacing the character's entries)
// on the first non-matching line OR after FlushIdle of no new matches.
//
// Why exact-match + burst: EQ emits no header/footer around /keys output
// and doesn't echo the command itself. The only signature is "many key_name
// lines in a row, same timestamp, no other content in between". Exact-match
// is the safest discriminator — chat lines that literally consist of just a
// key name with no surrounding text are vanishingly rare. The 1.5s idle
// flush handles the case where the player /keys then sits idle.
type Consumer struct {
	store      *Store
	nameIndex  map[string]int // key_name -> key_item (canonical id when duplicates collapse)
	activeChar func() string  // returns the current in-game character name; "" disables snapshots
	onSnapshot func(string)   // optional, fired after a successful Snapshot with the character name

	mu          sync.Mutex
	buffer      map[int]bool // key_items in the in-progress burst
	character   string       // active character snapshotted when the burst started
	lastMatchAt time.Time
	timer       *time.Timer
}

// SetOnSnapshot registers a callback fired after each successful snapshot
// commit. Used by the API layer to broadcast a WebSocket event so the UI
// can refresh the keyring tab in place instead of relying on a tab-switch
// remount to refetch. Safe to call before HandleLine starts feeding data.
func (c *Consumer) SetOnSnapshot(fn func(character string)) {
	c.mu.Lock()
	c.onSnapshot = fn
	c.mu.Unlock()
}

// NewConsumer constructs a consumer wired to store and using master as the
// match dictionary. activeChar should return the currently active character
// name (typically tailer.ActiveCharacter); when it returns empty the burst
// is still buffered but the snapshot is suppressed on flush so we don't
// commit a snapshot under no character.
func NewConsumer(store *Store, master []MasterEntry, activeChar func() string) *Consumer {
	return &Consumer{
		store:      store,
		nameIndex:  NameIndex(master),
		activeChar: activeChar,
		buffer:     make(map[int]bool),
	}
}

// HandleLine processes one raw log line. The tailer feeds this for every
// line that successfully parsed a timestamp, regardless of whether it also
// produced a typed event — same path the trigger engine uses.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	if msg == "" {
		return
	}
	if keyItem, isKey := c.nameIndex[msg]; isKey {
		c.appendMatch(keyItem, ts)
		return
	}
	// A non-matching line ends a burst. If nothing was buffered this is a
	// cheap no-op; otherwise we flush synchronously so the snapshot lands
	// before the next /keys can overwrite it.
	c.flush()
}

func (c *Consumer) appendMatch(keyItem int, ts time.Time) {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		// First match in a new burst — snapshot the active character now so
		// a mid-burst /camp+login can't move the snapshot to the wrong row.
		c.character = ""
		if c.activeChar != nil {
			c.character = c.activeChar()
		}
	}
	c.buffer[keyItem] = true
	c.lastMatchAt = ts
	if c.timer == nil {
		c.timer = time.AfterFunc(FlushIdle, c.flush)
	} else {
		c.timer.Reset(FlushIdle)
	}
	c.mu.Unlock()
}

// flush is called from three places — a non-matching log line, the idle
// AfterFunc, and Shutdown — so it locks internally and is safe to call
// when the buffer is empty.
func (c *Consumer) flush() {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	items := make([]int, 0, len(c.buffer))
	for k := range c.buffer {
		items = append(items, k)
	}
	character := c.character
	observedAt := c.lastMatchAt
	c.buffer = make(map[int]bool)
	c.character = ""
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()

	if character == "" {
		// Burst was buffered before the active character was known. Drop
		// it rather than commit under an empty name (which would be
		// invisible in the UI and would still get re-overwritten on the
		// next /keys anyway).
		slog.Debug("keyring: skipped snapshot — no active character", "items", len(items))
		return
	}
	if err := c.store.Snapshot(character, items, observedAt); err != nil {
		slog.Warn("keyring: snapshot failed", "character", character, "items", len(items), "err", err)
		return
	}
	slog.Info("keyring: snapshot committed", "character", character, "items", len(items))
	c.mu.Lock()
	cb := c.onSnapshot
	c.mu.Unlock()
	if cb != nil {
		cb(character)
	}
}

// Shutdown flushes any in-progress burst and stops the idle timer. Call
// before the process exits so a /keys that landed seconds before shutdown
// isn't lost. Safe to call multiple times.
func (c *Consumer) Shutdown() {
	c.flush()
}

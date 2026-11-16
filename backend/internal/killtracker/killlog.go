package killtracker

import (
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// NPCClassifier reports whether a log display name ("Darwol Adan") resolves
// to an npc_types row. Injected so the package stays free of the game DB and
// tests can stub it. nil treats every target as an NPC.
type NPCClassifier func(displayName string) bool

// ZoneLookup returns the zone a character is in right now, from outside the
// event stream (the newest zone-in line in their log). Used when the logger
// starts mid-session or the active character changes: the tailer opens a
// newly active log at EOF, so the login's "You have entered …" line is never
// dispatched and kills would otherwise carry the previous character's zone.
type ZoneLookup func(character string) string

// recorder turns EventKill events into kill rows. Shared by the live Logger
// and the BackfillHandler so both stamp identical rows — which is what lets
// the store's unique index dedup a kill seen live and again by a backfill.
type recorder struct {
	store    *Store
	classify NPCClassifier

	zone string
	// zoneChar is the character zone belongs to; lookupZone, when set, seeds
	// zone whenever that character changes (live logger only — a backfill
	// replays from the top and must learn zones in order).
	zoneChar   string
	zoneSeeded bool
	lookupZone ZoneLookup

	// seq numbering: the Nth identical (target, killer) kill within the same
	// second gets seq N-1. Re-reading the same log from the top rebuilds the
	// same numbers, so a replay dedups while an AE pull of three same-name
	// mobs in one second still stores three rows.
	seqSecond int64
	seqSeen   map[string]int

	npcCache map[string]bool
}

func newRecorder(store *Store, classify NPCClassifier) *recorder {
	return &recorder{store: store, classify: classify, seqSeen: map[string]int{}, npcCache: map[string]bool{}}
}

// handleEvent records ev when it is a kill and tracks zone changes. Returns
// the stored entry when a new row was written.
func (r *recorder) handleEvent(character string, ev logparser.LogEvent) (Entry, bool) {
	if !r.zoneSeeded || !strings.EqualFold(character, r.zoneChar) {
		r.zoneSeeded = true
		r.zoneChar = character
		r.zone = ""
		if r.lookupZone != nil && character != "" && ev.Type != logparser.EventZone {
			r.zone = r.lookupZone(character)
		}
	}
	switch ev.Type {
	case logparser.EventZone:
		if zd, ok := ev.Data.(logparser.ZoneData); ok && logparser.IsZoneName(zd.ZoneName) {
			r.zone = zd.ZoneName
		}
		return Entry{}, false
	case logparser.EventKill:
	default:
		return Entry{}, false
	}
	kd, ok := ev.Data.(logparser.KillData)
	if !ok {
		return Entry{}, false
	}
	target := strings.TrimSpace(kd.Target)
	if target == "" {
		return Entry{}, false
	}
	killer := strings.TrimSpace(kd.Killer)
	if strings.EqualFold(killer, "you") && character != "" {
		killer = character
	}

	sec := ev.Timestamp.Unix()
	if sec != r.seqSecond {
		r.seqSecond = sec
		clear(r.seqSeen)
	}
	key := strings.ToLower(target) + "\x00" + strings.ToLower(killer)
	seq := r.seqSeen[key]
	r.seqSeen[key] = seq + 1

	isNPC := r.isNPC(target)
	e, inserted, err := r.store.Insert(Input{
		Character: character,
		Target:    target,
		Killer:    killer,
		Zone:      r.zone,
		TS:        ev.Timestamp,
		Seq:       seq,
		IsNPC:     isNPC,
		Named:     isNPC && isProperName(target),
	})
	if err != nil {
		slog.Warn("killtracker: insert failed", "target", target, "err", err)
		return Entry{}, false
	}
	return e, inserted
}

func (r *recorder) isNPC(target string) bool {
	if r.classify == nil {
		return true
	}
	key := strings.ToLower(target)
	if v, ok := r.npcCache[key]; ok {
		return v
	}
	v := r.classify(target)
	// Bounded in practice by the number of distinct mob names a player meets;
	// the cap is only a guard against pathological input.
	if len(r.npcCache) > 20000 {
		clear(r.npcCache)
	}
	r.npcCache[key] = v
	return v
}

// isProperName reports whether an EQ display name reads as a named mob: it
// starts with a capital letter and doesn't lead with an article ("a gnoll",
// "an owlbear", "A Shissar Wrath"). Capitalized trash that has no article
// (Bertox's "Knight of Decay") still passes — there is no reliable column in
// npc_types (rare_spawn / raid_target are unset on Quarm) to do better.
func isProperName(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError || !unicode.IsUpper(r) {
		return false
	}
	lower := strings.ToLower(name)
	return !strings.HasPrefix(lower, "a ") && !strings.HasPrefix(lower, "an ")
}

// Logger is the live Kill Log consumer.
type Logger struct {
	activeChar func() string

	mu       sync.Mutex
	rec      *recorder
	onInsert func(Entry)
}

// NewLogger constructs a live kill logger. activeChar returns the current
// in-game character (used to scope rows and attribute "You" kills).
func NewLogger(store *Store, classify NPCClassifier, activeChar func() string) *Logger {
	return &Logger{activeChar: activeChar, rec: newRecorder(store, classify)}
}

// SetOnInsert registers a callback fired after each newly stored kill row,
// used to broadcast a WebSocket event so the Kill Log updates live.
func (l *Logger) SetOnInsert(fn func(Entry)) {
	l.mu.Lock()
	l.onInsert = fn
	l.mu.Unlock()
}

// SetZoneLookup registers how to find the active character's current zone
// when the logger starts or the character changes (see ZoneLookup).
func (l *Logger) SetZoneLookup(fn ZoneLookup) {
	l.mu.Lock()
	l.rec.lookupZone = fn
	l.mu.Unlock()
}

// HandleEvent records kills and tracks the zone from the parsed-event stream.
func (l *Logger) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventKill && ev.Type != logparser.EventZone {
		return
	}
	character := ""
	if l.activeChar != nil {
		character = l.activeChar()
	}
	l.mu.Lock()
	e, inserted := l.rec.handleEvent(character, ev)
	onInsert := l.onInsert
	l.mu.Unlock()
	if inserted && onInsert != nil {
		onInsert(e)
	}
}

// BackfillHandler replays a character's log to store past kills. It satisfies
// backfill.Handler.
type BackfillHandler struct {
	character string
	rec       *recorder
	inserted  int
}

// NewBackfillHandler returns a handler that attributes stored kills to
// character.
func NewBackfillHandler(store *Store, classify NPCClassifier, character string) *BackfillHandler {
	return &BackfillHandler{character: character, rec: newRecorder(store, classify)}
}

// HandleEvent stores kills and tracks zone changes.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if _, ok := h.rec.handleEvent(h.character, ev); ok {
		h.inserted++
	}
}

// HandleLine is a no-op; kills come from the parsed-event stream.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize is a no-op; kills are inserted as they're seen.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many new kill rows were stored.
func (h *BackfillHandler) Inserted() int { return h.inserted }

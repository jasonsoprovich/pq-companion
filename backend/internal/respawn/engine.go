package respawn

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// broadcastInterval is how often the engine recomputes remaining time and
// pushes state to WebSocket clients while timers are active.
const broadcastInterval = time.Second

// graceWindow is how long a timer stays visible at 0:00 ("POP") after its
// estimated respawn before it is auto-pruned. Manual removal works at any time.
const graceWindow = 60 * time.Second

// keySep separates zone and name in the per-(zone,name) numbering key. Chosen
// so it can't appear in either component.
const keySep = "\x00"

// Engine watches parsed log events for kills, starts a respawn countdown for
// each killed NPC that has spawn data in the current zone, and broadcasts state
// once per second. It mirrors spelltimer.Engine's lifecycle.
type Engine struct {
	hub *ws.Hub
	db  *db.DB

	mu     sync.Mutex
	timers map[string]*RespawnTimer // keyed by RespawnTimer.ID

	// nextIndex holds the next label number per zone+name key. Reset to 0
	// (deleted) once no timers remain for that key.
	nextIndex map[string]int

	// Zone state. The Zeal pipe (when connected) is authoritative; the log's
	// "You have entered" line is the fallback. currentZone() prefers the pipe.
	pipeZoneShort string
	pipeZoneLong  string
	logZoneShort  string
	logZoneLong   string
}

// NewEngine returns an initialised Engine ready to receive log events.
func NewEngine(hub *ws.Hub, database *db.DB) *Engine {
	return &Engine{
		hub:       hub,
		db:        database,
		timers:    make(map[string]*RespawnTimer),
		nextIndex: make(map[string]int),
	}
}

// Start runs the background ticker that prunes elapsed timers and broadcasts
// current state once per second. Blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pruneExpired()
			e.broadcast()
		}
	}
}

// Handle processes a single parsed log event. Only zone changes and kills are
// relevant.
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventZone:
		zd, ok := ev.Data.(logparser.ZoneData)
		if !ok {
			return
		}
		e.setLogZone(zd.ZoneName)

	case logparser.EventKill:
		kd, ok := ev.Data.(logparser.KillData)
		if !ok || kd.Target == "" {
			return
		}
		e.onKill(kd.Target, ev.Timestamp)
	}
}

// SetPipeZone records the player's current zone from the Zeal pipe player
// snapshot (a zoneidnumber). Resolved to a short/long name via the DB; an
// unknown or zero id leaves the pipe zone empty so the log fallback applies.
func (e *Engine) SetPipeZone(zoneIDNumber int) {
	var short, long string
	if zoneIDNumber > 0 && e.db != nil {
		if z, err := e.db.GetZoneByZoneIDNumber(zoneIDNumber); err == nil && z != nil {
			short = z.ShortName
			long = z.LongName
		}
	}
	e.mu.Lock()
	e.pipeZoneShort = short
	e.pipeZoneLong = long
	e.mu.Unlock()
}

// ResetPipeZone clears the pipe-derived zone when Zeal disconnects, so the
// log-driven zone takes over again.
func (e *Engine) ResetPipeZone() {
	e.mu.Lock()
	e.pipeZoneShort = ""
	e.pipeZoneLong = ""
	e.mu.Unlock()
}

// setLogZone resolves the "You have entered <long>." text to a short_name and
// stores it as the fallback zone.
func (e *Engine) setLogZone(longName string) {
	short := ""
	if e.db != nil {
		if s, err := e.db.GetZoneShortNameByLongName(longName); err == nil {
			short = s
		}
	}
	e.mu.Lock()
	e.logZoneShort = short
	e.logZoneLong = longName
	e.mu.Unlock()
}

// currentZone returns the effective (short, long) zone, preferring the Zeal
// pipe over the log. Caller must NOT hold e.mu.
func (e *Engine) currentZone() (short, long string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pipeZoneShort != "" {
		return e.pipeZoneShort, e.pipeZoneLong
	}
	return e.logZoneShort, e.logZoneLong
}

// onKill resolves the killed NPC's respawn time for the current zone and starts
// a countdown. Mobs with no spawn data in the zone are skipped (no timer).
func (e *Engine) onKill(displayName string, diedAt time.Time) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || e.db == nil {
		return
	}
	zoneShort, zoneLong := e.currentZone()
	if zoneShort == "" {
		slog.Info("respawn: kill ignored (zone unknown)", "npc", displayName)
		return
	}

	dbName := strings.ReplaceAll(displayName, " ", "_")
	infos, err := e.db.GetRespawnTimesInZone(dbName, zoneShort)
	if err != nil {
		slog.Warn("respawn: DB error looking up respawn times",
			"npc", dbName, "zone", zoneShort, "err", err)
		return
	}
	if len(infos) == 0 {
		// No spawn data (trash, players slain by mobs, named with no DB row,
		// or wrong zone). Per design we don't create a timer.
		slog.Info("respawn: kill skipped (no respawn data)",
			"npc", dbName, "zone", zoneShort)
		return
	}

	estimate, ambiguous, minS, maxS, npcID := summarize(infos)
	if estimate <= 0 {
		return
	}

	e.mu.Lock()
	key := zoneShort + keySep + displayName
	e.nextIndex[key]++
	idx := e.nextIndex[key]
	id := fmt.Sprintf("%s|%s|%d", zoneShort, displayName, idx)
	e.timers[id] = &RespawnTimer{
		ID:              id,
		NPCName:         displayName,
		LabelIndex:      idx,
		NPCID:           npcID,
		Zone:            zoneShort,
		ZoneName:        zoneLong,
		DiedAt:          diedAt,
		RespawnAt:       diedAt.Add(time.Duration(estimate) * time.Second),
		DurationSeconds: float64(estimate),
		Ambiguous:       ambiguous,
		MinSeconds:      minS,
		MaxSeconds:      maxS,
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Info("respawn: timer started",
		"npc", displayName, "zone", zoneShort, "index", idx,
		"estimate_sec", estimate, "ambiguous", ambiguous)
	e.hub.Broadcast(ws.Event{Type: WSEventRespawns, Data: snap})
}

// summarize reduces the set of spawn rows for a name+zone into a single
// estimate (the most common respawn time, ties broken toward the shortest),
// an ambiguity flag (more than one distinct respawn time), the min/max range
// across the distinct times, and a representative npc_types.id.
func summarize(infos []db.RespawnInfo) (estimate int, ambiguous bool, minS, maxS, npcID int) {
	counts := make(map[int]int)
	for _, ri := range infos {
		if ri.RespawnTime <= 0 {
			continue
		}
		counts[ri.RespawnTime]++
		if npcID == 0 {
			npcID = ri.NPCID
		}
		if minS == 0 || ri.RespawnTime < minS {
			minS = ri.RespawnTime
		}
		if ri.RespawnTime > maxS {
			maxS = ri.RespawnTime
		}
	}
	if len(counts) == 0 {
		return 0, false, 0, 0, npcID
	}
	// Pick the mode; on a tie prefer the smaller respawn time.
	bestCount := -1
	for t, c := range counts {
		if c > bestCount || (c == bestCount && t < estimate) {
			bestCount = c
			estimate = t
		}
	}
	ambiguous = len(counts) > 1
	if !ambiguous {
		minS, maxS = 0, 0
	}
	return estimate, ambiguous, minS, maxS, npcID
}

// GetState returns a point-in-time snapshot of all active respawn timers.
func (e *Engine) GetState() RespawnState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot(time.Now())
}

// RemoveByID removes a single timer by its ID (the per-row dismiss button).
// Returns true if a timer was removed.
func (e *Engine) RemoveByID(id string) bool {
	if id == "" {
		return false
	}
	e.mu.Lock()
	_, had := e.timers[id]
	if had {
		delete(e.timers, id)
		e.resetEmptyIndexesLocked()
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if had {
		e.hub.Broadcast(ws.Event{Type: WSEventRespawns, Data: snap})
	}
	return had
}

// Clear removes every active timer and broadcasts the empty state.
func (e *Engine) Clear() {
	e.mu.Lock()
	wasEmpty := len(e.timers) == 0
	e.timers = make(map[string]*RespawnTimer)
	e.nextIndex = make(map[string]int)
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if !wasEmpty {
		e.hub.Broadcast(ws.Event{Type: WSEventRespawns, Data: snap})
	}
}

// pruneExpired removes timers whose respawn (plus grace window) has elapsed.
func (e *Engine) pruneExpired() {
	now := time.Now()
	e.mu.Lock()
	for id, t := range e.timers {
		if now.After(t.RespawnAt.Add(graceWindow)) {
			delete(e.timers, id)
		}
	}
	e.resetEmptyIndexesLocked()
	e.mu.Unlock()
}

// resetEmptyIndexesLocked drops numbering state for any zone+name that no
// longer has an active timer, so labels restart at 01 next time. Caller must
// hold e.mu.
func (e *Engine) resetEmptyIndexesLocked() {
	active := make(map[string]bool, len(e.timers))
	for _, t := range e.timers {
		active[t.Zone+keySep+t.NPCName] = true
	}
	for key := range e.nextIndex {
		if !active[key] {
			delete(e.nextIndex, key)
		}
	}
}

func (e *Engine) broadcast() {
	e.mu.Lock()
	snap := e.snapshot(time.Now())
	e.mu.Unlock()
	e.hub.Broadcast(ws.Event{Type: WSEventRespawns, Data: snap})
}

// snapshot builds an immutable RespawnState. Must be called with e.mu held.
func (e *Engine) snapshot(now time.Time) RespawnState {
	curZone := e.pipeZoneShort
	if curZone == "" {
		curZone = e.logZoneShort
	}

	timers := make([]RespawnTimer, 0, len(e.timers))
	for _, t := range e.timers {
		remaining := t.RespawnAt.Sub(now).Seconds()
		if remaining < 0 {
			remaining = 0
		}
		entry := *t
		entry.RemainingSeconds = remaining
		timers = append(timers, entry)
	}

	// Current-zone timers first, then most imminent respawn first.
	sort.SliceStable(timers, func(i, j int) bool {
		iCur := timers[i].Zone == curZone
		jCur := timers[j].Zone == curZone
		if iCur != jCur {
			return iCur
		}
		return timers[i].RemainingSeconds < timers[j].RemainingSeconds
	})

	return RespawnState{
		Timers:      timers,
		CurrentZone: curZone,
		LastUpdated: now,
	}
}

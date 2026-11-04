package raidtrack

import (
	"context"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

const (
	// frameInterval is how often a frame is sampled.
	frameInterval = 3 * time.Second

	// rosterFresh is how long the last raid roster stands in for the live
	// one. Zeal sends it ~10 times a second while in a raid.
	rosterFresh = 10 * time.Second

	// sessionGap ends a session once no frame has been recorded for this
	// long (left the raid, camped, Zeal gone).
	sessionGap = 2 * time.Minute

	// chunkSpan is how much time one stored chunk covers.
	chunkSpan = time.Minute

	// minFrames is the shortest session worth keeping (one minute). A
	// shorter one, such as passing through a raid zone, is dropped.
	minFrames = 20

	// targetFresh is how long the last target name and position stand in
	// for the live ones. Zeal sends both several times a second.
	targetFresh = 2 * time.Second
)

// Complete Heal's landing lines (spells_new cast_on_you / cast_on_other).
var (
	reCHLandOther = regexp.MustCompile(`^([A-Z][a-zA-Z]{2,14}) is completely healed\.$`)
	reCHLandSelf  = regexp.MustCompile(`^You are completely healed\.$`)
)

// Member is one raider from a Zeal raid frame. HasLoc is false for raiders
// outside the observer's zone. X/Y are GameX/GameY.
type Member struct {
	Name    string
	Class   int
	Level   int
	Group   int
	HasLoc  bool
	X, Y, Z float64
	Heading float64
	HPPct   int // -1 unknown
}

// Recorder samples raid frames into the Store.
type Recorder struct {
	store    *Store
	settings func() config.RaidReplaySettings
	now      func() time.Time
	logChar  func() string

	mu        sync.Mutex
	roster    []Member
	rosterAt  time.Time
	character string
	zone      string
	sess      *session

	// The observer's Zeal target: its name from the target label, and its
	// position from the player snapshot when Zeal reports one.
	targetName   string
	targetNameAt time.Time
	targetLoc    [3]float64
	targetLocAt  time.Time
}

type session struct {
	id          int64
	character   string
	zone        string
	index       map[string]int // raider name (lower case) → member index
	chunk       []Frame
	frames      int
	peak        int
	lastFrameAt time.Time
}

// NewRecorder returns a recorder writing to store. settings is read on each
// tick so changes apply without a restart.
func NewRecorder(store *Store, settings func() config.RaidReplaySettings) *Recorder {
	return &Recorder{store: store, settings: settings, now: time.Now}
}

// SetLogCharacter names whose log feeds Handle. When boxing, Zeal's pipe can
// belong to one client while the active log is another's, so "You" in a log
// line is the log's character, not the pipe's.
func (r *Recorder) SetLogCharacter(fn func() string) {
	r.mu.Lock()
	r.logChar = fn
	r.mu.Unlock()
}

// SetRaid takes the latest Zeal raid roster. Called for every raid frame.
func (r *Recorder) SetRaid(character, zone string, members []Member) {
	r.mu.Lock()
	r.roster = members
	r.rosterAt = r.now()
	r.character = character
	r.zone = zone
	r.mu.Unlock()
}

// SetTargetName takes the observer's target name from Zeal's target label;
// "" means no target.
func (r *Recorder) SetTargetName(name string) {
	r.mu.Lock()
	if !strings.EqualFold(name, r.targetName) {
		r.targetLocAt = time.Time{}
	}
	r.targetName = name
	r.targetNameAt = r.now()
	r.mu.Unlock()
}

// SetTargetLoc takes the observer's target position (GameX, GameY, Z) from
// Zeal's target descriptors. name is the descriptors' target name, which is
// checked against the label's so a position isn't pinned on the wrong mob
// during a retarget; "" skips the check.
func (r *Recorder) SetTargetLoc(name string, x, y, z float64) {
	r.mu.Lock()
	if name == "" || strings.EqualFold(name, r.targetName) {
		r.targetLoc = [3]float64{x, y, z}
		r.targetLocAt = r.now()
	}
	r.mu.Unlock()
}

// HandleCHCall records a CH chain call (from the chain matcher) as a mark
// on the timeline.
func (r *Recorder) HandleCHCall(ts time.Time, caster, target string) {
	r.addEvent(Event{T: ts.UnixMilli(), Kind: EventCH, Name: target, Other: caster})
}

// HandleLine records Complete Heal landings from raw log lines.
func (r *Recorder) HandleLine(ts time.Time, msg string) {
	if m := reCHLandOther.FindStringSubmatch(msg); m != nil {
		r.addEvent(Event{T: ts.UnixMilli(), Kind: EventCHLand, Name: m[1]})
	} else if reCHLandSelf.MatchString(msg) {
		r.addEvent(Event{T: ts.UnixMilli(), Kind: EventCHLand, Name: "You"})
	}
}

// addEvent writes ev to the running session, naming "You" after the log's
// character.
func (r *Recorder) addEvent(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sess
	if s == nil {
		return
	}
	me := r.logCharLocked(s)
	if ev.Name == "You" {
		ev.Name = me
	}
	if ev.Other == "You" {
		ev.Other = me
	}
	if err := r.store.addEvent(s.id, ev); err != nil {
		slog.Warn("raidtrack: event write failed", "err", err)
	}
}

// logCharLocked is the character "You" means in a log line. Caller holds
// r.mu.
func (r *Recorder) logCharLocked(s *session) string {
	if r.logChar != nil {
		if n := r.logChar(); n != "" {
			return n
		}
	}
	return s.character
}

// Handle records kills and deaths from the log into the running session.
func (r *Recorder) Handle(ev logparser.LogEvent) {
	var out Event
	switch d := ev.Data.(type) {
	case logparser.KillData:
		out = Event{Kind: EventKill, Name: d.Target, Other: d.Killer}
	case logparser.DeathData:
		out = Event{Kind: EventDeath, Other: d.SlainBy}
	default:
		return
	}
	at := ev.Timestamp
	if at.IsZero() {
		at = r.now()
	}
	out.T = at.UnixMilli()

	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sess
	if s == nil {
		return
	}
	me := r.logCharLocked(s)
	switch {
	case out.Kind == EventDeath:
		out.Name = me
	case out.Name == "You" || strings.EqualFold(out.Name, me):
		out.Kind, out.Name = EventDeath, me
	default:
		if _, ok := s.index[strings.ToLower(out.Name)]; ok {
			out.Kind = EventDeath
		}
	}
	if out.Other == "You" {
		out.Other = me
	}
	if err := r.store.addEvent(s.id, out); err != nil {
		slog.Warn("raidtrack: event write failed", "err", err)
	}
}

// Start samples frames until ctx is cancelled, then closes the session.
func (r *Recorder) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.endLocked()
			r.mu.Unlock()
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Recorder) tick() {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.settings != nil && r.settings().Disabled {
		r.endLocked()
		return
	}
	live := !r.rosterAt.IsZero() && now.Sub(r.rosterAt) <= rosterFresh && r.zone != ""
	if s := r.sess; s != nil {
		moved := live && (s.zone != r.zone || s.character != r.character)
		if moved || (!s.lastFrameAt.IsZero() && now.Sub(s.lastFrameAt) > sessionGap) {
			r.endLocked()
		}
	}
	if !live {
		return
	}
	if s := r.sess; s != nil && now.Sub(s.lastFrameAt) < frameInterval {
		return
	}
	r.recordLocked(now)
}

// recordLocked samples the current roster into a frame. Caller holds r.mu.
func (r *Recorder) recordLocked(now time.Time) {
	var inZone []Member
	for _, m := range r.roster {
		if m.HasLoc && m.Name != "" {
			inZone = append(inZone, m)
		}
	}
	if len(inZone) == 0 {
		return
	}
	if r.sess == nil {
		id, err := r.store.createSession(r.character, r.zone, now)
		if err != nil {
			slog.Warn("raidtrack: session create failed", "err", err)
			return
		}
		r.sess = &session{id: id, character: r.character, zone: r.zone, index: map[string]int{}}
		slog.Info("raidtrack: recording started", "zone", r.zone, "character", r.character)
	}
	s := r.sess
	// Every raider gets an index, in zone or not, so a death of someone
	// who hasn't zoned in yet still reads as a raid death.
	for _, m := range r.roster {
		key := strings.ToLower(m.Name)
		if m.Name == "" {
			continue
		}
		if _, ok := s.index[key]; ok {
			continue
		}
		idx := len(s.index)
		s.index[key] = idx
		if err := r.store.addMember(s.id, MemberInfo{Index: idx, Name: m.Name, Class: m.Class, Level: m.Level}); err != nil {
			slog.Warn("raidtrack: member write failed", "err", err)
		}
	}
	f := Frame{T: now.UnixMilli(), M: make([][]int, 0, len(inZone))}
	for _, m := range inZone {
		f.M = append(f.M, []int{
			s.index[strings.ToLower(m.Name)],
			round(m.X), round(m.Y), round(m.Z), round(m.Heading),
			m.Group, m.HPPct,
		})
	}
	if r.targetName != "" && now.Sub(r.targetNameAt) <= targetFresh {
		f.Tg = &FrameTarget{N: r.targetName}
		if !r.targetLocAt.IsZero() && now.Sub(r.targetLocAt) <= targetFresh {
			f.Tg.L = []int{round(r.targetLoc[0]), round(r.targetLoc[1]), round(r.targetLoc[2])}
		}
	}
	if len(s.chunk) > 0 && f.T-s.chunk[0].T >= chunkSpan.Milliseconds() {
		r.flushLocked()
	}
	s.chunk = append(s.chunk, f)
	s.frames++
	if len(f.M) > s.peak {
		s.peak = len(f.M)
	}
	s.lastFrameAt = now
}

func (r *Recorder) flushLocked() {
	s := r.sess
	if s == nil || len(s.chunk) == 0 {
		return
	}
	if err := r.store.writeChunk(s.id, s.chunk, s.frames, s.peak); err != nil {
		slog.Warn("raidtrack: chunk write failed", "err", err)
	}
	s.chunk = nil
}

// endLocked closes the running session, dropping it when it's too short to
// be a raid. Caller holds r.mu.
func (r *Recorder) endLocked() {
	s := r.sess
	if s == nil {
		return
	}
	r.sess = nil
	if s.frames < minFrames {
		if err := r.store.DeleteSession(s.id); err != nil {
			slog.Warn("raidtrack: short session delete failed", "err", err)
		}
		return
	}
	r.sess = s
	r.flushLocked()
	r.sess = nil
	slog.Info("raidtrack: recording ended", "zone", s.zone, "frames", s.frames)
}

func round(v float64) int { return int(math.Round(v)) }

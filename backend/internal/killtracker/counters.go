package killtracker

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// WSEventCounters is broadcast with the full CountersState whenever any
// counter changes.
const WSEventCounters = "killcounters:state"

// Milestone is a count worth calling out ("Darwol Adan spawns at 44").
type Milestone struct {
	At    int    `json:"at"`
	Label string `json:"label"`
}

// SyncRule snaps the count to SetTo when a log line matches Pattern. Used for
// server announcements that reveal the true count (a boss-spawn emote fires at
// a known kill number), so a counter that missed kills — out of range, in
// another wing — corrects itself.
type SyncRule struct {
	Pattern string `json:"pattern"`
	SetTo   int    `json:"set_to"`
}

// ClockMark (re)starts the event clock when Pattern matches, backdated by
// AtSecs — a line known to appear N seconds into the event pins the clock even
// if the player missed the line that actually started it.
type ClockMark struct {
	Pattern string `json:"pattern"`
	AtSecs  int    `json:"at_secs"`
	// IfIdle only honors the mark while the clock isn't running — for a
	// start line the server repeats mid-event (Stone Ring prints its
	// "fortifications are summoned" line once per trigger mob).
	IfIdle bool `json:"if_idle,omitempty"`
	// Reset zeroes the count when the mark is honored, so a start line
	// that may repeat mid-event can still begin a fresh attempt.
	Reset bool `json:"reset,omitempty"`
}

// Clock is an optional event countdown shown alongside a counter.
type Clock struct {
	Label        string      `json:"label"`
	DurationSecs int         `json:"duration_secs"`
	Marks        []ClockMark `json:"marks"`
	StopPatterns []string    `json:"stop_patterns,omitempty"`
}

// CounterDef is the user-editable definition of a kill counter.
type CounterDef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Group   string `json:"group,omitempty"`  // display grouping on the overlay
	Preset  string `json:"preset,omitempty"` // built-in preset key, if installed from one
	Enabled bool   `json:"enabled"`

	// What counts: kills of any of Mobs (case-insensitive exact display
	// name), and/or every log line matching Pattern (e.g. an XP message).
	Mobs    []string `json:"mobs,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	// Zone, when set, only counts while the log's current zone (long name)
	// matches. An unknown current zone (app started mid-session, before any
	// zone-in) still counts, so a restart mid-raid doesn't freeze the count.
	Zone string `json:"zone,omitempty"`

	Goal       int         `json:"goal,omitempty"`
	Milestones []Milestone `json:"milestones,omitempty"`
	// WarnBefore fires a heads-up alert this many counts before each
	// milestone. 0 disables.
	WarnBefore int `json:"warn_before,omitempty"`

	Syncs        []SyncRule `json:"syncs,omitempty"`
	ResetPattern string     `json:"reset_pattern,omitempty"`
	Clock        *Clock     `json:"clock,omitempty"`

	AlertOverlay bool `json:"alert_overlay"`
	AlertTTS     bool `json:"alert_tts"`
}

// Counter is a definition plus its live state.
type Counter struct {
	Def            CounterDef `json:"def"`
	Count          int        `json:"count"`
	ClockStartedAt int64      `json:"clock_started_at"` // unix seconds; 0 = not running
	UpdatedAt      int64      `json:"updated_at"`
}

// Kill sources reported in CountersState.KillSource.
const (
	KillSourceLog  = "log"  // "has been slain" log lines only
	KillSourceZeal = "zeal" // Zeal pipe death events (every death in the zone)
)

// CountersState is the payload of GET /api/kill-counters and the WS event.
type CountersState struct {
	Counters []Counter `json:"counters"`
	Now      int64     `json:"now"` // server unix seconds, for clock math
	// KillSource is where kills are currently counted from: KillSourceZeal
	// once a Zeal death event has arrived on the current pipe connection,
	// KillSourceLog otherwise.
	KillSource string `json:"kill_source"`
}

// Broadcaster is the WS hub surface the engine needs.
type Broadcaster interface {
	Broadcast(ws.Event)
}

// compiled holds a counter's parsed regexes and lowercase mob set.
type compiled struct {
	mobs    map[string]bool
	pattern *regexp.Regexp
	syncs   []compiledSync
	reset   *regexp.Regexp
	marks   []compiledMark
	stops   []*regexp.Regexp
}

type compiledSync struct {
	re    *regexp.Regexp
	setTo int
}

type compiledMark struct {
	re     *regexp.Regexp
	atSecs int
	ifIdle bool
	reset  bool
}

// ErrNotFound is returned for an unknown counter id.
var ErrNotFound = errors.New("counter not found")

// Engine drives every kill counter from the log streams.
type Engine struct {
	store *Store
	hub   Broadcaster

	mu       sync.Mutex
	counters []*Counter
	comp     map[string]*compiled
	zone     string

	// pipeDeaths is set by the first Zeal death event (HandlePipeDeath) and
	// cleared on pipe disconnect (HandlePipeReset). While set, kills are
	// counted from the pipe — which sees every death in the zone, not only
	// the ones the client prints a "has been slain" line for — and log kill
	// lines are ignored so nothing counts twice. Released Zeal builds never
	// send death events, so counting stays log-driven there.
	pipeDeaths bool
	// recentLogKills are kills counted from the log shortly before the
	// switch to pipe deaths; the matching pipe event is skipped so the kill
	// that first lights up the pipe isn't counted twice.
	recentLogKills []recentKill
	now            func() time.Time // time.Now; overridden in tests
}

// recentKill is one log-counted kill, by lowercase mob name and the wall
// time it was counted.
type recentKill struct {
	name string
	at   time.Time
}

// logKillDedupWindow is how long a log-counted kill can still absorb the
// pipe death event for the same mob. The two arrive over different channels
// within a poll or two of each other.
const logKillDedupWindow = 5 * time.Second

// NewEngine loads stored counters and returns a ready engine. store may be nil
// (counters then live in memory only).
func NewEngine(store *Store, hub Broadcaster) *Engine {
	e := &Engine{store: store, hub: hub, comp: map[string]*compiled{}, now: time.Now}
	if store != nil {
		cs, err := store.LoadCounters()
		if err != nil {
			slog.Warn("killtracker: load counters", "err", err)
		}
		for i, c := range cs {
			if def, ok := upgradePresetDef(c.Def); ok {
				c.Def = def
				if err := store.SaveCounter(c, i); err != nil {
					slog.Warn("killtracker: save upgraded preset counter", "id", c.Def.ID, "err", err)
				}
			}
			comp, err := compile(c.Def)
			if err != nil {
				slog.Warn("killtracker: skip counter with invalid definition", "id", c.Def.ID, "err", err)
				continue
			}
			e.counters = append(e.counters, c)
			e.comp[c.Def.ID] = comp
		}
	}
	return e
}

// Validate normalizes def and checks it compiles. Returns the cleaned def.
func Validate(def CounterDef) (CounterDef, error) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return def, errors.New("name is required")
	}
	def.Group = strings.TrimSpace(def.Group)
	def.Zone = strings.TrimSpace(def.Zone)
	def.Pattern = strings.TrimSpace(def.Pattern)
	mobs := make([]string, 0, len(def.Mobs))
	seen := map[string]bool{}
	for _, m := range def.Mobs {
		m = strings.TrimSpace(m)
		if m == "" || seen[strings.ToLower(m)] {
			continue
		}
		seen[strings.ToLower(m)] = true
		mobs = append(mobs, m)
	}
	def.Mobs = mobs
	if len(def.Mobs) == 0 && def.Pattern == "" {
		return def, errors.New("add at least one mob name or a line pattern")
	}
	if def.Goal < 0 {
		def.Goal = 0
	}
	if def.WarnBefore < 0 {
		def.WarnBefore = 0
	}
	ms := def.Milestones[:0:0]
	for _, m := range def.Milestones {
		if m.At <= 0 {
			continue
		}
		m.Label = strings.TrimSpace(m.Label)
		ms = append(ms, m)
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].At < ms[j].At })
	def.Milestones = ms
	if def.Clock != nil && def.Clock.DurationSecs <= 0 {
		def.Clock = nil
	}
	if _, err := compile(def); err != nil {
		return def, err
	}
	return def, nil
}

func compile(def CounterDef) (*compiled, error) {
	c := &compiled{mobs: map[string]bool{}}
	for _, m := range def.Mobs {
		c.mobs[strings.ToLower(strings.TrimSpace(m))] = true
	}
	var err error
	if def.Pattern != "" {
		if c.pattern, err = regexp.Compile(def.Pattern); err != nil {
			return nil, fmt.Errorf("line pattern: %w", err)
		}
	}
	for _, s := range def.Syncs {
		if strings.TrimSpace(s.Pattern) == "" {
			continue
		}
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return nil, fmt.Errorf("sync pattern %q: %w", s.Pattern, err)
		}
		c.syncs = append(c.syncs, compiledSync{re: re, setTo: s.SetTo})
	}
	if strings.TrimSpace(def.ResetPattern) != "" {
		if c.reset, err = regexp.Compile(def.ResetPattern); err != nil {
			return nil, fmt.Errorf("reset pattern: %w", err)
		}
	}
	if def.Clock != nil {
		for _, m := range def.Clock.Marks {
			if strings.TrimSpace(m.Pattern) == "" {
				continue
			}
			re, err := regexp.Compile(m.Pattern)
			if err != nil {
				return nil, fmt.Errorf("clock pattern %q: %w", m.Pattern, err)
			}
			c.marks = append(c.marks, compiledMark{re: re, atSecs: m.AtSecs, ifIdle: m.IfIdle, reset: m.Reset})
		}
		for _, p := range def.Clock.StopPatterns {
			if strings.TrimSpace(p) == "" {
				continue
			}
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("clock stop pattern %q: %w", p, err)
			}
			c.stops = append(c.stops, re)
		}
	}
	return c, nil
}

// Handle consumes parsed events: zone changes and kills.
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventZone:
		if zd, ok := ev.Data.(logparser.ZoneData); ok {
			e.mu.Lock()
			e.zone = zd.ZoneName
			e.mu.Unlock()
		}
	case logparser.EventKill:
		kd, ok := ev.Data.(logparser.KillData)
		if !ok {
			return
		}
		target := strings.ToLower(strings.TrimSpace(kd.Target))
		if target == "" {
			return
		}
		e.mu.Lock()
		if e.pipeDeaths {
			e.mu.Unlock()
			return
		}
		now := e.now()
		kept := e.recentLogKills[:0]
		for _, k := range e.recentLogKills {
			if now.Sub(k.at) <= logKillDedupWindow {
				kept = append(kept, k)
			}
		}
		e.recentLogKills = append(kept, recentKill{name: target, at: now})
		e.mu.Unlock()
		e.countKill(ev.Timestamp, target)
	}
}

// HandlePipeDeath consumes a Zeal death event (zealpipe.MsgDeath): one death
// anywhere in the zone, by display name. The first one switches counting
// from log kill lines to pipe deaths for the rest of the pipe connection.
func (e *Engine) HandlePipeDeath(at time.Time, name string) {
	target := strings.ToLower(strings.TrimSpace(name))
	e.mu.Lock()
	switched := !e.pipeDeaths
	e.pipeDeaths = true
	skip := false
	now := e.now()
	for i, k := range e.recentLogKills {
		if k.name == target && now.Sub(k.at) <= logKillDedupWindow {
			e.recentLogKills = append(e.recentLogKills[:i], e.recentLogKills[i+1:]...)
			skip = true
			break
		}
	}
	e.mu.Unlock()
	if switched {
		slog.Info("killtracker: counting kills from Zeal death events")
		e.broadcast()
	}
	if target == "" || skip {
		return
	}
	e.countKill(at, target)
}

// HandlePipeReset drops back to counting log kill lines when the Zeal pipe
// disconnects.
func (e *Engine) HandlePipeReset() {
	e.mu.Lock()
	switched := e.pipeDeaths
	e.pipeDeaths = false
	e.recentLogKills = nil
	e.mu.Unlock()
	if switched {
		slog.Info("killtracker: counting kills from log lines")
		e.broadcast()
	}
}

// countKill adds one kill of target (lowercase display name) to every
// counter tracking that mob.
func (e *Engine) countKill(ts time.Time, target string) {
	e.apply(ts, func(c *Counter, comp *compiled) (int, bool) {
		if !comp.mobs[target] {
			return 0, false
		}
		return c.Count + 1, true
	})
}

// HandleLine consumes raw log lines: resets, clock marks/stops, syncs and
// line-pattern counting, in that order.
func (e *Engine) HandleLine(ts time.Time, msg string) {
	e.mu.Lock()
	var alerts []alert
	changed := false
	for _, c := range e.counters {
		if !c.Def.Enabled {
			continue
		}
		comp := e.comp[c.Def.ID]
		if comp == nil {
			continue
		}
		touched := false
		if comp.reset != nil && comp.reset.MatchString(msg) && (c.Count != 0 || c.ClockStartedAt != 0) {
			c.Count = 0
			c.ClockStartedAt = 0
			touched = true
		}
		for _, m := range comp.marks {
			if !m.re.MatchString(msg) {
				continue
			}
			// A clock past its duration counts as idle: some events fail
			// silently (Stone Ring just depops), leaving no stop line.
			if m.ifIdle && c.ClockStartedAt != 0 && c.Def.Clock != nil &&
				ts.Unix() < c.ClockStartedAt+int64(c.Def.Clock.DurationSecs) {
				break
			}
			c.ClockStartedAt = ts.Add(-time.Duration(m.atSecs) * time.Second).Unix()
			if m.reset {
				c.Count = 0
			}
			touched = true
			break
		}
		for _, re := range comp.stops {
			if c.ClockStartedAt != 0 && re.MatchString(msg) {
				c.ClockStartedAt = 0
				touched = true
				break
			}
		}
		next := c.Count
		for _, s := range comp.syncs {
			if s.re.MatchString(msg) {
				next = s.setTo
				break
			}
		}
		if comp.pattern != nil && e.zoneOKLocked(c) && comp.pattern.MatchString(msg) {
			next++
		}
		if next != c.Count {
			alerts = append(alerts, milestoneAlerts(c, c.Count, next)...)
			c.Count = next
			touched = true
		}
		if touched {
			c.UpdatedAt = ts.Unix()
			e.persistLocked(c)
			changed = true
		}
	}
	e.mu.Unlock()
	if changed {
		e.broadcast()
	}
	e.fire(alerts)
}

// apply runs step on each enabled, zone-eligible counter; step returns the new
// count and whether it applies.
func (e *Engine) apply(ts time.Time, step func(*Counter, *compiled) (int, bool)) {
	e.mu.Lock()
	var alerts []alert
	changed := false
	for _, c := range e.counters {
		comp := e.comp[c.Def.ID]
		if !c.Def.Enabled || comp == nil || !e.zoneOKLocked(c) {
			continue
		}
		next, ok := step(c, comp)
		if !ok || next == c.Count {
			continue
		}
		alerts = append(alerts, milestoneAlerts(c, c.Count, next)...)
		c.Count = next
		c.UpdatedAt = ts.Unix()
		e.persistLocked(c)
		changed = true
	}
	e.mu.Unlock()
	if changed {
		e.broadcast()
	}
	e.fire(alerts)
}

func (e *Engine) zoneOKLocked(c *Counter) bool {
	return c.Def.Zone == "" || e.zone == "" || strings.EqualFold(c.Def.Zone, e.zone)
}

type alert struct {
	counter *Counter
	text    string
}

// milestoneAlerts returns the alerts for a count moving from prev to next.
// Only upward moves alert. When a jump (a resync) crosses several milestones
// only the highest one is announced, and a heads-up is only given for a
// milestone that wasn't also reached by the same move.
func milestoneAlerts(c *Counter, prev, next int) []alert {
	if next <= prev || (!c.Def.AlertOverlay && !c.Def.AlertTTS) {
		return nil
	}
	var out []alert
	var reached *Milestone
	for i := range c.Def.Milestones {
		m := &c.Def.Milestones[i]
		if prev < m.At && m.At <= next {
			reached = m
		}
	}
	if reached != nil {
		out = append(out, alert{counter: c, text: milestoneText(reached, c.Def.Name)})
	}
	if w := c.Def.WarnBefore; w > 0 {
		for i := range c.Def.Milestones {
			m := &c.Def.Milestones[i]
			mark := m.At - w
			if m.At > next && prev < mark && mark <= next {
				n := m.At - next
				unit := "kills"
				if n == 1 {
					unit = "kill"
				}
				out = append(out, alert{counter: c, text: fmt.Sprintf("%s in %d %s", milestoneLabel(m, c.Def.Name), n, unit)})
				break
			}
		}
	}
	return out
}

func milestoneLabel(m *Milestone, counterName string) string {
	if m.Label != "" {
		return m.Label
	}
	return fmt.Sprintf("%s %d", counterName, m.At)
}

func milestoneText(m *Milestone, counterName string) string {
	if m.Label == "" {
		return fmt.Sprintf("%s: %d", counterName, m.At)
	}
	return fmt.Sprintf("%s (%d)", m.Label, m.At)
}

// fire broadcasts each alert as a synthetic trigger:fired event so it renders
// through the normal Trigger Alerts overlay / TTS pipeline.
func (e *Engine) fire(alerts []alert) {
	if e.hub == nil {
		return
	}
	for _, a := range alerts {
		var actions []trigger.Action
		if a.counter.Def.AlertOverlay {
			actions = append(actions, trigger.Action{
				Type:         trigger.ActionOverlayText,
				Text:         a.text,
				DurationSecs: 8,
				Color:        "#f59e0b",
			})
		}
		if a.counter.Def.AlertTTS {
			actions = append(actions, trigger.Action{
				Type:   trigger.ActionTextToSpeech,
				Text:   a.text,
				Volume: 1,
			})
		}
		if len(actions) == 0 {
			continue
		}
		e.hub.Broadcast(ws.Event{
			Type: trigger.WSEventTriggerFired,
			Data: trigger.TriggerFired{
				TriggerID:   "system:killcounter:" + a.counter.Def.ID,
				TriggerName: "Kill Counter: " + a.counter.Def.Name,
				MatchedLine: a.text,
				Actions:     actions,
				FiredAt:     time.Now(),
			},
		})
	}
}

func (e *Engine) persistLocked(c *Counter) {
	if e.store == nil {
		return
	}
	for i, cc := range e.counters {
		if cc == c {
			if err := e.store.SaveCounter(c, i); err != nil {
				slog.Warn("killtracker: save counter", "id", c.Def.ID, "err", err)
			}
			return
		}
	}
}

func (e *Engine) broadcast() {
	if e.hub != nil {
		e.hub.Broadcast(ws.Event{Type: WSEventCounters, Data: e.State()})
	}
}

// State returns a snapshot of every counter.
func (e *Engine) State() CountersState {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := CountersState{Counters: make([]Counter, 0, len(e.counters)), Now: time.Now().Unix(), KillSource: KillSourceLog}
	if e.pipeDeaths {
		out.KillSource = KillSourceZeal
	}
	for _, c := range e.counters {
		cp := *c
		out.Counters = append(out.Counters, cp)
	}
	return out
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Create adds a counter. def must already be validated.
func (e *Engine) Create(def CounterDef) (Counter, error) {
	comp, err := compile(def)
	if err != nil {
		return Counter{}, err
	}
	def.ID = newID()
	c := &Counter{Def: def, UpdatedAt: time.Now().Unix()}
	e.mu.Lock()
	e.counters = append(e.counters, c)
	e.comp[def.ID] = comp
	e.persistLocked(c)
	snap := *c
	e.mu.Unlock()
	e.broadcast()
	return snap, nil
}

// Update replaces a counter's definition, keeping its live count and clock.
func (e *Engine) Update(def CounterDef) (Counter, error) {
	comp, err := compile(def)
	if err != nil {
		return Counter{}, err
	}
	e.mu.Lock()
	c := e.findLocked(def.ID)
	if c == nil {
		e.mu.Unlock()
		return Counter{}, ErrNotFound
	}
	if def.Preset == "" {
		def.Preset = c.Def.Preset
	}
	c.Def = def
	if def.Clock == nil {
		c.ClockStartedAt = 0
	}
	e.comp[def.ID] = comp
	c.UpdatedAt = time.Now().Unix()
	e.persistLocked(c)
	snap := *c
	e.mu.Unlock()
	e.broadcast()
	return snap, nil
}

// Delete removes a counter.
func (e *Engine) Delete(id string) error {
	e.mu.Lock()
	idx := -1
	for i, c := range e.counters {
		if c.Def.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	e.counters = append(e.counters[:idx], e.counters[idx+1:]...)
	delete(e.comp, id)
	if e.store != nil {
		if err := e.store.DeleteCounter(id); err != nil {
			slog.Warn("killtracker: delete counter", "id", id, "err", err)
		}
		// Re-save the rest so sort_order stays contiguous.
		for i, c := range e.counters {
			_ = e.store.SaveCounter(c, i)
		}
	}
	e.mu.Unlock()
	e.broadcast()
	return nil
}

// Move shifts a counter one place up (dir < 0) or down (dir > 0).
func (e *Engine) Move(id string, dir int) error {
	e.mu.Lock()
	idx := -1
	for i, c := range e.counters {
		if c.Def.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	j := idx + 1
	if dir < 0 {
		j = idx - 1
	}
	if j >= 0 && j < len(e.counters) {
		e.counters[idx], e.counters[j] = e.counters[j], e.counters[idx]
		e.persistLocked(e.counters[idx])
		e.persistLocked(e.counters[j])
	}
	e.mu.Unlock()
	e.broadcast()
	return nil
}

// SetCount sets a counter's value by hand. Manual changes never alert.
func (e *Engine) SetCount(id string, n int) error {
	if n < 0 {
		n = 0
	}
	return e.mutate(id, func(c *Counter) { c.Count = n })
}

// Adjust nudges a counter's value by delta (never below zero).
func (e *Engine) Adjust(id string, delta int) error {
	return e.mutate(id, func(c *Counter) {
		c.Count += delta
		if c.Count < 0 {
			c.Count = 0
		}
	})
}

// Reset zeroes the count and stops the clock.
func (e *Engine) Reset(id string) error {
	return e.mutate(id, func(c *Counter) { c.Count = 0; c.ClockStartedAt = 0 })
}

// StartClock starts (or restarts) a counter's event clock now.
func (e *Engine) StartClock(id string) error {
	return e.mutate(id, func(c *Counter) {
		if c.Def.Clock != nil {
			c.ClockStartedAt = time.Now().Unix()
		}
	})
}

// StopClock stops a counter's event clock.
func (e *Engine) StopClock(id string) error {
	return e.mutate(id, func(c *Counter) { c.ClockStartedAt = 0 })
}

// SetEnabled toggles whether a counter listens to the log.
func (e *Engine) SetEnabled(id string, enabled bool) error {
	return e.mutate(id, func(c *Counter) { c.Def.Enabled = enabled })
}

func (e *Engine) mutate(id string, fn func(*Counter)) error {
	e.mu.Lock()
	c := e.findLocked(id)
	if c == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	fn(c)
	c.UpdatedAt = time.Now().Unix()
	e.persistLocked(c)
	e.mu.Unlock()
	e.broadcast()
	return nil
}

func (e *Engine) findLocked(id string) *Counter {
	for _, c := range e.counters {
		if c.Def.ID == id {
			return c
		}
	}
	return nil
}

// HasPreset reports whether counters from preset key are already installed.
func (e *Engine) HasPreset(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.counters {
		if c.Def.Preset == key {
			return true
		}
	}
	return false
}

// InstallPreset adds every counter in the named preset.
func (e *Engine) InstallPreset(key string) ([]Counter, error) {
	p, ok := presetByKey(key)
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]Counter, 0, len(p.Counters))
	for _, def := range p.Counters {
		def.Preset = key
		def, err := Validate(def)
		if err != nil {
			return out, fmt.Errorf("preset %s: %w", key, err)
		}
		c, err := e.Create(def)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, nil
}

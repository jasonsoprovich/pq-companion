package chatwindow

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Broadcaster is the WS hub surface the engine needs.
type Broadcaster interface {
	Broadcast(ws.Event)
}

// History is the chat history surface used to refill windows (chat.Store).
type History interface {
	Feed(f chat.FeedFilters) ([]chat.Message, error)
	Channels(character string) ([]string, error)
}

const (
	flushInterval  = 250 * time.Millisecond
	expiryInterval = 5 * time.Second
)

// Engine owns every chat window: it filters log lines into them, keeps their
// contents, and delivers them over the WebSocket. Safe for concurrent use.
type Engine struct {
	store      *Store
	hub        Broadcaster
	history    History
	activeChar func() string
	now        func() time.Time

	mu       sync.Mutex
	windows  []*Window
	comp     map[string]*compiledWindow
	compSelf string
	// pending holds each window's appends since the last flush.
	pending map[string][]Line
	// lastEpoch makes every epoch unique and increasing, across restarts too
	// (it starts from the clock).
	lastEpoch int64
	// filledFor records which character each window was last refilled from
	// chat history for, so a refill runs once per window and character.
	filledFor map[string]string
	// replaySaved holds each window's contents while a Log Feed replay runs
	// (nil otherwise); SetReplaying(false) puts them back.
	replaySaved map[string]WindowState
	// names lists known player names for Suggest (see SetNameSource).
	names func(name string) bool
}

// NewEngine loads stored windows. store, hub and history may be nil.
// activeChar returns the character whose log is being read.
func NewEngine(store *Store, hub Broadcaster, history History, activeChar func() string) *Engine {
	e := &Engine{
		store:      store,
		hub:        hub,
		history:    history,
		activeChar: activeChar,
		now:        time.Now,
		comp:       map[string]*compiledWindow{},
		pending:    map[string][]Line{},
		filledFor:  map[string]string{},
	}
	e.lastEpoch = e.now().UnixMilli()
	if store != nil {
		defs, err := store.Load()
		if err != nil {
			slog.Warn("chatwindow: load", "err", err)
		}
		for _, def := range defs {
			v, err := Validate(def)
			if err != nil {
				slog.Warn("chatwindow: invalid stored definition", "id", def.ID, "err", err)
				v = def
			}
			e.windows = append(e.windows, &Window{Def: v, State: WindowState{Epoch: e.nextEpoch()}})
		}
	}
	e.compSelf = e.self()
	for _, w := range e.windows {
		e.comp[w.Def.ID] = compileWindow(w.Def, e.compSelf)
	}
	return e
}

// Start runs the batch flusher and pin expiry until ctx ends.
func (e *Engine) Start(ctx context.Context) {
	flush := time.NewTicker(flushInterval)
	expiry := time.NewTicker(expiryInterval)
	defer flush.Stop()
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-flush.C:
			e.flush()
		case <-expiry.C:
			e.expirePins()
		}
	}
}

func (e *Engine) self() string {
	if e.activeChar == nil {
		return ""
	}
	return e.activeChar()
}

func (e *Engine) nextEpoch() int64 {
	e.lastEpoch++
	return e.lastEpoch
}

// recompileLocked rebuilds every window's rules when the active character
// (the {c} token) changed.
func (e *Engine) recompileLocked(self string) {
	if self == e.compSelf {
		return
	}
	e.compSelf = self
	for _, w := range e.windows {
		e.comp[w.Def.ID] = compileWindow(w.Def, self)
	}
}

// HandleLine filters one log message (timestamp stripped) into every
// enabled window. The line is classified once, whatever the window count.
func (e *Engine) HandleLine(ts time.Time, msg string) {
	self := e.self()
	line := lineFromLog(ts, msg, self)
	nowMs := e.now().UnixMilli()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.recompileLocked(self)
	for _, w := range e.windows {
		cw := e.comp[w.Def.ID]
		if !w.Def.Enabled || cw == nil {
			continue
		}
		v := cw.evaluate(&line)
		if !v.shown() {
			continue
		}
		l := line
		l.Color = v.color
		e.appendLocked(w, l, v.pin, nowMs)
	}
}

// appendLocked adds a shown line to a window, pins it when asked, and queues
// it for the next flush.
func (e *Engine) appendLocked(w *Window, l Line, pinMinutes int, nowMs int64) {
	st := &w.State
	st.Seq++
	l.Seq = st.Seq
	st.Lines = append(st.Lines, l)
	// Trim in bulk once the buffer reaches twice its size, so a full window
	// doesn't copy every line on every append. Snapshots show the last
	// MaxLines (see cloneWindow).
	if len(st.Lines) >= 2*w.Def.MaxLines {
		st.Lines = append([]Line(nil), st.Lines[len(st.Lines)-w.Def.MaxLines:]...)
	}
	if pinMinutes > 0 {
		st.Pins = append(st.Pins, Pin{Seq: l.Seq, Until: nowMs + int64(pinMinutes)*60_000, Line: l})
		if n := len(st.Pins) - MaxPins; n > 0 {
			st.Pins = append([]Pin(nil), st.Pins[n:]...)
		}
	}
	e.pending[w.Def.ID] = append(e.pending[w.Def.ID], l)
}

// flush sends every window's queued appends as one event.
func (e *Engine) flush() {
	e.mu.Lock()
	if len(e.pending) == 0 {
		e.mu.Unlock()
		return
	}
	ev := LinesEvent{Now: e.now().UnixMilli()}
	for _, w := range e.windows {
		lines := e.pending[w.Def.ID]
		if len(lines) == 0 {
			continue
		}
		ev.Windows = append(ev.Windows, LinesBatch{
			ID:     w.Def.ID,
			Epoch:  w.State.Epoch,
			Lines:  lines,
			Pins:   append([]Pin{}, w.State.Pins...),
			Replay: w.State.Replay,
		})
	}
	e.pending = map[string][]Line{}
	e.mu.Unlock()
	if len(ev.Windows) > 0 && e.hub != nil {
		e.hub.Broadcast(ws.Event{Type: WSEventLines, Data: ev})
	}
}

// replaceLocked marks a change that isn't an append: a new epoch, and the
// window's queued appends are dropped since the full window goes out instead.
// The caller broadcasts the window (or the whole state) after unlocking.
func (e *Engine) replaceLocked(w *Window) {
	w.State.Epoch = e.nextEpoch()
	delete(e.pending, w.Def.ID)
}

func (e *Engine) broadcastWindow(w Window) {
	if e.hub != nil {
		e.hub.Broadcast(ws.Event{Type: WSEventWindow, Data: w})
	}
}

func (e *Engine) broadcastState() {
	if e.hub != nil {
		e.hub.Broadcast(ws.Event{Type: WSEventState, Data: e.State()})
	}
}

// expirePins drops pins whose time is up.
func (e *Engine) expirePins() {
	nowMs := e.now().UnixMilli()
	var changed []Window
	e.mu.Lock()
	for _, w := range e.windows {
		kept := w.State.Pins[:0:0]
		for _, p := range w.State.Pins {
			if p.Until > nowMs {
				kept = append(kept, p)
			}
		}
		if len(kept) != len(w.State.Pins) {
			w.State.Pins = kept
			e.replaceLocked(w)
			changed = append(changed, cloneWindow(w))
		}
	}
	e.mu.Unlock()
	for _, w := range changed {
		e.broadcastWindow(w)
	}
}

// Dismiss removes one pin.
func (e *Engine) Dismiss(id string, seq int64) error {
	e.mu.Lock()
	w := e.findLocked(id)
	if w == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	kept := w.State.Pins[:0:0]
	for _, p := range w.State.Pins {
		if p.Seq != seq {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(w.State.Pins) {
		e.mu.Unlock()
		return ErrNotFound
	}
	w.State.Pins = kept
	e.replaceLocked(w)
	snap := cloneWindow(w)
	e.mu.Unlock()
	e.broadcastWindow(snap)
	return nil
}

// Clear empties one window's lines and pins.
func (e *Engine) Clear(id string) error {
	e.mu.Lock()
	w := e.findLocked(id)
	if w == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	w.State.Lines = nil
	w.State.Pins = nil
	e.replaceLocked(w)
	snap := cloneWindow(w)
	e.mu.Unlock()
	e.broadcastWindow(snap)
	return nil
}

// refilterLocked re-runs the window's rules over what it already shows (and
// its pins): lines the rules no longer show go, kept lines are recolored.
// Lines hidden earlier never come back; they were never stored.
func refilterLocked(w *Window, cw *compiledWindow) {
	st := &w.State
	lines := st.Lines[:0:0]
	for _, l := range st.Lines {
		if v := cw.evaluate(&l); v.shown() {
			l.Color = v.color
			lines = append(lines, l)
		}
	}
	st.Lines = lines
	if n := len(st.Lines) - w.Def.MaxLines; n > 0 {
		st.Lines = append([]Line(nil), st.Lines[n:]...)
	}
	pins := st.Pins[:0:0]
	for _, p := range st.Pins {
		if v := cw.evaluate(&p.Line); v.result == VerdictKept {
			p.Line.Color = v.color
			pins = append(pins, p)
		}
	}
	st.Pins = pins
}

// ── refill from chat history ────────────────────────────────────────────────

// CharacterChanged refills windows from the new character's chat history
// (each window once per character). Call it on every active-character change.
func (e *Engine) CharacterChanged(character string) {
	e.refill(character, "")
}

// refill merges chat history into the windows (only id when set) that
// haven't been refilled for character yet. History lines go through the same
// line builder and evaluation as live ones, and dedupe against what the
// window already shows.
func (e *Engine) refill(character, only string) {
	if e.history == nil || character == "" {
		return
	}
	type job struct {
		id       string
		def      WindowDef
		cw       *compiledWindow
		channels []string
	}
	var jobs []job
	e.mu.Lock()
	if e.replaySaved != nil {
		e.mu.Unlock()
		return
	}
	e.recompileLocked(e.self())
	for _, w := range e.windows {
		if (only != "" && w.Def.ID != only) || !w.Def.Enabled || e.filledFor[w.Def.ID] == character {
			continue
		}
		cw := e.comp[w.Def.ID]
		if cw == nil {
			continue
		}
		e.filledFor[w.Def.ID] = character
		var channels []string
		for _, c := range w.Def.Sources.Channels {
			if c != ChannelSay { // chat history doesn't keep /say
				channels = append(channels, c)
			}
		}
		jobs = append(jobs, job{id: w.Def.ID, def: w.Def, cw: cw, channels: channels})
	}
	e.mu.Unlock()
	if len(jobs) == 0 {
		return
	}

	var named []string
	for _, j := range jobs {
		if j.def.Sources.AllNamed {
			all, err := e.history.Channels(character)
			if err != nil {
				slog.Warn("chatwindow: list chat channels", "err", err)
			}
			for _, c := range all {
				if !standardChannels[c] {
					named = append(named, c)
				}
			}
			break
		}
	}

	results := make(map[string][]Line, len(jobs))
	for _, j := range jobs {
		channels := j.channels
		if j.def.Sources.AllNamed {
			seen := map[string]bool{}
			for _, c := range channels {
				seen[c] = true
			}
			for _, c := range named {
				if !seen[c] {
					channels = append(channels, c)
				}
			}
		}
		var lines []Line
		for _, c := range channels {
			msgs, err := e.history.Feed(chat.FeedFilters{
				Character: character, Channel: c, SortDesc: true, Limit: j.def.MaxLines,
			})
			if err != nil {
				slog.Warn("chatwindow: read chat history", "channel", c, "err", err)
				continue
			}
			// Newest first (to take the last MaxLines); walk back to oldest
			// first so same-second lines keep their order.
			for i := len(msgs) - 1; i >= 0; i-- {
				l := lineFromHistory(msgs[i])
				if v := j.cw.evaluate(&l); v.shown() {
					l.Color = v.color
					lines = append(lines, l)
				}
			}
		}
		results[j.id] = lines
	}

	var changed []Window
	e.mu.Lock()
	for id, hist := range results {
		w := e.findLocked(id)
		if w == nil || len(hist) == 0 || e.replaySaved != nil {
			continue
		}
		mergeHistoryLocked(w, hist)
		e.replaceLocked(w)
		changed = append(changed, cloneWindow(w))
	}
	e.mu.Unlock()
	for _, w := range changed {
		e.broadcastWindow(w)
	}
}

// mergeHistoryLocked puts history lines in front of what the window shows,
// dropping duplicates of lines it already has, and renumbers everything in
// time order (seq keeps increasing; the epoch changes with it).
func mergeHistoryLocked(w *Window, hist []Line) {
	st := &w.State
	have := make(map[lineKey]bool, len(st.Lines))
	for _, l := range st.Lines {
		have[keyOf(l)] = true
	}
	merged := make([]Line, 0, len(hist)+len(st.Lines))
	for _, l := range hist {
		if k := keyOf(l); !have[k] {
			have[k] = true
			merged = append(merged, l)
		}
	}
	merged = append(merged, st.Lines...)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].TS < merged[j].TS })
	if n := len(merged) - w.Def.MaxLines; n > 0 {
		merged = merged[n:]
	}
	remap := make(map[int64]int64, len(merged))
	for i := range merged {
		st.Seq++
		if merged[i].Seq != 0 {
			remap[merged[i].Seq] = st.Seq
		}
		merged[i].Seq = st.Seq
	}
	for i := range st.Pins {
		if s, ok := remap[st.Pins[i].Seq]; ok {
			st.Pins[i].Seq = s
			st.Pins[i].Line.Seq = s
		}
	}
	st.Lines = merged
}

// ── replay ──────────────────────────────────────────────────────────────────

// SetReplaying starts or ends a Log Feed replay. While it runs the windows
// start empty and show the replayed lines (so a window can be tried against
// an old log); when it ends they get their earlier contents back. Seqs keep
// increasing across both switches, and each gets a new epoch.
func (e *Engine) SetReplaying(active bool) {
	e.mu.Lock()
	if active == (e.replaySaved != nil) {
		e.mu.Unlock()
		return
	}
	if active {
		e.replaySaved = make(map[string]WindowState, len(e.windows))
		for _, w := range e.windows {
			e.replaySaved[w.Def.ID] = cloneState(w.State)
			w.State.Lines = nil
			w.State.Pins = nil
			w.State.Replay = true
			e.replaceLocked(w)
		}
	} else {
		for _, w := range e.windows {
			saved, ok := e.replaySaved[w.Def.ID]
			seq := w.State.Seq
			if ok {
				w.State = saved
			} else {
				w.State = WindowState{}
			}
			w.State.Seq = seq
			w.State.Replay = false
			e.replaceLocked(w)
		}
		e.replaySaved = nil
	}
	e.mu.Unlock()
	e.broadcastState()
}

// ── definitions ─────────────────────────────────────────────────────────────

func (e *Engine) findLocked(id string) *Window {
	for _, w := range e.windows {
		if w.Def.ID == id {
			return w
		}
	}
	return nil
}

func (e *Engine) indexLocked(id string) int {
	for i, w := range e.windows {
		if w.Def.ID == id {
			return i
		}
	}
	return -1
}

func (e *Engine) saveLocked(w *Window) {
	if e.store == nil {
		return
	}
	if err := e.store.Save(w.Def, e.indexLocked(w.Def.ID)); err != nil {
		slog.Warn("chatwindow: save", "id", w.Def.ID, "err", err)
	}
}

// Create validates and adds a new window, then fills it from chat history.
func (e *Engine) Create(def WindowDef) (WindowDef, error) {
	def.ID = ""
	v, err := Validate(def)
	if err != nil {
		return def, err
	}
	e.mu.Lock()
	if len(e.windows) >= MaxWindows {
		e.mu.Unlock()
		return def, fmt.Errorf("at most %d chat windows", MaxWindows)
	}
	v.ID = newID()
	w := &Window{Def: v, State: WindowState{Epoch: e.nextEpoch(), Replay: e.replaySaved != nil}}
	e.windows = append(e.windows, w)
	e.comp[v.ID] = compileWindow(v, e.compSelf)
	e.saveLocked(w)
	e.mu.Unlock()
	e.broadcastState()
	e.refill(e.self(), v.ID)
	return v, nil
}

// Update replaces a window's definition, re-filters what it shows, and fills
// it from chat history for any channels it gained.
func (e *Engine) Update(def WindowDef) (WindowDef, error) {
	v, err := Validate(def)
	if err != nil {
		return def, err
	}
	e.mu.Lock()
	w := e.findLocked(v.ID)
	if w == nil {
		e.mu.Unlock()
		return def, ErrNotFound
	}
	w.Def = v
	cw := compileWindow(v, e.compSelf)
	e.comp[v.ID] = cw
	refilterLocked(w, cw)
	e.replaceLocked(w)
	delete(e.filledFor, v.ID)
	e.saveLocked(w)
	e.mu.Unlock()
	e.broadcastState()
	e.refill(e.self(), v.ID)
	return v, nil
}

// SetAppearance changes only how a window looks (the Ctrl+scroll font size,
// the opacity slider), without re-filtering its lines.
func (e *Engine) SetAppearance(id string, a Appearance) error {
	e.mu.Lock()
	w := e.findLocked(id)
	if w == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	def := w.Def
	def.Appearance = a
	v, err := Validate(def)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	w.Def.Appearance = v.Appearance
	e.saveLocked(w)
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

// SetEnabled turns a window's filtering on or off.
func (e *Engine) SetEnabled(id string, enabled bool) error {
	e.mu.Lock()
	w := e.findLocked(id)
	if w == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	w.Def.Enabled = enabled
	e.saveLocked(w)
	e.mu.Unlock()
	e.broadcastState()
	if enabled {
		e.refill(e.self(), id)
	}
	return nil
}

// Delete removes a window.
func (e *Engine) Delete(id string) error {
	e.mu.Lock()
	i := e.indexLocked(id)
	if i < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	e.windows = append(e.windows[:i], e.windows[i+1:]...)
	delete(e.comp, id)
	delete(e.pending, id)
	delete(e.filledFor, id)
	if e.store != nil {
		if err := e.store.Delete(id); err != nil {
			slog.Warn("chatwindow: delete", "id", id, "err", err)
		}
	}
	for _, w := range e.windows {
		e.saveLocked(w)
	}
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

// Move shifts a window up (dir < 0) or down in the list.
func (e *Engine) Move(id string, dir int) error {
	e.mu.Lock()
	i := e.indexLocked(id)
	if i < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	j := i + 1
	if dir < 0 {
		j = i - 1
	}
	if j < 0 || j >= len(e.windows) {
		e.mu.Unlock()
		return nil
	}
	e.windows[i], e.windows[j] = e.windows[j], e.windows[i]
	e.saveLocked(e.windows[i])
	e.saveLocked(e.windows[j])
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

// ── snapshots ───────────────────────────────────────────────────────────────

func cloneState(st WindowState) WindowState {
	st.Lines = append([]Line{}, st.Lines...)
	st.Pins = append([]Pin{}, st.Pins...)
	return st
}

// cloneWindow snapshots a window, showing at most its last MaxLines lines
// (the buffer can run up to twice that between bulk trims).
func cloneWindow(w *Window) Window {
	st := w.State
	if n := len(st.Lines) - w.Def.MaxLines; n > 0 && w.Def.MaxLines > 0 {
		st.Lines = st.Lines[n:]
	}
	return Window{Def: w.Def, State: cloneState(st)}
}

// State returns every window with its contents.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := State{Windows: make([]Window, 0, len(e.windows)), Now: e.now().UnixMilli()}
	for _, w := range e.windows {
		out.Windows = append(out.Windows, cloneWindow(w))
	}
	return out
}

// ── test box ────────────────────────────────────────────────────────────────

// logPrefixRe strips a pasted "[Fri Nov 13 21:00:00 2026] " log timestamp.
var logPrefixRe = regexp.MustCompile(`^\[[^\]]*\d{4}\]\s*`)

// TestLines dry-runs log lines against an unsaved definition. Pasted lines
// may keep their [timestamp] prefix; blank lines are skipped. character
// stands in for the active character ({c}, the outgoing speaker); the
// Characters filter is ignored.
func (e *Engine) TestLines(def WindowDef, lines []string, character string) TestResult {
	v, err := Validate(def)
	if err != nil {
		return TestResult{Steps: []TestStep{}, Error: err.Error()}
	}
	if character == "" {
		character = e.self()
	}
	v.Characters = nil
	cw := compileWindow(v, character)
	res := TestResult{Steps: []TestStep{}, Def: &v}
	now := e.now()
	for _, raw := range lines {
		msg := strings.TrimSpace(logPrefixRe.ReplaceAllString(strings.TrimSpace(raw), ""))
		if msg == "" {
			continue
		}
		l := lineFromLog(now, msg, character)
		vd := cw.evaluate(&l)
		step := TestStep{Line: msg, Text: l.Text, Verdict: vd.result, Color: vd.color, Pin: vd.pin > 0}
		if vd.rule != nil {
			step.Rule = vd.rule.Name
			if step.Rule == "" {
				step.Rule = ruleLabel(*vd.rule)
			}
		}
		res.Steps = append(res.Steps, step)
	}
	return res
}

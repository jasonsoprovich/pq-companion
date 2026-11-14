package customoverlay

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Broadcaster is the WS hub surface the engine needs.
type Broadcaster interface {
	Broadcast(ws.Event)
}

type compiledRule struct {
	rule     Rule
	re       *regexp.Regexp // nil = match every message on Channel
	speakers map[string]bool
}

// Engine owns every custom overlay: it evaluates rules against log lines,
// keeps the live state, persists it and broadcasts snapshots.
type Engine struct {
	store      *Store
	hub        Broadcaster
	activeChar func() string
	now        func() time.Time

	mu       sync.Mutex
	overlays []*Overlay
	comp     map[string][]compiledRule
	// replaySaved holds each overlay's pre-replay contents while a Log Feed
	// replay runs (nil otherwise). Replayed lines still update the overlays
	// on screen so a new overlay can be tested against an old log, but
	// nothing is written, and SetReplaying(false) puts these back.
	replaySaved map[string]OverlayState
}

// NewEngine loads stored overlays. store and hub may be nil (in-memory only,
// no broadcasts). activeChar returns the character whose log is being read.
func NewEngine(store *Store, hub Broadcaster, activeChar func() string) *Engine {
	e := &Engine{
		store:      store,
		hub:        hub,
		activeChar: activeChar,
		now:        time.Now,
		comp:       map[string][]compiledRule{},
	}
	if store != nil {
		loaded, err := store.Load()
		if err != nil {
			slog.Warn("customoverlay: load", "err", err)
		}
		for _, o := range loaded {
			def, err := Validate(o.Def)
			if err != nil {
				// Keep it so the user can fix it, but with no rules running.
				slog.Warn("customoverlay: invalid stored definition", "id", o.Def.ID, "err", err)
				e.overlays = append(e.overlays, o)
				continue
			}
			o.Def = def
			pruneState(&o.State, def)
			e.overlays = append(e.overlays, o)
			e.comp[def.ID] = compileRules(def)
		}
	}
	return e
}

func compileRules(def OverlayDef) []compiledRule {
	out := make([]compiledRule, 0, len(def.Rules))
	for _, r := range def.Rules {
		if !r.Enabled {
			continue
		}
		cr := compiledRule{rule: r}
		if r.Pattern != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				continue // Validate already rejected these
			}
			cr.re = re
		}
		if len(r.Speakers) > 0 {
			cr.speakers = make(map[string]bool, len(r.Speakers))
			for _, s := range r.Speakers {
				cr.speakers[strings.ToLower(s)] = true
			}
		}
		out = append(out, cr)
	}
	return out
}

// ── line matching ───────────────────────────────────────────────────────────

var (
	reSayIn  = regexp.MustCompile(`^([A-Za-z]+) says,\s+'(.*)'$`)
	reSayOut = regexp.MustCompile(`^You say,\s+'(.*)'$`)
)

// chatLine is a log line classified as chat, with the speaker resolved.
type chatLine struct {
	channel   string
	direction string
	speaker   string // the local character's name for outgoing lines
	message   string
}

func parseChat(msg, self string) (chatLine, bool) {
	if p, ok := chat.ParseChat(msg); ok {
		cl := chatLine{channel: p.Channel, direction: p.Direction, message: p.Message, speaker: p.Peer}
		if p.Direction == chat.DirectionOut {
			cl.speaker = self
		}
		return cl, true
	}
	if m := reSayIn.FindStringSubmatch(msg); m != nil {
		return chatLine{channel: ChannelSay, direction: DirectionIn, speaker: chat.CapitalizeName(m[1]), message: m[2]}, true
	}
	if m := reSayOut.FindStringSubmatch(msg); m != nil {
		return chatLine{channel: ChannelSay, direction: DirectionOut, speaker: self, message: m[1]}, true
	}
	return chatLine{}, false
}

// lineCtx is one log line as seen by the rules, parsed at most once.
type lineCtx struct {
	ts     time.Time
	msg    string
	self   string
	parsed bool
	chat   chatLine
	isChat bool
}

func (lc *lineCtx) chatInfo() (chatLine, bool) {
	if !lc.parsed {
		lc.chat, lc.isChat = parseChat(lc.msg, lc.self)
		lc.parsed = true
	}
	return lc.chat, lc.isChat
}

// match runs one rule against the line, returning the template tokens when
// it matches.
func (cr *compiledRule) match(lc *lineCtx) (map[string]string, bool) {
	text := lc.msg
	var cl chatLine
	if cr.rule.Channel != "" {
		var ok bool
		cl, ok = lc.chatInfo()
		if !ok {
			return nil, false
		}
		if cr.rule.Channel != ChannelAny && cl.channel != cr.rule.Channel {
			return nil, false
		}
		if cr.rule.Direction != "" && cl.direction != cr.rule.Direction {
			return nil, false
		}
		if cr.speakers != nil && !cr.speakers[strings.ToLower(cl.speaker)] {
			return nil, false
		}
		text = cl.message
	}
	var m []string
	if cr.re != nil {
		m = cr.re.FindStringSubmatch(text)
		if m == nil {
			return nil, false
		}
	} else {
		m = []string{text}
	}
	tok := make(map[string]string, len(m)+8)
	for i, v := range m {
		tok[strconv.Itoa(i)] = v
	}
	if cr.re != nil {
		for i, name := range cr.re.SubexpNames() {
			if name != "" && i < len(m) {
				tok[name] = m[i]
			}
		}
	}
	setDefault := func(k, v string) {
		if _, ok := tok[k]; !ok {
			tok[k] = v
		}
	}
	setDefault("line", lc.msg)
	setDefault("message", text)
	if cr.rule.Channel != "" {
		setDefault("speaker", cl.speaker)
		setDefault("channel", cl.channel)
	}
	if lc.self != "" {
		setDefault("c", lc.self)
		setDefault("char", lc.self)
		setDefault("self", lc.self)
	}
	return tok, true
}

var (
	curlyRe  = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	dollarRe = regexp.MustCompile(`\$(\d+)`)
)

// expand fills {name}/{1}/$1 tokens. Unknown tokens are left as typed so a
// mistake is visible on the overlay rather than silently blank.
func expand(tmpl string, tok map[string]string) string {
	if tmpl == "" || !strings.ContainsAny(tmpl, "{$") {
		return tmpl
	}
	out := curlyRe.ReplaceAllStringFunc(tmpl, func(s string) string {
		if v, ok := tok[s[1:len(s)-1]]; ok {
			return v
		}
		return s
	})
	return dollarRe.ReplaceAllStringFunc(out, func(s string) string {
		if v, ok := tok[s[1:]]; ok {
			return v
		}
		return s
	})
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxCellLen {
		return s[:maxCellLen]
	}
	return s
}

// ── applying ops ────────────────────────────────────────────────────────────

func widgetByID(def *OverlayDef, id string) *Widget {
	for i := range def.Widgets {
		if def.Widgets[i].ID == id {
			return &def.Widgets[i]
		}
	}
	return nil
}

func (st *OverlayState) widget(id string) *WidgetState {
	if st.Widgets == nil {
		st.Widgets = map[string]*WidgetState{}
	}
	ws := st.Widgets[id]
	if ws == nil {
		ws = &WidgetState{}
		st.Widgets[id] = ws
	}
	return ws
}

func parseNum(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// applyOps applies ops to st and reports whether anything changed.
func applyOps(def *OverlayDef, st *OverlayState, ops []Op, tok map[string]string, now time.Time) bool {
	changed := false
	nowMs := now.UnixMilli()
	for _, op := range ops {
		if op.Type == OpResetAll {
			if len(st.Widgets) > 0 {
				st.Widgets = map[string]*WidgetState{}
				changed = true
			}
			continue
		}
		w := widgetByID(def, op.Widget)
		if w == nil {
			continue
		}
		ws := st.widget(w.ID)
		switch op.Type {
		case OpResetWidget:
			if !isEmpty(ws) {
				*ws = WidgetState{}
				changed = true
			}
		case OpSetField:
			v := clip(expand(op.Value, tok))
			if ws.Fields == nil {
				ws.Fields = map[string]string{}
			}
			if ws.Fields[op.Field] != v {
				ws.Fields[op.Field] = v
				changed = true
			}
		case OpClearField:
			if op.Field == "" {
				if len(ws.Fields) > 0 {
					ws.Fields = nil
					changed = true
				}
			} else if _, ok := ws.Fields[op.Field]; ok {
				delete(ws.Fields, op.Field)
				changed = true
			}
		case OpUpsertRow:
			key := clip(expand(op.Key, tok))
			if key == "" {
				continue
			}
			cells := make(map[string]string, len(op.Cells)+1)
			for k, t := range op.Cells {
				cells[k] = clip(expand(t, tok))
			}
			if len(w.Columns) > 0 {
				cells[w.Columns[0].Key] = key
			}
			if upsertRow(w, ws, st, key, cells, nowMs) {
				changed = true
			}
		case OpRemoveRow:
			if removeRow(ws, clip(expand(op.Key, tok))) {
				changed = true
			}
		case OpAddLine:
			text := clip(expand(op.Value, tok))
			if text == "" {
				continue
			}
			ws.Lines = append([]FeedLine{{Text: text, TS: nowMs}}, ws.Lines...)
			if len(ws.Lines) > w.MaxLines {
				ws.Lines = ws.Lines[:w.MaxLines]
			}
			changed = true
		case OpCounterAdd:
			if op.Amount != 0 {
				ws.Count += op.Amount
				changed = true
			}
		case OpCounterSet:
			if f, ok := parseNum(expand(op.Value, tok)); ok && int(f) != ws.Count {
				ws.Count = int(f)
				changed = true
			}
		case OpCountdownStart:
			secs := w.Seconds
			if op.Value != "" {
				secs = trigger.ParseDurationText(strings.TrimSpace(expand(op.Value, tok)))
			}
			if secs > 0 {
				ws.EndsAt = now.Add(time.Duration(secs * float64(time.Second))).UnixMilli()
				changed = true
			}
		case OpCountdownStop:
			if ws.EndsAt != 0 {
				ws.EndsAt = 0
				changed = true
			}
		}
	}
	return changed
}

func isEmpty(ws *WidgetState) bool {
	return len(ws.Fields) == 0 && len(ws.Rows) == 0 && len(ws.Lines) == 0 && ws.Count == 0 && ws.EndsAt == 0
}

func upsertRow(w *Widget, ws *WidgetState, st *OverlayState, key string, cells map[string]string, nowMs int64) bool {
	for i := range ws.Rows {
		row := &ws.Rows[i]
		if !strings.EqualFold(row.Key, key) {
			continue
		}
		switch w.Merge {
		case MergeHighest:
			oldV, okOld := parseNum(row.Cells[w.MergeColumn])
			newV, okNew := parseNum(cells[w.MergeColumn])
			if okOld && (!okNew || newV < oldV) {
				return false
			}
		case MergeAdd:
			oldV, _ := parseNum(row.Cells[w.MergeColumn])
			if newV, ok := parseNum(cells[w.MergeColumn]); ok {
				cells[w.MergeColumn] = formatNum(oldV + newV)
			} else {
				cells[w.MergeColumn] = row.Cells[w.MergeColumn]
			}
		}
		row.Cells = cells
		row.UpdatedAt = nowMs
		return true
	}
	st.NextSeq++
	ws.Rows = append(ws.Rows, Row{Key: key, Cells: cells, Seq: st.NextSeq, UpdatedAt: nowMs})
	if len(ws.Rows) > w.MaxRows {
		// Rows are kept in arrival order, so the oldest is first.
		ws.Rows = ws.Rows[len(ws.Rows)-w.MaxRows:]
	}
	return true
}

func removeRow(ws *WidgetState, key string) bool {
	for i := range ws.Rows {
		if strings.EqualFold(ws.Rows[i].Key, key) {
			ws.Rows = append(ws.Rows[:i], ws.Rows[i+1:]...)
			return true
		}
	}
	return false
}

// pruneState drops state for widgets that no longer exist (or changed type),
// fields/columns that were removed, and trims to the current limits.
func pruneState(st *OverlayState, def OverlayDef) {
	if st.Widgets == nil {
		return
	}
	byID := map[string]*Widget{}
	for i := range def.Widgets {
		byID[def.Widgets[i].ID] = &def.Widgets[i]
	}
	for id, ws := range st.Widgets {
		w := byID[id]
		if w == nil || ws == nil {
			delete(st.Widgets, id)
			continue
		}
		pruned := WidgetState{}
		switch w.Type {
		case WidgetFields:
			for k, v := range ws.Fields {
				if hasField(w, k) {
					if pruned.Fields == nil {
						pruned.Fields = map[string]string{}
					}
					pruned.Fields[k] = v
				}
			}
		case WidgetTable:
			pruned.Rows = ws.Rows
			if len(pruned.Rows) > w.MaxRows {
				pruned.Rows = pruned.Rows[len(pruned.Rows)-w.MaxRows:]
			}
		case WidgetFeed:
			pruned.Lines = ws.Lines
			if len(pruned.Lines) > w.MaxLines {
				pruned.Lines = pruned.Lines[:w.MaxLines]
			}
		case WidgetCounter:
			pruned.Count = ws.Count
		case WidgetCountdown:
			pruned.EndsAt = ws.EndsAt
		}
		*ws = pruned
	}
}

// ── live engine ─────────────────────────────────────────────────────────────

func appliesTo(def *OverlayDef, active string) bool {
	if len(def.Characters) == 0 || active == "" {
		return true
	}
	for _, c := range def.Characters {
		if strings.EqualFold(c, active) {
			return true
		}
	}
	return false
}

func (e *Engine) self() string {
	if e.activeChar == nil {
		return ""
	}
	return e.activeChar()
}

// HandleLine runs every enabled overlay's rules against one log message
// (timestamp stripped).
func (e *Engine) HandleLine(ts time.Time, msg string) {
	lc := &lineCtx{ts: ts, msg: msg, self: e.self()}
	e.mu.Lock()
	changed := false
	for _, o := range e.overlays {
		rules := e.comp[o.Def.ID]
		if !o.Def.Enabled || len(rules) == 0 || !appliesTo(&o.Def, lc.self) {
			continue
		}
		touched := false
		for i := range rules {
			tok, ok := rules[i].match(lc)
			if !ok {
				continue
			}
			if applyOps(&o.Def, &o.State, rules[i].rule.Ops, tok, ts) {
				touched = true
			}
		}
		if touched {
			o.UpdatedAt = ts.UnixMilli()
			e.saveStateLocked(o)
			changed = true
		}
	}
	e.mu.Unlock()
	if changed {
		e.broadcast()
	}
}

// saveStateLocked writes one overlay's state straight through. The sidecar
// exits without running deferred cleanup (os.Exit / process kill), so a
// batched write would lose the last changes on every quit; rules fire at
// chat speed, so writing each change is cheap.
func (e *Engine) saveStateLocked(o *Overlay) {
	if e.store == nil || e.replaySaved != nil {
		return
	}
	if err := e.store.SaveState(o.Def.ID, o.State, o.UpdatedAt); err != nil {
		slog.Warn("customoverlay: save state", "id", o.Def.ID, "err", err)
	}
}

func (e *Engine) broadcast() {
	if e.hub != nil {
		e.hub.Broadcast(ws.Event{Type: WSEventState, Data: e.State()})
	}
}

func cloneState(st OverlayState) OverlayState {
	b, err := json.Marshal(st)
	if err != nil {
		return OverlayState{}
	}
	var out OverlayState
	_ = json.Unmarshal(b, &out)
	return out
}

// sortedRows returns a table's rows in display order.
func sortedRows(w *Widget, rows []Row) []Row {
	out := append([]Row(nil), rows...)
	if w.SortBy == "" {
		if w.SortDesc {
			sort.SliceStable(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
		}
		return out
	}
	less := func(a, b Row) int {
		av, bv := a.Cells[w.SortBy], b.Cells[w.SortBy]
		an, aok := parseNum(av)
		bn, bok := parseNum(bv)
		switch {
		case aok && bok:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aok != bok:
			// Numbers before text either way, so blanks sink.
			if aok {
				return -2
			}
			return 2
		default:
			if c := strings.Compare(strings.ToLower(av), strings.ToLower(bv)); c != 0 {
				return c
			}
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		c := less(out[i], out[j])
		if c == -2 || c == 2 {
			return c == -2
		}
		if c == 0 {
			return out[i].Seq < out[j].Seq
		}
		if w.SortDesc {
			return c > 0
		}
		return c < 0
	})
	return out
}

// snapshot copies an overlay for display: deep-copied state with table rows
// sorted.
func snapshot(o *Overlay) Overlay {
	cp := Overlay{Def: o.Def, State: cloneState(o.State), UpdatedAt: o.UpdatedAt}
	if cp.State.Widgets == nil {
		cp.State.Widgets = map[string]*WidgetState{}
	}
	for i := range cp.Def.Widgets {
		w := &cp.Def.Widgets[i]
		if ws := cp.State.Widgets[w.ID]; ws != nil && w.Type == WidgetTable {
			ws.Rows = sortedRows(w, ws.Rows)
		}
	}
	return cp
}

// State returns a snapshot of every overlay.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := State{Overlays: make([]Overlay, 0, len(e.overlays)), Now: e.now().UnixMilli()}
	for _, o := range e.overlays {
		out.Overlays = append(out.Overlays, snapshot(o))
	}
	return out
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (e *Engine) findLocked(id string) *Overlay {
	for _, o := range e.overlays {
		if o.Def.ID == id {
			return o
		}
	}
	return nil
}

func (e *Engine) indexLocked(id string) int {
	for i, o := range e.overlays {
		if o.Def.ID == id {
			return i
		}
	}
	return -1
}

func (e *Engine) saveLocked(o *Overlay) {
	if e.store == nil {
		return
	}
	if e.replaySaved != nil {
		// Mid-replay definition edits still save, with the pre-replay
		// contents rather than the replayed ones.
		cp := *o
		cp.State = e.replaySaved[o.Def.ID]
		o = &cp
	}
	if err := e.store.Save(o, e.indexLocked(o.Def.ID)); err != nil {
		slog.Warn("customoverlay: save", "id", o.Def.ID, "err", err)
	}
}

// Create validates and adds an overlay, returning it with its new ID.
func (e *Engine) Create(def OverlayDef) (Overlay, error) {
	def.ID = ""
	def, err := Validate(def)
	if err != nil {
		return Overlay{}, err
	}
	def.ID = newID()
	o := &Overlay{Def: def, State: OverlayState{Widgets: map[string]*WidgetState{}}, UpdatedAt: e.now().UnixMilli()}
	e.mu.Lock()
	e.overlays = append(e.overlays, o)
	e.comp[def.ID] = compileRules(def)
	if e.replaySaved != nil {
		e.replaySaved[def.ID] = OverlayState{Widgets: map[string]*WidgetState{}}
	}
	e.saveLocked(o)
	snap := snapshot(o)
	e.mu.Unlock()
	e.broadcast()
	return snap, nil
}

// Update replaces an overlay's definition, keeping whatever live state still
// fits the new widgets.
func (e *Engine) Update(def OverlayDef) (Overlay, error) {
	def, err := Validate(def)
	if err != nil {
		return Overlay{}, err
	}
	e.mu.Lock()
	o := e.findLocked(def.ID)
	if o == nil {
		e.mu.Unlock()
		return Overlay{}, ErrNotFound
	}
	o.Def = def
	pruneState(&o.State, def)
	e.comp[def.ID] = compileRules(def)
	o.UpdatedAt = e.now().UnixMilli()
	e.saveLocked(o)
	snap := snapshot(o)
	e.mu.Unlock()
	e.broadcast()
	return snap, nil
}

// Delete removes an overlay.
func (e *Engine) Delete(id string) error {
	e.mu.Lock()
	idx := e.indexLocked(id)
	if idx < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	e.overlays = append(e.overlays[:idx], e.overlays[idx+1:]...)
	delete(e.comp, id)
	if e.store != nil {
		if err := e.store.Delete(id); err != nil {
			slog.Warn("customoverlay: delete", "id", id, "err", err)
		}
		for _, o := range e.overlays[idx:] {
			e.saveLocked(o) // keep sort_order contiguous
		}
	}
	e.mu.Unlock()
	e.broadcast()
	return nil
}

// Move shifts an overlay one place up (dir < 0) or down (dir > 0).
func (e *Engine) Move(id string, dir int) error {
	e.mu.Lock()
	idx := e.indexLocked(id)
	if idx < 0 {
		e.mu.Unlock()
		return ErrNotFound
	}
	to := idx + 1
	if dir < 0 {
		to = idx - 1
	}
	if to < 0 || to >= len(e.overlays) {
		e.mu.Unlock()
		return nil
	}
	e.overlays[idx], e.overlays[to] = e.overlays[to], e.overlays[idx]
	e.saveLocked(e.overlays[idx])
	e.saveLocked(e.overlays[to])
	e.mu.Unlock()
	e.broadcast()
	return nil
}

// mutate applies fn to one overlay's state under the lock, persisting and
// broadcasting when it reports a change.
func (e *Engine) mutate(id string, fn func(o *Overlay) bool) error {
	e.mu.Lock()
	o := e.findLocked(id)
	if o == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	changed := fn(o)
	if changed {
		o.UpdatedAt = e.now().UnixMilli()
		e.saveLocked(o)
	}
	e.mu.Unlock()
	if changed {
		e.broadcast()
	}
	return nil
}

// Reset clears every widget's contents.
func (e *Engine) Reset(id string) error {
	return e.mutate(id, func(o *Overlay) bool {
		return applyOps(&o.Def, &o.State, []Op{{Type: OpResetAll}}, nil, e.now())
	})
}

// ResetWidget clears one widget's contents.
func (e *Engine) ResetWidget(id, widget string) error {
	return e.mutate(id, func(o *Overlay) bool {
		return applyOps(&o.Def, &o.State, []Op{{Type: OpResetWidget, Widget: widget}}, nil, e.now())
	})
}

// RemoveRow deletes one table row by key (the overlay's right-click remove).
func (e *Engine) RemoveRow(id, widget, key string) error {
	return e.mutate(id, func(o *Overlay) bool {
		w := widgetByID(&o.Def, widget)
		if w == nil || w.Type != WidgetTable || o.State.Widgets[widget] == nil {
			return false
		}
		return removeRow(o.State.Widgets[widget], key)
	})
}

// SetEnabled turns an overlay's rules on or off without touching its state.
func (e *Engine) SetEnabled(id string, enabled bool) error {
	return e.mutate(id, func(o *Overlay) bool {
		if o.Def.Enabled == enabled {
			return false
		}
		o.Def.Enabled = enabled
		return true
	})
}

// SetReplaying is called when a Log Feed replay starts and stops. While it
// runs, overlays react to replayed lines but nothing is saved; when it stops,
// every overlay returns to its pre-replay contents.
func (e *Engine) SetReplaying(active bool) {
	e.mu.Lock()
	if active == (e.replaySaved != nil) {
		e.mu.Unlock()
		return
	}
	if active {
		e.replaySaved = make(map[string]OverlayState, len(e.overlays))
		for _, o := range e.overlays {
			e.replaySaved[o.Def.ID] = cloneState(o.State)
		}
		e.mu.Unlock()
		return
	}
	for _, o := range e.overlays {
		o.State = e.replaySaved[o.Def.ID]
		if o.State.Widgets == nil {
			o.State.Widgets = map[string]*WidgetState{}
		}
		pruneState(&o.State, o.Def)
	}
	e.replaySaved = nil
	e.mu.Unlock()
	e.broadcast()
}

// TestLine dry-runs one log message against an unsaved definition, starting
// from the saved overlay's current contents when def.ID names one (so the
// editor shows what the line would do right now) and from empty otherwise.
// Nothing is stored or broadcast. character overrides the active character
// for {c} and the Characters filter is ignored.
func (e *Engine) TestLine(def OverlayDef, line, character string) TestResult {
	id := def.ID
	def, err := Validate(def)
	if err != nil {
		return TestResult{Matched: []string{}, Error: err.Error()}
	}
	if character == "" {
		character = e.self()
	}
	st := OverlayState{Widgets: map[string]*WidgetState{}}
	if id != "" {
		e.mu.Lock()
		if o := e.findLocked(id); o != nil {
			st = cloneState(o.State)
		}
		e.mu.Unlock()
		pruneState(&st, def)
	}
	lc := &lineCtx{ts: e.now(), msg: line, self: character}
	res := TestResult{Matched: []string{}}
	for _, cr := range compileRules(def) {
		tok, ok := cr.match(lc)
		if !ok {
			continue
		}
		res.Matched = append(res.Matched, cr.rule.ID)
		applyOps(&def, &st, cr.rule.Ops, tok, lc.ts)
	}
	snap := snapshot(&Overlay{Def: def, State: st})
	res.State = snap.State
	return res
}

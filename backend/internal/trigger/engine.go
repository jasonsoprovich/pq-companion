package trigger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const historyMaxSize = 200

// TimerSink is the minimal interface the trigger engine needs to start and
// stop externally-driven spell timers. It is implemented by
// *spelltimer.Engine; kept abstract here to avoid an import cycle.
//
// displayThresholdSecs is forwarded to the engine so a per-trigger
// threshold (Trigger.DisplayThresholdSecs) can override the global
// buff/detrim defaults the user sets in Settings. 0 means "use the
// category default."
//
// alerts is the per-trigger fading-soon configuration (Trigger.TimerAlerts)
// pre-marshalled to JSON. Sink stores it opaquely on the active timer and
// re-emits it on the WS payload; the frontend parses and acts on it.
//
// spellID is the optional DB spell id this trigger was created from
// (Trigger.SpellID). When > 0, the sink looks up the spell and applies the
// active character's item/AA duration focuses to durationSecs so a
// trigger-driven timer extends to the same length as the spell-landed
// pipeline would produce. 0 = use durationSecs as given.
type TimerSink interface {
	StartExternal(name, category string, durationSecs, displayThresholdSecs int, startedAt time.Time, alerts json.RawMessage, spellID int)
	StopExternal(name string, spellID int)
}

// compiled pairs a Trigger with its pre-compiled patterns for efficient matching.
type compiled struct {
	trigger  *Trigger
	re       *regexp.Regexp
	wornOff  *regexp.Regexp // non-nil only when the trigger has a worn-off pattern
	timerKey string         // cached spelltimer key when timer_type != none
	// extras are the precompiled enabled ExtraPatterns. The trigger fires
	// when the primary pattern OR any extra matches; the first matching
	// pattern's capture groups feed the actions.
	extras []*regexp.Regexp
	// excludes are precompiled ExcludePatterns; if any match the same line
	// the primary match is suppressed. Lets a broad pattern (e.g. "incoming
	// tell") filter pet/merchant lines without needing RE2 lookbehind.
	excludes []*regexp.Regexp
}

// Engine loads triggers from the store and tests every incoming log line
// against them, firing actions and broadcasting events on match.
type Engine struct {
	store      *Store
	hub        *ws.Hub
	sink       TimerSink
	activeChar func() string // returns active character name, "" if unknown
	// currentTarget returns the inferred combat target's display name, "" when
	// no target. Wired to overlay.NPCTracker via SetTargetProvider; nil when
	// target integration is disabled (tests). Feeds the {target} action token.
	currentTarget func() string

	mu           sync.RWMutex
	compiled     []compiled // Source=="log" triggers, indexed by regex
	pipeCompiled []*Trigger // Source=="pipe" triggers, evaluated by PipeCondition

	// Pipe edge state. The engine compares each new pipe update against
	// these values so "drops below 20%" and "buff X falls off" don't refire
	// at every ~100 ms pipe tick. Reset to zero values on disconnect (via
	// HandlePipeReset) so a fresh session starts clean.
	pipeMu         sync.Mutex
	prevTargetName string
	prevTargetHP   int // 0 means "no prior value seen"; we treat that as 100% for threshold purposes
	prevBuffSet    map[string]bool

	histMu  sync.Mutex
	history []TriggerFired // ring buffer, newest appended last
}

// NewEngine creates an Engine backed by store. Call Reload before routing
// lines. sink may be nil when timer integration is disabled (e.g. in tests).
// activeChar returns the currently active character name; nil disables
// per-character filtering (used by tests).
func NewEngine(store *Store, hub *ws.Hub, sink TimerSink, activeChar func() string) *Engine {
	return &Engine{store: store, hub: hub, sink: sink, activeChar: activeChar}
}

// SetTargetProvider wires a current-target lookup (overlay.NPCTracker) into
// the engine so action text can reference {target}/{t}. Call before routing
// lines; nil leaves the token unresolved.
func (e *Engine) SetTargetProvider(fn func() string) {
	e.currentTarget = fn
}

// Reload re-reads all enabled triggers from the store and recompiles their
// patterns. Must be called after any CRUD mutation to keep the engine in sync.
func (e *Engine) Reload() {
	triggers, err := e.store.List()
	if err != nil {
		slog.Error("trigger: reload failed", "err", err)
		return
	}

	// Patterns referencing {c}/{char}/{self} expand to the active character
	// name at compile time, so Reload must rerun when the character changes
	// (wired via the tailer's onCharacterChange in main.go).
	character := ""
	if e.activeChar != nil {
		character = e.activeChar()
	}

	var cs []compiled
	var pipeCs []*Trigger
	for _, t := range triggers {
		if !t.Enabled {
			continue
		}
		// Pipe-source triggers don't use a regex pattern. Validate that
		// they have a usable condition and queue them separately.
		if t.Source == SourcePipe {
			if t.PipeCondition == nil || t.PipeCondition.Kind == "" {
				slog.Warn("trigger: pipe trigger missing condition, skipping", "id", t.ID, "name", t.Name)
				continue
			}
			pipeCs = append(pipeCs, t)
			continue
		}
		re, err := regexp.Compile(normalizePattern(t.Pattern, character))
		if err != nil {
			slog.Warn("trigger: invalid pattern, skipping", "id", t.ID, "name", t.Name, "err", err)
			continue
		}
		c := compiled{trigger: t, re: re}
		for _, ep := range t.ExtraPatterns {
			if !ep.Enabled || ep.Pattern == "" {
				continue
			}
			ex, err := regexp.Compile(normalizePattern(ep.Pattern, character))
			if err != nil {
				slog.Warn("trigger: invalid extra pattern, skipping", "id", t.ID, "name", t.Name, "pattern", ep.Pattern, "err", err)
				continue
			}
			c.extras = append(c.extras, ex)
		}
		if t.WornOffPattern != "" {
			if wornRe, err := regexp.Compile(normalizePattern(t.WornOffPattern, character)); err == nil {
				c.wornOff = wornRe
			} else {
				slog.Warn("trigger: invalid worn-off pattern", "id", t.ID, "name", t.Name, "err", err)
			}
		}
		if timerCategory(t.TimerType) != "" {
			c.timerKey = timerKeyFor(t)
		}
		for _, p := range t.ExcludePatterns {
			if p == "" {
				continue
			}
			ex, err := regexp.Compile(normalizePattern(p, character))
			if err != nil {
				slog.Warn("trigger: invalid exclude pattern, skipping", "id", t.ID, "name", t.Name, "pattern", p, "err", err)
				continue
			}
			c.excludes = append(c.excludes, ex)
		}
		cs = append(cs, c)
	}

	e.mu.Lock()
	e.compiled = cs
	e.pipeCompiled = pipeCs
	e.mu.Unlock()

	slog.Info("trigger: reloaded", "log_active", len(cs), "pipe_active", len(pipeCs))
}

// Handle tests a raw log line message against all enabled triggers.
// timestamp is when the line was logged; message is the text after the EQ
// timestamp prefix (i.e. the bare log message, without brackets).
func (e *Engine) Handle(timestamp time.Time, message string) {
	e.mu.RLock()
	cs := e.compiled
	e.mu.RUnlock()

	active := ""
	if e.activeChar != nil {
		active = e.activeChar()
	}

	for _, c := range cs {
		if !triggerAppliesTo(c.trigger, active) {
			continue
		}
		// Any-pattern semantics: try the primary pattern first, then each
		// enabled extra. The first match wins and supplies the capture
		// groups; excludes suppress regardless of which pattern matched.
		m, names := c.re.FindStringSubmatch(message), c.re.SubexpNames()
		for i := 0; m == nil && i < len(c.extras); i++ {
			m, names = c.extras[i].FindStringSubmatch(message), c.extras[i].SubexpNames()
		}
		if m != nil && !matchesAny(c.excludes, message) {
			e.fire(c, message, timestamp, m, names)
		}
		if c.wornOff != nil && c.wornOff.MatchString(message) {
			if e.sink != nil && c.timerKey != "" {
				e.sink.StopExternal(c.timerKey, c.trigger.SpellID)
			}
		}
	}
}

// HandlePipeTarget evaluates target_hp_below + target_name pipe triggers
// against a new target snapshot. hp == -1 means "no HP reading available"
// (e.g. no target selected); we skip HP-below evaluation in that case but
// still treat the name change as a transition. character is the Zeal
// envelope's character field, used for the same per-character filtering
// log triggers respect.
func (e *Engine) HandlePipeTarget(name string, hp int, character string, ts time.Time) {
	e.pipeMu.Lock()
	prevName := e.prevTargetName
	prevHP := e.prevTargetHP
	e.prevTargetName = name
	if hp >= 0 {
		e.prevTargetHP = hp
	} else {
		e.prevTargetHP = 0
	}
	e.pipeMu.Unlock()

	e.mu.RLock()
	pipeCs := e.pipeCompiled
	e.mu.RUnlock()
	if len(pipeCs) == 0 {
		return
	}
	active := ""
	if e.activeChar != nil {
		active = e.activeChar()
	}
	for _, t := range pipeCs {
		if !triggerAppliesTo(t, active) {
			continue
		}
		cond := t.PipeCondition
		if cond == nil {
			continue
		}
		switch cond.Kind {
		case PipeKindTargetName:
			if name != "" && name == cond.TargetName && prevName != name {
				e.firePipe(t, fmt.Sprintf("target: %s", name), ts)
			}
		case PipeKindTargetHPBelow:
			if hp < 0 {
				continue
			}
			// Edge: prev > threshold, now <= threshold. prev==0 with no prior
			// read counts as "100%" so the first reading below threshold
			// after selecting a low-HP target fires once.
			prev := prevHP
			if prev == 0 {
				prev = 101
			}
			if prev > cond.HPThreshold && hp <= cond.HPThreshold {
				e.firePipe(t, fmt.Sprintf("target hp %d%%", hp), ts)
			}
		}
	}
}

// HandlePipeBuffSlots evaluates buff_landed + buff_faded triggers against
// the current self-buff slot snapshot from the pipe. names contains the
// spell names in occupied slots (empty slots are simply absent from the
// pipe payload).
func (e *Engine) HandlePipeBuffSlots(names []string, character string, ts time.Time) {
	curr := make(map[string]bool, len(names))
	for _, n := range names {
		if n != "" {
			curr[n] = true
		}
	}
	e.pipeMu.Lock()
	prev := e.prevBuffSet
	e.prevBuffSet = curr
	e.pipeMu.Unlock()

	e.mu.RLock()
	pipeCs := e.pipeCompiled
	e.mu.RUnlock()
	if len(pipeCs) == 0 {
		return
	}
	active := ""
	if e.activeChar != nil {
		active = e.activeChar()
	}
	for _, t := range pipeCs {
		if !triggerAppliesTo(t, active) {
			continue
		}
		cond := t.PipeCondition
		if cond == nil {
			continue
		}
		switch cond.Kind {
		case PipeKindBuffLanded:
			if cond.SpellName == "" {
				continue
			}
			if curr[cond.SpellName] && !prev[cond.SpellName] {
				e.firePipe(t, fmt.Sprintf("buff landed: %s", cond.SpellName), ts)
			}
		case PipeKindBuffFaded:
			if cond.SpellName == "" {
				continue
			}
			if prev[cond.SpellName] && !curr[cond.SpellName] {
				e.firePipe(t, fmt.Sprintf("buff faded: %s", cond.SpellName), ts)
			}
		}
	}
}

// HandlePipeCommand evaluates pipe_command triggers against a /pipe <text>
// envelope. One-shot — no edge state; fires whenever a matching command
// text is seen.
func (e *Engine) HandlePipeCommand(text, character string, ts time.Time) {
	e.mu.RLock()
	pipeCs := e.pipeCompiled
	e.mu.RUnlock()
	if len(pipeCs) == 0 {
		return
	}
	active := ""
	if e.activeChar != nil {
		active = e.activeChar()
	}
	for _, t := range pipeCs {
		if !triggerAppliesTo(t, active) {
			continue
		}
		cond := t.PipeCondition
		if cond == nil || cond.Kind != PipeKindPipeCommand {
			continue
		}
		if cond.Text != "" && cond.Text == text {
			e.firePipe(t, fmt.Sprintf("/pipe %s", text), ts)
		}
	}
}

// HandlePipeReset clears edge-detection state. Called when the supervisor
// disconnects so the next session doesn't see spurious "transition" matches
// against stale values from a previous Zeal run.
func (e *Engine) HandlePipeReset() {
	e.pipeMu.Lock()
	e.prevTargetName = ""
	e.prevTargetHP = 0
	e.prevBuffSet = nil
	e.pipeMu.Unlock()
}

// firePipe records a pipe-driven match the same way fire() does for log
// triggers, including timer-sink dispatch when the trigger has a timer
// type. matchedLine is a synthetic human-readable description of the
// trigger condition for the history pane.
func (e *Engine) firePipe(t *Trigger, matchedLine string, firedAt time.Time) {
	// Pipe triggers have no regex captures, but built-in tokens
	// ({c}/{target}) still substitute into action text. Copy-on-write,
	// same as fire().
	actions := t.Actions
	if builtins := e.builtinTokens(); len(builtins) > 0 {
		actions = make([]Action, len(t.Actions))
		copy(actions, t.Actions)
		for i := range actions {
			actions[i].Text = substituteCaptures(actions[i].Text, nil, nil, builtins)
		}
	}
	event := TriggerFired{
		TriggerID:   t.ID,
		TriggerName: t.Name,
		MatchedLine: matchedLine,
		Actions:     actions,
		FiredAt:     firedAt,
	}
	e.histMu.Lock()
	e.history = append(e.history, event)
	if len(e.history) > historyMaxSize {
		e.history = e.history[len(e.history)-historyMaxSize:]
	}
	e.histMu.Unlock()

	e.hub.Broadcast(ws.Event{Type: WSEventTriggerFired, Data: event})
	slog.Debug("trigger fired (pipe)", "trigger", t.Name, "match", matchedLine)

	if e.sink != nil && t.TimerDurationSecs > 0 && timerCategory(t.TimerType) != "" {
		var alertJSON json.RawMessage
		if len(t.TimerAlerts) > 0 {
			if buf, err := json.Marshal(t.TimerAlerts); err == nil {
				alertJSON = buf
			}
		}
		e.sink.StartExternal(timerKeyFor(t), timerCategory(t.TimerType),
			t.TimerDurationSecs, t.DisplayThresholdSecs, firedAt, alertJSON, t.SpellID)
	}
	e.startCooldownTimer(t, firedAt)
}

// matchesAny returns true if any of the regexes match s. Used to suppress a
// primary trigger match when one of its ExcludePatterns also matches the
// same line.
func matchesAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// curlyCaptureRe / dollarCaptureRe find capture references in trigger action
// text: {name_or_number} and $number respectively.
var (
	curlyCaptureRe  = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	dollarCaptureRe = regexp.MustCompile(`\$(\d+)`)
)

// dotnetNamedGroupRe rewrites .NET-style (?<name>…) named groups — the syntax
// GINA exports use — into Go's (?P<name>…). Lookbehind assertions ((?<= and
// (?<!) don't match the alpha-first capture name so they pass through (RE2
// rejects them either way, but the compile error should name the real issue).
var dotnetNamedGroupRe = regexp.MustCompile(`\(\?<([A-Za-z][A-Za-z0-9_]*)>`)

// patternTokenRe finds brace tokens in a trigger pattern: {C}, {S}/{S1}…,
// {N}/{N1}…, {char}, {self}. Repetition syntax like \d{2} contains no letters
// so it never matches.
var patternTokenRe = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

// normalizePattern expands GINA-style convenience tokens in a trigger pattern
// before regex compilation:
//
//	{c} {char} {self} {C}   the active character's name, quoted literally —
//	                        left untouched (an unmatchable literal) until a
//	                        character is detected; Reload reruns on change
//	{S} {S1}…{S9}           text wildcard → (?P<SN>.+)
//	{N} {N1}…{N9}           number wildcard → (?P<NN>[0-9]+)
//
// and converts .NET (?<name>…) named groups to Go (?P<name>…) syntax so raw
// GINA regexes compile. Unrecognized brace tokens are left as-is (Go treats
// them as literals).
func normalizePattern(pattern, character string) string {
	pattern = dotnetNamedGroupRe.ReplaceAllString(pattern, `(?P<$1>`)
	return patternTokenRe.ReplaceAllStringFunc(pattern, func(tok string) string {
		key := tok[1 : len(tok)-1]
		switch strings.ToLower(key) {
		case "c", "char", "self":
			if character == "" {
				return tok
			}
			return regexp.QuoteMeta(character)
		}
		up := strings.ToUpper(key)
		if len(up) <= 2 && (up[0] == 'S' || up[0] == 'N') &&
			(len(up) == 1 || (up[1] >= '1' && up[1] <= '9')) {
			if up[0] == 'S' {
				return "(?P<" + up + ">.+)"
			}
			return "(?P<" + up + ">[0-9]+)"
		}
		return tok
	})
}

// substituteCaptures fills regex capture references in template using a matched
// line's submatches (#132) plus engine-provided built-in tokens. Supported:
//
//	{N} / $N        numbered group (0 = the whole matched line)
//	{name}          named group from a (?P<name>...) pattern
//	{S} {S1}…{S9}   GINA-style aliases for groups 1…9 (only when the pattern
//	                doesn't define a named group with that exact name)
//	{c}/{char}/{self}, {target}/{t}   built-ins (case-insensitive) supplied
//	                via builtins — active character and current combat target
//
// A reference that doesn't resolve is left untouched, so literal braces or
// dollar signs in alert text survive unchanged. Explicit capture groups always
// win over built-ins, so a (?P<target>…) group shadows the {target} built-in.
func substituteCaptures(template string, match []string, names []string, builtins map[string]string) string {
	if template == "" || (len(match) == 0 && len(builtins) == 0) {
		return template
	}
	lookup := make(map[string]string, len(match)*2)
	for i, v := range match {
		lookup[strconv.Itoa(i)] = v
		if i < len(names) && names[i] != "" {
			lookup[names[i]] = v
		}
	}
	resolve := func(key string) (string, bool) {
		if v, ok := lookup[key]; ok {
			return v, true
		}
		// GINA-style {S}/{SN} alias → numbered group (bare {S} = group 1).
		if l := len(key); l <= 2 && (key[0] == 'S' || key[0] == 's') {
			n := "1"
			if l == 2 {
				if key[1] < '1' || key[1] > '9' {
					return "", false
				}
				n = key[1:]
			}
			if v, ok := lookup[n]; ok {
				return v, true
			}
			return "", false
		}
		if v, ok := builtins[strings.ToLower(key)]; ok {
			return v, true
		}
		return "", false
	}
	template = curlyCaptureRe.ReplaceAllStringFunc(template, func(tok string) string {
		if v, ok := resolve(tok[1 : len(tok)-1]); ok { // strip { }
			return v
		}
		return tok
	})
	template = dollarCaptureRe.ReplaceAllStringFunc(template, func(tok string) string {
		if v, ok := lookup[tok[1:]]; ok { // strip $
			return v
		}
		return tok
	})
	return template
}

// builtinTokens assembles the implicit substitution values available to every
// action: the active character ({c}/{char}/{self}) and the current combat
// target ({target}/{t}). Empty values are omitted so unresolved tokens stay
// visible in the alert text rather than silently vanishing.
func (e *Engine) builtinTokens() map[string]string {
	b := make(map[string]string, 5)
	if e.activeChar != nil {
		if c := e.activeChar(); c != "" {
			b["c"], b["char"], b["self"] = c, c, c
		}
	}
	if e.currentTarget != nil {
		if t := e.currentTarget(); t != "" {
			b["target"], b["t"] = t, t
		}
	}
	return b
}

// triggerAppliesTo reports whether the trigger should fire for the given
// active character. Empty Characters list = applies to any character (legacy
// safety fallback). Empty active = trigger fires regardless (no character
// detected yet — preserves boot-time behavior).
func triggerAppliesTo(t *Trigger, active string) bool {
	if len(t.Characters) == 0 || active == "" {
		return true
	}
	for _, name := range t.Characters {
		if name == active {
			return true
		}
	}
	return false
}

// GetHistory returns a copy of the recent trigger firing history, newest last.
func (e *Engine) GetHistory() []TriggerFired {
	e.histMu.Lock()
	defer e.histMu.Unlock()
	result := make([]TriggerFired, len(e.history))
	copy(result, e.history)
	return result
}

// ── internal ─────────────────────────────────────────────────────────────────

// fire emits a trigger's actions. match/names are the regex submatches and
// their names from the line that matched, used to substitute capture
// references ({1}, $1, {name}) into the action text (#132).
func (e *Engine) fire(c compiled, matchedLine string, firedAt time.Time, match []string, names []string) {
	t := c.trigger

	// Substitute regex captures into the action text on a copy — never mutate
	// the shared trigger. Done for every fire so {1}/{name} in overlay or TTS
	// text resolve to the matched values.
	builtins := e.builtinTokens()
	actions := make([]Action, len(t.Actions))
	copy(actions, t.Actions)
	for i := range actions {
		actions[i].Text = substituteCaptures(actions[i].Text, match, names, builtins)
	}

	event := TriggerFired{
		TriggerID:   t.ID,
		TriggerName: t.Name,
		MatchedLine: matchedLine,
		Actions:     actions,
		FiredAt:     firedAt,
	}

	e.histMu.Lock()
	e.history = append(e.history, event)
	if len(e.history) > historyMaxSize {
		e.history = e.history[len(e.history)-historyMaxSize:]
	}
	e.histMu.Unlock()

	e.hub.Broadcast(ws.Event{Type: WSEventTriggerFired, Data: event})
	slog.Debug("trigger fired", "trigger", t.Name, "line", matchedLine)

	if e.sink != nil && c.timerKey != "" {
		if durationSecs := resolveTimerDuration(t, match, names); durationSecs > 0 {
			var alertJSON json.RawMessage
			if len(t.TimerAlerts) > 0 {
				if buf, err := json.Marshal(t.TimerAlerts); err == nil {
					alertJSON = buf
				}
			}
			e.sink.StartExternal(c.timerKey, timerCategory(t.TimerType), durationSecs, t.DisplayThresholdSecs, firedAt, alertJSON, t.SpellID)
		}
	}
	e.startCooldownTimer(t, firedAt)
}

// timerKeyFor returns the spelltimer key for a trigger. Prefers the trigger
// name so user-configured timers are stable across edits of the pattern.
func timerKeyFor(t *Trigger) string {
	if t.Name != "" {
		return t.Name
	}
	return t.ID
}

// cooldownKeyFor returns the spelltimer key used for a trigger's cooldown
// timer — same root as the primary timer with a " CD" suffix so the buff
// overlay shows e.g. "Furious Discipline" and "Furious Discipline CD" side
// by side without colliding.
func cooldownKeyFor(t *Trigger) string {
	return timerKeyFor(t) + " CD"
}

// startCooldownTimer spawns a second timer on the buff overlay tracking the
// trigger's CooldownSecs (recast_time from spells_new). Fires a TTS "ready"
// alert at 1s remaining. No-op when CooldownSecs is 0 or no sink is wired.
// SpellID is intentionally passed as 0 — duration focuses don't apply to
// recast time, so we want the raw CooldownSecs value, not a focused one.
func (e *Engine) startCooldownTimer(t *Trigger, firedAt time.Time) {
	if e.sink == nil || t.CooldownSecs <= 0 {
		return
	}
	// TTS template uses the trigger's own name as a literal (not {spell})
	// because the cooldown timer's spell_name is the suffixed "<Name> CD"
	// key — substituting would say "Furious Discipline CD ready".
	readyAlert := TimerAlert{
		ID:          "cooldown-ready-1s",
		Seconds:     1,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: t.Name + " ready",
		TTSVolume:   100,
	}
	var alertJSON json.RawMessage
	if buf, err := json.Marshal([]TimerAlert{readyAlert}); err == nil {
		alertJSON = buf
	}
	e.sink.StartExternal(cooldownKeyFor(t), "buff", t.CooldownSecs, 0, firedAt, alertJSON, 0)
}

// timerCategory maps a trigger's TimerType onto a spelltimer category string.
// Kept in string form to avoid depending on the spelltimer package here.
// Empty string = the timer type starts no timer.
func timerCategory(tt TimerType) string {
	switch tt {
	case TimerTypeBuff:
		return "buff"
	case TimerTypeDetrimental:
		return "debuff"
	case TimerTypeCustom:
		return "custom"
	}
	return ""
}

// durationUnitsRe parses "2h30m", "6m40s", "90s", "5m"-style durations.
var durationUnitsRe = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?$`)

// ParseDurationText converts a human duration string captured from a log line
// into seconds. Accepted: plain seconds ("400"), colon notation ("6:40",
// "1:02:03"), and unit notation ("6m40s", "2h", "90s"). Returns 0 when the
// text doesn't parse.
func ParseDurationText(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			return 0
		}
		return n
	}
	if strings.Contains(s, ":") { // MM:SS or HH:MM:SS
		total := 0
		for _, p := range strings.Split(s, ":") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 {
				return 0
			}
			total = total*60 + n
		}
		return total
	}
	m := durationUnitsRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	atoi := func(v string) int {
		if v == "" {
			return 0
		}
		n, _ := strconv.Atoi(v)
		return n
	}
	return atoi(m[1])*3600 + atoi(m[2])*60 + atoi(m[3])
}

// resolveTimerDuration returns the timer duration for a fire: the text
// captured by TimerDurationCapture when configured and parseable, otherwise
// the trigger's fixed TimerDurationSecs.
func resolveTimerDuration(t *Trigger, match []string, names []string) int {
	if t.TimerDurationCapture != "" && len(match) > 0 {
		ref := "{" + t.TimerDurationCapture + "}"
		if v := substituteCaptures(ref, match, names, nil); v != ref {
			if secs := ParseDurationText(v); secs > 0 {
				return secs
			}
		}
	}
	return t.TimerDurationSecs
}

// NewID generates a short random hex identifier suitable for trigger IDs.
func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

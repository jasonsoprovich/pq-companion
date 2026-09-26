package trigger

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// This file implements the Trigger Tester: pasting log lines into the
// Triggers page's Tester tab (or a sample line into the trigger editor) and
// running them through the real trigger engine as if they'd come from the
// live log — without a raid to test against. See planFire/applyFire in
// engine.go for the underlying split that makes "report what would happen"
// possible without requiring the match to actually fire.
//
// A test run never touches the log pipeline's other consumers (spell
// timers aside — a matched trigger's own timer still starts, so the
// overlay/audio can be previewed), the database, or the live tailer/
// replayer. It uses its own fireContext (refire-cooldown + boss-cast state)
// so pasted lines can never suppress a live trigger's cooldown or bind a
// live signature-spell timer to a fabricated caster observation.

// maxTestLines caps how many pasted lines a single test run processes, so a
// user accidentally pasting an entire multi-megabyte log file doesn't hang
// the tester (or, in real-time mode, take literal hours to finish).
const maxTestLines = 5000

// LineStatus categorizes why a trigger's pattern matching a test line did or
// didn't result in a fire.
type LineStatus string

const (
	// LineStatusMatched means the pattern matched and nothing suppressed it
	// — the trigger would really fire on this line.
	LineStatusMatched LineStatus = "matched"
	// LineStatusExcluded means the pattern matched but one of the trigger's
	// ExcludePatterns also matched, suppressing it.
	LineStatusExcluded LineStatus = "excluded"
	// LineStatusCooldown means the pattern matched but RefireCooldownSecs
	// suppressed it (a previous line in this same test run fired it too
	// recently).
	LineStatusCooldown LineStatus = "cooldown"
	// LineStatusWrongCharacter means the pattern matched but the trigger's
	// Characters list doesn't include the character the test is running as.
	LineStatusWrongCharacter LineStatus = "wrong_character"
)

// TestTimerInfo describes the spell-timer a matched trigger would start (or,
// for a worn-off match, the timer key it would stop).
type TestTimerInfo struct {
	Key      string  `json:"key"`
	Category string  `json:"category,omitempty"`
	Duration float64 `json:"duration_secs,omitempty"`
	Target   string  `json:"target,omitempty"`
}

// TestWebhook previews a discord_webhook action's outcome. A test run never
// actually posts — Resolved just reports whether the referenced webhook ID
// still exists in Settings, the same check applyFire's dispatchWebhooks does
// at fire time.
type TestWebhook struct {
	WebhookID string `json:"webhook_id"`
	Text      string `json:"text"`
	Resolved  bool   `json:"resolved"`
}

// TestMatch is one trigger's outcome against one test line. Only triggers
// whose primary pattern, an extra pattern, or the worn-off pattern actually
// matched the line are reported — a trigger that never matched is omitted
// entirely, the same as a real log line silently passing it by.
type TestMatch struct {
	TriggerID      string            `json:"trigger_id"`
	TriggerName    string            `json:"trigger_name"`
	Status         LineStatus        `json:"status"`
	PatternLabel   string            `json:"pattern_label,omitempty"` // "primary" or "extra N"
	ExcludePattern string            `json:"exclude_pattern,omitempty"`
	Captures       map[string]string `json:"captures,omitempty"`
	Actions        []Action          `json:"actions,omitempty"`
	Timer          *TestTimerInfo    `json:"timer,omitempty"`
	Webhooks       []TestWebhook     `json:"webhooks,omitempty"`
	// WornOff is true when this entry is the trigger's worn-off pattern
	// matching (stopping a timer), not its primary/extra pattern.
	WornOff bool `json:"worn_off,omitempty"`
	// Fired is true when fire effects were enabled and this match actually
	// ran applyFire (or, for a worn-off match, called StopExternal) — i.e.
	// the real overlay/audio/timer were triggered, not just reported.
	Fired bool `json:"fired"`
}

// TestLineResult is the tester's report for one pasted line.
type TestLineResult struct {
	Line string `json:"line"`
	// Timestamp is the line's own parsed EQ log timestamp, when it had a
	// recognizable "[Mon Jan _2 15:04:05 2006]" prefix. A bare line with no
	// timestamp omits this and inherits the previous line's time (or now,
	// for the first line) purely for cooldown/pacing purposes.
	Timestamp *time.Time  `json:"timestamp,omitempty"`
	Matches   []TestMatch `json:"matches"`
}

// TestRequest configures a tester run.
type TestRequest struct {
	Lines string `json:"lines"`
	// Character overrides the live active character for {c}/{char}/{self}
	// substitution and per-character trigger filtering. Empty = whichever
	// character is actually logged in right now.
	Character string `json:"character,omitempty"`
	// FireEffects also runs the real timer/overlay/audio side of a match
	// (via applyFire/StopExternal), so the user can watch/hear the alert.
	// Discord webhook posts and trigger history are never touched by a test
	// run regardless of this flag.
	FireEffects bool `json:"fire_effects,omitempty"`
	// Trigger, when set, tests only this (possibly unsaved) trigger instead
	// of every enabled trigger — used by the trigger editor's inline
	// "test against sample line" box.
	Trigger *Trigger `json:"trigger,omitempty"`
}

// TestReport is the result of an instant (non-realtime) test run.
type TestReport struct {
	Lines []TestLineResult `json:"lines"`
	// Errors surfaces problems that stopped the whole run from testing
	// anything meaningful — e.g. the draft trigger's pattern doesn't
	// compile, or a pipe-source trigger was submitted (pasted log lines
	// can't drive a pipe condition).
	Errors    []string `json:"errors,omitempty"`
	Matched   int      `json:"matched"`
	Fired     int      `json:"fired"`
	Excluded  int      `json:"excluded"`
	Cooldowns int      `json:"cooldowns"`
}

// compiledForTest builds the set of compiled triggers a test run matches
// against: every enabled log-source trigger (mirroring Reload, but against
// whatever character the test specifies), or — when draft is non-nil — just
// that one trigger, compiled fresh so an unsaved edit in the trigger editor
// can be tested before Save. Pattern-compile errors are always returned;
// the caller decides whether to surface them (the single-trigger draft path
// cares, since a typo there means nothing else in the response is useful).
func (e *Engine) compiledForTest(character string, draft *Trigger) ([]compiled, []string) {
	if draft != nil {
		if draft.Source == SourcePipe {
			return nil, []string{"pipe-source triggers can't be tested against pasted log lines"}
		}
		c, errs := compileOne(draft, character)
		if c.re == nil {
			return nil, errs
		}
		return []compiled{c}, errs
	}

	triggers, err := e.store.List()
	if err != nil {
		return nil, []string{fmt.Sprintf("load triggers: %v", err)}
	}
	var cs []compiled
	for _, t := range triggers {
		if !t.Enabled || t.Source == SourcePipe {
			continue
		}
		c, _ := compileOne(t, character) // compile warnings already logged by Reload
		if c.re == nil {
			continue
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// matchingExclude returns the first exclude pattern in res that matches s,
// or nil. Like matchesAny but keeps the matched regex for reporting.
func matchingExclude(res []*regexp.Regexp, s string) *regexp.Regexp {
	for _, re := range res {
		if re.MatchString(s) {
			return re
		}
	}
	return nil
}

// captureMap flattens a regex match into a JSON-friendly map: numbered
// groups ("0" = the whole match, "1", "2", …) plus any named groups, mirroring
// what substituteCaptures resolves for action text. Returns nil for a
// pattern with no capture groups at all (match has exactly one element).
func captureMap(match, names []string) map[string]string {
	if len(match) <= 1 {
		return nil
	}
	out := make(map[string]string, len(match)*2)
	for i, v := range match {
		out[strconv.Itoa(i)] = v
		if i < len(names) && names[i] != "" {
			out[names[i]] = v
		}
	}
	return out
}

// previewWebhooks reports, without posting, which discord_webhook actions
// (already capture-substituted) a fire would dispatch and whether each
// referenced webhook ID still resolves to a URL in Settings.
func (e *Engine) previewWebhooks(actions []Action) []TestWebhook {
	var out []TestWebhook
	for _, a := range actions {
		if a.Type != ActionDiscordWebhook || a.WebhookID == "" {
			continue
		}
		resolved := false
		if e.resolveWebhook != nil {
			if url, ok := e.resolveWebhook(a.WebhookID); ok && url != "" {
				resolved = true
			}
		}
		out = append(out, TestWebhook{WebhookID: a.WebhookID, Text: a.Text, Resolved: resolved})
	}
	return out
}

// testLine matches message against cs (the tester's compiled trigger set),
// returning one TestMatch per trigger whose primary pattern, an extra
// pattern, or its worn-off pattern matched — mirroring Engine.Handle's own
// matching loop so the tester reports exactly what live log parsing would
// do. fc is the test run's own fireContext (never the live engine's), so
// refire-cooldown and boss-cast state stay isolated to this run.
//
// fireEffects gates only the side effects (applyFire / sink.StopExternal);
// the match itself, its status, and every reported field are always
// computed regardless.
func (e *Engine) testLine(fc *fireContext, cs []compiled, character string, ts time.Time, message string, fireEffects bool) []TestMatch {
	var out []TestMatch
	for _, c := range cs {
		m, names := c.re.FindStringSubmatch(message), c.re.SubexpNames()
		label := "primary"
		var extra *ExtraPattern
		for i := 0; m == nil && i < len(c.extras); i++ {
			m, names = c.extras[i].re.FindStringSubmatch(message), c.extras[i].re.SubexpNames()
			if m != nil {
				extra = &c.extras[i].meta
				label = fmt.Sprintf("extra %d", i+1)
			}
		}
		applies := triggerAppliesTo(c.trigger, character)

		if m != nil {
			tm := TestMatch{
				TriggerID:    c.trigger.ID,
				TriggerName:  c.trigger.Name,
				PatternLabel: label,
				Captures:     captureMap(m, names),
			}
			switch {
			case !applies:
				tm.Status = LineStatusWrongCharacter
			case matchingExclude(c.excludes, message) != nil:
				tm.Status = LineStatusExcluded
				tm.ExcludePattern = matchingExclude(c.excludes, message).String()
			case !fc.passesRefireCooldown(c.trigger, ts):
				tm.Status = LineStatusCooldown
			default:
				tm.Status = LineStatusMatched
				plan := e.planFire(fc, c, message, ts, m, names, extra, character)
				tm.Actions = plan.actions
				tm.Webhooks = e.previewWebhooks(plan.actions)
				if plan.hasTimer {
					tm.Timer = &TestTimerInfo{Key: plan.timerKey, Category: plan.timerCategory, Duration: plan.timerDuration, Target: plan.timerTarget}
				}
				if fireEffects {
					e.applyFire(plan, fireOpts{test: true})
					tm.Fired = true
				}
			}
			out = append(out, tm)
		}

		if c.wornOff != nil && c.timerKey != "" {
			if wm := c.wornOff.FindStringSubmatch(message); wm != nil {
				key := resolveTimerKey(c.trigger, c.timerKey, wm, c.wornOff.SubexpNames())
				spellID := c.trigger.SpellID
				if key != c.timerKey {
					spellID = 0
				}
				wtm := TestMatch{
					TriggerID:   c.trigger.ID,
					TriggerName: c.trigger.Name,
					WornOff:     true,
					Timer:       &TestTimerInfo{Key: key, Category: timerCategory(c.trigger.TimerType)},
				}
				if !applies {
					wtm.Status = LineStatusWrongCharacter
				} else {
					wtm.Status = LineStatusMatched
					if fireEffects && e.sink != nil {
						e.sink.StopExternal(key, spellID)
						wtm.Fired = true
					}
				}
				out = append(out, wtm)
			}
		}
	}
	return out
}

// splitTestLines breaks a pasted blob into individual lines, dropping a
// single trailing newline (so pasting from a text editor that appends one
// doesn't produce a spurious empty final line) and capping the total at
// maxTestLines.
func splitTestLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxTestLines {
		lines = lines[:maxTestLines]
	}
	return lines
}

// RunTest runs req.Lines through the trigger engine synchronously and
// returns the full report in one shot — the path both the Tester tab's
// default (non-realtime) Run and the trigger editor's inline sample-line
// test use.
//
// Each line's own EQ log timestamp (when present) drives refire-cooldown
// and boss-cast-caster tracking, so a pasted fight can validate cooldown
// timing exactly as it would replay live; a line with no timestamp inherits
// the previous line's (or time.Now() for the first line). The timestamp
// forwarded to the sink/broadcast when fire effects are on is always
// time.Now() at dispatch (planFire receives ts as firedAt directly — see
// applyFire), matching the log Replayer's own remap: an old, pasted
// timestamp would otherwise make a just-started timer's ExpiresAt already in
// the past.
func (e *Engine) RunTest(req TestRequest) TestReport {
	character := req.Character
	if character == "" && e.activeChar != nil {
		character = e.activeChar()
	}
	cs, errs := e.compiledForTest(character, req.Trigger)
	report := TestReport{Errors: errs}
	if len(cs) == 0 {
		return report
	}

	fc := newFireContext()
	prevTS := time.Now()
	for _, raw := range splitTestLines(req.Lines) {
		ts, msg, ok := logparser.ParseRawLine(raw)
		if !ok {
			ts, msg = prevTS, raw
		}
		prevTS = ts

		lr := TestLineResult{Line: raw}
		if ok {
			t := ts
			lr.Timestamp = &t
		}
		lr.Matches = e.testLine(fc, cs, character, ts, msg, req.FireEffects)
		for _, m := range lr.Matches {
			switch m.Status {
			case LineStatusMatched:
				report.Matched++
				if m.Fired {
					report.Fired++
				}
			case LineStatusExcluded:
				report.Excluded++
			case LineStatusCooldown:
				report.Cooldowns++
			}
		}
		report.Lines = append(report.Lines, lr)
	}
	return report
}

// ── real-time playback ──────────────────────────────────────────────────────

// TestPlaybackState enumerates a Tester session's lifecycle.
type TestPlaybackState string

const (
	TestPlaybackIdle    TestPlaybackState = "idle"
	TestPlaybackPlaying TestPlaybackState = "playing"
)

// testMaxGap caps the real-time wait between two consecutive test lines,
// same reasoning as the log Replayer's replayMaxGap: a multi-minute gap
// between two pasted lines shouldn't stall the session for that long.
const testMaxGap = 3 * time.Second

// testPollStep is how often the playback goroutine re-checks for Stop while
// sleeping between lines.
const testPollStep = 100 * time.Millisecond

// ErrTestAlreadyActive is returned by Tester.Start when a real-time session
// is already playing.
var ErrTestAlreadyActive = errors.New("trigger tester: a session is already active")

// Tester runs a Trigger Tester session in real-time playback mode, pacing
// pasted lines by their own log-timestamp gaps instead of returning the
// whole report at once — lets the Tester tab's "Real-time" checkbox show
// alerts landing at the same cadence they would during the actual fight.
// One session at a time. Instant-mode requests (Engine.RunTest) don't use
// this type at all.
type Tester struct {
	engine *Engine
	onLine func(TestLineResult)
	onDone func(errs []string)

	mu    sync.Mutex
	state TestPlaybackState
	stop  chan struct{}
}

// NewTester creates a Tester. onLine is called (from the playback goroutine,
// not the caller's) as each line is processed; onDone is called once when
// the session ends, for any reason, with any errors that prevented testing
// from starting at all (an invalid draft pattern, a pipe-source trigger).
// Both may be nil.
func NewTester(e *Engine, onLine func(TestLineResult), onDone func(errs []string)) *Tester {
	return &Tester{engine: e, onLine: onLine, onDone: onDone, state: TestPlaybackIdle}
}

// Status returns the session's current playback state.
func (rt *Tester) Status() TestPlaybackState {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.state
}

// Start begins a real-time playback session. Errors when a session is
// already active.
func (rt *Tester) Start(req TestRequest) error {
	rt.mu.Lock()
	if rt.state != TestPlaybackIdle {
		rt.mu.Unlock()
		return ErrTestAlreadyActive
	}
	stop := make(chan struct{})
	rt.state = TestPlaybackPlaying
	rt.stop = stop
	rt.mu.Unlock()

	go rt.run(req, stop)
	return nil
}

// Stop aborts the active session. No-op when idle.
func (rt *Tester) Stop() {
	rt.mu.Lock()
	stop := rt.stop
	active := rt.state == TestPlaybackPlaying
	if active {
		rt.stop = nil
	}
	rt.mu.Unlock()
	if active && stop != nil {
		close(stop)
	}
}

func (rt *Tester) run(req TestRequest, stop <-chan struct{}) {
	character := req.Character
	if character == "" && rt.engine.activeChar != nil {
		character = rt.engine.activeChar()
	}
	cs, errs := rt.engine.compiledForTest(character, req.Trigger)
	defer rt.finish(errs)
	if len(cs) == 0 {
		return
	}

	fc := newFireContext()
	var prevTS time.Time
	for _, raw := range splitTestLines(req.Lines) {
		select {
		case <-stop:
			return
		default:
		}

		ts, msg, ok := logparser.ParseRawLine(raw)
		if !ok {
			ts, msg = time.Now(), raw
		} else if !prevTS.IsZero() {
			wait := ts.Sub(prevTS)
			if wait > testMaxGap {
				wait = testMaxGap
			}
			if wait > 0 && !sleepInterruptible(wait, stop) {
				return
			}
		}
		if ok {
			prevTS = ts
		}

		lr := TestLineResult{Line: raw}
		if ok {
			t := ts
			lr.Timestamp = &t
		}
		lr.Matches = rt.engine.testLine(fc, cs, character, ts, msg, req.FireEffects)
		if rt.onLine != nil {
			rt.onLine(lr)
		}
	}
}

func (rt *Tester) finish(errs []string) {
	rt.mu.Lock()
	rt.state = TestPlaybackIdle
	rt.stop = nil
	rt.mu.Unlock()
	if rt.onDone != nil {
		rt.onDone(errs)
	}
}

// sleepInterruptible sleeps for d in small steps, returning false when the
// session is stopped mid-sleep. Mirrors the log Replayer's own helper of the
// same name (kept package-local rather than shared — the two packages
// intentionally don't import each other for this).
func sleepInterruptible(d time.Duration, stop <-chan struct{}) bool {
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return true
		}
		step := testPollStep
		if remaining < step {
			step = remaining
		}
		select {
		case <-stop:
			return false
		case <-time.After(step):
		}
	}
}

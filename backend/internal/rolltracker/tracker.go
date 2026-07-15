package rolltracker

import (
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// pendingTTL bounds how long an unmatched EventRollAnnounce stays valid
// while waiting for its EventRollResult partner. EQ always logs the two
// lines back-to-back with identical timestamps, so even a tiny window
// catches every legitimate pair; the cap exists only so a malformed log
// (announce with no result) doesn't poison the next real roll.
const pendingTTL = 2 * time.Second

// maxSessions caps the number of sessions kept in memory. Once exceeded,
// the oldest stopped session is dropped to keep the broadcast payload
// bounded during an extended raid. Active sessions are never evicted.
const maxSessions = 20

// staleAfter is the inactivity window after which a session is auto-stopped.
// Users typically stop sessions manually, but if they forget, we don't want
// rolls from a brand-new drop with the same Max to merge into an old one.
const staleAfter = 5 * time.Minute

// lootCallTTL bounds how far back the auto-suggest looks for the chat line
// that named the item. A leader posts "Robe of the Lost Circle 333" and the
// rolls follow within seconds; two minutes is comfortably wide while keeping
// stale calls from mislabeling an unrelated later roll on the same number.
const lootCallTTL = 2 * time.Minute

// maxLootCalls caps the recent-loot-call ring so the buffer can't grow
// unbounded during a long session of number-heavy chatter.
const maxLootCalls = 32

// reLootNumber pulls candidate roll bounds (3–4 digit numbers) out of a chat
// line. Loot is essentially always called with a 3–4 digit /random max;
// requiring that width skips the 1–2 digit counts common in raid chatter
// ("inc in 30", "coth 1"). The optional trailing letter accommodates
// tier shorthand glued directly onto the number with no space ("511p",
// "522u", "533a" for pick/upgrade/alt) — the same shorthand style as the
// "x11" suffix format below, just spelled the other direction. A second
// glued-on letter ("511pick", "333rd") fails the trailing \b and is left
// unmatched rather than guessed at.
var reLootNumber = regexp.MustCompile(`\b(\d{3,4})[A-Za-z]?\b`)

// reLootSuffixNumber matches a leading-digit-omitted loot call token like
// "x11" or "x22" — shorthand some guilds use to denote a roll tier bracket
// (the "1xx pick / 122 upgrade / 133 alt" suffix scheme) without committing
// to which item's group digit it belongs to. It's treated as a suffix
// wildcard: it matches any active session whose Max ends in those digits,
// regardless of the leading digit(s).
var reLootSuffixNumber = regexp.MustCompile(`(?i)\bx(\d{2,3})\b`)

// ItemMatcher resolves a chat line to the loot-item name it mentions,
// returning ok=false when nothing convincing matches. It is injected (rather
// than the tracker importing the game DB directly) so the package stays
// decoupled and the heuristic is trivially stubbable in tests.
type ItemMatcher func(line string) (name string, ok bool)

// lootCall is a recent chat line that mentioned one or more 3–4 digit
// numbers, or "x"-prefixed suffix shorthand (e.g. "x11") — a candidate
// "roll <n> for <item>" announcement.
type lootCall struct {
	ts       time.Time
	numbers  []int
	suffixes []lootSuffix
	text     string
}

// lootSuffix is a suffix-only loot number, as written in "x11"-style
// shorthand: the digits are known but the leading (group) digit isn't, so
// it matches any Max that ends in those digits.
type lootSuffix struct {
	value int // the suffix digits, parsed as an int (e.g. 11)
	width int // number of suffix digits (e.g. 2), so "x11" doesn't also match a Max ending in "011"
}

// matchesSuffix reports whether max ends in the given suffix's digits.
func (sfx lootSuffix) matchesSuffix(max int) bool {
	mod := 1
	for i := 0; i < sfx.width; i++ {
		mod *= 10
	}
	return max%mod == sfx.value
}

// Tracker maintains the live set of /random sessions inferred from the
// EQ log feed. It is safe for concurrent use.
type Tracker struct {
	hub *ws.Hub

	mu              sync.Mutex
	sessions        []*Session // newest-first
	rule            WinnerRule
	mode            Mode
	autoStopSeconds int
	profile         RollProfile            // grouping profile; zero value = simple
	autoStops       map[uint64]*time.Timer // session ID → pending auto-stop
	nextID          uint64

	// pendingRoller holds the name from the most recent EventRollAnnounce
	// while we wait for its matching EventRollResult.
	pendingRoller string
	pendingAt     time.Time

	// matchItem, when set, turns on best-effort loot-item auto-suggest:
	// recent chat lines naming a 3–4 digit number are buffered, and when a
	// session's Max matches one the line is run through matchItem to label
	// the session. nil disables the feature entirely.
	matchItem   ItemMatcher
	recentCalls []lootCall // chronological; newest last

	// onChange, if set, is invoked (outside the lock) whenever the winner
	// rule, mode, auto-stop window, or grouping profile changes, so the
	// caller can persist the new settings. It is never fired by Configure.
	onChange func(rule WinnerRule, mode Mode, autoStopSeconds int, profile RollProfile)
}

// New returns an initialised Tracker with the default WinnerHighest rule.
func New(hub *ws.Hub) *Tracker {
	return &Tracker{
		hub:             hub,
		rule:            WinnerHighest,
		mode:            ModeManual,
		autoStopSeconds: DefaultAutoStopSeconds,
		profile:         RollProfile{Mode: ProfileSimple},
		autoStops:       make(map[uint64]*time.Timer),
	}
}

// Configure seeds the tracker's settings from persisted preferences. Invalid
// or zero values fall back to the built-in defaults. It does not fire
// onChange — it's meant to be called once at startup before any events flow.
func (t *Tracker) Configure(rule WinnerRule, mode Mode, autoStopSeconds int, profile RollProfile) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rule == WinnerHighest || rule == WinnerLowest {
		t.rule = rule
	}
	if mode == ModeManual || mode == ModeTimer {
		t.mode = mode
	}
	if autoStopSeconds >= 5 && autoStopSeconds <= 600 {
		t.autoStopSeconds = autoStopSeconds
	}
	if p, err := profile.Validate(); err == nil {
		t.profile = p
	}
}

// SetOnChange registers a callback fired whenever the winner rule, mode,
// auto-stop window, or grouping profile changes, so settings can be
// persisted.
func (t *Tracker) SetOnChange(fn func(rule WinnerRule, mode Mode, autoStopSeconds int, profile RollProfile)) {
	t.mu.Lock()
	t.onChange = fn
	t.mu.Unlock()
}

// SetProfile swaps the grouping profile. The profile must already be
// validated by the caller (the API handler normalizes it). Affects only how
// the UI groups sessions; the session engine is unchanged.
func (t *Tracker) SetProfile(profile RollProfile) {
	t.mu.Lock()
	t.profile = profile
	t.notifyChangeLocked()
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// SetItemMatcher enables best-effort loot-item auto-suggest by registering
// the resolver used to turn a chat line into an item name. Passing nil
// disables auto-suggest. Safe to call once at startup before events flow.
func (t *Tracker) SetItemMatcher(fn ItemMatcher) {
	t.mu.Lock()
	t.matchItem = fn
	t.mu.Unlock()
}

// HandleLine feeds a raw (post-timestamp) log line to the auto-suggest
// heuristic. It buffers chat lines that mention a 3–4 digit number as
// candidate loot calls and, when the number matches an active unlabeled
// session, labels it with the item the line names. A no-op unless an
// ItemMatcher is registered. Wire it to the same raw-line dispatch as the
// other line consumers.
func (t *Tracker) HandleLine(ts time.Time, msg string) {
	t.mu.Lock()
	matcher := t.matchItem
	t.mu.Unlock()
	if matcher == nil {
		return
	}
	// Cheap pre-filter before the ~16-regex chat parse (which the chat consumer
	// already ran on this same line): a loot call must mention a 3-4 digit roll
	// number or an "x11"-style suffix shorthand, so a line without either can't
	// be one. Reuses the regexes on the raw line — a necessary condition,
	// since parsing only strips the speaker prefix and never introduces one.
	if !reLootNumber.MatchString(msg) && !reLootSuffixNumber.MatchString(msg) {
		return
	}
	// Only genuine player chat — never combat spam, where "for 333 points
	// of damage" would masquerade as a roll call.
	parsed, ok := chat.ParseChat(msg)
	if !ok {
		return
	}
	nums := extractRollNumbers(parsed.Message)
	suffixes := extractLootSuffixes(parsed.Message)
	if len(nums) == 0 && len(suffixes) == 0 {
		return
	}

	t.mu.Lock()
	t.recentCalls = append(t.recentCalls, lootCall{ts: ts, numbers: nums, suffixes: suffixes, text: parsed.Message})
	t.pruneCallsLocked(ts)
	// Does this line name a number (exact or suffix-shorthand) belonging to
	// an active, still-unlabeled session? If so, claim the attempt now
	// (under the lock) and resolve the name outside it.
	var targets []uint64
	for _, s := range t.sessions {
		if s.Active && s.ItemName == "" && !s.autoSuggested && callMatchesMax(nums, suffixes, s.Max) {
			s.autoSuggested = true
			targets = append(targets, s.ID)
		}
	}
	t.mu.Unlock()
	if len(targets) == 0 {
		return
	}

	name, ok := matcher(parsed.Message)
	if !ok {
		return
	}
	for _, id := range targets {
		t.applyAutoSuggestion(id, name)
	}
}

// recentCallForLocked returns the text of the most recent buffered loot call
// (within lootCallTTL of now) that mentioned max, or ok=false. mu must be
// held.
func (t *Tracker) recentCallForLocked(max int, now time.Time) (string, bool) {
	for i := len(t.recentCalls) - 1; i >= 0; i-- {
		c := t.recentCalls[i]
		if now.Sub(c.ts) > lootCallTTL {
			break // older entries only; buffer is chronological
		}
		if callMatchesMax(c.numbers, c.suffixes, max) {
			return c.text, true
		}
	}
	return "", false
}

// pruneCallsLocked drops loot calls older than lootCallTTL and trims the
// buffer to maxLootCalls. mu must be held.
func (t *Tracker) pruneCallsLocked(now time.Time) {
	cut := 0
	for cut < len(t.recentCalls) && now.Sub(t.recentCalls[cut].ts) > lootCallTTL {
		cut++
	}
	if cut > 0 {
		t.recentCalls = t.recentCalls[cut:]
	}
	if len(t.recentCalls) > maxLootCalls {
		t.recentCalls = t.recentCalls[len(t.recentCalls)-maxLootCalls:]
	}
}

// applyAutoSuggestion sets a session's item name from the heuristic, but only
// while it's still blank — so a label the user typed in the brief window
// since the attempt was claimed always wins.
func (t *Tracker) applyAutoSuggestion(id uint64, name string) {
	t.mu.Lock()
	var changed bool
	for _, s := range t.sessions {
		if s.ID == id && s.ItemName == "" {
			s.ItemName = name
			changed = true
			break
		}
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if changed {
		t.broadcast(state)
	}
}

// extractRollNumbers returns the distinct 3–4 digit numbers in a chat line,
// ignoring any glued-on tier-letter suffix ("511p" → 511).
func extractRollNumbers(s string) []int {
	matches := reLootNumber.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil || containsInt(out, n) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// extractLootSuffixes returns the distinct "x11"-style suffix tokens in a
// chat line.
func extractLootSuffixes(s string) []lootSuffix {
	matches := reLootSuffixNumber.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]lootSuffix, 0, len(matches))
	for _, m := range matches {
		digits := m[1]
		n, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		sfx := lootSuffix{value: n, width: len(digits)}
		if !containsSuffix(out, sfx) {
			out = append(out, sfx)
		}
	}
	return out
}

// callMatchesMax reports whether a loot call's extracted numbers or
// suffix-shorthand tokens identify the given session Max.
func callMatchesMax(nums []int, suffixes []lootSuffix, max int) bool {
	if containsInt(nums, max) {
		return true
	}
	for _, sfx := range suffixes {
		if sfx.matchesSuffix(max) {
			return true
		}
	}
	return false
}

func containsInt(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func containsSuffix(xs []lootSuffix, want lootSuffix) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// notifyChangeLocked snapshots the current settings and schedules the
// onChange callback to run after the lock is released. mu must be held.
func (t *Tracker) notifyChangeLocked() {
	if t.onChange == nil {
		return
	}
	rule, mode, secs, profile := t.rule, t.mode, t.autoStopSeconds, t.profile
	fn := t.onChange
	go fn(rule, mode, secs, profile)
}

// Handle routes a parsed log event into the tracker. Only roll events do
// anything; the rest are ignored so callers can subscribe Tracker.Handle
// to the same dispatch as every other log consumer.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventRollAnnounce:
		data, ok := ev.Data.(logparser.RollAnnounceData)
		if !ok {
			return
		}
		t.recordAnnounce(data.Roller, ev.Timestamp)
	case logparser.EventRollResult:
		data, ok := ev.Data.(logparser.RollResultData)
		if !ok {
			return
		}
		t.recordResult(data.Min, data.Max, data.Value, ev.Timestamp)
	}
}

func (t *Tracker) recordAnnounce(roller string, ts time.Time) {
	t.mu.Lock()
	t.pendingRoller = roller
	t.pendingAt = ts
	t.mu.Unlock()
}

func (t *Tracker) recordResult(min, max, value int, ts time.Time) {
	t.mu.Lock()
	roller := t.pendingRoller
	pendingAt := t.pendingAt
	t.pendingRoller = ""
	t.pendingAt = time.Time{}
	// Drop an orphan result whose announce is too old — guards against
	// a torn pair where the announce line was lost or already consumed.
	if roller == "" || ts.Sub(pendingAt) > pendingTTL {
		t.mu.Unlock()
		return
	}

	sess := t.sessionForLocked(min, max, ts)
	dup := false
	for i := range sess.Rolls {
		if sess.Rolls[i].Roller == roller {
			dup = true
			break
		}
	}
	wasFirst := len(sess.Rolls) == 0
	sess.Rolls = append(sess.Rolls, Roll{
		Roller:    roller,
		Value:     value,
		Timestamp: ts,
		Duplicate: dup,
	})
	sess.LastRollAt = ts

	// On the session's first roll, see whether a recent chat line already
	// named the item (the leader's call usually precedes the rolls). Mark
	// the session attempted now, but run the (DB-backed) matcher after the
	// lock is released so we never hold mu across I/O.
	var suggestID uint64
	var suggestText string
	if wasFirst && sess.ItemName == "" && !sess.autoSuggested && t.matchItem != nil {
		if text, ok := t.recentCallForLocked(max, ts); ok {
			sess.autoSuggested = true
			suggestID = sess.ID
			suggestText = text
		}
	}

	matcher := t.matchItem
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)

	if suggestID != 0 && matcher != nil {
		if name, ok := matcher(suggestText); ok {
			t.applyAutoSuggestion(suggestID, name)
		}
	}
}

// sessionForLocked returns the active session for the min–max range,
// opening a new one if there isn't one or the existing one has gone
// stale. Sessions key on both bounds so a "/random 222 611" roll lands
// in its own 222–611 session instead of the 0–611 one. mu must be held.
func (t *Tracker) sessionForLocked(min, max int, ts time.Time) *Session {
	// Sweep stale actives to inactive so they become evictable. In manual mode
	// nothing else ever clears Active (only timer mode arms an auto-stop), so
	// without this every lull would spawn a fresh permanently-active session:
	// maxSessions never bites, and the per-roll stateLocked() deep-copy grows
	// linearly all night. A session past staleAfter can already never accept
	// another roll, so flipping it inactive changes no behavior.
	for _, s := range t.sessions {
		if s.Active && ts.Sub(s.LastRollAt) > staleAfter {
			s.Active = false
			s.AutoStopAt = time.Time{}
			t.cancelAutoStopLocked(s.ID)
		}
	}
	t.evictOldestStoppedLocked()
	for _, s := range t.sessions {
		if s.Min == min && s.Max == max && s.Active && ts.Sub(s.LastRollAt) <= staleAfter {
			return s
		}
	}
	t.nextID++
	s := &Session{
		ID:         t.nextID,
		Min:        min,
		Max:        max,
		StartedAt:  ts,
		LastRollAt: ts,
		Active:     true,
	}
	if t.mode == ModeTimer && t.autoStopSeconds > 0 {
		dur := time.Duration(t.autoStopSeconds) * time.Second
		// AutoStopAt is anchored to wall-clock time.Now() rather than the
		// log timestamp ts: the user sees the countdown ticking against
		// the clock on their wall, so a log timestamp pulled from
		// minutes-old backlog would otherwise show as "expired" the
		// instant it appeared. time.AfterFunc is also wall-clock by
		// nature, so both halves agree.
		s.AutoStopAt = time.Now().Add(dur)
		id := s.ID
		t.autoStops[id] = time.AfterFunc(dur, func() { t.fireAutoStop(id) })
	}
	// Prepend so newest sessions appear first in the broadcast payload.
	t.sessions = append([]*Session{s}, t.sessions...)
	t.evictOldestStoppedLocked()
	return s
}

// fireAutoStop is the callback time.AfterFunc invokes when a timer-mode
// session's window expires. It mirrors Stop without requiring the caller
// to know whether the session still exists or has been manually stopped
// in the interim.
func (t *Tracker) fireAutoStop(id uint64) {
	t.mu.Lock()
	delete(t.autoStops, id)
	var found bool
	for _, s := range t.sessions {
		if s.ID == id && s.Active {
			s.Active = false
			s.AutoStopAt = time.Time{}
			found = true
			break
		}
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
}

// cancelAutoStopLocked stops a pending auto-stop timer for the given
// session ID if one is registered. mu must be held.
func (t *Tracker) cancelAutoStopLocked(id uint64) {
	if timer, ok := t.autoStops[id]; ok {
		timer.Stop()
		delete(t.autoStops, id)
	}
}

// evictOldestStoppedLocked drops the oldest stopped session if we're over
// maxSessions. mu must be held.
func (t *Tracker) evictOldestStoppedLocked() {
	if len(t.sessions) <= maxSessions {
		return
	}
	for i := len(t.sessions) - 1; i >= 0; i-- {
		if !t.sessions[i].Active {
			t.sessions = append(t.sessions[:i], t.sessions[i+1:]...)
			return
		}
	}
}

// Stop marks the session with the given ID inactive. Returns true if a
// matching active session was found.
func (t *Tracker) Stop(id uint64) bool {
	t.mu.Lock()
	var found bool
	for _, s := range t.sessions {
		if s.ID == id && s.Active {
			s.Active = false
			s.AutoStopAt = time.Time{}
			found = true
			break
		}
	}
	if found {
		t.cancelAutoStopLocked(id)
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
	return found
}

// Remove deletes the session with the given ID outright. Returns true if a
// matching session was found.
func (t *Tracker) Remove(id uint64) bool {
	t.mu.Lock()
	var found bool
	for i, s := range t.sessions {
		if s.ID == id {
			t.sessions = append(t.sessions[:i], t.sessions[i+1:]...)
			found = true
			break
		}
	}
	if found {
		t.cancelAutoStopLocked(id)
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
	return found
}

// SetItemName labels the session with the given ID with a loot item
// name (trimmed). An empty name clears the label. Returns true if a
// matching session was found.
func (t *Tracker) SetItemName(id uint64, name string) bool {
	t.mu.Lock()
	var found bool
	for _, s := range t.sessions {
		if s.ID == id {
			s.ItemName = name
			found = true
			break
		}
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
	return found
}

// Clear removes every session.
func (t *Tracker) Clear() {
	t.mu.Lock()
	t.sessions = nil
	for id, timer := range t.autoStops {
		timer.Stop()
		delete(t.autoStops, id)
	}
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// SetWinnerRule swaps the global winner-selection rule.
func (t *Tracker) SetWinnerRule(rule WinnerRule) {
	if rule != WinnerHighest && rule != WinnerLowest {
		return
	}
	t.mu.Lock()
	if t.rule == rule {
		t.mu.Unlock()
		return
	}
	t.rule = rule
	t.notifyChangeLocked()
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// SetMode swaps the session-closing mode. Switching modes affects only
// future sessions — currently-active sessions keep their existing
// behavior (a Live session with a timer stays scheduled; a Live manual
// session does not gain a timer). Callers can also pass autoStopSeconds
// to update the timer-mode window in the same call. Pass 0 to leave the
// existing value untouched.
func (t *Tracker) SetMode(mode Mode, autoStopSeconds int) {
	if mode != ModeManual && mode != ModeTimer {
		return
	}
	t.mu.Lock()
	changed := false
	if t.mode != mode {
		t.mode = mode
		changed = true
	}
	if autoStopSeconds > 0 && t.autoStopSeconds != autoStopSeconds {
		t.autoStopSeconds = autoStopSeconds
		changed = true
	}
	if !changed {
		t.mu.Unlock()
		return
	}
	t.notifyChangeLocked()
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// State returns a snapshot of the current tracker state, safe to marshal.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateLocked()
}

func (t *Tracker) stateLocked() State {
	out := State{
		WinnerRule:      t.rule,
		Mode:            t.mode,
		AutoStopSeconds: t.autoStopSeconds,
		Profile:         t.profile,
		Sessions:        make([]Session, 0, len(t.sessions)),
	}
	for _, s := range t.sessions {
		rolls := make([]Roll, len(s.Rolls))
		copy(rolls, s.Rolls)
		out.Sessions = append(out.Sessions, Session{
			ID:         s.ID,
			Min:        s.Min,
			Max:        s.Max,
			ItemName:   s.ItemName,
			StartedAt:  s.StartedAt,
			LastRollAt: s.LastRollAt,
			Active:     s.Active,
			AutoStopAt: s.AutoStopAt,
			Rolls:      rolls,
		})
	}
	return out
}

func (t *Tracker) broadcast(state State) {
	if t.hub == nil {
		return
	}
	t.hub.Broadcast(ws.Event{Type: WSEventRolls, Data: state})
}

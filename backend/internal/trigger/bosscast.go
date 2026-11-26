package trigger

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// reNPCBeginsCast matches the generic bystander cast-start line EQ logs for
// any nearby NPC (or other player) casting a spell: "<Name> begins to cast a
// spell." — no spell name, just the caster. This is deliberately broader
// than chchain's reBeginCastOther (single capitalized word, sized for player
// names): raid boss names are multi-word and carry apostrophes/backticks
// ("Aten Ha Ra", "Vyzh`dra the Cursed") and some share NPCs use a
// lowercase-article name ("a warder of Xuzl"), and titled ones a comma
// ("Ralthazor, Champion of Marr", "Falto, Lord of Thunder").
var reNPCBeginsCast = regexp.MustCompile("^([A-Za-z][A-Za-z`', ]*?) begins to cast a spell\\.$")

// bossCastWindow bounds how long a "begins to cast" observation stays
// eligible to bind a later land-text timer to that caster. Generous
// relative to these spells' actual cast times (a few seconds, plus PBAE
// broadcast jitter across several near-simultaneous land lines) while
// staying well under the shortest boss recast these triggers track, so a
// stale observation — an interrupted cast, or one whose land line never
// reached the log — expires instead of binding the boss's next cast.
const bossCastWindow = 15 * time.Second

// bossDuplicateWindow is the default per-caster refire lockout for a
// boss-caster trigger with no RefireCooldownSecs of its own. A single PBAE
// cast logs one land line per raid member hit, all within the same second or
// two; this collapses them into one fire.
const bossDuplicateWindow = 3 * time.Second

// maxBossObservations caps the observation buffer. Observations older than
// bossCastWindow are pruned anyway; the cap only guards against a log replay
// whose timestamps jump backwards and defeat age-based pruning.
const maxBossObservations = 256

// bossCastTracker watches log lines for known raid-boss cast-start events so
// a boss-recast timer — whose land text never names the caster, only the
// victim — can bind to the actual casting NPC instead of guessing from
// whatever the player currently has targeted. That guess breaks the moment
// the player is off-tanking an add or has nothing targeted; this doesn't
// depend on target state at all.
//
// Cast-start lines name the boss but not the spell, so observations are
// shared by every trigger that lists that boss, but each trigger consumes an
// observation at most once (see bind). A boss with two tracked spells can
// therefore feed both triggers, and casting one spell never uses up the
// evidence the other spell's trigger needs.
//
// Attribution is best effort: the log can't say which of two overlapping
// casts produced a given land line. See bind for the exact rules.
type bossCastTracker struct {
	mu    sync.Mutex
	known map[string]bool // lowercased boss names worth recording
	obs   []castObservation
	last  map[string]bossBinding // trigger ID -> its most recent fire
}

// castObservation is one "<boss> begins to cast a spell." line.
type castObservation struct {
	name   string          // lowercased boss name
	at     time.Time       // log timestamp
	usedBy map[string]bool // trigger IDs that already consumed it
}

// bossBinding records when a boss-caster trigger last fired and which caster
// it bound to ("" when it fell back to the live target).
type bossBinding struct {
	caster string
	at     time.Time
}

func newBossCastTracker() *bossCastTracker {
	return &bossCastTracker{last: make(map[string]bossBinding)}
}

// setKnown rebuilds the set of boss names worth recording from the given
// triggers' BossCasters. Called on every engine reload, so a pack install,
// edit or removal takes effect without a restart.
func (b *bossCastTracker) setKnown(triggers []*Trigger) {
	known := make(map[string]bool)
	for _, t := range triggers {
		for _, c := range t.BossCasters {
			if n := strings.ToLower(strings.TrimSpace(c.Name)); n != "" {
				known[n] = true
			}
		}
	}
	b.mu.Lock()
	b.known = known
	b.mu.Unlock()
}

// observe records a cast-start line if its caster is a known boss; every
// other line (the vast majority in a busy raid — player casts, trash,
// everything else) is a cheap no-op after the name lookup misses.
func (b *bossCastTracker) observe(now time.Time, line string) {
	if !strings.HasSuffix(line, " begins to cast a spell.") {
		return
	}
	m := reNPCBeginsCast.FindStringSubmatch(line)
	if m == nil {
		return
	}
	name := strings.ToLower(m[1])
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.known[name] {
		return
	}
	b.pruneLocked(now)
	b.obs = append(b.obs, castObservation{name: name, at: now})
	if len(b.obs) > maxBossObservations {
		b.obs = b.obs[len(b.obs)-maxBossObservations:]
	}
}

// pruneLocked drops observations older than bossCastWindow relative to now.
// Observations are appended in log order, so the expired ones are a prefix.
func (b *bossCastTracker) pruneLocked(now time.Time) {
	i := 0
	for i < len(b.obs) && now.Sub(b.obs[i].at) > bossCastWindow {
		i++
	}
	if i > 0 {
		b.obs = append(b.obs[:0], b.obs[i:]...)
	}
}

// bind resolves which of t's BossCasters cast the spell whose land line just
// matched at now, and reports whether the trigger should fire at all. The
// returned caster is always one of t.BossCasters (canonical spelling) when
// fire is true.
//
// Rules, in priority order (timestamps only resolve to the second, so rule 1
// deliberately beats duplicate detection):
//
//  1. An observation of one of t's casters, within bossCastWindow and not yet
//     consumed by t, binds: the live target if it is one of those, otherwise
//     the oldest (the same spell has the same cast time, so casts finish in
//     the order they started). The observation is then consumed by t.
//  2. Otherwise, a line within the duplicate window of t's previous fire is
//     an AE broadcast duplicate: fire=false.
//  3. Otherwise, fall back to the live target, but only when it is one of
//     t's casters (returned as that caster). Any other target means the
//     spell came from an NPC this trigger doesn't track — typically the
//     same spell in another raid pack's zone — so fire=false. A player
//     who filters other people's spell casts (no cast-start lines) still
//     gets timers for whichever listed boss they have targeted.
//
// Worked case: A and B begin casting in the same second, then A's land line
// and its AE duplicate, then B's land line and duplicate. A's first line
// binds A, A's duplicate binds B (B's countdown starts a second or two
// early), and both of B's lines are then duplicates — one timer per boss.
func (b *bossCastTracker) bind(t *Trigger, now time.Time, liveTarget string) (caster string, fire bool) {
	canon := make(map[string]string, len(t.BossCasters))
	for _, c := range t.BossCasters {
		canon[strings.ToLower(c.Name)] = c.Name
	}
	live := strings.ToLower(liveTarget)

	b.mu.Lock()
	defer b.mu.Unlock()

	pick := -1
	for i := range b.obs {
		o := &b.obs[i]
		if _, ok := canon[o.name]; !ok || o.usedBy[t.ID] {
			continue
		}
		if d := now.Sub(o.at); d < 0 || d > bossCastWindow {
			continue
		}
		if live != "" && o.name == live {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick >= 0 {
		o := &b.obs[pick]
		if o.usedBy == nil {
			o.usedBy = make(map[string]bool)
		}
		o.usedBy[t.ID] = true
		caster = canon[o.name]
		b.last[t.ID] = bossBinding{caster: caster, at: now}
		return caster, true
	}

	window := bossDuplicateWindow
	if t.RefireCooldownSecs > 0 {
		window = time.Duration(t.RefireCooldownSecs * float64(time.Second))
	}
	if prev, ok := b.last[t.ID]; ok {
		if d := now.Sub(prev.at); d >= 0 && d < window {
			return "", false
		}
	}
	if c, ok := canon[live]; ok {
		b.last[t.ID] = bossBinding{caster: c, at: now}
		return c, true
	}
	return "", false
}

// casterDuration returns the per-caster duration override for caster on t,
// or 0 when there is none.
func casterDuration(t *Trigger, caster string) float64 {
	for _, c := range t.BossCasters {
		if strings.EqualFold(c.Name, caster) {
			return c.DurationSecs
		}
	}
	return 0
}

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
// lowercase-article name ("a warder of Xuzl").
var reNPCBeginsCast = regexp.MustCompile("^([A-Za-z][A-Za-z`' ]*?) begins to cast a spell\\.$")

// bossCastWindow bounds how long a "begins to cast" observation stays
// eligible to bind a later land-text timer to that caster. Generous
// relative to these spells' actual cast times (a few seconds, plus PBAE
// broadcast jitter across several near-simultaneous land lines) while
// staying well under the shortest signature-spell recast time (24s, Caustic
// Mist) so a stale observation can never carry over into the boss's next
// cast cycle.
const bossCastWindow = 15 * time.Second

// signatureSpellCasters maps a raidSignatureSpellAlerts() trigger's Name to
// the raid NPCs actually known (via quarm.db npc_spells) to cast that
// spell, so bossCastTracker only has to watch for these specific names
// instead of every cast-start line in a raid. Keyed by trigger Name rather
// than SpellID because the Caustic Mist/Putrefy Flesh entry covers two
// different underlying spell IDs cast by two different bosses (see the
// trigger's own doc comment on the text collision) — Name is the one field
// that already disambiguates that case.
//
// Scoped to the Kunark Ring War trial bosses these triggers were written
// for (plus Zlandicar, whose Putrefy Flesh produces byte-identical land
// text to Caustic Mist). Other npc_types rows that happen to share these
// spell IDs via generic/reused spell lists (ward/guardian trash) are left
// out to keep the allowlist tied to actual named encounters; extend here if
// another raid target turns out to share one of these signatures.
//
// A renamed trigger (user edits the Name in the UI) silently loses this
// lookup and falls back to the live-combat-target guess — a safe
// degradation, not a hard failure.
var signatureSpellCasters = map[string][]string{
	"Fling": {
		"Aten Ha Ra",
		"Kaas Thox Xi Aten Ha Ra",
		"Kaas Thox Xi Ans Dyek",
		"Va Xi Aten Ha Ra",
	},
	"Silence of the Shadows": {
		"Aten Ha Ra",
		"Diabo Xi Xin Thall",
		"Thall Va Xakra",
	},
	"Torturing Winds": {
		"Lord Inquisitor Seru",
	},
	"Caustic Mist / Putrefy Flesh": {
		"Vyzh`dra the Cursed",
		"Zlandicar",
	},
	"Touch of Vinitras": {
		"Shei Vinitras",
		"Vyzh`dra the Banished",
		"Vyzh`dra the Exiled",
	},
}

// knownSignatureBosses is the lowercased union of every name in
// signatureSpellCasters, used to cheaply reject cast-start lines from
// unrelated NPCs/players before bothering to record them.
var knownSignatureBosses = func() map[string]bool {
	m := make(map[string]bool)
	for _, casters := range signatureSpellCasters {
		for _, c := range casters {
			m[strings.ToLower(c)] = true
		}
	}
	return m
}()

// bossCastTracker watches log lines for known raid-boss cast-start events so
// a signature-spell timer — whose land text never names the caster, only the
// victim — can bind to the actual casting NPC instead of guessing from
// whatever the player currently has targeted. That guess (see engine.go's
// combat-target fallback) breaks the moment the player is off-tanking an add
// or has nothing targeted; this doesn't depend on target state at all.
type bossCastTracker struct {
	mu   sync.Mutex
	last map[string]time.Time // lowercased boss name -> last observed cast-start time
}

func newBossCastTracker() *bossCastTracker {
	return &bossCastTracker{last: make(map[string]time.Time)}
}

// observe records a cast-start line if its caster is a known signature-spell
// boss; every other line (the vast majority in a busy raid — player casts,
// trash, everything else) is a cheap no-op after the name lookup misses.
func (b *bossCastTracker) observe(now time.Time, line string) {
	m := reNPCBeginsCast.FindStringSubmatch(line)
	if m == nil {
		return
	}
	name := strings.ToLower(m[1])
	if !knownSignatureBosses[name] {
		return
	}
	b.mu.Lock()
	b.last[name] = now
	b.mu.Unlock()
}

// resolveCaster returns whichever of candidates most recently began casting
// within bossCastWindow before now, in its canonical (properly-cased) form
// from candidates — or "" if none of them were observed casting recently
// (e.g. the boss started casting before this log file was open, or the
// trigger fired well outside any observed cast-start).
func (b *bossCastTracker) resolveCaster(now time.Time, candidates []string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var best string
	var bestTime time.Time
	for _, c := range candidates {
		ts, ok := b.last[strings.ToLower(c)]
		if !ok {
			continue
		}
		if d := now.Sub(ts); d < 0 || d > bossCastWindow {
			continue
		}
		if ts.After(bestTime) {
			bestTime, best = ts, c
		}
	}
	return best
}

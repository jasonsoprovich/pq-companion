package logparser

import (
	"regexp"
	"sort"
	"sync/atomic"
)

// activeCastIndex holds the index used by ParseLine to detect spell-landed
// events. It's a process-wide singleton (the spell DB is read-only and the
// index is built once at startup) installed via SetCastIndex. nil means the
// feature is disabled and ParseLine emits no EventSpellLanded events —
// existing behaviour for callers that don't wire the index in.
var activeCastIndex atomic.Pointer[CastIndex]

// SetCastIndex installs the spell-landed lookup index used by ParseLine.
// Call once at startup after loading cast messages from the database. Pass
// nil to disable. Safe to call concurrently with ParseLine.
func SetCastIndex(idx *CastIndex) {
	activeCastIndex.Store(idx)
}

// MatchKind describes which side of a spell's cast text matched a log line.
type MatchKind int

const (
	// MatchSelf means the line matched cast_on_you — the spell landed on the
	// player whose log we're reading. The target is implicit ("you") and is
	// resolved by the caller (typically the active character name).
	MatchSelf MatchKind = iota
	// MatchOther means the line matched cast_on_other — the spell landed on
	// someone else. The target name is captured from the line.
	MatchOther
)

// CastMessage is the input row to NewCastIndex — one entry per spell with
// non-empty cast text. Mirrors db.CastMessage so logparser can stay free of
// a direct database dependency (and the index is trivially testable).
type CastMessage struct {
	SpellID     int
	SpellName   string
	CastOnYou   string
	CastOnOther string
}

// CastMatch is the result of CastIndex.Match. Many spells share identical
// cast text (e.g. dozens of buffs say "You feel different.") — when the
// matched text is unambiguous, SpellID and SpellName are set; otherwise they
// are zero/empty and Candidates lists every spell that could have produced
// the line. The engine disambiguates ambiguous matches via the most-recently
// cast spell.
type CastMatch struct {
	Kind       MatchKind
	SpellID    int
	SpellName  string
	TargetName string // empty for MatchSelf
	Candidates []CastMessage
}

// nameClass is the regex character class used to capture a target name in a
// cast_on_other line. EQ player and pet names are single tokens starting
// with an uppercase letter; we allow apostrophes for the rare pet name.
// 4-15 chars total covers PCs (4-15 chars by character creation rules) and
// the short canonical NPC names commonly buffed (e.g. charmed pets).
const nameClass = `[A-Z][a-zA-Z']{2,14}`

// CastIndex matches log lines against spell cast text. Built once at startup
// from the spells_new table. Match() is safe for concurrent calls.
type CastIndex struct {
	youByText      map[string][]CastMessage
	otherByPattern []otherEntry
}

type otherEntry struct {
	suffix     string
	re         *regexp.Regexp
	candidates []CastMessage
}

// NewCastIndex builds the lookup structures from msgs. Duplicate cast text
// across spells is preserved as ambiguous candidates — the matcher returns
// every spell that shares the line so the engine can disambiguate.
func NewCastIndex(msgs []CastMessage) *CastIndex {
	youByText := make(map[string][]CastMessage)
	otherGroups := make(map[string][]CastMessage)

	for _, m := range msgs {
		if m.CastOnYou != "" {
			youByText[m.CastOnYou] = append(youByText[m.CastOnYou], m)
		}
		if m.CastOnOther != "" {
			otherGroups[m.CastOnOther] = append(otherGroups[m.CastOnOther], m)
		}
	}

	// Sort suffixes longest-first so a more specific suffix wins over a
	// shorter one that happens to be its tail. RE2 has no backtracking, so
	// we can't rely on alternation order to disambiguate — the linear scan
	// below picks the first suffix that matches.
	suffixes := make([]string, 0, len(otherGroups))
	for s := range otherGroups {
		suffixes = append(suffixes, s)
	}
	sort.Slice(suffixes, func(i, j int) bool {
		return len(suffixes[i]) > len(suffixes[j])
	})

	patterns := make([]otherEntry, 0, len(suffixes))
	for _, suf := range suffixes {
		re := regexp.MustCompile(`^(` + nameClass + `)` + regexp.QuoteMeta(suf) + `$`)
		patterns = append(patterns, otherEntry{
			suffix:     suf,
			re:         re,
			candidates: otherGroups[suf],
		})
	}

	return &CastIndex{
		youByText:      youByText,
		otherByPattern: patterns,
	}
}

// Match attempts to interpret line as a spell-landed message. Returns nil if
// no cast text matches.
//
// MatchSelf is tried first (literal full-line lookup, O(1)) before the more
// expensive MatchOther scan, since spells the player casts on themselves are
// the common path.
func (c *CastIndex) Match(line string) *CastMatch {
	if c == nil {
		return nil
	}
	if cands, ok := c.youByText[line]; ok && len(cands) > 0 {
		return buildMatch(MatchSelf, "", cands)
	}
	for _, oe := range c.otherByPattern {
		if sub := oe.re.FindStringSubmatch(line); sub != nil {
			return buildMatch(MatchOther, sub[1], oe.candidates)
		}
	}
	return nil
}

func buildMatch(kind MatchKind, target string, cands []CastMessage) *CastMatch {
	m := &CastMatch{
		Kind:       kind,
		TargetName: target,
		Candidates: cands,
	}
	if len(cands) == 1 {
		m.SpellID = cands[0].SpellID
		m.SpellName = cands[0].SpellName
	}
	return m
}

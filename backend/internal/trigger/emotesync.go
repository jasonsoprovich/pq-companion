package trigger

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Emote sync
//
// When the Spell Emote Customizer changes a spell's chat emote text, any
// trigger explicitly linked to that spell (Trigger.SpellID) may have a
// Pattern/WornOffPattern/ExtraPattern regex written against the OLD text —
// editing the emote without touching the trigger silently breaks it.
//
// This is deliberately suggest-and-confirm, never an automatic rewrite: a
// regex is not guaranteed to embed the literal emote text as a clean
// substring (named capture groups, alternation, hand-tuned anchors), and the
// exact use case that motivates the emote customizer — one trigger
// intentionally matching several spells that share an emote — is precisely
// the case a blind find-and-replace would wrongly mangle. So a suggestion is
// only ever offered where the old text is unambiguously present, either
// literally or as its regexp.QuoteMeta-escaped form (how a hand-written
// pattern typically embeds it, e.g. a trailing "." escaped to "\."), and
// applying one is a single explicit action per trigger per location, with an
// audit trail so it can be reverted.

// PatternLocation identifies which regex field on a Trigger a match/apply
// targets.
type PatternLocation string

const (
	LocationPattern      PatternLocation = "pattern"
	LocationWornOff      PatternLocation = "worn_off_pattern"
	LocationExtraPattern PatternLocation = "extra_pattern"
)

// EmoteChange is one column's before/after text — the input to
// SuggestPatternUpdates, computed by the caller from a spell's default vs.
// current emote text.
type EmoteChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// PatternMatch is one regex field on one trigger whose text contains an old
// emote value, with the literal replacement already computed.
type PatternMatch struct {
	Location   PatternLocation `json:"location"`
	ExtraIndex int             `json:"extra_index"` // only meaningful when Location == LocationExtraPattern
	Field      string          `json:"field"`       // which EmoteChange.Field this match came from
	Current    string          `json:"current"`
	Suggested  string          `json:"suggested"`
}

// TriggerSuggestion groups every matched location on one trigger.
type TriggerSuggestion struct {
	TriggerID string         `json:"trigger_id"`
	Name      string         `json:"name"`
	PackName  string         `json:"pack_name"`
	Matches   []PatternMatch `json:"matches"`
}

// findReplacement looks for old's plain text, or its regexp.QuoteMeta-escaped
// form (how a hand-written pattern typically embeds literal text containing
// regex metacharacters, e.g. a trailing "." escaped to "\."), inside
// pattern. Returns the rewritten pattern and true if found; false if old
// isn't recognizably present — this never guesses at a fuzzy match.
func findReplacement(pattern, old, new string) (string, bool) {
	if old == "" {
		return "", false
	}
	if strings.Contains(pattern, old) {
		return strings.Replace(pattern, old, new, 1), true
	}
	escapedOld := regexp.QuoteMeta(old)
	if escapedOld != old && strings.Contains(pattern, escapedOld) {
		return strings.Replace(pattern, escapedOld, regexp.QuoteMeta(new), 1), true
	}
	return "", false
}

// SuggestPatternUpdates finds every trigger linked to spellID (via SpellID)
// whose Pattern, WornOffPattern, or an ExtraPattern contains one of changes'
// old values, and proposes the literal replacement for each.
func SuggestPatternUpdates(store *Store, spellID int, changes []EmoteChange) ([]TriggerSuggestion, error) {
	if spellID <= 0 {
		return []TriggerSuggestion{}, nil
	}
	triggers, err := store.List()
	if err != nil {
		return nil, err
	}

	var out []TriggerSuggestion
	for _, t := range triggers {
		if t.SpellID != spellID {
			continue
		}
		var matches []PatternMatch
		for _, c := range changes {
			if c.Old == "" || c.Old == c.New {
				continue
			}
			if suggested, ok := findReplacement(t.Pattern, c.Old, c.New); ok {
				matches = append(matches, PatternMatch{Location: LocationPattern, ExtraIndex: -1, Field: c.Field, Current: t.Pattern, Suggested: suggested})
			}
			if t.WornOffPattern != "" {
				if suggested, ok := findReplacement(t.WornOffPattern, c.Old, c.New); ok {
					matches = append(matches, PatternMatch{Location: LocationWornOff, ExtraIndex: -1, Field: c.Field, Current: t.WornOffPattern, Suggested: suggested})
				}
			}
			for i, ep := range t.ExtraPatterns {
				if suggested, ok := findReplacement(ep.Pattern, c.Old, c.New); ok {
					matches = append(matches, PatternMatch{Location: LocationExtraPattern, ExtraIndex: i, Field: c.Field, Current: ep.Pattern, Suggested: suggested})
				}
			}
		}
		if len(matches) > 0 {
			out = append(out, TriggerSuggestion{TriggerID: t.ID, Name: t.Name, PackName: t.PackName, Matches: matches})
		}
	}
	if out == nil {
		out = []TriggerSuggestion{}
	}
	return out, nil
}

// PatternAudit records one applied pattern change so it can be reverted.
// Storing NewPattern (not just PreviousPattern) is what lets RevertPatternUpdate
// verify nothing has changed the field again since this apply — otherwise
// reverting an EARLIER audit while a LATER one is still layered on top of it
// would blindly overwrite the field back to this audit's prior state,
// silently discarding the later change (the same staleness hazard
// ApplyPatternUpdate guards against, but on the revert side).
type PatternAudit struct {
	ID              string          `json:"id"`
	TriggerID       string          `json:"trigger_id"`
	Location        PatternLocation `json:"location"`
	ExtraIndex      int             `json:"extra_index"`
	PreviousPattern string          `json:"previous_pattern"`
	NewPattern      string          `json:"new_pattern"`
	AppliedAt       int64           `json:"applied_at"`
}

func (s *Store) migratePatternAudits() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trigger_pattern_audits (
			id               TEXT    NOT NULL PRIMARY KEY,
			trigger_id       TEXT    NOT NULL,
			location         TEXT    NOT NULL,
			extra_index      INTEGER NOT NULL DEFAULT -1,
			previous_pattern TEXT    NOT NULL,
			new_pattern      TEXT    NOT NULL DEFAULT '',
			applied_at       INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	// Idempotently add for databases created before new_pattern existed.
	if _, err := s.db.Exec(`ALTER TABLE trigger_pattern_audits ADD COLUMN new_pattern TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("add new_pattern column: %w", err)
	}
	return nil
}

func setPatternField(t *Trigger, location PatternLocation, extraIndex int, value string) error {
	switch location {
	case LocationPattern:
		t.Pattern = value
	case LocationWornOff:
		t.WornOffPattern = value
	case LocationExtraPattern:
		if extraIndex < 0 || extraIndex >= len(t.ExtraPatterns) {
			return fmt.Errorf("extra pattern index %d out of range", extraIndex)
		}
		t.ExtraPatterns[extraIndex].Pattern = value
	default:
		return fmt.Errorf("unknown pattern location %q", location)
	}
	return nil
}

func getPatternField(t *Trigger, location PatternLocation, extraIndex int) (string, error) {
	switch location {
	case LocationPattern:
		return t.Pattern, nil
	case LocationWornOff:
		return t.WornOffPattern, nil
	case LocationExtraPattern:
		if extraIndex < 0 || extraIndex >= len(t.ExtraPatterns) {
			return "", fmt.Errorf("extra pattern index %d out of range", extraIndex)
		}
		return t.ExtraPatterns[extraIndex].Pattern, nil
	default:
		return "", fmt.Errorf("unknown pattern location %q", location)
	}
}

// ApplyPatternUpdate rewrites one trigger's pattern at location (replacing
// old with new — only if the current text still literally contains old, so
// a stale suggestion can't clobber a pattern the user has since hand-edited
// again), records an audit row with the prior text, and saves the trigger.
// Returns the new audit ID.
func (s *Store) ApplyPatternUpdate(triggerID string, location PatternLocation, extraIndex int, old, new string) (string, error) {
	t, err := s.Get(triggerID)
	if err != nil {
		return "", err
	}
	current, err := getPatternField(t, location, extraIndex)
	if err != nil {
		return "", err
	}
	updated, ok := findReplacement(current, old, new)
	if !ok {
		return "", fmt.Errorf("pattern no longer contains %q — it may have already been edited", old)
	}
	if err := setPatternField(t, location, extraIndex, updated); err != nil {
		return "", err
	}

	id, err := NewID()
	if err != nil {
		return "", err
	}
	if _, err := s.db.Exec(
		`INSERT INTO trigger_pattern_audits (id, trigger_id, location, extra_index, previous_pattern, new_pattern, applied_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, triggerID, string(location), extraIndex, current, updated, time.Now().UTC().Unix(),
	); err != nil {
		return "", fmt.Errorf("record pattern audit: %w", err)
	}
	if err := s.Update(t); err != nil {
		return "", err
	}
	return id, nil
}

// RevertPatternUpdate restores the pattern text an audit row recorded before
// ApplyPatternUpdate changed it, then removes the audit row. Fails, without
// changing anything, if the field no longer equals exactly what this apply
// wrote — e.g. a later apply on the same field (the Caustic Mist / Putrefy
// Flesh case: one shared pattern, one audit per spell) is still layered on
// top, or the trigger was hand-edited again since. Reverting the more recent
// change first (or hand-editing directly) is the right move in that case,
// not silently discarding it.
func (s *Store) RevertPatternUpdate(auditID string) error {
	var a PatternAudit
	var location string
	err := s.db.QueryRow(
		`SELECT id, trigger_id, location, extra_index, previous_pattern, new_pattern, applied_at
		 FROM trigger_pattern_audits WHERE id = ?`, auditID,
	).Scan(&a.ID, &a.TriggerID, &location, &a.ExtraIndex, &a.PreviousPattern, &a.NewPattern, &a.AppliedAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("no such pattern audit %q", auditID)
	}
	if err != nil {
		return err
	}
	a.Location = PatternLocation(location)

	t, err := s.Get(a.TriggerID)
	if err != nil {
		return err
	}
	current, err := getPatternField(t, a.Location, a.ExtraIndex)
	if err != nil {
		return err
	}
	if current != a.NewPattern {
		return fmt.Errorf("this trigger's pattern has changed since this update was applied — revert the more recent change first, or edit the trigger directly")
	}
	if err := setPatternField(t, a.Location, a.ExtraIndex, a.PreviousPattern); err != nil {
		return err
	}
	if err := s.Update(t); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM trigger_pattern_audits WHERE id = ?`, auditID)
	return err
}

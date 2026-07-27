package trigger

import (
	"testing"
	"time"
)

func mustNewID(t *testing.T) string {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return id
}

// newLinkedTrigger inserts a minimal trigger linked to spellID via
// Trigger.SpellID, with the given Pattern/WornOffPattern.
func newLinkedTrigger(t *testing.T, store *Store, name string, spellID int, pattern, wornOff string) *Trigger {
	t.Helper()
	tr := &Trigger{
		ID:             mustNewID(t),
		Name:           name,
		Enabled:        true,
		Pattern:        pattern,
		WornOffPattern: wornOff,
		SpellID:        spellID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := store.Insert(tr); err != nil {
		t.Fatalf("insert trigger: %v", err)
	}
	return tr
}

// TestSuggestPatternUpdatesRealWorldCausticMist reproduces the exact
// built-in "Caustic Mist / Putrefy Flesh" trigger (packs.go): pattern
// `'s flesh begins to liquefy\.` — the canonical cast_on_other text
// regexp.QuoteMeta-escaped. Customizing Caustic Mist's cast_on_other emote
// must surface this as a suggestion with the escaped replacement text.
func TestSuggestPatternUpdatesRealWorldCausticMist(t *testing.T) {
	store := openTestStore(t)
	newLinkedTrigger(t, store, "Caustic Mist / Putrefy Flesh", 2814,
		`Your skin begins to rot\.`, "")

	changes := []EmoteChange{
		{Field: "cast_on_you", Old: "Your skin begins to rot.", New: "Your skin begins to rot (caustic mist)."},
	}
	suggestions, err := SuggestPatternUpdates(store, 2814, changes)
	if err != nil {
		t.Fatalf("SuggestPatternUpdates: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d: %+v", len(suggestions), suggestions)
	}
	sug := suggestions[0]
	if len(sug.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(sug.Matches), sug.Matches)
	}
	m := sug.Matches[0]
	if m.Location != LocationPattern {
		t.Fatalf("expected match on Pattern, got %v", m.Location)
	}
	want := `Your skin begins to rot \(caustic mist\)\.`
	if m.Suggested != want {
		t.Fatalf("suggested pattern = %q, want %q", m.Suggested, want)
	}
}

// TestSuggestPatternUpdatesPlainTextMatch covers a trigger whose author
// embedded the emote text with no metacharacters needing escape at all.
func TestSuggestPatternUpdatesPlainTextMatch(t *testing.T) {
	store := openTestStore(t)
	newLinkedTrigger(t, store, "Turgur's Insects Slow", 1588, `yawns`, "")

	changes := []EmoteChange{
		{Field: "cast_on_other", Old: " yawns.", New: " yawns. (Turgur's Insects)"},
	}
	suggestions, err := SuggestPatternUpdates(store, 1588, changes)
	if err != nil {
		t.Fatalf("SuggestPatternUpdates: %v", err)
	}
	// The trigger's pattern "yawns" contains neither " yawns." verbatim nor
	// its escaped form (the leading space / trailing period aren't in the
	// pattern at all) — this must NOT produce a false-confidence match.
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions for a non-matching pattern, got %+v", suggestions)
	}
}

// TestSuggestPatternUpdatesIgnoresUnlinkedTriggers ensures a trigger that
// happens to share the old emote text but isn't linked via SpellID (e.g. it
// intentionally matches several spells sharing an emote) is never touched —
// exactly the scenario the whole suggest-don't-auto-rewrite design exists
// to protect.
func TestSuggestPatternUpdatesIgnoresUnlinkedTriggers(t *testing.T) {
	store := openTestStore(t)
	// Linked to a different spell than the one being edited.
	newLinkedTrigger(t, store, "Some Other Slow", 9999, `yawns\.`, "")
	// Not linked to any spell at all (SpellID left 0).
	newLinkedTrigger(t, store, "Generic Slow Watcher", 0, `yawns\.`, "")

	changes := []EmoteChange{
		{Field: "cast_on_other", Old: " yawns.", New: " yawns. (Turgur's Insects)"},
	}
	suggestions, err := SuggestPatternUpdates(store, 1588, changes)
	if err != nil {
		t.Fatalf("SuggestPatternUpdates: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions for triggers not linked to this spell, got %+v", suggestions)
	}
}

// TestSuggestPatternUpdatesWornOffAndExtraPatterns covers matches outside
// the primary Pattern field.
func TestSuggestPatternUpdatesWornOffAndExtraPatterns(t *testing.T) {
	store := openTestStore(t)
	tr := newLinkedTrigger(t, store, "Divine Aura Watch", 207, `some other line`, `Your invulnerability fades\.`)
	tr.ExtraPatterns = []ExtraPattern{{Pattern: `has had Divine Aura cast on them`, Enabled: true}}
	if err := store.Update(tr); err != nil {
		t.Fatalf("update trigger: %v", err)
	}

	changes := []EmoteChange{
		{Field: "spell_fades", Old: "Your invulnerability fades.", New: "The divine light fades."},
		{Field: "cast_on_other", Old: "has had Divine Aura cast on them", New: "has had DIVINE AURA cast on them"},
	}
	suggestions, err := SuggestPatternUpdates(store, 207, changes)
	if err != nil {
		t.Fatalf("SuggestPatternUpdates: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	matches := suggestions[0].Matches
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (worn-off + extra pattern), got %d: %+v", len(matches), matches)
	}
	var sawWornOff, sawExtra bool
	for _, m := range matches {
		if m.Location == LocationWornOff {
			sawWornOff = true
			if m.Suggested != `The divine light fades\.` {
				t.Errorf("worn-off suggestion = %q", m.Suggested)
			}
		}
		if m.Location == LocationExtraPattern {
			sawExtra = true
			if m.ExtraIndex != 0 {
				t.Errorf("expected extra index 0, got %d", m.ExtraIndex)
			}
			if m.Suggested != "has had DIVINE AURA cast on them" {
				t.Errorf("extra pattern suggestion = %q", m.Suggested)
			}
		}
	}
	if !sawWornOff || !sawExtra {
		t.Fatalf("expected both worn-off and extra-pattern matches, got %+v", matches)
	}
}

// TestApplyAndRevertPatternUpdate covers the full explicit-confirm lifecycle:
// applying a suggestion changes exactly the targeted trigger field, and
// reverting via the recorded audit restores the original text exactly.
func TestApplyAndRevertPatternUpdate(t *testing.T) {
	store := openTestStore(t)
	tr := newLinkedTrigger(t, store, "Caustic Mist / Putrefy Flesh", 2814, `Your skin begins to rot\.`, "")

	auditID, err := store.ApplyPatternUpdate(tr.ID, LocationPattern, -1,
		"Your skin begins to rot.", "Your skin begins to rot (caustic mist).")
	if err != nil {
		t.Fatalf("ApplyPatternUpdate: %v", err)
	}
	if auditID == "" {
		t.Fatal("expected a non-empty audit id")
	}

	updated, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantApplied := `Your skin begins to rot \(caustic mist\)\.`
	if updated.Pattern != wantApplied {
		t.Fatalf("pattern after apply = %q, want %q", updated.Pattern, wantApplied)
	}

	if err := store.RevertPatternUpdate(auditID); err != nil {
		t.Fatalf("RevertPatternUpdate: %v", err)
	}
	reverted, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get after revert: %v", err)
	}
	if reverted.Pattern != `Your skin begins to rot\.` {
		t.Fatalf("pattern after revert = %q, want original", reverted.Pattern)
	}

	// The audit row itself must be gone — reverting twice is an error, not a
	// silent no-op that could revert to some other, unrelated prior state.
	if err := store.RevertPatternUpdate(auditID); err == nil {
		t.Fatal("expected reverting an already-consumed audit id to fail")
	}
}

// TestRevertPatternUpdateRejectsWhenLaterApplyIsLayeredOnTop reproduces the
// real Caustic Mist / Putrefy Flesh scenario live-tested in the app: one
// shared trigger pattern gets two separate applies (one per spell's emote
// change). Reverting the FIRST apply while the SECOND is still layered on
// top must fail cleanly rather than blindly overwriting the field back to
// the first apply's prior state — which would silently discard the second,
// still-valid change.
func TestRevertPatternUpdateRejectsWhenLaterApplyIsLayeredOnTop(t *testing.T) {
	store := openTestStore(t)
	tr := newLinkedTrigger(t, store, "Caustic Mist / Putrefy Flesh", 2814,
		`^(?:Your skin begins to rot\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`, "")

	auditYou, err := store.ApplyPatternUpdate(tr.ID, LocationPattern, -1,
		`^(?:Your skin begins to rot\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`,
		`^(?:Your skin begins to rot \(caustic mist\)\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`)
	if err != nil {
		t.Fatalf("ApplyPatternUpdate (cast_on_you): %v", err)
	}

	auditOther, err := store.ApplyPatternUpdate(tr.ID, LocationPattern, -1,
		`^(?:Your skin begins to rot \(caustic mist\)\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`,
		`^(?:Your skin begins to rot \(caustic mist\)\.|[a-zA-Z']+'s flesh begins to liquefy \(caustic mist\)\.)$`)
	if err != nil {
		t.Fatalf("ApplyPatternUpdate (cast_on_other): %v", err)
	}

	// Reverting the FIRST (cast_on_you) apply must fail, since the SECOND
	// (cast_on_other) apply changed the field again after it.
	if err := store.RevertPatternUpdate(auditYou); err == nil {
		t.Fatal("expected reverting an out-of-order audit to fail")
	}

	afterFailedRevert, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantBothApplied := `^(?:Your skin begins to rot \(caustic mist\)\.|[a-zA-Z']+'s flesh begins to liquefy \(caustic mist\)\.)$`
	if afterFailedRevert.Pattern != wantBothApplied {
		t.Fatalf("pattern corrupted by a rejected revert: got %q, want %q", afterFailedRevert.Pattern, wantBothApplied)
	}

	// Reverting the SECOND (most recent) apply must succeed, and must only
	// undo that one change — the first apply's fix stays in place.
	if err := store.RevertPatternUpdate(auditOther); err != nil {
		t.Fatalf("RevertPatternUpdate (most recent): %v", err)
	}
	afterOneRevert, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantOnlyYouApplied := `^(?:Your skin begins to rot \(caustic mist\)\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`
	if afterOneRevert.Pattern != wantOnlyYouApplied {
		t.Fatalf("pattern after reverting the most recent apply = %q, want %q", afterOneRevert.Pattern, wantOnlyYouApplied)
	}

	// Now reverting the first apply must succeed, since nothing is layered on top anymore.
	if err := store.RevertPatternUpdate(auditYou); err != nil {
		t.Fatalf("RevertPatternUpdate (now unblocked): %v", err)
	}
	final, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantOriginal := `^(?:Your skin begins to rot\.|[a-zA-Z']+'s flesh begins to liquefy\.)$`
	if final.Pattern != wantOriginal {
		t.Fatalf("pattern after both reverts = %q, want original %q", final.Pattern, wantOriginal)
	}
}

// TestApplyPatternUpdateRejectsStaleSuggestion covers the safety check: if
// the trigger's pattern no longer contains the old text (e.g. the user
// hand-edited it in the meantime), applying a stale suggestion must fail
// rather than silently doing nothing or corrupting an unrelated pattern.
func TestApplyPatternUpdateRejectsStaleSuggestion(t *testing.T) {
	store := openTestStore(t)
	tr := newLinkedTrigger(t, store, "Already Hand-Edited", 2814, `something completely different`, "")

	if _, err := store.ApplyPatternUpdate(tr.ID, LocationPattern, -1,
		"Your skin begins to rot.", "Your skin begins to rot (caustic mist)."); err == nil {
		t.Fatal("expected ApplyPatternUpdate to fail when old text is no longer present")
	}

	unchanged, err := store.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if unchanged.Pattern != "something completely different" {
		t.Fatalf("pattern was mutated despite a failed apply: %q", unchanged.Pattern)
	}
}

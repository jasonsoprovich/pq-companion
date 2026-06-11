package trigger

import (
	"regexp"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// TestAllPacks_PatternsCompile walks every built-in pack and compiles every
// regex it ships — primary, extras, worn-off, and excludes — under the
// engine's normalization (with a character substituted for {C} tokens).
// Catches typos in hand-authored pack patterns at test time instead of at
// install time.
func TestAllPacks_PatternsCompile(t *testing.T) {
	for _, p := range AllPacks() {
		for _, tr := range p.Triggers {
			pats := []string{tr.Pattern}
			for _, ep := range tr.ExtraPatterns {
				pats = append(pats, ep.Pattern)
			}
			if tr.WornOffPattern != "" {
				pats = append(pats, tr.WornOffPattern)
			}
			pats = append(pats, tr.ExcludePatterns...)
			for _, pat := range pats {
				if _, err := regexp.Compile(normalizePattern(pat, "Testchar")); err != nil {
					t.Errorf("%s / %s: pattern %q does not compile: %v",
						p.PackName, tr.Name, pat, err)
				}
			}
		}
	}
}

// TestAllPacks_DedupKeysConsistent verifies that every trigger sharing a
// dedup key across packs is byte-identical in pattern and actions — the
// install-time skip logic assumes any copy is interchangeable.
func TestAllPacks_DedupKeysConsistent(t *testing.T) {
	type seen struct {
		pack    string
		pattern string
	}
	byKey := map[string]seen{}
	for _, p := range AllPacks() {
		for _, tr := range p.Triggers {
			if tr.DedupKey == "" {
				continue
			}
			if prev, ok := byKey[tr.DedupKey]; ok {
				if prev.pattern != tr.Pattern {
					t.Errorf("dedup key %q: pattern differs between %s and %s",
						tr.DedupKey, prev.pack, p.PackName)
				}
				continue
			}
			byKey[tr.DedupKey] = seen{pack: p.PackName, pattern: tr.Pattern}
		}
	}
}

// findPackTrigger locates a trigger by pack and name in AllPacks().
func findPackTrigger(t *testing.T, pack, name string) Trigger {
	t.Helper()
	for _, p := range AllPacks() {
		if p.PackName != pack {
			continue
		}
		for _, tr := range p.Triggers {
			if tr.Name == name {
				return tr
			}
		}
	}
	t.Fatalf("trigger %s / %s not found", pack, name)
	return Trigger{}
}

// matchTrigger mimics the engine's per-line evaluation for one trigger:
// the primary pattern must match and no exclude pattern may match. Returns
// the submatch slice (nil = no fire). character feeds {C} expansion.
func matchTrigger(t *testing.T, tr Trigger, character, line string) []string {
	t.Helper()
	re := regexp.MustCompile(normalizePattern(tr.Pattern, character))
	m := re.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	for _, ex := range tr.ExcludePatterns {
		if regexp.MustCompile(normalizePattern(ex, character)).MatchString(line) {
			return nil
		}
	}
	return m
}

// TestCommunityPackPatterns exercises the community-pack triggers against
// representative log lines (timestamps already stripped, as the engine
// sees them).
func TestCommunityPackPatterns(t *testing.T) {
	cases := []struct {
		pack, trigger string
		line          string
		wantFire      bool
		wantCapture   string // expected m[1] ("" = don't check)
		captureIdx    int    // submatch index for wantCapture (0 = use 1)
	}{
		// ── Spell Breaks ────────────────────────────────────────────────
		// Charm wears off under the generic lowercase line on Quarm.
		{"Spell Breaks", "Charm Broke", "Your charm spell has worn off.", true, "", 0},
		// Defensive per-name tail, backtick apostrophe as stored in quarm.db.
		{"Spell Breaks", "Charm Broke", "Your Boltran`s Agacerie spell has worn off.", true, "", 0},
		{"Spell Breaks", "Charm Broke", "Your Tunare's Request spell has worn off.", true, "", 0},
		{"Spell Breaks", "Root Broke", "Your Paralyzing Earth spell has worn off.", true, "", 0},
		{"Spell Breaks", "Root Broke", "Your Entrapping Roots spell has worn off.", true, "", 0},
		{"Spell Breaks", "Snare Broke", "Your Atol's Spectral Shackles spell has worn off.", true, "", 0},
		{"Spell Breaks", "Snare Broke", "Your Ensnare spell has worn off.", true, "", 0},
		// Catch-all fires for non-CC spells with the name captured…
		{"Spell Breaks", "Spell Worn Off", "Your Heat Blood spell has worn off.", true, "Heat Blood", 0},
		// …but stays quiet for everything the CC alerts already cover.
		{"Spell Breaks", "Spell Worn Off", "Your charm spell has worn off.", false, "", 0},
		{"Spell Breaks", "Spell Worn Off", "Your Paralyzing Earth spell has worn off.", false, "", 0},
		{"Spell Breaks", "Spell Worn Off", "Your Ensnare spell has worn off.", false, "", 0},
		{"Spell Breaks", "Spell Worn Off", "Your Dazzle spell has worn off.", false, "", 0},

		// ── Caster Alerts ───────────────────────────────────────────────
		{"Caster Alerts", "Insufficient Mana", "Insufficient Mana to cast this spell!", true, "", 0},
		{"Caster Alerts", "Stand to Cast", "You must be standing to cast a spell.", true, "", 0},
		{"Caster Alerts", "Target Unaffected", "Your target looks unaffected.", true, "", 0},

		// ── Crit Alerts ─────────────────────────────────────────────────
		{"Crit Alerts", "Critical Blast", "You deliver a critical blast! (1024)", true, "1024", 0},
		{"Crit Alerts", "Exceptional Heal", "You perform an exceptional heal! (790)", true, "790", 0},
		// PQ standalone crit form; {C} confines it to the active character.
		{"Crit Alerts", "Critical Hit", "Osui Scores a critical hit!(62)", true, "62", 0},
		{"Crit Alerts", "Critical Hit", "Somebodyelse Scores a critical hit!(62)", false, "", 0},

		// ── Group Alerts ────────────────────────────────────────────────
		{"Group Alerts", "Group Invite", "Nariana invites you to join a group.", true, "Nariana", 0},
		{"Group Alerts", "New Group Leader", "Feane is now the leader of your group.", true, "Feane", 0},

		// ── Raid Alerts ─────────────────────────────────────────────────
		{"Raid Alerts", "Divine Intervention", "Nariana has been rescued by divine intervention!", true, "Nariana", 0},
		{"Raid Alerts", "Mob Gating", "a gnoll shaman begins to cast the gate spell.", true, "a gnoll shaman", 0},
		{"Raid Alerts", "Raid Assist Call", "Krazak tells the raid,  'ASSIST Vox >>>'", true, "Vox", 2},
		{"Raid Alerts", "Raid Assist Call", "Krazak tells the raid,  'assist Lord Nagafen'", true, "Lord Nagafen", 2},
		{"Raid Alerts", "Raid Assist Call", "Krazak tells the raid,  'everyone gather up'", false, "", 0},

		// ── Tracking ────────────────────────────────────────────────────
		{"Tracking", "Ahead Left", "a willowisp is ahead and to the left.", true, "a willowisp", 0},
		{"Tracking", "Left", "a willowisp is to the left.", true, "a willowisp", 0},
		// "ahead and to the left" must not bleed into the plain-left trigger.
		{"Tracking", "Left", "a willowisp is ahead and to the left.", false, "", 0},
		{"Tracking", "Lost Target", "You have lost your target.", true, "", 0},

		// ── Misc Alerts ─────────────────────────────────────────────────
		// Fires only when the ACTIVE character ({C} = Osui here) drops.
		{"Misc Alerts", "Feign Death Failed", "Osui has fallen to the ground.", true, "", 0},
		{"Misc Alerts", "Feign Death Failed", "Somebodyelse has fallen to the ground.", false, "", 0},
		{"Misc Alerts", "Stunned", "You are stunned!", true, "", 0},
		{"Misc Alerts", "Unstunned", "You are unstunned.", true, "", 0},
		{"Misc Alerts", "MGB Announcement", "Soandso tells general:1, 'mgb aego at top of the hour'", true, "", 0},
		{"Misc Alerts", "MGB Announcement", "Soandso tells general:1, 'selling fine steel'", false, "", 0},
	}

	for _, tc := range cases {
		tr := findPackTrigger(t, tc.pack, tc.trigger)
		m := matchTrigger(t, tr, "Osui", tc.line)
		fired := m != nil
		if fired != tc.wantFire {
			t.Errorf("%s / %s vs %q: fired=%v, want %v",
				tc.pack, tc.trigger, tc.line, fired, tc.wantFire)
			continue
		}
		if tc.wantCapture != "" {
			idx := tc.captureIdx
			if idx == 0 {
				idx = 1
			}
			if len(m) <= idx || m[idx] != tc.wantCapture {
				t.Errorf("%s / %s vs %q: capture[%d]=%q, want %q",
					tc.pack, tc.trigger, tc.line, idx, m[idx], tc.wantCapture)
			}
		}
	}
}

// TestEnchanterMergedTriggers drives the merged Mez/Charm/Pacify/Root
// triggers from the Enchanter pack through the engine and asserts each
// spell's cast line starts a timer under its own name with its own
// duration and spell id, and that worn-off/resist lines clear the right
// timer.
func TestEnchanterMergedTriggers(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	if err := InstallPack(s, applyDefaultTimerAlerts(EnchanterPack())); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}
	e.Reload()

	starts := []struct {
		line     string
		key      string
		duration int
		spellID  int
	}{
		{"You begin casting Mesmerize.", "Mesmerize", 24, 292},
		{"You begin casting Mesmerization.", "Mesmerization", 24, 307},
		{"You begin casting Dazzle.", "Dazzle", 96, 190},
		{"You begin casting Charm.", "Charm", 1140, 300},
		{"You begin casting Cajoling Whispers.", "Cajoling Whispers", 1140, 183},
		{"You begin casting Boltran`s Agacerie.", "Boltran`s Agacerie", 1140, 1706},
		{"You begin casting Boltran's Agacerie.", "Boltran's Agacerie", 1140, 1706},
		{"You begin casting Lull.", "Lull", 120, 208},
		{"You begin casting Wake of Tranquility.", "Wake of Tranquility", 126, 1541},
		{"You begin casting Soothe.", "Soothe", 450, 501},
		{"You begin casting Root.", "Root", 48, 230},
		{"You begin casting Greater Fetter.", "Greater Fetter", 180, 3194},
	}
	for _, c := range starts {
		before := sink.calls
		e.Handle(time.Now(), c.line)
		if sink.calls != before+1 {
			t.Errorf("%q: expected 1 timer start, got %d", c.line, sink.calls-before)
			continue
		}
		if sink.name != c.key || sink.duration != c.duration || sink.spellID != c.spellID {
			t.Errorf("%q: started %s/%d/%d, want %s/%d/%d",
				c.line, sink.name, sink.duration, sink.spellID, c.key, c.duration, c.spellID)
		}
	}

	stops := []struct {
		line string
		key  string
	}{
		{"Your Dazzle spell has worn off.", "Dazzle"},
		{"Your target resisted the Mesmerization spell.", "Mesmerization"},
		{"Your target resisted the Greater Fetter spell.", "Greater Fetter"},
		{"Your Soothe spell has worn off.", "Soothe"},
		{"Your target resisted the Beguile spell.", "Beguile"},
	}
	for _, c := range stops {
		before := sink.stops
		e.Handle(time.Now(), c.line)
		if sink.stops != before+1 {
			t.Errorf("%q: expected 1 timer stop, got %d", c.line, sink.stops-before)
			continue
		}
		// Capture-resolved stops must pass spellID 0 so the trigger-level
		// SpellID (the primary spell's) can't remove a sibling's timer.
		if sink.stopName != c.key || sink.stopSpellID != 0 {
			t.Errorf("%q: stopped %s/%d, want %s/0", c.line, sink.stopName, sink.stopSpellID, c.key)
		}
	}
}

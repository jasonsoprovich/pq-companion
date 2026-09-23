package popflag

import "testing"

// splitBlock is a small test helper: a block as printed in-game, one message
// per line (matching what logparser.ParseRawLine would hand the consumer,
// stripped of the "[Mon Apr 13 ...]" timestamp prefix).
func splitBlock(lines ...string) []string { return lines }

func TestParsePopFlagsReportOverview(t *testing.T) {
	block := splitBlock(
		"=== Planes of Power Progression ===",
		"Tier 1: Complete",
		"Tier 2: In progress",
		"Tier 3: Not started",
		"Tier 4: Not started",
		"Tier 5 - Plane of Time: Not started",
		"Details: #popflags 1, 2, 3, 4, or 5 (tier1-tier5 also work).",
	)
	r := ParsePopFlagsReport(block)
	if r.Section != PopFlagsOverview {
		t.Fatalf("section = %q, want overview", r.Section)
	}
	wantAtLeast := map[string]int{"mavuin": 3, "fuirstel": 5, "thelin": 4, "poi_door": 1, "zeks": 2}
	for k, v := range wantAtLeast {
		if r.AtLeast[k] != v {
			t.Errorf("AtLeast[%q] = %d, want %d", k, r.AtLeast[k], v)
		}
	}
	// Tier 2 "In progress" contributes nothing.
	if _, ok := r.Exact["aerindar"]; ok {
		t.Errorf("Tier 2 In progress should not set aerindar")
	}
	// Tier 3 Not started marks its fields confirmed absent.
	for _, q := range []string{"hohtrials", "mmarr", "mmarr_book", "cipher", "zebuxoruk", "zeks", "sol_room", "pofire"} {
		if v, ok := r.Exact[q]; !ok || v != "" {
			// zeks already got AtLeast=2 from Tier 1; Tier3's absence-mark for
			// zeks would contradict that, but the server would never actually
			// print Tier1:Complete AND Tier3:NotStarted in the same real
			// report (self-contradictory), so we only assert the OTHER,
			// non-contradicted fields here.
			if q == "zeks" {
				continue
			}
			t.Errorf("Exact[%q] = (%q,%v), want (\"\",true)", q, v, ok)
		}
	}
}

func TestParsePopFlagsReportTier1(t *testing.T) {
	block := splitBlock(
		"=== Tier 1 Progression ===",
		"--- Plane of Justice ---",
		"Mavuin's case: Mavuin's case is complete",
		"Seventh Hammer access: Unlocked",
		"--- Plane of Disease ---",
		"Fuirstel progression: Crypt of Decay access was granted",
		"Crypt of Decay access: Unlocked",
		"Pending memory: Grummus",
		"--- Plane of Nightmare ---",
		"Thelin progression: Terris Thule was defeated",
		"Pending memory: Thelin's hedge maze",
		"--- Plane of Innovation ---",
		"Factory door access: Locked",
		"Giwin and Manaetic Behemoth progression: Not started",
		"Tier complete: Justice has been served, Disease's grip weakened, Nightmare ended, and Innovation's hidden plot uncovered.",
	)
	r := ParsePopFlagsReport(block)
	if r.Section != PopFlagsTier1 {
		t.Fatalf("section = %q, want tier1", r.Section)
	}
	if r.Exact["mavuin"] != "3" {
		t.Errorf("mavuin = %q, want 3", r.Exact["mavuin"])
	}
	if r.Exact["seventh"] != "1" {
		t.Errorf("seventh = %q, want 1", r.Exact["seventh"])
	}
	if r.Exact["fuirstel"] != "4" {
		t.Errorf("fuirstel = %q, want 4 (Crypt of Decay access was granted = index 4)", r.Exact["fuirstel"])
	}
	if r.Exact["grummus"] != "1" {
		t.Errorf("grummus = %q, want 1", r.Exact["grummus"])
	}
	if r.Exact["thelin"] != "3" {
		t.Errorf("thelin = %q, want 3", r.Exact["thelin"])
	}
	if r.Exact["poi_door"] != "" {
		t.Errorf("poi_door = %q, want \"\" (Locked)", r.Exact["poi_door"])
	}
	if r.Exact["zeks"] != "" {
		t.Errorf("zeks = %q, want \"\" (Not started)", r.Exact["zeks"])
	}
	if !r.Pending["cl_grummus"] || !r.Pending["cl_maze"] {
		t.Errorf("pending = %+v, want cl_grummus and cl_maze", r.Pending)
	}
}

func TestParsePopFlagsReportBitmasks(t *testing.T) {
	block := splitBlock(
		"=== Tier 3 Progression ===",
		"--- Halls of Honor ---",
		"Rydda`Dar trial: Complete",
		"Village trial: Incomplete",
		"Nomad trial: Complete",
		"Mithaniel Marr cipher half: Incomplete",
		"--- Tower of Solusek Ro ---",
		"Xuzl: Complete",
		"Arlyxir: Incomplete",
		"Dresolik: Incomplete",
		"Rizlona: Complete",
		"Jiva: Incomplete",
	)
	r := ParsePopFlagsReport(block)
	if r.Exact["hohtrials"] != "101" {
		t.Errorf("hohtrials = %q, want 101", r.Exact["hohtrials"])
	}
	if r.Exact["sol_room"] != "10010" {
		t.Errorf("sol_room = %q, want 10010", r.Exact["sol_room"])
	}
	if r.Exact["mmarr"] != "" {
		t.Errorf("mmarr = %q, want \"\" (Incomplete)", r.Exact["mmarr"])
	}
}

func TestParsePopFlagsReportNoneCompleted(t *testing.T) {
	block := splitBlock(
		"=== Tier 3 Progression ===",
		"--- Halls of Honor ---",
		"Halls of Honor trials: None completed",
		"--- Tower of Solusek Ro ---",
		"Tower wing flags: None completed",
	)
	r := ParsePopFlagsReport(block)
	if r.Exact["hohtrials"] != "000" {
		t.Errorf("hohtrials = %q, want 000", r.Exact["hohtrials"])
	}
	if r.Exact["sol_room"] != "00000" {
		t.Errorf("sol_room = %q, want 00000", r.Exact["sol_room"])
	}
}

func TestParsePopFlagsReportCipherAndZebuxoruk(t *testing.T) {
	block := splitBlock(
		"=== Tier 3 Progression ===",
		"--- Halls of Honor ---",
		"Mithaniel Marr cipher half: Combined into Cipher",
		"--- Bastion of Thunder ---",
		"Agnarr and Karana progression: Complete; combined into Zebuxoruk lore",
		"--- Grand Librarian Maelin ---",
		"Cipher information: Received",
		"Zebuxoruk lore: Received",
		"Combined Zek information: Received",
		"Final elemental information: Received",
	)
	r := ParsePopFlagsReport(block)
	if r.AtLeast["cipher"] != 1 {
		t.Errorf("cipher AtLeast = %d, want 1", r.AtLeast["cipher"])
	}
	if r.AtLeast["zebuxoruk"] != 2 {
		t.Errorf("zebuxoruk AtLeast = %d, want 2 (final elemental info)", r.AtLeast["zebuxoruk"])
	}
	if r.AtLeast["zeks"] != 5 {
		t.Errorf("zeks AtLeast = %d, want 5", r.AtLeast["zeks"])
	}
	// mmarr must NOT be set to "1" here — it was reported as combined, not complete.
	if _, ok := r.Exact["mmarr"]; ok {
		t.Errorf("mmarr should be untouched when combined into cipher, got %q", r.Exact["mmarr"])
	}
}

func TestParsePopFlagsReportKaranaTier2(t *testing.T) {
	block := splitBlock(
		"=== Tier 2 Progression ===",
		"--- Plane of Storms ---",
		"Askr and Karana progression: Bastion of Thunder progression recorded",
	)
	r := ParsePopFlagsReport(block)
	if r.Exact["karana"] != "3" {
		t.Errorf("karana = %q, want 3", r.Exact["karana"])
	}
}

func TestParsePopFlagsReportTime(t *testing.T) {
	block := splitBlock(
		"=== Plane of Time ===",
		"Plane of Time access: Unlocked",
		"Tier complete: The Plane of Time recognizes your soul. Beyond its shifting portals, the gods await.",
	)
	r := ParsePopFlagsReport(block)
	if r.Section != PopFlagsTime {
		t.Fatalf("section = %q, want time", r.Section)
	}
	if r.Exact["time"] != "1" {
		t.Errorf("time = %q, want 1", r.Exact["time"])
	}
}

func TestMatchPopFlagsLine(t *testing.T) {
	matching := []string{
		"=== Planes of Power Progression ===",
		"=== Tier 3 Progression ===",
		"--- Halls of Honor ---",
		"Tier 1: Complete",
		"Mavuin's case: Mavuin's case is complete",
		"Askr and Karana progression: Complete; combined into Zebuxoruk lore",
		"Rydda`Dar trial: Complete",
		"Pending memory: Saryrn",
		"Tier complete: whatever lore text follows",
		"Details: #popflags 1, 2, 3, 4, or 5 (tier1-tier5 also work).",
		"A checklist memory is ready to be unlocked.",
	}
	for _, line := range matching {
		if !MatchPopFlagsLine(line) {
			t.Errorf("MatchPopFlagsLine(%q) = false, want true", line)
		}
	}
	nonMatching := []string{
		"",
		"You slash a gnoll for 150 points of damage.",
		"Mavuin is grateful to you for taking his case before the Tribunal.", // Seer text, not popflags
		"=== Current Loot Lockouts ===",                                      // /sll header
	}
	for _, line := range nonMatching {
		if MatchPopFlagsLine(line) {
			t.Errorf("MatchPopFlagsLine(%q) = true, want false", line)
		}
	}
}

// TestPopFlagsAllFlagIDsExist guards against a typo in any qglobal name used
// above by cross-checking against the flags actually keyed off each one.
func TestPopFlagsQglobalNamesAreReal(t *testing.T) {
	known := map[string]bool{}
	for _, f := range Flags() {
		if f.Qglobal != "" {
			known[f.Qglobal] = true
		}
		for _, c := range f.SatisfiedBy {
			known[c.Qglobal] = true
		}
	}
	// Names referenced by this file that must exist somewhere in the dataset.
	for _, q := range []string{
		"mavuin", "seventh", "fuirstel", "grummus", "thelin", "poi_door", "zeks",
		"bertox_key", "saryrn", "mmarr", "cipher", "zebuxoruk", "pofire",
		"earthb_key", "time", "hohtrials", "sol_room", "karana", "aerindar", "tylis",
	} {
		if !known[q] {
			t.Errorf("qglobal %q used by the popflags parser is not backed by any dataset flag", q)
		}
	}
}

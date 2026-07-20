package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestIsPoPGated_QuestSources verifies the quest-script source data corrects
// the two classes of leak the DB-only derivation couldn't handle:
//
//   - Jade Hoop of Speed (32106): no drop/vendor row, but a quest reward in
//     Plane of Knowledge (PoP). Must now be gated (was leaking through).
//   - Sigil Earring of Veracity (29861): no drop/vendor row, but a quest
//     reward in Katta Castellum (Luclin, current era). Must NOT be gated.
func TestIsPoPGated_QuestSources(t *testing.T) {
	d := openTestDB(t)

	cases := []struct {
		id       int
		name     string
		wantGate bool
	}{
		{32106, "Jade Hoop of Speed (PoK quest reward)", true},
		{29861, "Sigil Earring of Veracity (Katta/Luclin quest reward)", false},
		{15929, "Headsman's Hoop (pojustice drop)", true},
		// Drops only from unspawned Plane of Time NPCs — invisible to the
		// spawn2 join, caught via the id-derived home zone.
		{9444, "Mask of Conceptual Energy (unspawned PoP NPC drop)", true},
	}
	for _, c := range cases {
		if got := d.IsPoPGated(c.id); got != c.wantGate {
			t.Errorf("IsPoPGated(%d) [%s] = %v, want %v", c.id, c.name, got, c.wantGate)
		}
	}
}

// TestPoPGatedEmbeddedMatchesLive guards the precomputed pop_gated.json the
// server actually reads (via IsPoPGated) against a fresh live build from the
// DB. They must be identical — if this fails, either buildPoPGated changed or
// quarm.db was updated without regenerating: run `go run ./cmd/pop-index`.
func TestPoPGatedEmbeddedMatchesLive(t *testing.T) {
	d := openTestDB(t)
	live, err := d.ComputePoPGated()
	if err != nil {
		t.Fatalf("ComputePoPGated: %v", err)
	}
	// Forward: every live-gated item is gated by the embedded set.
	for id := range live {
		if !d.IsPoPGated(id) {
			t.Fatalf("embedded set missing live-gated item %d — regenerate pop_gated.json (go run ./cmd/pop-index)", id)
		}
	}
	// Reverse: the embedded set gates nothing beyond the live set. Item IDs sit
	// well under this bound, so a range scan covers the whole space.
	for id := 1; id <= 100000; id++ {
		if d.IsPoPGated(id) && !live[id] {
			t.Fatalf("embedded set over-gates item %d (not in live build) — regenerate pop_gated.json", id)
		}
	}
}

// TestQuestsForItem checks the item→quest index backing the Quests tab.
func TestQuestsForItem(t *testing.T) {
	rewardedBy, _ := db.QuestsForItem(29861)
	if len(rewardedBy) == 0 {
		t.Fatal("expected Sigil Earring of Veracity to be rewarded by a quest")
	}
	found := false
	for _, q := range rewardedBy {
		if q.Zone == "katta" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a katta quest reward for 29861, got %+v", rewardedBy)
	}

	// A turn-in lookup: the Signet Earring (29860) is handed in to Lcea Katta.
	_, usedIn := db.QuestsForItem(29860)
	if len(usedIn) == 0 {
		t.Errorf("expected Signet Earring (29860) to be used as a quest turn-in")
	}
}

// TestSearchQuests checks the DB-explorer quest search matches on a related
// item name and resolves zone/item display fields.
func TestSearchQuests(t *testing.T) {
	d := openTestDB(t)
	res := d.SearchQuests("Sigil Earring of Veracity", 50, 0)
	if res.Total == 0 {
		t.Fatal("expected a quest matching the Sigil Earring item name")
	}
	found := false
	for _, q := range res.Results {
		if q.NPC == "Lcea Katta" && q.ZoneName != "" && q.ZoneName != q.ZoneShortName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Lcea Katta with a resolved zone name in results")
	}

	// Empty query returns the full set, paged.
	all := d.SearchQuests("", 10, 0)
	if all.Total < 100 || len(all.Results) != 10 {
		t.Errorf("empty-query paging off: total=%d page=%d", all.Total, len(all.Results))
	}
}

// TestGetItemQuests checks the display-resolved walkthrough: the Sigil Earring
// of Veracity is rewarded by Lcea Katta, whose dialogue must resolve with a
// zone long-name, readable text, and a branch that grants the item.
func TestGetItemQuests(t *testing.T) {
	d := openTestDB(t)
	q, err := d.GetItemQuests(29861) // Sigil Earring of Veracity
	if err != nil {
		t.Fatalf("GetItemQuests: %v", err)
	}
	if len(q.Walkthrough) == 0 {
		t.Fatal("expected a walkthrough for the Sigil Earring")
	}
	w := q.Walkthrough[0]
	if w.ZoneName == "" || w.ZoneName == w.ZoneShortName {
		t.Errorf("zone long-name not resolved: %+v", w)
	}
	if len(w.Dialogue) < 2 {
		t.Fatalf("expected multiple dialogue branches, got %d", len(w.Dialogue))
	}
	grantsTarget, hasText := false, false
	for _, b := range w.Dialogue {
		if b.Text != "" {
			hasText = true
		}
		for _, g := range b.Grants {
			if g.ID == 29861 {
				grantsTarget = true
			}
			if g.Name == "" {
				t.Errorf("unresolved grant item name for %d", g.ID)
			}
		}
	}
	if !grantsTarget {
		t.Error("expected a dialogue branch granting the Sigil Earring")
	}
	if !hasText {
		t.Error("expected at least one dialogue branch with NPC text")
	}
}

// TestQuestSources_CountHandedItemTurnIns checks the generator's
// count_handed_item support (a second turn-in API used alongside
// check_turn_in) recovered two previously-missing NPCs: Canloe Nusback
// (Crushbone Belt/shoulder pad turn-ins) and Herald Telcha's Green Goblin
// Skin turn-in, which check_turn_in-only parsing couldn't see at all.
func TestQuestSources_CountHandedItemTurnIns(t *testing.T) {
	// Crushbone Belt (13318): 3 real quest givers per pqdi.cc — Linadian
	// (Freeport West + Greater Faydark) and Canloe Nusback (South Kaladim).
	// Canloe Nusback was previously dropped entirely (zero resolvable
	// rewards or turn-ins), even though the NPC clearly turns the belt in.
	_, usedIn := db.QuestsForItem(13318)
	foundCanloe := false
	for _, q := range usedIn {
		if q.NPC == "Canloe Nusback" {
			foundCanloe = true
		}
	}
	if !foundCanloe {
		t.Errorf("expected Canloe Nusback among Crushbone Belt (13318) turn-ins, got %+v", usedIn)
	}

	// Green Goblin Skin (22135): turned in to Herald Telcha in Chardok.
	_, usedIn = db.QuestsForItem(22135)
	foundTelcha := false
	for _, q := range usedIn {
		if q.NPC == "Herald Telcha" {
			foundTelcha = true
		}
	}
	if !foundTelcha {
		t.Errorf("expected Herald Telcha among Green Goblin Skin (22135) turn-ins, got %+v", usedIn)
	}
}

// TestGetItemQuests_ResolvesFactionDeltas checks a dialogue branch's exact
// faction deltas (extracted from the quest script's own Faction() calls)
// resolve with a real faction_list name, backing both the item Quests tab
// and the Faction Tracker's quest-turn-in correlation.
func TestGetItemQuests_ResolvesFactionDeltas(t *testing.T) {
	d := openTestDB(t)
	// Di'zok Signet of Service (5728) is rewarded by Herald Telcha's Head of
	// Skargus turn-in branch, which also carries the branch's Faction()
	// deltas — unlike Green Goblin Skin (a pure turn-in with no reward, so
	// it never appears in a Walkthrough, only UsedIn).
	q, err := d.GetItemQuests(5728)
	if err != nil {
		t.Fatalf("GetItemQuests: %v", err)
	}
	found := false
	for _, w := range q.Walkthrough {
		for _, b := range w.Dialogue {
			for _, f := range b.Factions {
				if f.FactionName == "" {
					t.Errorf("unresolved faction name for id %d", f.FactionID)
				}
				if f.Delta != 0 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected at least one non-zero faction delta in Green Goblin Skin's walkthrough")
	}
}

// TestResolveQuestFactionDialogue checks the log-correlation resolver used by
// the Faction Tracker: an NPC's exact spoken text (as it would appear in a
// "<NPC> says, '...'" log line) resolves to the quest branch's faction
// deltas, while an unrelated hail line for the same NPC does not.
func TestResolveQuestFactionDialogue(t *testing.T) {
	openTestDB(t) // ensures quarm.db-backed quest sources are loaded once

	hits, ok := db.ResolveQuestFactionDialogue("Herald Telcha",
		"Green Goblin Skin! You have indeed been busy!")
	if !ok || len(hits) == 0 {
		t.Fatalf("expected a faction match for Herald Telcha's Green Goblin Skin turn-in, got hits=%+v ok=%v", hits, ok)
	}
	foundSarnak := false
	for _, h := range hits {
		if h.Delta == 3 {
			foundSarnak = true
		}
	}
	if !foundSarnak {
		t.Errorf("expected a +3 faction delta among %+v", hits)
	}

	if _, ok := db.ResolveQuestFactionDialogue("Herald Telcha", "Hail to you, lesser being!"); ok {
		t.Error("expected the unconditional hail line to have no faction match")
	}
	if _, ok := db.ResolveQuestFactionDialogue("Nobody Real", "anything"); ok {
		t.Error("expected an unknown NPC to have no faction match")
	}
}

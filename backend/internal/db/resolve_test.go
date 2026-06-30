package db_test

import (
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestGetNPCIDByName_RoundTrip takes a real NPC, renders its name the way the
// game (and /sll) prints it — underscores as spaces — and confirms the lockout
// resolver maps it back to a valid npc id.
func TestGetNPCIDByName_RoundTrip(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchNPCs("Nagafen", 1, 0, false)
	if err != nil {
		t.Fatalf("search npcs: %v", err)
	}
	if len(res.Items) == 0 {
		t.Skip("no Nagafen NPC in test DB")
	}
	want := res.Items[0]
	display := strings.ReplaceAll(want.Name, "_", " ")

	id, ok := d.GetNPCIDByName(display)
	if !ok {
		t.Fatalf("GetNPCIDByName(%q) found nothing", display)
	}
	// Resolves to *a* same-name NPC; with the ORDER BY id LIMIT 1 it should be
	// the lowest id sharing that name, which is <= the search hit's id.
	if id <= 0 {
		t.Errorf("got non-positive id %d", id)
	}

	if _, miss := d.GetNPCIDByName("Definitely Not A Real Boss 12345"); miss {
		t.Errorf("expected no match for a bogus name")
	}
}

// TestGetItemIDByName_RoundTrip confirms an exact item name resolves back to a
// valid item id for the legacy-lockout link.
func TestGetItemIDByName_RoundTrip(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchItems(db.ItemFilter{Query: "Robe", ItemType: -1, Limit: 1})
	if err != nil {
		t.Fatalf("search items: %v", err)
	}
	if len(res.Items) == 0 {
		t.Skip("no Robe item in test DB")
	}
	want := res.Items[0]

	id, ok := d.GetItemIDByName(want.Name)
	if !ok {
		t.Fatalf("GetItemIDByName(%q) found nothing", want.Name)
	}
	if id <= 0 {
		t.Errorf("got non-positive id %d", id)
	}

	// Case-insensitivity: lower-cased name resolves to the same canonical id.
	if lid, lok := d.GetItemIDByName(strings.ToLower(want.Name)); !lok || lid != id {
		t.Errorf("case-insensitive lookup: got (%d,%v), want (%d,true)", lid, lok, id)
	}

	if _, miss := d.GetItemIDByName("Nonexistent Item Zzzz 99999"); miss {
		t.Errorf("expected no match for a bogus item name")
	}
}

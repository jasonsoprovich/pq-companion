// Package keyring tracks per-character keyring state (the EQ /keys command's
// list of granted zone-access items) by parsing the /keys log output.
//
// The master list of "what items are keyring-eligible" comes from the
// authoritative quarm.db keyring_data table, NOT from items.itemtype: that
// table includes assembled keys (Ring of the Shissar, Scepter of Shadows),
// Plane of Sky island flags, and a handful of alternates that aren't tagged
// itemtype=33. Each keyring_data.key_name string matches exactly what EQ
// prints to the log on /keys, which is also what the consumer matches on.
package keyring

import (
	"database/sql"
	"fmt"
)

// MasterEntry is one keyring-eligible item from quarm.db keyring_data,
// enriched with the underlying item name (for tooltips / sorting) and the
// zone the key applies to (for UI grouping).
type MasterEntry struct {
	KeyItem  int    `json:"key_item"`  // canonical item ID (lowest, when duplicates share a key_name)
	KeyName  string `json:"key_name"`  // exact string EQ prints to /keys output — also the consumer's match key
	ItemName string `json:"item_name"` // items.Name for the canonical KeyItem (may differ from KeyName)
	ZoneID   int    `json:"zone_id"`
	ZoneName string `json:"zone_name"` // zone.long_name, empty if zone row is missing
	Stage    int    `json:"stage"`     // multi-stage progression position (e.g. Tower of Frozen Shadow 1–7); 0 for single-key zones
}

// LoadMaster reads the keyring master list from quarm.db, deduping rows that
// share a (key_name, zoneid) — Veeshan's Key has two item IDs for the same
// keyring slot, so we pick the lower one as canonical and the UI treats them
// as one entry.
func LoadMaster(db *sql.DB) ([]MasterEntry, error) {
	rows, err := db.Query(`
		SELECT
			MIN(k.key_item) AS key_item,
			k.key_name,
			k.zoneid,
			MIN(k.stage)    AS stage,
			COALESCE(i.Name, '')         AS item_name,
			COALESCE(z.long_name, '')    AS zone_name
		FROM keyring_data k
		LEFT JOIN items i ON i.id = (SELECT MIN(kk.key_item) FROM keyring_data kk WHERE kk.key_name = k.key_name AND kk.zoneid = k.zoneid)
		LEFT JOIN zone  z ON z.zoneidnumber = k.zoneid
		GROUP BY k.key_name, k.zoneid
		ORDER BY z.long_name, k.stage, k.key_name
	`)
	if err != nil {
		return nil, fmt.Errorf("query keyring_data: %w", err)
	}
	defer rows.Close()

	var out []MasterEntry
	for rows.Next() {
		var e MasterEntry
		if err := rows.Scan(&e.KeyItem, &e.KeyName, &e.ZoneID, &e.Stage, &e.ItemName, &e.ZoneName); err != nil {
			return nil, fmt.Errorf("scan keyring_data row: %w", err)
		}
		if name, ok := zoneIDOverrides[e.ZoneID]; ok && e.ZoneName == "" {
			e.ZoneName = name
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// zoneIDOverrides patches keyring_data rows whose zoneid doesn't match any
// row in the zone table. The upstream Quarm dump has Griegs Key pointing at
// zoneid=140, but Grieg's End is zoneidnumber=163 — the JOIN comes back
// empty and the UI shows "Unknown zone". Single static fallback covers the
// case without obscuring legitimate future mismatches (they'll still surface
// as "Unknown zone" until added here).
var zoneIDOverrides = map[int]string{
	140: "Grieg's End",
}

// NameIndex maps each master entry's key_name to its canonical KeyItem ID.
// The consumer uses this to translate matched log lines into item IDs.
func NameIndex(entries []MasterEntry) map[string]int {
	m := make(map[string]int, len(entries))
	for _, e := range entries {
		m[e.KeyName] = e.KeyItem
	}
	return m
}

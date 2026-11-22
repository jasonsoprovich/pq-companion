package popflag

// zoneAccess maps a flag-gated zone's short_name (zone.flag_needed is set)
// to the dataset flag IDs that grant entry. quarm.db's flag_needed column only
// holds the zone's own short name, so it says THAT a flag is needed but not
// how to get it — this curated table answers the "how" from the same dataset
// the PoP Flags page uses. Every listed ID must be required to enter (AND).
//
// Not listed: powar (flagged in the DB but not part of Quarm's PoP
// progression) — the zone page still shows "flag required" for it.
var zoneAccess = map[string][]string{
	"povalor":    {"poj_mavuin_return"},
	"postorms":   {"poj_mavuin_return"},
	"hohonora":   {"pov_aerindar"},
	"hohonorb":   {"hoh_trial_rydda", "hoh_trial_villagers", "hoh_trial_maidens"},
	"bothunder":  {"post_bot_shrine"},
	"nightmareb": {"ponb_hedge"},
	"codecay":    {"pod_grummus"},
	"potorment":  {"cod_postflag", "ponb_postflag"},
	"potactics":  {"poi_mb"},
	"solrotower": {"solro_miak"},
	"pofire":     {"solro_solusek"},
	"poair":      {"pok_librarian"},
	"powater":    {"pok_librarian"},
	"poeartha":   {"pok_librarian"},
	"poearthb":   {"poearth_arbitor"},
	"potimea":    {"potime"},
	"potimeb":    {"potime"},
}

// ZoneAccess returns a copy of the zone short_name → required flag IDs table.
func ZoneAccess() map[string][]string {
	out := make(map[string][]string, len(zoneAccess))
	for k, v := range zoneAccess {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// zoneLevelAccess maps the zones the Plane of Tranquility portals open by
// level alone to that level ("alternate access", restored by Project Quarm's
// 2026-11-23 patch: SecretsOTheP/quests potranquility/player.lua @ 05a3ec0).
// Clicking the portal at or above the level sets the zone flag, though not
// the progression steps. The higher planes, Halls of Honor B, Nightmare B,
// the elemental planes and Time still need their flags (or raid willing).
var zoneLevelAccess = map[string]int{
	"povalor":   55,
	"postorms":  55,
	"codecay":   55,
	"potorment": 55,
	"potactics": 60,
	"bothunder": 62,
	"hohonora":  62,
}

// ZoneLevelAccess returns a copy of the zone short_name → portal level table.
func ZoneLevelAccess() map[string]int {
	out := make(map[string]int, len(zoneLevelAccess))
	for k, v := range zoneLevelAccess {
		out[k] = v
	}
	return out
}

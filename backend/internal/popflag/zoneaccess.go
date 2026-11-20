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

package shoproute

// Alignment classifies a zone as a good, evil, or neutral place to shop. It
// exists so a player can keep the route out of cities where they'd be kill-on-
// sight (an evil character routed through Qeynos, or vice versa).
//
// This is a curated proxy, not a faction calculation. True "will the guards and
// merchants deal with my specific character" depends on race/class/deity and
// faction grind (npc_faction → faction_list → character_faction_values) and is
// deliberately out of scope here. Instead we hard-classify the handful of
// race/city-aligned hometowns that are reliably hostile to the opposite
// alignment; everything else — wilderness, dungeons, and the neutral hubs
// (PoK, Bazaar, Nexus, Shadow Haven, Shar Vahl, Freeport, Highpass) — is
// neutral and never filtered. The lists are intentionally easy to edit as
// feedback comes in.

const (
	AlignmentGood    = "good"
	AlignmentNeutral = "neutral"
	AlignmentEvil    = "evil"
)

// goodZones are hometowns hostile to evil/most-evil races. Keyed by zone
// short_name; multi-zone cities list each segment.
var goodZones = map[string]bool{
	"qeynos":     true, // North Qeynos
	"qeynos2":    true, // South Qeynos
	"qrg":        true, // Surefall Glade
	"erudnint":   true, // Erudin (inner)
	"erudnext":   true, // Erudin (outer)
	"felwithea":  true, // Northern Felwithe
	"felwitheb":  true, // Southern Felwithe
	"kaladima":   true, // Kaladim (north)
	"kaladimb":   true, // Kaladim (south)
	"gfaydark":   true, // Kelethin (Greater Faydark)
	"kelethin":   true, // Kelethin (if separately keyed)
	"rivervale":  true, // Rivervale
	"thurgadina": true, // Thurgadin (Coldain)
	"thurgadinb": true, // Thurgadin (Icewell Keep)
}

// evilZones are hometowns hostile to good races.
var evilZones = map[string]bool{
	"neriaka": true, // Neriak Foreign Quarter
	"neriakb": true, // Neriak Commons
	"neriakc": true, // Neriak Third Gate
	"grobb":   true, // Grobb (trolls)
	"oggok":   true, // Oggok (ogres)
	"cabwest": true, // Cabilis West (Iksar)
	"cabeast": true, // Cabilis East (Iksar)
	"paineel": true, // Paineel (Hole erudites / necromancers)
}

// Alignment returns the alignment of a zone by its short_name. Unknown zones
// (the common case) are neutral.
func Alignment(zoneShort string) string {
	switch {
	case goodZones[zoneShort]:
		return AlignmentGood
	case evilZones[zoneShort]:
		return AlignmentEvil
	default:
		return AlignmentNeutral
	}
}

package db

import "strings"

// This file curates the race restrictions on "cultural" tradeskill combine
// containers — the racial newbie-armor kits bought from a race's home-city
// merchant with faction. quarm.db encodes NO race/class restriction on these
// container items (the Vale Sewing Kit reads races=0, classes=0), and the recipe
// tables carry no race column either, so this small hand-maintained table is the
// only signal that a recipe is race-locked. Without it the leveling planner will
// happily recommend, say, a Halfling-only Leatherfoot Haversack to a Gnome.
//
// Scope: only kits that are strictly race-locked in classic EQ / Project Quarm
// are listed. Faction-gated Velious kits (Coldain Tanner's, Stormhealer's) are
// intentionally omitted — they're gated by faction, not race, so excluding them
// by race could hide recipes a character can actually make. Add entries here as
// more race-locked containers are confirmed (verify with the community before
// adding — a wrong entry silently hides valid recipes).
//
// EQ race ids: 3=Erudite, 4=Wood Elf, 5=High Elf, 6=Dark Elf, 8=Dwarf,
// 9=Troll, 10=Ogre, 11=Halfling, 12=Gnome. Keyed by the container's lowercased
// name with backticks stripped (quarm.db spells it both "Fier`Dal" and
// "Fierdal").
var culturalContainerRace = map[string]int{
	"vale sewing kit":       11, // Halfling  (Rivervale cultural tailoring)
	"fierdal sewing kit":    4,  // Wood Elf   (Kelethin cultural tailoring)
	"fierdal fletching kit": 4,  // Wood Elf   (Kelethin cultural fletching)
	"erudite sewing kit":    3,  // Erudite    (Erudin cultural tailoring)
}

// normalizeContainerName lowercases a container label and drops backticks so the
// two spellings of the Feir'Dal kits collapse to one key.
func normalizeContainerName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "`", ""))
}

// ContainerRaceRestrict returns the EQ race id required to use a combine
// container (a cultural kit), or 0 if any race can use it. Callers filter a
// character's plannable recipes with it: a recipe whose container returns a
// non-zero race the character isn't can't be made by that character.
func ContainerRaceRestrict(container string) int {
	if container == "" {
		return 0
	}
	return culturalContainerRace[normalizeContainerName(container)]
}

// containersRaceRestrict resolves a recipe's race restriction from its full set
// of combine containers (an OR set — any one suffices). A recipe is race-locked
// only when EVERY container is a cultural kit for the SAME race; if any vessel is
// open to all races, or the restricted kits span multiple races, anyone (or any
// of several races) can craft it, so it returns 0.
func containersRaceRestrict(containers []RecipeEntry) int {
	race := 0
	for _, c := range containers {
		r := ContainerRaceRestrict(c.ItemName)
		if r == 0 {
			return 0 // an unrestricted vessel exists — open to all races
		}
		if race == 0 {
			race = r
		} else if race != r {
			return 0 // restricted to different races — not a single-race lock
		}
	}
	return race
}

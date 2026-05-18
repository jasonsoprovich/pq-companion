package players

import "strings"

// ClassTitles maps a class's base name to every level-progression title it
// can be displayed as in /who output. Each list includes the base name plus
// the 51 / 55 / 60 / 65 titles. The filter expansion below uses these
// lists so "Enchanter" in the class dropdown matches Enchanters,
// Illusionists, Beguilers, and Phantasmists alike.
//
// Source: https://gaming.invisibill.net/eq/titles.html (verified 2026-05-17).
// Project Quarm is currently Velious-era and caps at level 60, so the 65
// titles are forward-compatible padding — they cost nothing to include and
// save a follow-up edit when Quarm progresses.
//
// Berserker (PoP-and-later) is not present in Quarm and is intentionally
// excluded; if it lands later, add an entry following the same shape.
var ClassTitles = map[string][]string{
	"Bard":         {"Bard", "Minstrel", "Troubador", "Virtuoso", "Maestro"},
	"Beastlord":    {"Beastlord", "Primalist", "Animist", "Savage Lord", "Feral Lord"},
	"Cleric":       {"Cleric", "Vicar", "Templar", "High Priest", "Archon"},
	"Druid":        {"Druid", "Wanderer", "Preserver", "Hierophant", "Storm Warden"},
	"Enchanter":    {"Enchanter", "Illusionist", "Beguiler", "Phantasmist", "Coercer"},
	"Magician":     {"Magician", "Elementalist", "Conjurer", "Arch Mage", "Arch Convoker"},
	"Monk":         {"Monk", "Disciple", "Master", "Grandmaster", "Transcendant"},
	"Necromancer":  {"Necromancer", "Heretic", "Defiler", "Warlock", "Arch Lich"},
	"Paladin":      {"Paladin", "Cavalier", "Knight", "Crusader", "Lord Protector"},
	"Ranger":       {"Ranger", "Pathfinder", "Outrider", "Warder", "Forest Stalker"},
	"Rogue":        {"Rogue", "Rake", "Blackguard", "Assassin", "Deceiver"},
	"Shadow Knight": {"Shadow Knight", "Reaver", "Revenant", "Grave Lord", "Dread Lord"},
	"Shaman":       {"Shaman", "Mystic", "Luminary", "Oracle", "Prophet"},
	"Warrior":      {"Warrior", "Champion", "Myrmidon", "Warlord", "Overlord"},
	"Wizard":       {"Wizard", "Channeler", "Evoker", "Sorcerer", "Arcanist"},
}

// classIndexNames maps the 0-indexed EQ class id (0=WAR … 14=BST) to its
// canonical base name as used elsewhere in this package and in the /who
// parser. Matches the index scheme described in config.Config.CharacterClass
// and zeal/watcher.go.
var classIndexNames = [...]string{
	"Warrior",       // 0
	"Cleric",        // 1
	"Paladin",       // 2
	"Ranger",        // 3
	"Shadow Knight", // 4
	"Druid",         // 5
	"Monk",          // 6
	"Bard",          // 7
	"Rogue",         // 8
	"Shaman",        // 9
	"Necromancer",   // 10
	"Wizard",        // 11
	"Magician",      // 12
	"Enchanter",     // 13
	"Beastlord",     // 14
}

// ClassNameByIndex returns the canonical base class name for an EQ class
// index, or "" when the index is out of range.
func ClassNameByIndex(idx int) string {
	if idx < 0 || idx >= len(classIndexNames) {
		return ""
	}
	return classIndexNames[idx]
}

// BaseClassOf returns the canonical base class name for any /who-style title
// string (e.g. "Illusionist" → "Enchanter", "Warlord" → "Warrior"). Empty
// input or an unknown title returns "".
func BaseClassOf(title string) string {
	if title == "" {
		return ""
	}
	for base, titles := range ClassTitles {
		for _, t := range titles {
			if strings.EqualFold(t, title) {
				return base
			}
		}
	}
	return ""
}

// expandClassFilter returns the list of class names a filter value should
// match. When the filter is a known base class, the result is every
// progression title for that class. When it's a single non-base value
// (e.g. "Illusionist") or unknown, the result is a single-element slice —
// preserving exact-match behaviour for direct title queries.
func expandClassFilter(filter string) []string {
	if filter == "" {
		return nil
	}
	for base, titles := range ClassTitles {
		if strings.EqualFold(base, filter) {
			return titles
		}
	}
	return []string{filter}
}

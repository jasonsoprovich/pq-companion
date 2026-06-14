package upgrade

// Archetype groups the 15 classes by gearing priority. The default weights are
// starting points only — the user edits them per character.
type Archetype string

const (
	ArchTank      Archetype = "tank"       // plate tanks: WAR/PAL/SHD
	ArchMelee     Archetype = "melee"      // MNK/ROG/BRD
	ArchHybrid    Archetype = "hybrid"     // RNG/BST — melee with some mana
	ArchWisCaster Archetype = "wis_caster" // priests: CLR/DRU/SHM
	ArchIntCaster Archetype = "int_caster" // NEC/WIZ/MAG/ENC
)

// Class indices are 0-indexed EQ class ids (0=Warrior … 14=Beastlord), matching
// the character store and eqstat.
const (
	classWarrior = iota
	classCleric
	classPaladin
	classRanger
	classShadowKnight
	classDruid
	classMonk
	classBard
	classRogue
	classShaman
	classNecromancer
	classWizard
	classMagician
	classEnchanter
	classBeastlord
)

// classArchetype maps a 0-indexed class to its default gearing archetype.
var classArchetype = map[int]Archetype{
	classWarrior:      ArchTank,
	classPaladin:      ArchTank,
	classShadowKnight: ArchTank,
	classMonk:         ArchMelee,
	classRogue:        ArchMelee,
	classBard:         ArchMelee,
	classRanger:       ArchHybrid,
	classBeastlord:    ArchHybrid,
	classCleric:       ArchWisCaster,
	classDruid:        ArchWisCaster,
	classShaman:       ArchWisCaster,
	classNecromancer:  ArchIntCaster,
	classWizard:       ArchIntCaster,
	classMagician:     ArchIntCaster,
	classEnchanter:    ArchIntCaster,
}

// ArchetypeFor returns the default archetype for a 0-indexed class, falling
// back to tank for an unknown/unset class.
func ArchetypeFor(class int) Archetype {
	if a, ok := classArchetype[class]; ok {
		return a
	}
	return ArchTank
}

// archetypeWeights holds the default weight set per archetype. The HP weight is
// pinned at 1.0 so every other weight reads as "HP-equivalent" — e.g. AC 5.0
// means the tank default treats 1 AC as 5 HP, the community baseline.
var archetypeWeights = map[Archetype]Weights{
	ArchTank: {
		HP: 1.0, Mana: 0, AC: 5.0,
		STR: 0.2, STA: 0.5, AGI: 0.2, DEX: 0.1, WIS: 0, INT: 0, CHA: 0,
		MR: 0.3, FR: 0.3, CR: 0.3, DR: 0.3, PR: 0.3,
		// Tanks want haste capped too; ATK matters for aggro/dps. A shield
		// offhand (high AC) still usually wins via the AC weight.
		ATK: 0.6, Haste: 10, DPS: 40,
	},
	ArchMelee: {
		HP: 0.6, Mana: 0, AC: 3.0,
		STR: 0.8, STA: 0.6, AGI: 0.4, DEX: 0.8, WIS: 0, INT: 0, CHA: 0,
		MR: 0.2, FR: 0.2, CR: 0.2, DR: 0.2, PR: 0.2,
		// Cap worn haste first (high weight, drops to 0 once capped), then ATK
		// and weapon ratio dominate.
		ATK: 1.0, Haste: 12, DPS: 250,
	},
	ArchHybrid: {
		HP: 0.7, Mana: 0.4, AC: 2.0,
		STR: 0.6, STA: 0.5, AGI: 0.3, DEX: 0.5, WIS: 0.3, INT: 0, CHA: 0,
		MR: 0.2, FR: 0.2, CR: 0.2, DR: 0.2, PR: 0.2,
		ATK: 0.8, Haste: 11, DPS: 180,
	},
	ArchWisCaster: {
		HP: 0.5, Mana: 1.0, AC: 1.0,
		STR: 0, STA: 0.2, AGI: 0, DEX: 0, WIS: 1.0, INT: 0, CHA: 0,
		MR: 0.2, FR: 0.2, CR: 0.2, DR: 0.2, PR: 0.2,
		ATK: 0, Haste: 0, DPS: 0,
	},
	ArchIntCaster: {
		HP: 0.4, Mana: 1.0, AC: 0.3,
		STR: 0, STA: 0.1, AGI: 0, DEX: 0, WIS: 0, INT: 1.0, CHA: 0,
		MR: 0.2, FR: 0.2, CR: 0.2, DR: 0.2, PR: 0.2,
		ATK: 0, Haste: 0, DPS: 0,
	},
}

// DefaultWeights returns the starting weight set for a 0-indexed class.
func DefaultWeights(class int) Weights {
	w := archetypeWeights[ArchetypeFor(class)]
	w.FocusBonus = DefaultFocusBonus
	return w
}

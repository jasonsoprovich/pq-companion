package enums

import "database/sql"

// Maps in this file label the spell-side enums surfaced by the database
// explorer's spell detail panel. Source for all five maps is EQEmu
// `common/spdat.h` (SE_*, ST_*, RESIST_*) — the codes are stable across
// EQMacEmu and modern EQEmu because spells_new is one of the few tables
// the two forks share verbatim.
//
// 254, 255, and 320 are "blank slot" sentinels used by EQEmu to terminate
// the 12-slot effect array — they must never render and are excluded
// from the audit.

// spellEffects maps a SPA (Spell Affecting Attribute) code from any of
// spells_new.effectid1..effectid12 to its display label.
//
// Source: EQMacEmu/Server common/spdat.h SE_* enum. The Quarm dump
// uses EQMacEmu numbering verbatim, which differs from modern EQEmu in
// several places (most notably codes 41/42 = Destroy/ShadowStep and a
// scattered set of codes in the 110–125 range). Labels here are
// humanized SE_* names, occasionally using the more recognizable EQ
// player term in parentheses or instead (e.g. "Memblur" for
// SE_WipeHateList).
//
// Quarm-specific codes (164, 165, 301, 500–504) are derived empirically
// from the spells that use them and may need refinement once an
// in-game tooltip is checked.
var spellEffects = map[int]string{
	0:   "Hitpoints",      // SE_CurrentHP — instant nuke/heal/DoT/HoT depending on duration
	1:   "AC",             // SE_ArmorClass
	2:   "ATK",            // SE_ATK
	3:   "Movement Speed", // SE_MovementSpeed
	4:   "STR",            // SE_STR
	5:   "DEX",            // SE_DEX
	6:   "AGI",            // SE_AGI
	7:   "STA",            // SE_STA
	8:   "INT",            // SE_INT
	9:   "WIS",            // SE_WIS
	10:  "CHA",            // SE_CHA
	11:  "Melee Haste",    // SE_AttackSpeed
	12:  "Invisibility",
	13:  "See Invisible",
	14:  "Enduring Breath", // SE_WaterBreathing
	15:  "Mana",            // SE_CurrentMana
	18:  "Lull",
	19:  "Faction", // SE_AddFaction
	20:  "Blind",
	21:  "Stun",
	22:  "Charm",
	23:  "Fear",
	24:  "Fatigue", // SE_Stamina
	25:  "Bind Affinity",
	26:  "Gate",
	27:  "Cancel Magic",
	28:  "Invis vs Undead",
	29:  "Invis vs Animals",
	30:  "Pacify", // SE_ChangeFrenzyRad
	31:  "Mez",
	32:  "Summon Item",
	33:  "Summon Pet",
	35:  "Disease Counter",
	36:  "Poison Counter",
	40:  "Divine Aura",
	41:  "Destroy",     // SE_Destroy — previously mislabeled as "Shadow Step"
	42:  "Shadow Step", // SE_ShadowStep — previously mislabeled as "Berserker Strength"
	43:  "Berserk",     // SE_Berserk
	44:  "Lycanthropy",
	45:  "Vampirism", // SE_Vampirism
	46:  "Resist Fire",
	47:  "Resist Cold",
	48:  "Resist Poison",
	49:  "Resist Disease",
	50:  "Resist Magic",
	52:  "Sense Undead", // SE_SenseDead
	53:  "Sense Summoned",
	54:  "Sense Animals",
	55:  "Stoneskin",  // SE_Rune — classic stoneskin absorption
	56:  "True North", // SE_TrueNorth
	57:  "Levitate",   // SE_Levitate — previously mislabeled as "True North"
	58:  "Illusion",   // SE_Illusion — previously mislabeled as "Levitate"
	59:  "Damage Shield",
	61:  "Identify",
	63:  "Memblur", // SE_WipeHateList
	64:  "Spin Stun",
	65:  "Infravision",
	66:  "Ultravision",
	67:  "Eye of Zomm",
	68:  "Reclaim Pet", // SE_ReclaimPet
	69:  "Max HP",      // SE_TotalHP
	71:  "Animate Dead",
	73:  "Bind Sight",
	74:  "Feign Death",
	75:  "Voice Graft",
	76:  "Sentinel",
	77:  "Locate Corpse",
	78:  "Absorb Magic Damage", // SE_AbsorbMagicAtt
	79:  "Current HP",          // SE_CurrentHPOnce — instant HP delta with no DoT/HoT tail
	81:  "Resurrect",           // SE_Revive
	82:  "Summon PC",           // SE_SummonPC
	83:  "Teleport",
	84:  "Gravity Flux", // SE_TossUp
	85:  "Add Proc",     // SE_WeaponProc
	86:  "Harmony",      // SE_Harmony
	87:  "Magnification",
	88:  "Evacuate", // SE_Succor
	89:  "Player Size",
	90:  "Cloak",
	91:  "Summon Corpse",
	92:  "Hate", // SE_InstantHate
	93:  "Stop Rain",
	94:  "Negate if Combat", // SE_NegateIfCombat — previously mislabeled as "Stop Rain"
	95:  "Sacrifice",
	96:  "Silence",
	97:  "Mana Pool",
	98:  "Bard Haste", // SE_AttackSpeed2
	99:  "Root",
	100: "Heal Over Time",
	101: "Complete Heal",
	102: "Pet Fearless",
	103: "Summon Pet",  // SE_CallPet
	104: "Translocate", // SE_Translocate
	105: "Anti-Gate",
	106: "Summon Beastlord Pet", // SE_SummonBSTPet
	107: "Alter NPC Level",
	108: "Summon Familiar", // SE_Familiar
	109: "Summon Item Into Bag",
	110: "Increase Archery",
	111: "Resistances", // SE_ResistAll
	112: "Casting Level",
	113: "Summon Mount",
	114: "Hate Generated", // SE_ChangeAggro
	115: "Hunger",         // SE_Hunger — previously mislabeled as "Cannibalize"
	116: "Curse Counter",  // SE_CurseCounter — previously mislabeled as "Crit Melee"
	117: "Magic Weapon",   // SE_MagicWeapon — previously mislabeled as "Crit Direct Damage"
	118: "Amplification",  // SE_Amplification — previously mislabeled as "Crippling Blow"
	119: "Melee Haste v2", // SE_AttackSpeed3
	120: "Healing Bonus",  // SE_HealRate
	121: "Reverse Damage Shield",
	123: "Screech",                // SE_Screech — previously mislabeled as "Reflect Spell"
	124: "Spell Damage Bonus",     // SE_ImprovedDamage
	125: "Healing Effectiveness",  // SE_ImprovedHeal
	126: "Spell Resist Reduction", // SE_SpellResistReduction
	127: "Spell Haste",            // SE_IncreaseSpellHaste
	128: "Spell Duration",         // SE_IncreaseSpellDuration
	129: "Spell Range",            // SE_IncreaseRange
	130: "Spell Hate",             // SE_SpellHateMod
	131: "Reagent Chance",         // SE_ReduceReagentCost
	132: "Mana Cost",              // SE_ReduceManaCost
	133: "Spell Stun Time Mod",    // SE_FcStunTimeMod
	134: "Limit: Max Level",
	135: "Limit: Resist",
	136: "Limit: Target",
	137: "Limit: Effect",
	138: "Limit: Spell Type",
	139: "Limit: Spell",
	140: "Limit: Min Duration",
	141: "Limit: Instant Only",
	142: "Limit: Min Level",
	143: "Limit: Min Cast Time",
	144: "Limit: Max Cast Time",
	145: "Teleport",
	147: "Percent Heal", // SE_PercentalHeal
	148: "Stacking Block",
	149: "Stacking Override",
	150: "Death Save",
	151: "Suspend Pet",
	152: "Temporary Pet",
	153: "Balance Group HP",
	154: "Dispel Detrimental",
	155: "Spell Crit Damage", // SE_SpellCritDmgIncrease
	156: "Illusion Copy",
	157: "Spell Damage Shield",
	158: "Reflect Spell", // SE_Reflect
	159: "All Stats",
	160: "Make Drunk", // Modern EQEmu SE_MakeDrunk; EQMacEmu spdat.h omits this code but Quarm uses it on Swiftness/Fleetness/Nimbleness — label may need refinement
	161: "Magic Rune",         // SE_MitigateSpellDamage — previously labeled "Rune" (swapped with 162)
	162: "Rune",               // SE_MitigateMeleeDamage — previously labeled "Magic Rune" (swapped with 161)
	163: "Negate Attacks",
	164: "Kick Damage Bonus",  // Quarm-specific; powers Power Kick / Savage Kick
	165: "Bash Damage Bonus",  // Quarm-specific; powers Power Bash / Savage Bash
	167: "Pet Power",
	168: "Melee Mitigation",
	169: "Crit Hit Chance",
	170: "Spell Crit Chance",
	171: "Crippling Blow Chance",
	172: "Avoidance",
	173: "Riposte Chance",
	174: "Dodge Chance",
	175: "Parry Chance",
	176: "Dual Wield Chance",
	177: "Double Attack Chance",
	178: "Melee Lifetap",
	179: "Instrument Modifier",
	180: "Resist Spell Chance",
	181: "Resist Fear",
	182: "Hundred Hands",
	183: "Melee Skill Check",
	184: "Hit Chance",
	185: "Damage Modifier",
	186: "Min Damage Modifier",
	189: "Endurance",
	190: "Endurance Pool",
	191: "Amnesia",
	192: "Hate Override", // SE_Hate
	193: "Skill Attack",
	194: "Fading Memories",
	195: "Stun Resist",
	196: "Strikethrough",
	197: "Skill Damage Taken",
	198: "Endurance (instant)",
	199: "Taunt",
	200: "Proc Chance",
	201: "Ranged Proc",
	204: "Group Fear Immunity",
	205: "Rampage",
	206: "AE Taunt",
	208: "Cure Poison",
	209: "Dispel Beneficial",
	210: "Pet Shield",
	211: "AE Melee",
	214: "Max HP %",
	215: "Pet Avoidance",
	216: "Accuracy",
	217: "Headshot",
	218: "Pet Crit Chance",
	219: "Slay Undead",
	220: "Skill Damage",
	301: "Archery Damage Modifier", // SE_ArcheryDamageModifier — powers Trueshot Discipline
	500: "Quarm SPA 500",           // Quarm-specific; needs verification — appears on Maelin's Magical Concoction
	501: "Quarm SPA 501",           // Quarm-specific; needs verification
	503: "Quarm SPA 503",           // Quarm-specific; needs verification
	504: "Quarm SPA 504",           // Quarm-specific; needs verification
}

// SpellEffectName returns the label for a SPA code, or empty for the
// blank-slot sentinels. Unknown codes return "" so the caller can fall
// back to its own format (e.g. "Effect N").
func SpellEffectName(id int) string {
	if id == 254 || id == 255 || id == 320 {
		return ""
	}
	return spellEffects[id]
}

// SpellEffectsAudit validates that every distinct SPA seen across all
// twelve effectidN columns is mapped above. Sentinels 254/255/320 and
// the synthetic -1 sometimes emitted by the converter are excluded.
var SpellEffectsAudit = AuditDef{
	Name:       "Spell Effect (SPA)",
	KnownCodes: keysAsSet(spellEffects),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`
			SELECT DISTINCT n FROM (
				SELECT effectid1 AS n FROM spells_new UNION
				SELECT effectid2 FROM spells_new UNION
				SELECT effectid3 FROM spells_new UNION
				SELECT effectid4 FROM spells_new UNION
				SELECT effectid5 FROM spells_new UNION
				SELECT effectid6 FROM spells_new UNION
				SELECT effectid7 FROM spells_new UNION
				SELECT effectid8 FROM spells_new UNION
				SELECT effectid9 FROM spells_new UNION
				SELECT effectid10 FROM spells_new UNION
				SELECT effectid11 FROM spells_new UNION
				SELECT effectid12 FROM spells_new
			) WHERE n NOT IN (254, 255, 320, -1)`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}

// spellResistTypes maps spells_new.resisttype to its element / save name.
//
// Source: EQEmu `common/spdat.h` `RESIST_*` enum.
var spellResistTypes = map[int]string{
	0: "Unresistable",
	1: "Magic",
	2: "Fire",
	3: "Cold",
	4: "Poison",
	5: "Disease",
	6: "Chromatic",
	7: "Prismatic",
	8: "Physical",
	9: "Corruption",
}

// SpellResistName returns the resist-type label, or "" for unknown.
func SpellResistName(id int) string {
	return spellResistTypes[id]
}

// SpellResistsAudit validates every distinct spells_new.resisttype.
var SpellResistsAudit = AuditDef{
	Name:       "Spell Resist",
	KnownCodes: keysAsSet(spellResistTypes),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT resisttype FROM spells_new`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}

// spellTargetTypes maps spells_new.targettype to its targeting-mode label.
//
// Source: EQEmu `common/spdat.h` `ST_*` enum. Covers every value
// observed in the live Quarm dump plus several upstream-only codes
// for forward compatibility.
var spellTargetTypes = map[int]string{
	0:  "No Target",
	1:  "Line of Sight",
	2:  "Caster Group",
	3:  "Directional AE",
	4:  "Single (Pet)",
	5:  "Single",
	6:  "Self",
	8:  "Targeted AE",
	9:  "Animal",
	10: "Corpse",
	11: "Plant",
	12: "Undead",
	13: "Summoned",
	14: "Tap (Single)",
	15: "PB AE",
	16: "AE Line of Sight",
	17: "Hate List",
	18: "AE Undead",
	20: "Targeted AE Tap",
	24: "Full Zone",
	25: "Group v2",
	36: "Directional AE v2",
	40: "Group Mercenary",
	41: "AE Pet",
	42: "Group (Target)",
	43: "Group with Pets",
	50: "AE (No PC)",
}

// SpellTargetName returns the target-type label, or "" for unknown.
func SpellTargetName(id int) string {
	return spellTargetTypes[id]
}

// SpellTargetsAudit validates every distinct spells_new.targettype.
var SpellTargetsAudit = AuditDef{
	Name:       "Spell Target",
	KnownCodes: keysAsSet(spellTargetTypes),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT targettype FROM spells_new`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}

// spellSkills maps spells_new.skill to its spellbook school / instrument
// label. Bard "skills" reuse this column to mark which instrument the
// song scales off of (Brass/Wind/Stringed/Percussion/Singing).
//
// Source: EQEmu `common/skills.h` `EQ::skills::*Skill*` enum.
var spellSkills = map[int]string{
	4:  "Abjuration",
	5:  "Alteration",
	12: "Percussion Instruments",
	14: "Conjuration",
	15: "Discipline",
	18: "Divination",
	24: "Evocation",
	33: "Discipline",
	41: "Brass Instruments",
	49: "Singing",
	52: "Channeling",
	54: "Stringed Instruments",
	70: "Wind Instruments",
}

// SpellSkillName returns the spell-school label, or "" for unknown /
// "no school". Callers that show this as a row generally hide the row
// entirely when this returns empty.
func SpellSkillName(id int) string {
	return spellSkills[id]
}

// SpellSkillsAudit validates every distinct spells_new.skill.
var SpellSkillsAudit = AuditDef{
	Name:       "Spell Skill",
	KnownCodes: keysAsSet(spellSkills),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT skill FROM spells_new`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}

// spellTypeFilter maps the SPA 138 base value to its allowed-spell-class
// label (the filter that a focus / limit slot uses to narrow which
// spells it applies to).
//
// Source: EQEmu `common/spdat.h` — buffmod.SpellType{Detrimental,
// Beneficial,Any} constants mirror these values for backend use.
var spellTypeFilter = map[int]string{
	0: "Detrimental only",
	1: "Beneficial only",
	2: "Beneficial - Group Only",
}

// SpellTypeFilterName returns the filter label, or "" for unknown.
func SpellTypeFilterName(id int) string {
	return spellTypeFilter[id]
}

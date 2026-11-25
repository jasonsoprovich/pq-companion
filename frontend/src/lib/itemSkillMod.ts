// Item skill modifiers: +N% to one skill while worn (items.skillmodtype /
// skillmodvalue). Only the best mod per skill across worn gear applies, and
// the raised skill is capped at 252 (EQMacEmu bonuses.cpp, client.cpp).

// EQMacEmu skill ids (common/skills.h, the order of GetSkillTypeMap).
const SKILL_NAMES = [
  '1H Blunt', '1H Slashing', '2H Blunt', '2H Slashing', 'Abjuration',
  'Alteration', 'Apply Poison', 'Archery', 'Backstab', 'Bind Wound', 'Bash',
  'Block', 'Brass Instruments', 'Channeling', 'Conjuration', 'Defense',
  'Disarm', 'Disarm Traps', 'Divination', 'Dodge', 'Double Attack',
  'Dragon Punch', 'Dual Wield', 'Eagle Strike', 'Evocation', 'Feign Death',
  'Flying Kick', 'Forage', 'Hand to Hand', 'Hide', 'Kick', 'Meditate', 'Mend',
  'Offense', 'Parry', 'Pick Lock', '1H Piercing', 'Riposte', 'Round Kick',
  'Safe Fall', 'Sense Heading', 'Singing', 'Sneak', 'Specialize Abjuration',
  'Specialize Alteration', 'Specialize Conjuration', 'Specialize Divination',
  'Specialize Evocation', 'Pick Pockets', 'Stringed Instruments', 'Swimming',
  'Throwing', 'Tiger Claw', 'Tracking', 'Wind Instruments', 'Fishing',
  'Make Poison', 'Tinkering', 'Research', 'Alchemy', 'Baking', 'Tailoring',
  'Sense Traps', 'Blacksmithing', 'Fletching', 'Brewing', 'Alcohol Tolerance',
  'Begging', 'Jewelry Making', 'Pottery', 'Percussion Instruments',
  'Intimidation', 'Berserking', 'Taunt',
]

export const SKILL_MOD_HINT =
  'Raises this skill by the percent while worn (only your best mod for a ' +
  'skill counts; capped at 252).'

export interface SkillMod {
  skill: string
  value: number
}

// skillModOf returns the item's skill mod, or null when it has none. Read
// defensively so an older backend without the fields yields null.
export function skillModOf(item: {
  skill_mod_type?: number
  skill_mod_value?: number
}): SkillMod | null {
  const value = item.skill_mod_value ?? 0
  if (value === 0) return null
  const type = item.skill_mod_type ?? 0
  return { skill: SKILL_NAMES[type] ?? `Skill ${type}`, value }
}

// skillModText renders "Backstab +12%".
export function skillModText(m: SkillMod): string {
  return `${m.skill} ${m.value > 0 ? '+' : ''}${m.value}%`
}

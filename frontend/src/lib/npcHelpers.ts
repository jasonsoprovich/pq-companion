import type { NPC } from '../types/npc'

// ── Name formatting ─────────────────────────────────────────────────────────────

// EQEmu stores NPC names with underscores instead of spaces.
function formatEQName(name: string): string {
  return name.replace(/_/g, ' ').trim()
}

export function npcDisplayName(npc: NPC): string {
  const name = formatEQName(npc.name)
  const last = formatEQName(npc.last_name)
  return last ? `${name} ${last}` : name
}

// ── Class ───────────────────────────────────────────────────────────────────────

const CLASS_NAMES: Record<number, string> = {
  1: 'Warrior',
  2: 'Cleric',
  3: 'Paladin',
  4: 'Ranger',
  5: 'Shadow Knight',
  6: 'Druid',
  7: 'Monk',
  8: 'Bard',
  9: 'Rogue',
  10: 'Shaman',
  11: 'Necromancer',
  12: 'Wizard',
  13: 'Magician',
  14: 'Enchanter',
  15: 'Beastlord',
  16: 'Berserker',
}

export function className(classId: number): string {
  return CLASS_NAMES[classId] ?? `Class ${classId}`
}

// ── Race ────────────────────────────────────────────────────────────────────────

const RACE_NAMES: Record<number, string> = {
  1: 'Human',
  2: 'Barbarian',
  3: 'Erudite',
  4: 'Wood Elf',
  5: 'High Elf',
  6: 'Dark Elf',
  7: 'Half Elf',
  8: 'Dwarf',
  9: 'Troll',
  10: 'Ogre',
  11: 'Halfling',
  12: 'Gnome',
  13: 'Iksar',
  14: 'Vah Shir',
  15: 'Froglok',
  16: 'Drakkin',
  42: 'Goblin',
  44: 'Kobold',
  45: 'Leprechaun',
  46: 'Lizard Man',
  47: 'Minotaur',
  48: 'Orc',
  49: 'Hill Giant',
  50: 'Bone Skeleton',
  51: 'Shark',
  52: 'Zombie',
  54: 'Clam',
  55: 'Piranha',
  56: 'Efreeti',
  60: 'Cyclops',
  62: 'Werewolf',
  65: 'Drake',
  66: 'Golem',
  67: 'Storm Giant',
  68: 'Basilisk',
  69: 'Dragon',
  70: 'Innoruuk',
  71: 'Unicorn',
  72: 'Pegasus',
  73: 'Djinn',
  74: 'Invisible Man',
  75: 'Iksar',
  76: 'Scorpion',
  77: 'Vah Shir',
  78: 'Sarnak',
  79: 'Draglock',
  80: 'Lycanthrope',
  81: 'Mosquito',
  82: 'Rhino',
  83: 'Xalgoz',
  84: 'Kunark Goblin',
  85: 'Yeti',
  86: 'Iksar',
  87: 'Giant Skeleton',
  88: 'Icebone Skeleton',
  89: 'Ratman',
  90: 'Wyvern',
  91: 'Wurm',
  92: 'Devourer',
  93: 'Iksar Golem',
  94: 'Iksar Skeleton',
  95: 'Man Eating Plant',
  96: 'Raptor',
  97: 'Sarnak Spirit',
  98: 'Sarnak Skeleton',
  99: 'Scorpikis',
  100: 'Sebilite Crocodile',
  101: 'Seblyte Gargoyle',
  102: 'Seblyte Golem',
  103: 'Shik\'Nar',
  104: 'Rockhopper',
  105: 'Underbulk',
  106: 'Grachnist the Destroyer',
  107: 'Moss Snake',
  108: 'Burynai',
  109: 'Goo',
  110: 'Sarnak Berserker',
  111: 'Sarnak Collective',
  112: 'Sarnak Spirit',
  113: 'Sarnak Mind Worm',
  114: 'Kunark Zombie',
  115: 'Cazic Golem',
  116: 'Cazic Thule',
  117: 'Aghar',
  118: 'Battle Skeleton',
  119: 'Caiman',
  120: 'Taveeshi',
  121: 'Tortoise',
  122: 'Festering Hag',
  123: 'Diced',
  124: 'Shissar',
  125: 'Fungal Fiend',
  126: 'Vampire Bat',
  127: 'Akheva',
  128: 'Sonic Wolf',
  129: 'Ground Shaker',
  130: 'Vah Shir',
}

export function raceName(raceId: number): string {
  return RACE_NAMES[raceId] ?? `Race ${raceId}`
}

// ── Body Type ───────────────────────────────────────────────────────────────────

const BODY_TYPES: Record<number, string> = {
  0: 'Humanoid',
  1: 'Biped',
  2: 'Giant',
  3: 'Animal',
  4: 'Insect',
  5: 'Undead',
  6: 'Construct',
  7: 'Extraplanar',
  8: 'Magical',
  9: 'Summoned Undead',
  10: 'Slime',
  11: 'Plant',
  12: 'Dragon',
  14: 'Akheva',
  21: 'Untargetable',
  23: 'Muramite',
  25: 'Swarmcreature',
  28: 'Humanoid 2',
  33: 'Invulnerable',
  34: 'No Corpse',
  66: 'Regenerating',
  67: 'Trap',
  100: 'Invisible Man',
}

export function bodyTypeName(bodyType: number): string {
  return BODY_TYPES[bodyType] ?? `Type ${bodyType}`
}

// ── Special Abilities ───────────────────────────────────────────────────────────

export interface SpecialAbility {
  code: number
  value: number
  name: string
  description: string
}

const SPECIAL_ABILITIES: Record<number, { name: string; description: string }> = {
  1:  { name: 'Summon',                    description: 'Will summon players who run out of melee range.' },
  2:  { name: 'Enrage',                    description: 'Randomly enrages at low HP, attacking all nearby players for a short duration.' },
  3:  { name: 'Rampage',                   description: 'Randomly hits nearby players instead of only the current target.' },
  4:  { name: 'Flurry',                    description: 'Can strike multiple times in rapid succession on a single attack round.' },
  5:  { name: 'Triple Attack',             description: 'Attacks three times per combat round.' },
  6:  { name: 'Dual Wield',                description: 'Attacks with two weapons simultaneously.' },
  7:  { name: 'Bane Damage',               description: 'Weapons below a certain material type do reduced or no damage.' },
  10: { name: 'Unflinching',               description: 'Does not flee at low HP.' },
  12: { name: 'Immune to Melee',           description: 'Cannot be damaged by melee attacks.' },
  13: { name: 'Immune to Magic',           description: 'Immune to magic-based spells and effects.' },
  14: { name: 'Immune to Fire',            description: 'Takes no damage from fire-based spells.' },
  15: { name: 'Immune to Cold',            description: 'Takes no damage from cold-based spells.' },
  16: { name: 'Immune to Poison',          description: 'Immune to poison-based spells and effects.' },
  17: { name: 'Uncharmable',               description: 'Cannot be charmed by any spell or effect.' },
  18: { name: 'Unmezzable',                description: 'Cannot be mesmerized or sleep-stunned.' },
  19: { name: 'Unfearable',                description: 'Cannot be feared or made to flee.' },
  20: { name: 'Immune to Slow',            description: 'Cannot be slowed by any spell or effect.' },
  21: { name: 'Immune to Stun',            description: 'Cannot be stunned.' },
  23: { name: 'Immune to Fleeing',         description: 'Does not flee when health drops low.' },
  24: { name: 'No Target',                 description: 'Cannot be targeted directly by players.' },
  26: { name: 'See Through Invis',         description: 'Can see invisible players and pets.' },
  28: { name: 'See Through Invis vs Undead', description: 'Can see players hidden with Invisibility vs. Undead.' },
  31: { name: 'Immune to Dispel',          description: 'Buffs and effects cannot be removed by dispel spells.' },
  35: { name: 'Immune to NPC Aggro',       description: 'Will not be attacked by other NPCs.' },
  42: { name: 'Destructible Object',       description: 'Can be destroyed by player damage (e.g. doors, crates).' },
  43: { name: 'Immune to Lull',            description: 'Cannot be pacified or lulled by any spell or effect.' },
}

export function parseSpecialAbilities(raw: string): SpecialAbility[] {
  if (!raw || raw.trim() === '') return []
  return raw
    .split('^')
    .map((part) => part.trim())
    .filter(Boolean)
    .flatMap((part) => {
      const [codeStr, valueStr] = part.split(',')
      const code = parseInt(codeStr ?? '', 10)
      const value = parseInt(valueStr ?? '0', 10)
      if (isNaN(code) || isNaN(value)) return []
      const entry = SPECIAL_ABILITIES[code]
      return [{
        code,
        value,
        name: entry?.name ?? `Ability ${code}`,
        description: entry?.description ?? '',
      }]
    })
    .filter((sa) => sa.value !== 0)
}

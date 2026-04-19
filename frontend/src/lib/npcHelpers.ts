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
}

const SPECIAL_ABILITY_NAMES: Record<number, string> = {
  1: 'Summon',
  2: 'Enrage',
  3: 'Rampage',
  4: 'Flurry',
  5: 'Triple Attack',
  6: 'Dual Wield',
  7: 'Bane Damage',
  10: 'Unflinching',
  12: 'Immune to Melee',
  13: 'Immune to Magic',
  14: 'Immune to Fire',
  15: 'Immune to Cold',
  16: 'Immune to Poison',
  17: 'Uncharmable',
  18: 'Unmezzable',
  19: 'Unfearable',
  20: 'Immune to Slow',
  21: 'Immune to Stun',
  23: 'Immune to Fleeing',
  24: 'No Target',
  26: 'See Through Invis',
  28: 'See Through Invis vs Undead',
  31: 'Immune to Dispel',
  35: 'Immune to NPC Aggro',
  42: 'Destructible Object',
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
      return [{ code, value, name: SPECIAL_ABILITY_NAMES[code] ?? `Ability ${code}` }]
    })
    .filter((sa) => sa.value !== 0)
}

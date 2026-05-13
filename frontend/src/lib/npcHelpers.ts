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

// Class IDs 1–16 are the PC classes. 20–34 are the corresponding "GM"
// trainer variants (offset of +19). 40+ cover the NPC service roles
// (banker, merchant, etc.) using the standard EQEmu class.h numbering.
const CLASS_NAMES: Record<number, string> = {
  0: 'Unknown',
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
  20: 'Warrior GM',
  21: 'Cleric GM',
  22: 'Paladin GM',
  23: 'Ranger GM',
  24: 'Shadow Knight GM',
  25: 'Druid GM',
  26: 'Monk GM',
  27: 'Bard GM',
  28: 'Rogue GM',
  29: 'Shaman GM',
  30: 'Necromancer GM',
  31: 'Wizard GM',
  32: 'Magician GM',
  33: 'Enchanter GM',
  34: 'Beastlord GM',
  35: 'Berserker GM',
  40: 'Banker',
  41: 'Merchant',
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

// Synthetic ability codes used for flags stored on dedicated NPC columns
// rather than encoded in the special_abilities string. Above
// SpecialAbility::Max (55) so they never collide with a real Quarm code.
export const SYN_SEE_INVIS = 1001
export const SYN_SEE_INVIS_UNDEAD = 1002

// Canonical mapping from Project Quarm's EQMacEmu fork's `SpecialAbility`
// namespace (`common/emu_constants.h`). These match the codes the server
// itself uses when reading `npc_types.special_abilities`, and differ from
// modern EQEmu master numbering.
const SPECIAL_ABILITIES: Record<number, { name: string; description: string }> = {
  1:  { name: 'Summon',                       description: 'Will summon players who run out of melee range.' },
  2:  { name: 'Enrage',                       description: 'Randomly enrages at low HP, attacking all nearby players for a short duration.' },
  3:  { name: 'Rampage',                      description: 'Randomly hits nearby players instead of only the current target.' },
  4:  { name: 'Area Rampage',                 description: 'Hits every player within melee range on each rampage tick.' },
  5:  { name: 'Flurry',                       description: 'Can strike multiple times in rapid succession on a single attack round.' },
  6:  { name: 'Triple Attack',                description: 'Attacks three times per combat round.' },
  7:  { name: 'Dual Wield',                   description: 'Attacks with two weapons simultaneously.' },
  8:  { name: 'Disallow Equip',               description: 'Cannot equip items in this slot.' },
  9:  { name: 'Bane Attack',                  description: "NPC's melee counts as bane damage." },
  10: { name: 'Magical Attack',               description: "NPC's melee counts as magical and bypasses immune-to-melee." },
  11: { name: 'Ranged Attack',                description: 'Performs ranged attacks (bow/throwing) at distance.' },
  12: { name: 'Immune to Slow',               description: 'Cannot be slowed by any spell or effect.' },
  13: { name: 'Immune to Mez',                description: 'Cannot be mesmerized.' },
  14: { name: 'Immune to Charm',              description: 'Cannot be charmed by any spell or effect.' },
  15: { name: 'Immune to Stun',               description: 'Cannot be stunned.' },
  16: { name: 'Immune to Snare',              description: 'Cannot be snared or rooted.' },
  17: { name: 'Immune to Fear',               description: 'Cannot be feared.' },
  18: { name: 'Immune to Dispel',             description: 'Buffs and effects cannot be removed by dispel spells.' },
  19: { name: 'Immune to Melee',              description: 'Cannot be damaged by ordinary melee attacks.' },
  20: { name: 'Immune to Magic',              description: 'Immune to all spell damage.' },
  21: { name: 'Immune to Fleeing',            description: 'Does not flee when health drops low.' },
  22: { name: 'Immune to Non-Bane Melee',     description: 'Only takes damage from melee weapons with bane damage.' },
  23: { name: 'Immune to Non-Magical Melee',  description: 'Only takes damage from magical melee weapons.' },
  24: { name: 'Immune to Aggro',              description: 'Cannot generate aggro on other NPCs.' },
  25: { name: "Immune to Being Aggro'd",      description: 'Other NPCs cannot generate aggro on this mob.' },
  26: { name: 'Immune to Ranged Spells',      description: 'Spells cast from outside melee range have no effect.' },
  27: { name: 'Immune to Feign Death',        description: 'Will not be fooled by Feign Death.' },
  28: { name: 'Immune to Taunt',              description: 'Cannot be taunted off its current target.' },
  29: { name: 'Tunnel Vision',                description: 'Sticks to its current target until it dies or zones.' },
  30: { name: "Won't Heal/Buff Allies",       description: 'Will not heal or buff other NPCs in its faction.' },
  31: { name: 'Immune to Pacify',             description: 'Cannot be pacified or lulled.' },
  32: { name: 'Leashed',                      description: 'Returns to spawn point if pulled too far.' },
  33: { name: 'Tethered',                     description: 'Resets to full HP if pulled out of its tether range.' },
  34: { name: 'Permaroot Flee',               description: 'Flees in place when low HP — does not move.' },
  35: { name: 'Immune to Harm from Client',   description: 'Players cannot damage this NPC directly.' },
  36: { name: 'Always Flees',                 description: 'Always tries to flee, regardless of HP.' },
  37: { name: 'Flee Percent',                 description: 'Flees at a custom HP percentage.' },
  38: { name: 'Allows Beneficial Spells',     description: 'Will accept beneficial spells from players.' },
  39: { name: 'Melee Disabled',               description: 'Will not perform melee attacks.' },
  40: { name: 'Chase Distance',               description: 'Custom maximum chase distance from spawn.' },
  41: { name: 'Allowed to Tank',              description: 'Can be the primary target for charmed pets/swarm pets.' },
  42: { name: 'Proximity Aggro',              description: 'Aggros on any player entering its proximity, regardless of faction.' },
  43: { name: 'Always Calls for Help',        description: 'Always calls nearby allies into combat.' },
  44: { name: 'Use Warrior Skills',           description: 'Performs warrior-class melee specials regardless of NPC class.' },
  45: { name: 'Always Flee on Low Con',       description: 'Always flees from gray-con players.' },
  46: { name: 'No Loitering',                 description: 'Does not loiter — returns to spawn or despawns immediately.' },
  47: { name: 'Block Handin on Bad Faction',  description: 'Refuses quest hand-ins from players with bad faction.' },
  48: { name: 'PC Deathblow Corpse',          description: 'Corpse can be deathblown for the killing PC.' },
  49: { name: 'Corpse Camper',                description: 'Lingers near corpses after kills.' },
  50: { name: 'Reverse Slow',                 description: 'Slows applied to this NPC instead haste it.' },
  51: { name: 'Immune to Haste',              description: 'Cannot be hasted.' },
  52: { name: 'Immune to Disarm',             description: 'Cannot be disarmed.' },
  53: { name: 'Immune to Riposte',            description: 'Melee attacks against this NPC cannot be riposted.' },
  54: { name: 'Proximity Aggro 2',            description: 'Secondary proximity-aggro variant.' },
  [SYN_SEE_INVIS]:        { name: 'See Invis',            description: 'Can see invisible players and pets.' },
  [SYN_SEE_INVIS_UNDEAD]: { name: 'See Invis vs Undead',  description: 'Can see players hidden with Invisibility vs. Undead.' },
}

/**
 * Log-line patterns to alert on when an NPC uses a special ability. Only codes
 * with a well-known trigger message are listed here — for the rest, the user
 * can write a custom pattern. Patterns intentionally match any attacker so the
 * trigger works regardless of which specific mob uses the ability.
 */
export const SPECIAL_ABILITY_ALERT_PATTERNS: Record<number, { pattern: string; text: string }> = {
  1: { pattern: `You have been summoned!`, text: 'SUMMONED!' },
  2: { pattern: `\\w+ has become ENRAGED\\.`, text: 'ENRAGED!' },
  3: { pattern: `\\w+ goes on a rampage!`, text: 'RAMPAGE!' },
  4: { pattern: `\\w+ goes on a rampage!`, text: 'AREA RAMPAGE!' },
  5: { pattern: `\\w+ flurries a strike!`, text: 'FLURRY!' },
}

export function specialAbilityAlertPattern(code: number): { pattern: string; text: string } | null {
  return SPECIAL_ABILITY_ALERT_PATTERNS[code] ?? null
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

// Returns the parsed special-ability list for an NPC, merged with synthetic
// entries derived from the dedicated see_invis / see_invis_undead columns.
// Those columns are the authoritative source for see-invis flags in Quarm —
// the special_abilities string doesn't encode them at all.
export function npcSpecialAbilities(npc: NPC): SpecialAbility[] {
  const abilities = parseSpecialAbilities(npc.special_abilities)
  const ensure = (code: number) => {
    if (abilities.some((a) => a.code === code)) return
    const entry = SPECIAL_ABILITIES[code]
    abilities.push({
      code,
      value: 1,
      name: entry?.name ?? `Ability ${code}`,
      description: entry?.description ?? '',
    })
  }
  if (npc.see_invis) ensure(SYN_SEE_INVIS)
  if (npc.see_invis_undead) ensure(SYN_SEE_INVIS_UNDEAD)
  return abilities
}

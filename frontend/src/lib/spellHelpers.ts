// ── Class names ────────────────────────────────────────────────────────────────

const CLASS_NAMES = [
  'WAR', 'CLR', 'PAL', 'RNG', 'SHD', 'DRU', 'MNK',
  'BRD', 'ROG', 'SHM', 'NEC', 'WIZ', 'MAG', 'ENC', 'BST',
]

const CLASS_FULL_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

/** Returns array of {abbr, full, level} for every class that can cast the spell. */
export function castableClasses(classLevels: number[]): Array<{ abbr: string; full: string; level: number }> {
  return classLevels
    .map((lvl, i) => ({ abbr: CLASS_NAMES[i] ?? `C${i}`, full: CLASS_FULL_NAMES[i] ?? `Class ${i}`, level: lvl }))
    .filter((c) => c.level < 255)
    .sort((a, b) => a.level - b.level)
}

/** Short label for the list row (first 4 castable classes). */
export function castableClassesShort(classLevels: number[]): string {
  const classes = castableClasses(classLevels)
  if (classes.length === 0) return 'No Class'
  const shown = classes.slice(0, 4).map((c) => `${c.abbr} ${c.level}`)
  return shown.join(' · ') + (classes.length > 4 ? ` +${classes.length - 4}` : '')
}

// ── Resist type ────────────────────────────────────────────────────────────────

const RESIST_LABELS: Record<number, string> = {
  0: 'Unresistable',
  1: 'Magic',
  2: 'Fire',
  3: 'Cold',
  4: 'Poison',
  5: 'Disease',
  6: 'Chromatic',
  7: 'Prismatic',
  8: 'Physical',
  9: 'Corruption',
}

export function resistLabel(r: number): string {
  return RESIST_LABELS[r] ?? `Unknown (${r})`
}

// ── Target type ────────────────────────────────────────────────────────────────

const TARGET_LABELS: Record<number, string> = {
  1: 'Line of Sight',
  2: 'Caster Group',
  3: 'Directional AE',
  4: 'Single (Pet)',
  5: 'Single',
  6: 'Self',
  8: 'Targeted AE',
  10: 'Corpse',
  11: 'Plant',
  12: 'Undead',
  13: 'Summoned',
  14: 'Tap (Single)',
  15: 'PB AE',
  16: 'AE Line of Sight',
  18: 'AE Undead',
  20: 'Targeted AE Tap',
  24: 'Full Zone',
  25: 'Group v2',
  36: 'Directional AE v2',
  40: 'Group Mercenary',
  41: 'AE Pet',
  42: 'Group (Target)',
  43: 'Group with Pets',
  50: 'AE (No PC)',
}

export function targetLabel(t: number): string {
  return TARGET_LABELS[t] ?? `Unknown (${t})`
}

// ── Spell skill (school) ───────────────────────────────────────────────────────

const SKILL_LABELS: Record<number, string> = {
  4: 'Abjuration',
  5: 'Alteration',
  12: 'Percussion Instruments',
  14: 'Conjuration',
  15: 'Discipline',
  18: 'Divination',
  24: 'Evocation',
  33: 'Discipline',
  41: 'Brass Instruments',
  49: 'Singing',
  52: 'Channeling',
  54: 'Stringed Instruments',
  70: 'Wind Instruments',
}

export function skillLabel(s: number): string {
  return SKILL_LABELS[s] ?? ''
}

// ── Timing helpers ─────────────────────────────────────────────────────────────

/** Convert milliseconds to a human-readable string (e.g. "2.5s"). */
export function msLabel(ms: number): string {
  if (ms <= 0) return 'Instant'
  const s = ms / 1000
  return s === Math.floor(s) ? `${s}s` : `${s.toFixed(1)}s`
}

/** Convert buff duration ticks to a human-readable string (1 tick = 6 seconds). */
export function durationLabel(formula: number, ticks: number): string {
  if (ticks === 0) return 'Instant'
  if (ticks >= 50000) return 'Permanent'
  const seconds = ticks * 6
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  const timeStr = mins > 0
    ? (secs > 0 ? `${mins}m ${secs}s` : `${mins}m`)
    : `${secs}s`
  // Formula 0 = fixed; anything else = scales with caster level up to max
  return formula === 0 ? timeStr : `up to ${timeStr}`
}

// ── Effect ID labels ───────────────────────────────────────────────────────────

const EFFECT_LABELS: Record<number, string> = {
  0: 'Blank',
  1: 'HP Bonus',
  2: 'AC Bonus',
  3: 'ATK Bonus',
  4: 'Movement Speed',
  5: 'STR',
  6: 'DEX',
  7: 'AGI',
  8: 'STA',
  9: 'WIS',
  10: 'INT',
  11: 'CHA',
  12: 'WIS (Cap)',
  13: 'INT (Cap)',
  14: 'Mana Bonus',
  15: 'HP Regen',
  16: 'Mana Regen',
  17: 'Magic Resist',
  18: 'Fire Resist',
  19: 'Cold Resist',
  20: 'Poison Resist',
  21: 'Disease Resist',
  22: 'Summon Item',
  23: 'Summon Pet',
  24: 'Blind',
  25: 'Stun',
  26: 'Charm',
  27: 'Fear',
  28: 'Fatigue',
  29: 'Bind Affinity',
  30: 'Gate',
  31: 'Cancel Magic',
  32: 'Invisibility',
  33: 'See Invisible',
  34: 'WaterBreathing',
  35: 'Current HP',
  36: 'Stacking Blocker',
  37: 'Mesmerize',
  38: 'Summon Item',
  39: 'Feign Death',
  40: 'Vision',
  41: 'Eye of Zomm',
  42: 'Reclaim Energy',
  43: 'Max HP',
  44: 'Corpse Bomb',
  45: 'NPC Frenzy',
  46: 'NPC Awareness',
  47: 'NPC Aggro',
  48: 'NPC Faction',
  49: 'Destroy',
  50: 'Shadow Step',
  51: 'Berserk',
  52: 'AE Amnesia',
  53: 'Hate',
  54: 'Magic Resist',
  55: 'Sense Undead',
  56: 'Sense Summoned',
  57: 'Sense Animals',
  58: 'Absorb',
  59: 'Rune',
  60: 'True North',
  61: 'Levitate',
  62: 'Illusion',
  63: 'Damage Shield',
  64: 'Identify',
  65: 'Item ID',
  66: 'Memblur',
  67: 'Spin Stun',
  68: 'Infravision',
  69: 'Ultravision',
  70: 'Eye of Zomm',
  71: 'Reclaim Energy',
  72: 'Teleport',
  73: 'Teleport 2',
  74: 'Current HP v2',
  75: 'Empathy',
  76: 'Translocate',
  77: 'NPC Special Attack',
  78: 'Enthrall',
  79: 'Create Item',
  80: 'Summon Pet v2',
  81: 'Confuse',
  82: 'Disease',
  83: 'Poison',
  84: 'Unknown',
  85: 'Invuln',
  86: 'Illusion',
  87: 'Spell Damage',
  88: 'Heal',
  89: 'Resurrect',
  90: 'Summon PC',
  91: 'Dispel',
  92: 'Movement Speed',
  93: 'Disguise',
  94: 'Invuln',
  95: 'SpellShield',
  96: 'Absorption',
  97: 'Unknown',
  98: 'Riposte',
  99: 'Dodge',
  100: 'Parry',
  101: 'Dual Wield',
  102: 'Double Attack',
  103: 'Lifetap',
  104: 'Weapon Proc',
  105: 'Block',
  106: 'Endurance',
  107: 'Discipline',
  108: 'Fizzle Rate',
  109: 'Mana Pool',
  110: 'Healing Bonus',
  111: 'Spell Dmg Bonus',
  112: 'Clairvoyance',
  113: 'Spell Crit',
  114: 'Add Proc',
  115: 'HP Regen v2',
  116: 'Mana Regen v2',
  117: 'Spell Duration',
  118: 'Spell Range',
  119: 'Spell Hate',
  120: 'Talent',
  121: 'Lifetap',
  122: 'Amnesia',
  123: 'Invis v2',
  124: 'Invis vs. Undead',
  125: 'Invis vs. Animals',
  126: 'Frenzied Burnout',
  127: 'Pet ATK',
  128: 'Max Mana',
  129: 'Frenzy Radius',
  130: 'Add Hate',
  131: 'Fade',
  132: 'Stacking Blocker',
  133: 'Mana Burn',
  134: 'Persistent Effect',
  135: 'Trap Circumvent',
  136: 'Voice Graft',
  137: 'Sentinel',
  138: 'Locate Corpse',
  139: 'Spell Resist',
  140: 'TB Combat',
  141: 'Teleport (Group)',
  142: 'Translocate (Group)',
  143: 'Assassinate',
  144: 'FinHold',
  145: 'Song Duration',
  146: 'Purify',
  147: 'Invis (Enhanced)',
  148: 'Melee Haste v2',
  149: 'WS Increase',
  150: 'Limit Max Level',
  151: 'Limit Resist',
  152: 'Limit Target',
  153: 'Limit Spell Type',
  154: 'Limit Spell',
  155: 'Limit Min Mana',
  156: 'Limit Effect',
  157: 'Limit Spell Cat',
  158: 'Limit Min Level',
  159: 'Limit Cast Time (Min)',
  160: 'Limit Cast Time (Max)',
  320: 'Blank',
  254: 'Blank',
}

export function effectLabel(id: number): string {
  if (id === 254 || id === 255 || id === 320) return ''
  return EFFECT_LABELS[id] ?? `Effect ${id}`
}

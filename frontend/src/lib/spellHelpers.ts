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

/** Convert ticks to a human-readable duration string (1 tick = 6 seconds). */
export function ticksToTime(ticks: number): string {
  if (ticks <= 0) return 'Instant'
  if (ticks >= 50000) return 'Permanent'
  const seconds = ticks * 6
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return mins > 0 ? (secs > 0 ? `${mins}m ${secs}s` : `${mins}m`) : `${secs}s`
}

/** Returns true if the spell duration scales with caster level. */
export function durationScales(formula: number, ticks: number): boolean {
  return formula !== 0 && ticks > 0 && ticks < 50000
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
  return formula === 0 ? timeStr : `up to ${timeStr}`
}

// ── Effect ID labels ───────────────────────────────────────────────────────────
//
// Source: EQEmu spdat.h SE_* enum (canonical Spell Affecting Attribute codes).
// The codes stored in `spells_new.effectidN` are these canonical values, copied
// verbatim from the Quarm MySQL dump by the converter — they are stable
// across DB rebuilds and EQEmu forks. Only display labels live here; if a new
// Quarm-specific SPA appears, add it below by ID rather than renumbering.
//
// IDs 254 and 255 are sentinel "blank slot" markers used by EQEmu to terminate
// the 12-slot effect array. They must never render.

const EFFECT_LABELS: Record<number, string> = {
  0: 'Hitpoints',
  1: 'AC',
  2: 'ATK',
  3: 'Movement Speed',
  4: 'STR',
  5: 'DEX',
  6: 'AGI',
  7: 'STA',
  8: 'INT',
  9: 'WIS',
  10: 'CHA',
  11: 'Melee Haste',
  12: 'Invisibility',
  13: 'See Invisible',
  14: 'Enduring Breath',
  15: 'Mana',
  18: 'Lull',
  19: 'Faction',
  20: 'Blind',
  21: 'Stun',
  22: 'Charm',
  23: 'Fear',
  24: 'Fatigue',
  25: 'Bind Affinity',
  26: 'Gate',
  27: 'Cancel Magic',
  28: 'Invis vs Undead',
  29: 'Invis vs Animals',
  31: 'Mez',
  32: 'Summon Item',
  33: 'Summon Pet',
  35: 'Disease Counter',
  36: 'Poison Counter',
  40: 'Divine Aura',
  41: 'Shadow Step',
  42: 'Berserker Strength',
  44: 'Lycanthropy',
  46: 'Resist Fire',
  47: 'Resist Cold',
  48: 'Resist Poison',
  49: 'Resist Disease',
  50: 'Resist Magic',
  52: 'Sense Undead',
  53: 'Sense Summoned',
  54: 'Sense Animals',
  55: 'Stoneskin',
  57: 'True North',
  58: 'Levitate',
  59: 'Damage Shield',
  61: 'Identify',
  63: 'Memblur',
  64: 'Spin Stun',
  65: 'Infravision',
  66: 'Ultravision',
  67: 'Eye of Zomm',
  68: 'Reclaim Energy',
  69: 'Max HP',
  73: 'Bind Sight',
  74: 'Feign Death',
  75: 'Voice Graft',
  76: 'Sentinel',
  77: 'Locate Corpse',
  78: 'Absorb Magic Damage',
  79: 'Current HP',
  81: 'Resurrect',
  83: 'Teleport',
  85: 'Add Proc',
  86: 'Reaction Radius',
  87: 'Magnification',
  88: 'Evacuate',
  89: 'Player Size',
  90: 'Cloak',
  91: 'Summon Corpse',
  92: 'Hate',
  94: 'Stop Rain',
  96: 'Silence',
  97: 'Mana Pool',
  98: 'Bard Haste',
  99: 'Root',
  100: 'Heal Over Time',
  101: 'Complete Heal',
  102: 'Pet Fearless',
  103: 'Summon Pet',
  104: 'Translocate',
  105: 'Anti-Gate',
  106: 'Summon Warder',
  108: 'Summon Familiar',
  109: 'Summon Item Group',
  111: 'Resistances',
  112: 'Casting Level',
  113: 'Summon Mount',
  114: 'Hate Generated',
  115: 'Cannibalize',
  116: 'Crit Melee',
  117: 'Crit Direct Damage',
  118: 'Crippling Blow',
  119: 'Melee Haste v2',
  120: 'Healing Bonus',
  121: 'Reverse Damage Shield',
  123: 'Reflect Spell',
  124: 'Spell Damage Bonus',
  125: 'Healing Effectiveness',
  126: 'Spell Resist Reduction',
  127: 'Spell Haste',
  128: 'Spell Duration',
  129: 'Spell Range',
  130: 'Spell Hate',
  131: 'Reagent Chance',
  132: 'Mana Cost',
  134: 'Limit: Max Level',
  135: 'Limit: Resist',
  136: 'Limit: Target',
  137: 'Limit: Effect',
  138: 'Limit: Spell Type',
  139: 'Limit: Spell',
  140: 'Limit: Min Duration',
  141: 'Limit: Instant Only',
  142: 'Limit: Min Level',
  143: 'Limit: Min Cast Time',
  144: 'Limit: Max Cast Time',
  145: 'Teleport',
  147: 'Percent Heal',
  148: 'Stacking Block',
  149: 'Stacking Override',
  150: 'Death Save',
  151: 'Suspend Pet',
  152: 'Temporary Pet',
  153: 'Balance Group HP',
  154: 'Dispel Detrimental',
  155: 'Spell Crit Damage',
  156: 'Illusion Copy',
  157: 'Spell Damage Shield',
  158: 'Reflect Spell',
  159: 'All Stats',
  160: 'Make Drunk',
  161: 'Rune',
  162: 'Magic Rune',
  163: 'Negate Attacks',
  167: 'Pet Power',
  168: 'Melee Mitigation',
  169: 'Crit Hit Chance',
  170: 'Spell Crit Chance',
  171: 'Crippling Blow Chance',
  172: 'Avoidance',
  173: 'Riposte Chance',
  174: 'Dodge Chance',
  175: 'Parry Chance',
  176: 'Dual Wield Chance',
  177: 'Double Attack Chance',
  178: 'Melee Lifetap',
  179: 'Instrument Modifier',
  180: 'Resist Spell Chance',
  181: 'Resist Fear',
  182: 'Hundred Hands',
  183: 'Melee Skill Check',
  184: 'Hit Chance',
  185: 'Damage Modifier',
  186: 'Min Damage Modifier',
  189: 'Endurance',
  190: 'Endurance Pool',
  191: 'Amnesia',
  192: 'Hate Override',
  193: 'Skill Attack',
  194: 'Fading Memories',
  195: 'Stun Resist',
  196: 'Strikethrough',
  197: 'Skill Damage Taken',
  198: 'Endurance (instant)',
  199: 'Taunt',
  200: 'Proc Chance',
  201: 'Ranged Proc',
  204: 'Group Fear Immunity',
  205: 'Rampage',
  206: 'AE Taunt',
  208: 'Cure Poison',
  209: 'Dispel Beneficial',
  210: 'Pet Shield',
  211: 'AE Melee',
  214: 'Max HP %',
  215: 'Pet Avoidance',
  216: 'Accuracy',
  217: 'Headshot',
  218: 'Pet Crit Chance',
  219: 'Slay Undead',
  220: 'Skill Damage',
}

// SPAs whose base value is a percentage — used in the default-branch
// formatter so chance modifiers render as "Riposte Chance: +100%" instead
// of the bare "+100".
const PERCENT_SPAS = new Set<number>([
  155, // Spell Crit Damage
  168, // Melee Mitigation
  169, // Crit Hit Chance
  170, // Spell Crit Chance
  171, // Crippling Blow Chance
  172, // Avoidance
  173, // Riposte Chance
  174, // Dodge Chance
  175, // Parry Chance
  176, // Dual Wield Chance
  177, // Double Attack Chance
  180, // Resist Spell Chance
  181, // Resist Fear
  182, // Hundred Hands
  184, // Hit Chance
  185, // Damage Modifier
  186, // Min Damage Modifier
  195, // Stun Resist
  196, // Strikethrough
  197, // Skill Damage Taken
  200, // Proc Chance
  214, // Max HP %
  215, // Pet Avoidance
  216, // Accuracy
  218, // Pet Crit Chance
])

export function effectLabel(id: number): string {
  if (id === 254 || id === 255 || id === 320) return ''
  return EFFECT_LABELS[id] ?? `Effect ${id}`
}

// ── Zone type ──────────────────────────────────────────────────────────────────

const ZONE_TYPE_LABELS: Record<number, string> = {
  1: 'Outdoor',
  2: 'Indoor',
  3: 'Outdoor & Underground',
  4: 'City',
}

/** Returns zone restriction label, or empty string for unrestricted (0). */
export function zoneTypeLabel(z: number): string {
  return ZONE_TYPE_LABELS[z] ?? ''
}

// ── Timer trigger helpers ──────────────────────────────────────────────────────

export type SpellTimerTriggerPrefill = {
  name: string
  pattern: string
  wornOffPattern: string
  timerType: 'buff' | 'detrimental'
  timerDurationSecs: number
  spellId: number
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * Decide whether a spell is a buff (beneficial) or detrimental based on its
 * target type. Mirrors the Go backend's spelltimer.categorize() heuristic.
 * Target types 3 (Group v1), 6 (Self), 10 (Group v2), 41 (All of group) → buff.
 */
export function spellIsBuff(targetType: number): boolean {
  return targetType === 3 || targetType === 6 || targetType === 10 || targetType === 41
}

/**
 * Build a trigger prefill from a spell DB record. Chooses the best available
 * landed-message (cast_on_you → cast_on_other → spell name) and the
 * spell_fades message as the worn-off pattern.
 */
export function buildSpellTriggerPrefill(spell: {
  id: number
  name: string
  cast_on_you: string
  cast_on_other: string
  spell_fades: string
  target_type: number
  buff_duration: number
  buff_duration_formula: number
}): SpellTimerTriggerPrefill {
  const landed = spell.cast_on_you || spell.cast_on_other || spell.name
  const pattern = escapeRegex(landed)
  const wornOff = spell.spell_fades ? escapeRegex(spell.spell_fades) : ''

  // Approximate duration at the level cap; scaling formulas generally hit
  // their cap by 60, so this is a useful default the user can tweak.
  const durationTicks = approxDurationTicks(spell.buff_duration_formula, spell.buff_duration, 60)
  const durationSecs = durationTicks > 0 ? durationTicks * 6 : 0

  return {
    name: spell.name,
    pattern,
    wornOffPattern: wornOff,
    timerType: spellIsBuff(spell.target_type) ? 'buff' : 'detrimental',
    timerDurationSecs: durationSecs,
    spellId: spell.id,
  }
}

/**
 * Mirror of backend spelltimer.CalcDurationTicks for the common formulas.
 * Returns 0 for instant / permanent / unknown cases.
 */
function approxDurationTicks(formula: number, base: number, level: number): number {
  if (level <= 0) level = 1
  switch (formula) {
    case 0: return 0
    case 1: return Math.min(Math.floor(level / 2), base)
    case 2: return Math.min(Math.floor(30 / level) + base, base * 2)
    case 3: return Math.min(level * 30, base)
    case 4: return Math.min(level * 2 + base, base * 3)
    case 5: return Math.min(level * 5 + base, base * 3)
    case 6: return Math.min(level * 30 + base, base * 3)
    case 7: return Math.min(level * 5, base)
    case 8: return Math.min(level + base, base * 3)
    case 9: return Math.min(level * 2, base)
    case 10: return Math.min(level, base)
    case 11: return base
    case 50: return Math.max(1, Math.floor(level / 5))
    case 3600: return 0
    default: return base
  }
}

// ── Effect descriptions ────────────────────────────────────────────────────────

const STAT_NAMES: Record<number, string> = {
  4: 'STR', 5: 'DEX', 6: 'AGI', 7: 'STA', 8: 'INT', 9: 'WIS', 10: 'CHA',
}

const RESIST_NAMES: Record<number, string> = {
  46: 'Fire', 47: 'Cold', 48: 'Poison', 49: 'Disease', 50: 'Magic',
}

// SPA 138 base value → spell-type filter label.
const SPELL_TYPE_FILTER: Record<number, string> = {
  0: 'Detrimental only',
  1: 'Beneficial only',
  2: 'Beneficial - Group Only',
}

/**
 * Returns a human-readable description for a spell effect slot.
 *
 * `id` is the canonical EQEmu SPA code (see EFFECT_LABELS above). `base` is the
 * unscaled base value from `effect_base_valueN`; level/formula scaling is not
 * modelled — this matches how pqdi.cc displays focus and limit effects.
 *
 * Returns empty string for sentinel/blank slots and for ID/base combinations
 * that should not render.
 */
export function effectDescription(id: number, base: number, buffduration: number): string {
  if (id === 254 || id === 255 || id === 320) return ''

  const sign = base > 0 ? '+' : ''

  // Stat buffs/debuffs (SPAs 4-10) — render as "+42 STR".
  if (STAT_NAMES[id] !== undefined) {
    if (base === 0) return ''
    return `${sign}${base} ${STAT_NAMES[id]}`
  }

  // Per-resist SPAs (46-50).
  if (RESIST_NAMES[id] !== undefined) {
    if (base === 0) return ''
    return `${sign}${base} ${RESIST_NAMES[id]} Resist`
  }

  switch (id) {
    case 0: // Hitpoints — heal/instant damage, or HoT/DoT when buffduration>0
      if (base === 0) return ''
      if (buffduration > 0) {
        return base > 0
          ? `Increase HP by ${base} per tick`
          : `Decrease HP by ${Math.abs(base)} per tick`
      }
      return base > 0 ? `Heal ${base} HP` : `Deal ${Math.abs(base)} damage`
    case 79: // Current HP (single application — nuke/heal landing)
      if (base === 0) return ''
      return base > 0 ? `Heal ${base} HP` : `Deal ${Math.abs(base)} damage`
    case 1: // AC
      if (base === 0) return ''
      return `${sign}${base} AC`
    case 2: // ATK
      if (base === 0) return ''
      return `${sign}${base} ATK`
    case 3: // Movement Speed (% modifier)
      if (base === 0) return ''
      return `Movement Speed ${sign}${base}%`
    case 11: // Melee Haste / Attack Speed (% modifier)
      if (base === 0) return ''
      return `Attack Speed ${sign}${base}%`
    case 15: { // Mana — instant or per-tick depending on buff duration
      if (base === 0) return ''
      if (buffduration > 0) {
        return base > 0
          ? `Increase Mana by ${base} per tick`
          : `Decrease Mana by ${Math.abs(base)} per tick`
      }
      return base > 0 ? `Restore ${base} Mana` : `Drain ${Math.abs(base)} Mana`
    }
    case 21: // Stun (duration in ms)
      if (base === 0) return ''
      return `Stun for ${(base / 1000).toFixed(base % 1000 === 0 ? 0 : 1)}s`
    case 35: // Disease counter
      if (base === 0) return ''
      return base > 0 ? `Apply ${base} Disease counters` : `Cure ${Math.abs(base)} Disease counters`
    case 36: // Poison counter
      if (base === 0) return ''
      return base > 0 ? `Apply ${base} Poison counters` : `Cure ${Math.abs(base)} Poison counters`
    case 59: // Damage Shield
      if (base === 0) return ''
      return `Damage Shield: ${Math.abs(base)} per hit`
    case 69: // Max HP
      if (base === 0) return ''
      return `${sign}${base} Max HP`
    case 92: // Hate
      if (base === 0) return ''
      return `${sign}${base} Hate`
    case 97: // Mana Pool / Max Mana
      if (base === 0) return ''
      return `${sign}${base} Max Mana`
    case 100: // Heal Over Time
      if (base === 0) return ''
      return `Increase HP by ${base} per tick`

    // ── Focus effects (% modifiers on other spells) ─────────────────────────
    case 124: // Spell Damage % bonus
      return `Increase Spell Damage by ${base}%`
    case 125: // Healing Effectiveness % bonus (outgoing)
      return `Increase Healing by ${base}%`
    case 126: // Spell Resist Reduction
      return `Decrease Resist Check by ${base}`
    case 127: // Spell Haste — decrease cast time %
      return base > 0 ? `Decrease Spell Cast Time by ${base}%` : `Increase Spell Cast Time by ${Math.abs(base)}%`
    case 128: // Spell Duration — increase by %
      return `Increase Spell Duration by ${base}%`
    case 129: // Spell Range %
      return `Increase Spell Range by ${base}%`
    case 130: // Spell Hate %
      return `Modify Spell Hate by ${sign}${base}%`
    case 131: // Reagent Chance — chance to not consume reagent
      return `${base}% chance to not consume reagent`
    case 132: // Mana Cost — decrease %
      return `Decrease Mana Cost by ${base}%`

    // ── Focus limits (constrain which spells the focus applies to) ──────────
    case 134: // Limit: Max Level
      return `Limit: Max Level (${base})`
    case 135: // Limit: Resist — base value matches the EQEmu resist code
      return `Limit: Resist (${RESIST_LABELS[base] ?? `code ${base}`})`
    case 136: // Limit: Target Type
      return `Limit: Target (${TARGET_LABELS[base] ?? `code ${base}`})`
    case 137: { // Limit: Effect — base is itself an SPA code; negative = exclude
      const verb = base < 0 ? 'Exclude' : 'Include'
      const spa = Math.abs(base)
      const name = EFFECT_LABELS[spa] ?? `Effect ${spa}`
      return `${verb}: Effect(${name})`
    }
    case 138: // Limit: Spell Type
      return `Limit: Spell Type (${SPELL_TYPE_FILTER[base] ?? `code ${base}`})`
    case 139: { // Limit: Spell ID — base is a spell ID; negative = exclude
      const verb = base < 0 ? 'Exclude' : 'Include'
      return `${verb}: Spell(ID ${Math.abs(base)})`
    }
    case 140: // Limit: Min Duration — base is in 6-second ticks
      return `Limit: Min Duration (${base * 6} sec)`
    case 141: // Limit: Instant only
      return 'Limit: Instant Spells Only'
    case 142: // Limit: Min Level
      return `Limit: Min Level (${base})`
    case 143: // Limit: Min Cast Time (ms)
      return `Limit: Min Cast Time (${(base / 1000).toFixed(base % 1000 === 0 ? 0 : 1)}s)`
    case 144: // Limit: Max Cast Time (ms)
      return `Limit: Max Cast Time (${(base / 1000).toFixed(base % 1000 === 0 ? 0 : 1)}s)`

    default: {
      const label = effectLabel(id)
      if (!label) return ''
      if (base === 0) return label
      // Chance/percent-modifier SPAs whose base values are %s.
      if (PERCENT_SPAS.has(id)) return `${label}: ${sign}${base}%`
      return `${label}: ${sign}${base}`
    }
  }
}

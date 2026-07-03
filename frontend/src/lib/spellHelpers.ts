// Label lookups for spells_new enum columns (resist, target, skill,
// effect/SPA, type filter) live in the canonical Go catalog
// (backend/internal/db/enums/spell.go). Helpers below re-export the
// cache-backed accessors so existing call sites keep working.
import {
  spellEffectLabel,
  spellResistLabel,
  spellSkillLabel,
  spellTargetLabel,
  spellTypeFilterLabel,
} from './enumsCache'
import type { TimerAlertThreshold } from '../types/trigger'

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
    .filter((c) => c.level < 254)
    .sort((a, b) => a.level - b.level)
}

/** Short label for the list row (first 4 castable classes). */
export function castableClassesShort(classLevels: number[]): string {
  const classes = castableClasses(classLevels)
  if (classes.length === 0) return 'No Class'
  const shown = classes.slice(0, 4).map((c) => `${c.abbr} ${c.level}`)
  return shown.join(' · ') + (classes.length > 4 ? ` +${classes.length - 4}` : '')
}

// ── Known-spell matching (Zeal spellbook) ──────────────────────────────────────

/**
 * Expands a raw set of Zeal-exported spell IDs to include every duplicate-name
 * variant. Quarm's spells_new ships several rows per spell name with different
 * IDs (e.g. "Skin like Wood" as both 26 and 2110); list views collapse these to
 * one canonical row, but the in-game client can scribe — and Zeal can export —
 * whichever duplicate it pleases. Matching the export against canonical IDs
 * alone would then mark an owned spell as missing.
 *
 * For each spell in `spells` (canonical list rows carrying their `variant_ids`),
 * if ANY ID in its variant group is present in `rawKnown`, every ID in the group
 * is added to the result. Callers can then keep doing `known.has(spell.id)` /
 * `known.has(gemSpellId)` and match regardless of which duplicate was scribed.
 *
 * `knownNames` (the spell names from a Zeal export that carries a name column)
 * is an additional match key: a spell whose name appears in that set is marked
 * known even when NONE of its ids match. This keeps owned spells registering
 * when the exported spell id has drifted from the bundled quarm.db id — the
 * ids move between server patches, the names don't.
 */
export function expandKnownSpellIds(
  rawKnown: Iterable<number>,
  spells: Iterable<{ id: number; name?: string; variant_ids?: number[] }>,
  knownNames?: Iterable<string>,
): Set<number> {
  const known = new Set(rawKnown)
  const names = new Set<string>()
  if (knownNames) {
    for (const n of knownNames) names.add(n.trim().toLowerCase())
  }
  for (const s of spells) {
    const group = [s.id, ...(s.variant_ids ?? [])]
    const matched =
      group.some((id) => known.has(id)) ||
      (s.name != null && names.has(s.name.trim().toLowerCase()))
    if (matched) {
      for (const id of group) known.add(id)
    }
  }
  return known
}

// ── Resist / Target / Skill labels ────────────────────────────────────
//
// All three label maps live in backend/internal/db/enums/spell.go. The
// re-exported names below preserve the legacy call sites.

export const resistLabel = spellResistLabel
export const targetLabel = spellTargetLabel
export const skillLabel = spellSkillLabel

// ── Timing helpers ─────────────────────────────────────────────────────────────

/**
 * Format a seconds value for display, keeping it clean: whole numbers render
 * as plain integers ("16") and only fractional values show a decimal ("1.5").
 * Rounds to at most 2 decimals and strips trailing zeros, so float artifacts
 * (e.g. 1.4999999) and ".0" suffixes never reach the UI.
 */
export function secsLabel(secs: number): string {
  return String(Math.round(secs * 100) / 100)
}

/** Convert milliseconds to a human-readable string (e.g. "2.5s"). */
export function msLabel(ms: number): string {
  if (ms <= 0) return 'Instant'
  const s = ms / 1000
  return s === Math.floor(s) ? `${s}s` : `${s.toFixed(1)}s`
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
// SPA labels live in the canonical Go catalog
// (backend/internal/db/enums/spell.go). Local TS no longer carries the
// label map.

// SPAs whose base value is a percentage — used in the default-branch
// formatter so chance modifiers render as "Riposte Chance: +100%" instead
// of the bare "+100". This stays in TS because it's a behavior set, not
// a label table.
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
  return spellEffectLabel(id) || (id === 254 || id === 255 || id === 320 ? '' : `Effect ${id}`)
}

// ── Zone type ──────────────────────────────────────────────────────────────────
//
// Zone type labels live in the canonical Go catalog
// (backend/internal/db/enums/zone.go).
export { zoneTypeLabel } from './enumsCache'

// ── Timer trigger helpers ──────────────────────────────────────────────────────

export type SpellTimerTriggerPrefill = {
  name: string
  pattern: string
  wornOffPattern: string
  timerType: 'buff' | 'detrimental'
  timerDurationSecs: number
  spellId: number
  timerAlerts: TimerAlertThreshold[]
}

/**
 * Seed a single "fading soon" alert for a spell-timer trigger so the From-spell
 * flow gives a proactive heads-up out of the box (the user can edit or remove
 * it in the trigger editor). Buffs warn ~3 min out so there's time to recast;
 * detrimentals/mez warn ~10s out. The lead never exceeds half the duration, so
 * short buffs still get a meaningful alert rather than one that fires instantly
 * or never (useTimerAlerts only fires once remaining crosses below the
 * threshold from above). Returns [] when the spell has no trackable duration.
 */
function buildDefaultFadeAlert(
  timerType: 'buff' | 'detrimental',
  durationSecs: number,
): TimerAlertThreshold[] {
  if (durationSecs <= 0) return []
  const desired = timerType === 'buff' ? 180 : 10
  const seconds = Math.min(desired, Math.max(1, Math.floor(durationSecs / 2)))
  return [
    {
      id: 'spell-fade-default',
      seconds,
      type: 'text_to_speech',
      sound_path: '',
      volume: 80,
      tts_template: '{spell} fading soon',
      voice: '',
      tts_volume: 80,
    },
  ]
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * Coarse default for the trigger prefill: treat self/group/pet target types
 * as buffs. Uses EQMacEmu numbering (see backend/internal/db/enums/spell.go):
 * 6 (Self), 14 (Pet), 41 (Group). The backend's spelltimer.categorize() is
 * authoritative at runtime — it keys off effect IDs + goodEffect, not just
 * target type.
 */
export function spellIsBuff(targetType: number): boolean {
  return targetType === 6 || targetType === 14 || targetType === 41
}

// Matches a spell target's name in a cast_on_other line so a trigger fires
// regardless of who the spell landed on. Mirrors backend logparser.nameClass
// (castindex.go): lowercase-leading articled mobs ("a sand giant"), multi-word
// named NPCs, and apostrophe/backtick possessives ("Gygr`s warder"). The old
// uppercase-only, single-word form silently dropped every detrimental landing
// on trash mobs — the bulk of real targets — so bard debuff songs and other
// "+"-button triggers never fired on anything but a single-word named mob.
const NAME_REGEX = `[a-zA-Z][a-zA-Z' \`]{2,29}`

// Bard class index in the 15-class level table (WAR..BST). Mirrors backend
// spelltimer.isBardSong / buffmod.BardClassIdx.
const BARD_CLASS_IDX = 7

// isBardSong reports whether a spell is a bard song — castable only by the
// bard class. Mirrors backend spelltimer.isBardSong: bard (index 7) below the
// 255 "cannot cast" sentinel and every other class at 255. Bard songs report
// their base duration rather than the scaled formula result (see backend
// duration.go bardSongUseBaseDuration) — songs pulse-refresh every tick while
// the bard sings, so the timer only matters for the ~base ticks after they
// stop, not the inflated formula cap.
function isBardSong(classLevels: number[] | undefined): boolean {
  if (!classLevels || classLevels.length <= BARD_CLASS_IDX) return false
  if (classLevels[BARD_CLASS_IDX] >= 255) return false
  return classLevels.every((lvl, i) => i === BARD_CLASS_IDX || lvl >= 255)
}

/**
 * Build a trigger prefill from a spell DB record. Generates an alternation
 * regex covering both the self landed text (cast_on_you) and the third-party
 * landed text (cast_on_other prefixed with a name placeholder) so the trigger
 * fires whether the buff/debuff lands on the caster, a group member, or an
 * enemy. spell_fades is used for the worn-off pattern.
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
  class_levels?: number[]
}): SpellTimerTriggerPrefill {
  const branches: string[] = []
  if (spell.cast_on_you) branches.push(escapeRegex(spell.cast_on_you))
  if (spell.cast_on_other) branches.push(NAME_REGEX + escapeRegex(spell.cast_on_other))
  const pattern =
    branches.length > 0
      ? `^(?:${branches.join('|')})$`
      : `^${escapeRegex(spell.name)}$`
  const wornOff = spell.spell_fades ? `^${escapeRegex(spell.spell_fades)}$` : ''

  // Approximate duration at the level cap (60); scaling formulas generally
  // hit their cap by 60, so this is a useful default the user can tweak. Bard
  // songs are the exception — they report their base duration rather than the
  // formula result (see isBardSong).
  const durationTicks = isBardSong(spell.class_levels)
    ? spell.buff_duration
    : approxDurationTicks(spell.buff_duration_formula, spell.buff_duration, 60)
  const durationSecs = durationTicks > 0 ? durationTicks * 6 : 0

  const timerType = spellIsBuff(spell.target_type) ? 'buff' : 'detrimental'
  return {
    name: spell.name,
    pattern,
    wornOffPattern: wornOff,
    timerType,
    timerDurationSecs: durationSecs,
    spellId: spell.id,
    timerAlerts: buildDefaultFadeAlert(timerType, durationSecs),
  }
}

/**
 * Mirror of backend spelltimer.CalcDurationTicks — a faithful port of
 * EQMacEmu's CalcBuffDuration_formula (the EQMac/Quarm ruleset, which diverges
 * from modern EQEmu). Keep in lockstep with backend/internal/spelltimer/
 * duration.go. Returns 0 for instant / permanent / unknown cases.
 */
function approxDurationTicks(formula: number, base: number, level: number): number {
  if (level <= 0) level = 1
  // Any formula value >= 200 is a literal tick count.
  if (formula >= 200) return formula
  switch (formula) {
    case 0: return 0
    case 1: return capAtBase(Math.floor(level / 2), base)
    case 2: return capAtBase(level <= 1 ? 6 : Math.floor(level / 2) + 5, base)
    case 3: return capAtBase(level * 30, base)
    case 4: return capToBase(50, base)
    case 5: return capToBase(2, base)
    case 6: return capToBase(Math.floor(level / 2) + 2, base)
    case 7: return capToBase(level, base)
    case 8: return capAtBase(level + 10, base)
    case 9: return capAtBase(level * 2 + 10, base)
    case 10: return capAtBase(level * 3 + 10, base)
    case 11: return capAtBase(level * 30 + 90, base)
    case 12: return capToBase(Math.max(1, Math.floor(level / 4)), base)
    case 50: return 0 // permanent buff — no countdown
    default: return 0
  }
}

// capAtBase / capToBase mirror the two clamping patterns in EQMacEmu's
// CalcBuffDuration_formula (see duration.go for the C++ correspondence).
function capAtBase(i: number, base: number): number {
  if (i < base) return i < 1 ? 1 : i
  return base
}

function capToBase(i: number, base: number): number {
  if (base !== 0) return i < base ? i : base
  return i
}

// ── Effect descriptions ────────────────────────────────────────────────────────

const STAT_NAMES: Record<number, string> = {
  4: 'STR', 5: 'DEX', 6: 'AGI', 7: 'STA', 8: 'INT', 9: 'WIS', 10: 'CHA',
}

const RESIST_NAMES: Record<number, string> = {
  46: 'Fire', 47: 'Cold', 48: 'Poison', 49: 'Disease', 50: 'Magic',
}

// Project Quarm classic-era player level cap. Used as the upper bound when
// scaling level-formula effects past the highest castable class level.
const SERVER_LEVEL_CAP = 60

// Mirrors EQMacEmu Mob::CalcSpellEffectValue_formula (zone/spell_effects.cpp).
// Returns the effect value at a given caster level, applying updownsign and
// the post-formula max clamp. Unknown formulas fall back to raw base.
export function applyLevelFormula(formula: number, base: number, max: number, level: number): number {
  const ubase = Math.abs(base)
  const updownsign = max !== 0 && max < base ? -1 : 1
  let result: number
  switch (formula) {
    case 0:
    case 100: result = ubase; break
    case 101: result = updownsign * (ubase + Math.floor(level / 2)); break
    case 102: result = updownsign * (ubase + level); break
    case 103: result = updownsign * (ubase + level * 2); break
    case 104: result = updownsign * (ubase + level * 3); break
    case 105: result = updownsign * (ubase + level * 4); break
    case 109: result = updownsign * (ubase + Math.floor(level / 4)); break
    case 110: result = updownsign * (ubase + Math.floor(level / 6)); break
    case 119: result = updownsign * (ubase + Math.floor(level / 8)); break
    case 121: result = ubase + Math.floor(level / 3); break
    default: return base
  }
  if (max !== 0) {
    if (updownsign === 1 && result > max) result = max
    if (updownsign === -1 && result < max) result = max
  }
  if (base < 0 && result > 0) result = -result
  return result
}

// Smallest caster level at which `applyLevelFormula` first reaches `max`.
// Returns undefined for static formulas or when the value never grows past base.
function formulaCapLevel(formula: number, base: number, max: number): number | undefined {
  const ubase = Math.abs(base)
  const umax = Math.abs(max)
  if (max === 0 || umax <= ubase) return undefined
  const span = umax - ubase
  switch (formula) {
    case 101: return span * 2
    case 102: return span
    case 103: return Math.ceil(span / 2)
    case 104: return Math.ceil(span / 3)
    case 105: return Math.ceil(span / 4)
    case 119: return span * 8
    default: return undefined
  }
}

// Lowest castable level across the spell's class table. 255 is the
// "not castable" sentinel; entries above the server cap (e.g. Ranger 65 on
// some spells) are ignored.
function minCasterLevel(classLevels?: number[], levelCap = SERVER_LEVEL_CAP): number | undefined {
  if (!classLevels || classLevels.length === 0) return undefined
  let min = Infinity
  for (const lvl of classLevels) {
    if (lvl > 0 && lvl <= levelCap && lvl < min) min = lvl
  }
  return Number.isFinite(min) ? min : undefined
}

/**
 * Returns a human-readable description for a spell effect slot.
 *
 * `id` is the canonical EQEmu SPA code (see backend/internal/db/enums/spell.go).
 * `base` is the unscaled base value from `effect_base_valueN`; level/formula
 * scaling is not modelled — this matches how pqdi.cc displays focus and limit
 * effects.
 *
 * `max` and `formula` are optional; when supplied for SPA 11/119 (melee haste)
 * with formula 102 (linear scaling by level), the description renders a range
 * `+1% to +50%` matching pqdi rather than the raw base value.
 *
 * `classLevels` is the spell's 15-class level table — when supplied alongside
 * a level-scaling formula on SPA 3 (movement speed) or a resist SPA, the
 * description renders a pqdi-style "+N (Lx) to +M (Ly)" range.
 *
 * `levelCap` bounds the high end of those ranges to the active era cap (60
 * pre-PoP, 65 with pop_enabled); defaults to the pre-PoP cap.
 *
 * `spellNames` resolves spell IDs referenced by effect base values (SPA 85
 * Add Proc — see effectSpellRef) to display names; without it those slots
 * fall back to "Spell #N".
 *
 * `itemNames` resolves item IDs referenced by effect base values (SPA 32
 * Summon Item / SPA 109 Summon Item Into Bag — see effectItemRef) to display
 * names; without it those slots fall back to "Item #N".
 *
 * Returns empty string for sentinel/blank slots and for ID/base combinations
 * that should not render.
 */
export function effectDescription(id: number, base: number, buffduration: number, max?: number, formula?: number, classLevels?: number[], levelCap = SERVER_LEVEL_CAP, spellNames?: ReadonlyMap<number, string>, itemNames?: ReadonlyMap<number, string>): string {
  if (id === 254 || id === 255 || id === 320) return ''

  const sign = base > 0 ? '+' : ''

  // Stat buffs/debuffs (SPAs 4-10) — render as "+42 STR".
  if (STAT_NAMES[id] !== undefined) {
    if (base === 0) return ''
    return `${sign}${base} ${STAT_NAMES[id]}`
  }

  // Per-resist SPAs (46-50). Like movement speed, resist debuffs/buffs are
  // commonly level-scaled (formula 101/102 with the true magnitude in `max`),
  // so render a "low (Lx) to high (Ly)" range rather than the raw base — e.g.
  // Tashanian is -9 base but scales to ~-39 at L60.
  if (RESIST_NAMES[id] !== undefined) {
    if (base === 0) return ''
    const name = RESIST_NAMES[id]
    const fmt = (v: number): string => `${v > 0 ? '+' : ''}${v}`
    if (formula !== undefined && formula !== 100 && formula !== 0) {
      const minL = minCasterLevel(classLevels, levelCap)
      if (minL !== undefined) {
        const lowVal = applyLevelFormula(formula, base, max ?? 0, minL)
        const cap = formulaCapLevel(formula, base, max ?? 0)
        const highL = Math.min(levelCap, cap ?? levelCap)
        const highVal = applyLevelFormula(formula, base, max ?? 0, highL)
        if (lowVal !== highVal) {
          return `${fmt(lowVal)} (L${minL}) to ${fmt(highVal)} (L${highL}) ${name} Resist`
        }
        return `${fmt(highVal)} ${name} Resist`
      }
    }
    return `${fmt(base)} ${name} Resist`
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
    case 3: { // Movement Speed (% modifier)
      if (base === 0) return ''
      const verb = base > 0 ? 'Increase' : 'Decrease'
      const minL = minCasterLevel(classLevels, levelCap)
      if (formula !== undefined && formula !== 100 && formula !== 0 && minL !== undefined) {
        const lowVal = applyLevelFormula(formula, base, max ?? 0, minL)
        const cap = formulaCapLevel(formula, base, max ?? 0)
        const highL = Math.min(levelCap, cap ?? levelCap)
        const highVal = applyLevelFormula(formula, base, max ?? 0, highL)
        if (Math.abs(lowVal) !== Math.abs(highVal)) {
          return `${verb} Movement by ${Math.abs(lowVal)}% (L${minL}) to ${Math.abs(highVal)}% (L${highL})`
        }
        return `${verb} Movement by ${Math.abs(highVal)}%`
      }
      return `${verb} Movement by ${Math.abs(base)}%`
    }
    case 11: { // Melee Haste v1 — "+100" encoded (base 122 → +22%)
      if (base === 0) return ''
      const pct = base - 100
      // For formula 102 (linear scale by level) with a larger max, pqdi
      // renders the range — e.g. spell 998 (base 101, max 150) shows
      // "+1% to +50%". Otherwise show the single converted value.
      if (formula === 102 && max !== undefined && max > base) {
        const maxPct = max - 100
        return `Attack Speed +${pct}% to +${maxPct}%`
      }
      const psign = pct >= 0 ? '+' : ''
      return `Attack Speed ${psign}${pct}%`
    }
    case 119: { // Melee Haste v2 — raw % (base 25 → +25%), no +100 shift
      if (base === 0) return ''
      const sign = base > 0 ? '+' : ''
      return `Attack Speed ${sign}${base}%`
    }
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
    case 85: { // Add Proc — base is the proc spell's ID, not a magnitude
      if (base <= 0) return ''
      return `Add Proc: ${spellNames?.get(base) ?? `Spell #${base}`}`
    }
    case 32: { // Summon Item — base is the summoned item's ID, not a magnitude
      if (base <= 0) return ''
      return `Summon Item: ${itemNames?.get(base) ?? `Item #${base}`}`
    }
    case 109: { // Summon Item Into Bag — base is the summoned item's ID
      if (base <= 0) return ''
      return `Summon Item Into Bag: ${itemNames?.get(base) ?? `Item #${base}`}`
    }
    case 89: { // Player Size — base is % of normal size (66 = shrink to 66%)
      if (base <= 0 || base === 100) return ''
      return base > 100
        ? `Increase Player Size by ${base - 100}%`
        : `Decrease Player Size by ${100 - base}%`
    }
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

    // ── Quarm-specific SPAs (codes 160, 500-504) ────────────────────────────
    // pqdi.cc renders these with descriptive per-base templates rather than
    // the static catalog labels; mirror that so the spell detail panel
    // matches the public reference.
    case 160: // Movement Speed (Stackable) — Swiftness/Fleetness/Nimbleness
      if (base === 0) return ''
      return `Increase Movement by ${base}%`
    case 500:
      if (base === 0) return ''
      return `Increase Kill XP by ${base}%`
    case 501:
      if (base === 0) return ''
      return `Increase Quest XP by ${base}%`
    case 503:
      if (base === 0) return ''
      return `Increase Skillup Rate by ${base}%`
    case 504:
      if (base === 0) return ''
      return `Increase Tradeskill Skillup Rate by ${base}%`

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
      return `Limit: Resist (${spellResistLabel(base)})`
    case 136: // Limit: Target Type
      return `Limit: Target (${spellTargetLabel(base)})`
    case 137: { // Limit: Effect — base is itself an SPA code; negative = exclude
      const verb = base < 0 ? 'Exclude' : 'Include'
      const spa = Math.abs(base)
      const name = spellEffectLabel(spa) || `Effect ${spa}`
      return `${verb}: Effect(${name})`
    }
    case 138: // Limit: Spell Type
      return `Limit: Spell Type (${spellTypeFilterLabel(base)})`
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

/**
 * Spell ID referenced by an effect slot's base value, or null when the slot's
 * base is a plain magnitude. Currently SPA 85 (Add Proc), whose base is the
 * proc spell's ID — callers use this to fetch the name effectDescription
 * substitutes via its `spellNames` map.
 */
export function effectSpellRef(id: number, base: number): number | null {
  return id === 85 && base > 0 ? base : null
}

/**
 * Item ID referenced by an effect slot's base value, or null when the slot's
 * base is a plain magnitude. Covers SPA 32 (Summon Item) and SPA 109 (Summon
 * Item Into Bag), whose base is the summoned item's ID — callers use this to
 * fetch the name effectDescription substitutes via its `itemNames` map.
 */
export function effectItemRef(id: number, base: number): number | null {
  return (id === 32 || id === 109) && base > 0 ? base : null
}

import type { NPC } from '../types/npc'
import { npcClassName, npcRaceName, specialAbilityMeta } from './enumsCache'

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

// NPC class labels live in the canonical Go catalog
// (backend/internal/db/enums/npc_class.go).
export function className(classId: number): string {
  return npcClassName(classId)
}

// ── Race ────────────────────────────────────────────────────────────────────────

// NPC race labels live in the canonical Go catalog
// (backend/internal/db/enums/npc_race.go).
export function raceName(raceId: number): string {
  return npcRaceName(raceId)
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
// rather than encoded in the special_abilities string. Mirrors
// backend/internal/db/enums/special_abilities.go — kept in sync there.
export const SYN_SEE_INVIS = 1001
export const SYN_SEE_INVIS_UNDEAD = 1002

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
      const entry = specialAbilityMeta(code)
      return [{
        code,
        value,
        name: entry.name,
        description: entry.description ?? '',
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
    const entry = specialAbilityMeta(code)
    abilities.push({
      code,
      value: 1,
      name: entry.name,
      description: entry.description ?? '',
    })
  }
  if (npc.see_invis) ensure(SYN_SEE_INVIS)
  if (npc.see_invis_undead) ensure(SYN_SEE_INVIS_UNDEAD)
  return abilities
}

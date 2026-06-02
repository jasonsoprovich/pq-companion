import type { NPC } from '../types/npc'
import { npcBodyTypeName, npcClassName, npcRaceName, specialAbilityMeta } from './enumsCache'

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
//
// NPC body type labels live in the canonical Go catalog
// (backend/internal/db/enums/npc_body_type.go). The previous local map
// here had nearly every label mis-assigned; importers see corrected
// labels automatically after this swap.
export function bodyTypeName(bodyType: number): string {
  return npcBodyTypeName(bodyType)
}

// ── Run Speed ─────────────────────────────────────────────────────────────────────
//
// npc_types.runspeed is on the NPC movement scale, NOT the client scale. Per
// EQMacEmu (zone/mob.cpp): "in game a 0.7 speed for client, is same as a 1.4
// speed for NPCs." So an unbuffered player's base run speed corresponds to an
// NPC runspeed of 1.4 — that's our 100% reference. Most NPCs sit around 1.25
// (~89%, slightly slower than a player); faster mobs run 1.5+ (107%+).
//
// (Dividing by 0.7 — the client-scale value — double-counted and made every
// 1.25 NPC read as ~179%, which is actually mount/bridle speed.)
const PLAYER_BASE_RUNSPEED = 1.4

// NPC run speed as a percentage of an unbuffered player's base run speed.
export function npcRunSpeedPct(runSpeed: number): number {
  return Math.round((runSpeed / PLAYER_BASE_RUNSPEED) * 100)
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

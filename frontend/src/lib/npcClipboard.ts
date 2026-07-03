import type { TargetState, SpecialAbility, NamedSpell } from '../types/overlay'

// Builds the one-line "copy target stats" string a raid leader can paste into
// chat to call the next target, e.g.:
//
//   Aten Ha Ra | HP: 1.9M | DMG: 294-1054 | MR:162 FR:176 CR:195 PR:160 DR:179 |
//   Slowable, Rampage, Enrage | Spells: Word of Command (30s, 35 PBAE, -100 MR),
//   Silence of the Shadows (30s, 80 TAE), Fling (45s, 200 PBAE)
//
// Returns null when there's nothing to copy (no target, or a target with no DB
// record — a player, whose stats we don't have).

// SPA code 12 = Immune to Slow (EQMacEmu special_abilities). Its presence flips
// the derived slowability tag; the raw ability name is suppressed so we never
// print both "Slowable" and "Immune to Slow".
const IMMUNE_TO_SLOW = 12

// Curated "pertinent to a pull" ability codes: dangerous melee specials plus the
// CC / damage immunities a raid actually plans around. Code 12 is intentionally
// excluded — it's surfaced by the Slowable/Unslowable tag instead.
const PERTINENT_ABILITY_CODES = new Set([
  1, 2, 3, 4, 5, 6, 7, // Summon, Enrage, Rampage, Area Rampage, Flurry, Triple Attack, Dual Wield
  13, 14, 15, 16, 17, 18, // Immune to Mez, Charm, Stun, Snare, Fear, Dispel
  19, 20, 22, 23, 35, // Immune to Melee, Magic, Non-Bane Melee, Non-Magical Melee, Harm-from-Client
])

// Abbreviates a max-HP figure the way a raid callout wants it: 1.9M, 45K, 8,200.
function fmtHP(hp: number): string {
  if (hp >= 1_000_000) return `${(hp / 1_000_000).toFixed(hp >= 10_000_000 ? 0 : 1)}M`
  if (hp >= 10_000) return `${Math.round(hp / 1000)}K`
  return hp.toLocaleString()
}

// Derives the tag segment: slowability first, then the pertinent special-ability
// names in the order the backend resolved them.
function buildTags(abilities: SpecialAbility[]): string[] {
  const active = abilities.filter((a) => a.value !== 0)
  const tags = [active.some((a) => a.code === IMMUNE_TO_SLOW) ? 'Unslowable' : 'Slowable']
  for (const a of active) {
    if (PERTINENT_ABILITY_CODES.has(a.code) && a.name) tags.push(a.name)
  }
  return tags
}

// Formats one signature spell as "Name (recast, AE, resist)", dropping any
// detail the spell lacks; a spell with no detail is just its name.
function fmtSpell(s: NamedSpell): string {
  const parts: string[] = []
  if (s.recast_secs) parts.push(`${s.recast_secs}s`)
  if (s.ae_type && s.ae_range) parts.push(`${s.ae_range} ${s.ae_type}`)
  else if (s.ae_type) parts.push(s.ae_type)
  if (s.resist_type && s.resist_diff) {
    parts.push(`${s.resist_diff > 0 ? '+' : ''}${s.resist_diff} ${s.resist_type}`)
  }
  return parts.length > 0 ? `${s.spell_name} (${parts.join(', ')})` : s.spell_name
}

export function buildTargetStatsLine(state: TargetState | null): string | null {
  const npc = state?.npc_data
  if (!state || !npc) return null

  const segments: string[] = [
    state.target_name ?? npc.name ?? 'Unknown',
    `HP: ${fmtHP(npc.hp)}`,
    `DMG: ${npc.min_dmg}-${npc.max_dmg}`,
    `MR:${npc.mr} FR:${npc.fr} CR:${npc.cr} PR:${npc.pr} DR:${npc.dr}`,
  ]

  const tags = buildTags(state.special_abilities ?? [])
  if (tags.length > 0) segments.push(tags.join(', '))

  const signature = state.caster_summary?.signature ?? []
  if (signature.length > 0) {
    segments.push(`Spells: ${signature.map(fmtSpell).join(', ')}`)
  }

  return segments.join(' | ')
}

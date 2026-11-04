import type { NPC, NPCCasterSummary } from './npc'

// Caster-summary types live in ./npc (shared by the overlays and the DB page);
// re-exported here so existing overlay imports keep resolving.
export type {
  NPCCasterSummary,
  CasterHighlight,
  NamedSpell,
  ClassListSummary,
} from './npc'

export interface SpecialAbility {
  code: number
  value: number
  name: string
}

// TargetVariant carries one alternative interpretation when the targeted
// name maps to multiple npc_types rows the backend couldn't reduce to one
// (typically Quarm RNG-pair NPCs that share a spawngroup, e.g. ssratemple's
// shissar revenant necro/SK split). Present in TargetState.variants when
// ambiguity is real; absent or single-element otherwise.
export interface TargetVariant {
  npc: NPC
  special_abilities: SpecialAbility[]
  caster_summary?: NPCCasterSummary
  // spawn_chance is the row's share (percent) of its spawn group in the
  // current zone, normalized the same way as the Zones → NPCs tab;
  // distinguish lists what sets it apart from sibling rows ("summons",
  // "texture 2"). Both only present on multi-variant results.
  spawn_chance?: number
  distinguish?: string[]
}

// variantSuffix renders the spawn weight and distinguishing traits of a
// variant as " · 10% spawn · summons" (empty when it has neither), for
// appending to a class/level/HP label.
export function variantSuffix(v: TargetVariant): string {
  const parts: string[] = []
  if (v.spawn_chance !== undefined) parts.push(`${v.spawn_chance}% spawn`)
  if (v.distinguish?.length) parts.push(...v.distinguish)
  return parts.length ? ` · ${parts.join(' · ')}` : ''
}

// MobThreat is the player's estimated personal hate into one mob (mirrors the
// Go threat.MobThreat). hate is floored at zero; tps is the lifetime average
// hate-per-second; tps_live is the recent rolling-window rate.
export interface MobThreat {
  name: string
  hate: number
  tps: number
  // tps_live is hate generated in the last ~6s ÷ that window: it rises during a
  // burst and decays toward zero when idle — the responsive "live" meter.
  tps_live: number
  is_target: boolean
}

// ThreatState mirrors the Go threat.ThreatState broadcast on overlay:threat.
// It is an estimate of the active character's OWN hate per mob, derived from
// that character's own log lines — see internal/threat for the model and its
// limitations.
export interface ThreatState {
  in_combat: boolean
  // target is the highlighted mob (current Zeal target, else most-recently
  // engaged). Absent when nothing is tracked; mirrors an entry in mobs.
  target?: MobThreat
  mobs: MobThreat[]
  hatemod_pct: number
  last_updated: string
}

// ── Raid-estimate threat (mirrors Go internal/raidthreat) ──────────────────
// An ESTIMATED per-mob, per-player hate view derived from observed damage. It
// carries the DPS meter's accuracy limits plus threat blind spots (others'
// DoTs/heals/taunts/out-of-range damage are unseen) — see internal/raidthreat.

// RaidThreatConfidence flags surface which estimates to distrust.
export type RaidThreatConfidence = 'class_unknown' | 'dot_undercount' | 'heal_undercount'

// RaidEntry is one player's (or pet's) estimated hate on a mob.
export interface RaidEntry {
  name: string
  class?: string
  owner_name?: string
  is_you: boolean
  is_pet: boolean
  hate: number
  hate_pct: number // hate / top_hate, 0..1
  confidence?: RaidThreatConfidence[]
}

// RaidMob is one mob's ranked per-player estimated hate.
export interface RaidMob {
  name: string
  is_target: boolean
  top_hate: number
  players: RaidEntry[]
}

// RaidThreatState mirrors the Go raidthreat.RaidThreatState on overlay:raidthreat.
export interface RaidThreatState {
  in_combat: boolean
  mobs: RaidMob[]
  last_updated: string
}

export interface TargetState {
  has_target: boolean
  target_name?: string
  npc_data?: NPC
  special_abilities?: SpecialAbility[]
  caster_summary?: NPCCasterSummary
  // variants is populated (length >= 2) when the target name is ambiguous —
  // e.g. two shissar revenant rows that share a spawngroup. Renderers should
  // surface all variants (class label, loot, abilities) instead of pretending
  // npc_data is the only answer. Single-variant lookups leave this empty.
  variants?: TargetVariant[]
  current_zone?: string
  // hp_percent is 0-100 when fed by the Zeal pipe, or -1 when unknown
  // (Zeal not running or no value yet for the current target).
  hp_percent: number
  // hp_at / hp_source stamp hp_percent (lib/live.ts): when Zeal last
  // confirmed it and from which feed. Absent while it's unknown.
  hp_at?: string
  hp_source?: string
  pet_owner?: string
  // is_corpse is true when the target name ended in "'s corpse" — the lookup
  // strips the suffix to find the underlying NPC, but the HP bar should pin
  // to 0% regardless of what Zeal reports.
  is_corpse?: boolean
  // live_level / live_class / target_type are what the client reports for the
  // live spawn (Zeal target descriptors). Absent on Zeal builds without them —
  // fall back to npc_data's level range and class then. target_type: 0 PC,
  // 1 NPC, 2 NPC corpse, 3 PC corpse.
  live_level?: number
  live_class?: number
  target_type?: number
  // target_id is the current target's spawn id from the Zeal pipe (v1.4.6+);
  // absent without Zeal or with no target.
  target_id?: number
  last_updated: string
}

// TargetDistance is the player→target distance feed (overlay:npc_target_distance
// and GET /api/overlay/npc/distance). distance is null when Zeal withheld the
// target position (250+ units away); has_descriptors is false on Zeal builds
// that don't report target info at all, in which case hide the readout.
export interface TargetDistance {
  distance: number | null
  has_descriptors: boolean
}

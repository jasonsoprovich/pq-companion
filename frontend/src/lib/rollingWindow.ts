import type {
  EntityStats,
  FightState,
  FightSummary,
  HealerStats,
} from '../types/combat'

// AggregatableFight is the structural subset aggregateRecentFights reads. Both
// FightSummary (live combat log / overlay) and StoredFight (persisted history)
// satisfy it, so the same pooling powers all three surfaces.
export type AggregatableFight = Pick<
  FightSummary,
  | 'start_time'
  | 'duration_seconds'
  | 'total_damage'
  | 'you_damage'
  | 'total_heal'
  | 'you_heal'
  | 'combatants'
  | 'healers'
>

// ROLLING_WINDOW_SIZE is how many recent completed fights the "Last N mobs"
// meter scope pools over. It is deliberately a separate constant from the
// backend's maxRecentFights (the in-memory fight scrollback cap, currently
// also 20): that buffer exists to feed the live combat log and the post-kill
// freeze, and could be retuned for those reasons without intending to move
// the analytics window. The backend only ships up to maxRecentFights fights,
// so this is effectively clamped to "however many recent fights are
// available, at most this many".
export const ROLLING_WINDOW_SIZE = 20

interface DamageAccum {
  name: string
  total_damage: number
  hit_count: number
  max_hit: number
  // active_seconds is summed across every fight this entity appeared in, so
  // the pooled Personal denominator counts only the time the entity was
  // actually engaged (consistent with per-fight Personal DPS).
  active_seconds: number
  // encounter_seconds / raid_seconds are ALSO summed only across the fights
  // this entity actually appeared in — not every fight in the window. A
  // shared whole-window denominator (the previous behaviour) divided a
  // part-time participant's damage by fights they took no part in at all,
  // which is what produced 0.1–0.9 DPS readings on hundreds/thousands of
  // damage for anyone who wasn't present for every pooled fight.
  encounter_seconds: number
  raid_seconds: number
  crit_count: number
  crit_damage: number
  owner_name?: string
  class?: string
  is_you?: boolean
}

interface HealAccum {
  name: string
  total_heal: number
  heal_count: number
  max_heal: number
  active_seconds: number
  // See DamageAccum.encounter_seconds / raid_seconds.
  encounter_seconds: number
  raid_seconds: number
  is_you?: boolean
}

// aggregateRecentFights pools the most recent `windowSize` completed fights
// into a single synthetic FightState so the DPS meter can render a moving
// average over the last N mobs using the exact same row/rollup/mode code as
// a single fight. Returns null when there are no completed fights yet.
//
// Each of the three DPS modes keeps its semantic identity, just pooled:
//   encounter (dps)        sum(damage) / sum(THAT ENTITY'S fight wall-clocks)
//   raid      (raid_dps)   sum(damage) / sum(THAT ENTITY'S fight raid spans)
//   personal  (active_dps) sum(damage) / sum(that entity's active spans)
// All three are per-entity denominators, summed only across the fights that
// entity actually appears in — a groupmate who was only present for one of
// the pooled fights is divided by that one fight's time, not by every fight
// in the window (that mismatch was the "0.1–0.9 DPS on hundreds/thousands of
// damage" bug: a shared whole-window denominator applied to everyone
// regardless of how many of the pooled fights they took part in). The
// fight-level totals below (total_dps/you_dps/etc.) are unaffected — those
// intentionally describe the whole pooled window, not one entity.
export function aggregateRecentFights(
  fights: AggregatableFight[],
  windowSize: number = ROLLING_WINDOW_SIZE,
): FightState | null {
  if (!fights || fights.length === 0) return null
  // recent_fights is newest-first, so the head is the most recent window.
  const window = fights.slice(0, windowSize)
  if (window.length === 0) return null

  const dmgByName = new Map<string, DamageAccum>()
  const healByName = new Map<string, HealAccum>()
  let totalDamage = 0
  let youDamage = 0
  let totalHeal = 0
  let youHeal = 0
  let sumDuration = 0
  let earliestStart: string | null = null

  for (const f of window) {
    sumDuration += f.duration_seconds
    totalDamage += f.total_damage
    youDamage += f.you_damage
    totalHeal += f.total_heal
    youHeal += f.you_heal
    if (earliestStart === null || f.start_time < earliestStart) {
      earliestStart = f.start_time
    }

    for (const c of f.combatants) {
      const a = dmgByName.get(c.name) ?? {
        name: c.name,
        total_damage: 0,
        hit_count: 0,
        max_hit: 0,
        active_seconds: 0,
        encounter_seconds: 0,
        raid_seconds: 0,
        crit_count: 0,
        crit_damage: 0,
        owner_name: c.owner_name,
        class: c.class,
        is_you: c.is_you,
      }
      a.total_damage += c.total_damage
      a.hit_count += c.hit_count
      a.max_hit = Math.max(a.max_hit, c.max_hit)
      a.active_seconds += c.active_seconds
      // This fight's wall-clock and raid span count toward this entity's own
      // denominators only because they appeared in f.combatants — i.e. only
      // for fights they actually took part in.
      a.encounter_seconds += f.duration_seconds
      a.raid_seconds += c.raid_seconds
      a.crit_count += c.crit_count
      a.crit_damage += c.crit_damage
      // A combatant may lack owner/class in one fight but carry it in
      // another (e.g. class resolved later) — keep the first non-empty.
      if (!a.owner_name && c.owner_name) a.owner_name = c.owner_name
      if (!a.class && c.class) a.class = c.class
      if (!a.is_you && c.is_you) a.is_you = true
      dmgByName.set(c.name, a)
    }

    for (const h of f.healers) {
      const a = healByName.get(h.name) ?? {
        name: h.name,
        total_heal: 0,
        heal_count: 0,
        max_heal: 0,
        active_seconds: 0,
        encounter_seconds: 0,
        raid_seconds: 0,
        is_you: h.is_you,
      }
      a.total_heal += h.total_heal
      a.heal_count += h.heal_count
      a.max_heal = Math.max(a.max_heal, h.max_heal)
      a.active_seconds += h.active_seconds
      a.encounter_seconds += f.duration_seconds
      a.raid_seconds += h.raid_seconds
      if (!a.is_you && h.is_you) a.is_you = true
      healByName.set(h.name, a)
    }
  }

  const combatants: EntityStats[] = [...dmgByName.values()]
    .map((a) => ({
      name: a.name,
      total_damage: a.total_damage,
      hit_count: a.hit_count,
      max_hit: a.max_hit,
      dps: a.encounter_seconds > 0 ? a.total_damage / a.encounter_seconds : 0,
      active_dps: a.active_seconds > 0 ? a.total_damage / a.active_seconds : 0,
      active_seconds: a.active_seconds,
      raid_dps: a.raid_seconds > 0 ? a.total_damage / a.raid_seconds : 0,
      raid_seconds: a.raid_seconds,
      crit_count: a.crit_count,
      crit_damage: a.crit_damage,
      owner_name: a.owner_name,
      class: a.class,
      is_you: a.is_you,
    }))
    .sort((x, y) => y.total_damage - x.total_damage)

  const healers: HealerStats[] = [...healByName.values()]
    .map((a) => ({
      name: a.name,
      total_heal: a.total_heal,
      heal_count: a.heal_count,
      max_heal: a.max_heal,
      hps: a.encounter_seconds > 0 ? a.total_heal / a.encounter_seconds : 0,
      active_hps: a.active_seconds > 0 ? a.total_heal / a.active_seconds : 0,
      active_seconds: a.active_seconds,
      raid_hps: a.raid_seconds > 0 ? a.total_heal / a.raid_seconds : 0,
      raid_seconds: a.raid_seconds,
      is_you: a.is_you,
    }))
    .sort((x, y) => y.total_heal - x.total_heal)

  const label = `Last ${window.length} ${window.length === 1 ? 'mob' : 'mobs'}`

  return {
    start_time: earliestStart ?? '',
    duration_seconds: sumDuration,
    primary_target: label,
    combatants,
    total_damage: totalDamage,
    total_dps: sumDuration > 0 ? totalDamage / sumDuration : 0,
    you_damage: youDamage,
    you_dps: sumDuration > 0 ? youDamage / sumDuration : 0,
    healers,
    total_heal: totalHeal,
    total_hps: sumDuration > 0 ? totalHeal / sumDuration : 0,
    you_heal: youHeal,
    you_hps: sumDuration > 0 ? youHeal / sumDuration : 0,
  }
}

import type {
  EntityStats,
  FightState,
  FightSummary,
  HealerStats,
} from '../types/combat'

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
  crit_count: number
  crit_damage: number
  owner_name?: string
  class?: string
}

interface HealAccum {
  name: string
  total_heal: number
  heal_count: number
  max_heal: number
  active_seconds: number
}

// aggregateRecentFights pools the most recent `windowSize` completed fights
// into a single synthetic FightState so the DPS meter can render a moving
// average over the last N mobs using the exact same row/rollup/mode code as
// a single fight. Returns null when there are no completed fights yet.
//
// Each of the three DPS modes keeps its semantic identity, just pooled:
//   encounter (dps)        sum(damage) / sum(all fight wall-clocks)  — shared
//   raid      (raid_dps)   sum(damage) / sum(all fight raid spans)   — shared
//   personal  (active_dps) sum(damage) / sum(that entity's active spans)
// The two "shared" denominators stay identical across every combatant (as
// they are within one fight), so bar % and ranking still line up; Personal
// stays per-entity and fair to a late-joiner or OOM caster.
export function aggregateRecentFights(
  fights: FightSummary[],
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
  let sumRaidSeconds = 0
  let sumHealRaidSeconds = 0
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

    // raid_seconds is constant across a fight's combatants — accumulate it
    // once per fight from the first available row.
    sumRaidSeconds += f.combatants[0]?.raid_seconds ?? 0
    for (const c of f.combatants) {
      const a = dmgByName.get(c.name) ?? {
        name: c.name,
        total_damage: 0,
        hit_count: 0,
        max_hit: 0,
        active_seconds: 0,
        crit_count: 0,
        crit_damage: 0,
        owner_name: c.owner_name,
        class: c.class,
      }
      a.total_damage += c.total_damage
      a.hit_count += c.hit_count
      a.max_hit = Math.max(a.max_hit, c.max_hit)
      a.active_seconds += c.active_seconds
      a.crit_count += c.crit_count
      a.crit_damage += c.crit_damage
      // A combatant may lack owner/class in one fight but carry it in
      // another (e.g. class resolved later) — keep the first non-empty.
      if (!a.owner_name && c.owner_name) a.owner_name = c.owner_name
      if (!a.class && c.class) a.class = c.class
      dmgByName.set(c.name, a)
    }

    sumHealRaidSeconds += f.healers[0]?.raid_seconds ?? 0
    for (const h of f.healers) {
      const a = healByName.get(h.name) ?? {
        name: h.name,
        total_heal: 0,
        heal_count: 0,
        max_heal: 0,
        active_seconds: 0,
      }
      a.total_heal += h.total_heal
      a.heal_count += h.heal_count
      a.max_heal = Math.max(a.max_heal, h.max_heal)
      a.active_seconds += h.active_seconds
      healByName.set(h.name, a)
    }
  }

  const combatants: EntityStats[] = [...dmgByName.values()]
    .map((a) => ({
      name: a.name,
      total_damage: a.total_damage,
      hit_count: a.hit_count,
      max_hit: a.max_hit,
      dps: sumDuration > 0 ? a.total_damage / sumDuration : 0,
      active_dps: a.active_seconds > 0 ? a.total_damage / a.active_seconds : 0,
      active_seconds: a.active_seconds,
      raid_dps: sumRaidSeconds > 0 ? a.total_damage / sumRaidSeconds : 0,
      raid_seconds: sumRaidSeconds,
      crit_count: a.crit_count,
      crit_damage: a.crit_damage,
      owner_name: a.owner_name,
      class: a.class,
    }))
    .sort((x, y) => y.total_damage - x.total_damage)

  const healers: HealerStats[] = [...healByName.values()]
    .map((a) => ({
      name: a.name,
      total_heal: a.total_heal,
      heal_count: a.heal_count,
      max_heal: a.max_heal,
      hps: sumDuration > 0 ? a.total_heal / sumDuration : 0,
      active_hps: a.active_seconds > 0 ? a.total_heal / a.active_seconds : 0,
      active_seconds: a.active_seconds,
      raid_hps: sumHealRaidSeconds > 0 ? a.total_heal / sumHealRaidSeconds : 0,
      raid_seconds: sumHealRaidSeconds,
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

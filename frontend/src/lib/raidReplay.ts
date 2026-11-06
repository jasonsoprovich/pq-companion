// Raid replay playback: where every raider is at a moment between two
// recorded frames. Frames are 1 s apart (backend raidtrack.frameInterval;
// recordings made before that are 3 s); positions in between are
// interpolated so playback glides instead of jumping every sample.

import type { RaidEvent, RaidEventKind, RaidFrame, RaidFrameTarget, RaidMemberInfo } from '../types/raidReplay'

// RETENTION_CHOICES are the "keep recordings for" options in days; -1 (keep
// forever) is offered alongside.
export const RETENTION_CHOICES = [7, 14, 30, 60, 90]

// MAX_GAP_MS is the longest gap interpolated across. A raider missing for
// longer (zoned out, a recording gap) is shown at their last frame instead
// of sliding across the map.
const MAX_GAP_MS = 10_000

// DEAD_SHOW_MS is how long a raider who died stays drawn as a cross at the
// spot, since a dead raider leaves the zone's frames once they respawn.
export const DEAD_SHOW_MS = 30_000

export interface RaiderAt {
  index: number
  // GameX/GameY, as recorded.
  x: number
  y: number
  z: number
  heading: number
  group: number
  hp: number
}

// frameIndexAt returns the index of the last frame at or before t, or -1
// before the first.
export function frameIndexAt(frames: RaidFrame[], t: number): number {
  let lo = 0
  let hi = frames.length - 1
  let best = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (frames[mid].t <= t) {
      best = mid
      lo = mid + 1
    } else {
      hi = mid - 1
    }
  }
  return best
}

// raidersAt returns each raider present at t, interpolated between the
// frames either side.
export function raidersAt(frames: RaidFrame[], t: number): RaiderAt[] {
  const i = frameIndexAt(frames, t)
  if (i < 0) return []
  const a = frames[i]
  const b = frames[i + 1]
  const span = b ? b.t - a.t : 0
  const f = b && span > 0 && span <= MAX_GAP_MS ? Math.min(1, (t - a.t) / span) : 0
  const next = new Map<number, number[]>()
  if (f > 0) for (const row of b.m) next.set(row[0], row)
  return a.m.map((row) => {
    const n = next.get(row[0])
    const lerp = (k: number): number => (n ? row[k] + (n[k] - row[k]) * f : row[k])
    return {
      index: row[0],
      x: lerp(1),
      y: lerp(2),
      z: lerp(3),
      heading: row[4],
      group: row[5],
      hp: row[6],
    }
  })
}

// lastSeen returns each raider's last recorded row at or before t, for
// placing a death where it happened.
export function lastSeenAt(frames: RaidFrame[], t: number, index: number): RaiderAt | null {
  for (let i = frameIndexAt(frames, t); i >= 0; i--) {
    const row = frames[i].m.find((r) => r[0] === index)
    if (row) {
      return { index, x: row[1], y: row[2], z: row[3], heading: row[4], group: row[5], hp: row[6] }
    }
  }
  return null
}

// targetAt returns the observer's target in the frame at or before t.
export function targetAt(frames: RaidFrame[], t: number): RaidFrameTarget | null {
  const i = frameIndexAt(frames, t)
  return i >= 0 ? frames[i].tg ?? null : null
}

// groupsAt lays out the raiders present at t by group number, for the
// groups panel: groups in order, then ungrouped (0) last.
export function groupsAt(present: RaiderAt[]): { group: number; indexes: number[] }[] {
  const by = new Map<number, number[]>()
  for (const r of present) {
    const g = by.get(r.group) ?? []
    g.push(r.index)
    by.set(r.group, g)
  }
  return [...by.entries()]
    .sort(([a], [b]) => (a || 99) - (b || 99))
    .map(([group, indexes]) => ({ group, indexes }))
}

// KILL_LOOKBACK_MS is how long before a kill is searched for the dead mob's
// position (it was the recorder's target with a known position).
const KILL_LOOKBACK_MS = 60_000

// recentKills returns the kills in the DEAD_SHOW_MS before t that can be
// placed: the dead mob was the recorder's target shortly before, with its
// position known. Positions are as recorded (GameX/GameY).
export function recentKills(
  frames: RaidFrame[],
  events: RaidEvent[],
  t: number,
): { name: string; x: number; y: number; z: number }[] {
  const out: { name: string; x: number; y: number; z: number }[] = []
  for (const e of events) {
    if (e.kind !== 'kill' || e.t > t || t - e.t > DEAD_SHOW_MS) continue
    const name = e.name.toLowerCase()
    const i = frameIndexAt(frames, e.t)
    for (let j = i; j >= 0 && frames[j].t >= e.t - KILL_LOOKBACK_MS; j--) {
      const tg = frames[j].tg
      if (tg?.l && tg.n.toLowerCase() === name) {
        out.push({ name: e.name, x: tg.l[0], y: tg.l[1], z: tg.l[2] })
        break
      }
    }
  }
  return out
}

// recentDeaths returns the death events in the DEAD_SHOW_MS before t.
export function recentDeaths(events: RaidEvent[], t: number): RaidEvent[] {
  return events.filter((e) => e.kind === 'death' && e.t <= t && t - e.t <= DEAD_SHOW_MS)
}

// LOW_HP_PCT is the health a raider has to drop under for a low-HP mark.
const LOW_HP_PCT = 20

// lowHPEvents marks each time a raider drops under LOW_HP_PCT, from the
// health sampled in every frame, so a death can be traced back. A raider
// missing from a frame starts over when they're back.
export function lowHPEvents(frames: RaidFrame[], members: RaidMemberInfo[]): RaidEvent[] {
  const names = new Map(members.map((m) => [m.index, m.name]))
  const out: RaidEvent[] = []
  let prev = new Map<number, number>()
  for (const f of frames) {
    const next = new Map<number, number>()
    for (const row of f.m) {
      const [idx, , , , , , hp] = row
      next.set(idx, hp)
      if (hp < 0 || hp >= LOW_HP_PCT) continue
      const was = prev.get(idx)
      if (was === undefined || was < 0 || was >= LOW_HP_PCT) {
        out.push({ t: f.t, kind: 'low_hp', name: names.get(idx) ?? '?', other: `${hp}%` })
      }
    }
    prev = next
  }
  return out
}

// EVENT_GROUPS are the event filters in Raid Replay, each a set of kinds.
export const EVENT_GROUPS: { key: string; label: string; kinds: RaidEventKind[] }[] = [
  { key: 'deaths', label: 'Kills & deaths', kinds: ['kill', 'death'] },
  { key: 'ch', label: 'CH', kinds: ['ch', 'ch_land'] },
  { key: 'mech', label: 'Mob mechanics', kinds: ['enrage', 'rampage', 'flurry', 'cast', 'npc_shout'] },
  { key: 'hp', label: 'Low HP & rez', kinds: ['low_hp', 'rez'] },
  { key: 'calls', label: 'Raid calls', kinds: ['call'] },
  { key: 'loot', label: 'Loot', kinds: ['loot'] },
]

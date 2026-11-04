// Raid replay playback: where every raider is at a moment between two
// recorded frames. Frames are ~3 s apart (backend raidtrack.frameInterval);
// positions in between are interpolated so playback glides instead of
// jumping a body-length every sample.

import type { RaidEvent, RaidFrame } from '../types/raidReplay'

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

// recentDeaths returns the death events in the DEAD_SHOW_MS before t.
export function recentDeaths(events: RaidEvent[], t: number): RaidEvent[] {
  return events.filter((e) => e.kind === 'death' && e.t <= t && t - e.t <= DEAD_SHOW_MS)
}

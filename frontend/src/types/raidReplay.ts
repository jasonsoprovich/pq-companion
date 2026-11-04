// TypeScript types mirroring backend/internal/raidtrack/store.go

export interface RaidSession {
  id: number
  character: string
  zone: string
  zone_name?: string
  started_at: string
  ended_at: string
  frames: number
  /** Most raiders seen in the zone in one frame. */
  peak_members: number
  kills: number
  deaths: number
}

export interface RaidMemberInfo {
  index: number
  name: string
  /** Zeal 1-indexed class id (1 = Warrior … 15 = Beastlord), 0 unknown. */
  class: number
  level: number
}

/**
 * One sample. Each row of `m` is [index, x, y, z, heading, group, hp%]:
 * x/y in the spawn2 (GameX/GameY) convention, heading EQ's 0-512, group 0
 * for ungrouped, hp% -1 when unknown.
 */
export interface RaidFrame {
  t: number
  m: number[][]
  /** The observer's target, when they had one. */
  tg?: RaidFrameTarget
}

/**
 * The observer's Zeal target in a frame. `l` is [x, y, z] like frame rows,
 * present only when Zeal reported its position (within 250 units).
 */
export interface RaidFrameTarget {
  n: string
  l?: number[]
}

/**
 * A timeline mark. kill: an NPC died. death: a raider died. ch: a CH chain
 * call on `name` by `other`. ch_land: a Complete Heal landed on `name`.
 */
export interface RaidEvent {
  t: number
  kind: 'kill' | 'death' | 'ch' | 'ch_land'
  name: string
  other?: string
}

export interface RaidRecording {
  session: RaidSession
  members: RaidMemberInfo[]
  frames: RaidFrame[]
  events: RaidEvent[]
}

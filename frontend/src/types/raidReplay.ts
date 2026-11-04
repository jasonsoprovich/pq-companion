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
}

export interface RaidEvent {
  t: number
  kind: 'kill' | 'death'
  name: string
  other?: string
}

export interface RaidRecording {
  session: RaidSession
  members: RaidMemberInfo[]
  frames: RaidFrame[]
  events: RaidEvent[]
}

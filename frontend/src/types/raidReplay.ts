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
 * enrage / rampage / flurry: mob `name`'s mechanic, on `other` when logged.
 * cast: the recorder's target, `name`, began casting. npc_shout: mob `name`
 * shouted `other`. call: raider `name` said `other` to the raid or shouted
 * it. loot: `name` looted `other`. rez: `name` (the recorder) was
 * resurrected. low_hp is made in the app from the frames' health: `name`
 * dropped to `other`. zone_out / zone_in: `name` left or entered the
 * recorder's zone, and raid_join / raid_leave: `name` joined or left the
 * raid, are derived by the backend for a whole night (raidtrack/night.go).
 */
export type RaidEventKind =
  | 'kill' | 'death' | 'ch' | 'ch_land'
  | 'enrage' | 'rampage' | 'flurry' | 'cast' | 'npc_shout'
  | 'call' | 'loot' | 'rez' | 'low_hp'
  | 'zone_out' | 'zone_in' | 'raid_join' | 'raid_leave'

export interface RaidEvent {
  t: number
  kind: RaidEventKind
  name: string
  other?: string
}

export interface RaidRecording {
  session: RaidSession
  members: RaidMemberInfo[]
  frames: RaidFrame[]
  events: RaidEvent[]
}

/** A raid night: one character's sessions with short gaps between them. */
export interface RaidNight {
  /** The first session's id. */
  id: number
  character: string
  started_at: string
  ended_at: string
  /** Oldest first. */
  sessions: RaidSession[]
  peak_members: number
  kills: number
  deaths: number
}

/** One raider's night. Times are unix ms, 0 when unknown. */
export interface RaiderSummary {
  name: string
  class: number
  level: number
  first_seen: number
  last_seen: number
  /** 0 when already in the raid when the roster log started. */
  joined_at: number
  /** 0 when they stayed to the end. */
  left_at: number
  in_zone_ms: number
  /** Times they left the recorder's zone mid-session. */
  away: number
  deaths: number
  /** Time standing in one spot for 2+ minutes. */
  still_ms: number
  longest_still_ms: number
}

export interface RaidNightData {
  night: RaidNight
  /** Oldest first, one per session. */
  recordings: RaidRecording[]
  /** zone_out / zone_in / raid_join / raid_leave, oldest first. */
  events: RaidEvent[]
  summary: RaiderSummary[]
  /** False for nights recorded before joins and leaves were logged. */
  roster_tracked: boolean
}

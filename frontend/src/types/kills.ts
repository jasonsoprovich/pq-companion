// Mirrors backend internal/killtracker models.

export interface KillEntry {
  id: number
  character: string // local log owner
  target: string // what died
  killer: string // '' when the log line names no killer
  zone: string
  ts: number
  is_npc: boolean // false = a player slain by a mob
  named: boolean // best-effort proper-name flag
}

export type KillScope = 'npc' | 'named' | 'all'

export interface KillListResponse {
  kills: KillEntry[]
}

export interface KillMetaResponse {
  characters: string[]
  zones: string[]
  active: string
}

export interface KillMilestone {
  at: number
  label: string
}

export interface KillSyncRule {
  pattern: string
  set_to: number
}

export interface KillClockMark {
  pattern: string
  at_secs: number
  if_idle?: boolean // only honored while the clock isn't running
  reset?: boolean // also zeroes the count when honored
}

export interface KillClock {
  label: string
  duration_secs: number
  marks: KillClockMark[]
  stop_patterns?: string[]
}

export interface KillCounterDef {
  id: string
  name: string
  group?: string
  preset?: string
  enabled: boolean
  mobs?: string[]
  pattern?: string
  zone?: string
  goal?: number
  milestones?: KillMilestone[]
  warn_before?: number
  syncs?: KillSyncRule[]
  reset_pattern?: string
  clock?: KillClock | null
  alert_overlay: boolean
  alert_tts: boolean
}

export interface KillCounter {
  def: KillCounterDef
  count: number
  clock_started_at: number // unix seconds; 0 = not running
  updated_at: number
}

export interface KillCountersState {
  counters: KillCounter[]
  now: number
  // Where kills are counted from: 'zeal' once a Zeal death event has arrived
  // on the current pipe connection (every death in the zone), else 'log'.
  kill_source?: 'log' | 'zeal'
}

export interface KillCounterPreset {
  key: string
  name: string
  zone: string
  description: string
  counters: KillCounterDef[]
  installed: boolean
}

export type KillCounterAction =
  | { action: 'adjust'; value: number }
  | { action: 'set'; value: number }
  | { action: 'reset' }
  | { action: 'start_clock' }
  | { action: 'stop_clock' }
  | { action: 'enable'; enabled: boolean }
  | { action: 'move'; value: number }

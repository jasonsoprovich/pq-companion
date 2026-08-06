// Mirrors backend/internal/progress/progress.go and recap.go.

export type ProgressEventKind = 'level' | 'aa' | 'skill' | 'spell'

export interface ProgressEvent {
  character: string
  at: string // RFC3339
  kind: ProgressEventKind
  detail?: string
  value: number
  delta?: number
}

export interface DayBucket {
  date: string // YYYY-MM-DD
  count: number
}

export interface CharacterRecap {
  character: string
  window_start: string // RFC3339
  window_end: string // RFC3339

  start_level?: number
  end_level?: number
  levels_gained: number
  aas_gained: number
  spells_scribed: number
  skill_ups: number
  tradeskill_ups: number
  active_days: number

  has_snapshot_data: boolean
  coin_delta: number

  daily_activity: DayBucket[] | null
}

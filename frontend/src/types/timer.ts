// TypeScript types mirroring backend/internal/spelltimer/models.go

export type TimerCategory = 'buff' | 'debuff' | 'mez' | 'dot' | 'stun'

export interface ActiveTimer {
  id: string
  spell_name: string
  spell_id: number
  category: TimerCategory
  cast_at: string
  starts_at: string
  expires_at: string
  duration_seconds: number
  remaining_seconds: number
}

export interface TimerState {
  timers: ActiveTimer[]
  last_updated: string
}

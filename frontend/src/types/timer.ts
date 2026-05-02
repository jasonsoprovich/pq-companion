// TypeScript types mirroring backend/internal/spelltimer/models.go

export type TimerCategory = 'buff' | 'debuff' | 'mez' | 'dot' | 'stun'

export interface ActiveTimer {
  id: string
  spell_name: string
  spell_id: number
  /**
   * Recipient of the spell. The active player's character name for
   * self-cast buffs and "Your X spell has worn off." entries; the captured
   * name for buffs / debuffs cast on others; empty for trigger-driven
   * timers that don't carry a target.
   */
  target_name: string
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

// TypeScript types mirroring backend/internal/spelltimer/models.go

import type { TimerAlertThreshold } from './trigger'

export type TimerCategory = 'buff' | 'debuff' | 'mez' | 'dot' | 'stun'

export interface ActiveTimer {
  id: string
  spell_name: string
  spell_id: number
  icon?: number
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
  /**
   * Per-timer override for the user-configured global display threshold.
   * > 0 means "only show me when remaining time is at or below this
   * value"; 0 (the typical case) means "let the frontend resolve against
   * the global default for my category". Set on a per-trigger basis;
   * spell-landed-driven timers always emit 0.
   */
  display_threshold_secs: number
  /**
   * Per-trigger fading-soon notifications, copied from the source trigger
   * when the timer started. Only set on trigger-driven timers; absent (or
   * empty) for spell-cast-driven timers, which never fire fading alerts.
   */
  timer_alerts?: TimerAlertThreshold[]
}

export interface TimerState {
  timers: ActiveTimer[]
  last_updated: string
}

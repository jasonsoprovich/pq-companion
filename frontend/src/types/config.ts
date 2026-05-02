export interface Preferences {
  overlay_opacity: number
  minimize_to_tray: boolean
  parse_combat_log: boolean
  overlay_dps_enabled: boolean
  overlay_hps_enabled: boolean
}

export interface BackupSettings {
  auto_backup: boolean
  schedule: 'off' | 'hourly' | 'daily'
  max_backups: number
}

export type TrackingScope = 'self' | 'anyone'

export interface SpellTimerSettings {
  /**
   * Whose spell lands the engine tracks as timers.
   *   "self"   — only buffs/debuffs landing on the active player
   *   "anyone" — every recognised land (default; required for raid buff tracking)
   */
  tracking_scope: TrackingScope

  /**
   * Hide buff overlay rows whose remaining time exceeds this many seconds.
   * 0 (default) means always show — useful as-is for most users; bump to
   * e.g. 600 to only see buffs in the last 10 minutes of their duration.
   */
  buff_display_threshold_secs: number

  /**
   * Same as buff_display_threshold_secs, applied to the Detrimental
   * overlay (debuffs, DoTs, mez, stuns). 0 (default) means always show.
   */
  detrim_display_threshold_secs: number
}

export interface Config {
  eq_path: string
  character: string
  character_class: number // -1 = not set, 0-14 = EQ class index
  server_addr: string
  preferences: Preferences
  backup: BackupSettings
  spell_timer: SpellTimerSettings
  onboarding_completed: boolean
}

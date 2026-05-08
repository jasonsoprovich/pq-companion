export interface Preferences {
  overlay_opacity: number
  minimize_to_tray: boolean
  parse_combat_log: boolean
  overlay_dps_enabled: boolean
  overlay_hps_enabled: boolean
  master_volume: number
}

export interface BackupSettings {
  auto_backup: boolean
  schedule: 'off' | 'hourly' | 'daily'
  max_backups: number
}

export type TrackingScope = 'self' | 'cast_by_me' | 'anyone'

export type TrackingMode = 'auto' | 'triggers_only'

export interface SpellTimerSettings {
  /**
   * Whose spell lands the engine tracks as timers.
   *   "self"       — only buffs/debuffs landing on the active player
   *   "cast_by_me" — every land where the active character is the caster
   *                  (default; uses recent-cast correlation since EQ logs
   *                  don't record the caster on third-party land messages)
   *   "anyone"     — every recognised land, including others buffing each
   *                  other (useful for tracking raid mob debuffs cast by
   *                  another enchanter, etc.)
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

  /**
   * When true, drop buff timers whose source spell isn't castable by the
   * active character's class. Useful for hiding paladin/shaman/bard buffs
   * from a class with a long buff list of its own. Detrimentals are always
   * tracked regardless of this setting.
   */
  class_filter: boolean

  /**
   * Controls whether the spell-landed pipeline auto-creates timer rows.
   *   "auto"          — every recognised landing creates a timer; triggers
   *                     can attach metadata (thresholds, fading-soon TTS)
   *                     by firing on the same cast. The default.
   *   "triggers_only" — only triggers/packs create timers; the spell-landed
   *                     pipeline still parses log lines for cast
   *                     disambiguation but stops producing rows.
   */
  tracking_mode?: TrackingMode
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

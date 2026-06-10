import type { OverlayName, LockedMode } from '../lib/overlays'

export interface NPCOverlaySections {
  identity: boolean
  combat: boolean
  resists: boolean
  attributes: boolean
  special_abilities: boolean
  faction: boolean
  // spells is the master toggle for the caster-summary section (highlights are
  // always shown when on). The spells_* flags are per-group sub-toggles.
  spells: boolean
  spells_procs: boolean
  spells_signature: boolean
  spells_class: boolean
}

export const DEFAULT_NPC_OVERLAY_SECTIONS: NPCOverlaySections = {
  identity: true,
  combat: true,
  resists: true,
  attributes: true,
  special_abilities: true,
  faction: true,
  spells: true,
  spells_procs: true,
  spells_signature: true,
  spells_class: true,
}

export interface Preferences {
  overlay_opacity: number
  // Fade overlay chrome (background, border, title bar) to transparent a few
  // seconds after the cursor leaves an overlay window; content stays visible.
  // Hovering restores overlay_opacity. Off by default.
  overlay_fade_enabled?: boolean
  minimize_to_tray: boolean
  high_contrast: boolean
  zoom_factor: number
  parse_combat_log: boolean
  overlay_dps_enabled: boolean
  overlay_hps_enabled: boolean
  master_volume: number
  // Voice for any text_to_speech alert whose own voice field is empty
  // ("App default" in the editor). Empty = the OS default voice.
  default_tts_voice?: string
  // Anchors trigger overlay_text alerts that have no per-trigger pinned
  // position at a fixed point (alerts stack downward from it). Coordinates
  // are window-local pixels on the trigger overlay's chosen monitor.
  // Null/missing = centered stack (pre-existing behaviour).
  default_overlay_position?: { x: number; y: number } | null
  developer_mode: boolean
  npc_overlay_dashboard_sections: NPCOverlaySections
  npc_overlay_popout_sections: NPCOverlaySections
  // Per-overlay locked behaviour, keyed by canonical overlay name. Missing
  // keys default to "interactive". See lib/overlays.ts.
  overlay_locked_modes?: Partial<Record<OverlayName, LockedMode>>
  // Side-nav route keys the user has hidden from the navigation menu (the
  // page is still reachable by URL). The fixed controls are never hideable.
  sidebar_hidden?: string[]
  // Flat list of side-nav route keys in preferred display order; items are
  // ordered within their section by position here.
  sidebar_order?: string[]
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

// CHChainSettings configures the Complete-Heal-chain overlay matcher. Mirrors
// backend config.CHChainSettings.
export interface CHChainSettings {
  enabled: boolean
  // Regex matched against raid-chat lines; should capture named groups
  // caster, chainnum, target. Empty = backend default.
  pattern: string
  // Per-cast countdown cadence in seconds.
  interval_secs: number
}

// DPSClassColors stores the user's per-class bar colour for the DPS meter
// and combat history rows. Each field is a CSS-style "#RRGGBB" hex string;
// the frontend renders the value directly. Unknown / unresolved
// combatants fall back to the `unknown` colour.
export interface DPSClassColors {
  warrior: string
  cleric: string
  paladin: string
  ranger: string
  shadow_knight: string
  druid: string
  monk: string
  bard: string
  rogue: string
  shaman: string
  necromancer: string
  wizard: string
  magician: string
  enchanter: string
  beastlord: string
  unknown: string
}

export const DEFAULT_DPS_CLASS_COLORS: DPSClassColors = {
  warrior: '#C79C6E',
  cleric: '#FFFFFF',
  paladin: '#F58CBA',
  ranger: '#ABD473',
  shadow_knight: '#C41F3B',
  druid: '#FF7D0A',
  monk: '#00FF96',
  bard: '#8A47E8',
  rogue: '#FFF569',
  shaman: '#0070DE',
  necromancer: '#9482C9',
  wizard: '#40ED57',
  magician: '#69CCF0',
  enchanter: '#ED5CE5',
  beastlord: '#03B78A',
  unknown: '#B2B2B2',
}

export interface Config {
  eq_path: string
  character: string
  character_class: number // -1 = not set, 0-14 = EQ class index
  server_addr: string
  preferences: Preferences
  backup: BackupSettings
  spell_timer: SpellTimerSettings
  ch_chain: CHChainSettings
  dps_class_colors: DPSClassColors
  onboarding_completed: boolean
  // Days of Chat History to keep before the daily purge. Default 30; a
  // negative value (-1) keeps chat forever. 0 is coerced to the default
  // server-side.
  chat_retention_days: number
}

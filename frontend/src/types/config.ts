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

/**
 * Global default "fading soon" notification for the Custom Timer and Respawn
 * overlays — the settings counterpart to a trigger's TimerAlertThreshold.
 * Mirrors backend config.TimerAlertPref. `type` is the action kind; `seconds`
 * is the remaining-time threshold the alert fires at; volumes are 0–100.
 */
export interface TimerAlertPref {
  enabled: boolean
  seconds: number
  type: 'play_sound' | 'text_to_speech'
  sound_path: string
  volume: number
  tts_template: string
  voice: string
  tts_volume: number
}

export interface Preferences {
  overlay_opacity: number
  // Fade overlay chrome (background, border, title bar) to transparent a few
  // seconds after the cursor leaves an overlay window; content stays visible.
  // Hovering restores overlay_opacity. Off by default.
  overlay_fade_enabled?: boolean
  // Seconds after the cursor leaves an overlay before the chrome fade kicks
  // in. 0/missing = the default (2.5), mirroring the zoom_factor convention.
  overlay_fade_delay_secs?: number
  // Mutes the sound + on-screen warning when a PVP-flagged player shows up
  // in a /who or joins the group. Inverted so missing/false = warning on.
  pvp_warning_disabled?: boolean
  minimize_to_tray: boolean
  high_contrast: boolean
  zoom_factor: number
  // Per-overlay zoom for popout overlay windows, keyed by canonical overlay
  // name (see lib/overlays.ts). Scales an overlay independently of the main
  // window's zoom_factor. Missing/0 = 1.0 (100%).
  overlay_zoom_factors?: Record<string, number>
  parse_combat_log: boolean
  overlay_dps_enabled: boolean
  overlay_hps_enabled: boolean
  // Makes entity rows in overlay windows (NPC overlay loot/spells) clickable
  // links that open the item/spell in the main database explorer. On by
  // default; missing is treated as enabled.
  overlay_entity_links_enabled?: boolean
  master_volume: number
  // Voice for any text_to_speech alert whose own voice field is empty
  // ("App default" in the editor). Empty = the OS default voice.
  default_tts_voice?: string
  // Repeat-audio cooldown (seconds): after a trigger plays a sound/TTS,
  // further audio from that SAME trigger is suppressed for this long.
  // Collapses rapid same-trigger bursts (AE mez breaking several mobs) to
  // one alert; overlay text / history / timers are unaffected. 0 = off.
  trigger_audio_cooldown_secs?: number
  // Anchors trigger overlay_text alerts that have no per-trigger pinned
  // position at a fixed point (alerts stack downward from it). Coordinates
  // are window-local pixels on the trigger overlay's chosen monitor.
  // Null/missing = centered stack (pre-existing behaviour).
  default_overlay_position?: { x: number; y: number } | null
  // Global default style for trigger overlay_text alerts. Each field applies
  // to alerts whose own action leaves it unset ("App default" in the editor);
  // per-action values always win. Empty/0/missing = the renderer built-ins
  // (white, glow derived from the text color, system-ui, 20px) — the
  // pre-existing look. See lib/overlayTextStyle.ts.
  default_overlay_text_color?: string
  default_overlay_glow_color?: string
  default_overlay_font_family?: string
  default_overlay_font_size?: number
  // Default fading-soon alert armed on manually-added Custom Timers (the
  // overlay quick-add form lets the player toggle it off per timer). Disabled
  // by default. See TimerAlertPref.
  custom_timer_alert?: TimerAlertPref
  // Audio cue fired as each NPC respawn timer crosses `seconds` remaining
  // (0 = at "POP"). Applies to every respawn timer; disabled by default.
  respawn_alert?: TimerAlertPref
  developer_mode: boolean
  // Planes of Power era switch: level cap 65, PoP spells/AA tabs, PoK as a
  // shopping source. Off until the expansion launches on Quarm; until then
  // it's a preview toggle in the Developer tab. See lib/era.ts.
  pop_enabled?: boolean
  // Static gear/AA hate modifier (signed %) applied to the Threat Meter's
  // generated hate. Logs can't reveal it, so the user supplies it.
  threat_hatemod_pct?: number
  // Experimental raid-estimate threat mode (dev-gated, off by default).
  raid_threat_enabled?: boolean
  // Per-class hate adjustment (class name → signed %) for the raid estimate;
  // an entry overrides the built-in default (tanks +30 when unset).
  raid_threat_class_mods?: Record<string, number>
  // Per-player hate adjustment (player name → signed %), added on top of class.
  raid_threat_player_mods?: Record<string, number>
  // Planes of Power flagging tracker (dev-gated, off by default; preview until
  // PoP launches on Quarm).
  pop_flags_enabled?: boolean
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

export interface CombatSettings {
  // Days of combat history to keep in user.db before pruning. 0 = default
  // (30), negative = keep forever.
  retention_days: number
  // Inactivity window, in seconds, before an active fight (with damage) is
  // archived and dropped from the live meter. Now the main way a parse ends —
  // zoning and death no longer auto-clear fights. 0 = default (60). Raids
  // still floor to the built-in 120s window.
  fight_timeout_seconds: number
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

  /**
   * When true, a buff/detrimental stays in the overlay after it expires —
   * shown as an overdue row counting up — until it's recast or dismissed with
   * the row's X, so the overlay doubles as a "buffs I still need to refresh"
   * checklist. Off by default (timers vanish the moment they expire).
   */
  keep_expired_timers?: boolean

  /**
   * Timer overlay bar fill: '' / 'faded' (default ~15% tint), 'solid'
   * (~55% fill), or 'none' (no fill — just the countdown text).
   */
  timer_bar_fill?: 'faded' | 'solid' | 'none'

  /** Timer row spell-name font size in px. 0/absent = default (12). */
  timer_name_font_size?: number
  /** Timer row countdown font size in px. 0/absent = default (11). */
  timer_time_font_size?: number
  /** Timer row vertical padding in px (controls row height). 0/absent = 3. */
  timer_row_padding?: number
}

// CHChainSettings configures the Complete-Heal-chain overlay matcher. Mirrors
// backend config.CHChainSettings.
export interface CHChainSettings {
  enabled: boolean
  // Regex matched against raid-chat lines; should capture named groups
  // caster, chainnum, target. Empty = backend default.
  pattern: string
  // Secondary (ramp/split) chain: when enabled, lines matching
  // secondary_pattern get their own ch_chain_2 timers and the overlay /
  // metronome grow a Main/Ramp switch. The secondary pattern is tried
  // before the primary so letter calls split off even under the
  // catch-all primary default.
  secondary_enabled?: boolean
  secondary_pattern?: string
  // Per-cast countdown cadence in seconds (fractional allowed).
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
  combat: CombatSettings
  spell_timer: SpellTimerSettings
  ch_chain: CHChainSettings
  dps_class_colors: DPSClassColors
  onboarding_completed: boolean
  // Days of Chat History to keep before the daily purge. Default 30; a
  // negative value (-1) keeps chat forever. 0 is coerced to the default
  // server-side.
  chat_retention_days: number
}

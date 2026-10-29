import type { OverlayName, LockedMode, FadeMode } from '../lib/overlays'
import type { SavedRollProfile } from './rolls'

// User overrides for NPC overlay special-ability badges (lib/npcTagStyle).
// Colour values of '' mean "use the default" — config map keys can't be
// deleted through the PUT, so a reset stores an empty string.
export interface NPCOverlayTagStyle {
  // Category id ('attack' | 'damage_immune' | 'cc_immune' | 'other') → hex.
  category_colors?: Record<string, string>
  // Special-ability code (as a string) → hex, beating the category colour.
  tag_colors?: Record<string, string>
  // Ability codes drawn with a glow so they stand out.
  emphasized?: number[]
}

// Outlines the NPC overlay when the target matches (lib/npcOutlineRules).
// Tag and class conditions are each optional; when both are set both must
// pass. Always send every field — the config PUT decodes slices over their
// old elements, so an omitted field would inherit a stale value.
export interface NPCOutlineRule {
  id: string
  name: string
  enabled: boolean
  // How tags apply: the target has any / all / none of them.
  match: 'any' | 'all' | 'none'
  // Special-ability codes.
  tags: number[]
  // NPC class ids; the target's class must be one of them.
  classes: number[]
  color: string
  flash: boolean
}

// One character's own outline rules (character is lowercased).
export interface NPCOutlineRuleSet {
  character: string
  rules: NPCOutlineRule[]
}

// Item Info overlay summary-card setup. Mirrors backend
// config.ItemInfoOverlaySettings; key scheme and defaults in lib/itemInfoLayout.
export interface ItemInfoOverlaySettings {
  section_order?: string[] | null
  hidden_items?: string[] | null
  list_limit?: number
  section_limits?: Record<string, number> | null
  empty_modes?: Record<string, string> | null
  auto_show?: boolean
  auto_hide_secs?: number
  defaults_seeded?: boolean
}

export interface NPCOverlaySections {
  identity: boolean
  combat: boolean
  resists: boolean
  attributes: boolean
  special_abilities: boolean
  faction: boolean
  // behavior is the aggro radius / assist radius / attack delay section.
  behavior: boolean
  // spells is the master toggle for the caster-summary section (highlights are
  // always shown when on). The spells_* flags are per-group sub-toggles.
  spells: boolean
  spells_procs: boolean
  spells_signature: boolean
  spells_class: boolean
  // distance is the player→target distance readout and the caster spells'
  // in-reach markers (needs a Zeal build that reports target descriptors).
  distance: boolean
}

export const DEFAULT_NPC_OVERLAY_SECTIONS: NPCOverlaySections = {
  identity: true,
  combat: true,
  resists: true,
  attributes: true,
  special_abilities: true,
  faction: true,
  behavior: true,
  spells: true,
  spells_procs: true,
  spells_signature: true,
  spells_class: true,
  distance: true,
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
  type: 'play_sound' | 'text_to_speech' | 'overlay_text'
  sound_path: string
  volume: number
  tts_template: string
  voice: string
  tts_volume: number

  // Overlay-text fields, used only when type === 'overlay_text'. Mirror
  // TimerAlertThreshold's overlay-text fields exactly — the same
  // NotificationActionEditor renders both. text supports the same
  // {npc}/{spell} placeholder as tts_template.
  text?: string
  duration_secs?: number
  color?: string
  position?: { x: number; y: number } | null
  font_size?: number
  glow_color?: string
  font_family?: string
  align?: string
}

/**
 * Wishlist watcher: an alert fired when a wishlisted item's name appears
 * anywhere in the active character's log (loot lines, chat, raid calls, ...).
 * Mirrors backend config.WishlistWatchSettings.
 */
export interface WishlistWatchSettings {
  enabled: boolean
  // Extends alerts to every character's wishlist, not just the active
  // one — including no-drop items. Intended for swapping in an alt to loot
  // for another character (e.g. one camped outside a raid). Off by default.
  include_other_chars?: boolean
  // Alert text. {item} and {character} tokens expand to the matched item's
  // name and the wishlisting character. Empty = the backend default.
  template?: string
  // Suppresses a repeat alert for the SAME item within this window (seconds),
  // so a spammed auction line or echoed loot message doesn't fire repeatedly.
  // 0/missing = backend default (30s).
  cooldown_secs?: number
  overlay_enabled?: boolean
  overlay_color?: string
  overlay_duration_secs?: number
  tts_enabled?: boolean
  tts_voice?: string
  tts_volume?: number
  sound_enabled?: boolean
  sound_path?: string
  sound_volume?: number
}

/**
 * One saved StreamKit voice-overlay link — a name the user picked (e.g.
 * "Raid channel", "Guild Hangout") plus the URL Discord's StreamKit tool
 * generated for that one guild+channel. See Preferences.discord_voice_links/
 * discord_voice_active_link_id and issue #150.
 */
export interface DiscordVoiceLink {
  id: string
  name: string
  url: string
}

/**
 * One saved Discord incoming webhook — a name the user picked plus the URL
 * Discord issued in a channel's Integrations → Webhooks settings. Trigger
 * discord_webhook actions reference this by id (Action.webhook_id) rather
 * than storing the URL on the action itself, so exporting a trigger
 * category/pack never leaks it. See Preferences.discord_webhooks.
 */
export interface DiscordWebhook {
  id: string
  name: string
  url: string
}

export interface Preferences {
  overlay_opacity: number
  // The map drawing every surface opens in. 'external' is a map pack installed
  // in the EQ folder; the renderer falls back if one is not there, since a pack
  // can be removed between launches.
  map_style?: 'outline' | 'detailed' | 'external'
  // Keep map POI pins (zone lines, succor points, etc.) at full opacity
  // regardless of the depth window, instead of fading them with geometry
  // outside the focused height range. Off by default.
  map_poi_ignore_zfade?: boolean
  // Draw a faint arrow for each in-zone groupmate on the live map, from Zeal's
  // group positions. On by default (undefined is treated as true).
  map_show_group?: boolean
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
  // Output device for every trigger / alert sound and TTS (a Chromium
  // audiooutput deviceId); '' = system default. The label is the device name
  // at selection time, used to re-find it and to show it while unplugged.
  audio_output_device_id?: string
  audio_output_device_label?: string
  // Voice for any text_to_speech alert whose own voice field is empty
  // ("App default" in the editor). Empty = the OS default voice.
  default_tts_voice?: string
  // Global TTS speaking rate applied to every text_to_speech alert. 1.0 =
  // normal speed, higher = faster, lower = slower. Clamped 0.5–2.0 at
  // playback. 0/missing is treated as 1.0.
  tts_rate?: number
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
  default_overlay_text_align?: string
  // Default fading-soon alert armed on manually-added Custom Timers (the
  // overlay quick-add form lets the player toggle it off per timer). Disabled
  // by default. See TimerAlertPref.
  custom_timer_alert?: TimerAlertPref
  // Audio cue fired as each NPC respawn timer crosses `seconds` remaining
  // (0 = at "POP"). Applies to every respawn timer; disabled by default.
  respawn_alert?: TimerAlertPref
  // When true, a popped pinned respawn timer passes its pin to the next kill at
  // the same spot (proximity via the Zeal pipe, else same name). Off by
  // default — the match is a guess and can pin the wrong mob.
  respawn_pin_handoff?: boolean
  // Per-spell overrides for native Detrimental timers, keyed by spell name:
  // "mute" skips the default fading-soon alert for that spell, "hide" never
  // shows its timer. See lib/detrimOverrides.
  detrim_spell_overrides?: Record<string, 'mute' | 'hide'>
  // Default fading-soon alert applied to every auto-detected Detrimental
  // timer (debuff/dot/mez/stun) that has no per-trigger alert of its own —
  // i.e. native spell-timer timers on any mob, raid or not. Trigger-driven
  // detrimental timers keep their own alert. Disabled by default.
  detrim_timer_alert?: TimerAlertPref
  // CH Metronome audio cues: metronome_start_alert fires when the personal
  // countdown starts (the watched chain slot ahead of you calls), and
  // metronome_cast_alert fires at "cast now". Independent, disabled by
  // default. The `seconds` field is unused for these (no threshold concept).
  metronome_start_alert?: TimerAlertPref
  metronome_cast_alert?: TimerAlertPref
  developer_mode: boolean
  // Verbose backend logging: raises server.log to debug level (log-file
  // reads/offsets, trigger fires, …) for diagnosing intermittent issues.
  // Off by default; toggled in Settings → Advanced → Diagnostics.
  debug_logging?: boolean
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
  npc_overlay_dashboard_sections: NPCOverlaySections
  npc_overlay_popout_sections: NPCOverlaySections
  // Individual NPC overlay items hidden on both surfaces (keys from
  // lib/npcItemFilters, e.g. "ability.10", "combat.atk"), plus per-character
  // overrides keyed by lowercased name — a present key replaces the global list.
  npc_overlay_hidden_items?: string[]
  npc_overlay_hidden_items_by_character?: Record<string, string[]>
  // npc_overlay_cast_range: the spell range the NPC overlay's distance readout
  // is coloured against (backend normalises <= 0 to 200).
  // npc_overlay_ranged_range: optional second range (e.g. a bow's); 0 = off.
  npc_overlay_cast_range: number
  npc_overlay_ranged_range: number
  // npc_overlay_ranges_manual: use the two ranges above as typed. Off, the
  // overlay works them out from the active character's gear instead.
  npc_overlay_ranges_manual?: boolean
  // Special-ability badge colour overrides for both NPC overlay surfaces.
  npc_overlay_tag_style?: NPCOverlayTagStyle
  // Stats-view section order (lib/npcOverlayLayout ids) and special-ability
  // codes to show first, shared by both surfaces. Empty/absent = built-in order.
  npc_overlay_section_order?: string[]
  npc_overlay_tag_order?: number[]
  // Outline the NPC overlay when the target matches a rule (first enabled
  // match wins), plus per-character replacement lists.
  npc_overlay_outline_rules?: NPCOutlineRule[]
  npc_overlay_outline_rule_sets?: NPCOutlineRuleSet[]
  // Per-overlay locked behaviour, keyed by canonical overlay name. Missing
  // keys default to "interactive". See lib/overlays.ts.
  overlay_locked_modes?: Partial<Record<OverlayName, LockedMode>>
  // Per-overlay override of overlay_fade_enabled ('on' / 'off'). Missing keys
  // (or '') follow the global toggle. See lib/overlays.ts resolveFadeEnabled.
  overlay_fade_modes?: Partial<Record<OverlayName, FadeMode | ''>>
  // Per-overlay override of overlay_opacity (0–1). Missing keys, or a
  // negative value ("Default"), follow the global setting. See lib/overlays.ts
  // resolveOverlayOpacity.
  overlay_opacities?: Partial<Record<OverlayName, number>>
  // Named custom Roll Tracker grouping profiles, listed with the presets so
  // a custom setup survives switching to Simple and back.
  roll_tracker_saved_profiles?: SavedRollProfile[]
  // Side-nav route keys the user has hidden from the navigation menu (the
  // page is still reachable by URL). The fixed controls are never hideable.
  sidebar_hidden?: string[]
  // Flat list of side-nav route keys in preferred display order; items are
  // ordered within their section by position here.
  sidebar_order?: string[]
  // Side-nav route keys the user has starred as favorites; these render in a
  // pinned "Favorites" section at the top of the sidebar in addition to their
  // normal section. A hidden item is never shown, even if favorited.
  sidebar_favorites?: string[]
  // ── Local TTS (Piper) ──────────────────────────────────────────────────
  // Opt-in, user-installed local neural TTS (see docs/piper-tts-plan.md).
  // When enabled and pointed at a valid piper executable + .onnx voice model,
  // a trigger/alert voice of PIPER_VOICE_ID ("piper:local") is synthesized to
  // a cached WAV by the backend and played through the normal sound pipeline;
  // any failure falls back to Web Speech. All fields default off/empty.
  piper_enabled?: boolean
  piper_exe_path?: string
  piper_model_path?: string
  // "spawn" (default, a fresh subprocess per phrase) | "warm" (Phase 2, one
  // persistent subprocess kept alive across requests). Empty/missing =
  // "spawn".
  piper_mode?: string
  // ── Local TTS (Kokoro) ─────────────────────────────────────────────────
  // Second opt-in local neural TTS provider, alongside Piper — a self-hosted
  // Kokoro-FastAPI server the user runs themselves (e.g. `docker run`),
  // reached over HTTP at kokoro_base_url. Unlike Piper there's no exe/model
  // file to point at; the voice catalog is fetched live from the service.
  // When enabled and reachable, a trigger/alert voice of KOKORO_VOICE_ID
  // ("kokoro:local") is synthesized to a cached WAV by the backend and
  // played through the normal sound pipeline; any failure falls back to Web
  // Speech. All fields default off/empty.
  kokoro_enabled?: boolean
  // e.g. "http://localhost:8880". Empty defaults to that same address.
  kokoro_base_url?: string
  // The API voice id, e.g. "af_heart". Empty defaults to "af_heart".
  kokoro_voice?: string
  // The API's speed multiplier. Empty/zero defaults to 1.0.
  kokoro_speed?: number
  // Wishlist watcher settings. See WishlistWatchSettings.
  wishlist_watch: WishlistWatchSettings
  // ── Discord Voice overlay ──────────────────────────────────────────────
  // Embeds Discord's own hosted StreamKit Overlay page in a popout overlay
  // window (see issue #150) — we never talk to Discord's API ourselves, the
  // user generates each link via streamkit.discord.com/overlay and pastes it
  // in. Each link is pinned to one guild+channel, so instead of a single URL
  // the user can save several named links and pick which is active — see
  // DiscordVoiceLink. Off by default.
  discord_voice_enabled?: boolean
  discord_voice_links?: DiscordVoiceLink[]
  discord_voice_active_link_id?: string
  // By default the header AND content area both follow overlay_opacity/
  // overlay_fade_enabled exactly like every other overlay, for visual
  // consistency. true is an opt-in "clean" look for once the window is
  // positioned where it's wanted: both the header and the background tint
  // fade to fully transparent permanently (ignoring overlay_opacity), leaving
  // only the live Discord avatars/names visible — the header's lock/close
  // controls stay put with no backdrop behind them. Off by default.
  discord_voice_minimal_mode?: boolean
  // Saved Discord incoming webhooks, referenced by id from trigger
  // discord_webhook actions. See DiscordWebhook.
  discord_webhooks?: DiscordWebhook[]
  // Set by the backend the first time Zeal sends an item display pipe event
  // (a proposed Zeal message, not yet released). Reveals the Item Info
  // overlay; never edited by the user.
  zeal_item_display_seen?: boolean
  item_info_overlay?: ItemInfoOverlaySettings
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
   * When true, hide buff/detrimental overlay rows whose caster_character
   * doesn't match the currently active character. Rows with no recorded
   * caster are always shown. Switching characters never clears the
   * underlying timers — this only affects what's displayed, so switching
   * back to the casting character brings hidden rows right back. Off by
   * default.
   */
  hide_other_character_timers?: boolean

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
   * How long (minutes) a kept-expired row lingers before it's dropped, for
   * the Buff and Detrimental overlays. 0/absent = 60. Only used when
   * keep_expired_timers is on.
   */
  buff_expired_keep_mins?: number
  detrim_expired_keep_mins?: number

  /**
   * Per-spell linger time in minutes, keyed by spell name, overriding the
   * Buff/Detrimental values above. 0 drops that spell as soon as it expires.
   */
  expired_keep_overrides?: Record<string, number>

  /**
   * Timer overlay bar fill: '' / 'faded' (default ~15% tint), 'solid'
   * (~55% fill), 'none' (no fill — just the countdown text), or 'custom'
   * (use the exact percentage in timer_bar_fill_pct).
   */
  timer_bar_fill?: 'faded' | 'solid' | 'none' | 'custom'
  /** Bar fill opacity 0-100, used only when timer_bar_fill is 'custom'. */
  timer_bar_fill_pct?: number

  /** Timer row spell-name font size in px. 0/absent = default (12). */
  timer_name_font_size?: number
  /** Timer row countdown font size in px. 0/absent = default (11). */
  timer_time_font_size?: number
  /** Timer row vertical padding in px (controls row height). 0/absent = 3. */
  timer_row_padding?: number

  /**
   * When true, timer overlay rows stack upward from the bottom of the
   * window (new rows appear at the bottom, pushing older ones up) instead
   * of downward from underneath the header. Off by default.
   */
  timer_bar_stack_from_bottom?: boolean

  /**
   * When true, a timer row's spell-name/countdown text stays white instead
   * of switching to red once the timer drops under 20% remaining. Off by
   * default (red is the long-standing look).
   */
  timer_bar_urgent_text_white?: boolean

  /**
   * When true, buff/detrimental timer countdowns show minutes and seconds
   * (e.g. "4m 32s") instead of the default rounded-up minutes-only display
   * ("5m"). Off by default.
   */
  timer_show_seconds?: boolean

  /**
   * When true, each timer row's mute / hide / remove buttons stay visible
   * instead of appearing only while the row is hovered. Off by default.
   */
  timer_row_controls_always_show?: boolean
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
  // Cast-begin correlation: flags a chain bar red ("possible miss") when its
  // callout's caster is never observed starting a cast ("begins to cast a
  // spell" bystander line, or "You begin casting …" for your own) before the
  // 10s cast window elapses. Purely additive — never affects chain-call
  // matching or timer creation. Class-agnostic: never inspects which spell
  // was cast, so it covers Cleric Complete Healing and Druid Tunare's/
  // Karana's Renewal identically.
  possible_miss_enabled?: boolean
  // Refines possible_miss_enabled: un-confirms an already-confirmed chain
  // bar (and re-flags it red) if that caster is then observed being
  // interrupted before the cast would have landed. Off by default — only
  // takes effect when possible_miss_enabled is also true, and never changes
  // existing confirm/expiry behavior when left off.
  interrupt_detection_enabled?: boolean
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

// How Archive & Trim shortens the live log: keep the last keep_days days of
// entries, or the newest keep_mb megabytes. warn_mb is the size at which the
// log is flagged as large; a negative value disables the warning.
export interface LogTrimSettings {
  mode: 'days' | 'size'
  keep_days: number
  keep_mb: number
  warn_mb: number
}

export const DEFAULT_LOG_TRIM: LogTrimSettings = {
  mode: 'days',
  keep_days: 30,
  keep_mb: 50,
  warn_mb: 75,
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
  // Settings > Logs Archive & Trim window and large-log warning threshold.
  log_trim: LogTrimSettings
  // App version the user last saw the "what's new" popup for. Empty means
  // "never stamped" (fresh install) — the popup is skipped in that case.
  last_seen_changelog_version: string
  // True once the user checks "Don't show this again" on the popup, or
  // flips the equivalent Settings > Changelog toggle.
  changelog_popup_disabled: boolean
}

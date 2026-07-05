// Package config manages the PQ Companion user configuration file.
// The config is stored as YAML at ~/.pq-companion/config.yaml and can be
// read and updated via the REST API.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all user-configurable settings.
type Config struct {
	// EQPath is the path to the EverQuest installation directory.
	// Used to locate log files and Zeal export files.
	EQPath string `yaml:"eq_path" json:"eq_path"`

	// Character is the active character name (no server suffix).
	// Used to locate the correct log file: eqlog_<Character>_pq.proj.txt
	Character string `yaml:"character" json:"character"`

	// CharacterClass is the EverQuest class index (0–14) for the active
	// character. Used to auto-select the correct class in the Spell Checklist.
	// 0=WAR 1=CLR 2=PAL 3=RNG 4=SHD 5=DRU 6=MNK 7=BRD 8=ROG 9=SHM
	// 10=NEC 11=WIZ 12=MAG 13=ENC 14=BST   -1 = not set
	CharacterClass int `yaml:"character_class" json:"character_class"`

	// ServerAddr is the HTTP listen address for the backend API.
	ServerAddr string `yaml:"server_addr" json:"server_addr"`

	// Preferences holds UI and overlay preferences.
	Preferences Preferences `yaml:"preferences" json:"preferences"`

	// Backup holds backup manager settings.
	Backup BackupSettings `yaml:"backup" json:"backup"`

	// SpellTimer holds spell timer engine settings.
	SpellTimer SpellTimerSettings `yaml:"spell_timer" json:"spell_timer"`

	// CHChain configures the Complete-Heal-chain overlay matcher.
	CHChain CHChainSettings `yaml:"ch_chain" json:"ch_chain"`

	// Combat holds combat tracker / history settings.
	Combat CombatSettings `yaml:"combat" json:"combat"`

	// DPSClassColors maps each EQ class to the bar colour the DPS meter and
	// combat-history rows render for combatants of that class. Stored as
	// CSS-style #RRGGBB hex strings so the frontend can apply them
	// verbatim. Unknown / unclassified combatants fall back to Unknown.
	DPSClassColors DPSClassColors `yaml:"dps_class_colors" json:"dps_class_colors"`

	// OnboardingCompleted is true once the user has finished the first-launch
	// setup wizard. When false (default), the wizard is shown on app launch.
	OnboardingCompleted bool `yaml:"onboarding_completed" json:"onboarding_completed"`

	// ChatRetentionDays is how many days of Chat History to keep before the
	// daily purge deletes older messages. Default 30. A negative value (e.g.
	// -1) keeps chat forever. As with Combat.RetentionDays, the YAML decoder
	// can't tell a missing field from an explicit 0, so applyDefaults treats 0
	// as "use the default"; "keep forever" is stored as -1.
	ChatRetentionDays int `yaml:"chat_retention_days" json:"chat_retention_days"`
}

// BackupSettings configures automatic backup behaviour.
type BackupSettings struct {
	// AutoBackup enables file-watcher-triggered backups when *.ini files change.
	AutoBackup bool `yaml:"auto_backup" json:"auto_backup"`

	// Schedule controls periodic backups: "off", "hourly", or "daily".
	Schedule string `yaml:"schedule" json:"schedule"`

	// MaxBackups is the maximum number of backups to retain (0 = unlimited).
	// When exceeded, the oldest unlocked backups are deleted automatically.
	MaxBackups int `yaml:"max_backups" json:"max_backups"`
}

// SpellTimerSettings configures the spell timer engine.
type SpellTimerSettings struct {
	// TrackingScope controls which spell-landed events become timers.
	//
	//	"self"       — only spells where the recipient is the active player.
	//	"cast_by_me" — every land where the active character is the caster
	//	               (default). Heuristic: a land within lastCastWindow of
	//	               our most recent EventSpellCast for the same spell name.
	//	               Filters out the noise of other players buffing each
	//	               other while still showing buffs we put on group/raid.
	//	"anyone"     — every spell that lands on a recognised target. Useful
	//	               when you want to see e.g. another enchanter's Tash on
	//	               a raid mob.
	//
	// An empty string is treated as "cast_by_me" so existing config files
	// without this field migrate to the new default.
	TrackingScope string `yaml:"tracking_scope" json:"tracking_scope"`

	// BuffDisplayThresholdSecs hides buff overlay rows whose remaining time
	// exceeds this value. 0 (default) means always show. Useful for raid
	// buff tracking where dozens of long-duration buffs would otherwise
	// drown the overlay — set to e.g. 600 to only see buffs in the last 10
	// minutes of their duration.
	BuffDisplayThresholdSecs int `yaml:"buff_display_threshold_secs" json:"buff_display_threshold_secs"`

	// DetrimDisplayThresholdSecs is the corresponding cap for the
	// Detrimental overlay. 0 (default) means always show. Detrimentals are
	// usually short-lived so the default of 0 matches existing behaviour.
	DetrimDisplayThresholdSecs int `yaml:"detrim_display_threshold_secs" json:"detrim_display_threshold_secs"`

	// CastByMeMigrationDone records that the one-time migration from the
	// old "anyone" default to "cast_by_me" has run for this config file.
	// Once true, the user's explicit "anyone" choice is preserved on load.
	CastByMeMigrationDone bool `yaml:"cast_by_me_migration_done,omitempty" json:"cast_by_me_migration_done,omitempty"`

	// ClassFilter, when true, drops buff timers whose source spell isn't
	// castable by the active character's class. Useful for hiding
	// out-of-class buffs (e.g. paladin Spiritual Purity, shaman Talisman,
	// bard songs) from a class with a long buff list of its own. Composes
	// with TrackingScope: detrimentals are always allowed regardless.
	// Defaults to false to preserve existing behaviour.
	ClassFilter bool `yaml:"class_filter,omitempty" json:"class_filter,omitempty"`

	// TrackingMode controls whether the spell-landed pipeline creates timer
	// rows automatically or only triggers/packs do.
	//
	//	"auto"          — default. Every recognised spell-landed event creates
	//	                  a timer; triggers/packs may attach metadata
	//	                  (thresholds, fading-soon TTS) by firing on the same
	//	                  cast.
	//	"triggers_only" — the spell-landed pipeline still parses log lines
	//	                  for cast disambiguation but does NOT create timers.
	//	                  Only triggers (custom or pack-installed) produce
	//	                  rows. Best for users who want a curated overlay
	//	                  rather than full auto-coverage.
	//
	// Empty string is treated as "auto" so existing config files migrate
	// cleanly without an explicit value.
	TrackingMode string `yaml:"tracking_mode,omitempty" json:"tracking_mode,omitempty"`

	// KeepExpiredTimers, when true, keeps a buff/detrimental row in the
	// overlay after it expires — shown as an overdue count-up — instead of
	// dropping it. The row lingers until the spell is recast (refreshing the
	// timer) or the user dismisses it with the per-row X, so the overlay
	// doubles as a "buffs I still need to refresh" checklist. Off by default;
	// the standard behaviour is to remove a timer the moment it expires.
	KeepExpiredTimers bool `yaml:"keep_expired_timers,omitempty" json:"keep_expired_timers"`

	// TimerBarFill controls the timer overlay row's bar fill: "" / "faded"
	// (the default ~15% tint), "solid" (a stronger ~55% fill), or "none"
	// (no fill — just the countdown text on the transparent overlay). Purely
	// cosmetic and resolved in the frontend; the engine ignores it.
	TimerBarFill string `yaml:"timer_bar_fill,omitempty" json:"timer_bar_fill,omitempty"`

	// TimerNameFontSize / TimerTimeFontSize / TimerRowPadding override the
	// timer overlay row's spell-name font size, countdown font size, and
	// vertical row padding (in px). 0 (default) means "use the built-in
	// default" (12 / 11 / 3). Frontend-only; the engine never reads them.
	TimerNameFontSize int `yaml:"timer_name_font_size,omitempty" json:"timer_name_font_size,omitempty"`
	TimerTimeFontSize int `yaml:"timer_time_font_size,omitempty" json:"timer_time_font_size,omitempty"`
	TimerRowPadding   int `yaml:"timer_row_padding,omitempty" json:"timer_row_padding,omitempty"`
}

// DPSClassColors holds the user-customisable bar colours for the DPS meter
// and combat history rows. One hex value per EQ class plus an Unknown
// fallback for combatants whose class can't be resolved.
type DPSClassColors struct {
	Warrior      string `yaml:"warrior" json:"warrior"`
	Cleric       string `yaml:"cleric" json:"cleric"`
	Paladin      string `yaml:"paladin" json:"paladin"`
	Ranger       string `yaml:"ranger" json:"ranger"`
	ShadowKnight string `yaml:"shadow_knight" json:"shadow_knight"`
	Druid        string `yaml:"druid" json:"druid"`
	Monk         string `yaml:"monk" json:"monk"`
	Bard         string `yaml:"bard" json:"bard"`
	Rogue        string `yaml:"rogue" json:"rogue"`
	Shaman       string `yaml:"shaman" json:"shaman"`
	Necromancer  string `yaml:"necromancer" json:"necromancer"`
	Wizard       string `yaml:"wizard" json:"wizard"`
	Magician     string `yaml:"magician" json:"magician"`
	Enchanter    string `yaml:"enchanter" json:"enchanter"`
	Beastlord    string `yaml:"beastlord" json:"beastlord"`
	Unknown      string `yaml:"unknown" json:"unknown"`
}

// DefaultDPSClassColors is the seeded palette new installs (and any field
// that's been left blank on an existing config) fall back to.
func DefaultDPSClassColors() DPSClassColors {
	return DPSClassColors{
		Warrior:      "#C79C6E",
		Cleric:       "#FFFFFF",
		Paladin:      "#F58CBA",
		Ranger:       "#ABD473",
		ShadowKnight: "#C41F3B",
		Druid:        "#FF7D0A",
		Monk:         "#00FF96",
		Bard:         "#8A47E8",
		Rogue:        "#FFF569",
		Shaman:       "#0070DE",
		Necromancer:  "#9482C9",
		Wizard:       "#40ED57",
		Magician:     "#69CCF0",
		Enchanter:    "#ED5CE5",
		Beastlord:    "#03B78A",
		Unknown:      "#B2B2B2",
	}
}

// CombatSettings configures the combat tracker and persisted fight history.
type CombatSettings struct {
	// RetentionDays is the number of days of combat history to keep in
	// user.db. Fights older than this are pruned on startup and once per
	// hour. 0 (or negative) disables pruning entirely. Default 30 — covers
	// the common "let me see last week's raid" case without growing the DB
	// beyond a few MB for normal use.
	RetentionDays int `yaml:"retention_days" json:"retention_days"`

	// FightTimeoutSeconds is the inactivity window, in seconds, after which an
	// active fight that has recorded damage is archived and dropped from the
	// live meter. This is now the primary way a parse ends: zoning and player
	// death no longer force-clear fights (a wizard who zones to drop aggro, or
	// dies and gets rezzed, keeps the same parse), so the encounter ends only
	// on the mob's death, the user's manual discard, or this timeout. Raid-
	// scale encounters still use at least the built-in raid window even when
	// this is set lower, since raid bosses have long mechanic-driven lulls.
	// 0 (or missing) means "use the default" (60) — long enough to survive a
	// zone-out / run-back cycle without splitting the fight.
	FightTimeoutSeconds int `yaml:"fight_timeout_seconds" json:"fight_timeout_seconds"`
}

// DefaultFightTimeoutSeconds is the inactivity window applied to an active
// fight (with damage) when the user hasn't set one. Bumped past the old hard-
// coded 30s so a zone round-trip to drop aggro keeps the parse alive.
const DefaultFightTimeoutSeconds = 60

// TrackingScope* are the canonical values for SpellTimerSettings.TrackingScope.
const (
	TrackingScopeSelf     = "self"
	TrackingScopeCastByMe = "cast_by_me"
	TrackingScopeAnyone   = "anyone"
)

// TrackingMode* are the canonical values for SpellTimerSettings.TrackingMode.
const (
	TrackingModeAuto         = "auto"
	TrackingModeTriggersOnly = "triggers_only"
)

// Preferences holds optional UI and overlay settings.
type Preferences struct {
	// OverlayOpacity is the transparency of overlay windows (0.0–1.0).
	OverlayOpacity float64 `yaml:"overlay_opacity" json:"overlay_opacity"`

	// OverlayFadeEnabled fades overlay chrome (background, border, title
	// bar) to fully transparent a few seconds after the cursor leaves an
	// overlay window, leaving only the content (timer bars, NPC stats)
	// visible. Hovering the overlay restores OverlayOpacity instantly.
	// Off by default.
	OverlayFadeEnabled bool `yaml:"overlay_fade_enabled,omitempty" json:"overlay_fade_enabled"`

	// OverlayFadeDelaySecs is how long after the cursor leaves an overlay
	// window the chrome fade kicks in, in seconds. Only meaningful while
	// OverlayFadeEnabled is on. 0 is treated as the default (2.5) by the
	// client, mirroring the ZoomFactor convention.
	OverlayFadeDelaySecs float64 `yaml:"overlay_fade_delay_secs,omitempty" json:"overlay_fade_delay_secs"`

	// PVPWarningDisabled mutes the sound + on-screen warning fired when a
	// PVP-flagged player shows up in a /who or joins the group. Stored
	// inverted so the zero value means "warning on" — flagging a player is
	// already opt-in, so the warning defaults to enabled without needing a
	// default-true migration in applyDefaults. Flags in the tracker are
	// unaffected; only the alert is muted.
	PVPWarningDisabled bool `yaml:"pvp_warning_disabled,omitempty" json:"pvp_warning_disabled"`

	// MinimizeToTray controls whether closing the main window hides to tray.
	MinimizeToTray bool `yaml:"minimize_to_tray" json:"minimize_to_tray"`

	// HighContrast raises the contrast of muted text and borders for
	// readability on high-resolution displays (issue #130). Off by default.
	HighContrast bool `yaml:"high_contrast,omitempty" json:"high_contrast"`

	// ZoomFactor scales the entire main window via Electron's zoom (like a
	// browser Ctrl+/Ctrl-), for readability on high-resolution displays
	// (issue #130). 1.0 = 100% (default). 0 is treated as 1.0 by the client.
	ZoomFactor float64 `yaml:"zoom_factor,omitempty" json:"zoom_factor"`

	// OverlayZoomFactors scales individual popout overlay windows via the same
	// Electron zoom, keyed by canonical overlay name (see frontend
	// lib/overlays.ts). Lets the NPC info / timer / meter overlays be sized
	// independently of the main window for different play resolutions. Missing
	// keys / 0 are treated as 1.0 (100%) by the client.
	OverlayZoomFactors map[string]float64 `yaml:"overlay_zoom_factors,omitempty" json:"overlay_zoom_factors"`

	// ParseCombatLog enables real-time combat log parsing.
	ParseCombatLog bool `yaml:"parse_combat_log" json:"parse_combat_log"`

	// OverlayDPSEnabled controls whether the DPS floating overlay is shown.
	OverlayDPSEnabled bool `yaml:"overlay_dps_enabled" json:"overlay_dps_enabled"`

	// OverlayHPSEnabled controls whether the HPS floating overlay is shown.
	OverlayHPSEnabled bool `yaml:"overlay_hps_enabled" json:"overlay_hps_enabled"`

	// OverlayEntityLinksEnabled makes entity rows in overlay windows (e.g. the
	// NPC info overlay's loot items and castable spells) clickable links that
	// focus the main window and open that item/spell in the database explorer.
	// On by default; users who don't want a click pulling focus out of the game
	// can turn it off, leaving those rows as plain text. Not omitempty so the
	// default-true value survives a round-trip once written, matching
	// minimize_to_tray.
	OverlayEntityLinksEnabled bool `yaml:"overlay_entity_links_enabled" json:"overlay_entity_links_enabled"`

	// MasterVolume scales every trigger / event / timer alert sound and TTS
	// utterance multiplicatively on top of its per-action volume. Stored as
	// 0–100 (percent); the frontend converts to 0.0–1.0 at playback time.
	// 100 = no dampening (default), 0 = mute everything.
	MasterVolume int `yaml:"master_volume" json:"master_volume"`

	// DefaultTTSVoice is the voice used for any text_to_speech alert whose
	// own voice field is empty ("App default" in the editor). It names a
	// SpeechSynthesisVoice on the user's machine; the frontend resolves it at
	// playback time. Empty = the OS default voice (pre-existing behaviour).
	DefaultTTSVoice string `yaml:"default_tts_voice,omitempty" json:"default_tts_voice"`

	// TTSRate is the global speaking rate for every text_to_speech alert,
	// multiplying the voice's normal speed: 1.0 = normal, higher = faster,
	// lower = slower. The frontend applies it as SpeechSynthesisUtterance.rate
	// and clamps to 0.5–2.0 (the range the Settings slider exposes). 0 or
	// unset falls back to 1.0 at playback time, so existing configs speak at
	// normal speed. Global, not per-trigger.
	TTSRate float64 `yaml:"tts_rate,omitempty" json:"tts_rate"`

	// TriggerAudioCooldownSecs rate-limits repeat audio alerts from the SAME
	// trigger: after a trigger plays a sound or TTS, further audio from that
	// same trigger is suppressed for this many seconds. It collapses rapid
	// same-trigger bursts (e.g. an AE mez wearing off several mobs at once →
	// one alert instead of five) without touching the overlay-text, history,
	// or spell-timer pipelines — those still fire per match. Applied per
	// trigger id in the frontend audio engine (useAudioEngine). 0 (default)
	// disables it: every fire plays, the pre-existing behaviour. Experimental
	// and self-contained — removing the feature means deleting this field, the
	// audio-engine gate, and the Settings input.
	TriggerAudioCooldownSecs float64 `yaml:"trigger_audio_cooldown_secs,omitempty" json:"trigger_audio_cooldown_secs"`

	// DefaultOverlayPosition anchors trigger overlay_text alerts that have no
	// per-trigger pinned position at a fixed on-screen point (alerts stack
	// downward from it). Coordinates are window-local pixels on the trigger
	// overlay's chosen monitor, same space as trigger.ActionPosition. Nil =
	// the pre-existing behaviour (centered stack).
	DefaultOverlayPosition *OverlayPosition `yaml:"default_overlay_position,omitempty" json:"default_overlay_position,omitempty"`

	// DefaultOverlayTextColor / GlowColor / FontFamily / FontSize style every
	// trigger overlay_text alert whose own action leaves the matching field
	// unset ("App default" in the editor). Per-action values always win. Zero
	// values mean "no global default" and fall back to the renderer built-ins
	// (white text, glow derived from the text color, system-ui, 20px), which
	// is exactly the pre-existing look — so upgrading users see no change and
	// no migration is needed.
	DefaultOverlayTextColor  string `yaml:"default_overlay_text_color,omitempty" json:"default_overlay_text_color"`
	DefaultOverlayGlowColor  string `yaml:"default_overlay_glow_color,omitempty" json:"default_overlay_glow_color"`
	DefaultOverlayFontFamily string `yaml:"default_overlay_font_family,omitempty" json:"default_overlay_font_family"`
	DefaultOverlayFontSize   int    `yaml:"default_overlay_font_size,omitempty" json:"default_overlay_font_size"`

	// CustomTimerAlert is the default audio cue attached to manually-added
	// Custom Timer countdowns. When Enabled, the Custom Timers overlay's
	// quick-add form arms a fading-soon alert on each new timer (the player
	// can toggle it off per timer with the bell button). Trigger-driven
	// custom timers carry their own alerts and are unaffected. Nil/omitted by
	// default — manual timers stay silent as before. A pointer so an unset
	// field is dropped from JSON entirely; a zero struct would otherwise
	// serialize with an empty Type and read client-side as a misconfigured
	// alert instead of "not configured".
	CustomTimerAlert *TimerAlertPref `yaml:"custom_timer_alert,omitempty" json:"custom_timer_alert,omitempty"`

	// RespawnAlert announces NPC respawns: when Enabled, an audio cue fires as
	// each respawn timer crosses Seconds remaining (0 = at "POP"). Applies to
	// every auto-generated respawn timer; there is no per-timer toggle since
	// they aren't added by hand. Nil/omitted (disabled) by default; pointer for
	// the same reason as CustomTimerAlert.
	RespawnAlert *TimerAlertPref `yaml:"respawn_alert,omitempty" json:"respawn_alert,omitempty"`

	// DeveloperMode reveals the Developer tab in the Settings page, which
	// hosts power-user tools (SQL sandbox, schema viewer). Toggled in-app
	// via the Ctrl+Shift+D shortcut while the Settings page is focused.
	// Off by default so casual users don't stumble into raw-DB territory.
	DeveloperMode bool `yaml:"developer_mode,omitempty" json:"developer_mode"`

	// DebugLogging raises the backend log level from Info to Debug, writing
	// verbose diagnostics (log-file reads/offsets, trigger matches, duplicate-
	// line guard drops, …) to ~/.pq-companion/logs/server.log. Off by default
	// to keep the log readable; users turn it on from Settings → Advanced →
	// Diagnostics when reproducing an intermittent issue, then hand back the
	// log. Applied to applog at startup and on every config save (no restart).
	DebugLogging bool `yaml:"debug_logging,omitempty" json:"debug_logging"`

	// PoPEnabled switches the app into Planes of Power era: level cap 65
	// instead of 60, PoP spells in the class spell lists, the PoP AA tabs,
	// and Plane of Knowledge as a shopping-route source. Off by default
	// because the expansion isn't live on Project Quarm yet — until launch
	// it's a preview toggle in the Developer tab (see internal/era). When
	// PoP launches the default flips in a release and this becomes a no-op.
	PoPEnabled bool `yaml:"pop_enabled,omitempty" json:"pop_enabled"`

	// ThreatHatemodPct is a MANUAL hate modifier, as a signed percentage, that the
	// Threat Meter adds to its spell- and heal-hate. The Spell Casting Subtlety AA
	// is now auto-detected from the character's trained AAs, so this is for sources
	// the logs/DB can't reveal (e.g. hate-reduction gear) — leave it 0 unless you
	// have such an effect, and do NOT re-enter Spell Casting Subtlety here.
	ThreatHatemodPct int `yaml:"threat_hatemod_pct,omitempty" json:"threat_hatemod_pct"`

	// RaidThreatEnabled controls the raid-estimate mode of the Threat Meter (a
	// per-mob, per-player ESTIMATED hate view derived from observed damage).
	// Experimental and OFF by default: it carries the DPS meter's accuracy
	// limits plus threat-specific blind spots (DoTs/heals/taunts/out-of-range),
	// which mislead in many raid setups. Gated behind a Developer-tab flag —
	// enabling it there adds the Solo/Raid toggle to the Threat overlay and the
	// per-class/player tuning card in Settings > Overlays.
	RaidThreatEnabled bool `yaml:"raid_threat_enabled,omitempty" json:"raid_threat_enabled"`

	// RaidThreatMigratedOff marks the one-time migration that turned the raid-
	// estimate threat mode off (it previously defaulted on) when it became an
	// experimental, Developer-tab-gated feature. The marker keeps the migration
	// from re-disabling a user who deliberately re-enables raid mode.
	RaidThreatMigratedOff bool `yaml:"raid_threat_migrated_off,omitempty" json:"raid_threat_migrated_off,omitempty"`

	// RaidThreatClassMods is the per-class hate adjustment (class name → signed
	// percent) the raid-estimate view applies to a player's observed damage, to
	// offset hate sources logs can't see (a tank's taunt/disciplines/+hate gear).
	// A present entry (including 0) overrides the built-in default; tank classes
	// default to +30 when unset. May be nil/omitted.
	RaidThreatClassMods map[string]int `yaml:"raid_threat_class_mods,omitempty" json:"raid_threat_class_mods,omitempty"`

	// RaidThreatPlayerMods is an optional per-player hate adjustment (player
	// name → signed percent) added on top of the class mod, for known cases
	// (e.g. a player running a hate-amp buff). May be nil/omitted.
	RaidThreatPlayerMods map[string]int `yaml:"raid_threat_player_mods,omitempty" json:"raid_threat_player_mods,omitempty"`

	// PoPFlagsEnabled gates the Planes of Power flagging tracker (per-character
	// planar-progression checklist + dependency graph). Dev-gated, off by
	// default — PoP isn't live on Project Quarm yet, so this is a preview/build-
	// ahead feature whose live-detection paths can only be tuned post-release.
	PoPFlagsEnabled bool `yaml:"pop_flags_enabled,omitempty" json:"pop_flags_enabled"`

	// NPCOverlayDashboardSections controls which optional sections of the
	// NPC overlay are visible in the dashboard panel. Name, zone, pet
	// owner, raid/rare badges, and the HP bar are always shown regardless
	// of these toggles.
	NPCOverlayDashboardSections NPCOverlaySections `yaml:"npc_overlay_dashboard_sections" json:"npc_overlay_dashboard_sections"`

	// NPCOverlayPopoutSections is the same toggle set applied to the
	// floating popout overlay window, so users can run a denser surface
	// in one place and a sparser one in the other.
	NPCOverlayPopoutSections NPCOverlaySections `yaml:"npc_overlay_popout_sections" json:"npc_overlay_popout_sections"`

	// NPCFactionSectionMigrationDone marks that the one-time migration which
	// turns the (later-added) Faction section on for pre-existing configs has
	// run. Without it, configs saved before the Faction field existed would
	// deserialize Faction=false and never show the new section even though it
	// defaults on for fresh installs. See applyDefaults.
	NPCFactionSectionMigrationDone bool `yaml:"npc_faction_section_migration_done,omitempty" json:"npc_faction_section_migration_done"`

	// NPCSpellsSectionMigrationDone marks that the one-time migration which turns
	// the (later-added) Spells & Abilities section — and its sub-toggles — on for
	// pre-existing configs has run. Same rationale as the Faction marker above:
	// a bool can't distinguish "missing" from an explicit false, so without this
	// upgrading users would never see the new section. See applyDefaults.
	NPCSpellsSectionMigrationDone bool `yaml:"npc_spells_section_migration_done,omitempty" json:"npc_spells_section_migration_done"`

	// OverlayLockedModes maps each popout overlay (by its canonical name:
	// "dps", "hps", "buffTimer", "detrimTimer", "npc", "rollTracker",
	// "respawnTimer") to how it behaves while locked:
	//   "interactive"  — hover the overlay to scroll / clear individual rows;
	//                     move off and clicks pass through to the game (default)
	//   "clickthrough" — only the title-bar buttons are clickable; scrolling
	//                     and clicks everywhere else pass through to the game
	// Missing keys default to "interactive" (the pre-existing behaviour), so
	// the map can be nil/omitted for upgrading users with no migration.
	OverlayLockedModes map[string]string `yaml:"overlay_locked_modes,omitempty" json:"overlay_locked_modes,omitempty"`

	// Roll Tracker overlay defaults, remembered across sessions. The tracker
	// is otherwise stateless across restarts, but these UI preferences (e.g.
	// a guild that always rolls lowest-wins, or runs in timer mode) should
	// stick. Empty/zero means "use the built-in default" (highest / manual /
	// DefaultAutoStopSeconds).
	RollTrackerWinnerRule      string `yaml:"roll_tracker_winner_rule,omitempty" json:"roll_tracker_winner_rule,omitempty"`
	RollTrackerMode            string `yaml:"roll_tracker_mode,omitempty" json:"roll_tracker_mode,omitempty"`
	RollTrackerAutoStopSeconds int    `yaml:"roll_tracker_auto_stop_seconds,omitempty" json:"roll_tracker_auto_stop_seconds,omitempty"`
	// RollTrackerProfile is the active grouping profile, stored as a JSON
	// blob (the structured RollProfile lives in the rolltracker package; the
	// config layer round-trips it opaquely). Empty means the simple default.
	RollTrackerProfile string `yaml:"roll_tracker_profile,omitempty" json:"roll_tracker_profile,omitempty"`

	// SidebarHidden lists side-nav route keys (e.g. "/loot") the user has
	// hidden from the navigation menu. Hiding only removes the tab from the
	// sidebar — the page is still reachable by URL. Empty/omitted = nothing
	// hidden. The fixed controls (back/forward, search, character switcher,
	// settings) are never hideable.
	SidebarHidden []string `yaml:"sidebar_hidden,omitempty" json:"sidebar_hidden,omitempty"`

	// SidebarOrder is a flat list of side-nav route keys in the user's
	// preferred display order. Items are ordered within their section by their
	// position here; keys absent from the list keep their default order after
	// the listed ones. Empty/omitted = default order.
	SidebarOrder []string `yaml:"sidebar_order,omitempty" json:"sidebar_order,omitempty"`
}

// TimerAlertPref is a global default "fading soon" notification for the Custom
// Timer and Respawn overlays — the settings-page counterpart to a trigger's
// per-trigger TimerAlert. The frontend stores and fires it (mirroring
// useTimerAlerts), so the config package only persists the fields and never
// introspects them. Type is "play_sound" or "text_to_speech"; Seconds is the
// remaining-time threshold the alert fires at; volumes are 0–100.
type TimerAlertPref struct {
	Enabled     bool   `yaml:"enabled,omitempty" json:"enabled"`
	Seconds     int    `yaml:"seconds,omitempty" json:"seconds"`
	Type        string `yaml:"type,omitempty" json:"type"`
	SoundPath   string `yaml:"sound_path,omitempty" json:"sound_path"`
	Volume      int    `yaml:"volume,omitempty" json:"volume"`
	TTSTemplate string `yaml:"tts_template,omitempty" json:"tts_template"`
	Voice       string `yaml:"voice,omitempty" json:"voice"`
	TTSVolume   int    `yaml:"tts_volume,omitempty" json:"tts_volume"`
}

// Respawn-alert TTS default. The frontend mirrors these in
// frontend/src/lib/timerAlerts.ts — keep them in sync. The legacy spelling
// "respawned" is mispronounced by the speech engine ("R-E-S-P-awned"), so the
// hyphenated form is the current default; applyDefaults upgrades a saved pref
// still pinned to the exact legacy string (see the migration there).
const (
	legacyRespawnTTSTemplate  = "{npc} has respawned"
	DefaultRespawnTTSTemplate = "{npc} has re-spawned"
)

// OverlayPosition is an on-screen point in the trigger overlay window's
// local pixel space (mirrors trigger.ActionPosition, but lives here so the
// config package doesn't import the trigger package).
type OverlayPosition struct {
	X int `yaml:"x" json:"x"`
	Y int `yaml:"y" json:"y"`
}

// NPCOverlaySections toggles individual NPC overlay sections on/off. All
// fields default to true so behaviour is unchanged for upgrading users.
type NPCOverlaySections struct {
	Identity         bool `yaml:"identity" json:"identity"`
	Combat           bool `yaml:"combat" json:"combat"`
	Resists          bool `yaml:"resists" json:"resists"`
	Attributes       bool `yaml:"attributes" json:"attributes"`
	SpecialAbilities bool `yaml:"special_abilities" json:"special_abilities"`
	Faction          bool `yaml:"faction" json:"faction"`
	// Spells is the master toggle for the caster-summary section (highlights are
	// always shown when it's on). SpellsProcs / SpellsSignature / SpellsClass are
	// the per-group sub-toggles for procs, named signature spells, and the
	// inherited class-list counts respectively.
	Spells          bool `yaml:"spells" json:"spells"`
	SpellsProcs     bool `yaml:"spells_procs" json:"spells_procs"`
	SpellsSignature bool `yaml:"spells_signature" json:"spells_signature"`
	SpellsClass     bool `yaml:"spells_class" json:"spells_class"`
}

// CHChainSettings configures the Complete-Heal-chain overlay matcher, which
// watches raid chat for chain-call lines and creates ch_chain countdown timers.
type CHChainSettings struct {
	// Enabled turns the matcher on. When false, no ch_chain timers are
	// created and the CH Chain overlay stays empty. On by default for fresh
	// installs.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Pattern is the regex matched against each raw raid-chat line. It must
	// capture named groups `caster`, `chainnum`, and `target`. Empty means
	// "use the built-in default" (DefaultCHChainPattern).
	Pattern string `yaml:"pattern" json:"pattern"`

	// SecondaryEnabled turns on a second, independent chain (a ramp/split
	// chain healed alongside the main tank chain). Calls matching
	// SecondaryPattern get their own ch_chain_2 timers, which the overlay
	// and metronome can switch to. When enabled, the settings UI swaps the
	// default primary pattern to its numeric-only variant and fills the
	// secondary with the letters-only variant so AAA/BBB ramp calls split
	// away from 001/002 main-chain calls out of the box.
	SecondaryEnabled bool `yaml:"secondary_enabled" json:"secondary_enabled"`

	// SecondaryPattern is the regex for the secondary chain, with the same
	// named-group contract as Pattern. Empty means "use the built-in
	// letters-only default" (DefaultCHChainSecondaryPattern). Only consulted
	// when SecondaryEnabled is true; it is tried BEFORE Pattern so letter
	// calls still route to the secondary chain even if the primary pattern
	// also happens to match them.
	SecondaryPattern string `yaml:"secondary_pattern" json:"secondary_pattern"`

	// IntervalSecs is the EXPECTED chain cadence — the spacing the raid is
	// aiming for between consecutive casts. The overlay no longer uses it to
	// size the countdown bars (those now run the fixed CH cast time); it's a
	// reference baseline the overlay compares the live measured cadence
	// against to flag a stalled chain. Fractional values (e.g. 4.5) are
	// allowed. 0 means "use DefaultCHChainIntervalSecs".
	IntervalSecs float64 `yaml:"interval_secs" json:"interval_secs"`
}

// CHCastSecs is Complete Heal's cast time in seconds. Each ch_chain countdown
// bar runs this long so it reads as "time until this cleric's heal lands",
// making chain gaps visible. A constant of the spell, not user-configurable.
const CHCastSecs = 10

// DefaultCHChainPattern matches the common guild chain-call formats, e.g.
// "Soandso tells the raid, '--- 001 --- CH Winian with << 100% Mana >>'",
// capturing caster, chainnum, and target. It tolerates a range of channels
// (raid/group/guild tells, party/raid/guild "tell your", custom channel
// tells, OOC, shout, auction) and decorations around the number and target.
//
// Note: this is RE2 (Go's regexp), which has no backreferences. The user
// feedback pattern expressed the letter-marker form as a same-letter
// backreference ((?<letter>[A-Za-z])\k<letter>{2,3}); RE2 can't enforce
// "same letter repeated", so chainnum's letter branch is [A-Za-z]{3,4} (the
// surrounding CH/COMPLETE HEALING context keeps it from over-matching).
// Numeric markers parse to their position directly; letter markers map
// A=1, B=2, … from the first letter in the matcher.
//
// The pattern is assembled from a shared prefix/suffix around the chainnum
// branch so the single-chain default (numbers OR letters) and the split
// numeric-only / letters-only variants used by the secondary-chain feature
// stay in lockstep. Mirrored in frontend/src/lib/chChainPatterns.ts — keep
// them in sync.
const (
	// Channel verbs carry an optional trailing "s" (tells?, says?, shouts?,
	// auctions?) so both conjugations match: third person for others' casts
	// ("Soandso shouts") AND second person for your own ("You shout"). Missing
	// the "?" on shout/OOC was a bug — own shout/OOC chain calls never matched.
	chChainPatternPrefix = `^(?P<caster>(You|[A-Z][a-z]{3,14})) (?:tells? (?:the (?:raid|group|guild)|your (party|raid|guild)|[A-Za-z]+(?:-[A-Za-z]+)+:\d)|says? out of character|shouts?|auctions?),\s+'[^a-zA-Z0-9]*\b(?P<chainnum>`
	// The heal token tolerates the common calling variants: "CH",
	// "COMPLETE HEALING", "DCH" (druid complete heal), and a bare "RAMP"
	// marker. Each is whole-word-anchored (\b…\b) so the surrounding context
	// keeps it from over-matching ordinary chat.
	chChainPatternSuffix = `)[^a-zA-Z0-9]*\b(?:DCH|CH|COMPLETE HEALING|RAMP)\b(?:[^a-zA-Z0-9]*(?:on|to)[^a-zA-Z0-9]*)?[^a-zA-Z0-9]*(?P<target>[A-Z][a-z]{3,14})\b(.*)$`

	// DefaultCHChainPattern is the single-chain catch-all: numeric (001) and
	// letter (AAA) markers both feed the one main chain.
	DefaultCHChainPattern = chChainPatternPrefix + `\d{3,4}|[A-Za-z]{3,4}` + chChainPatternSuffix

	// DefaultCHChainNumericPattern matches only numeric markers (001 002 …).
	// The settings UI swaps the primary pattern to this when the secondary
	// chain is enabled, so letter calls stop feeding the main chain.
	DefaultCHChainNumericPattern = chChainPatternPrefix + `\d{3,4}` + chChainPatternSuffix

	// DefaultCHChainSecondaryPattern matches only letter markers (AAA BBB …),
	// the common ramp/split-chain calling style.
	DefaultCHChainSecondaryPattern = chChainPatternPrefix + `[A-Za-z]{3,4}` + chChainPatternSuffix
)

// legacyCHChainPrefixV2 is the chChainPatternPrefix value shipped before the
// shout/OOC verbs gained their optional trailing "s" (own casts not matched).
// Kept verbatim so the three v2 defaults below can be reconstructed for the
// upgrade map.
const legacyCHChainPrefixV2 = `^(?P<caster>(You|[A-Z][a-z]{3,14})) (?:tells? (?:the (?:raid|group|guild)|your (party|raid|guild)|[A-Za-z]+(?:-[A-Za-z]+)+:\d)|says out of character|shouts|auctions?),\s+'[^a-zA-Z0-9]*\b(?P<chainnum>`

// legacyCHChainSuffixV3 is the chChainPatternSuffix value shipped before the
// heal token expanded from "CH"/"COMPLETE HEALING" to also accept "DCH" and
// "RAMP". Kept verbatim so the prior defaults (which paired this suffix with
// either prefix) can be reconstructed for the upgrade maps.
const legacyCHChainSuffixV3 = `)[^a-zA-Z0-9]*\b(?:CH|COMPLETE HEALING)\b(?:[^a-zA-Z0-9]*(?:on|to)[^a-zA-Z0-9]*)?[^a-zA-Z0-9]*(?P<target>[A-Z][a-z]{3,14})\b(.*)$`

// legacyCHChainPatternUpgrades maps every PREVIOUS shipped value of the
// primary pattern (catch-all or numeric-only when split) to its current
// equivalent. applyDefaults upgrades a saved pattern that exactly matches a
// key — configs snapshot the default at save time, so without this an
// upgrading user would stay pinned to an old default forever (and the settings
// UI's default-detection, e.g. the secondary-chain pattern swap, wouldn't
// recognize their pattern as a default). Hand-customized patterns never match
// and are left alone. Add the outgoing values whenever a default changes.
var legacyCHChainPatternUpgrades = map[string]string{
	// v1 (89f3cd1): numeric-only, raid tells only, strict dash decorations.
	`^(?P<caster>\w+) tells the raid, '-+\s*0*(?P<chainnum>\d+)\s*-+\s*CH\s+(?P<target>\w+)`: DefaultCHChainPattern,
	// v2: own "shout"/"say OOC" not matched (verbs lacked the optional "s").
	legacyCHChainPrefixV2 + `\d{3,4}|[A-Za-z]{3,4}` + legacyCHChainSuffixV3: DefaultCHChainPattern,
	legacyCHChainPrefixV2 + `\d{3,4}` + legacyCHChainSuffixV3:               DefaultCHChainNumericPattern,
	// v3: current prefix, heal token CH/COMPLETE HEALING only (pre DCH/RAMP).
	chChainPatternPrefix + `\d{3,4}|[A-Za-z]{3,4}` + legacyCHChainSuffixV3: DefaultCHChainPattern,
	chChainPatternPrefix + `\d{3,4}` + legacyCHChainSuffixV3:               DefaultCHChainNumericPattern,
}

// legacyCHChainSecondaryUpgrades is the same upgrade map for the secondary
// (ramp/split) letters-only pattern.
var legacyCHChainSecondaryUpgrades = map[string]string{
	legacyCHChainPrefixV2 + `[A-Za-z]{3,4}` + legacyCHChainSuffixV3: DefaultCHChainSecondaryPattern,
	chChainPatternPrefix + `[A-Za-z]{3,4}` + legacyCHChainSuffixV3:  DefaultCHChainSecondaryPattern,
}

// DefaultCHChainIntervalSecs is the default per-cast countdown cadence.
const DefaultCHChainIntervalSecs = 6

// DefaultCHChainSettings returns the on-by-default settings for fresh installs.
func DefaultCHChainSettings() CHChainSettings {
	return CHChainSettings{
		Enabled:          true,
		Pattern:          DefaultCHChainPattern,
		SecondaryPattern: DefaultCHChainSecondaryPattern,
		IntervalSecs:     DefaultCHChainIntervalSecs,
	}
}

// DefaultNPCOverlaySections returns the all-visible default — matches the
// behaviour before this preference existed (Faction was added later and is
// also on by default; see the migration in applyDefaults for existing configs).
func DefaultNPCOverlaySections() NPCOverlaySections {
	return NPCOverlaySections{
		Identity:         true,
		Combat:           true,
		Resists:          true,
		Attributes:       true,
		SpecialAbilities: true,
		Faction:          true,
		Spells:           true,
		SpellsProcs:      true,
		SpellsSignature:  true,
		SpellsClass:      true,
	}
}

// defaults returns a Config populated with sensible default values.
func defaults() Config {
	return Config{
		// 17654 is in the IANA registered range (1024-49151) but not used by
		// any well-known service — picked specifically to avoid collisions
		// with the more commonly squatted ports (8080, 3000, 5000, etc.).
		// If this port is still occupied on a particular user's machine the
		// server falls back to an OS-assigned port at startup.
		ServerAddr:     ":17654",
		CharacterClass: -1,
		Preferences: Preferences{
			OverlayOpacity:              0.25,
			OverlayFadeDelaySecs:        2.5,
			MinimizeToTray:              true,
			ParseCombatLog:              true,
			OverlayDPSEnabled:           true,
			OverlayHPSEnabled:           false,
			OverlayEntityLinksEnabled:   true,
			RaidThreatEnabled:           false,
			MasterVolume:                100,
			TTSRate:                     1.0,
			ZoomFactor:                  1.0,
			NPCOverlayDashboardSections: DefaultNPCOverlaySections(),
			NPCOverlayPopoutSections:    DefaultNPCOverlaySections(),
		},
		Backup: BackupSettings{
			AutoBackup: false,
			Schedule:   "off",
			MaxBackups: 10,
		},
		SpellTimer: SpellTimerSettings{
			TrackingScope: TrackingScopeCastByMe,
			// CastByMeMigrationDone is intentionally left false here so that
			// applyDefaults can detect a config file that predates the
			// migration. For brand-new files we set it to true in the
			// not-exist branch of LoadFrom.
		},
		Combat: CombatSettings{
			RetentionDays:       30,
			FightTimeoutSeconds: DefaultFightTimeoutSeconds,
		},
		CHChain:           DefaultCHChainSettings(),
		ChatRetentionDays: 30,
		DPSClassColors:    DefaultDPSClassColors(),
	}
}

// Manager holds a loaded Config and serializes concurrent access.
type Manager struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

// Load reads (or creates) the config file at the default location
// (~/.pq-companion/config.yaml) and returns a Manager.
func Load() (*Manager, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads (or creates) the config file at the given path.
func LoadFrom(path string) (*Manager, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		// File does not exist — write defaults so the user has a template.
		// New files are born already-migrated; the markers only matter for
		// pre-existing configs (the "anyone" spell-timer default, and the
		// raid-threat-on default that predates the Developer-tab gate).
		cfg.SpellTimer.CastByMeMigrationDone = true
		cfg.Preferences.RaidThreatMigratedOff = true
		m := &Manager{cfg: cfg, path: path}
		if saveErr := m.save(); saveErr != nil {
			// Non-fatal: we can operate without a file on disk.
			_ = saveErr
		}
		return m, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	migrated := applyDefaults(&cfg)

	m := &Manager{cfg: cfg, path: path}
	if migrated {
		// Persist the migration so it doesn't run again next launch.
		_ = m.save()
	}
	return m, nil
}

// applyDefaults fills in fields the user's on-disk file may be missing.
// Older config files predate newly-added settings; setting safe defaults
// here keeps the engine and UI from having to special-case empty values.
//
// Returns true when a value was changed and the file should be re-saved.
func applyDefaults(cfg *Config) bool {
	changed := false
	// One-time migration: the prior default was "anyone", which produced
	// noisy overlays when other players were buffing each other. New default
	// is "cast_by_me". Existing configs that haven't been migrated yet are
	// rewritten exactly once; subsequent explicit choices of "anyone" are
	// preserved.
	if !cfg.SpellTimer.CastByMeMigrationDone {
		if cfg.SpellTimer.TrackingScope == "" || cfg.SpellTimer.TrackingScope == TrackingScopeAnyone {
			cfg.SpellTimer.TrackingScope = TrackingScopeCastByMe
		}
		cfg.SpellTimer.CastByMeMigrationDone = true
		changed = true
	}

	// One-time migration: raid-estimate threat mode used to default on. It is
	// now an experimental, Developer-tab-gated feature that is off by default,
	// because its per-player estimates mislead in many raid setups. Force it
	// off once for existing configs (which persisted the old on-by-default via
	// the previously-omitempty-less field); a later re-enable in the Developer
	// tab sticks because the marker keeps this from running again.
	if !cfg.Preferences.RaidThreatMigratedOff {
		cfg.Preferences.RaidThreatEnabled = false
		cfg.Preferences.RaidThreatMigratedOff = true
		changed = true
	} else if cfg.SpellTimer.TrackingScope == "" {
		cfg.SpellTimer.TrackingScope = TrackingScopeCastByMe
		changed = true
	}
	// Combat history retention default — applies to configs that predate
	// the combat history feature. The YAML decoder leaves missing numeric
	// fields at zero, so a missing value and an explicit "0" are
	// indistinguishable. Treat both as "use the default"; users who really
	// want pruning disabled can write a negative value (e.g. -1), which
	// pruneCombatHistory honours as "skip".
	if cfg.Combat.RetentionDays == 0 {
		cfg.Combat.RetentionDays = 30
		changed = true
	}
	// Fight-inactivity timeout: same "0 means use the default" convention as
	// retention above (a missing field and an explicit 0 are indistinguishable
	// to the YAML decoder). Backfills configs that predate the setting so the
	// meter behaves consistently whether or not the field is present.
	if cfg.Combat.FightTimeoutSeconds == 0 {
		cfg.Combat.FightTimeoutSeconds = DefaultFightTimeoutSeconds
		changed = true
	}
	// Chat history retention: same convention as Combat — a missing/zero value
	// means "use the default" (30 days); a negative value disables the purge
	// (keep forever).
	if cfg.ChatRetentionDays == 0 {
		cfg.ChatRetentionDays = 30
		changed = true
	}
	// TTS speaking rate: backfill configs that predate the setting so the
	// Settings slider reads 1.0 (normal speed) rather than 0. A missing field
	// and an explicit 0 are indistinguishable to the YAML decoder, and 0 is
	// not a meaningful rate, so both become the default.
	if cfg.Preferences.TTSRate == 0 {
		cfg.Preferences.TTSRate = 1.0
		changed = true
	}
	// DPS class colour palette: fill in any blank entries from the defaults so
	// upgrading users get the palette without losing per-class overrides
	// they may have set previously.
	if fillDPSColorDefaults(&cfg.DPSClassColors) {
		changed = true
	}
	// CH-chain matcher: backfill the regex and cadence so the feature has
	// sensible values whenever the user enables it. Enabled itself is left as
	// loaded (false for configs predating the feature) — it's opt-in for
	// upgrading users and on by default only for fresh installs.
	if cfg.CHChain.Pattern == "" {
		cfg.CHChain.Pattern = DefaultCHChainPattern
		changed = true
	}
	// Upgrade patterns pinned to an outdated default: configs save the
	// default verbatim, so a user who never customized theirs would
	// otherwise miss every improvement to the shipped pattern (channel
	// coverage, letter markers, …). Exact match only — anything the user
	// edited won't be in the map and stays untouched.
	if up, ok := legacyCHChainPatternUpgrades[cfg.CHChain.Pattern]; ok {
		cfg.CHChain.Pattern = up
		changed = true
	}
	// Backfill the secondary (ramp/split chain) pattern the same way so the
	// letters-only default is visible in settings before the user enables it.
	// SecondaryEnabled is left as loaded — the second chain is opt-in.
	if cfg.CHChain.SecondaryPattern == "" {
		cfg.CHChain.SecondaryPattern = DefaultCHChainSecondaryPattern
		changed = true
	} else if up, ok := legacyCHChainSecondaryUpgrades[cfg.CHChain.SecondaryPattern]; ok {
		cfg.CHChain.SecondaryPattern = up
		changed = true
	}
	if cfg.CHChain.IntervalSecs <= 0 {
		cfg.CHChain.IntervalSecs = DefaultCHChainIntervalSecs
		changed = true
	}
	// NPC overlay sections: configs that predate this preference deserialize
	// with every bool false, which would hide every section. Detect the
	// all-zero case and substitute the all-visible default so the overlay
	// keeps working after upgrade. Once the user makes any explicit choice
	// (toggling at least one section on), the struct will have a true value
	// and this branch becomes a no-op.
	if isZeroNPCOverlaySections(cfg.Preferences.NPCOverlayDashboardSections) {
		cfg.Preferences.NPCOverlayDashboardSections = DefaultNPCOverlaySections()
		changed = true
	}
	if isZeroNPCOverlaySections(cfg.Preferences.NPCOverlayPopoutSections) {
		cfg.Preferences.NPCOverlayPopoutSections = DefaultNPCOverlaySections()
		changed = true
	}
	// One-time migration for the later-added Faction section: configs written
	// before the field existed deserialize Faction=false on otherwise-populated
	// section structs (so the all-zero reset above doesn't touch them). Turn it
	// on once to match the fresh-install default, then never override the user's
	// explicit choice again.
	if !cfg.Preferences.NPCFactionSectionMigrationDone {
		cfg.Preferences.NPCOverlayDashboardSections.Faction = true
		cfg.Preferences.NPCOverlayPopoutSections.Faction = true
		cfg.Preferences.NPCFactionSectionMigrationDone = true
		changed = true
	}
	// One-time migration for the later-added Spells & Abilities section and its
	// three sub-toggles. Same pattern as Faction above: turn them on once to
	// match the fresh-install default, then never override an explicit choice.
	if !cfg.Preferences.NPCSpellsSectionMigrationDone {
		for _, s := range []*NPCOverlaySections{
			&cfg.Preferences.NPCOverlayDashboardSections,
			&cfg.Preferences.NPCOverlayPopoutSections,
		} {
			s.Spells = true
			s.SpellsProcs = true
			s.SpellsSignature = true
			s.SpellsClass = true
		}
		cfg.Preferences.NPCSpellsSectionMigrationDone = true
		changed = true
	}
	// Respawn-alert TTS spelling: a config saved with the exact legacy default
	// "{npc} has respawned" predates the pronunciation fix. Swap it for the
	// hyphenated form so existing users who enabled the alert stop hearing
	// "R-E-S-P-awned". Exact match only — a customized template stays untouched.
	if cfg.Preferences.RespawnAlert != nil &&
		cfg.Preferences.RespawnAlert.TTSTemplate == legacyRespawnTTSTemplate {
		cfg.Preferences.RespawnAlert.TTSTemplate = DefaultRespawnTTSTemplate
		changed = true
	}
	return changed
}

func isZeroNPCOverlaySections(s NPCOverlaySections) bool {
	return !s.Identity && !s.Combat && !s.Resists && !s.Attributes &&
		!s.SpecialAbilities && !s.Faction
}

// fillDPSColorDefaults populates any empty hex fields with the corresponding
// default. Returns true when at least one field was changed.
func fillDPSColorDefaults(c *DPSClassColors) bool {
	d := DefaultDPSClassColors()
	changed := false
	pairs := []struct {
		dst *string
		def string
	}{
		{&c.Warrior, d.Warrior},
		{&c.Cleric, d.Cleric},
		{&c.Paladin, d.Paladin},
		{&c.Ranger, d.Ranger},
		{&c.ShadowKnight, d.ShadowKnight},
		{&c.Druid, d.Druid},
		{&c.Monk, d.Monk},
		{&c.Bard, d.Bard},
		{&c.Rogue, d.Rogue},
		{&c.Shaman, d.Shaman},
		{&c.Necromancer, d.Necromancer},
		{&c.Wizard, d.Wizard},
		{&c.Magician, d.Magician},
		{&c.Enchanter, d.Enchanter},
		{&c.Beastlord, d.Beastlord},
		{&c.Unknown, d.Unknown},
	}
	for _, p := range pairs {
		if *p.dst == "" {
			*p.dst = p.def
			changed = true
		}
	}
	return changed
}

// Get returns a copy of the current config.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update replaces the current config with c and persists it to disk.
func (m *Manager) Update(c Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = c
	return m.save()
}

// Modify applies mutate to the live config under the manager lock, then
// persists it. Unlike Get()+Update() (a read-copy, then a wholesale replace),
// the read-modify-write is atomic — so concurrent backend writers (roll-tracker
// persist, character auto-detect, …) can no longer clobber each other's fields
// by writing back a stale copy.
func (m *Manager) Modify(mutate func(*Config)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mutate(&m.cfg)
	return m.save()
}

// save writes the current config to disk (must be called with m.mu held).
//
// The write is atomic: marshal to a temp file in the same directory, then
// os.Rename over the target. A crash / power loss / disk-full mid-write can
// then only leave a stray temp file — never a truncated config.yaml that
// fails to parse on next launch and blocks startup.
func (m *Manager) save() error {
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(&m.cfg)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if we bail before the rename succeeds.
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, m.path)
}

// Path returns the path to the config file on disk.
func (m *Manager) Path() string { return m.path }

// defaultPath returns ~/.pq-companion/config.yaml.
func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pq-companion", "config.yaml"), nil
}

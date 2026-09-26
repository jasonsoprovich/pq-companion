export type ActionType =
  | 'overlay_text'
  | 'play_sound'
  | 'text_to_speech'
  | 'clipboard'
  | 'discord_webhook'

export type TimerType = 'none' | 'buff' | 'detrimental' | 'custom'

export type TimerAlertType = 'play_sound' | 'text_to_speech' | 'overlay_text'

/**
 * One "fading soon" notification on a timer-bound trigger. Fires when the
 * trigger's spell timer crosses {seconds} remaining. A trigger may carry any
 * number of these (e.g. 300s + 60s for a long buff, 10s for a mez).
 */
export interface TimerAlertThreshold {
  id: string
  seconds: number
  type: TimerAlertType
  sound_path: string
  volume: number       // 0–100
  tts_template: string // supports {spell} placeholder
  voice: string
  tts_volume: number   // 0–100

  // Overlay-text fields, used only when type === 'overlay_text'. Mirror
  // Action's overlay_text fields exactly — the same NotificationActionEditor
  // renders both. text supports the same {spell} placeholder as tts_template.
  text?: string
  duration_secs?: number
  color?: string
  position?: ActionPosition | null
  font_size?: number
  glow_color?: string
  font_family?: string
  align?: string
}

/**
 * On-screen placement of an overlay_text action in the trigger overlay
 * window's local space (top-left origin, CSS pixels). Absence on an
 * Action means the renderer falls back to the default stacking layout.
 */
export interface ActionPosition {
  x: number
  y: number
}

export interface Action {
  type: ActionType
  text: string
  duration_secs: number
  color: string
  sound_path: string
  volume: number   // 0.0–1.0; 0 means use default (1.0)
  voice: string    // TTS voice name; empty = system default
  /** Pins overlay_text alerts to a fixed location; omit/null = stack. */
  position?: ActionPosition | null
  /** Overlay font size in CSS pixels; 0/omit = global default. */
  font_size?: number
  /** Overlay text-glow hex color; empty/omit = global default, falling
   *  back to a glow derived from the text color. */
  glow_color?: string
  /** Overlay font family (a font installed on the user's machine);
   *  empty/omit = global default, falling back to the system-ui stack. */
  font_family?: string
  /** Anchor/text alignment for a pinned Position: 'left' | 'center' | 'right'.
   *  empty/omit = global default, falling back to 'left'. */
  align?: string
  /** discord_webhook: id of a Preferences.discord_webhooks entry. The URL
   *  itself is never stored here — see DiscordWebhook's doc comment. */
  webhook_id?: string
}

/**
 * Match source for a trigger:
 *   'log'  — Pattern regex against log lines (default).
 *   'pipe' — typed match on PipeCondition against ZealPipe events.
 * Existing triggers persisted without this field deserialise as 'log'.
 */
export type TriggerSource = 'log' | 'pipe'

/**
 * Kind discriminator for pipe-source trigger conditions. Each kind reads
 * a different subset of PipeCondition fields — see the field comments.
 */
export type PipeConditionKind =
  | 'target_hp_below'
  | 'target_name'
  | 'buff_landed'
  | 'buff_faded'
  | 'pipe_command'

/**
 * Typed match definition for Source='pipe' triggers. Only the fields
 * relevant to the chosen kind are populated; the backend ignores the rest.
 */
export interface PipeCondition {
  kind: PipeConditionKind
  /** target_hp_below: fires when target HP crosses below this percentage (0-100). */
  hp_threshold?: number
  /** target_name: fires when the player's target becomes this name (exact match). */
  target_name?: string
  /** buff_landed / buff_faded: spell name to watch in the player's buff slots. */
  spell_name?: string
  /** pipe_command: matches `/pipe <text>` typed in-game (exact match). */
  text?: string
}

/**
 * One additional match pattern on a log-source trigger, individually
 * toggleable without being deleted. The trigger fires when the primary
 * pattern OR any enabled extra pattern matches; the matching pattern's
 * capture groups feed the action text.
 */
export interface ExtraPattern {
  pattern: string
  enabled: boolean
  /**
   * Per-pattern timer overrides for merged spell-line triggers (one "Mez"
   * trigger covering several spells with different durations). When this
   * pattern is the one that matched, a non-zero duration replaces the
   * trigger's timer_duration_secs and a non-zero spell_id replaces its
   * spell link. Zero/omitted = inherit from the trigger.
   */
  timer_duration_secs?: number
  spell_id?: number
}

export interface Trigger {
  id: string
  name: string
  enabled: boolean
  pattern: string
  actions: Action[]
  /** Display cache derived from category_id — never authoritative. */
  pack_name: string
  /** Authoritative category link (TriggerCategory.id). Empty = Uncategorized. */
  category_id?: string
  created_at: string
  timer_type: TimerType
  timer_duration_secs: number
  /**
   * Capture group ("1", "2", or a named group) whose matched text supplies
   * the timer duration dynamically — e.g. capturing "6m40s" from the log
   * line. Empty = use the fixed timer_duration_secs.
   */
  timer_duration_capture?: string
  /**
   * Capture group ("1", "2", or a named group) whose matched text becomes
   * the spell-timer key instead of the trigger name. Lets a merged trigger
   * (one pattern per spell, each capturing the spell name) run an
   * independent countdown per captured value. The worn-off pattern must
   * capture the same value for early clear. Empty = key by trigger name.
   */
  timer_key_capture?: string
  /**
   * Capture group ("1", "2", or a named group) whose matched text becomes the
   * timer's target name — the grey "on <target>" suffix the buff/detrimental
   * overlays show for spells cast on others. Use it on a "lands on other"
   * pattern that includes the target, e.g. capture the name in
   * `(?P<target>[A-Z][a-zA-Z']{2,14}) experiences visions of grandeur\.`.
   * Empty (or a group that didn't match, e.g. a self-cast branch) = no suffix.
   */
  timer_target_capture?: string
  /**
   * When timer_type is 'custom', gives every firing its own independent
   * timer row instead of restarting/overwriting the existing same-name row
   * — e.g. one respawn timer per mob killed instead of one shared bar that
   * resets. False (default) = today's behavior. Ignored for buff/detrimental.
   */
  timer_stack?: boolean
  worn_off_pattern: string
  spell_id: number
  /**
   * Anti-spam lockout: after firing, suppress this trigger from firing again
   * for this many seconds. 0/absent = fire on every match (default). Distinct
   * from cooldown_secs — this is a silent gate, not a visible recast timer.
   */
  refire_cooldown_secs?: number
  /**
   * Cooldown timer (seconds) spawned alongside the duration timer to track
   * reuse cooldown. Counts down on the buff overlay with a " CD" suffix.
   * 0 = no cooldown timer.
   */
  cooldown_secs?: number
  /** Match source — defaults to 'log' when absent on the wire. */
  source?: TriggerSource
  /** Typed match definition; only present (and required) when source='pipe'. */
  pipe_condition?: PipeCondition
  /**
   * Per-trigger override for the global buff / detrim display threshold
   * (in seconds). > 0 means the timer this trigger creates is hidden
   * until its remaining time falls at or below this value. 0 (default)
   * defers to the user's global setting.
   */
  display_threshold_secs: number
  /**
   * Optional CSS color (e.g. "#22c55e") for this trigger's timer bar, for
   * color-coding the overlay. Empty/absent = use the overlay's automatic
   * category/remaining-based color.
   */
  bar_color?: string
  /**
   * Character names this trigger fires for. Empty = fires for any active
   * character (legacy / safety fallback).
   */
  characters: string[]
  /**
   * Per-trigger fading-soon notifications. Each entry fires an audio cue
   * when the timer this trigger creates crosses the configured remaining
   * seconds. Empty = no fading alert (timer counts down silently).
   */
  timer_alerts: TimerAlertThreshold[]
  /**
   * Regexes that suppress this trigger when any of them also match the
   * same log line. Lets a broad primary pattern (e.g. `\w+ tells you,`)
   * filter out pet/merchant lines without RE2 lookbehind. Each entry is
   * tested independently — empty list = no exclusions.
   */
  exclude_patterns: string[]
  /**
   * Additional regexes matched alongside the primary pattern — the trigger
   * fires when ANY enabled pattern matches ("any" semantics). Each entry
   * toggles independently in the editor. Empty = single-pattern trigger.
   */
  extra_patterns: ExtraPattern[]
  /**
   * Identifies the conceptual spell/discipline/skill this trigger
   * represents, independent of which class pack ships it. When two
   * packs both define a trigger with the same dedup_key, only one
   * is installed; the second is skipped. Empty/undefined = no dedup.
   */
  dedup_key?: string
  /**
   * Manual position within the trigger's category, used by the Triggers
   * page "Manual" sort mode. Lower sorts first; set on create/move to
   * append at the end of the category.
   */
  sort_order: number
  /**
   * Keeps this trigger's timer(s) floating at the top of whichever overlay
   * they render in (buff/detrimental/custom), ahead of the normal
   * ascending-remaining-time sort. False (default) = normal sort order.
   */
  pinned?: boolean
  /**
   * Which Custom Timers window this trigger's timer appears in, when
   * timer_type is 'custom'. References TimerGroup.id. Empty/absent = the
   * original/default Custom Timers window. Ignored for buff/detrimental.
   */
  custom_group_id?: string
  /**
   * Which pack this trigger was installed from, independent of pack_name
   * (its display category). Lets a pack trigger moved into a custom
   * category still be tagged with its origin and removed on pack
   * deactivation. Empty for user-authored triggers.
   */
  source_pack?: string
}

export interface TriggerFired {
  trigger_id: string
  trigger_name: string
  matched_line: string
  actions: Action[]
  fired_at: string
  /** True when this fire came from the Trigger Tester, not a real log line
   *  — see TriggerTesterTab. The History tab filters these out. */
  test?: boolean
}

// ── Trigger Tester ───────────────────────────────────────────────────────────
// Paste log lines, hit Run, and see/hear what would fire — without a raid to
// test against. See backend/internal/trigger/tester.go.

export type TestLineStatus = 'matched' | 'excluded' | 'cooldown' | 'wrong_character'

export interface TestTimerInfo {
  key: string
  category?: string
  duration_secs?: number
  target?: string
}

export interface TestWebhook {
  webhook_id: string
  text: string
  /** False = the webhook id no longer resolves to one in Settings. */
  resolved: boolean
}

/** One trigger's outcome against one pasted line. Triggers whose pattern
 *  never matched the line are simply absent — not represented as a "no
 *  match" entry. */
export interface TestMatch {
  trigger_id: string
  trigger_name: string
  status: TestLineStatus
  /** "primary" or "extra N" — which pattern matched. */
  pattern_label?: string
  /** Set only when status === 'excluded'. */
  exclude_pattern?: string
  /** Numbered ("0", "1", …) and named capture groups. */
  captures?: Record<string, string>
  /** Rendered action text (captures/built-ins already substituted in). */
  actions?: Action[]
  timer?: TestTimerInfo
  webhooks?: TestWebhook[]
  /** True when this entry is the worn-off pattern matching (stopping a
   *  timer), not the primary/extra pattern. */
  worn_off?: boolean
  /** True when fire effects were on and this match actually ran — the real
   *  overlay/audio/timer fired, not just the report row. */
  fired: boolean
}

export interface TestLineResult {
  line: string
  /** The line's own parsed EQ log timestamp, if it had one. */
  timestamp?: string
  matches: TestMatch[]
}

export interface TestRequest {
  lines: string
  /** Defaults to the live active character server-side when omitted. */
  character?: string
  /** Also starts real timers and broadcasts trigger:fired (test:true) so
   *  the overlay/audio can preview a match. Never posts webhooks or writes
   *  history regardless of this flag. */
  fire_effects?: boolean
  /** Tests only this (possibly unsaved) trigger instead of every enabled
   *  trigger — used by the trigger editor's inline sample-line test. */
  trigger?: Trigger
  /** Paces playback by the lines' own timestamp gaps and streams results
   *  over WS instead of returning the whole report at once. */
  realtime?: boolean
}

export interface TestReport {
  lines: TestLineResult[]
  /** Problems that stopped the whole run — e.g. an invalid draft pattern. */
  errors?: string[]
  matched: number
  fired: number
  excluded: number
  cooldowns: number
}

export type TestPlaybackState = 'idle' | 'playing'

export interface TestStatus {
  state: TestPlaybackState
  errors?: string[]
}

export interface TriggerPack {
  pack_name: string
  description: string
  // EQ class index (0=Warrior … 14=Beastlord) for class-specific packs;
  // omitted/null/undefined for class-agnostic packs (e.g. General Triggers)
  // and user-authored packs that don't specify a class.
  class?: number | null
  triggers: Trigger[]
}

// ── Built-in pack updates ────────────────────────────────────────────────────
//
// A new app release can change the built-in pack definitions compiled into
// it. The backend snapshots each trigger's definition at install time and
// diffs it against the current build, so user customizations never read as
// pending updates. See backend/internal/trigger/packupdate.go.

/** Per-pack pending-update counts for the Packs tab badge/banner. */
export interface PackUpdateSummary {
  pack_name: string
  changed: number
  added: number
  removed: number
  deleted_locally: number
}

/** One field-level difference between installed baseline and current build. */
export interface PackFieldDiff {
  field: string
  label: string
  /** Value when the pack was installed/last updated. */
  old: string
  /** Value in the current build. */
  new: string
  /** The user's current value. */
  current: string
  /**
   * True when the user changed this field after installing — a "keep my
   * customizations" update leaves their value in place.
   */
  user_customized: boolean
}

export interface PackChangedTrigger {
  pack_key: string
  name: string
  installed_name: string
  fields: PackFieldDiff[]
}

export interface PackAddedTrigger {
  pack_key: string
  name: string
  pattern: string
}

export interface PackRemovedTrigger {
  pack_key: string
  name: string
}

export interface PackDeletedLocalTrigger {
  pack_key: string
  name: string
  pattern: string
}

export interface PackDiff {
  pack_name: string
  changed: PackChangedTrigger[] | null
  added: PackAddedTrigger[] | null
  removed: PackRemovedTrigger[] | null
  deleted_locally: PackDeletedLocalTrigger[] | null
  up_to_date: number
}

/** How to apply a pack update: keep user customizations, or full reset. */
export type PackUpdateMode = 'preserve' | 'reset'

// ── Spell Emote Customizer integration ──────────────────────────────────────
// Flags triggers linked to an edited spell (via Trigger.spell_id) whose
// pattern may be stale, with a suggested replacement — suggest/apply/revert
// only, never an automatic bulk rewrite.

export interface EmoteChange {
  field: string
  old: string
  new: string
}

export type PatternLocation = 'pattern' | 'worn_off_pattern' | 'extra_pattern'

export interface TriggerPatternMatch {
  location: PatternLocation
  extra_index: number
  field: string
  current: string
  suggested: string
}

export interface TriggerEmoteSuggestion {
  trigger_id: string
  name: string
  pack_name: string
  matches: TriggerPatternMatch[]
}

export interface PackUpdateResult {
  updated: number
  added: number
  removed: number
}

// ── Action templates + bulk edits ────────────────────────────────────────────

/**
 * A named, reusable Actions list saved from the trigger editor. At most one
 * is the default; its actions prefill newly created triggers.
 */
export interface ActionTemplate {
  id: string
  name: string
  actions: Action[]
  is_default: boolean
  created_at: string
}

/** Outcome of a bulk action edit. Skipped = nothing to change (e.g. no TTS). */
export interface BulkResult {
  updated: number
  skipped: number
}

/** Source app a trigger import file came from, as detected by the backend. */
export type ImportFormat = 'pqc' | 'gina' | 'eqnag' | 'eqlogparser'

/** Human-facing label for an ImportFormat. */
export const IMPORT_FORMAT_LABELS: Record<ImportFormat, string> = {
  pqc: 'PQ Companion',
  gina: 'GINA',
  eqnag: 'EQNag',
  eqlogparser: 'EQLogParser',
}

/**
 * One trigger produced by parsing an import file, with the lossy-mapping
 * warnings the wizard surfaces and the source group path it lived under.
 */
export interface ImportedTrigger {
  trigger: Trigger
  original_group?: string
  warnings?: string[]
  // false = the mapped pattern doesn't compile under Go's RE2; the trigger is
  // imported disabled and flagged for manual editing.
  regex_ok: boolean
}

/** Result of detecting + parsing an import file, reviewed before commit. */
export interface ImportPreview {
  format: ImportFormat
  source_name: string
  triggers: ImportedTrigger[]
}

/**
 * A trigger grouping (category), id-keyed (Trigger.category_id references
 * it). parent_id is empty for a top-level category; a category with a
 * non-empty parent_id is always a leaf — nesting is capped at one level (see
 * MAX_CATEGORY_DEPTH). Custom categories are user-created and editable;
 * built-in (class) and imported packs surface here too but are flagged
 * is_builtin and stay read-only (managed from the Packs tab). The
 * Uncategorized bucket (empty category_id) is not represented here — the
 * frontend renders it separately. Returned as a flat list; buildCategoryTree
 * assembles it into a tree via parent_id.
 */
export interface TriggerCategory {
  id: string
  name: string
  parent_id: string
  count: number       // triggers in this category, plus — for a top-level category — its children's
  is_builtin: boolean // true = managed via the Packs tab, not editable here
  custom: boolean     // true = user-created (always visible, editable)
  explicit: boolean   // true = has a persisted row (visible even when empty)
  sort_order: number  // display order among siblings; lower sorts first
}

/** How many levels deep a category may nest: 0 = top level, 1 = child. Mirrors trigger.maxCategoryDepth on the backend. */
export const MAX_CATEGORY_DEPTH = 1

/** One category positioned in a reorder/reparent request — see reorderCategories. */
export interface CategoryPlacement {
  id: string
  parent_id: string
  sort_order: number
}

/**
 * A user-created Custom Timers window, letting raid leaders split
 * signature-spell/boss timers into their own overlay separate from general
 * trigger timers. Referenced by Trigger.custom_group_id via id (not name),
 * so renaming never needs to cascade. The implicit default window
 * (custom_group_id === '' / absent) has no TimerGroup row — the frontend
 * renders it separately as the original "Custom Timers" window.
 */
export interface TimerGroup {
  id: string
  name: string
  count: number      // triggers currently assigned to this group
  sort_order: number // display order; lower sorts first
  created_at: string
}

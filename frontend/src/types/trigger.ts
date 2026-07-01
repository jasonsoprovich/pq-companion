export type ActionType =
  | 'overlay_text'
  | 'play_sound'
  | 'text_to_speech'
  | 'clipboard'

export type TimerType = 'none' | 'buff' | 'detrimental' | 'custom'

export type TimerAlertType = 'play_sound' | 'text_to_speech'

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
  pack_name: string
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
 * A trigger grouping (category), keyed off pack_name. Custom categories are
 * user-created and editable; built-in (class) and imported packs surface here
 * too but are flagged is_builtin and stay read-only (managed from the Packs
 * tab). The Uncategorized bucket (empty pack_name) is not represented here.
 */
export interface TriggerCategory {
  name: string
  count: number       // triggers currently in this category
  is_builtin: boolean // true = managed via the Packs tab, not editable here
  custom: boolean     // true = user-created (always visible, editable)
  explicit: boolean   // true = has a persisted row (visible even when empty)
  sort_order: number  // display order; lower sorts first
}

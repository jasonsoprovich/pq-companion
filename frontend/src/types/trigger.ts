export type ActionType = 'overlay_text' | 'play_sound' | 'text_to_speech'

export type TimerType = 'none' | 'buff' | 'detrimental'

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
  /** Overlay font size in CSS pixels; 0/omit = renderer default. */
  font_size?: number
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
  worn_off_pattern: string
  spell_id: number
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
  // omitted/null/undefined for class-agnostic packs (e.g. Group Awareness)
  // and user-authored packs that don't specify a class.
  class?: number | null
  triggers: Trigger[]
}

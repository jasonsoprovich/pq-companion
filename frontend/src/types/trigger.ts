export type ActionType = 'overlay_text' | 'play_sound' | 'text_to_speech'

export type TimerType = 'none' | 'buff' | 'detrimental'

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
  triggers: Trigger[]
}

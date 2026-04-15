// Types for the Timer Audio Alerts system (Task 9.2).
//
// Each TimerAlertThreshold defines a moment (remaining seconds) at which an
// audio alert should fire for any active spell timer that crosses it.

export type TimerAlertType = 'play_sound' | 'text_to_speech'

export interface TimerAlertThreshold {
  id: string
  // Fire when remaining_seconds drops to or below this value.
  seconds: number
  type: TimerAlertType
  // play_sound fields
  sound_path: string
  volume: number // 0–100, mapped to 0.0–1.0 for playSound()
  // text_to_speech fields
  // Supports {spell} placeholder — replaced with the spell name at runtime.
  tts_template: string
  voice: string
  tts_volume: number // 0–100
}

export interface TimerAlertConfig {
  enabled: boolean
  thresholds: TimerAlertThreshold[]
}

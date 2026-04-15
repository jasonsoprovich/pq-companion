// Types for the Event Notification Audio Alert system (Task 9.3).
//
// Each EventAlertRule maps a specific log event type to an audio action
// (TTS or sound file). Only a curated subset of events are supported — the
// ones that are infrequent enough and important enough to warrant an alert.

export type EventAlertType = 'play_sound' | 'text_to_speech'

// Supported log event types for audio alerts. Combat hit/miss events are
// excluded because they are far too frequent.
export type AlertableEventType =
  | 'log:death'
  | 'log:zone'
  | 'log:spell_resist'
  | 'log:spell_interrupt'

export interface EventAlertRule {
  id: string
  event_type: AlertableEventType
  enabled: boolean
  type: EventAlertType
  // play_sound fields
  sound_path: string
  volume: number // 0–100, mapped to 0.0–1.0 at playback time
  // text_to_speech fields
  // Supported placeholders vary by event type:
  //   log:death         — {slain_by}
  //   log:zone          — {zone}
  //   log:spell_resist  — {spell}
  //   log:spell_interrupt — {spell}
  tts_template: string
  voice: string      // empty string = system default
  tts_volume: number // 0–100
}

export interface EventAlertConfig {
  enabled: boolean
  rules: EventAlertRule[]
}

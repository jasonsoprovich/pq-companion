// Helpers for the global Custom-timer / Respawn alert preferences. A
// TimerAlertPref is the settings-level default; the Custom Timers overlay
// converts it into the per-timer TimerAlertThreshold[] that useTimerAlerts
// fires, while the Respawn alert hook reads the pref fields directly.

import type { TimerAlertPref } from '../types/config'
import type { TimerAlertThreshold } from '../types/trigger'

/** A sensible enabled-default for first-time setup of each alert kind. */
export function defaultTimerAlertPref(kind: 'custom' | 'respawn'): TimerAlertPref {
  return {
    enabled: true,
    // Custom timers usually want a short heads-up before completion; respawns
    // are most useful announced right as they pop (0 = at "POP").
    seconds: kind === 'custom' ? 5 : 0,
    type: 'text_to_speech',
    sound_path: '',
    volume: 80,
    tts_template: kind === 'custom' ? '{spell} done' : '{npc} has respawned',
    voice: '',
    tts_volume: 80,
  }
}

/**
 * Convert a Custom-timer alert preference into the timer_alerts payload sent
 * with a new manual timer. Returns [] when disabled, so the timer stays
 * silent. The single threshold uses the same {spell} placeholder convention
 * useTimerAlerts substitutes against the timer's name.
 */
export function customAlertThresholds(
  pref: TimerAlertPref | undefined,
): TimerAlertThreshold[] {
  if (!pref || !pref.enabled) return []
  return [
    {
      id: 'custom-timer-default',
      seconds: Math.max(0, pref.seconds || 0),
      type: pref.type,
      sound_path: pref.sound_path,
      volume: pref.volume,
      tts_template: pref.tts_template,
      voice: pref.voice,
      tts_volume: pref.tts_volume,
    },
  ]
}

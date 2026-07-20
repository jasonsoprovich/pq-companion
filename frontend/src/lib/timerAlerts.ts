// Helpers for the global Custom-timer / Respawn alert preferences. A
// TimerAlertPref is the settings-level default; the Custom Timers overlay
// converts it into the per-timer TimerAlertThreshold[] that useTimerAlerts
// fires, while the Respawn alert hook reads the pref fields directly.

import type { TimerAlertPref } from '../types/config'
import type { TimerAlertThreshold } from '../types/trigger'

export type TimerAlertKind = 'custom' | 'respawn' | 'metronome_start' | 'metronome_cast'

/** A sensible enabled-default for first-time setup of each alert kind. */
export function defaultTimerAlertPref(kind: TimerAlertKind): TimerAlertPref {
  // Metronome cues mark a split-second timing window (cast-now especially),
  // so they default louder than the other kinds' 80% to make sure they cut
  // through raid voice chat/game audio.
  const isMetronome = kind === 'metronome_start' || kind === 'metronome_cast'
  return {
    enabled: true,
    // Custom timers usually want a short heads-up before completion; respawns
    // are most useful announced right as they pop (0 = at "POP"). Metronome
    // alerts have no threshold concept — seconds is unused for those kinds.
    seconds: kind === 'custom' ? 5 : 0,
    type: 'text_to_speech',
    sound_path: '',
    volume: isMetronome ? 100 : 80,
    tts_template:
      kind === 'custom' ? '{spell} done'
      : kind === 'respawn' ? '{npc} has re-spawned'
      : kind === 'metronome_start' ? 'Chain starting'
      : 'Cast now',
    voice: '',
    tts_volume: isMetronome ? 100 : 80,
  }
}

/**
 * Fill any unset/invalid fields of a stored preference from the kind's
 * defaults, keeping the user's enabled flag and configured values. Returns a
 * complete, valid pref so the Settings editor never starts from an empty Type
 * (which would render a blank dropdown and an enabled-but-silent alert) and so
 * every onChange persists a fully-populated object. `undefined` (never
 * configured) yields the defaults with the alert left disabled.
 */
export function withTimerAlertDefaults(
  pref: TimerAlertPref | undefined,
  kind: TimerAlertKind,
): TimerAlertPref {
  const d = defaultTimerAlertPref(kind)
  if (!pref) return { ...d, enabled: false }
  return {
    enabled: pref.enabled,
    seconds: Number.isFinite(pref.seconds) ? pref.seconds : d.seconds,
    type: pref.type === 'play_sound' || pref.type === 'text_to_speech' ? pref.type : d.type,
    sound_path: pref.sound_path ?? d.sound_path,
    volume: pref.volume || d.volume,
    tts_template: pref.tts_template || d.tts_template,
    voice: pref.voice ?? d.voice,
    tts_volume: pref.tts_volume || d.tts_volume,
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

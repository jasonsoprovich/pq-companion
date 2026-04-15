/**
 * useTimerAlerts — watches overlay:timers WebSocket events and fires
 * audio alerts when a timer's remaining_seconds crosses a configured threshold.
 *
 * Mount once at the App level (alongside useAudioEngine) so alerts fire
 * regardless of which page the user is on.
 *
 * Algorithm: track the previous remaining_seconds for each timer. When a
 * timer crosses from above a threshold to at-or-below, fire the alert.
 * This handles recasts naturally — if the timer is refreshed and remaining
 * jumps back up, the threshold becomes "armed" again automatically.
 */
import { useCallback, useRef } from 'react'
import { useWebSocket } from './useWebSocket'
import { playSound, speakText } from '../services/audio'
import { loadTimerAlertConfig } from '../services/timerAlertStore'
import type { TimerState } from '../types/timer'

export function useTimerAlerts(): void {
  // prevRemaining tracks the last-seen remaining_seconds per timer ID.
  const prevRemaining = useRef<Map<string, number>>(new Map())

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== 'overlay:timers') return

    const state = msg.data as TimerState
    if (!state?.timers) return

    const cfg = loadTimerAlertConfig()
    if (!cfg.enabled || cfg.thresholds.length === 0) {
      prevRemaining.current.clear()
      return
    }

    const activeIds = new Set(state.timers.map((t) => t.id))

    // Fire alerts for timers that just crossed a threshold.
    for (const timer of state.timers) {
      const prev = prevRemaining.current.get(timer.id) ?? timer.remaining_seconds + 1

      for (const threshold of cfg.thresholds) {
        // Crossed: was above threshold last tick, now at or below.
        if (prev > threshold.seconds && timer.remaining_seconds <= threshold.seconds) {
          const spellName = timer.spell_name

          if (threshold.type === 'play_sound' && threshold.sound_path) {
            playSound(threshold.sound_path, threshold.volume / 100)
          } else if (threshold.type === 'text_to_speech' && threshold.tts_template) {
            const text = threshold.tts_template.replace('{spell}', spellName)
            speakText(text, threshold.voice, threshold.tts_volume / 100)
          }
        }
      }

      prevRemaining.current.set(timer.id, timer.remaining_seconds)
    }

    // Remove entries for timers that have expired or been removed.
    for (const id of prevRemaining.current.keys()) {
      if (!activeIds.has(id)) {
        prevRemaining.current.delete(id)
      }
    }
  }, [])

  useWebSocket(handleMessage)
}

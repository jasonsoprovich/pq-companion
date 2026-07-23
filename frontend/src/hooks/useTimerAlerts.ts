/**
 * useTimerAlerts — watches overlay:timers WebSocket events and fires
 * audio alerts when a timer's remaining_seconds crosses one of its
 * trigger-defined "fading soon" thresholds.
 *
 * Mount once at the App level (alongside useAudioEngine) so alerts fire
 * regardless of which page the user is on.
 *
 * Each ActiveTimer carries the trigger's `timer_alerts` list directly on
 * the WS payload, so this hook needs no separate config or trigger lookup.
 * Spell-cast-driven timers (no source trigger) have an empty/absent list
 * and produce no audio.
 *
 * Algorithm: track previous remaining_seconds per timer ID. When a timer
 * crosses from above a threshold to at-or-below, fire the alert. Recasts
 * naturally re-arm the threshold (remaining jumps back up).
 *
 * Each of the Buff / Detrimental / Custom overlay windows has its own bell
 * mute toggle in its header (see OverlayMuteButton), scoped to the timer
 * categories that window shows. This hook reads all three flags from
 * localStorage since it's mounted once at the App level, not inside any of
 * those windows.
 */
import { useCallback, useEffect, useRef } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { playSound, speakText } from '../services/audio'
import {
  BUFF_TIMER_ALERTS_KEY,
  CUSTOM_TIMER_ALERTS_KEY,
  DETRIM_TIMER_ALERTS_KEY,
  loadAlertsEnabled,
} from '../lib/overlayAlertMute'
import type { TimerCategory, TimerState } from '../types/timer'

const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

export function useTimerAlerts(): void {
  const prevRemaining = useRef<Map<string, number>>(new Map())
  const muteRef = useRef({
    buff: loadAlertsEnabled(BUFF_TIMER_ALERTS_KEY),
    detrim: loadAlertsEnabled(DETRIM_TIMER_ALERTS_KEY),
    custom: loadAlertsEnabled(CUSTOM_TIMER_ALERTS_KEY),
  })

  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === BUFF_TIMER_ALERTS_KEY) muteRef.current.buff = loadAlertsEnabled(BUFF_TIMER_ALERTS_KEY)
      else if (e.key === DETRIM_TIMER_ALERTS_KEY) muteRef.current.detrim = loadAlertsEnabled(DETRIM_TIMER_ALERTS_KEY)
      else if (e.key === CUSTOM_TIMER_ALERTS_KEY) muteRef.current.custom = loadAlertsEnabled(CUSTOM_TIMER_ALERTS_KEY)
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.OverlayTimers) return

    const state = msg.data as TimerState
    if (!state?.timers) return

    const activeIds = new Set(state.timers.map((t) => t.id))

    for (const timer of state.timers) {
      const alerts = timer.timer_alerts ?? []
      const prev = prevRemaining.current.get(timer.id) ?? timer.remaining_seconds + 1

      const muted =
        timer.category === 'buff'
          ? !muteRef.current.buff
          : DETRIM_CATEGORIES.has(timer.category)
            ? !muteRef.current.detrim
            : timer.category === 'custom'
              ? !muteRef.current.custom
              : false

      if (alerts.length > 0 && !muted) {
        for (const threshold of alerts) {
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
      }

      prevRemaining.current.set(timer.id, timer.remaining_seconds)
    }

    for (const id of prevRemaining.current.keys()) {
      if (!activeIds.has(id)) {
        prevRemaining.current.delete(id)
      }
    }
  }, [])

  useWebSocket(handleMessage)
}

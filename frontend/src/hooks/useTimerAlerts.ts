/**
 * useTimerAlerts — watches overlay:timers WebSocket events and fires
 * audio alerts when a timer's remaining_seconds crosses one of its
 * trigger-defined "fading soon" thresholds.
 *
 * Mount once at the App level (alongside useAudioEngine) so alerts fire
 * regardless of which page the user is on.
 *
 * Each ActiveTimer carries the trigger's `timer_alerts` list directly on
 * the WS payload, so this hook needs no separate config or trigger lookup
 * for trigger-driven timers. Spell-cast-driven timers (no source trigger)
 * have an empty/absent list; for those in a Detrimental category (debuff/
 * dot/mez/stun) this hook falls back to the global Preferences.detrim_timer_
 * alert default (Settings > Overlays), so a plain auto-detected mez/DoT
 * timer on any mob still gets a fading-soon cue without the user having to
 * hand-build a trigger for it. Buff-category native timers have no such
 * fallback (would be noisy — see 76fe0428's commit note).
 *
 * Algorithm: track previous remaining_seconds per timer ID. When a timer
 * crosses from above a threshold to at-or-below, fire the alert. Recasts
 * naturally re-arm the threshold (remaining jumps back up).
 *
 * Each of the Buff / Detrimental / Custom overlay windows has its own bell
 * mute toggle in its header (see OverlayMuteButton), scoped to the timer
 * categories that window shows. This hook reads all three flags from
 * localStorage since it's mounted once at the App level, not inside any of
 * those windows. The mute also gates the detrim_timer_alert fallback.
 */
import { useCallback, useEffect, useRef } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getConfig } from '../services/api'
import { playSound, speakText } from '../services/audio'
import {
  BUFF_TIMER_ALERTS_KEY,
  CUSTOM_TIMER_ALERTS_KEY,
  DETRIM_TIMER_ALERTS_KEY,
  loadAlertsEnabled,
} from '../lib/overlayAlertMute'
import type { TimerAlertPref } from '../types/config'
import type { TimerCategory, TimerState } from '../types/timer'

const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

export function useTimerAlerts(): void {
  const prevRemaining = useRef<Map<string, number>>(new Map())
  const muteRef = useRef({
    buff: loadAlertsEnabled(BUFF_TIMER_ALERTS_KEY),
    detrim: loadAlertsEnabled(DETRIM_TIMER_ALERTS_KEY),
    custom: loadAlertsEnabled(CUSTOM_TIMER_ALERTS_KEY),
  })
  // Global fallback alert for native (non-trigger) Detrimental timers, read
  // live so a Settings change takes effect without remounting this hook.
  const detrimPrefRef = useRef<TimerAlertPref | undefined>(undefined)

  useEffect(() => {
    getConfig()
      .then((c) => { detrimPrefRef.current = c.preferences?.detrim_timer_alert })
      .catch(() => {})
  }, [])

  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === BUFF_TIMER_ALERTS_KEY) muteRef.current.buff = loadAlertsEnabled(BUFF_TIMER_ALERTS_KEY)
      else if (e.key === DETRIM_TIMER_ALERTS_KEY) muteRef.current.detrim = loadAlertsEnabled(DETRIM_TIMER_ALERTS_KEY)
      else if (e.key === CUSTOM_TIMER_ALERTS_KEY) muteRef.current.custom = loadAlertsEnabled(CUSTOM_TIMER_ALERTS_KEY)
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const handleMessage = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigUpdated) {
      getConfig()
        .then((c) => { detrimPrefRef.current = c.preferences?.detrim_timer_alert })
        .catch(() => {})
      return
    }
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
      } else if (!muted && alerts.length === 0 && DETRIM_CATEGORIES.has(timer.category)) {
        const pref = detrimPrefRef.current
        if (pref?.enabled) {
          const threshold = Math.max(0, pref.seconds || 0)
          if (prev > threshold && timer.remaining_seconds <= threshold) {
            const spellName = timer.spell_name
            if (pref.type === 'play_sound' && pref.sound_path) {
              playSound(pref.sound_path, pref.volume / 100)
            } else if (pref.type === 'text_to_speech' && pref.tts_template) {
              const text = pref.tts_template.replace('{spell}', spellName)
              speakText(text, pref.voice, pref.tts_volume / 100)
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

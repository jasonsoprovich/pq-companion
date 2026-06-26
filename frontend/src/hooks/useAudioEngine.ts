import { useCallback, useRef } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { playSound, speakText, getRepeatAudioCooldownMs } from '../services/audio'
import type { TriggerFired } from '../types/trigger'

/**
 * useAudioEngine subscribes to WebSocket trigger:fired events and dispatches
 * any play_sound or text_to_speech actions to the audio service.
 *
 * Mount this hook once at the App level so audio fires regardless of which
 * page the user is on.
 */

// DEDUP_WINDOW_MS: kept in sync with TriggerOverlayWindowPage. A single
// trigger should never legitimately fire twice for the same matched line
// inside this window, so collapse duplicates to one audio play.
const DEDUP_WINDOW_MS = 750

export function useAudioEngine(): void {
  const lastFired = useRef<Map<string, number>>(new Map())
  // Per-trigger repeat-audio cooldown clock: trigger id → last time audio
  // played for it. Distinct from lastFired (which is keyed on the matched
  // line) — this collapses a trigger firing on *different* lines in quick
  // succession (AE mez wearing off several mobs) down to one audio alert.
  // Keyed by trigger id, so it's naturally bounded by the trigger count.
  const lastAudioByTrigger = useRef<Map<string, number>>(new Map())

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.TriggerFired) return

    const fired = msg.data as TriggerFired
    if (!fired?.actions) return

    const now = Date.now()
    const key = `${fired.trigger_id}|${fired.matched_line}`
    const prev = lastFired.current.get(key)
    if (prev !== undefined && now - prev < DEDUP_WINDOW_MS) return
    lastFired.current.set(key, now)
    if (lastFired.current.size > 256) {
      for (const [k, t] of lastFired.current) {
        if (now - t > DEDUP_WINDOW_MS) lastFired.current.delete(k)
      }
    }

    // Repeat-audio cooldown (Preferences.trigger_audio_cooldown_secs, applied
    // per trigger id). Only gates AUDIO here — overlay text lives in the
    // separate overlay window, and history/timers are produced backend-side,
    // so all of those still fire per match. 0 = disabled (no behaviour change).
    // Experimental: delete this block + the audio.ts/useAudioPrefs/Settings
    // wiring to remove the feature.
    const cooldownMs = getRepeatAudioCooldownMs()
    if (cooldownMs > 0) {
      const prevAudio = lastAudioByTrigger.current.get(fired.trigger_id)
      if (prevAudio !== undefined && now - prevAudio < cooldownMs) return
      lastAudioByTrigger.current.set(fired.trigger_id, now)
    }

    for (const action of fired.actions) {
      const vol = action.volume > 0 ? action.volume : 1.0

      if (action.type === 'play_sound') {
        playSound(action.sound_path, vol)
      } else if (action.type === 'text_to_speech') {
        speakText(action.text, action.voice, vol)
      }
    }
  }, [])

  useWebSocket(handleMessage)
}

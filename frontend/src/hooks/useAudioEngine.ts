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

// IDENTITY_TTL_MS bounds how long a played line's identity is remembered for
// the duplicate guard below. It only caps memory: a physical log line's
// identity (trigger + log-second + text) corresponds to exactly one real
// moment, so any repeat of it is a duplicate no matter how long ago — 60s is
// far longer than the seconds-late re-delivery this guards against.
const IDENTITY_TTL_MS = 60_000

export function useAudioEngine(): void {
  // Duplicate guard keyed on the line's IDENTITY (trigger id + log timestamp +
  // matched text), not on arrival time. The backend's polling log reader can,
  // in rare idle/filesystem-timing edges, dispatch the same physical line more
  // than once; those re-deliveries can land seconds apart in wall-clock, so the
  // old arrival-time window let them through and the alert played 2–3×. Keying
  // on the immutable line identity collapses any re-delivery to one play while
  // leaving genuinely distinct lines (different sender/text, or a later second)
  // to play freely — "one chime per read event," which is what the user wants.
  // Value is the wall-clock time we first played it, used only for TTL cleanup.
  // Audio-only: overlay text and trigger history fire per match as before.
  const playedIdentities = useRef<Map<string, number>>(new Map())
  // Per-trigger repeat-audio cooldown clock: trigger id → last time audio
  // played for it. Distinct from playedIdentities (keyed on the exact line) —
  // this collapses a trigger firing on *different* lines in quick succession
  // (AE mez wearing off several mobs) down to one audio alert. Keyed by
  // trigger id, so it's naturally bounded by the trigger count.
  const lastAudioByTrigger = useRef<Map<string, number>>(new Map())

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.TriggerFired) return

    const fired = msg.data as TriggerFired
    if (!fired?.actions) return

    const now = Date.now()
    // fired_at is the log line's own timestamp (1-second resolution) for log
    // triggers, so this key is the physical line's identity. A re-delivery of
    // the same line repeats it exactly; a genuinely new event differs in text
    // or second.
    const key = `${fired.trigger_id}|${fired.fired_at}|${fired.matched_line}`
    if (playedIdentities.current.has(key)) return
    playedIdentities.current.set(key, now)
    if (playedIdentities.current.size > 512) {
      for (const [k, t] of playedIdentities.current) {
        if (now - t > IDENTITY_TTL_MS) playedIdentities.current.delete(k)
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

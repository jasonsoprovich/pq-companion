import { useEffect } from 'react'
import { getConfig } from '../services/api'
import {
  setMasterVolume,
  setDefaultTTSVoice,
  setRepeatAudioCooldownMs,
} from '../services/audio'

const POLL_INTERVAL = 3000

/**
 * Polls the user config and pushes the audio-related preferences into the
 * audio service so subsequent playSound / speakText calls (and their test
 * variants) pick them up:
 *   - Preferences.master_volume scales every playback on top of each
 *     action's per-trigger volume.
 *   - Preferences.default_tts_voice is the voice used by any TTS alert
 *     whose own voice field is empty ("App default").
 *   - Preferences.trigger_audio_cooldown_secs rate-limits repeat audio from
 *     the same trigger (consumed in useAudioEngine). 0 = off.
 *
 * Mounted only inside MainWindowLayout — overlay windows don't fire alerts.
 */
export function useAudioPrefs(): void {
  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => {
          if (cancelled) return
          const pct = c.preferences?.master_volume
          // Treat missing/invalid as 100% so a transient backend hiccup
          // doesn't silently mute alerts.
          const value = typeof pct === 'number' && Number.isFinite(pct) ? pct : 100
          setMasterVolume(value / 100)
          setDefaultTTSVoice(c.preferences?.default_tts_voice ?? '')
          const cd = c.preferences?.trigger_audio_cooldown_secs
          setRepeatAudioCooldownMs(
            typeof cd === 'number' && Number.isFinite(cd) ? cd * 1000 : 0,
          )
        })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, POLL_INTERVAL)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])
}

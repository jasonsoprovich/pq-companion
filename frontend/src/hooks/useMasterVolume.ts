import { useEffect } from 'react'
import { getConfig } from '../services/api'
import { setMasterVolume } from '../services/audio'

const POLL_INTERVAL = 3000

/**
 * Polls the user config and pushes Preferences.master_volume into the audio
 * service so subsequent playSound / speakText calls (and their test variants)
 * are scaled by the master volume on top of each action's per-trigger volume.
 *
 * Mounted only inside MainWindowLayout — overlay windows don't fire alerts.
 */
export function useMasterVolume(): void {
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

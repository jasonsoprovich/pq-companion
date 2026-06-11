import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// usePoPEnabled returns the Planes of Power era flag
// (preferences.pop_enabled). It reads the config once on mount and tracks
// live changes via the config:updated broadcast, so flipping the Developer
// tab preview toggle swaps every consumer without a reload. Defaults to
// false (the pre-PoP era) until the config loads.
export function usePoPEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.pop_enabled))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.pop_enabled))
  })

  return enabled
}

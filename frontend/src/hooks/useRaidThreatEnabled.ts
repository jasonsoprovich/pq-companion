import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// useRaidThreatEnabled returns the experimental raid-estimate threat flag
// (preferences.raid_threat_enabled). Reads config once on mount and tracks
// live changes via config:updated, so flipping the Developer-tab toggle shows
// or hides the Solo/Raid control on the threat overlay without a reload.
// Defaults to false until config loads.
export function useRaidThreatEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.raid_threat_enabled))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.raid_threat_enabled))
  })

  return enabled
}

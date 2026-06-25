import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// useTraderTrackerEnabled returns the Bazaar Trader Tracker feature flag
// (preferences.trader_tracker_enabled). It reads the config once on mount and
// tracks live changes via the config:updated broadcast, so flipping the
// Developer-tab toggle shows/hides the nav entry without a reload. Defaults to
// false until the config loads. Mirrors useResistCalcEnabled.
export function useTraderTrackerEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.trader_tracker_enabled))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.trader_tracker_enabled))
  })

  return enabled
}

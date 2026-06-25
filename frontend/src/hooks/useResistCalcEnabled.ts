import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// useResistCalcEnabled returns the resist-calculator feature flag
// (preferences.resist_calc_enabled). It reads the config once on mount and
// tracks live changes via the config:updated broadcast, so flipping the
// Developer-tab toggle shows/hides the nav entry without a reload. Defaults to
// false until the config loads. Mirrors usePoPEnabled.
export function useResistCalcEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.resist_calc_enabled))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.resist_calc_enabled))
  })

  return enabled
}

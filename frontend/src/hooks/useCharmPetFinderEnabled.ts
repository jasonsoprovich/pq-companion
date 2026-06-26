import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// useCharmPetFinderEnabled returns the charm-pet-finder feature flag
// (preferences.charm_pet_finder_enabled). It reads the config once on mount and
// tracks live changes via the config:updated broadcast, so flipping the
// Developer-tab toggle shows/hides the nav entry without a reload. Defaults to
// false until the config loads. Mirrors useResistCalcEnabled.
export function useCharmPetFinderEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.charm_pet_finder_enabled))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.charm_pet_finder_enabled))
  })

  return enabled
}

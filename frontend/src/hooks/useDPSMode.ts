import { useCallback, useEffect, useState } from 'react'
import type { EntityStats, HealerStats } from '../types/combat'

/**
 * Which DPS / HPS metric the meter shows.
 *
 * 'active'   — total damage divided by the union of intervals during which
 *              this combatant was actually engaging. Throughput rate while
 *              engaged. Default — fairer for DoT casters and mez-duty
 *              enchanters who shouldn't be punished for legitimate downtime.
 * 'duration' — total damage divided by the fight's wall-clock duration.
 *              Same denominator for everyone. Contribution rate.
 */
export type DPSMode = 'active' | 'duration'

const STORAGE_KEY = 'pq-dps-mode'

function readStored(): DPSMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === 'duration' ? 'duration' : 'active'
  } catch {
    return 'active'
  }
}

export function useDPSMode(): {
  mode: DPSMode
  toggle: () => void
} {
  const [mode, setMode] = useState<DPSMode>(() => readStored())

  // Pick up changes from sibling renderer processes (popout overlay vs
  // in-app panel). localStorage 'storage' events fire on other windows only.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setMode(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const toggle = useCallback(() => {
    setMode((prev) => {
      const next: DPSMode = prev === 'active' ? 'duration' : 'active'
      try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
      return next
    })
  }, [])

  return { mode, toggle }
}

/** DPS value to display for a combatant under the current mode. */
export function dpsForMode(c: Pick<EntityStats, 'dps' | 'active_dps'>, mode: DPSMode): number {
  return mode === 'active' ? c.active_dps : c.dps
}

/** HPS value to display for a healer under the current mode. */
export function hpsForMode(h: Pick<HealerStats, 'hps' | 'active_hps'>, mode: DPSMode): number {
  return mode === 'active' ? h.active_hps : h.hps
}

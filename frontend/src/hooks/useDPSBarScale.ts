import { useCallback, useEffect, useState } from 'react'

/**
 * What a DPS meter row's background bar is measured against. The % column
 * always shows share of total damage; this only changes the bar fill.
 *
 * 'leader' — the top row fills 100% and everyone else is scaled relative to
 *            it (EQLogParser / Details! style). Readable in raids, where
 *            share-of-total bars are all nearly empty.
 * 'total'  — each bar is the row's share of the fight's total damage. The
 *            original behaviour.
 */
export type DPSBarScale = 'leader' | 'total'

const STORAGE_KEY = 'pq-dps-bar-scale'

function readStored(): DPSBarScale {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'leader' || v === 'total') return v
  } catch {
    /* noop */
  }
  return 'leader'
}

export function useDPSBarScale(): {
  barScale: DPSBarScale
  toggle: () => void
} {
  const [barScale, setBarScale] = useState<DPSBarScale>(() => readStored())

  // Mirror the choice across sibling renderer processes (in-app panel vs
  // popped-out overlay). 'storage' events fire on other windows only.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setBarScale(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const toggle = useCallback(() => {
    setBarScale((prev) => {
      const next: DPSBarScale = prev === 'leader' ? 'total' : 'leader'
      try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
      return next
    })
  }, [])

  return { barScale, toggle }
}

/** Tooltip for the bar-scale toggle button. */
export function dpsBarScaleTooltip(scale: DPSBarScale): string {
  return scale === 'leader'
    ? 'Bars scaled to the top damage dealer (leader = full bar) — click to show share of total damage'
    : 'Bars show share of total damage — click to scale bars to the top damage dealer'
}

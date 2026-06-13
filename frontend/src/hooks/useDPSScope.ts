import { useCallback, useEffect, useState } from 'react'

/**
 * Which span of combat the meter aggregates over. This is orthogonal to
 * DPSMode (personal/raid/encounter): scope picks WHICH fights are pooled,
 * mode picks HOW each combatant's seconds are counted within them.
 *
 * 'current' — the live fight (or the just-finished one). The original
 *             behaviour; shows the mob you're on right now.
 * 'window'  — a pooled moving average over the last N completed fights
 *             (see ROLLING_WINDOW_SIZE). Performance over time rather than a
 *             single pull — better for groups grinding many mobs where any
 *             one fight is too short to read.
 */
export type DPSScope = 'current' | 'window'

const STORAGE_KEY = 'pq-dps-scope'

function readStored(): DPSScope {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'current' || v === 'window') return v
  } catch {
    /* noop */
  }
  return 'current'
}

export function useDPSScope(): {
  scope: DPSScope
  toggle: () => void
  setScope: (s: DPSScope) => void
} {
  const [scope, setScopeState] = useState<DPSScope>(() => readStored())

  // Mirror the choice across sibling renderer processes (in-app panel vs
  // popped-out overlay). 'storage' events fire on other windows only.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setScopeState(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setScope = useCallback((next: DPSScope) => {
    setScopeState(next)
    try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
  }, [])

  const toggle = useCallback(() => {
    setScopeState((prev) => {
      const next: DPSScope = prev === 'current' ? 'window' : 'current'
      try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
      return next
    })
  }, [])

  return { scope, toggle, setScope }
}

/** Short label for the scope toggle button / header. */
export function dpsScopeLabel(scope: DPSScope): string {
  return scope === 'window' ? 'Last 20' : 'Current'
}

import { useCallback, useEffect, useState } from 'react'
import type { ActiveTimer } from '../types/timer'

/**
 * Sort mode for the buff overlay.
 *
 * 'remaining' — least time remaining first (default; matches the backend
 *               snapshot order).
 * 'recent'    — most recently cast first; stable across the lifetime of
 *               the timer so the row doesn't shuffle as time ticks down.
 */
export type BuffSortMode = 'remaining' | 'recent'

const STORAGE_KEY = 'pq-buff-sort-mode'

function readStored(): BuffSortMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === 'recent' ? 'recent' : 'remaining'
  } catch {
    return 'remaining'
  }
}

export function useBuffSortMode(): {
  mode: BuffSortMode
  toggle: () => void
} {
  const [mode, setMode] = useState<BuffSortMode>(() => readStored())

  // Pick up changes made in another window (the standalone overlay and the
  // in-app dashboard panel are separate webContents).
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setMode(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const toggle = useCallback(() => {
    setMode((prev) => {
      const next: BuffSortMode = prev === 'remaining' ? 'recent' : 'remaining'
      try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
      return next
    })
  }, [])

  return { mode, toggle }
}

export function sortBuffs(timers: ActiveTimer[], mode: BuffSortMode): ActiveTimer[] {
  if (mode === 'recent') {
    // cast_at is RFC3339 — descending lexicographic sort = newest first.
    return [...timers].sort((a, b) => (a.cast_at < b.cast_at ? 1 : a.cast_at > b.cast_at ? -1 : 0))
  }
  // 'remaining': least time first, with cast_at as a stable tiebreak.
  return [...timers].sort((a, b) => {
    if (a.remaining_seconds !== b.remaining_seconds) {
      return a.remaining_seconds - b.remaining_seconds
    }
    return a.cast_at < b.cast_at ? -1 : a.cast_at > b.cast_at ? 1 : 0
  })
}

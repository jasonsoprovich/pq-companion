import { useCallback, useEffect, useState } from 'react'

/**
 * "Pinned only" view toggle for the Respawn Timer overlay/panel — hides
 * every timer the player hasn't pinned, so a camp full of trash kills
 * doesn't bury the one being tracked. Per-viewer, localStorage-backed
 * (default off), synced across windows the same way useBuffSortMode is.
 */
const STORAGE_KEY = 'pq-respawn-pinned-only'

function readStored(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

export function useRespawnPinnedOnly(): [boolean, () => void] {
  const [pinnedOnly, setPinnedOnly] = useState<boolean>(() => readStored())

  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setPinnedOnly(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const toggle = useCallback(() => {
    setPinnedOnly((prev) => {
      const next = !prev
      try {
        localStorage.setItem(STORAGE_KEY, String(next))
      } catch {
        /* noop */
      }
      return next
    })
  }, [])

  return [pinnedOnly, toggle]
}

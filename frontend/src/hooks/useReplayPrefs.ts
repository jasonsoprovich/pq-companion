import { useCallback, useState } from 'react'

/**
 * Persisted Replay form selections (file / from / to / speed).
 *
 * The Replay panel lives inside the Log Feed page, which unmounts on every
 * tab change — and the panel itself collapses when toggled off. Holding the
 * selections in component state meant the file and date/time range reset any
 * time the user navigated away, paused to look at something, or closed the
 * panel, which is painful while iterating on triggers.
 *
 * These prefs survive navigation by living in localStorage, but self-expire
 * after IDLE_TTL_MS of inactivity (each write re-stamps savedAt) so a stale
 * day-old range doesn't silently pre-fill a fresh session.
 */

const STORAGE_KEY = 'pq.replayPrefs'
const IDLE_TTL_MS = 30 * 60 * 1000 // 30 minutes since the last change

export interface ReplayPrefs {
  file: string
  fromStr: string
  toStr: string
  speed: number
}

const DEFAULTS: ReplayPrefs = { file: '', fromStr: '', toStr: '', speed: 1 }

function load(): ReplayPrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<ReplayPrefs> & { savedAt?: number }
    if (typeof parsed.savedAt !== 'number' || Date.now() - parsed.savedAt > IDLE_TTL_MS) {
      localStorage.removeItem(STORAGE_KEY)
      return { ...DEFAULTS }
    }
    return {
      file: parsed.file ?? DEFAULTS.file,
      fromStr: parsed.fromStr ?? DEFAULTS.fromStr,
      toStr: parsed.toStr ?? DEFAULTS.toStr,
      speed: parsed.speed ?? DEFAULTS.speed,
    }
  } catch {
    return { ...DEFAULTS }
  }
}

function save(prefs: ReplayPrefs): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...prefs, savedAt: Date.now() }))
  } catch {
    // localStorage may be unavailable (private mode, quota) — selections just
    // won't persist; the panel still works for the current session.
  }
}

/**
 * Returns the current Replay selections and a patch setter. Every patch merges
 * into the existing prefs, persists them (re-stamping the idle timer), and
 * re-renders.
 */
export function useReplayPrefs(): [ReplayPrefs, (patch: Partial<ReplayPrefs>) => void] {
  const [prefs, setPrefs] = useState<ReplayPrefs>(load)
  const patch = useCallback((p: Partial<ReplayPrefs>) => {
    setPrefs((prev) => {
      const next = { ...prev, ...p }
      save(next)
      return next
    })
  }, [])
  return [prefs, patch]
}

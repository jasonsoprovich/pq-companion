import { useCallback, useEffect, useState } from 'react'

// ModelPreviewPrefs is how the item and NPC pages show 3D models.
export interface ModelPreviewPrefs {
  // show turns the 3D previews on. Turned off, the pages never start
  // WebGL, for machines it's heavy on.
  show: boolean
  // luclin shows player races as their Luclin models.
  luclin: boolean
  // gender (0 male, 1 female) and race (0 for the active character's, if
  // it can wear the item) armor is shown on.
  gender: number
  race: number
}

const KEY = 'models.preview'
const DEFAULTS: ModelPreviewPrefs = { show: true, luclin: true, gender: 0, race: 0 }

function load(): ModelPreviewPrefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return DEFAULTS
    const p = JSON.parse(raw) as Partial<ModelPreviewPrefs>
    return {
      show: typeof p.show === 'boolean' ? p.show : DEFAULTS.show,
      luclin: typeof p.luclin === 'boolean' ? p.luclin : DEFAULTS.luclin,
      gender: p.gender === 1 ? 1 : 0,
      race: typeof p.race === 'number' && p.race > 0 ? p.race : 0,
    }
  } catch {
    return DEFAULTS
  }
}

const listeners = new Set<(p: ModelPreviewPrefs) => void>()

// useModelPreviewPrefs is the item and NPC pages' 3D preview settings,
// shared between them and kept across restarts.
export function useModelPreviewPrefs(): [ModelPreviewPrefs, (patch: Partial<ModelPreviewPrefs>) => void] {
  const [prefs, setPrefs] = useState<ModelPreviewPrefs>(load)
  useEffect(() => {
    listeners.add(setPrefs)
    return () => {
      listeners.delete(setPrefs)
    }
  }, [])
  const update = useCallback((patch: Partial<ModelPreviewPrefs>) => {
    const next = { ...load(), ...patch }
    try {
      localStorage.setItem(KEY, JSON.stringify(next))
    } catch {
      /* noop */
    }
    listeners.forEach((fn) => fn(next))
  }, [])
  return [prefs, update]
}

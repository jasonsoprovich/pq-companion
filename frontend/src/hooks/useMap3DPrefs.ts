import { useCallback, useState } from 'react'

// Map3DStyle is how the 3D view colours the zone: the client's own
// textures, a height ramp, or plain floors and walls.
export type Map3DStyle = 'textured' | 'height' | 'plain'

// Map3DLabels is whose names are drawn: nobody's but the followed marker,
// you and your group, or everyone close enough to read.
export type Map3DLabels = 'off' | 'group' | 'all'

export interface Map3DPrefs {
  style: Map3DStyle
  // wallOpacity applies to walls and ceilings (0.1–1); floors stay solid so
  // there's always ground to read.
  wallOpacity: number
  // seeThroughOuter hides the back of every face, so from outside a
  // building or dungeon you look in through its walls instead of at them.
  seeThroughOuter: boolean
  hideCeilings: boolean
  // showObjects draws the zone's placed objects (trees, buildings, lamps,
  // furniture) and its doors.
  showObjects: boolean
  labels: Map3DLabels
}

const KEY = 'map3d.prefs'
const DEFAULTS: Map3DPrefs = {
  style: 'textured', wallOpacity: 1, seeThroughOuter: true, hideCeilings: false, showObjects: true,
  labels: 'group',
}
const STYLES: readonly Map3DStyle[] = ['textured', 'height', 'plain']
const LABELS: readonly Map3DLabels[] = ['off', 'group', 'all']

function load(): Map3DPrefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return DEFAULTS
    const p = JSON.parse(raw) as Partial<Map3DPrefs>
    return {
      style: STYLES.includes(p.style as Map3DStyle) ? (p.style as Map3DStyle) : DEFAULTS.style,
      wallOpacity: typeof p.wallOpacity === 'number' ? Math.min(1, Math.max(0.1, p.wallOpacity)) : DEFAULTS.wallOpacity,
      seeThroughOuter: typeof p.seeThroughOuter === 'boolean' ? p.seeThroughOuter : DEFAULTS.seeThroughOuter,
      hideCeilings: typeof p.hideCeilings === 'boolean' ? p.hideCeilings : DEFAULTS.hideCeilings,
      showObjects: typeof p.showObjects === 'boolean' ? p.showObjects : DEFAULTS.showObjects,
      labels: LABELS.includes(p.labels as Map3DLabels) ? (p.labels as Map3DLabels) : DEFAULTS.labels,
    }
  } catch {
    return DEFAULTS
  }
}

// useMap3DPrefs is the 3D map's look, shared by every 3D surface (Maps,
// Live Map, Raid Replay) and kept across restarts.
export function useMap3DPrefs(): [Map3DPrefs, (patch: Partial<Map3DPrefs>) => void] {
  const [prefs, setPrefs] = useState<Map3DPrefs>(load)
  const update = useCallback((patch: Partial<Map3DPrefs>) => {
    setPrefs((prev) => {
      const next = { ...prev, ...patch }
      try {
        localStorage.setItem(KEY, JSON.stringify(next))
      } catch {
        /* noop */
      }
      return next
    })
  }, [])
  return [prefs, update]
}

import { useCallback, useEffect, useState } from 'react'

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
  // seeInvisEyes marks aggro rings whose mob can see invisible with an eye,
  // and dashFaint dashes the rings of green mobs that won't attack on
  // sight, both as the 2D map does.
  seeInvisEyes: boolean
  dashFaint: boolean
  // cutAbove hides everything from the ceiling over you up while the Top
  // or Chase camera follows you, so the floors over you in a stacked zone
  // (Bastion of Thunder, Plane of Knowledge) don't block the view down.
  cutAbove: boolean
}

const KEY = 'map3d.prefs'
const DEFAULTS: Map3DPrefs = {
  style: 'textured', wallOpacity: 1, seeThroughOuter: true, hideCeilings: false, showObjects: true,
  labels: 'group',
  seeInvisEyes: true,
  dashFaint: true,
  cutAbove: true,
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
      seeInvisEyes: typeof p.seeInvisEyes === 'boolean' ? p.seeInvisEyes : DEFAULTS.seeInvisEyes,
      dashFaint: typeof p.dashFaint === 'boolean' ? p.dashFaint : DEFAULTS.dashFaint,
      cutAbove: typeof p.cutAbove === 'boolean' ? p.cutAbove : DEFAULTS.cutAbove,
    }
  } catch {
    return DEFAULTS
  }
}

// Same-window listeners keep every mounted 3D view in step; the storage
// event carries a change to the map overlay's window, which has no options
// of its own and so follows the Live Map's.
const listeners = new Set<(p: Map3DPrefs) => void>()

// useMap3DPrefs is the 3D map's look, shared by every 3D surface (Maps,
// Live Map, Raid Replay, the map overlay) and kept across restarts.
export function useMap3DPrefs(): [Map3DPrefs, (patch: Partial<Map3DPrefs>) => void] {
  const [prefs, setPrefs] = useState<Map3DPrefs>(load)
  useEffect(() => {
    listeners.add(setPrefs)
    const onStorage = (e: StorageEvent): void => {
      if (e.key === KEY) setPrefs(load())
    }
    window.addEventListener('storage', onStorage)
    return () => {
      listeners.delete(setPrefs)
      window.removeEventListener('storage', onStorage)
    }
  }, [])
  const update = useCallback((patch: Partial<Map3DPrefs>) => {
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

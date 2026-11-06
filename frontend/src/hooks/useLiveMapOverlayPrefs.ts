import { useCallback, useEffect, useState } from 'react'
import type { Map3DLabels } from './useMap3DPrefs'
import type { AggroScope } from '../components/maps/spawnOverlays'
import type { Camera3D } from '../components/maps/ZoneMap3D'

// LiveMapOverlayPrefs is what the map overlay draws. It's set from the
// overlay's own toolbar while unlocked, or from Settings → Maps, which
// matters once the overlay is locked and clicks go through it.
export interface LiveMapOverlayPrefs {
  view3D: boolean
  // camera3D is the 3D camera; headingUp turns the 2D map with you.
  camera3D: Camera3D
  headingUp: boolean
  // group null follows the Settings default for showing groupmates.
  group: boolean | null
  raid: boolean
  raidColor: 'group' | 'class'
  target: boolean
  aggro: AggroScope
  aggroNear: number
  dots: boolean
  hideKilled: boolean
  labels: Map3DLabels
}

export const LIVE_MAP_OVERLAY_DEFAULTS: LiveMapOverlayPrefs = {
  view3D: false,
  camera3D: 'free',
  headingUp: false,
  group: null,
  raid: true,
  raidColor: 'group',
  target: true,
  aggro: 'off',
  aggroNear: 300,
  dots: false,
  hideKilled: true,
  labels: 'group',
}

// Kept in localStorage, which the overlay window shares with the main
// window, so a change in Settings reaches an open overlay through the
// storage event; same-window listeners cover the dashboard copy.
const KEY = 'liveMapOverlay.prefs'
const listeners = new Set<(p: LiveMapOverlayPrefs) => void>()

function load(): LiveMapOverlayPrefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return LIVE_MAP_OVERLAY_DEFAULTS
    const p = JSON.parse(raw) as Partial<LiveMapOverlayPrefs>
    const d = LIVE_MAP_OVERLAY_DEFAULTS
    const bool = (v: unknown, def: boolean): boolean => (typeof v === 'boolean' ? v : def)
    return {
      view3D: bool(p.view3D, d.view3D),
      camera3D: p.camera3D === 'top' || p.camera3D === 'chase' ? p.camera3D : 'free',
      headingUp: bool(p.headingUp, d.headingUp),
      group: typeof p.group === 'boolean' ? p.group : null,
      raid: bool(p.raid, d.raid),
      raidColor: p.raidColor === 'class' ? 'class' : 'group',
      target: bool(p.target, d.target),
      aggro: p.aggro === 'near' || p.aggro === 'all' ? p.aggro : 'off',
      aggroNear: typeof p.aggroNear === 'number' ? Math.min(1500, Math.max(100, p.aggroNear)) : d.aggroNear,
      dots: bool(p.dots, d.dots),
      hideKilled: bool(p.hideKilled, d.hideKilled),
      labels: p.labels === 'off' || p.labels === 'all' ? p.labels : 'group',
    }
  } catch {
    return LIVE_MAP_OVERLAY_DEFAULTS
  }
}

// useLiveMapOverlayPrefs is the map overlay's settings, kept across restarts
// and in step between the overlay, its dashboard copy and Settings.
export function useLiveMapOverlayPrefs(): [LiveMapOverlayPrefs, (patch: Partial<LiveMapOverlayPrefs>) => void] {
  const [prefs, setPrefs] = useState<LiveMapOverlayPrefs>(load)
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
  const update = useCallback((patch: Partial<LiveMapOverlayPrefs>) => {
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

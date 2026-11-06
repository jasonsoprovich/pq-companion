import { useCallback, useEffect, useState } from 'react'
import type { MapPOICategory } from '../types/map'

// DEFAULT_MAP_LAYERS is what a player is usually hunting for, doors
// included. Ground spawns, tradeskill containers and succor points start
// off: switching everything on at once is what makes dense zones unreadable.
export const DEFAULT_MAP_LAYERS: MapPOICategory[] = [
  'vendor', 'raid_target', 'zone_line', 'trap', 'locked', 'teleport', 'switch',
  'wall', 'hazard', 'note', 'door',
]

// Kept in localStorage, which the map overlay window shares with the main
// window, so turning doors on in the Live Map reaches an open overlay
// through the storage event; same-window listeners cover the Maps page,
// the Zones tab and the overlay's dashboard copy.
const KEY = 'maps.layers'
// DOORS_KEY marks the one-time switch of doors on for lists saved before
// doors were a default; after it, a user's doors-off choice is kept.
const DOORS_KEY = 'maps.layers.doorsDefault'
const listeners = new Set<(l: MapPOICategory[]) => void>()

function load(): MapPOICategory[] {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return DEFAULT_MAP_LAYERS
    const l = JSON.parse(raw) as unknown
    if (!Array.isArray(l)) return DEFAULT_MAP_LAYERS
    const layers = l.filter((c): c is MapPOICategory => typeof c === 'string')
    if (localStorage.getItem(DOORS_KEY) === null) {
      localStorage.setItem(DOORS_KEY, '1')
      if (!layers.includes('door')) {
        layers.push('door')
        localStorage.setItem(KEY, JSON.stringify(layers))
      }
    }
    return layers
  } catch {
    return DEFAULT_MAP_LAYERS
  }
}

// useMapLayers is which POI layers the maps show, kept across restarts and
// in step between every map surface, the overlay included.
export function useMapLayers(): [
  MapPOICategory[],
  (value: MapPOICategory[] | ((prev: MapPOICategory[]) => MapPOICategory[])) => void,
] {
  const [layers, setLayers] = useState<MapPOICategory[]>(load)
  useEffect(() => {
    listeners.add(setLayers)
    const onStorage = (e: StorageEvent): void => {
      if (e.key === KEY) setLayers(load())
    }
    window.addEventListener('storage', onStorage)
    return () => {
      listeners.delete(setLayers)
      window.removeEventListener('storage', onStorage)
    }
  }, [])
  const update = useCallback(
    (value: MapPOICategory[] | ((prev: MapPOICategory[]) => MapPOICategory[])) => {
      const next = typeof value === 'function' ? value(load()) : value
      try {
        localStorage.setItem(KEY, JSON.stringify(next))
        localStorage.setItem(DOORS_KEY, '1')
      } catch {
        /* noop */
      }
      listeners.forEach((fn) => fn(next))
    },
    [],
  )
  return [layers, update]
}

import { useEffect, useState } from 'react'
import { getMapZones } from '../services/api'
import type { MapZone } from '../types/map'

// Module-level cache: the zone list is ~178 small rows, identical for the life
// of an install, and several surfaces ask "does this zone have a map?" while
// the user clicks around. Fetching it once avoids a request per navigation.
let cache: MapZone[] | null = null
let inflight: Promise<MapZone[]> | null = null

function load(): Promise<MapZone[]> {
  if (cache) return Promise.resolve(cache)
  if (!inflight) {
    inflight = getMapZones()
      .then((r) => {
        cache = r.zones ?? []
        return cache
      })
      // A build without maps.db is a normal state, not an error: report an
      // empty list so callers simply hide their map UI.
      .catch(() => {
        cache = []
        return cache
      })
      .finally(() => {
        inflight = null
      })
  }
  return inflight
}

export function useMapZones(): { zones: MapZone[]; loaded: boolean } {
  const [zones, setZones] = useState<MapZone[]>(cache ?? [])
  const [loaded, setLoaded] = useState(cache !== null)

  useEffect(() => {
    if (cache !== null) return
    let cancelled = false
    load().then((z) => {
      if (cancelled) return
      setZones(z)
      setLoaded(true)
    })
    return () => { cancelled = true }
  }, [])

  return { zones, loaded }
}

// useHasMap reports whether a zone short name has map data.
export function useHasMap(shortName: string | null | undefined): boolean {
  const { zones } = useMapZones()
  if (!shortName) return false
  return zones.some((z) => z.zone === shortName)
}

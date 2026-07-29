import { useEffect, useState } from 'react'
import { getMapGeometry, getMapZone } from '../services/api'
import type { MapGeometry, MapPOI, MapZone } from '../types/map'

interface ZoneMapData {
  zone: MapZone | null
  geometry: MapGeometry | null
  detail: MapGeometry | null
  pois: MapPOI[]
  loading: boolean
  error: string | null
}

// useZoneMap loads one zone's metadata, POIs and geometry.
//
// Metadata and POIs land first so pins and controls can render immediately;
// geometry is the big payload and arrives after. Both are discarded if the
// zone changes mid-flight, so fast switching can't paint one zone's lines over
// another's.
export function useZoneMap(zone: string | null): ZoneMapData {
  const [data, setData] = useState<ZoneMapData>({
    zone: null, geometry: null, detail: null, pois: [], loading: false, error: null,
  })

  useEffect(() => {
    if (!zone) {
      setData({ zone: null, geometry: null, detail: null, pois: [], loading: false, error: null })
      return
    }
    let cancelled = false
    setData((d) => ({ ...d, loading: true, error: null }))

    getMapZone(zone)
      .then((detail) => {
        if (cancelled) return
        setData({
          zone: detail.zone, pois: detail.pois ?? [],
          geometry: null, detail: null, loading: true, error: null,
        })
        // Base layer first so something draws immediately; the optional
        // boundary-detail layer follows and refines it.
        return getMapGeometry(zone, 0)
      })
      .then((geom) => {
        if (cancelled || !geom) return
        setData((d) => ({ ...d, geometry: geom, loading: false }))
        return getMapGeometry(zone, 1)
      })
      .then((det) => {
        // A zone with no detail layer returns zero segments, not an error.
        if (cancelled || !det || det.count === 0) return
        setData((d) => ({ ...d, detail: det }))
      })
      .catch((err: Error) => {
        if (cancelled) return
        setData({
          zone: null, geometry: null, detail: null, pois: [], loading: false,
          error: err.message,
        })
      })

    return () => { cancelled = true }
  }, [zone])

  return data
}

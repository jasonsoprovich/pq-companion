import { useEffect, useRef, useState } from 'react'
import { getExternalMapGeometry, getMapGeometry, getMapZone } from '../services/api'
import type { MapGeometry, MapPOI, MapRenderMode, MapZone } from '../types/map'

// Layer numbers, mirroring backend/internal/mapgen/mapsdb.go.
const LAYER_GEOMETRY = 0
const LAYER_DETAIL = 1
const LAYER_OUTLINE = 2

interface ZoneMapData {
  zone: MapZone | null
  // outline is the clean layer, drawn in outline mode.
  outline: MapGeometry | null
  // geometry and detail are the classifier's own layers, drawn in detailed mode.
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
//
// Layers are fetched on demand rather than all at once. Outline mode is the
// default and needs only layer 2; the detailed layers together run several
// times larger, and most sessions never ask for them. Once fetched a layer is
// kept, so toggling back and forth costs nothing.
export function useZoneMap(zone: string | null, mode: MapRenderMode): ZoneMapData {
  const [data, setData] = useState<ZoneMapData>({
    zone: null, outline: null, geometry: null, detail: null,
    pois: [], loading: false, error: null,
  })
  // Tracks which (zone, mode) geometry fetches have been started, so a re-render
  // does not refetch what is already in hand.
  const fetched = useRef<Set<string>>(new Set())

  useEffect(() => {
    if (!zone) {
      setData({
        zone: null, outline: null, geometry: null, detail: null,
        pois: [], loading: false, error: null,
      })
      return
    }
    let cancelled = false
    fetched.current = new Set()
    setData((d) => ({ ...d, loading: true, error: null }))

    getMapZone(zone)
      .then((detail) => {
        if (cancelled) return
        setData({
          zone: detail.zone, pois: detail.pois ?? [],
          outline: null, geometry: null, detail: null, loading: true, error: null,
        })
      })
      .catch((err: Error) => {
        if (cancelled) return
        setData({
          zone: null, outline: null, geometry: null, detail: null,
          pois: [], loading: false, error: err.message,
        })
      })

    return () => { cancelled = true }
  }, [zone])

  // Geometry for the active mode, fetched once the zone metadata is in.
  //
  // The guard must compare the loaded zone to the requested one, not merely
  // check that some metadata exists. On a zone change both effects run in the
  // same commit with data.zone still holding the *previous* zone, so this
  // effect would start the new zone's geometry fetch, then be torn down and
  // cancelled the moment the new metadata landed and changed data.zone. The
  // tag was already recorded as fetched, so nothing ever retried: POIs drew
  // (they come from the metadata call) and the map had no lines until the
  // window was closed and reopened, which remounted into the no-race path.
  const loadedZone = data.zone?.zone
  useEffect(() => {
    if (!zone || loadedZone !== zone) return
    const tag = `${zone}:${mode}`
    if (fetched.current.has(tag)) return
    fetched.current.add(tag)

    let cancelled = false
    setData((d) => ({ ...d, loading: true }))

    const load =
      mode === 'external'
        ? // A user-installed pack. Stored in `outline` because it is drawn the
          // same way — one flat layer, no detail companion — so the renderer
          // needs no third slot.
          getExternalMapGeometry(zone).then((outline) => {
            if (!cancelled) setData((d) => ({ ...d, outline, loading: false }))
          })
        : mode === 'outline'
        ? getMapGeometry(zone, LAYER_OUTLINE).then((outline) => {
            if (!cancelled) setData((d) => ({ ...d, outline, loading: false }))
          })
        : getMapGeometry(zone, LAYER_GEOMETRY)
            .then((geometry) => {
              if (cancelled) return
              setData((d) => ({ ...d, geometry, loading: false }))
              return getMapGeometry(zone, LAYER_DETAIL)
            })
            .then((det) => {
              // A zone with no detail layer returns zero segments, not an error.
              if (cancelled || !det || det.count === 0) return
              setData((d) => ({ ...d, detail: det }))
            })

    load.catch((err: Error) => {
      if (cancelled) return
      // Geometry failing is not fatal — metadata and POIs are already drawn, so
      // degrade to pins on an empty canvas rather than blanking the panel.
      fetched.current.delete(tag)
      setData((d) => ({ ...d, loading: false, error: err.message }))
    })

    return () => { cancelled = true }
  }, [zone, mode, loadedZone])

  return data
}

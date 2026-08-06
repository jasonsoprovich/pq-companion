import { useEffect, useState } from 'react'
import { getConfig, getExternalMapStatus } from '../services/api'
import type { ExternalMapStatus, MapRenderMode } from '../types/map'

// DEFAULT_STYLE matches the backend's default. Detailed carries the most
// information and wins in the large majority of zones; outline omits anything
// that is not a wall, which loses features like Oasis of Marr's lake.
const DEFAULT_STYLE: MapRenderMode = 'detailed'

// useMapStyle returns the saved default map style, whether an external map
// pack is installed, and whether POI pins are set to ignore the depth fade.
//
// The fallback is the point: a pack lives in the user's game folder and can be
// deleted, moved, or renamed between launches, so a stored 'external' style can
// outlive the files it names. Rather than rendering an empty canvas, this
// resolves to the built-in default whenever no pack is present.
//
// All values are fetched once per mount. None change without a settings visit
// or a file-system change, so polling would buy nothing.
export function useMapStyle(): {
  // style is the resolved default, safe to use directly.
  style: MapRenderMode
  // pack is null until the check completes, then reports what was found.
  pack: ExternalMapStatus | null
  // poiIgnoreZFade mirrors preferences.map_poi_ignore_zfade — false (the
  // default) until the config is loaded.
  poiIgnoreZFade: boolean
  // ready is false until both answers are in. Surfaces that pick an initial
  // mode wait for it, or they would mount in the wrong style and switch under
  // the user — which reads as a flicker and costs a wasted geometry fetch.
  ready: boolean
} {
  const [style, setStyle] = useState<MapRenderMode>(DEFAULT_STYLE)
  const [pack, setPack] = useState<ExternalMapStatus | null>(null)
  const [poiIgnoreZFade, setPoiIgnoreZFade] = useState(false)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      getConfig().catch(() => null),
      getExternalMapStatus().catch(() => ({ available: false }) as ExternalMapStatus),
    ]).then(([cfg, status]) => {
      if (cancelled) return
      setPack(status)
      const saved = cfg?.preferences.map_style
      if (saved === 'external' && !status.available) {
        setStyle(DEFAULT_STYLE)
      } else if (saved) {
        setStyle(saved)
      }
      setPoiIgnoreZFade(cfg?.preferences.map_poi_ignore_zfade ?? false)
      setReady(true)
    })
    return () => { cancelled = true }
  }, [])

  return { style, pack, poiIgnoreZFade, ready }
}

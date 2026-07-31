import { useEffect, useState } from 'react'
import { getConfig, getExternalMapStatus } from '../services/api'
import type { ExternalMapStatus, MapRenderMode } from '../types/map'

// DEFAULT_STYLE matches the backend's default. Detailed carries the most
// information and wins in the large majority of zones; outline omits anything
// that is not a wall, which loses features like Oasis of Marr's lake.
const DEFAULT_STYLE: MapRenderMode = 'detailed'

// useMapStyle returns the saved default map style, plus whether an external map
// pack is installed.
//
// The fallback is the point: a pack lives in the user's game folder and can be
// deleted, moved, or renamed between launches, so a stored 'external' style can
// outlive the files it names. Rather than rendering an empty canvas, this
// resolves to the built-in default whenever no pack is present.
//
// Both values are fetched once per mount. Neither changes without a settings
// visit or a file-system change, so polling would buy nothing.
export function useMapStyle(): {
  // style is the resolved default, safe to use directly.
  style: MapRenderMode
  // pack is null until the check completes, then reports what was found.
  pack: ExternalMapStatus | null
  // ready is false until both answers are in. Surfaces that pick an initial
  // mode wait for it, or they would mount in the wrong style and switch under
  // the user — which reads as a flicker and costs a wasted geometry fetch.
  ready: boolean
} {
  const [style, setStyle] = useState<MapRenderMode>(DEFAULT_STYLE)
  const [pack, setPack] = useState<ExternalMapStatus | null>(null)
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
      setReady(true)
    })
    return () => { cancelled = true }
  }, [])

  return { style, pack, ready }
}

import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { DEFAULT_DPS_CLASS_COLORS, type DPSClassColors } from '../types/config'

// useDPSClassColors returns the user's configured per-class DPS bar palette.
// Loads on mount and refreshes when the backend broadcasts config:updated
// (e.g. after the user saves the Settings → DPS Class Colors tab) so the
// overlay and dashboard panel pick up colour changes without reloading.
//
// Falls back to DEFAULT_DPS_CLASS_COLORS until the API responds so first
// paint still draws class-coloured bars.
export function useDPSClassColors(): DPSClassColors {
  const [palette, setPalette] = useState<DPSClassColors>(DEFAULT_DPS_CLASS_COLORS)

  useEffect(() => {
    getConfig()
      .then((c) => {
        if (c.dps_class_colors) setPalette(c.dps_class_colors)
      })
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((c) => {
        if (c.dps_class_colors) setPalette(c.dps_class_colors)
      })
      .catch(() => {})
  }, [])
  useWebSocket(handle)

  return palette
}

import { useCallback, useEffect } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { applyZoom } from './useZoom'

// useOverlayZoom applies a popout overlay window's own zoom factor, keyed by
// canonical overlay name (see lib/overlays.ts). Each overlay runs in its own
// BrowserWindow, so applyZoom() (the `window:set-zoom` IPC) scales just this
// window — independently of the main window's zoom_factor.
//
// Unlike the main window's useZoom (apply-once-then-stop), overlays refresh on
// every `config:updated` WebSocket event so the Settings sliders preview live
// across overlays without a reload — the same approach as useTimerAppearance.
export function useOverlayZoom(overlayKey: string): void {
  const apply = useCallback(() => {
    getConfig()
      .then((c) => applyZoom(c.preferences.overlay_zoom_factors?.[overlayKey] ?? 1))
      .catch(() => {})
  }, [overlayKey])

  useEffect(() => {
    apply()
  }, [apply])

  const handle = useCallback(
    (msg: WsMessage) => {
      if (msg.type !== WSEvent.ConfigUpdated) return
      apply()
    },
    [apply],
  )
  useWebSocket(handle)
}

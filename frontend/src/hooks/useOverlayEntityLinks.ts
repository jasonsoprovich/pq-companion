import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

/**
 * Whether overlay windows should render entity rows (NPC overlay loot items,
 * castable spells) as clickable links into the main database explorer. Polls on
 * mount and refreshes on the `config:updated` WebSocket event so the Settings
 * toggle applies to open overlays without a reload. Defaults to enabled
 * (missing value is treated as on), matching the backend default.
 */
export function useOverlayEntityLinks(): boolean {
  const [enabled, setEnabled] = useState(true)

  useEffect(() => {
    getConfig()
      .then((c) => setEnabled(c.preferences?.overlay_entity_links_enabled !== false))
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((c) => setEnabled(c.preferences?.overlay_entity_links_enabled !== false))
      .catch(() => {})
  }, [])
  useWebSocket(handle)

  return enabled
}

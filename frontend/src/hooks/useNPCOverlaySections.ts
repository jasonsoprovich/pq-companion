import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import {
  DEFAULT_NPC_OVERLAY_SECTIONS,
  type NPCOverlaySections,
} from '../types/config'

export type NPCOverlaySurface = 'dashboard' | 'popout'

/**
 * Returns the user's currently-configured NPC overlay section visibility
 * for the given surface. Reads on mount and re-fetches on the
 * `config:updated` WebSocket event so toggling in Settings updates both
 * the embedded dashboard panel and the floating popout window live.
 *
 * Falls back to the all-visible default on initial load and on any error
 * so the overlay keeps working if the config endpoint is briefly
 * unavailable.
 */
export function useNPCOverlaySections(
  surface: NPCOverlaySurface,
): NPCOverlaySections {
  const [sections, setSections] = useState<NPCOverlaySections>(
    DEFAULT_NPC_OVERLAY_SECTIONS,
  )

  const read = useCallback(() => {
    getConfig()
      .then((c) => {
        const value =
          surface === 'dashboard'
            ? c.preferences?.npc_overlay_dashboard_sections
            : c.preferences?.npc_overlay_popout_sections
        if (value) setSections(value)
      })
      .catch(() => {})
  }, [surface])

  useEffect(() => {
    read()
  }, [read])

  const handle = useCallback(
    (msg: WsMessage) => {
      if (msg.type !== WSEvent.ConfigUpdated) return
      read()
    },
    [read],
  )
  useWebSocket(handle)

  return sections
}

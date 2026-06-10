import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { CHChainSettings } from '../types/config'

/**
 * Returns the user's current CH-chain settings. Reads on mount and
 * re-fetches on the `config:updated` WebSocket event, so enabling the
 * secondary chain in Settings makes the Main/Ramp switch appear on the
 * CH Chain overlay and metronome live, without reopening them.
 *
 * Returns null until the first successful load so callers can tell
 * "not loaded yet" from "secondary disabled".
 */
export function useCHChainConfig(): CHChainSettings | null {
  const [settings, setSettings] = useState<CHChainSettings | null>(null)

  const read = useCallback(() => {
    getConfig()
      .then((c) => {
        if (c.ch_chain) setSettings(c.ch_chain)
      })
      .catch(() => {})
  }, [])

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

  return settings
}

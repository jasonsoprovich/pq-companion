import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { navFlags } from '../lib/sidebarNav'

export interface SidebarPrefs {
  hidden: string[]
  order: string[]
  flags: Record<string, boolean>
}

/**
 * Returns the user's sidebar hide/order preferences plus the flag map that
 * gates dev-preview tabs, read on mount and re-fetched on the `config:updated`
 * WebSocket event so toggling tabs (or feature flags) in Settings updates the
 * live sidebar immediately. Defaults to "nothing hidden, default order".
 */
export function useSidebarPrefs(): SidebarPrefs {
  const [prefs, setPrefs] = useState<SidebarPrefs>({ hidden: [], order: [], flags: navFlags() })

  const read = useCallback(() => {
    getConfig()
      .then((c) => {
        setPrefs({
          hidden: c.preferences?.sidebar_hidden ?? [],
          order: c.preferences?.sidebar_order ?? [],
          flags: navFlags(c.preferences),
        })
      })
      .catch(() => {})
  }, [])

  useEffect(() => { read() }, [read])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigUpdated) read()
  }, [read])
  useWebSocket(handle)

  return prefs
}

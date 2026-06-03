import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export interface SidebarPrefs {
  hidden: string[]
  order: string[]
}

/**
 * Returns the user's sidebar hide/order preferences, read on mount and
 * re-fetched on the `config:updated` WebSocket event so toggling tabs in
 * Settings updates the live sidebar immediately. Defaults to "nothing hidden,
 * default order".
 */
export function useSidebarPrefs(): SidebarPrefs {
  const [prefs, setPrefs] = useState<SidebarPrefs>({ hidden: [], order: [] })

  const read = useCallback(() => {
    getConfig()
      .then((c) => {
        setPrefs({
          hidden: c.preferences?.sidebar_hidden ?? [],
          order: c.preferences?.sidebar_order ?? [],
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

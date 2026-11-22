import { useCallback, useEffect, useRef, useState } from 'react'
import { getZealSettings } from '../services/api'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { WsMessage } from './useWebSocket'
import type { ZealSettings } from '../types/zeal'

/**
 * The Zeal settings checklist for the configured EQ folder, or for `path`
 * when given (the setup wizard, before config is saved). Refetches when the
 * backend reports a change (zeal.ini rewritten, feature first seen on the
 * pipe, pipe connect/disconnect) or config changes.
 */
export function useZealSettings(path?: string): {
  settings: ZealSettings | null
  refresh: () => void
} {
  const [settings, setSettings] = useState<ZealSettings | null>(null)
  const seq = useRef(0)

  const refresh = useCallback(() => {
    const n = ++seq.current
    getZealSettings(path?.trim() || undefined)
      .then((s) => {
        if (n === seq.current) setSettings(s)
      })
      .catch(() => null)
  }, [path])

  useEffect(() => {
    refresh()
  }, [refresh])

  useWebSocket((msg: WsMessage) => {
    if (
      msg.type === WSEvent.ZealSettings ||
      (!path && msg.type === WSEvent.ConfigUpdated)
    ) {
      refresh()
    }
  })

  return { settings, refresh }
}

/** True when the install's Zeal supports checklist feature `id`. */
export function zealSupports(
  settings: ZealSettings | null,
  id: string,
): boolean {
  return !!settings?.capabilities.some((c) => c.id === id)
}

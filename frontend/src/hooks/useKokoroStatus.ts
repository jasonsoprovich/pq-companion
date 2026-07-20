import { useCallback, useEffect, useState } from 'react'

import { getKokoroStatus } from '../services/api'
import type { KokoroStatus } from '../lib/kokoro'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'

export interface UseKokoroStatusResult {
  status: KokoroStatus | null
  /**
   * Re-fetches status immediately. Exposed for callers that just triggered a
   * synthesis directly (e.g. Settings "Test voice") — a config:updated event
   * won't fire for that.
   */
  refresh: () => void
}

/**
 * Fetches the backend's Kokoro (local TTS) install status and keeps it fresh
 * when the config changes. status is null until the first fetch resolves.
 * Mirrors usePiperStatus for the second local-TTS provider.
 */
export function useKokoroStatus(): UseKokoroStatusResult {
  const [status, setStatus] = useState<KokoroStatus | null>(null)

  const refresh = useCallback(() => {
    getKokoroStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const onMessage = useCallback(
    (msg: WsMessage) => {
      if (msg.type === 'config:updated') refresh()
    },
    [refresh],
  )
  useWebSocket(onMessage)

  return { status, refresh }
}

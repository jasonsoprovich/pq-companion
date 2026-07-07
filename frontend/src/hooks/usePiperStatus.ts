import { useCallback, useEffect, useState } from 'react'

import { getPiperStatus } from '../services/api'
import { PIPER_VOICE_ID, type PiperStatus } from '../lib/piper'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'

/**
 * Fetches the backend's Piper (local TTS) install status and keeps it fresh
 * when the config changes. Returns null until the first fetch resolves.
 *
 * The status drives the Settings card and gates whether the Piper voice is
 * offered in voice dropdowns — see useTTSVoices. We refetch on `config:updated`
 * so editing the exe/model path (or toggling Piper) re-detects without a
 * manual refresh.
 */
export function usePiperStatus(): PiperStatus | null {
  const [status, setStatus] = useState<PiperStatus | null>(null)

  const refresh = useCallback(() => {
    getPiperStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  // Re-detect whenever the config changes (path edits, enable toggle) so the
  // card and voice list reflect the just-saved paths without a manual refresh.
  const onMessage = useCallback(
    (msg: WsMessage) => {
      if (msg.type === 'config:updated') refresh()
    },
    [refresh],
  )
  useWebSocket(onMessage)

  return status
}

/**
 * Returns the Web Speech voice names with the configured Piper voice appended
 * (as PIPER_VOICE_ID) when Piper is enabled and ready. Drop-in replacement for
 * useVoices() in the alert/trigger voice dropdowns.
 */
export function useTTSVoices(baseVoices: string[]): string[] {
  const piper = usePiperStatus()
  if (piper?.enabled && piper.ready) {
    return [PIPER_VOICE_ID, ...baseVoices]
  }
  return baseVoices
}

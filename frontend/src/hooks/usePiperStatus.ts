import { useCallback, useEffect, useState } from 'react'

import { getPiperStatus } from '../services/api'
import { PIPER_VOICE_ID, type PiperStatus } from '../lib/piper'
import { KOKORO_VOICE_ID } from '../lib/kokoro'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'
import { useKokoroStatus } from './useKokoroStatus'

export interface UsePiperStatusResult {
  status: PiperStatus | null
  /**
   * Re-fetches status immediately. Exposed for callers that just triggered a
   * synthesis directly (e.g. Settings "Test voice") — a config:updated event
   * won't fire for that, so without an explicit refresh the warm-worker
   * health dot wouldn't update until some later, unrelated config change.
   */
  refresh: () => void
}

/**
 * Fetches the backend's Piper (local TTS) install status and keeps it fresh
 * when the config changes. status is null until the first fetch resolves.
 *
 * The status drives the Settings card and gates whether the Piper voice is
 * offered in voice dropdowns — see useTTSVoices. We refetch on `config:updated`
 * so editing the exe/model path (or toggling Piper) re-detects without a
 * manual refresh.
 */
export function usePiperStatus(): UsePiperStatusResult {
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

  return { status, refresh }
}

/**
 * Returns the Web Speech voice names with the configured local-TTS voices
 * (Piper, Kokoro) appended when each is enabled and ready. Drop-in
 * replacement for useVoices() in the alert/trigger voice dropdowns. Kept in
 * this file (rather than split per-provider) because every call site already
 * imports it from here.
 */
export function useTTSVoices(baseVoices: string[]): string[] {
  const { status: piper } = usePiperStatus()
  const { status: kokoro } = useKokoroStatus()
  const localVoices: string[] = []
  if (piper?.enabled && piper.ready) localVoices.push(PIPER_VOICE_ID)
  if (kokoro?.enabled && kokoro.ready) localVoices.push(KOKORO_VOICE_ID)
  return [...localVoices, ...baseVoices]
}

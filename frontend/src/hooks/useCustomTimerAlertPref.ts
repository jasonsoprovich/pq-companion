import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { TimerAlertPref } from '../types/config'

/**
 * Returns the user's global Custom-timer alert preference, polled on mount and
 * refreshed via the `config:updated` WebSocket event so the Custom Timers
 * overlay's quick-add form reflects Settings changes without a reload.
 * `undefined` until loaded (and on error) — the form treats that as "no
 * default alert", same as a disabled pref.
 */
export function useCustomTimerAlertPref(): TimerAlertPref | undefined {
  const [pref, setPref] = useState<TimerAlertPref | undefined>(undefined)

  useEffect(() => {
    getConfig()
      .then((c) => setPref(c.preferences?.custom_timer_alert))
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((c) => setPref(c.preferences?.custom_timer_alert))
      .catch(() => {})
  }, [])
  useWebSocket(handle)

  return pref
}

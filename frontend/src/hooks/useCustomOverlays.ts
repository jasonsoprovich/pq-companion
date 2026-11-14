/**
 * useCustomOverlays — live Custom Overlay state shared by the popout windows,
 * the dashboard panels and (later) the hub editor. Seeds from the REST
 * snapshot, then follows customoverlays:state broadcasts. Refetches whenever
 * the socket (re)opens so a backend restart doesn't leave stale contents.
 */
import { useCallback, useEffect, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { customOverlayAction, getCustomOverlays } from '../services/api'
import type {
  CustomOverlay,
  CustomOverlayAction,
  CustomOverlaysState,
} from '../types/customOverlay'

export interface CustomOverlaysHook {
  overlays: CustomOverlay[]
  // Milliseconds to add to Date.now() to get server time. Countdown ends and
  // last-change times are server/log stamped, so a skewed local clock must
  // not shift them.
  skewMs: number
  loaded: boolean
  error: string
  act: (id: string, body: CustomOverlayAction) => Promise<void>
}

export function useCustomOverlays(): CustomOverlaysHook {
  const [overlays, setOverlays] = useState<CustomOverlay[]>([])
  const [skewMs, setSkewMs] = useState(0)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')

  const apply = useCallback((s: CustomOverlaysState) => {
    setOverlays(s.overlays ?? [])
    if (s.now) setSkewMs(s.now - Date.now())
    setLoaded(true)
  }, [])

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.CustomOverlays) apply(msg.data as CustomOverlaysState)
  })

  useEffect(() => {
    if (ready !== 'open' && loaded) return
    getCustomOverlays()
      .then((s) => { apply(s); setError('') })
      .catch((e: Error) => setError(e.message))
  }, [ready]) // eslint-disable-line react-hooks/exhaustive-deps

  const act = useCallback(async (id: string, body: CustomOverlayAction) => {
    try {
      apply(await customOverlayAction(id, body))
      setError('')
    } catch (e) {
      setError((e as Error).message)
    }
  }, [apply])

  return { overlays, skewMs, loaded, error, act }
}

// useNowMs ticks while `active`, returning server-adjusted unix ms. Ticks
// every `everyMs` (1s for countdowns); idle costs nothing.
export function useNowMs(active: boolean, skewMs: number, everyMs = 1000): number {
  const [now, setNow] = useState(() => Date.now() + skewMs)
  useEffect(() => {
    setNow(Date.now() + skewMs)
    if (!active) return
    const t = setInterval(() => setNow(Date.now() + skewMs), everyMs)
    return () => clearInterval(t)
  }, [active, skewMs, everyMs])
  return now
}

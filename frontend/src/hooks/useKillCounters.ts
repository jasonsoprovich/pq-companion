/**
 * useKillCounters — live kill-counter state shared by the Kill Tracker page,
 * the dashboard panel and the popout overlay. Seeds from the REST snapshot,
 * then follows killcounters:state broadcasts. Refetches whenever the socket
 * (re)opens so a backend restart doesn't leave a stale count on screen.
 */
import { useCallback, useEffect, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getKillCounters, killCounterAction } from '../services/api'
import type { KillCounter, KillCounterAction, KillCountersState } from '../types/kills'

export interface KillCountersHook {
  counters: KillCounter[]
  // Seconds to add to Date.now()/1000 to get server time — clock math uses
  // backend-stamped (log) times, so a skewed local clock must not shift them.
  skewSecs: number
  killSource: 'log' | 'zeal'
  loaded: boolean
  error: string
  act: (id: string, body: KillCounterAction) => Promise<void>
  apply: (state: KillCountersState) => void
}

export function useKillCounters(): KillCountersHook {
  const [counters, setCounters] = useState<KillCounter[]>([])
  const [skewSecs, setSkewSecs] = useState(0)
  const [killSource, setKillSource] = useState<'log' | 'zeal'>('log')
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')

  const apply = useCallback((s: KillCountersState) => {
    setCounters(s.counters ?? [])
    if (s.now) setSkewSecs(s.now - Date.now() / 1000)
    setKillSource(s.kill_source ?? 'log')
    setLoaded(true)
  }, [])

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.KillCounters) apply(msg.data as KillCountersState)
  })

  useEffect(() => {
    if (ready !== 'open' && loaded) return
    getKillCounters()
      .then((s) => { apply(s); setError('') })
      .catch((e: Error) => setError(e.message))
  }, [ready]) // eslint-disable-line react-hooks/exhaustive-deps

  const act = useCallback(async (id: string, body: KillCounterAction) => {
    try {
      apply(await killCounterAction(id, body))
      setError('')
    } catch (e) {
      setError((e as Error).message)
    }
  }, [apply])

  return { counters, skewSecs, killSource, loaded, error, act, apply }
}

// useNowSecs ticks once a second while `active`, returning server-adjusted
// unix seconds. Idle (no running clock) costs nothing.
export function useNowSecs(active: boolean, skewSecs: number): number {
  const [now, setNow] = useState(() => Date.now() / 1000 + skewSecs)
  useEffect(() => {
    setNow(Date.now() / 1000 + skewSecs)
    if (!active) return
    const t = setInterval(() => setNow(Date.now() / 1000 + skewSecs), 1000)
    return () => clearInterval(t)
  }, [active, skewSecs])
  return now
}

// nextMilestone returns the first milestone above the current count.
export function nextMilestone(c: KillCounter): { at: number; label: string } | null {
  for (const m of c.def.milestones ?? []) {
    if (m.at > c.count) return m
  }
  return null
}

// fmtClock renders seconds as h:mm:ss / m:ss.
export function fmtClock(secs: number): string {
  const s = Math.max(0, Math.floor(secs))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

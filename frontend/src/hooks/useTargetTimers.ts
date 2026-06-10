import { useCallback, useEffect, useMemo, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getTimerState } from '../services/api'
import { normalizeTargetName } from '../lib/targetName'
import type { ActiveTimer, TimerState } from '../types/timer'

/**
 * Active spell timers currently ticking on a given target name, sorted
 * soonest-expiring first.
 *
 * Unlike the buff/detrim overlays, this deliberately does NOT apply the
 * display threshold — the NPC overlay's Timers tab is meant to surface every
 * timer on the target, including ones still hidden from the main overlays
 * because their remaining time is above their reveal threshold. The backend
 * tracks all timers regardless of threshold (it's a frontend-only display
 * filter), so the full state is always available here.
 *
 * Names are normalized the same way the engine keys timers (article-stripped,
 * lowercased) so log-driven NPC names line up with the engine's stored target.
 */
export function useTargetTimers(targetName: string | undefined): ActiveTimer[] {
  const [state, setState] = useState<TimerState | null>(null)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handle = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayTimers) setState(msg.data as TimerState)
  }, [])
  useWebSocket(handle)

  return useMemo(() => {
    if (!targetName) return []
    const want = normalizeTargetName(targetName)
    return (state?.timers ?? [])
      .filter((t) => t.target_name && normalizeTargetName(t.target_name) === want)
      .sort((a, b) => a.remaining_seconds - b.remaining_seconds)
  }, [state, targetName])
}

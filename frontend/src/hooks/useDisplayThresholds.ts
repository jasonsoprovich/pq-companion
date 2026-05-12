import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { ActiveTimer, TimerCategory } from '../types/timer'

export interface DisplayThresholds {
  /** Buff overlay default — 0 means always show. */
  buff: number
  /** Detrimental overlay default — applies to debuff / dot / mez / stun. 0 = always show. */
  detrim: number
}

const ZERO: DisplayThresholds = { buff: 0, detrim: 0 }

/**
 * Returns the user's currently-configured global display thresholds for
 * spell timers. Polls on mount and updates instantly via the
 * `config:updated` WebSocket event so changes in Settings are reflected
 * across overlays without a page reload.
 *
 * Both values default to 0 (always show) on initial load and on any error
 * — the overlays should keep working if the config endpoint is briefly
 * unavailable.
 */
export function useDisplayThresholds(): DisplayThresholds {
  const [t, setT] = useState<DisplayThresholds>(ZERO)

  useEffect(() => {
    getConfig()
      .then((c) =>
        setT({
          buff: c.spell_timer?.buff_display_threshold_secs ?? 0,
          detrim: c.spell_timer?.detrim_display_threshold_secs ?? 0,
        }),
      )
      .catch(() => {})
  }, [])

  // Refresh when the user saves Settings — the API broadcasts a
  // "config:updated" event after a successful PUT /api/config so
  // overlays in any window stay in sync.
  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((c) =>
        setT({
          buff: c.spell_timer?.buff_display_threshold_secs ?? 0,
          detrim: c.spell_timer?.detrim_display_threshold_secs ?? 0,
        }),
      )
      .catch(() => {})
  }, [])
  useWebSocket(handle)

  return t
}

/**
 * Resolves the effective threshold for a single timer:
 *
 *   1. If the timer carries a per-trigger override (display_threshold_secs > 0),
 *      that wins.
 *   2. Otherwise the global default for the timer's category is used.
 *
 * Returns 0 when nothing should be hidden.
 */
function resolveThreshold(timer: ActiveTimer, defaults: DisplayThresholds): number {
  if (timer.display_threshold_secs > 0) return timer.display_threshold_secs
  return categoryDefault(timer.category, defaults)
}

function categoryDefault(category: TimerCategory, d: DisplayThresholds): number {
  if (category === 'buff') return d.buff
  return d.detrim
}

/**
 * True when the timer should be visible given the user's threshold config.
 * 0 thresholds always pass; otherwise we hide rows whose remaining time is
 * still above the threshold (i.e. they "haven't crossed yet").
 */
export function passesThreshold(timer: ActiveTimer, defaults: DisplayThresholds): boolean {
  const threshold = resolveThreshold(timer, defaults)
  if (threshold <= 0) return true
  return timer.remaining_seconds <= threshold
}

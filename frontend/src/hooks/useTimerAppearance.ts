import { useCallback, useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { SpellTimerSettings } from '../types/config'

/**
 * Resolved, render-ready timer-bar appearance values for the overlay panels.
 * Bar fill is expressed as an opacity (0 = transparent / text-only) so the
 * panels can apply it directly; font sizes and row padding are concrete px.
 */
export interface TimerAppearance {
  /** Bar fill opacity (0 = none/transparent, ~0.15 faded, ~0.55 solid). */
  fillOpacity: number
  /** Spell-name font size in px. */
  nameFontSize: number
  /** Countdown font size in px. */
  timeFontSize: number
  /** Vertical row padding in px. */
  rowPadding: number
}

// Built-in defaults, matching the panels' historical hardcoded values.
const DEFAULTS: TimerAppearance = {
  fillOpacity: 0.15,
  nameFontSize: 12,
  timeFontSize: 11,
  rowPadding: 3,
}

function resolve(st: SpellTimerSettings | undefined): TimerAppearance {
  if (!st) return DEFAULTS
  const fillOpacity =
    st.timer_bar_fill === 'none'
      ? 0
      : st.timer_bar_fill === 'solid'
        ? 0.55
        : DEFAULTS.fillOpacity
  return {
    fillOpacity,
    // 0/absent → default; these never legitimately want a 0 value.
    nameFontSize: st.timer_name_font_size || DEFAULTS.nameFontSize,
    timeFontSize: st.timer_time_font_size || DEFAULTS.timeFontSize,
    rowPadding: st.timer_row_padding || DEFAULTS.rowPadding,
  }
}

/**
 * Returns the user's timer overlay appearance settings, resolved to concrete
 * render values. Polls on mount and refreshes on the `config:updated`
 * WebSocket event so Settings changes apply across overlays without a reload.
 * Falls back to built-in defaults on initial load or any error.
 */
export function useTimerAppearance(): TimerAppearance {
  const [a, setA] = useState<TimerAppearance>(DEFAULTS)

  useEffect(() => {
    getConfig()
      .then((c) => setA(resolve(c.spell_timer)))
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((c) => setA(resolve(c.spell_timer)))
      .catch(() => {})
  }, [])
  useWebSocket(handle)

  return a
}

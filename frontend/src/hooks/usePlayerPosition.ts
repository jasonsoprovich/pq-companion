import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'

// PlayerPosition mirrors backend/internal/playerpos.State. Coordinates are
// already in map space — the backend applies the same negation as the geometry
// pipeline, so exactly one place in the codebase knows that transform.
export interface PlayerPosition {
  zone: string
  x: number
  y: number
  z: number
  // heading is EQ's 0-512 counter-clockwise value, 0 = north.
  heading: number
}

// STALE_MS is how long a position stays trusted without an update.
//
// The backend heartbeats every 2s even when the player is standing perfectly
// still, so silence past this means Zeal died, the game closed, or the pipe
// stalled — not that nobody moved. Comfortably above the heartbeat so ordinary
// jitter never blanks a live arrow.
//
// This is the app's only defence against a stalled pipe: the pipe-connected gate
// has no staleness check of its own (project_npc_overlay_pipe_stall_no_fallback),
// and a frozen arrow is worse than no arrow because it still looks authoritative.
const STALE_MS = 6000

// usePlayerPosition returns the player's live map position, or null when there
// is no fresh one. Null covers every "we don't know" case — Zeal not running,
// not on Windows, pipe stalled, zone unresolved — so callers have one thing to
// check rather than a connection state plus a position.
export function usePlayerPosition(): PlayerPosition | null {
  const [pos, setPos] = useState<PlayerPosition | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = (): void => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
  }

  const onMessage = useCallback((msg: WsMessage) => {
    if (msg.type !== 'player:position') return
    // The backend sends a null payload on pipe disconnect, which is an explicit
    // "gone" rather than a timeout — honour it immediately.
    const p = msg.data as PlayerPosition | null
    clearTimer()
    if (!p || !p.zone) {
      setPos(null)
      return
    }
    setPos(p)
    timer.current = setTimeout(() => setPos(null), STALE_MS)
  }, [])

  useWebSocket(onMessage)
  useEffect(() => clearTimer, [])

  return pos
}

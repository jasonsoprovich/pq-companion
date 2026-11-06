import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export interface TargetPosition {
  zone: string
  name: string
  // type is Zeal's target type: 0 PC, 1 NPC, 2 NPC corpse, 3 PC corpse.
  type: number
  // Map space, like the player's position.
  x: number
  y: number
  z: number
}

// STALE_MS matches usePlayerPosition: the backend heartbeats every 2 s.
const STALE_MS = 6000

// useTargetPosition returns your Zeal target and where it stands, or null
// with no target, none within the 250 units Zeal reports positions for, or
// no fresh frame.
export function useTargetPosition(): TargetPosition | null {
  const [target, setTarget] = useState<TargetPosition | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = (): void => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
  }

  const onMessage = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.PlayerTarget) return
    const ts = msg.data as TargetPosition | null
    clearTimer()
    if (!ts || !ts.name) {
      setTarget(null)
      return
    }
    setTarget(ts)
    timer.current = setTimeout(() => setTarget(null), STALE_MS)
  }, [])

  useWebSocket(onMessage)
  useEffect(() => clearTimer, [])

  return target
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

// GroupMemberPosition mirrors backend/internal/playerpos.GroupMemberState.
// Coordinates are already in map space (same negation as the self arrow).
export interface GroupMemberPosition {
  name: string
  x: number
  y: number
  z: number
  // heading is EQ's 0-512 counter-clockwise value, 0 = north.
  heading: number
}

interface GroupState {
  zone: string
  members: GroupMemberPosition[]
}

// STALE_MS matches usePlayerPosition — the backend heartbeats the group frame
// on the same cadence, so silence past this means the pipe stalled or Zeal
// died, and lingering groupmate arrows are worse than none.
const STALE_MS = 6000

// useGroupPositions returns the in-zone groupmates' live map positions, or an
// empty array when there is no fresh set (no pipe, not grouped, all groupmates
// in another zone, pipe stalled). The backend only ever sends members it
// resolved in the local player's zone, so no zone filtering is needed here.
export function useGroupPositions(): GroupMemberPosition[] {
  const [members, setMembers] = useState<GroupMemberPosition[]>([])
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = (): void => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
  }

  const onMessage = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.PlayerGroupPositions) return
    const gs = msg.data as GroupState | null
    clearTimer()
    const next = gs?.members ?? []
    setMembers(next)
    if (next.length > 0) {
      timer.current = setTimeout(() => setMembers([]), STALE_MS)
    }
  }, [])

  useWebSocket(onMessage)
  useEffect(() => clearTimer, [])

  return members
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export interface RaidMemberPosition {
  name: string
  // class is Zeal's 1-indexed class id; group the raid group, 0 ungrouped.
  class: number
  group: number
  x: number
  y: number
  z: number
  // heading is EQ's 0-512 counter-clockwise value, 0 = north.
  heading: number
}

interface RaidState {
  zone: string
  members: RaidMemberPosition[]
}

// STALE_MS matches useGroupPositions: the backend heartbeats every 2 s.
const STALE_MS = 6000

// useRaidPositions returns the in-zone raiders' live map positions (not
// including you), or an empty array when there is no fresh set: no pipe,
// not in a raid, or the pipe stalled.
export function useRaidPositions(): RaidMemberPosition[] {
  const [members, setMembers] = useState<RaidMemberPosition[]>([])
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = (): void => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
  }

  const onMessage = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.PlayerRaidPositions) return
    const rs = msg.data as RaidState | null
    clearTimer()
    const next = rs?.members ?? []
    setMembers(next)
    if (next.length > 0) {
      timer.current = setTimeout(() => setMembers([]), STALE_MS)
    }
  }, [])

  useWebSocket(onMessage)
  useEffect(() => clearTimer, [])

  return members
}

/**
 * useAnimatedChainList — smooths the CH Chain overlay's list when a callout
 * lands and drops out of the backend's timer feed. Without this, removing an
 * entry from the `chain` array collapses the gap instantly and every row
 * below jumps up in one frame. This hook keeps a just-removed entry mounted
 * for a short exit animation (see ChainRow's `exiting` style), so the row
 * collapses smoothly and the rows below reflow with it via normal CSS layout
 * rather than snapping.
 */
import { useRef, useState } from 'react'
import type { ActiveTimer } from '../types/timer'

const EXIT_DURATION_MS = 220

interface ExitingEntry {
  timer: ActiveTimer
  index: number
}

export function useAnimatedChainList(chain: ActiveTimer[]): {
  displayList: ActiveTimer[]
  exitingIds: Set<string>
} {
  const [exiting, setExiting] = useState<Map<string, ExitingEntry>>(new Map())
  const prevChainRef = useRef<ActiveTimer[]>(chain)
  const timersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  // Detect removed entries synchronously during render, not in a useEffect.
  // An effect fires after React has already committed a render with the row
  // gone, so the row unmounts and a later render remounts it fresh — CSS
  // transitions can't animate a fresh mount, so the collapse would never be
  // visible. Doing this inline (React's documented "adjust state during
  // render" pattern, guarded by the prevChainRef identity check below so it
  // terminates) keeps the row mounted on the very same render where it drops
  // out of `chain`, so the browser has a "before" style to transition from.
  if (chain !== prevChainRef.current) {
    const currentIds = new Set(chain.map((t) => t.id))
    const removed = prevChainRef.current.filter((t) => !currentIds.has(t.id))
    if (removed.length > 0) {
      const removedIndex = new Map(prevChainRef.current.map((t, i) => [t.id, i]))
      setExiting((prev) => {
        const next = new Map(prev)
        for (const t of removed) {
          next.set(t.id, { timer: t, index: removedIndex.get(t.id) ?? next.size })
        }
        return next
      })
      for (const t of removed) {
        if (timersRef.current.has(t.id)) continue
        const handle = setTimeout(() => {
          timersRef.current.delete(t.id)
          setExiting((prev) => {
            if (!prev.has(t.id)) return prev
            const next = new Map(prev)
            next.delete(t.id)
            return next
          })
        }, EXIT_DURATION_MS)
        timersRef.current.set(t.id, handle)
      }
    }
    prevChainRef.current = chain
  }

  const displayList = [...chain]
  for (const { timer, index } of exiting.values()) {
    displayList.splice(Math.min(index, displayList.length), 0, timer)
  }

  return { displayList, exitingIds: new Set(exiting.keys()) }
}

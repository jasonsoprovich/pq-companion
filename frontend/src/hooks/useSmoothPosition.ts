import { useEffect, useRef, useState } from 'react'
import type { PlayerPosition } from './usePlayerPosition'

// Zeal sends a position about every 100 ms, and drawn as it arrives the
// arrow steps along ten times a second while the game's own glides. Easing
// from one sample to the next over the gap between them costs one gap of
// lag (~100 ms) and reads as smooth.
//
// SNAP_GAP_MS and SNAP_DIST are when to jump instead: after a pause (the
// heartbeat while standing still), or a move too far to have been walked
// (a gate, a port, a summon).
const SNAP_GAP_MS = 400
const SNAP_DIST = 150
const MAX_EASE_MS = 250

// useSmoothPosition eases the live position between Zeal's samples, for
// drawing your arrow. Logic that only needs where you are (rings near you,
// the depth band) should keep the raw position, which changes less often.
export function useSmoothPosition(target: PlayerPosition | null): PlayerPosition | null {
  const [shown, setShown] = useState(target)
  const shownRef = useRef(target)
  const lastAt = useRef(0)
  const frame = useRef(0)

  useEffect(() => {
    cancelAnimationFrame(frame.current)
    const now = performance.now()
    const gap = now - lastAt.current
    lastAt.current = now
    const from = shownRef.current
    const show = (p: PlayerPosition | null): void => {
      shownRef.current = p
      setShown(p)
    }
    if (
      !target || !from || from.zone !== target.zone || gap > SNAP_GAP_MS ||
      Math.hypot(target.x - from.x, target.y - from.y, target.z - from.z) > SNAP_DIST
    ) {
      show(target)
      return
    }
    if (from.x === target.x && from.y === target.y && from.z === target.z && from.heading === target.heading) {
      show(target)
      return
    }
    const dur = Math.min(gap, MAX_EASE_MS)
    // Turn the short way round: EQ headings wrap at 512.
    const turn = ((((target.heading - from.heading) % 512) + 768) % 512) - 256
    const step = (t0: number): void => {
      const t = Math.min(1, (t0 - now) / dur)
      show(t >= 1 ? target : {
        zone: target.zone,
        x: from.x + (target.x - from.x) * t,
        y: from.y + (target.y - from.y) * t,
        z: from.z + (target.z - from.z) * t,
        heading: (((from.heading + turn * t) % 512) + 512) % 512,
      })
      if (t < 1) frame.current = requestAnimationFrame(step)
    }
    frame.current = requestAnimationFrame(step)
  }, [target])

  useEffect(() => () => cancelAnimationFrame(frame.current), [])

  return shown
}

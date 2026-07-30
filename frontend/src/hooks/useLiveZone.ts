import { useEffect, useState } from 'react'
import { usePlayerPosition } from './usePlayerPosition'

// useLiveZone returns the zone the player is standing in, and whether the
// position behind it is current.
//
// The zone is *sticky*: once seen it is remembered even after the position goes
// away. That matters because zoning in EQ takes several seconds during which the
// pipe is silent, and a live map that blanked itself on every zone would spend
// those seconds showing nothing and then rebuild from scratch. Holding the last
// zone keeps the map on screen through the gap; `live` tells the caller whether
// to trust the arrow on it.
//
// Sticky also covers the ordinary case of closing EQ with the app still open:
// the map you were last on stays readable instead of reverting to an empty
// state.
export function useLiveZone(): { zone: string | null; live: boolean } {
  const pos = usePlayerPosition()
  const [lastZone, setLastZone] = useState<string | null>(null)

  useEffect(() => {
    if (pos?.zone) setLastZone(pos.zone)
  }, [pos?.zone])

  return { zone: pos?.zone ?? lastZone, live: pos !== null }
}

import { useEffect, useState } from 'react'
import { freshness, liveAgeMs, type Freshness, type LiveTTL } from '../lib/live'

export interface LiveStatus {
  freshness: Freshness
  ageMs: number
}

/**
 * useLiveValue reports how fresh a live value is from its "<field>_at"
 * stamp, re-rendering once a second while there is a stamp to age. A
 * missing stamp reads as 'gone'. See lib/live.ts.
 */
export function useLiveValue(at: string | undefined, ttl: LiveTTL): LiveStatus {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    setNow(Date.now())
    if (!at) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [at])
  const ageMs = liveAgeMs(at, now)
  return { freshness: freshness(ageMs, ttl), ageMs }
}

// Live-value convention (mirrors backend/internal/live): a value read from a
// live feed travels with "<field>_at", when the feed last confirmed it, and
// "<field>_source", which feed it came from. The backend re-sends an
// unchanged value at least every LIVE_HEARTBEAT_MS while the feed keeps
// arriving, so an old "_at" means the feed stopped, not that the value held.
//
// Past a field's staleMs a renderer dims the value and says how old it is;
// past goneMs it blanks it.

export const LIVE_HEARTBEAT_MS = 2000

export interface LiveTTL {
  staleMs: number
  goneMs: number
}

// LIVE_TTL holds the per-field TTLs. Keep staleMs well above the heartbeat.
export const LIVE_TTL = {
  // Zeal target HP%: labels arrive ~10 Hz while the pipe is healthy.
  targetHP: { staleMs: 6000, goneMs: 20000 },
  // Zeal pet HP%.
  petHP: { staleMs: 6000, goneMs: 20000 },
  // A `/pet health` reply is a one-off reading; it ages but stays shown.
  petHealthHP: { staleMs: 60000, goneMs: Infinity },
} satisfies Record<string, LiveTTL>

export type Freshness = 'fresh' | 'stale' | 'gone'

// STALE_OPACITY is how far a stale value is dimmed.
export const STALE_OPACITY = 0.45

export function liveAgeMs(at: string | undefined, now: number): number {
  if (!at) return Infinity
  const t = Date.parse(at)
  return Number.isNaN(t) ? Infinity : Math.max(0, now - t)
}

export function freshness(ageMs: number, ttl: LiveTTL): Freshness {
  if (ageMs >= ttl.goneMs) return 'gone'
  if (ageMs >= ttl.staleMs) return 'stale'
  return 'fresh'
}

// fmtLiveAge is the short "how old" text shown on a stale value.
export function fmtLiveAge(ageMs: number): string {
  const s = Math.floor(ageMs / 1000)
  if (s < 60) return `${s}s old`
  const m = Math.floor(s / 60)
  return m < 60 ? `${m}m old` : `${Math.floor(m / 60)}h old`
}

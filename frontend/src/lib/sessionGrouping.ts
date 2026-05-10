// SESSION_GAP_SECONDS is the inactivity gap between two consecutive fights
// that counts as a "session break" for visual grouping. 120 s matches
// EQLogParser's GroupTimeout — short enough to keep medding pauses inside
// a single session, long enough that distinct camps / pull groups separate
// cleanly.
export const SESSION_GAP_SECONDS = 120

// FightLike is the structural minimum needed to compute a session gap:
// any object with RFC3339 start_time and end_time strings. Lets the helper
// serve both StoredFight (combat history page) and FightSummary (combat
// log page) without coupling to either concrete type.
export interface FightLike {
  start_time: string
  end_time: string
}

// SessionRow is the renderer-friendly union: either a fight to render or a
// gap divider to render between two fights. The 'key' on gap rows is built
// from the surrounding fights so React reconciliation stays stable across
// re-renders.
export type SessionRow<T extends FightLike> =
  | { kind: 'fight'; fight: T }
  | { kind: 'gap'; gapSeconds: number; key: string }

// groupBySession walks a newest-first list of fights and inserts a 'gap'
// row before any fight whose start_time is more than SESSION_GAP_SECONDS
// after the previous fight's end_time. The very first row never gets a
// preceding gap (we don't know what came before this page).
//
// keyOf yields the React key portion contributed by each side of the gap.
// Most callers pass a row id; for FightSummary which lacks an id, the
// start_time string is unique enough.
export function groupBySession<T extends FightLike>(
  fights: T[],
  keyOf: (f: T) => string,
): SessionRow<T>[] {
  const out: SessionRow<T>[] = []
  for (let i = 0; i < fights.length; i++) {
    const f = fights[i]
    if (i > 0) {
      const prev = fights[i - 1]
      // Newest-first ordering: fights[i-1] is the NEWER one (rendered
      // above f); the gap is from when f ended to when the newer one
      // started.
      const prevStart = new Date(prev.start_time).getTime()
      const fEnd = new Date(f.end_time).getTime()
      const gap = (prevStart - fEnd) / 1000
      if (gap >= SESSION_GAP_SECONDS) {
        out.push({ kind: 'gap', gapSeconds: gap, key: `gap-${keyOf(prev)}-${keyOf(f)}` })
      }
    }
    out.push({ kind: 'fight', fight: f })
  }
  return out
}

// fmtSessionGap renders a session-break duration in human terms — minutes
// for short gaps, hours+minutes once we cross 60 minutes.
export function fmtSessionGap(secs: number): string {
  const totalMin = Math.round(secs / 60)
  if (totalMin < 60) return `${totalMin}m`
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

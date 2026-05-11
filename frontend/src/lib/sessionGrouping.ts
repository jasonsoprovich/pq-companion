// SESSION_GAP_SECONDS is the inactivity gap between two consecutive fights
// that counts as a "session break" for visual grouping in the LIVE combat
// log. 120 s matches EQLogParser's GroupTimeout — short enough to keep
// medding pauses inside a single session, long enough that distinct camps
// / pull groups separate cleanly.
export const SESSION_GAP_SECONDS = 120

// LONG_GAP_SECONDS is the inactivity threshold used by the event-based
// grouping (combat history) as a stand-in for camping-out / quitting the
// game. We can't observe the log line for /camp here — by the time fights
// are archived, the raw event is gone — so a 5-minute idle stretch acts
// as a reasonable proxy: long enough to bridge medding/buffing/looting,
// short enough that a relog or AFK reads as a break.
export const LONG_GAP_SECONDS = 300

// FightLike is the structural minimum needed to compute a session gap:
// any object with RFC3339 start_time and end_time strings. Lets the helper
// serve both StoredFight (combat history page) and FightSummary (combat
// log page) without coupling to either concrete type.
export interface FightLike {
  start_time: string
  end_time: string
}

// SessionBreakReason discriminates why two fights are visually separated.
// 'gap' covers timer-based + camp-proxy breaks; 'zone' and 'character'
// are event-driven and only fire from groupByEventSession.
export type SessionBreakReason = 'gap' | 'zone' | 'character'

// SessionRow is the renderer-friendly union: either a fight to render or a
// gap divider to render between two fights. For zone/character breaks the
// divider also carries from/to labels so the UI can show "Zone changed:
// X → Y" instead of an opaque timer.
export type SessionRow<T extends FightLike> =
  | { kind: 'fight'; fight: T }
  | {
      kind: 'gap'
      gapSeconds: number
      reason: SessionBreakReason
      from?: string
      to?: string
      key: string
    }

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
        out.push({ kind: 'gap', gapSeconds: gap, reason: 'gap', key: `gap-${keyOf(prev)}-${keyOf(f)}` })
      }
    }
    out.push({ kind: 'fight', fight: f })
  }
  return out
}

// FightContext is the side-channel data event-based grouping needs from
// each fight: which zone the player was in and which character was active.
// Decoupled from any concrete fight type so this module stays generic.
export interface FightContext {
  zone: string
  character: string
}

export interface EventSessionOptions {
  // When false, the helper short-circuits and emits only fight rows —
  // history without dividers, for users who prefer a flat scroll.
  enabled: boolean
  // Inactivity threshold (seconds) that triggers a 'gap' break. Defaults
  // to LONG_GAP_SECONDS — see that constant for the rationale.
  longGapSeconds?: number
}

// groupByEventSession is the combat-history variant: instead of breaking
// on a single short timer, it breaks on observable session boundaries —
// zone change, character switch, or a long idle gap that stands in for a
// /camp or quit. Newest-first ordering, same as groupBySession.
//
// Priority when multiple conditions fire between two fights: character >
// zone > gap. Characters mean "different log file"; zones mean "same
// player, different place"; gap is the weakest signal.
export function groupByEventSession<T extends FightLike>(
  fights: T[],
  keyOf: (f: T) => string,
  contextOf: (f: T) => FightContext,
  options: EventSessionOptions,
): SessionRow<T>[] {
  if (!options.enabled) {
    return fights.map((f) => ({ kind: 'fight', fight: f }))
  }
  const longGap = options.longGapSeconds ?? LONG_GAP_SECONDS
  const out: SessionRow<T>[] = []
  for (let i = 0; i < fights.length; i++) {
    const f = fights[i]
    if (i > 0) {
      const prev = fights[i - 1]
      const prevCtx = contextOf(prev)
      const fCtx = contextOf(f)
      const prevStart = new Date(prev.start_time).getTime()
      const fEnd = new Date(f.end_time).getTime()
      const gap = (prevStart - fEnd) / 1000
      const key = `gap-${keyOf(prev)}-${keyOf(f)}`
      if (fCtx.character && prevCtx.character && fCtx.character !== prevCtx.character) {
        out.push({ kind: 'gap', gapSeconds: gap, reason: 'character', from: fCtx.character, to: prevCtx.character, key })
      } else if (fCtx.zone && prevCtx.zone && fCtx.zone !== prevCtx.zone) {
        out.push({ kind: 'gap', gapSeconds: gap, reason: 'zone', from: fCtx.zone, to: prevCtx.zone, key })
      } else if (gap >= longGap) {
        out.push({ kind: 'gap', gapSeconds: gap, reason: 'gap', key })
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

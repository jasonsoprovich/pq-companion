/**
 * Shared CH Metronome logic used by both the in-dashboard panel
 * (components/overlays/CHMetronomePanel.tsx) and the popped-out window
 * (pages/CHMetronomeOverlayWindowPage.tsx). Keeping the position math and the
 * slot-learning here means the two views can't drift apart.
 *
 * The core problem this solves: the metronome needs to anchor off the cleric
 * in the slot immediately before yours, but guilds encode chain positions
 * differently. Some call 001/002/003 (the number equals the slot), some call
 * AAA/BBB/CCC (A→1, B→2…), and some call coded sequences like 111/222/333 where
 * the number does NOT equal the slot. Matching the raw number against the slot
 * index only works for the first two; this module ranks the distinct numbers
 * actually seen into ordinal slots so all three behave the same.
 */
import type { ActiveTimer } from '../types/timer'

// Which chain the metronome follows. 'main' = ch_chain timers, 'ramp' =
// ch_chain_2 timers (the optional secondary ramp/split chain).
export type ChainView = 'main' | 'ramp'

export interface MetronomeCfg {
  position: number
  chainSize: number
  delay: number
}

// CH cast time in seconds — mirrors backend config.CHCastSecs. A constant of
// the spell, so the personal countdown window is always 10s long.
export const CH_CAST = 10

export function categoryFor(chain: ChainView): string {
  return chain === 'ramp' ? 'ch_chain_2' : 'ch_chain'
}

// parsePosition pulls the leading "#N" chain position out of a ch_chain timer
// label ("#N  Target  ← Caster"). Returns 0 when the label has no position.
export function parsePosition(label: string): number {
  const m = /^#(\d+)/.exec(label)
  return m ? parseInt(m[1], 10) : 0
}

// watchPosition is the slot immediately before mine — the cleric whose callout
// starts my countdown. Position 1 follows the last position (wrap).
export function watchPosition(c: MetronomeCfg): number {
  if (c.position <= 1) return c.chainSize
  return c.position - 1
}

// learnWindowMs is how long a call number stays "learned" without being seen
// again. It must exceed one full chain cycle (chainSize × delay) — a cleric who
// only calls once per cycle would otherwise be pruned before their next call —
// so we keep two cycles, with a 60s floor for tiny/fast chains.
export function learnWindowMs(c: MetronomeCfg): number {
  return Math.max(60_000, c.chainSize * c.delay * 1000 * 2)
}

// recordPositions refreshes the learned set of distinct chain-call numbers for
// the active chain from the current timer feed, then prunes numbers not seen
// within windowMs. Accumulating across snapshots matters because a CH cast only
// lasts ~10s while a full chain cycle is longer, so the live feed never holds
// all of a chain's distinct numbers at once. `seen` maps call-number → last-seen
// ms and is owned by the caller (a ref) so it survives between renders.
export function recordPositions(
  seen: Map<number, number>,
  timers: ActiveTimer[],
  category: string,
  nowMs: number,
  windowMs: number,
): void {
  for (const t of timers) {
    if (t.category !== category) continue
    const p = parsePosition(t.spell_name)
    if (p > 0) seen.set(p, nowMs)
  }
  for (const [num, ts] of seen) {
    if (nowMs - ts > windowMs) seen.delete(num)
  }
}

// watchNumberFor returns the chain-call NUMBER occupying the watched slot, by
// ranking the learned distinct numbers ascending (slot 1 = smallest). Until the
// full set (>= chainSize numbers) has been learned it falls back to the literal
// slot index, so guilds whose calls already equal their slot (001/002/003,
// AAA/BBB/CCC) behave exactly as before with no warm-up penalty — for them the
// ranked value equals the literal index anyway. Coded sequences like
// 111/222/333 engage the ordinal mapping once their full set is observed (one
// chain cycle), after which 111→slot 1, 222→slot 2, 333→slot 3.
export function watchNumberFor(seen: Map<number, number>, c: MetronomeCfg, watch: number): number {
  const ranked = [...seen.keys()].sort((a, b) => a - b)
  if (ranked.length >= c.chainSize) {
    const n = ranked[watch - 1]
    if (n !== undefined) return n
  }
  return watch
}

// positionForNumber is the inverse of watchNumberFor: given a chain-call
// NUMBER actually seen in the feed, returns which slot it occupies. Same
// warm-up fallback (assume number === slot until the full set is learned).
function positionForNumber(seen: Map<number, number>, chainSize: number, num: number): number {
  const ranked = [...seen.keys()].sort((a, b) => a - b)
  if (ranked.length >= chainSize) {
    const idx = ranked.indexOf(num)
    if (idx !== -1) return idx + 1
  }
  return num
}

// forwardDistance is how many chain beats separate `from` and `to` moving
// forward through the cycle (1 → 2 → … → chainSize → 1 → …), wrapping as
// needed. Used to project the watched slot's expected cast time off of
// whichever other slot's callout is freshest. Never called with from === to.
function forwardDistance(from: number, to: number, chainSize: number): number {
  const d = to - from
  return d > 0 ? d : d + chainSize
}

// anchorRemaining returns the remaining_seconds value to derive an anchor
// from, floored at 0. possible_miss deliberately gets no special treatment:
// the backend now holds a flagged row past its expiry via a separate
// missGraceUntil deadline rather than by pushing ExpiresAt forward (see
// spelltimer.Engine.pruneExpired), so remaining_seconds falls cleanly to 0 and
// stays there. It previously moved, which made the anchor jump by the grace
// amount the moment a miss was flagged (and jump back when a late confirmation
// cleared it) — read by the metronome as a whole new cycle, re-firing the
// countdown-start and CAST NOW alerts.
function anchorRemaining(t: ActiveTimer): number {
  return Math.max(0, t.remaining_seconds)
}

// measureCadence derives the chain's live spacing from the median gap between
// consecutive callout start times in the feed, so extrapolation follows the
// raid actually speeding up or slowing down. Returns null with fewer than two
// callouts to measure.
//
// This is deliberately NOT the configured `delay`. Delay is a personal offset
// ("cast this many seconds after the cleric ahead of me"), which a healer
// retunes mid-fight to compensate for slow — using it as the chain's beat
// spacing meant every such adjustment silently re-timed every projection, one
// of the ways the metronome drifted after a mid-fight delay change.
export function measureCadence(timers: ActiveTimer[], category: string): number | null {
  const starts = timers
    .filter((t) => t.category === category)
    .map((t) => Date.parse(t.starts_at))
    .filter((ms) => !Number.isNaN(ms))
    .sort((a, b) => a - b)
  if (starts.length < 2) return null
  const gaps: number[] = []
  for (let i = 1; i < starts.length; i++) gaps.push((starts[i] - starts[i - 1]) / 1000)
  gaps.sort((a, b) => a - b)
  const median = gaps[Math.floor(gaps.length / 2)]
  return median > 0 ? median : null
}

// latestRealAnchor finds the freshest confirmed callout currently in the feed
// — for ANY slot in the chain, not just the watched one — and its local-clock
// cast-start time. This is the anchor computeAnchorMs extrapolates from when
// the watched slot itself hasn't called this cycle.
function latestRealAnchor(
  timers: ActiveTimer[],
  category: string,
  seen: Map<number, number>,
  chainSize: number,
  nowMs: number,
): { position: number; anchorMs: number } | null {
  let best: { position: number; anchorMs: number; startMs: number } | null = null
  for (const t of timers) {
    if (t.category !== category) continue
    const num = parsePosition(t.spell_name)
    if (num <= 0) continue
    const startMs = Date.parse(t.starts_at)
    if (Number.isNaN(startMs)) continue
    if (best && startMs <= best.startMs) continue
    const position = positionForNumber(seen, chainSize, num)
    best = { position, anchorMs: nowMs - (CH_CAST - anchorRemaining(t)) * 1000, startMs }
  }
  return best ? { position: best.position, anchorMs: best.anchorMs } : null
}

// AnchorResult distinguishes a confirmed callout from a projected one so the
// UI can tell a healer "this countdown is a prediction" rather than implying
// the cleric ahead of them was actually heard casting. cadenceSecs is the beat
// spacing this anchor was derived under, carried along so acceptNewAnchor and
// the alert hooks can reason in whole chain cycles without re-measuring.
export interface AnchorResult {
  anchorMs: number
  predicted: boolean
  cadenceSecs: number
}

// cycleMs is how long one full pass around the chain takes at the given
// cadence — the spacing between two consecutive callouts from the SAME slot,
// and so the natural unit for "is this the same cycle or the next one".
export function cycleMs(chainSize: number, cadenceSecs: number): number {
  return Math.max(1, chainSize) * cadenceSecs * 1000
}

// computeAnchorMs derives the local-clock ms at which the watched cleric's
// cast started (their heal lands CH_CAST seconds later), or null when there's
// nothing to go on yet. Using the timer's backend-computed remaining_seconds
// (not the log timestamp) keeps the countdown immune to game-log/local clock
// skew. Mutates `seen` to fold in the latest feed.
//
// When the watched slot's own callout is missing this cycle (interrupted,
// fizzled, skipped, or simply hasn't happened yet), this falls back to
// extrapolating from whichever OTHER slot's callout is freshest, projecting
// forward by (slots between them) × delay. That's what lets the chain keep
// advancing when a cleric misses their cast instead of stalling forever on
// the one slot immediately ahead: e.g. in a 5-cleric chain, if 003 never
// calls, 004 still gets a predicted cast time derived from 002's real call.
export function computeAnchorMs(
  timers: ActiveTimer[],
  c: MetronomeCfg,
  chain: ChainView,
  seen: Map<number, number>,
  nowMs: number,
): AnchorResult | null {
  const category = categoryFor(chain)
  recordPositions(seen, timers, category, nowMs, learnWindowMs(c))
  const watch = watchPosition(c)
  const watchNum = watchNumberFor(seen, c, watch)
  const cadenceSecs = measureCadence(timers, category) ?? c.delay
  let best: ActiveTimer | null = null
  for (const t of timers) {
    if (t.category !== category) continue
    if (parsePosition(t.spell_name) !== watchNum) continue
    if (!best || Date.parse(t.starts_at) > Date.parse(best.starts_at)) best = t
  }
  if (best) {
    return { anchorMs: nowMs - (CH_CAST - anchorRemaining(best)) * 1000, predicted: false, cadenceSecs }
  }

  const latest = latestRealAnchor(timers, category, seen, c.chainSize, nowMs)
  if (!latest || latest.position === watch) return null
  const gap = forwardDistance(latest.position, watch, c.chainSize)
  return { anchorMs: latest.anchorMs + gap * cadenceSecs * 1000, predicted: true, cadenceSecs }
}

// acceptNewAnchor decides whether a freshly computed anchor should replace the
// currently active one.
//
// Two things it must get right, both of which the old "gap >= delay" rule got
// wrong in a busy chain:
//
//  1. A REAL anchor (the watched slot's own callout) is ground truth and must
//     be able to correct a projection for the same cycle in either direction.
//     The watched slot's timer only exists for the 10s cast, while a full
//     cycle runs longer, so most ticks fall back to the projected path and
//     claim the cycle first — under the old rule the real callout then arrived
//     "too close" to the projection and was rejected outright. The metronome
//     ended up running almost entirely on projections, inheriting their drift
//     (measured against a real 6-cleric raid log: it fired off the real
//     callout twice in 35 cycles, and ran up to 3s early).
//
//  2. Anything else must be a genuinely NEW cycle, not a re-derivation of the
//     current one from a still-live timer or a slightly different projection
//     source — those restart the countdown and flash CAST NOW a second time
//     within a few seconds. Cadence, not the configured delay, sets that bar,
//     so retuning delay mid-fight can't loosen it.
export function acceptNewAnchor(prev: AnchorResult | null, next: AnchorResult, c: MetronomeCfg): boolean {
  if (!prev) return true
  const gapMs = next.anchorMs - prev.anchorMs
  if (!next.predicted && prev.predicted && Math.abs(gapMs) < cycleMs(c.chainSize, next.cadenceSecs) / 2) {
    return true
  }
  if (gapMs <= 0) return false // not newer than what's already anchored
  return gapMs >= Math.min(c.delay, next.cadenceSecs) * 800 // 0.8 × the beat, in ms
}

// sameCycle reports whether two anchor times describe the same trip around the
// chain. Used to fire each audio cue once per cycle: a real callout correcting
// its own projection legitimately moves the anchor by a second or two, and the
// alert must not treat that correction as a fresh cycle and speak twice.
export function sameCycle(aMs: number, bMs: number, chainSize: number, cadenceSecs: number): boolean {
  return Math.abs(aMs - bMs) < cycleMs(chainSize, cadenceSecs) / 2
}

// SelfCastEvent mirrors the backend's chchain.SelfCastEvent — broadcast when
// the local player begins casting a recognized CH-chain heal, so the
// metronome can show a confirmed "cast sent" instead of an assumed one.
export interface SelfCastEvent {
  spell_name: string
  cast_at: string
}

// selfCastConfirmsAnchor reports whether a self-cast event should be treated
// as confirming the currently active anchor's cycle (active = within
// graceSecs of the watched slot's CH_CAST-length cast, matching each view's
// own ANCHOR_GRACE_SECS so "confirmed" can never outlive "active").
// Deliberately loose otherwise (any self-cast heal while the cycle is active
// counts, not just one landing near the configured delay) — there's only
// ever one active cycle at a time, and requiring a tighter match would risk
// missing a genuine confirmation over a false negative, which is the wrong
// tradeoff here (see SelfCastWatcher's doc comment on why this can only ever
// confirm, never flag a miss).
export function selfCastConfirmsAnchor(
  anchor: AnchorResult | null,
  castAtMs: number,
  nowMs: number,
  graceSecs: number,
): boolean {
  if (anchor == null || Number.isNaN(castAtMs)) return false
  const elapsedSinceAnchor = castAtMs - anchor.anchorMs
  const elapsedSinceCast = nowMs - castAtMs
  return elapsedSinceAnchor >= 0 && elapsedSinceAnchor <= (CH_CAST + graceSecs) * 1000 && elapsedSinceCast >= 0
}

// seenStorageKey namespaces the persisted learned-number map per chain view
// (main/ramp) so switching chains never mixes their numbering. Exported so
// callers can recognize this key in a 'storage' event.
export function seenStorageKey(chain: ChainView): string {
  return `chMetronome:seen:${chain}`
}

// loadSeen restores the learned chain-call-number map from localStorage. The
// dashboard panel and the popped-out overlay are two views of the same
// metronome, not two independent learners — without this, whichever one
// happens to (re)mount most recently starts the ordinal ranking from scratch
// and can spend a full chain cycle mapping coded (111/222/333-style) calls to
// the wrong slot before it catches up, even though the other view already
// learned the mapping correctly.
export function loadSeen(chain: ChainView): Map<number, number> {
  try {
    const raw = localStorage.getItem(seenStorageKey(chain))
    if (!raw) return new Map()
    const obj = JSON.parse(raw) as Record<string, number>
    const out = new Map<number, number>()
    for (const [k, v] of Object.entries(obj)) {
      const num = parseInt(k, 10)
      if (Number.isFinite(num) && Number.isFinite(v)) out.set(num, v)
    }
    return out
  } catch {
    return new Map()
  }
}

// saveSeen persists the learned map so a reload of either view picks up
// where learning left off, and so the native 'storage' event fires on the
// other same-origin window for live sync while both are open.
export function saveSeen(chain: ChainView, seen: Map<number, number>): void {
  try {
    localStorage.setItem(seenStorageKey(chain), JSON.stringify(Object.fromEntries(seen)))
  } catch {
    /* noop */
  }
}

// mergeSeen folds a persisted snapshot into a live map, keeping the newer
// last-seen timestamp per number so a lagging write from one window can never
// regress state the other window already learned more recently.
export function mergeSeen(into: Map<number, number>, from: Map<number, number>): void {
  for (const [num, ts] of from) {
    const cur = into.get(num)
    if (cur === undefined || ts > cur) into.set(num, ts)
  }
}

// alertsEnabledKey namespaces the bell-icon mute toggle in the popped-out
// overlay's header. It's a master switch layered on top of the per-alert
// enabled flags configured in Settings > Spell Timers: muting here doesn't
// touch those settings, it just silences whichever of them are on. Shared via
// localStorage so useMetronomeAlerts (mounted once at the App level, not
// inside the overlay window) can read the same flag the header button writes.
export const ALERTS_ENABLED_KEY = 'chMetronome:alertsEnabled'

// loadAlertsEnabled defaults to true (unmuted) so existing configured alerts
// keep firing until the user explicitly mutes them from the overlay header.
export function loadAlertsEnabled(): boolean {
  return localStorage.getItem(ALERTS_ENABLED_KEY) !== 'false'
}

export function saveAlertsEnabled(enabled: boolean): void {
  try {
    localStorage.setItem(ALERTS_ENABLED_KEY, String(enabled))
  } catch {
    /* noop */
  }
}

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
    best = { position, anchorMs: nowMs - (CH_CAST - t.remaining_seconds) * 1000, startMs }
  }
  return best ? { position: best.position, anchorMs: best.anchorMs } : null
}

// AnchorResult distinguishes a confirmed callout from a projected one so the
// UI can tell a healer "this countdown is a prediction" rather than implying
// the cleric ahead of them was actually heard casting.
export interface AnchorResult {
  anchorMs: number
  predicted: boolean
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
  let best: ActiveTimer | null = null
  for (const t of timers) {
    if (t.category !== category) continue
    if (parsePosition(t.spell_name) !== watchNum) continue
    if (!best || Date.parse(t.starts_at) > Date.parse(best.starts_at)) best = t
  }
  if (best) return { anchorMs: nowMs - (CH_CAST - best.remaining_seconds) * 1000, predicted: false }

  const latest = latestRealAnchor(timers, category, seen, c.chainSize, nowMs)
  if (!latest || latest.position === watch) return null
  const gap = forwardDistance(latest.position, watch, c.chainSize)
  return { anchorMs: latest.anchorMs + gap * c.delay * 1000, predicted: true }
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

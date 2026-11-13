/**
 * One-line summaries shown in a popout's title bar while it's collapsed (see
 * useOverlayCollapse). Each returns null when there's nothing worth showing,
 * so an idle overlay collapses to a plain title instead of a placeholder.
 */
import type { ActiveTimer } from '../types/timer'
import { fmtRemaining } from './timeFormat'

function remainingSecs(t: ActiveTimer): number {
  return (Date.parse(t.expires_at) - Date.now()) / 1000
}

function soonest(timers: ActiveTimer[]): ActiveTimer | null {
  let best: ActiveTimer | null = null
  for (const t of timers) {
    if (t.expired) continue
    if (!best || remainingSecs(t) < remainingSecs(best)) best = t
  }
  return best
}

/**
 * "Clarity 42s · 9 buffs" — the timer expiring soonest plus a count.
 * `countLabel` turns the number of live timers into the trailing text.
 */
export function timerSummary(
  timers: ActiveTimer[],
  countLabel: (n: number, live: ActiveTimer[]) => string,
): string | null {
  const live = timers.filter((t) => !t.expired)
  const next = soonest(live)
  if (!next) return null
  return `${next.spell_name} ${fmtRemaining(remainingSecs(next), true)} · ${countLabel(live.length, live)}`
}

export const plural = (n: number, one: string, many = `${one}s`): string =>
  `${n} ${n === 1 ? one : many}`

// Distinct mobs across a set of detrimental timers.
export function mobCount(live: ActiveTimer[]): number {
  return new Set(live.map((t) => t.target_id ?? t.target_name)).size
}

// Compact number for meters: 980, 1.4k, 12k.
export function compactNumber(n: number): string {
  if (!Number.isFinite(n)) return '0'
  if (n >= 10_000) return `${Math.round(n / 1000)}k`
  if (n >= 1_000) return `${(n / 1000).toFixed(1)}k`
  return `${Math.round(n)}`
}

export function ordinal(n: number): string {
  const s = ['th', 'st', 'nd', 'rd']
  const v = n % 100
  return `${n}${s[(v - 20) % 10] ?? s[v] ?? s[0]}`
}

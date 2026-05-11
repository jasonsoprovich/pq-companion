import type { RollSession, WinnerRule, Roll } from '../types/rolls'

export function fmtRollTime(iso: string | undefined): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleTimeString()
  } catch {
    return iso
  }
}

/**
 * winnersFor returns the set of player names currently considered winners
 * for a session under the given rule. Each player's *first* roll counts
 * (re-rolls excluded); ties on the winning value all return as co-winners
 * so the user can resolve them however they like.
 */
export function winnersFor(session: RollSession, rule: WinnerRule): Set<string> {
  const firstByPlayer = new Map<string, Roll>()
  for (const r of session.rolls) {
    if (!firstByPlayer.has(r.roller)) firstByPlayer.set(r.roller, r)
  }
  if (firstByPlayer.size === 0) return new Set()
  const values = [...firstByPlayer.values()].map((r) => r.value)
  const target = rule === 'highest' ? Math.max(...values) : Math.min(...values)
  const winners = new Set<string>()
  for (const r of firstByPlayer.values()) {
    if (r.value === target) winners.add(r.roller)
  }
  return winners
}

/**
 * sortRolls orders rolls so the leader under the current rule appears at
 * the top of the list. Duplicate (re-roll) entries keep their relative
 * order to make the chronology obvious.
 */
export function sortRolls(rolls: Roll[], rule: WinnerRule): Roll[] {
  const copy = [...rolls]
  copy.sort((a, b) => (rule === 'highest' ? b.value - a.value : a.value - b.value))
  return copy
}

/**
 * countdownSeconds returns the whole seconds remaining until session
 * auto-stop, or null if there is no scheduled stop. Negative values are
 * clamped to 0 so the UI never shows "−1s" during the brief window
 * between expiry and the broadcast arriving.
 */
export function countdownSeconds(session: RollSession, now: number): number | null {
  if (!session.auto_stop_at) return null
  const target = new Date(session.auto_stop_at).getTime()
  if (Number.isNaN(target)) return null
  return Math.max(0, Math.ceil((target - now) / 1000))
}

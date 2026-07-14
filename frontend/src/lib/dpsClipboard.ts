import type { EntityStats } from '../types/combat'
import { rollupCombatants, petBadge } from './dpsRollup'
import { dpsForMode, dpsModeAbbrev, type DPSMode } from '../hooks/useDPSMode'
import { EQ_CHAT_LINE_MAX } from './eqClipboard'

// Cap on ranked entries regardless of how much character budget is left —
// a raid callout only needs the top of the parse, not the full 40-person
// breakdown.
const MAX_ENTRIES = 10

function formatEntry(rank: number, c: ReturnType<typeof rollupCombatants>[number], mode: DPSMode, unitLabel: string): string {
  return `#${rank} ${c.name}${petBadge(c.pets)} ${Math.round(dpsForMode(c, mode))}${unitLabel} ${c.total_damage.toLocaleString()}dmg`
}

/**
 * buildDpsFightSummary produces a single-line, paste-into-EQ ranked DPS
 * summary, e.g. `Aten Ha Ra: #1 Belnoctourne 3421pdps 987,654dmg | #2 ...`.
 * Pasting multi-line text into EQ's chat box collapses the newlines into one
 * giant line instead of sending each as a separate message, so this must
 * stay single-line — every DPS clipboard button (live overlay, dashboard
 * panel, Combat Log, Combat History) shares this one implementation rather
 * than each building its own text. Entries are added until the next one
 * would exceed EQ_CHAT_LINE_MAX (reserving room for a trailing "+N more"),
 * so a big roster truncates at a whole entry instead of mid-name. Returns
 * '' when there are no combatants yet.
 */
export function buildDpsFightSummary(
  target: string | undefined,
  fight: { combatants: EntityStats[]; duration_seconds: number },
  combine: boolean,
  mode: DPSMode,
): string {
  const rolled = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  if (rolled.length === 0) return ''
  const unitLabel = dpsModeAbbrev(mode).toLowerCase()
  const capped = rolled.slice(0, MAX_ENTRIES)
  let out = target ? `${target}: ` : ''
  let used = 0
  for (let i = 0; i < capped.length; i++) {
    const entry = formatEntry(i + 1, capped[i], mode, unitLabel)
    const curSep = used === 0 ? '' : ' | '
    const candidate = out + curSep + entry
    // After adding this entry there's always >=1 entry present, so a
    // trailing "+N more" (if still needed) is always pipe-separated.
    const remainingAfter = rolled.length - used - 1
    const suffixLen = remainingAfter > 0 ? ` | +${remainingAfter} more`.length : 0
    if (candidate.length + suffixLen > EQ_CHAT_LINE_MAX) {
      const remaining = rolled.length - used
      return remaining > 0 ? `${out}${curSep}+${remaining} more` : out
    }
    out = candidate
    used++
  }
  if (rolled.length > used) {
    const sep = used === 0 ? '' : ' | '
    const withMore = `${out}${sep}+${rolled.length - used} more`
    return withMore.length <= EQ_CHAT_LINE_MAX ? withMore : out
  }
  return out
}

import type { RollSession, WinnerRule, Roll, RollProfile } from '../types/rolls'
import { EQ_CHAT_LINE_MAX, clampChatLine } from './eqClipboard'

export function fmtRollTime(iso: string | undefined): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleTimeString()
  } catch {
    return iso
  }
}

/**
 * winnersAmong returns the winning player name(s) and the winning value from
 * an arbitrary set of rolls under the given rule. Each player's *first* roll
 * counts (re-rolls excluded); ties on the winning value all return as
 * co-winners. value is 0 when there are no rolls.
 */
export function winnersAmong(
  rolls: Roll[],
  rule: WinnerRule,
): { winners: Set<string>; value: number } {
  const firstByPlayer = new Map<string, Roll>()
  for (const r of rolls) {
    if (!firstByPlayer.has(r.roller)) firstByPlayer.set(r.roller, r)
  }
  if (firstByPlayer.size === 0) return { winners: new Set(), value: 0 }
  const values = [...firstByPlayer.values()].map((r) => r.value)
  const target = rule === 'highest' ? Math.max(...values) : Math.min(...values)
  const winners = new Set<string>()
  for (const r of firstByPlayer.values()) {
    if (r.value === target) winners.add(r.roller)
  }
  return { winners, value: target }
}

/**
 * winnersFor returns the set of player names currently considered winners
 * for a session under the given rule. Thin wrapper over winnersAmong.
 */
export function winnersFor(session: RollSession, rule: WinnerRule): Set<string> {
  return winnersAmong(session.rolls, rule).winners
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

function formatWinners(names: string[]): string {
  return names.length === 1 ? names[0] : `${names.join(' & ')} (tie)`
}

/**
 * buildRollSummary produces a one-line, paste-into-EQ summary of a
 * session's outcome, e.g. `Robe of the Lost Circle — Belnoctourne (532/611)`.
 * Falls back to the range when the session has no item label, and lists
 * co-winners on a tie. Returns '' when there are no rolls yet (so callers
 * can disable the copy button). The result is clamped to EQ's chat-line
 * length so it pastes cleanly.
 */
export function buildRollSummary(session: RollSession, rule: WinnerRule): string {
  const { winners, value } = winnersAmong(session.rolls, rule)
  if (winners.size === 0) return ''
  const label = session.item_name.trim() || `Roll ${session.min}–${session.max}`
  return clampChatLine(`${label} — ${formatWinners([...winners])} (${value}/${session.max})`)
}

/**
 * rankRolls dedupes to each player's first roll and buckets the whole field
 * into ordered tie-groups under the rule — group 0 is the winning value,
 * group 1 the next distinct value, and so on. Players who never rolled
 * don't appear.
 */
export function rankRolls(rolls: Roll[], rule: WinnerRule): { value: number; names: string[] }[] {
  const firstByPlayer = new Map<string, Roll>()
  for (const r of rolls) {
    if (!firstByPlayer.has(r.roller)) firstByPlayer.set(r.roller, r)
  }
  const byValue = new Map<number, string[]>()
  for (const r of firstByPlayer.values()) {
    const names = byValue.get(r.value) ?? []
    names.push(r.roller)
    byValue.set(r.value, names)
  }
  const values = [...byValue.keys()].sort((a, b) => (rule === 'highest' ? b - a : a - b))
  return values.map((value) => ({ value, names: byValue.get(value)! }))
}

function formatPickOrderEntry(rank: number, group: { value: number; names: string[] }): string {
  const suffix = group.names.length > 1 ? `(${group.value},tie)` : `(${group.value})`
  return `${rank}) ${group.names.join('/')}${suffix}`
}

/**
 * buildPickOrderText produces a paste-into-EQ line listing every roller in
 * rank order, e.g. `Robe of the Lost Circle picks: 1) A(611) 2) B(598) ...`.
 * Tied rollers share a rank number and are joined with '/'. Entries are
 * added until the next one would exceed EQ's chat-line length (reserving
 * room for a trailing "+N more"), so a long roster is truncated at a whole
 * entry instead of mid-name. Returns '' when there are no rolls yet.
 */
function buildPickOrderText(label: string, rolls: Roll[], rule: WinnerRule): string {
  const ranked = rankRolls(rolls, rule)
  if (ranked.length === 0) return ''
  const totalPlayers = ranked.reduce((n, g) => n + g.names.length, 0)
  let out = `${label} picks: `
  let usedPlayers = 0
  for (let i = 0; i < ranked.length; i++) {
    const group = ranked[i]
    const entry = formatPickOrderEntry(i + 1, group)
    const candidate = usedPlayers === 0 ? out + entry : `${out} ${entry}`
    const remainingAfter = totalPlayers - usedPlayers - group.names.length
    const suffixLen = remainingAfter > 0 ? ` +${remainingAfter} more`.length : 0
    if (candidate.length + suffixLen > EQ_CHAT_LINE_MAX) {
      const remaining = totalPlayers - usedPlayers
      return remaining > 0 ? `${out.trimEnd()} +${remaining} more` : out.trimEnd()
    }
    out = candidate
    usedPlayers += group.names.length
  }
  return out
}

/**
 * buildPickOrderSummary is the session-level entry point for the "Copy Pick
 * Order" button: the full field, ranked highest/lowest-first per the rule,
 * for pasting a raid's loot pick order into EQ chat.
 */
export function buildPickOrderSummary(session: RollSession, rule: WinnerRule): string {
  const label = session.item_name.trim() || `Roll ${session.min}–${session.max}`
  return buildPickOrderText(label, session.rolls, rule)
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

// ── Tiered roll grouping (profiles) ────────────────────────────────────────
//
// A tiered profile folds the flat session list into "contests". Each contest
// is one loot item rolled across ranked tiers (e.g. Pick > Upgrade > Alt);
// the winner is taken from the highest-priority tier that received any rolls.
// Grouping is derived here on the client — the backend only stores the
// profile. See rolltracker.RollProfile (Go) for the matching model.

/** A single bracket within a contest, carrying the rolls cast into it. */
export interface ContestTier {
  label: string
  priority: number // 0 = best; lower wins
  max: number // representative upper bound, for display (e.g. 111)
  rolls: Roll[] // every roll in this tier, arrival order
  sessions: RollSession[] // backing sessions (usually one)
}

/** One loot item being rolled for across one or more tiers. */
export interface Contest {
  key: string // stable React key
  itemName: string // contest-level label (from the top labeled tier)
  startedAt: string // earliest session start, for ordering
  active: boolean // any backing session still live
  tiers: ContestTier[] // present tiers only, ordered best→worst
  sessions: RollSession[] // all backing sessions, for stop/remove
}

/** The resolved winner of a contest: highest/lowest within the best tier. */
export interface ContestOutcome {
  tierLabel: string
  tierMax: number
  winners: string[]
  value: number
}

// Sessions whose starts are more than this far apart never join the same
// contest. Tier rolls for one item land within seconds of each other; a later
// reuse of the same numbers for a different item is minutes away (and the
// backend's 5-min stale split has already given it fresh sessions).
const contestClusterGapMs = 150_000

/**
 * tierForMax maps a roll's upper bound to a tier index and contest group key
 * under the given profile, or null when the bound matches no tier (those
 * sessions stay ungrouped). Group key separates simultaneous items; for the
 * exact scheme it is always 0 (items are separated by time instead).
 */
export function tierForMax(
  max: number,
  profile: RollProfile,
): { tierIndex: number; groupKey: number } | null {
  if (profile.mode !== 'tiered' || !profile.tiers?.length) return null
  if (profile.scheme === 'suffix') {
    const div = profile.divisor && profile.divisor > 0 ? profile.divisor : 100
    const tierIndex = profile.tiers.findIndex((t) => t.match === max % div)
    if (tierIndex < 0) return null
    return { tierIndex, groupKey: Math.floor(max / div) }
  }
  // exact scheme
  const tierIndex = profile.tiers.findIndex((t) => t.match === max)
  if (tierIndex < 0) return null
  return { tierIndex, groupKey: 0 }
}

function startMs(s: RollSession): number {
  return new Date(s.started_at).getTime()
}

function endMs(s: RollSession): number {
  return new Date(s.last_roll_at).getTime()
}

/** Split sessions sharing a group key into time-clusters so a later reuse of
 *  the same numbers becomes a separate contest. */
function clusterByTime(sessions: RollSession[]): RollSession[][] {
  const sorted = [...sessions].sort((a, b) => startMs(a) - startMs(b))
  const clusters: RollSession[][] = []
  let cur: RollSession[] = []
  let curEnd = 0
  for (const s of sorted) {
    if (cur.length === 0 || startMs(s) - curEnd <= contestClusterGapMs) {
      cur.push(s)
    } else {
      clusters.push(cur)
      cur = [s]
    }
    curEnd = Math.max(curEnd, endMs(s))
  }
  if (cur.length) clusters.push(cur)
  return clusters
}

function buildContest(
  sessions: RollSession[],
  profile: RollProfile,
  groupKey: number,
): Contest {
  const byTier = new Map<number, RollSession[]>()
  for (const s of sessions) {
    const m = tierForMax(s.max, profile)
    if (!m) continue
    const arr = byTier.get(m.tierIndex) ?? []
    arr.push(s)
    byTier.set(m.tierIndex, arr)
  }
  const tiers: ContestTier[] = []
  for (const [tierIndex, tierSessions] of byTier) {
    const def = profile.tiers![tierIndex]
    tiers.push({
      label: def.label,
      priority: tierIndex,
      max: Math.max(...tierSessions.map((s) => s.max)),
      rolls: tierSessions.flatMap((s) => s.rolls),
      sessions: tierSessions,
    })
  }
  tiers.sort((a, b) => a.priority - b.priority)
  // Contest item name = the first non-empty label, best tier first.
  let itemName = ''
  for (const t of tiers) {
    const named = t.sessions.find((s) => s.item_name.trim() !== '')
    if (named) {
      itemName = named.item_name.trim()
      break
    }
  }
  const startedAt = sessions.reduce(
    (min, s) => (s.started_at < min ? s.started_at : min),
    sessions[0].started_at,
  )
  return {
    key: `g${groupKey}-${startedAt}`,
    itemName,
    startedAt,
    active: sessions.some((s) => s.active),
    tiers,
    sessions,
  }
}

/**
 * groupContests folds the flat session list into tiered contests under the
 * active profile. In simple mode (or when nothing matches) every session is
 * returned untouched in `ungrouped`. Matched sessions are bucketed by group
 * key, time-clustered, and assembled into contests (newest first); unmatched
 * sessions fall through to `ungrouped` so they still render as plain cards.
 */
export function groupContests(
  sessions: RollSession[],
  profile: RollProfile,
): { contests: Contest[]; ungrouped: RollSession[] } {
  if (profile.mode !== 'tiered' || !profile.tiers?.length) {
    return { contests: [], ungrouped: sessions }
  }
  const byGroup = new Map<number, RollSession[]>()
  const ungrouped: RollSession[] = []
  for (const s of sessions) {
    const m = tierForMax(s.max, profile)
    if (!m) {
      ungrouped.push(s)
      continue
    }
    const arr = byGroup.get(m.groupKey) ?? []
    arr.push(s)
    byGroup.set(m.groupKey, arr)
  }
  const contests: Contest[] = []
  for (const [groupKey, members] of byGroup) {
    for (const cluster of clusterByTime(members)) {
      contests.push(buildContest(cluster, profile, groupKey))
    }
  }
  contests.sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())
  return { contests, ungrouped }
}

/**
 * contestOutcome resolves a contest's winner: walk tiers best→worst and the
 * first tier with any rolls decides it, winner picked by the rule within that
 * tier. Returns null when no tier has rolls yet.
 */
export function contestOutcome(contest: Contest, rule: WinnerRule): ContestOutcome | null {
  for (const tier of contest.tiers) {
    const { winners, value } = winnersAmong(tier.rolls, rule)
    if (winners.size > 0) {
      return { tierLabel: tier.label, tierMax: tier.max, winners: [...winners], value }
    }
  }
  return null
}

/**
 * buildContestSummary produces the paste-into-EQ line for a contest, e.g.
 * `Robe of the Lost Circle — Belnoctourne (118/133, Alt)`. Returns '' when no
 * tier has rolls yet.
 */
export function buildContestSummary(contest: Contest, rule: WinnerRule): string {
  const o = contestOutcome(contest, rule)
  if (!o) return ''
  const label = contest.itemName.trim() || 'Roll'
  return clampChatLine(
    `${label} — ${formatWinners(o.winners)} (${o.value}/${o.tierMax}, ${o.tierLabel})`,
  )
}

/**
 * buildContestPickOrderSummary is the tiered-contest counterpart of
 * buildPickOrderSummary: it ranks the full field within the same tier
 * contestOutcome would resolve the winner from (the highest-priority tier
 * that has any rolls), rather than merging ranks across tiers — a low roll
 * in a better tier still out-picks a high roll in a worse one.
 */
export function buildContestPickOrderSummary(contest: Contest, rule: WinnerRule): string {
  for (const tier of contest.tiers) {
    if (tier.rolls.length === 0) continue
    const label = `${contest.itemName.trim() || 'Roll'} (${tier.label})`
    const text = buildPickOrderText(label, tier.rolls, rule)
    if (text) return text
  }
  return ''
}

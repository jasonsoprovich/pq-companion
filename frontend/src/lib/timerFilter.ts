/**
 * timerFilter — pure filtering/grouping for the Filtered Timers overlay.
 *
 * The Filtered Timers window is a second, independently-configured view over
 * the same overlay:timers feed the Buff and Detrimental overlays read. Nothing
 * here touches the backend: it narrows the live ActiveTimer list by type
 * (beneficial/detrimental), groups it by spell or by target, and — for the
 * beneficial "by spell" view — diffs a live group/raid roster against who
 * currently has each spell, to list who is missing it.
 */
import type { ActiveTimer, TimerCategory } from '../types/timer'

export type FilterType = 'beneficial' | 'detrimental'
// 'target' is labelled "Player" for beneficial and "Mob" for detrimental.
export type FilterViewBy = 'spell' | 'target'

// Sentinel target values for the target view. Real names never start with
// "__", so these can share the same string field.
export const TARGET_SELF = '__self__'
export const TARGET_CURRENT = '__current_target__'

export interface FilteredTimerConfig {
  type: FilterType
  viewBy: FilterViewBy
  // Spell name (spell view) or target name / sentinel (target view). Empty
  // means "all" — every matching spell/target, one group each.
  target: string
  onlySelected: boolean
  selectedSpells: string[]
  underOneMin: boolean
  // Beneficial spell view only: list roster members missing each spell.
  showMissing: boolean
  // Apply the global / per-trigger display thresholds the Buff and
  // Detrimental overlays use. Off by default: this window's point is to show
  // the picked spells regardless of time left.
  respectThresholds: boolean
}

export const DEFAULT_FILTERED_TIMER_CONFIG: FilteredTimerConfig = {
  type: 'beneficial',
  viewBy: 'spell',
  target: '',
  onlySelected: false,
  selectedSpells: [],
  underOneMin: false,
  showMissing: false,
  respectThresholds: false,
}

export const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

const UNDER_ONE_MIN_SECS = 60

export interface FilterContext {
  activePlayer: string
  // Name of the Zeal-pipe current target, '' when none / unknown.
  currentTarget: string
  // Its spawn id, when Zeal reports one. With it, the current-target view
  // drops timers pinned to a different same-named spawn and marks the ones
  // pinned to no spawn as unproven.
  currentTargetId?: number
  // Live group/raid roster (including the active player) for the missing
  // list; empty when no roster is available.
  roster: string[]
  // Timers to check the missing list against, before any time-based
  // filtering (display thresholds, "under 1 min"). A buff that's hidden
  // because it still has plenty of time left is held, not missing.
  // Defaults to the `timers` argument.
  holders?: ActiveTimer[]
}

export interface TimerGroup {
  key: string
  // Spell name (spell view) or target display name (target view).
  title: string
  icon?: number
  rows: ActiveTimer[]
  // Roster members without this spell (beneficial spell view + showMissing
  // only; empty otherwise).
  missing: string[]
  // Ids of rows matched by name only, not by spawn id (current-target view
  // with a known target id). They may belong to another same-named spawn.
  unproven: Set<string>
  // Spawn id of the mob the group is for (current-target view only).
  targetId?: number
}

// One-click setup for "what's on my target": every detrimental on the
// current target, regardless of time left.
export const TARGET_DEBUFFS_PRESET: Partial<FilteredTimerConfig> = {
  type: 'detrimental',
  viewBy: 'target',
  target: TARGET_CURRENT,
  onlySelected: false,
  underOneMin: false,
  showMissing: false,
  respectThresholds: false,
}

export function matchesType(t: ActiveTimer, type: FilterType): boolean {
  return type === 'beneficial' ? t.category === 'buff' : DETRIM_CATEGORIES.has(t.category)
}

// The engine keys self-buffs on the active character's name, but falls back
// to the literal "You" before the character context is known. Fold both to
// one name so grouping and roster matching agree.
export function normalizeTarget(name: string, activePlayer: string): string {
  if (name === 'You' && activePlayer) return activePlayer
  return name
}

function sameName(a: string, b: string): boolean {
  return a.localeCompare(b, undefined, { sensitivity: 'accent' }) === 0
}

// Resolve the configured target (which may be a sentinel) to a concrete
// name. Returns '' for "all", and null when a sentinel can't be resolved
// (no current target) — the view then shows nothing rather than everything.
export function resolveTarget(cfg: FilteredTimerConfig, ctx: FilterContext): string | null {
  if (cfg.viewBy === 'spell') return cfg.target
  if (cfg.target === TARGET_SELF) return ctx.activePlayer || 'You'
  if (cfg.target === TARGET_CURRENT) return ctx.currentTarget || null
  return cfg.target
}

// Timers passing every filter except the spell/target selection — the pool
// the target dropdown is populated from.
export function baseFilter(timers: ActiveTimer[], cfg: FilteredTimerConfig): ActiveTimer[] {
  const selected = new Set(cfg.selectedSpells.map((s) => s.toLowerCase()))
  return timers.filter((t) => {
    if (!matchesType(t, cfg.type)) return false
    if (cfg.onlySelected && !selected.has(t.spell_name.toLowerCase())) return false
    if (cfg.underOneMin && t.remaining_seconds > UNDER_ONE_MIN_SECS) return false
    return true
  })
}

function byRemaining(a: ActiveTimer, b: ActiveTimer): number {
  return a.remaining_seconds - b.remaining_seconds
}

export function applyTimerFilter(
  timers: ActiveTimer[],
  cfg: FilteredTimerConfig,
  ctx: FilterContext,
): TimerGroup[] {
  const target = resolveTarget(cfg, ctx)
  if (target === null) return []
  const byId =
    cfg.viewBy === 'target' && cfg.target === TARGET_CURRENT ? ctx.currentTargetId : undefined

  const pool = baseFilter(timers, cfg)
  const groups = new Map<string, TimerGroup>()
  for (const t of pool) {
    const who = normalizeTarget(t.target_name, ctx.activePlayer)
    const key = cfg.viewBy === 'spell' ? t.spell_name : who
    if (!key) continue
    if (target && !sameName(key, target)) continue
    // Same name, different spawn: another mob's debuffs.
    if (byId !== undefined && t.target_id && t.target_id !== byId) continue
    const gk = key.toLowerCase()
    let g = groups.get(gk)
    if (!g) {
      g = {
        key: gk,
        title: key,
        icon: cfg.viewBy === 'spell' ? t.icon : undefined,
        rows: [],
        missing: [],
        unproven: new Set(),
        targetId: byId,
      }
      groups.set(gk, g)
    }
    g.rows.push(t)
    if (byId !== undefined && !t.target_id) g.unproven.add(t.id)
  }

  // A specific selected spell with nobody holding it still gets a (rowless)
  // group, so the missing list can show the whole roster.
  const wantMissing =
    cfg.showMissing && cfg.type === 'beneficial' && cfg.viewBy === 'spell' && ctx.roster.length > 0
  if (wantMissing) {
    const want = target ? [target] : cfg.onlySelected ? cfg.selectedSpells : []
    for (const s of want) {
      const gk = s.toLowerCase()
      if (!groups.has(gk)) {
        groups.set(gk, { key: gk, title: s, rows: [], missing: [], unproven: new Set() })
      }
    }
  }

  const holders = (ctx.holders ?? timers).filter((t) => matchesType(t, cfg.type))
  const out = [...groups.values()]
  for (const g of out) {
    g.rows.sort(byRemaining)
    if (wantMissing) {
      const held = holders.filter((t) => sameName(t.spell_name, g.title))
      g.missing = missingFrom(held, ctx.roster, ctx.activePlayer)
    }
  }
  // Most urgent group first, then name — rowless groups sink to the bottom.
  return out.sort((a, b) => {
    const ar = a.rows[0]?.remaining_seconds ?? Infinity
    const br = b.rows[0]?.remaining_seconds ?? Infinity
    if (ar !== br) return ar - br
    return a.title.localeCompare(b.title)
  })
}

// Roster members with no row in `rows`, matched case-insensitively.
export function missingFrom(rows: ActiveTimer[], roster: string[], activePlayer: string): string[] {
  const have = new Set(rows.map((t) => normalizeTarget(t.target_name, activePlayer).toLowerCase()))
  const seen = new Set<string>()
  const out: string[] = []
  for (const name of roster) {
    const k = name.toLowerCase()
    if (!name || have.has(k) || seen.has(k)) continue
    seen.add(k)
    out.push(name)
  }
  return out.sort((a, b) => a.localeCompare(b))
}

// Dropdown options for the target selector: distinct spell names (spell
// view) or target names (target view) in the current pool, plus any
// selected spells and the saved selection so it survives an empty feed.
export function targetOptions(
  timers: ActiveTimer[],
  cfg: FilteredTimerConfig,
  activePlayer: string,
): string[] {
  const names = new Map<string, string>()
  const add = (n: string): void => {
    if (n && !n.startsWith('__') && !names.has(n.toLowerCase())) names.set(n.toLowerCase(), n)
  }
  for (const t of baseFilter(timers, cfg)) {
    if (cfg.viewBy === 'spell') add(t.spell_name)
    else {
      const who = normalizeTarget(t.target_name, activePlayer)
      if (who !== activePlayer && who !== 'You') add(who)
    }
  }
  if (cfg.viewBy === 'spell') cfg.selectedSpells.forEach(add)
  add(cfg.target)
  return [...names.values()].sort((a, b) => a.localeCompare(b))
}

// Distinct spell names in the feed for the "selected spells" suggestions.
export function knownSpellNames(timers: ActiveTimer[], type: FilterType): string[] {
  const names = new Map<string, string>()
  for (const t of timers) {
    if (matchesType(t, type) && t.spell_name) names.set(t.spell_name.toLowerCase(), t.spell_name)
  }
  return [...names.values()].sort((a, b) => a.localeCompare(b))
}

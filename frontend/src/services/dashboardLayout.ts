/**
 * dashboardLayout — the Overlays Dashboard's panel arrangement (positions,
 * sizes, which panels are shown). Each overlay layout keeps its own, stored
 * by the Electron main process in overlay-layouts.json; useDashboardLayouts()
 * is the one shared copy the Library and the Dashboard both read and write.
 * Switching layouts swaps it out.
 */
import { useEffect, useSyncExternalStore } from 'react'
import type { LayoutDashboard } from '../types/electron'

export type DashboardPanelKey =
  | 'buff'
  | 'detrim'
  | 'dps'
  | 'npc'
  | 'threat'
  | 'rolls'
  | 'respawn'
  | 'chChain'
  | 'chMetronome'
  | 'custom'
  | 'discordVoice'
  | 'liveMap'
  | 'zoneLockouts'
  | 'raidReadiness'
  | 'filtered'
  | 'kills'
  | 'itemInfo'
  | 'pet'
  | 'deathRecap'
  | 'gearUpgrades'

export interface PanelLayout {
  x: number
  y: number
  width: number
  height: number
  visible: boolean
}

export type DashboardLayout = Record<DashboardPanelKey, PanelLayout>

// All defaults are aligned to the 16px snap grid so panels don't visibly
// jump on the user's first drag.
export const DEFAULT_DASHBOARD_LAYOUT: DashboardLayout = {
  buff:   { x: 16,  y: 16,  width: 304, height: 384, visible: true },
  detrim: { x: 336, y: 16,  width: 304, height: 384, visible: true },
  dps:    { x: 16,  y: 416, width: 624, height: 384, visible: true },
  npc:    { x: 656, y: 16,  width: 400, height: 784, visible: true },
  threat:  { x: 1072, y: 16, width: 288, height: 384, visible: true },
  rolls:   { x: 656, y: 816, width: 400, height: 384, visible: false },
  respawn: { x: 336, y: 416, width: 304, height: 384, visible: false },
  chChain:     { x: 656, y: 416, width: 304, height: 336, visible: false },
  chMetronome: { x: 976, y: 416, width: 240, height: 272, visible: false },
  custom:      { x: 976, y: 16,  width: 304, height: 336, visible: false },
  discordVoice: { x: 336, y: 816, width: 240, height: 200, visible: false },
  // Square and larger than the list panels: a map in a narrow strip is unreadable.
  liveMap: { x: 976, y: 704, width: 384, height: 384, visible: false },
  zoneLockouts: { x: 1072, y: 416, width: 300, height: 320, visible: false },
  raidReadiness: { x: 1072, y: 744, width: 280, height: 260, visible: false },
  filtered: { x: 1376, y: 16, width: 304, height: 384, visible: false },
  kills: { x: 1376, y: 416, width: 304, height: 240, visible: false },
  itemInfo: { x: 1072, y: 416, width: 320, height: 464, visible: false },
  pet: { x: 1376, y: 672, width: 304, height: 432, visible: false },
  deathRecap: { x: 1696, y: 16, width: 352, height: 368, visible: false },
  gearUpgrades: { x: 1696, y: 400, width: 304, height: 384, visible: false },
}

export const DASHBOARD_PANEL_KEYS: DashboardPanelKey[] =
  ['buff', 'filtered', 'detrim', 'dps', 'npc', 'threat', 'rolls', 'respawn', 'chChain', 'chMetronome', 'custom', 'discordVoice', 'liveMap', 'zoneLockouts', 'raidReadiness', 'kills', 'itemInfo', 'pet', 'deathRecap', 'gearUpgrades']

export const DASHBOARD_PANEL_LABELS: Record<DashboardPanelKey, string> = {
  buff: 'Buff Timers',
  detrim: 'Detrimental Timers',
  dps: 'DPS Meter',
  npc: 'NPC Overlay',
  threat: 'Threat Meter',
  rolls: 'Roll Tracker',
  respawn: 'Respawn Timers',
  chChain: 'CH Chain',
  chMetronome: 'CH Metronome',
  custom: 'Custom Timers (default)',
  discordVoice: 'Discord Voice',
  liveMap: 'Live Map',
  zoneLockouts: 'Zone Lockouts',
  raidReadiness: 'Raid Readiness',
  filtered: 'Filtered Timers',
  kills: 'Kill Counters',
  itemInfo: 'Item Info',
  pet: 'Pet',
  deathRecap: 'Death Recap',
  gearUpgrades: 'Gear Upgrades',
}

export function isPanelLayout(v: unknown): v is PanelLayout {
  if (!v || typeof v !== 'object') return false
  const o = v as Record<string, unknown>
  return (
    typeof o.x === 'number' &&
    typeof o.y === 'number' &&
    typeof o.width === 'number' &&
    typeof o.height === 'number' &&
    typeof o.visible === 'boolean'
  )
}

// ── Named timer-group dashboard panels ──────────────────────────────────────
//
// Unlike the fixed panels above, a group panel's key (TimerGroup.id) is
// dynamic and unbounded — it can't live in the DashboardPanelKey union or the
// DEFAULT_DASHBOARD_LAYOUT record, both of which need a value for every
// possible key. So group layouts are a parallel map keyed by group id. Same
// shape (PanelLayout), same drag/resize/visibility semantics.
export type GroupPanelLayouts = Record<string, PanelLayout>

// Default layout for a group panel the user has never positioned before.
// Staggered by index so several newly-added groups don't stack exactly on
// top of each other before the user drags them apart. Starts hidden (like
// most fixed panels default to) — a group panel is opt-in, not automatic.
export function defaultGroupPanelLayout(index: number): PanelLayout {
  const offset = (index % 6) * 24
  return { x: 16 + offset, y: 16 + offset, width: 280, height: 280, visible: false }
}

// ── Shared store ────────────────────────────────────────────────────────────

// Where the arrangement lived before layouts had their own. Read once and
// handed to main (layouts:migrate-dashboard), then removed. Also the fallback
// store when there's no Electron main process.
const STORAGE_KEY = 'pq-overlay-dashboard-layout-v1'
const GROUP_STORAGE_KEY = 'pq-overlay-dashboard-group-layout-v1'
const SAVE_DEBOUNCE_MS = 300

function panelsFrom(raw: unknown): DashboardLayout {
  const merged = { ...DEFAULT_DASHBOARD_LAYOUT }
  if (!raw || typeof raw !== 'object') return merged
  const o = raw as Record<string, unknown>
  for (const key of DASHBOARD_PANEL_KEYS) {
    if (isPanelLayout(o[key])) merged[key] = o[key] as PanelLayout
  }
  return merged
}

function groupsFrom(raw: unknown): GroupPanelLayouts {
  const out: GroupPanelLayouts = {}
  if (!raw || typeof raw !== 'object') return out
  for (const [id, p] of Object.entries(raw as Record<string, unknown>)) {
    if (isPanelLayout(p)) out[id] = p
  }
  return out
}

function readLocal(key: string): unknown {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

export interface DashboardState {
  // False until the active layout's arrangement has loaded. Writes before
  // then are dropped, so defaults never overwrite a saved arrangement.
  ready: boolean
  layoutId: string
  panels: DashboardLayout
  groups: GroupPanelLayouts
  // Bumped when the arrangement is replaced from outside a drag (layout
  // switch, reset, first load). Panels keep their position in local state,
  // so the dashboard keys them on this to remount with the new values.
  version: number
}

let state: DashboardState = {
  ready: false,
  layoutId: '',
  panels: { ...DEFAULT_DASHBOARD_LAYOUT },
  groups: {},
  version: 0,
}
const listeners = new Set<() => void>()
let started = false
let saveTimer: ReturnType<typeof setTimeout> | null = null
let pendingSave: { id: string; dashboard: LayoutDashboard } | null = null

function hasMain(): boolean {
  return typeof window.electron?.overlay?.layoutsDashboardGet === 'function'
}

function emit(next: DashboardState): void {
  state = next
  listeners.forEach((l) => l())
}

function replaceFrom(id: string, d: LayoutDashboard | null): void {
  emit({
    ready: true,
    layoutId: id,
    panels: panelsFrom(d?.panels),
    groups: groupsFrom(d?.groups),
    version: state.version + 1,
  })
}

// Sends a debounced save now. Called before anything main does with the
// active layout's arrangement (switching, copying it to a new layout).
export function flushDashboardSave(): void {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = null
  const p = pendingSave
  pendingSave = null
  if (p) window.electron?.overlay?.layoutsDashboardSet?.(p.id, p.dashboard).catch(() => {})
}

function scheduleSave(): void {
  if (!hasMain()) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state.panels))
      localStorage.setItem(GROUP_STORAGE_KEY, JSON.stringify(state.groups))
    } catch {
      // localStorage may be unavailable / full — ignore.
    }
    return
  }
  pendingSave = { id: state.layoutId, dashboard: { panels: state.panels, groups: state.groups } }
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(flushDashboardSave, SAVE_DEBOUNCE_MS)
}

async function loadActive(): Promise<void> {
  const o = window.electron.overlay
  let s = await o.layoutsDashboardGet()
  if (!s.migrated) {
    const panels = readLocal(STORAGE_KEY)
    const groups = readLocal(GROUP_STORAGE_KEY)
    const old = panels || groups ? { panels: panelsFrom(panels), groups: groupsFrom(groups) } : null
    s = await o.layoutsMigrateDashboard(old)
    try {
      localStorage.removeItem(STORAGE_KEY)
      localStorage.removeItem(GROUP_STORAGE_KEY)
    } catch {
      // ignore
    }
  }
  replaceFrom(s.id, s.dashboard)
}

function start(): void {
  if (started) return
  started = true
  if (!hasMain()) {
    emit({
      ready: true,
      layoutId: 'local',
      panels: panelsFrom(readLocal(STORAGE_KEY)),
      groups: groupsFrom(readLocal(GROUP_STORAGE_KEY)),
      version: state.version + 1,
    })
    return
  }
  loadActive().catch(() => {})
  window.electron.overlay.onLayoutsChanged((s) => {
    if (s.activeId === state.layoutId) return
    flushDashboardSave()
    loadActive().catch(() => {})
  })
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => { listeners.delete(l) }
}

function update(patch: Partial<Pick<DashboardState, 'panels' | 'groups'>>, remount = false): void {
  if (!state.ready) return
  emit({ ...state, ...patch, version: remount ? state.version + 1 : state.version })
  scheduleSave()
}

export function updateDashboardPanels(fn: (prev: DashboardLayout) => DashboardLayout): void {
  const next = fn(state.panels)
  if (next !== state.panels) update({ panels: next })
}

export function updateDashboardGroups(fn: (prev: GroupPanelLayouts) => GroupPanelLayouts): void {
  const next = fn(state.groups)
  if (next !== state.groups) update({ groups: next })
}

// Back to the default arrangement, for the active layout only. Timer window
// panels go back to their staggered hidden defaults.
export function resetDashboard(): void {
  update({ panels: { ...DEFAULT_DASHBOARD_LAYOUT }, groups: {} }, true)
}

export function useDashboardLayouts(): DashboardState {
  useEffect(start, [])
  return useSyncExternalStore(subscribe, () => state)
}

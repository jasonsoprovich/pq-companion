/**
 * dashboardLayout — localStorage-backed persistence for the Overlays Dashboard
 * panel layout (positions, sizes, visibility). Bumping the version key
 * invalidates older saved shapes.
 */

const STORAGE_KEY = 'pq-overlay-dashboard-layout-v1'

export type DashboardPanelKey = 'buff' | 'detrim' | 'dps' | 'npc' | 'hps' | 'rolls'

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
  hps:    { x: 16,  y: 816, width: 624, height: 384, visible: true },
  rolls:  { x: 656, y: 816, width: 400, height: 384, visible: false },
}

export const DASHBOARD_PANEL_KEYS: DashboardPanelKey[] = ['buff', 'detrim', 'dps', 'npc', 'hps', 'rolls']

export const DASHBOARD_PANEL_LABELS: Record<DashboardPanelKey, string> = {
  buff: 'Buff Timers',
  detrim: 'Detrimental Timers',
  dps: 'DPS Meter',
  npc: 'NPC Overlay',
  hps: 'HPS Meter',
  rolls: 'Roll Tracker',
}

function isPanelLayout(v: unknown): v is PanelLayout {
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

export function loadDashboardLayout(): DashboardLayout {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULT_DASHBOARD_LAYOUT }
    const parsed = JSON.parse(raw) as Partial<Record<DashboardPanelKey, unknown>>
    const merged = { ...DEFAULT_DASHBOARD_LAYOUT }
    for (const key of DASHBOARD_PANEL_KEYS) {
      const candidate = parsed[key]
      if (isPanelLayout(candidate)) merged[key] = candidate
    }
    return merged
  } catch {
    return { ...DEFAULT_DASHBOARD_LAYOUT }
  }
}

export function saveDashboardLayout(layout: DashboardLayout): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(layout))
  } catch {
    // localStorage may be unavailable / full — ignore.
  }
}

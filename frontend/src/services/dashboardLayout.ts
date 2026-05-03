/**
 * dashboardLayout — localStorage-backed persistence for the Overlays Dashboard
 * panel layout (positions, sizes, visibility). Bumping the version key
 * invalidates older saved shapes.
 */

const STORAGE_KEY = 'pq-overlay-dashboard-layout-v1'

export type DashboardPanelKey = 'buff' | 'detrim' | 'dps' | 'npc' | 'hps'

export interface PanelLayout {
  x: number
  y: number
  width: number
  height: number
  visible: boolean
}

export type DashboardLayout = Record<DashboardPanelKey, PanelLayout>

export const DEFAULT_DASHBOARD_LAYOUT: DashboardLayout = {
  buff:   { x: 24,  y: 24,  width: 300, height: 380, visible: true },
  detrim: { x: 344, y: 24,  width: 300, height: 380, visible: true },
  dps:    { x: 24,  y: 424, width: 620, height: 380, visible: true },
  npc:    { x: 664, y: 24,  width: 400, height: 780, visible: true },
  hps:    { x: 24,  y: 820, width: 620, height: 380, visible: true },
}

export const DASHBOARD_PANEL_KEYS: DashboardPanelKey[] = ['buff', 'detrim', 'dps', 'npc', 'hps']

export const DASHBOARD_PANEL_LABELS: Record<DashboardPanelKey, string> = {
  buff: 'Buff Timers',
  detrim: 'Detrimental Timers',
  dps: 'DPS Meter',
  npc: 'NPC Overlay',
  hps: 'HPS Meter',
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

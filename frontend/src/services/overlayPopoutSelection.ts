/**
 * overlayPopoutSelection — localStorage-backed memory of which overlays the
 * user has individually popped out via "Manage overlays". "Pop Out All" (the
 * sidebar quick-toggle and the dashboard's bulk action) reopens exactly this
 * set rather than whatever panels happen to be visible on the Overlays
 * Dashboard — dashboard visibility and popout state are independent, so a
 * panel can be arranged on the dashboard without ever floating as a window,
 * and a popped-out overlay doesn't need a dashboard panel at all.
 *
 * "Close All" doesn't touch this selection — it just closes whatever's
 * currently open — so the next "Pop Out All" restores the same set.
 */
import { VISIBLE_DASHBOARD_PANEL_KEYS, loadDashboardLayout, type DashboardPanelKey } from './dashboardLayout'

const STORAGE_KEY = 'pq-overlay-popout-selection'

export interface PopoutSelection {
  panels: DashboardPanelKey[]
  groups: string[]
}

// First-run default (no selection saved yet): whatever's visible on the
// dashboard, matching the old "Pop Out All" behavior until the user curates
// their own set via Manage overlays.
function defaultSelection(): PopoutSelection {
  const layout = loadDashboardLayout()
  return {
    panels: VISIBLE_DASHBOARD_PANEL_KEYS.filter((k) => layout[k].visible),
    groups: [],
  }
}

export function loadPopoutSelection(): PopoutSelection {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return defaultSelection()
    const parsed = JSON.parse(raw) as Partial<PopoutSelection>
    return {
      panels: Array.isArray(parsed.panels) ? parsed.panels : [],
      groups: Array.isArray(parsed.groups) ? parsed.groups : [],
    }
  } catch {
    return defaultSelection()
  }
}

function saveSelection(sel: PopoutSelection): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(sel))
  } catch {
    // localStorage may be unavailable / full — ignore.
  }
}

export function setPanelInPopoutSelection(panel: DashboardPanelKey, wanted: boolean): void {
  const sel = loadPopoutSelection()
  const has = sel.panels.includes(panel)
  if (has === wanted) return
  saveSelection({
    ...sel,
    panels: wanted ? [...sel.panels, panel] : sel.panels.filter((p) => p !== panel),
  })
}

export function setGroupInPopoutSelection(groupId: string, wanted: boolean): void {
  const sel = loadPopoutSelection()
  const has = sel.groups.includes(groupId)
  if (has === wanted) return
  saveSelection({
    ...sel,
    groups: wanted ? [...sel.groups, groupId] : sel.groups.filter((g) => g !== groupId),
  })
}

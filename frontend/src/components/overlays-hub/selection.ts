/**
 * What the Overlays hub's settings drawer is showing: the all-overlays
 * settings, one overlay, the trigger alert layer, or a named timer window.
 * `panelParam` round-trips it through the /overlays?panel= deep link.
 */
import type { DashboardPanelKey } from '../../services/dashboardLayout'
import { OVERLAY_CATALOG } from '../../lib/overlayCatalog'

export type HubSelection =
  | { kind: 'global' }
  | { kind: 'overlay'; panel: DashboardPanelKey }
  | { kind: 'trigger' }
  | { kind: 'group'; id: string }

export function selectionFromParam(p: string | null): HubSelection | null {
  if (!p) return null
  if (p === 'global') return { kind: 'global' }
  if (p === 'trigger') return { kind: 'trigger' }
  if (p.startsWith('group:')) return { kind: 'group', id: p.slice('group:'.length) }
  const entry = OVERLAY_CATALOG.find((e) => e.panel === p || e.name === p)
  return entry ? { kind: 'overlay', panel: entry.panel } : null
}

export function sameSelection(a: HubSelection | null, b: HubSelection): boolean {
  if (!a || a.kind !== b.kind) return false
  if (a.kind === 'overlay' && b.kind === 'overlay') return a.panel === b.panel
  if (a.kind === 'group' && b.kind === 'group') return a.id === b.id
  return true
}

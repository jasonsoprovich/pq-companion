/**
 * What the Overlays hub's settings drawer is showing: the all-overlays
 * settings, one overlay, the trigger alert layer, a named timer window, a
 * custom overlay (`custom:<id>`), a chat window (`chat:<id>`), or a stack (by its window key, `stack:<id>`).
 * selectionFromParam reads it from the /overlays?panel= deep link.
 */
import type { DashboardPanelKey } from '../../services/dashboardLayout'
import { OVERLAY_CATALOG } from '../../lib/overlayCatalog'

export type HubSelection =
  | { kind: 'global'; section?: 'timers' }
  | { kind: 'overlay'; panel: DashboardPanelKey }
  | { kind: 'trigger' }
  | { kind: 'group'; id: string }
  | { kind: 'stack'; key: string }
  | { kind: 'custom'; id: string }
  // A chat window. edit opens its editor; suggest also adds a "lines like
  // this" rule built from that stored line (deep link from the popout).
  | { kind: 'chat'; id: string; edit?: boolean; suggest?: { seq: number; mode: 'hide' | 'keep' } }

export function selectionFromParam(p: string | null, params?: URLSearchParams): HubSelection | null {
  if (!p) return null
  if (p.startsWith('chat:')) {
    const id = p.slice('chat:'.length)
    const seq = Number(params?.get('suggest'))
    const mode = params?.get('mode') === 'keep' ? 'keep' : 'hide'
    return seq > 0 ? { kind: 'chat', id, edit: true, suggest: { seq, mode } } : { kind: 'chat', id, edit: true }
  }
  if (p === 'global') return { kind: 'global' }
  if (p === 'timers') return { kind: 'global', section: 'timers' }
  if (p === 'trigger') return { kind: 'trigger' }
  if (p.startsWith('group:')) return { kind: 'group', id: p.slice('group:'.length) }
  if (p.startsWith('stack:')) return { kind: 'stack', key: p }
  if (p.startsWith('custom:')) return { kind: 'custom', id: p.slice('custom:'.length) }
  const entry = OVERLAY_CATALOG.find((e) => e.panel === p || e.name === p)
  return entry ? { kind: 'overlay', panel: entry.panel } : null
}

export function sameSelection(a: HubSelection | null, b: HubSelection): boolean {
  if (!a || a.kind !== b.kind) return false
  if (a.kind === 'overlay' && b.kind === 'overlay') return a.panel === b.panel
  if (a.kind === 'group' && b.kind === 'group') return a.id === b.id
  if (a.kind === 'stack' && b.kind === 'stack') return a.key === b.key
  if (a.kind === 'custom' && b.kind === 'custom') return a.id === b.id
  if (a.kind === 'chat' && b.kind === 'chat') return a.id === b.id
  return true
}

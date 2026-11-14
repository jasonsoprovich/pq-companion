/**
 * Display helpers for Custom Overlays (user-built widget windows). The
 * backend owns rule evaluation and state; these only decide how that state
 * reads on screen.
 */
import type {
  CustomField,
  CustomOverlay,
  CustomWidget,
  CustomWidgetState,
} from '../types/customOverlay'

export const CUSTOM_OVERLAY_PREFIX = 'customOverlay:'

// A custom overlay counts as idle (for collapse-when-idle) once no rule has
// matched for this long and no countdown is running.
export const CUSTOM_OVERLAY_IDLE_MS = 2 * 60 * 1000

export function customOverlayWindowKey(id: string): string {
  return `${CUSTOM_OVERLAY_PREFIX}${id}`
}

export function widgetState(o: CustomOverlay, w: CustomWidget): CustomWidgetState {
  return o.state.widgets?.[w.id] ?? {}
}

// Collapsed title-bar summary: the first fields widget's values joined with
// " · " ("Robe of the Oracle · CLOSING"), else the first table's row count,
// else null (nothing to show).
export function customOverlaySummary(o: CustomOverlay): string | null {
  for (const w of o.def.widgets) {
    if (w.type !== 'fields') continue
    const values = (w.fields ?? [])
      .map((f) => widgetState(o, w).fields?.[f.key]?.trim())
      .filter((v): v is string => !!v)
    if (values.length > 0) return values.join(' · ')
    break
  }
  for (const w of o.def.widgets) {
    if (w.type !== 'table') continue
    const n = widgetState(o, w).rows?.length ?? 0
    if (n > 0) return `${n} ${n === 1 ? 'row' : 'rows'}`
    break
  }
  return null
}

// Active = a rule matched within the idle window, or a countdown is running.
// nowMs is server-adjusted (see useCustomOverlays' skew).
export function customOverlayActive(o: CustomOverlay, nowMs: number): boolean {
  if (o.updated_at > 0 && nowMs - o.updated_at < CUSTOM_OVERLAY_IDLE_MS) return true
  return o.def.widgets.some(
    (w) => w.type === 'countdown' && (widgetState(o, w).ends_at ?? 0) > nowMs,
  )
}

// A running countdown's end, the soonest of several, or 0.
export function nextCountdownEnd(o: CustomOverlay, nowMs: number): number {
  let soonest = 0
  for (const w of o.def.widgets) {
    if (w.type !== 'countdown') continue
    const end = widgetState(o, w).ends_at ?? 0
    if (end > nowMs && (soonest === 0 || end < soonest)) soonest = end
  }
  return soonest
}

// The color of a field value: the first color rule whose text the value
// contains (case-insensitive), or undefined for the default.
export function fieldColor(field: CustomField, value: string): string | undefined {
  const v = value.toLowerCase()
  for (const r of field.colors ?? []) {
    if (r.contains && v.includes(r.contains.toLowerCase())) return r.color || undefined
  }
  return undefined
}

// "0:12", "4:05", "1:02:03".
export function fmtCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const ss = String(s).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

// "21:04" for a feed line's time.
export function fmtFeedTime(ts: number): string {
  const d = new Date(ts)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

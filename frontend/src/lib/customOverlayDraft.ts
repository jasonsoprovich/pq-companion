/**
 * Pure helpers for the Custom Overlay editor's draft: new keys, what each
 * action type does and which widget it targets, pruning actions when a
 * widget/field/column is deleted, and a display mirror of the backend's
 * Simple-mode pattern builder (backend/internal/customoverlay/simple.go).
 */
import type {
  CustomOp,
  CustomOpType,
  CustomOverlayDef,
  CustomRule,
  CustomSimpleMatch,
  CustomWidget,
  CustomWidgetType,
} from '../types/customOverlay'

// A short random key matching the backend's ^[A-Za-z0-9_-]{1,32}$. Users
// never see these; labels are what they edit.
export function newKey(prefix: string): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}`
}

export const WIDGET_TYPES: { type: CustomWidgetType; label: string; hint: string }[] = [
  { type: 'fields', label: 'Fields', hint: 'Named values, like Item and Status' },
  { type: 'table', label: 'Table', hint: 'Rows by name, like each bidder’s bid' },
  { type: 'feed', label: 'Feed', hint: 'The last few lines, newest first' },
  { type: 'counter', label: 'Counter', hint: 'One number' },
  { type: 'countdown', label: 'Countdown', hint: 'A timer a rule starts' },
]

export function widgetTypeLabel(t: CustomWidgetType): string {
  return WIDGET_TYPES.find((w) => w.type === t)?.label ?? t
}

export function newWidget(type: CustomWidgetType): CustomWidget {
  const id = newKey(type)
  switch (type) {
    case 'fields':
      return { id, type, fields: [{ key: newKey('f'), label: 'Status' }] }
    case 'table':
      return {
        id,
        type,
        title: 'Table',
        columns: [{ key: newKey('c'), label: 'Name' }, { key: newKey('c'), label: 'Value' }],
      }
    case 'feed':
      return { id, type, title: 'Recent', max_lines: 5 }
    case 'counter':
      return { id, type, title: 'Count' }
    case 'countdown':
      return { id, type, title: 'Timer', seconds: 60 }
  }
}

// A widget's name in dropdowns: its title, else its first field's label,
// else its type.
export function widgetName(w: CustomWidget): string {
  if (w.title) return w.title
  if (w.type === 'fields' && w.fields?.length) return w.fields.map((f) => f.label).join(' / ')
  return widgetTypeLabel(w.type)
}

export interface OpMeta {
  type: CustomOpType
  label: string
  // Widget type the action targets; null = any widget; undefined = none.
  widget?: CustomWidgetType | null
}

export const OP_TYPES: OpMeta[] = [
  { type: 'set_field', label: 'Set a field', widget: 'fields' },
  { type: 'clear_field', label: 'Clear a field', widget: 'fields' },
  { type: 'upsert_row', label: 'Add or update a table row', widget: 'table' },
  { type: 'remove_row', label: 'Remove a table row', widget: 'table' },
  { type: 'add_line', label: 'Add a line to a feed', widget: 'feed' },
  { type: 'counter_add', label: 'Add to a counter', widget: 'counter' },
  { type: 'counter_set', label: 'Set a counter', widget: 'counter' },
  { type: 'countdown_start', label: 'Start a countdown', widget: 'countdown' },
  { type: 'countdown_stop', label: 'Stop a countdown', widget: 'countdown' },
  { type: 'reset_widget', label: 'Clear a widget', widget: null },
  { type: 'reset_all', label: 'Clear everything' },
]

export function opMeta(t: CustomOpType): OpMeta {
  return OP_TYPES.find((m) => m.type === t) ?? { type: t, label: t }
}

export function widgetsForOp(t: CustomOpType, widgets: CustomWidget[]): CustomWidget[] {
  const m = opMeta(t)
  if (m.widget === undefined) return []
  return m.widget === null ? widgets : widgets.filter((w) => w.type === m.widget)
}

// A fresh action of type t aimed at the first widget it can target.
export function newOp(t: CustomOpType, widgets: CustomWidget[]): CustomOp {
  const w = widgetsForOp(t, widgets)[0]
  const op: CustomOp = { type: t }
  if (w) op.widget = w.id
  if (t === 'set_field' && w?.fields?.length) op.field = w.fields[0].key
  if (t === 'counter_add') op.amount = 1
  if (t === 'upsert_row') { op.key = '{speaker}'; op.cells = {} }
  return op
}

// The first action type that has a widget to target in this draft.
export function defaultOpType(widgets: CustomWidget[]): CustomOpType {
  return OP_TYPES.find((m) => m.widget !== undefined && widgetsForOp(m.type, widgets).length > 0)?.type ?? 'reset_all'
}

export interface PruneResult {
  def: CustomOverlayDef
  removedOps: number
  removedRules: number
}

// Applies keep() to every action, then drops rules left with none. Used when
// a widget, field or column goes away so the draft still validates.
function pruneOps(
  def: CustomOverlayDef,
  map: (op: CustomOp) => CustomOp | null,
): PruneResult {
  let removedOps = 0
  let removedRules = 0
  const rules: CustomRule[] = []
  for (const r of def.rules) {
    const ops: CustomOp[] = []
    for (const op of r.ops) {
      const next = map(op)
      if (next) ops.push(next)
      else removedOps++
    }
    if (ops.length === 0 && r.ops.length > 0) { removedRules++; continue }
    rules.push({ ...r, ops })
  }
  return { def: { ...def, rules }, removedOps, removedRules }
}

export function removeWidget(def: CustomOverlayDef, widgetId: string): PruneResult {
  const res = pruneOps(def, (op) => (op.widget === widgetId ? null : op))
  res.def = { ...res.def, widgets: def.widgets.filter((w) => w.id !== widgetId) }
  return res
}

export function removeField(def: CustomOverlayDef, widgetId: string, fieldKey: string): PruneResult {
  const res = pruneOps(def, (op) =>
    op.widget === widgetId && (op.type === 'set_field' || op.type === 'clear_field') && op.field === fieldKey
      ? null
      : op,
  )
  res.def = {
    ...res.def,
    widgets: def.widgets.map((w) =>
      w.id === widgetId ? { ...w, fields: (w.fields ?? []).filter((f) => f.key !== fieldKey) } : w,
    ),
  }
  return res
}

// A column's cells come out of upserts; an action left with nothing to write
// is kept (its row key still records the row). Sort and merge settings that
// named the column fall back to their defaults.
export function removeColumn(def: CustomOverlayDef, widgetId: string, colKey: string): PruneResult {
  const res = pruneOps(def, (op) => {
    if (op.widget !== widgetId || !op.cells || !(colKey in op.cells)) return op
    const cells = { ...op.cells }
    delete cells[colKey]
    return { ...op, cells }
  })
  res.def = {
    ...res.def,
    widgets: def.widgets.map((w) => {
      if (w.id !== widgetId) return w
      const next: CustomWidget = { ...w, columns: (w.columns ?? []).filter((c) => c.key !== colKey) }
      if (next.sort_by === colKey) { next.sort_by = ''; next.sort_desc = false }
      if (next.merge_column === colKey) { next.merge = 'replace'; next.merge_column = '' }
      return next
    }),
  }
  return res
}

export function pruneNote(what: string, r: PruneResult): string {
  if (r.removedOps === 0) return ''
  const parts = [`${r.removedOps} ${r.removedOps === 1 ? 'action' : 'actions'}`]
  if (r.removedRules > 0) parts.push(`${r.removedRules} ${r.removedRules === 1 ? 'rule' : 'rules'} left empty`)
  return `Removed ${what} and ${parts.join(' and ')} that used it.`
}

// ── Simple mode ──────────────────────────────────────────────────────────────

const PLACEHOLDER_RE = /\{([A-Za-z_][A-Za-z0-9_]*)(?::([A-Za-z]+))?\}/g

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/ +/g, '\\s+')
}

// Mirrors simplePattern in simple.go, for showing the regex a Simple rule
// becomes (and seeding Regex mode). The backend remains the authority.
export function simplePattern(m: CustomSimpleMatch): string {
  const text = m.text.trim()
  if (!text) return ''
  let out = '(?i)'
  if (m.kind !== 'contains') out += '^'
  const matches = [...text.matchAll(PLACEHOLDER_RE)]
  let last = 0
  matches.forEach((mt, i) => {
    const start = mt.index ?? 0
    out += escapeRe(text.slice(last, start))
    const typ = (mt[2] ?? 'text').toLowerCase()
    const end = start + mt[0].length
    const expr =
      typ === 'number' ? '\\d[\\d,]*'
        : typ === 'word' ? '\\S+'
          : i === matches.length - 1 && end === text.length ? '.+' : '.+?'
    out += `(?P<${mt[1]}>${expr})`
    last = end
  })
  out += escapeRe(text.slice(last))
  if (m.kind === 'exact') out += '$'
  return out
}

// Capture names a rule offers as {name} tokens: Simple placeholders, or a
// regex's named groups plus {1}…{n} for its groups.
export function ruleTokens(r: CustomRule): string[] {
  if (r.simple) return [...r.simple.text.matchAll(PLACEHOLDER_RE)].map((m) => m[1])
  const p = r.pattern ?? ''
  const names = [...p.matchAll(/\(\?P?<([A-Za-z_][A-Za-z0-9_]*)>/g)].map((m) => m[1])
  // Go numbers named groups too; skip (?:…) and (?i) flag groups.
  const groups = [...p.matchAll(/(?<!\\)\((?!\?:|\?[a-zA-Z]*\))/g)].length
  const numbered = Array.from({ length: Math.min(groups, 9) }, (_, i) => String(i + 1))
  return [...names, ...numbered]
}

// Tokens every rule can use, besides its own captures.
export const LINE_TOKENS: { token: string; hint: string; chatOnly?: boolean }[] = [
  { token: 'speaker', hint: 'who said it', chatOnly: true },
  { token: 'message', hint: 'what they said', chatOnly: true },
  { token: 'channel', hint: 'the chat channel', chatOnly: true },
  { token: 'line', hint: 'the whole log line' },
  { token: 'c', hint: 'your character' },
]

export const CHANNELS: { value: string; label: string }[] = [
  { value: '', label: 'Any log line' },
  { value: 'any', label: 'Any chat' },
  { value: 'guild', label: 'Guild' },
  { value: 'raid', label: 'Raid' },
  { value: 'group', label: 'Group' },
  { value: 'tell', label: 'Tells' },
  { value: 'say', label: 'Say' },
  { value: 'ooc', label: 'OOC' },
  { value: 'shout', label: 'Shout' },
  { value: 'auction', label: 'Auction' },
]

export function isNamedChannel(c: string | undefined): boolean {
  return !!c && !CHANNELS.some((x) => x.value === c)
}

export function newRule(widgets: CustomWidget[]): CustomRule {
  return {
    id: newKey('r'),
    name: '',
    enabled: true,
    simple: { kind: 'starts', text: '' },
    ops: [newOp(defaultOpType(widgets), widgets)],
  }
}

export function cloneDef(d: CustomOverlayDef): CustomOverlayDef {
  return JSON.parse(JSON.stringify(d)) as CustomOverlayDef
}

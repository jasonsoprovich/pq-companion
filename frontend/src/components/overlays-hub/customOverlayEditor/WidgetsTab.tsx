/**
 * Widgets tab: the blocks the overlay shows, top to bottom. Keys are made
 * up when a widget, field or column is added and never shown; deleting one
 * also removes the rule actions that wrote to it (the editor reports how
 * many).
 */
import React from 'react'
import { X } from 'lucide-react'
import type {
  CustomColorRule,
  CustomMergeMode,
  CustomOverlayDef,
  CustomWidget,
} from '../../../types/customOverlay'
import {
  WIDGET_TYPES,
  newKey,
  newWidget,
  pruneNote,
  removeColumn,
  removeField,
  removeWidget,
  widgetTypeLabel,
} from '../../../lib/customOverlayDraft'
import {
  AddButton,
  Box,
  Hint,
  IconButton,
  ListControls,
  NumberInput,
  Row,
  Select,
  TextInput,
  inputStyle,
  move,
} from './fields'

const MERGE_OPTIONS: { value: CustomMergeMode; label: string }[] = [
  { value: 'replace', label: 'Replace the row' },
  { value: 'highest', label: 'Keep the highest' },
  { value: 'add', label: 'Add them up' },
]

export default function WidgetsTab({
  draft,
  setDraft,
  setNote,
}: {
  draft: CustomOverlayDef
  setDraft: (d: CustomOverlayDef) => void
  setNote: (n: string) => void
}): React.ReactElement {
  const patchWidget = (id: string, patch: Partial<CustomWidget>): void =>
    setDraft({ ...draft, widgets: draft.widgets.map((w) => (w.id === id ? { ...w, ...patch } : w)) })

  const deleteWidget = (w: CustomWidget): void => {
    const r = removeWidget(draft, w.id)
    setDraft(r.def)
    setNote(pruneNote(`the ${widgetTypeLabel(w.type).toLowerCase()} widget`, r))
  }

  return (
    <div className="flex flex-col gap-3">
      <Hint>Widgets show top to bottom. Rules fill them in from the log.</Hint>
      {draft.widgets.map((w, i) => (
        <Box key={w.id}>
          <div className="flex items-center gap-2">
            <span
              className="shrink-0 rounded px-1.5 py-px text-[10px] font-semibold uppercase tracking-wide"
              style={{ backgroundColor: '#a78bfa22', color: '#a78bfa' }}
            >
              {widgetTypeLabel(w.type)}
            </span>
            <TextInput
              value={w.title ?? ''}
              onChange={(v) => patchWidget(w.id, { title: v })}
              placeholder={w.type === 'fields' ? 'Title (optional)' : 'Title'}
              className="flex-1"
            />
            <ListControls
              index={i}
              count={draft.widgets.length}
              what="widget"
              onMove={(d) => setDraft({ ...draft, widgets: move(draft.widgets, i, d) })}
              onDelete={() => deleteWidget(w)}
            />
          </div>
          <WidgetBody w={w} draft={draft} setDraft={setDraft} setNote={setNote} patch={(p) => patchWidget(w.id, p)} />
        </Box>
      ))}
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Add:</span>
        {WIDGET_TYPES.map((t) => (
          <AddButton
            key={t.type}
            label={t.label}
            title={t.hint}
            onClick={() => setDraft({ ...draft, widgets: [...draft.widgets, newWidget(t.type)] })}
          />
        ))}
      </div>
    </div>
  )
}

function WidgetBody({
  w,
  draft,
  setDraft,
  setNote,
  patch,
}: {
  w: CustomWidget
  draft: CustomOverlayDef
  setDraft: (d: CustomOverlayDef) => void
  setNote: (n: string) => void
  patch: (p: Partial<CustomWidget>) => void
}): React.ReactElement | null {
  switch (w.type) {
    case 'fields': {
      const fields = w.fields ?? []
      return (
        <div className="flex flex-col gap-2">
          {fields.map((f, i) => (
            <div key={f.key} className="flex flex-col gap-1.5 rounded p-2" style={{ backgroundColor: 'var(--color-surface-2)' }}>
              <div className="flex items-center gap-2">
                <TextInput
                  value={f.label}
                  onChange={(v) => patch({ fields: fields.map((x) => (x.key === f.key ? { ...x, label: v } : x)) })}
                  placeholder="Field label"
                  className="flex-1"
                />
                <ListControls
                  index={i}
                  count={fields.length}
                  what="field"
                  onMove={(d) => patch({ fields: move(fields, i, d) })}
                  onDelete={() => {
                    const r = removeField(draft, w.id, f.key)
                    setDraft(r.def)
                    setNote(pruneNote(`the “${f.label}” field`, r))
                  }}
                />
              </div>
              <ColorRules
                rules={f.colors ?? []}
                onChange={(colors) =>
                  patch({ fields: fields.map((x) => (x.key === f.key ? { ...x, colors } : x)) })
                }
              />
            </div>
          ))}
          <div>
            <AddButton
              label="Field"
              onClick={() => patch({ fields: [...fields, { key: newKey('f'), label: '' }] })}
            />
          </div>
        </div>
      )
    }
    case 'table': {
      const cols = w.columns ?? []
      const colOptions = cols.map((c, i) => ({ value: c.key, label: c.label || `Column ${i + 1}` }))
      return (
        <div className="flex flex-col gap-2">
          <Row label="Columns">
            {cols.map((c, i) => (
              <span key={c.key} className="flex items-center gap-0.5">
                <TextInput
                  value={c.label}
                  onChange={(v) => patch({ columns: cols.map((x) => (x.key === c.key ? { ...x, label: v } : x)) })}
                  placeholder={i === 0 ? 'Row name' : 'Column'}
                  className="w-24"
                />
                {i > 0 && (
                  <IconButton
                    icon={X}
                    title={`Delete the ${c.label || 'column'} column`}
                    onClick={() => {
                      const r = removeColumn(draft, w.id, c.key)
                      setDraft(r.def)
                      setNote(pruneNote(`the “${c.label}” column`, r))
                    }}
                  />
                )}
              </span>
            ))}
            {cols.length < 8 && (
              <AddButton label="Column" onClick={() => patch({ columns: [...cols, { key: newKey('c'), label: '' }] })} />
            )}
          </Row>
          <Hint>The first column names the row (a bidder, a player…). A rule that writes the same name again updates that row.</Hint>
          <Row label="Sort by">
            <Select
              value={w.sort_by ?? ''}
              options={[{ value: '', label: 'Arrival order' }, ...colOptions]}
              onChange={(v) => patch({ sort_by: v })}
              ariaLabel="Sort by"
            />
            {w.sort_by && (
              <Select
                value={w.sort_desc ? 'desc' : 'asc'}
                options={[{ value: 'desc', label: 'Highest first' }, { value: 'asc', label: 'Lowest first' }]}
                onChange={(v) => patch({ sort_desc: v === 'desc' })}
                ariaLabel="Sort direction"
              />
            )}
          </Row>
          <Row label="Same name again">
            <Select
              value={w.merge || 'replace'}
              options={MERGE_OPTIONS}
              onChange={(v) =>
                patch({
                  merge: v,
                  merge_column: v === 'replace' ? '' : w.merge_column || cols[cols.length - 1]?.key || '',
                })
              }
              ariaLabel="When a row's name comes up again"
            />
            {w.merge && w.merge !== 'replace' && (
              <>
                <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>of</span>
                <Select
                  value={w.merge_column ?? ''}
                  options={colOptions.slice(1)}
                  onChange={(v) => patch({ merge_column: v })}
                  ariaLabel="Merge column"
                />
              </>
            )}
          </Row>
          <Row label="Max rows">
            <NumberInput value={w.max_rows} onChange={(v) => patch({ max_rows: v })} min={1} max={200} placeholder="50" />
          </Row>
        </div>
      )
    }
    case 'feed':
      return (
        <Row label="Lines kept">
          <NumberInput value={w.max_lines} onChange={(v) => patch({ max_lines: v })} min={1} max={200} placeholder="20" />
        </Row>
      )
    case 'countdown':
      return (
        <Row label="Seconds">
          <NumberInput value={w.seconds} onChange={(v) => patch({ seconds: v })} min={0} placeholder="60" />
          <Hint>Used when a rule starts it without its own duration.</Hint>
        </Row>
      )
    default:
      return null
  }
}

function ColorRules({
  rules,
  onChange,
}: {
  rules: CustomColorRule[]
  onChange: (r: CustomColorRule[]) => void
}): React.ReactElement {
  const set = (i: number, p: Partial<CustomColorRule>): void =>
    onChange(rules.map((r, j) => (j === i ? { ...r, ...p } : r)))
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {rules.map((r, i) => (
        <span key={i} className="flex items-center gap-1">
          <input
            type="color"
            value={r.color || '#4ade80'}
            onChange={(e) => set(i, { color: e.target.value })}
            aria-label="Color"
            className="h-6 w-6 cursor-pointer rounded"
            style={{ ...inputStyle, padding: 1 }}
          />
          <TextInput value={r.contains} onChange={(v) => set(i, { contains: v })} placeholder="when it contains…" className="w-32" />
          <IconButton icon={X} title="Remove color" onClick={() => onChange(rules.filter((_, j) => j !== i))} />
        </span>
      ))}
      <button
        onClick={() => onChange([...rules, { contains: '', color: '#4ade80' }])}
        className="text-[11px] underline"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        + color when…
      </button>
    </div>
  )
}

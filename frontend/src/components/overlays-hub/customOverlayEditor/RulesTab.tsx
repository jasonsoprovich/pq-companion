/**
 * Rules tab: each rule says where to look (any line, or one chat channel),
 * what to match (Simple text with {placeholders}, or a regex), and what to do
 * on a match. Token chips insert into whichever action text box was focused
 * last.
 */
import React, { useRef } from 'react'
import type {
  CustomOp,
  CustomOpType,
  CustomOverlayDef,
  CustomRule,
  CustomSimpleKind,
  CustomWidget,
} from '../../../types/customOverlay'
import {
  CHANNELS,
  LINE_TOKENS,
  OP_TYPES,
  defaultOpType,
  isNamedChannel,
  newOp,
  newRule,
  opMeta,
  ruleTokens,
  simplePattern,
  widgetName,
  widgetsForOp,
} from '../../../lib/customOverlayDraft'
import { AddButton, Box, Hint, IconButton, ListControls, NumberInput, Row, Select, TextInput, move } from './fields'
import { Trash2 } from 'lucide-react'

const SIMPLE_KINDS: { value: CustomSimpleKind; label: string }[] = [
  { value: 'starts', label: 'Starts with' },
  { value: 'contains', label: 'Contains' },
  { value: 'exact', label: 'Is exactly' },
]

const NAMED = '__named__'

export default function RulesTab({
  draft,
  setDraft,
  matchedCounts,
}: {
  draft: CustomOverlayDef
  setDraft: (d: CustomOverlayDef) => void
  // Rule id → lines matched in the last test run, when there is one.
  matchedCounts: Record<string, number> | null
}): React.ReactElement {
  const patchRule = (id: string, patch: Partial<CustomRule>): void =>
    setDraft({ ...draft, rules: draft.rules.map((r) => (r.id === id ? { ...r, ...patch } : r)) })

  return (
    <div className="flex flex-col gap-3">
      <Hint>
        Every rule that matches a line runs, in this order. Pick a chat channel to match just what
        was said; leave it on “Any log line” to match the whole line.
      </Hint>
      {draft.rules.length === 0 && (
        <p className="py-4 text-center text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          No rules yet, so nothing will fill this overlay.
        </p>
      )}
      {draft.rules.map((r, i) => (
        <RuleCard
          key={r.id}
          rule={r}
          index={i}
          count={draft.rules.length}
          widgets={draft.widgets}
          matched={matchedCounts ? matchedCounts[r.id] ?? 0 : null}
          patch={(p) => patchRule(r.id, p)}
          onMove={(d) => setDraft({ ...draft, rules: move(draft.rules, i, d) })}
          onDelete={() => setDraft({ ...draft, rules: draft.rules.filter((x) => x.id !== r.id) })}
        />
      ))}
      <div>
        <AddButton label="Rule" onClick={() => setDraft({ ...draft, rules: [...draft.rules, newRule(draft.widgets)] })} />
      </div>
    </div>
  )
}

interface FocusTarget {
  el: HTMLInputElement
  set: (v: string) => void
}

function RuleCard({
  rule: r,
  index,
  count,
  widgets,
  matched,
  patch,
  onMove,
  onDelete,
}: {
  rule: CustomRule
  index: number
  count: number
  widgets: CustomWidget[]
  matched: number | null
  patch: (p: Partial<CustomRule>) => void
  onMove: (d: -1 | 1) => void
  onDelete: () => void
}): React.ReactElement {
  const focus = useRef<FocusTarget | null>(null)
  const named = isNamedChannel(r.channel)
  const chat = !!r.channel

  const insertToken = (token: string): void => {
    const t = focus.current
    if (!t || !t.el.isConnected) return
    const v = t.el.value
    const start = t.el.selectionStart ?? v.length
    const end = t.el.selectionEnd ?? v.length
    const text = `{${token}}`
    t.set(v.slice(0, start) + text + v.slice(end))
    requestAnimationFrame(() => {
      t.el.focus()
      t.el.setSelectionRange(start + text.length, start + text.length)
    })
  }

  const setOps = (ops: CustomOp[]): void => patch({ ops })
  const tokens = [
    ...ruleTokens(r).map((t) => ({ token: t, hint: 'from the match' })),
    ...LINE_TOKENS.filter((t) => chat || !t.chatOnly),
  ]

  return (
    <Box>
      <div className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={r.enabled}
          onChange={(e) => patch({ enabled: e.target.checked })}
          title={r.enabled ? 'Rule on' : 'Rule off'}
          aria-label="Rule on"
        />
        <TextInput value={r.name ?? ''} onChange={(v) => patch({ name: v })} placeholder={`Rule ${index + 1} name`} className="flex-1" />
        {matched !== null && (
          <span
            className="shrink-0 text-[10px] font-semibold"
            style={{ color: matched > 0 ? 'var(--color-success)' : 'var(--color-muted-foreground)' }}
            title="Lines this rule matched in the last test"
          >
            {matched > 0 ? `${matched} matched` : 'no matches'}
          </span>
        )}
        <ListControls index={index} count={count} what="rule" onMove={onMove} onDelete={onDelete} />
      </div>

      <Row label="Look in">
        <Select
          value={named ? NAMED : r.channel ?? ''}
          options={[...CHANNELS, { value: NAMED, label: 'Named channel…' }]}
          onChange={(v) => {
            if (v === NAMED) { patch({ channel: 'dkp' }); return }
            patch(v ? { channel: v } : { channel: '', direction: '', speakers: [] })
          }}
          ariaLabel="Look in"
        />
        {named && (
          <TextInput
            value={r.channel ?? ''}
            onChange={(v) => patch({ channel: v.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
            placeholder="channel name"
            className="w-28"
          />
        )}
        {chat && (
          <Select
            value={r.direction ?? ''}
            options={[
              { value: '', label: 'From anyone' },
              { value: 'in', label: 'From others' },
              { value: 'out', label: 'From me' },
            ]}
            onChange={(v) => patch({ direction: v })}
            ariaLabel="Who said it"
          />
        )}
      </Row>
      {chat && r.direction !== 'out' && (
        <Row label="Only from">
          <TextInput
            value={(r.speakers ?? []).join(', ')}
            onChange={(v) => patch({ speakers: v.split(',').map((s) => s.trimStart()) })}
            placeholder="anyone · or names, comma separated"
            className="flex-1"
          />
        </Row>
      )}

      <Row label="Match">
        <Select
          value={r.simple ? 'simple' : 'regex'}
          options={[{ value: 'simple', label: 'Text' }, { value: 'regex', label: 'Regex' }]}
          onChange={(v) => {
            if (v === 'regex' && r.simple) patch({ pattern: simplePattern(r.simple), simple: undefined })
            if (v === 'simple' && !r.simple) patch({ simple: { kind: 'starts', text: '' } })
          }}
          ariaLabel="Match mode"
        />
        {r.simple ? (
          <>
            <Select
              value={r.simple.kind}
              options={SIMPLE_KINDS}
              onChange={(k) => patch({ simple: { ...r.simple!, kind: k } })}
              ariaLabel="Match kind"
            />
            <TextInput
              value={r.simple.text}
              onChange={(t) => patch({ simple: { ...r.simple!, text: t } })}
              placeholder={chat ? 'Bids open on {item}' : 'You have entered {zone}'}
              className="flex-1"
            />
          </>
        ) : (
          <TextInput
            value={r.pattern ?? ''}
            onChange={(p) => patch({ pattern: p })}
            placeholder={chat ? 'blank = every message' : '^You have entered (?P<zone>.+)\\.$'}
            mono
            className="flex-1"
          />
        )}
      </Row>
      {r.simple ? (
        <Hint>
          Not case sensitive. Put <code>{'{name}'}</code> where the text changes; use{' '}
          <code>{'{name:number}'}</code> for a number or <code>{'{name:word}'}</code> for one word.
          {chat ? ' Matches what was said, not the speaker or channel.' : ''}
        </Hint>
      ) : (
        <Hint>
          Go RE2 syntax (no lookaround). Name a group with <code>{'(?P<name>…)'}</code> to use it as{' '}
          <code>{'{name}'}</code>.
        </Hint>
      )}

      <div className="flex flex-col gap-1.5">
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Then</span>
        {r.ops.map((op, j) => (
          <OpRow
            key={j}
            op={op}
            widgets={widgets}
            onChange={(next) => setOps(r.ops.map((x, k) => (k === j ? next : x)))}
            onDelete={() => setOps(r.ops.filter((_, k) => k !== j))}
            onFocusText={(el, set) => { focus.current = { el, set } }}
          />
        ))}
        {r.ops.length < 10 && (
          <div>
            <AddButton label="Action" onClick={() => setOps([...r.ops, newOp(defaultOpType(widgets), widgets)])} />
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1">
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Insert:</span>
        {tokens.map((t) => (
          <button
            key={t.token}
            // Keep focus in the text box the token goes into.
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => insertToken(t.token)}
            title={`{${t.token}} · ${t.hint}`}
            className="rounded px-1.5 py-px font-mono text-[10px]"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
          >
            {`{${t.token}}`}
          </button>
        ))}
      </div>
    </Box>
  )
}

function OpRow({
  op,
  widgets,
  onChange,
  onDelete,
  onFocusText,
}: {
  op: CustomOp
  widgets: CustomWidget[]
  onChange: (op: CustomOp) => void
  onDelete: () => void
  onFocusText: (el: HTMLInputElement, set: (v: string) => void) => void
}): React.ReactElement {
  const targets = widgetsForOp(op.type, widgets)
  const w = widgets.find((x) => x.id === op.widget)
  const meta = opMeta(op.type)
  const set = (p: Partial<CustomOp>): void => onChange({ ...op, ...p })

  const text = (
    value: string,
    onText: (v: string) => void,
    placeholder: string,
    className = 'flex-1',
  ): React.ReactElement => (
    <TextInput
      value={value}
      onChange={onText}
      placeholder={placeholder}
      className={className}
      onFocus={(e) => onFocusText(e.currentTarget, onText)}
    />
  )

  let body: React.ReactNode = null
  switch (op.type) {
    case 'set_field':
    case 'clear_field': {
      const fields = w?.fields ?? []
      const opts = fields.map((f, i) => ({ value: f.key, label: f.label || `Field ${i + 1}` }))
      body = (
        <>
          <Select
            value={op.field ?? ''}
            options={op.type === 'clear_field' ? [{ value: '', label: 'All fields' }, ...opts] : opts}
            onChange={(v) => set({ field: v })}
            ariaLabel="Field"
          />
          {op.type === 'set_field' && (
            <>
              <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>to</span>
              {text(op.value ?? '', (v) => set({ value: v }), '{item}')}
            </>
          )}
        </>
      )
      break
    }
    case 'upsert_row':
    case 'remove_row': {
      const cols = w?.columns ?? []
      body = (
        <div className="flex w-full flex-col gap-1.5">
          <div className="flex items-center gap-1.5">
            <span className="w-20 shrink-0 truncate text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              {cols[0]?.label || 'Row name'}
            </span>
            {text(op.key ?? '', (v) => set({ key: v }), '{speaker}')}
          </div>
          {op.type === 'upsert_row' &&
            cols.slice(1).map((c) => (
              <div key={c.key} className="flex items-center gap-1.5">
                <span className="w-20 shrink-0 truncate text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                  {c.label || 'Column'}
                </span>
                {text(
                  op.cells?.[c.key] ?? '',
                  (v) => {
                    const cells = { ...(op.cells ?? {}) }
                    if (v) cells[c.key] = v
                    else delete cells[c.key]
                    set({ cells })
                  },
                  'empty',
                )}
              </div>
            ))}
        </div>
      )
      break
    }
    case 'add_line':
      body = text(op.value ?? '', (v) => set({ value: v }), '{speaker} bid {amount}')
      break
    case 'counter_add':
      body = (
        <>
          <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>by</span>
          <NumberInput value={op.amount} onChange={(v) => set({ amount: v ?? 0 })} placeholder="1" ariaLabel="Amount" />
        </>
      )
      break
    case 'counter_set':
      body = text(op.value ?? '', (v) => set({ value: v }), '0 or {count}')
      break
    case 'countdown_start':
      body = text(
        op.value ?? '',
        (v) => set({ value: v }),
        w?.seconds ? `${w.seconds}s (widget default)` : '90s, 2m, 1:30',
      )
      break
  }

  return (
    <div className="flex flex-col gap-1.5 rounded p-2" style={{ backgroundColor: 'var(--color-surface-2)' }}>
      <div className="flex flex-wrap items-center gap-1.5">
        <Select<CustomOpType>
          value={op.type}
          options={OP_TYPES.map((m) => ({
            value: m.type,
            label: m.widget !== undefined && widgetsForOp(m.type, widgets).length === 0 ? `${m.label} (no widget)` : m.label,
          }))}
          onChange={(t) => onChange(newOp(t, widgets))}
          ariaLabel="Action"
        />
        {meta.widget !== undefined && (
          targets.length > 0 ? (
            <Select
              value={op.widget ?? ''}
              options={targets.map((x) => ({ value: x.id, label: widgetName(x) }))}
              onChange={(v) => {
                const nw = widgets.find((x) => x.id === v)
                set({
                  widget: v,
                  field: op.type === 'set_field' ? nw?.fields?.[0]?.key : op.type === 'clear_field' ? '' : op.field,
                  cells: op.type === 'upsert_row' ? {} : op.cells,
                })
              }}
              ariaLabel="Widget"
            />
          ) : (
            <span className="text-[11px]" style={{ color: 'var(--color-warning)' }}>
              Add a {meta.widget ?? ''} widget first
            </span>
          )
        )}
        {op.type !== 'upsert_row' && op.type !== 'remove_row' && body}
        <span className="flex-1" />
        <IconButton icon={Trash2} title="Delete action" danger onClick={onDelete} />
      </div>
      {(op.type === 'upsert_row' || op.type === 'remove_row') && body}
    </div>
  )
}

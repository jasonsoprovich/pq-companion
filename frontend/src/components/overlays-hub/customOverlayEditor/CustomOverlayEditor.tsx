/**
 * The Custom Overlay editor: a wide modal over the hub with Widgets / Rules /
 * Test / JSON tabs and a live preview. Edits stay in a draft until Save;
 * Discard (or Esc) drops them, asking first when there are any.
 *
 * The draft is checked against the backend's validator as it changes (a test
 * run with no lines), so problems show before Save. Once a test has been run
 * it re-runs on every change and the preview shows its result instead of
 * the overlay's live contents.
 */
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { LayoutTemplate } from 'lucide-react'
import type {
  CustomOverlay,
  CustomOverlayDef,
  CustomOverlayTestResult,
} from '../../../types/customOverlay'
import { testCustomOverlayLines, updateCustomOverlay } from '../../../services/api'
import { cloneDef } from '../../../lib/customOverlayDraft'
import CustomOverlayView from '../../overlays/CustomOverlayView'
import { ConfirmModal } from '../../ConfirmModal'
import { useEscapeToClose } from '../../../hooks/useEscapeToClose'
import WidgetsTab from './WidgetsTab'
import RulesTab from './RulesTab'
import { Hint, TextInput, inputStyle } from './fields'

type Tab = 'widgets' | 'rules' | 'test' | 'json'

const TABS: { id: Tab; label: string }[] = [
  { id: 'widgets', label: 'Widgets' },
  { id: 'rules', label: 'Rules' },
  { id: 'test', label: 'Test' },
  { id: 'json', label: 'JSON' },
]

const CHECK_DEBOUNCE_MS = 400

export default function CustomOverlayEditor({
  overlay,
  skewMs,
  initialTab = 'widgets',
  onClose,
}: {
  overlay: CustomOverlay
  skewMs: number
  initialTab?: Tab
  onClose: () => void
}): React.ReactElement {
  const [draft, setDraftState] = useState<CustomOverlayDef>(() => cloneDef(overlay.def))
  const [tab, setTab] = useState<Tab>(initialTab)
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [confirmDiscard, setConfirmDiscard] = useState(false)

  // Test state. testOn turns on after the first Run and keeps re-running.
  const [testText, setTestText] = useState('')
  const testTextRef = useRef(testText)
  testTextRef.current = testText
  const [fromCurrent, setFromCurrent] = useState(false)
  const [testOn, setTestOn] = useState(false)
  const [runNonce, setRunNonce] = useState(0)
  const [check, setCheck] = useState<CustomOverlayTestResult | null>(null)

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(overlay.def), [draft, overlay.def])

  const setDraft = (d: CustomOverlayDef): void => {
    setDraftState(d)
    setNote('')
    setSaveError('')
  }

  useEffect(() => {
    let cancelled = false
    const lines = testOn ? testTextRef.current.split('\n').filter((l) => l.trim()) : []
    const t = setTimeout(() => {
      testCustomOverlayLines(draft, lines, fromCurrent)
        .then((r) => { if (!cancelled) setCheck(r) })
        .catch(() => {})
    }, runNonce > 0 && testOn ? 0 : CHECK_DEBOUNCE_MS)
    return () => { cancelled = true; clearTimeout(t) }
  }, [draft, testOn, fromCurrent, runNonce])

  const requestClose = (): void => {
    if (dirty) setConfirmDiscard(true)
    else onClose()
  }

  // The app's shared Escape stack: this editor sits above the hub drawer,
  // and the discard confirm (its own layer) above it.
  useEscapeToClose(requestClose)

  const save = (): void => {
    setSaving(true)
    setSaveError('')
    updateCustomOverlay(draft)
      .then(() => onClose())
      .catch((e: Error) => { setSaveError(e.message); setSaving(false) })
  }

  const matchedCounts = useMemo(() => {
    if (!testOn || !check || check.error) return null
    const counts: Record<string, number> = {}
    for (const s of check.steps) for (const id of s.matched) counts[id] = (counts[id] ?? 0) + 1
    return counts
  }, [testOn, check])

  const showingTest = testOn && !!check && !check.error
  const preview: CustomOverlay = {
    def: draft,
    state: showingTest ? check!.state : overlay.state,
    updated_at: overlay.updated_at,
  }
  const problem = saveError || check?.error || ''

  return (
    <div
      onMouseDown={(e) => { if (e.target === e.currentTarget) requestClose() }}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1050,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        role="dialog"
        aria-label={`Edit ${overlay.def.name}`}
        className="flex flex-col overflow-hidden rounded-lg"
        style={{
          backgroundColor: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 1180,
          height: 'min(90vh, 860px)',
        }}
      >
        {/* Header */}
        <div
          className="flex shrink-0 items-center gap-2.5 border-b px-4 py-2.5"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <LayoutTemplate size={18} style={{ color: '#a78bfa' }} />
          <TextInput
            value={draft.name}
            onChange={(v) => setDraft({ ...draft, name: v })}
            placeholder="Overlay name"
            ariaLabel="Overlay name"
            className="w-64 !text-sm font-semibold"
          />
          <div className="ml-2 flex gap-1">
            {TABS.map((t) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="rounded px-2.5 py-1 text-xs"
                style={{
                  backgroundColor: tab === t.id ? 'var(--color-surface-2)' : 'transparent',
                  color: tab === t.id ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                  border: `1px solid ${tab === t.id ? 'var(--color-border)' : 'transparent'}`,
                  fontWeight: tab === t.id ? 600 : 400,
                }}
              >
                {t.label}
                {t.id === 'test' && testOn ? ' •' : ''}
              </button>
            ))}
          </div>
        </div>

        {/* Body */}
        <div className="flex min-h-0 flex-1">
          <div className="min-w-0 flex-1 overflow-y-auto p-4">
            {note && (
              <p
                className="mb-3 rounded px-2 py-1.5 text-xs"
                style={{ backgroundColor: 'color-mix(in srgb, var(--color-warning) 12%, transparent)', color: 'var(--color-warning)' }}
              >
                {note}
              </p>
            )}
            {tab === 'widgets' && <WidgetsTab draft={draft} setDraft={setDraft} setNote={setNote} />}
            {tab === 'rules' && <RulesTab draft={draft} setDraft={setDraft} matchedCounts={matchedCounts} />}
            {tab === 'test' && (
              <TestTab
                draft={draft}
                text={testText}
                setText={setTestText}
                fromCurrent={fromCurrent}
                setFromCurrent={setFromCurrent}
                testOn={testOn}
                result={testOn ? check : null}
                onRun={() => { setTestOn(true); setRunNonce((n) => n + 1) }}
                onClear={() => { setTestOn(false); setRunNonce(0) }}
              />
            )}
            {tab === 'json' && <JsonTab draft={draft} setDraft={setDraft} />}
          </div>

          {/* Preview */}
          <div
            className="flex w-[320px] shrink-0 flex-col gap-2 border-l p-3"
            style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
          >
            <div className="flex items-center gap-2">
              <span className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted-foreground)' }}>
                Preview
              </span>
              <span className="flex-1" />
              <span className="text-[10px]" style={{ color: showingTest ? '#a78bfa' : 'var(--color-muted-foreground)' }}>
                {showingTest ? 'Test result' : 'Live contents'}
              </span>
            </div>
            <div
              className="flex min-h-0 flex-col overflow-hidden rounded"
              style={{
                border: '1px solid var(--color-border)',
                backgroundColor: 'var(--color-background)',
                height: 400,
                maxHeight: '100%',
              }}
            >
              <CustomOverlayView theme="panel" overlay={preview} skewMs={skewMs} />
            </div>
            <Hint>
              {showingTest
                ? 'Shows what the test lines produce with your unsaved changes.'
                : 'Run lines on the Test tab to preview your changes against them.'}
            </Hint>
          </div>
        </div>

        {/* Footer */}
        <div
          className="flex shrink-0 items-center gap-3 border-t px-4 py-2.5"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <p className="min-w-0 flex-1 truncate text-xs" style={{ color: problem ? 'var(--color-danger)' : 'var(--color-muted-foreground)' }} title={problem}>
            {problem || (dirty ? 'Unsaved changes' : 'No changes')}
          </p>
          <button
            onClick={requestClose}
            className="rounded px-3 py-1.5 text-sm"
            style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            {dirty ? 'Discard' : 'Close'}
          </button>
          <button
            onClick={save}
            disabled={!dirty || saving}
            className="rounded px-3 py-1.5 text-sm font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              opacity: !dirty || saving ? 0.5 : 1,
              cursor: !dirty || saving ? 'default' : 'pointer',
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
      {confirmDiscard && (
        <ConfirmModal
          title="Discard changes?"
          message="Your edits to this overlay haven’t been saved."
          confirmLabel="Discard"
          tone="danger"
          onConfirm={onClose}
          onCancel={() => setConfirmDiscard(false)}
        />
      )}
    </div>
  )
}

function TestTab({
  draft,
  text,
  setText,
  fromCurrent,
  setFromCurrent,
  testOn,
  result,
  onRun,
  onClear,
}: {
  draft: CustomOverlayDef
  text: string
  setText: (v: string) => void
  fromCurrent: boolean
  setFromCurrent: (v: boolean) => void
  testOn: boolean
  result: CustomOverlayTestResult | null
  onRun: () => void
  onClear: () => void
}): React.ReactElement {
  const ruleName = (id: string): string => {
    const i = draft.rules.findIndex((r) => r.id === id)
    return i < 0 ? id : draft.rules[i].name || `Rule ${i + 1}`
  }
  return (
    <div className="flex flex-col gap-3">
      <Hint>
        Paste lines from your log (with or without the [timestamp]) and run them through your
        unsaved rules. Nothing touches the real overlay.
      </Hint>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={8}
        spellCheck={false}
        placeholder={'[Fri Nov 13 20:01:00 2026] Lootmaster tells the guild, \'Bids open on Cloak of Flames\'\n[Fri Nov 13 20:01:05 2026] Soandso tells the guild, \'25\''}
        className="w-full resize-y rounded p-2 font-mono text-[11px] outline-none"
        style={inputStyle}
      />
      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={onRun}
          disabled={!text.trim()}
          className="rounded px-3 py-1 text-xs font-semibold"
          style={{
            backgroundColor: 'var(--color-primary)',
            color: 'var(--color-background)',
            opacity: text.trim() ? 1 : 0.5,
          }}
        >
          Run test
        </button>
        {testOn && (
          <button onClick={onClear} className="text-xs underline" style={{ color: 'var(--color-muted-foreground)' }}>
            Clear test
          </button>
        )}
        <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
          <input type="checkbox" checked={fromCurrent} onChange={(e) => setFromCurrent(e.target.checked)} />
          Start from what the overlay shows now
        </label>
      </div>
      {testOn && result?.error && (
        <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{result.error}</p>
      )}
      {testOn && result && !result.error && (
        <div className="flex flex-col gap-1">
          {result.steps.length === 0 && <Hint>No lines to run.</Hint>}
          {result.steps.map((s, i) => (
            <div key={i} className="flex items-start gap-2 rounded px-2 py-1" style={{ backgroundColor: 'var(--color-surface)' }}>
              <span className="min-w-0 flex-1 break-words font-mono text-[11px]" style={{ color: s.matched.length ? 'var(--color-foreground)' : 'var(--color-muted-foreground)' }}>
                {s.line}
              </span>
              <span className="flex shrink-0 flex-wrap justify-end gap-1">
                {s.matched.length === 0 ? (
                  <span className="text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>no match</span>
                ) : (
                  s.matched.map((id) => (
                    <span key={id} className="rounded px-1.5 py-px text-[10px]" style={{ backgroundColor: '#a78bfa22', color: '#a78bfa' }}>
                      {ruleName(id)}
                    </span>
                  ))
                )}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function JsonTab({
  draft,
  setDraft,
}: {
  draft: CustomOverlayDef
  setDraft: (d: CustomOverlayDef) => void
}): React.ReactElement {
  const pretty = useMemo(() => JSON.stringify(draft, null, 2), [draft])
  const [text, setText] = useState(pretty)
  const [error, setError] = useState('')
  const edited = text !== pretty

  // Follow the draft while the text box isn't holding edits of its own.
  const lastPretty = useRef(pretty)
  useEffect(() => {
    if (text === lastPretty.current) setText(pretty)
    lastPretty.current = pretty
  }, [pretty]) // eslint-disable-line react-hooks/exhaustive-deps

  const apply = (): void => {
    try {
      const parsed = JSON.parse(text) as CustomOverlayDef
      if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.widgets) || !Array.isArray(parsed.rules)) {
        setError('Needs an object with widgets and rules lists.')
        return
      }
      setError('')
      setDraft({ ...parsed, id: draft.id })
    } catch (e) {
      setError(`Not valid JSON: ${(e as Error).message}`)
    }
  }

  return (
    <div className="flex h-full flex-col gap-2">
      <Hint>The overlay’s definition. Edit it here for bulk changes, then Apply to load it into the draft.</Hint>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        spellCheck={false}
        className="min-h-[360px] w-full flex-1 resize-none rounded p-2 font-mono text-[11px] outline-none"
        style={inputStyle}
        aria-label="Overlay JSON"
      />
      <div className="flex items-center gap-3">
        <button
          onClick={apply}
          disabled={!edited}
          className="rounded px-3 py-1 text-xs font-semibold"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)', opacity: edited ? 1 : 0.5 }}
        >
          Apply
        </button>
        {edited && (
          <button onClick={() => { setText(pretty); setError('') }} className="text-xs underline" style={{ color: 'var(--color-muted-foreground)' }}>
            Undo edits
          </button>
        )}
        <button
          onClick={() => { void navigator.clipboard?.writeText(pretty) }}
          className="text-xs underline"
          style={{ color: 'var(--color-muted-foreground)' }}
        >
          Copy
        </button>
        {error && <span className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</span>}
      </div>
    </div>
  )
}

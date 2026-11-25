/**
 * The Chat Window editor: a wide modal over the hub with Sources / Keep /
 * Hide / Appearance / Test / Window tabs and a live preview. It opens from
 * the window's hub card. Edits stay in a draft until Save; Discard (or Esc)
 * drops them, asking first when there are any. The Window tab (built by the
 * hub) acts at once, outside the draft.
 *
 * Some changes save straight away while the editor is open (Pause/Resume,
 * Ctrl+scroll font size over the popout). The draft is rebased onto the new
 * saved definition: fields the user hasn't edited follow it, edited ones
 * keep the draft, so Save doesn't undo them and Discard keeps them.
 *
 * Opened from the popout's "Hide/Keep lines like this", it asks the backend
 * for a rule built from that line and adds it to the draft, so the user can
 * adjust it before saving. The draft is checked against the backend's
 * validator as it changes (a test run with no lines).
 */
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { MessagesSquare } from 'lucide-react'
import type {
  ChatRule,
  ChatTestResult,
  ChatWindow,
  ChatWindowDef,
} from '../../../types/chatWindow'
import {
  getChatChannels,
  suggestChatRule,
  testChatWindowLines,
  updateChatWindow,
} from '../../../services/api'
import { ConfirmModal } from '../../ConfirmModal'
import { useEscapeToClose } from '../../../hooks/useEscapeToClose'
import ChatLinesView from '../../overlays/ChatLinesView'
import ChatTabStrip from '../../overlays/ChatTabStrip'
import { useChatTabs } from '../../../hooks/useChatTabs'
import { Hint, TextInput } from '../customOverlayEditor/fields'
import { CHAT_WINDOW_COLOR } from '../../../lib/chatWindow'
import { AppearanceTab, RulesTab, SourcesTab, TestTab } from './tabs'

type Tab = 'sources' | 'keep' | 'hide' | 'appearance' | 'test' | 'window'

const TABS: { id: Tab; label: string }[] = [
  { id: 'sources', label: 'Sources' },
  { id: 'keep', label: 'Keep' },
  { id: 'hide', label: 'Hide' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'test', label: 'Test' },
  { id: 'window', label: 'Window' },
]

const CHECK_DEBOUNCE_MS = 400

function cloneDef(d: ChatWindowDef): ChatWindowDef {
  return JSON.parse(JSON.stringify(d)) as ChatWindowDef
}

const same = (a: unknown, b: unknown): boolean => JSON.stringify(a) === JSON.stringify(b)
const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v)

// rebase merges a new saved value into a draft field by field: a field the
// draft hasn't changed from base takes the saved value; a changed one keeps
// the draft. Objects merge per key; arrays are one value.
export function rebase(base: unknown, draft: unknown, saved: unknown): unknown {
  if (same(draft, base)) return saved
  if (isObj(base) && isObj(draft) && isObj(saved)) {
    const out: Record<string, unknown> = {}
    for (const k of new Set([...Object.keys(base), ...Object.keys(draft), ...Object.keys(saved)])) {
      const v = rebase(base[k], draft[k], saved[k])
      if (v !== undefined) out[k] = v
    }
    return out
  }
  return draft
}

export default function ChatWindowEditor({
  window: win,
  nowMs,
  suggest,
  windowTab,
  onClose,
}: {
  window: ChatWindow
  nowMs: number
  suggest?: { seq: number; mode: 'hide' | 'keep' }
  windowTab?: React.ReactNode
  onClose: () => void
}): React.ReactElement {
  const [draft, setDraftState] = useState<ChatWindowDef>(() => cloneDef(win.def))
  // The saved definition the draft started from (see rebase).
  const [base, setBase] = useState<ChatWindowDef>(() => cloneDef(win.def))
  const [tab, setTab] = useState<Tab>(suggest ? suggest.mode : 'sources')
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [confirmDiscard, setConfirmDiscard] = useState(false)
  const [check, setCheck] = useState<ChatTestResult | null>(null)
  const [namedChannels, setNamedChannels] = useState<string[]>([])
  const [characters, setCharacters] = useState<string[]>([])
  const [active, setActive] = useState('')
  // The rule just added by a suggestion, highlighted until edited away.
  const [newRuleId, setNewRuleId] = useState('')

  const dirty = useMemo(() => !same(draft, base), [draft, base])

  // The saved definition changed under the editor (an immediate action, or
  // another window's save): rebase the draft onto it.
  useEffect(() => {
    if (same(win.def, base)) return
    const saved = cloneDef(win.def)
    setDraftState((d) => rebase(base, d, saved) as ChatWindowDef)
    setBase(saved)
  }, [win.def, base])

  const setDraft = (d: ChatWindowDef): void => {
    setDraftState(d)
    setSaveError('')
  }

  useEffect(() => {
    getChatChannels()
      .then((r) => {
        setNamedChannels(r.channels ?? [])
        setCharacters(r.characters ?? [])
        setActive(r.active ?? '')
      })
      .catch(() => {})
  }, [])

  // Add the suggested rule once.
  const suggested = useRef(false)
  useEffect(() => {
    if (!suggest || suggested.current) return
    suggested.current = true
    suggestChatRule(win.def.id, suggest.seq, suggest.mode)
      .then((s) => {
        const rule: ChatRule = s.rule
        setDraftState((d) =>
          suggest.mode === 'keep' ? { ...d, keep: [...d.keep, rule] } : { ...d, hide: [...d.hide, rule] },
        )
        setNewRuleId(rule.id)
        setNote(
          `Added a ${suggest.mode} rule from that line. It matches ${s.matches} ` +
            `${s.matches === 1 ? 'line' : 'lines'} in the window now. ` +
            'Numbers and names are placeholders; adjust it, then Save.',
        )
      })
      .catch((e: Error) => setNote(`Couldn’t build a rule from that line: ${e.message}`))
  }, [suggest, win.def.id])

  useEffect(() => {
    let cancelled = false
    const t = setTimeout(() => {
      testChatWindowLines(draft, [])
        .then((r) => { if (!cancelled) setCheck(r) })
        .catch(() => {})
    }, CHECK_DEBOUNCE_MS)
    return () => { cancelled = true; clearTimeout(t) }
  }, [draft])

  const requestClose = (): void => {
    if (dirty) setConfirmDiscard(true)
    else onClose()
  }
  useEscapeToClose(requestClose)

  const save = (): void => {
    setSaving(true)
    setSaveError('')
    updateChatWindow({ ...draft, id: win.def.id })
      .then(() => onClose())
      .catch((e: Error) => { setSaveError(e.message); setSaving(false) })
  }

  // The preview's tabs live in memory only (no unread: it doesn't follow
  // appends).
  const tabs = useChatTabs({ def: draft, lines: win.state.lines, replay: !!win.state.replay, persistKey: null })

  const problem = saveError || check?.error || ''
  const sourceChannels = [
    ...draft.sources.channels,
    ...(draft.sources.all_named ? namedChannels.filter((c) => !draft.sources.channels.includes(c)) : []),
  ]

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
        aria-label={`Edit ${win.def.name}`}
        className="flex flex-col overflow-hidden rounded-lg"
        style={{
          backgroundColor: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 1180,
          height: 'min(90vh, 860px)',
        }}
      >
        <div
          className="flex shrink-0 flex-wrap items-center gap-2.5 border-b px-4 py-2.5"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <MessagesSquare size={18} style={{ color: CHAT_WINDOW_COLOR }} />
          <TextInput
            value={draft.name}
            onChange={(v) => setDraft({ ...draft, name: v })}
            placeholder="Window name"
            ariaLabel="Window name"
            className="w-64 !text-sm font-semibold"
          />
          <div className="ml-2 flex gap-1">
            {TABS.map((t) => {
              const count = t.id === 'keep' ? draft.keep.length : t.id === 'hide' ? draft.hide.length : 0
              return (
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
                  {count > 0 ? ` (${count})` : ''}
                </button>
              )
            })}
          </div>
        </div>

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
            {tab === 'sources' && (
              <SourcesTab draft={draft} setDraft={setDraft} namedChannels={namedChannels} characters={characters} />
            )}
            {(tab === 'keep' || tab === 'hide') && (
              <RulesTab
                key={tab}
                keep={tab === 'keep'}
                draft={draft}
                setDraft={setDraft}
                channels={sourceChannels}
                highlightId={newRuleId}
              />
            )}
            {tab === 'appearance' && <AppearanceTab draft={draft} setDraft={setDraft} />}
            {tab === 'test' && <TestTab draft={draft} defaultCharacter={active} />}
            {tab === 'window' && (
              <div className="flex flex-col gap-3">
                <Hint>
                  These take effect right away; they aren’t part of Save.
                </Hint>
                {windowTab}
              </div>
            )}
          </div>

          <div
            className="flex w-[340px] shrink-0 flex-col gap-2 border-l p-3"
            style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
          >
            <span className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted-foreground)' }}>
              Preview
            </span>
            <div
              className="flex min-h-0 flex-col overflow-hidden rounded"
              style={{
                border: '1px solid var(--color-border)',
                backgroundColor: `rgba(10,10,12,${draft.appearance.opacity})`,
                height: 420,
                maxHeight: '100%',
                color: 'rgba(255,255,255,0.9)',
              }}
            >
              <ChatLinesView
                def={draft}
                lines={tabs.lines}
                viewKey={tabs.active}
                between={
                  tabs.enabled && (
                    <ChatTabStrip
                      tabs={tabs.tabs}
                      active={tabs.active}
                      unread={tabs.unread}
                      onSelect={tabs.select}
                      onClose={tabs.close}
                      onCloseAllRead={tabs.closeAllRead}
                      fontPx={Math.round(Math.max(10, Math.min(16, 11 * (draft.appearance.font_scale || 1))))}
                    />
                  )
                }
                pins={win.state.pins}
                nowMs={nowMs}
                emptyText="Nothing in this window yet."
              />
            </div>
            <Hint>
              The window’s current lines with your appearance changes. Saving re-checks these lines
              against your rules; lines hidden before can’t come back.
            </Hint>
          </div>
        </div>

        <div
          className="flex shrink-0 items-center gap-3 border-t px-4 py-2.5"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <p
            className="min-w-0 flex-1 truncate text-xs"
            style={{ color: problem ? 'var(--color-danger)' : 'var(--color-muted-foreground)' }}
            title={problem}
          >
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
          message="Your edits to this chat window haven’t been saved."
          confirmLabel="Discard"
          tone="danger"
          onConfirm={onClose}
          onCancel={() => setConfirmDiscard(false)}
        />
      )}
    </div>
  )
}

/**
 * The Chat Window editor: a wide modal over the hub with Sources / Keep /
 * Hide / Appearance / Test tabs and a live preview. Edits stay in a draft
 * until Save; Discard (or Esc) drops them, asking first when there are any.
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
import { Hint, TextInput } from '../customOverlayEditor/fields'
import { CHAT_WINDOW_COLOR } from '../../../lib/chatWindow'
import { AppearanceTab, RulesTab, SourcesTab, TestTab } from './tabs'

type Tab = 'sources' | 'keep' | 'hide' | 'appearance' | 'test'

const TABS: { id: Tab; label: string }[] = [
  { id: 'sources', label: 'Sources' },
  { id: 'keep', label: 'Keep' },
  { id: 'hide', label: 'Hide' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'test', label: 'Test' },
]

const CHECK_DEBOUNCE_MS = 400

function cloneDef(d: ChatWindowDef): ChatWindowDef {
  return JSON.parse(JSON.stringify(d)) as ChatWindowDef
}

export default function ChatWindowEditor({
  window: win,
  nowMs,
  suggest,
  onClose,
}: {
  window: ChatWindow
  nowMs: number
  suggest?: { seq: number; mode: 'hide' | 'keep' }
  onClose: () => void
}): React.ReactElement {
  const [draft, setDraftState] = useState<ChatWindowDef>(() => cloneDef(win.def))
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

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(win.def), [draft, win.def])

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
                lines={win.state.lines}
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

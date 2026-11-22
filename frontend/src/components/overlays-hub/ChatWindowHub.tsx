/**
 * Chat Windows pieces for the Overlays hub: "+ New chat window" (a preset
 * picker), import from a share, and the share buttons in a window's drawer.
 */
import React, { useEffect, useRef, useState } from 'react'
import { Check, Copy, Download, FileText, MessagesSquare, Upload } from 'lucide-react'
import type { ChatWindowDef } from '../../types/chatWindow'
import { createChatWindow, exportChatWindows, importChatWindows } from '../../services/api'
import { CHAT_PRESETS, CHAT_WINDOW_COLOR, channelLabel, presetDef, type ChatPreset } from '../../lib/chatWindow'
import { ModalShell } from './CustomOverlayHub'

const PREVIEW_DEBOUNCE_MS = 300
const COPIED_MS = 2000

const muted = { color: 'var(--color-muted-foreground)' }
const secondaryBtn: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

export const CHAT_WINDOW_INFO = {
  blurb: 'Your own chat window: pick channels, hide spam, highlight and pin what matters.',
  details:
    'A filtered chat window over the game. Pick its channels (and optionally every other log line), ' +
    'then add keep rules (always shown, highlighted, optionally pinned) and hide rules. Right-click a ' +
    'line in the window to hide or keep lines like it.',
}

export function NewChatWindowModal({
  onCreated,
  onImport,
  onClose,
}: {
  onCreated: (def: ChatWindowDef & { id: string }) => void
  onImport: () => void
  onClose: () => void
}): React.ReactElement {
  const [preset, setPreset] = useState<ChatPreset>('guild')
  const [name, setName] = useState(presetDef('guild').name)
  const [nameTouched, setNameTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const pick = (p: ChatPreset): void => {
    setPreset(p)
    if (!nameTouched) setName(presetDef(p).name)
  }

  const create = (): void => {
    setBusy(true)
    createChatWindow({ ...presetDef(preset), name: name.trim() || presetDef(preset).name })
      .then((def) => onCreated(def as ChatWindowDef & { id: string }))
      .catch((e: Error) => { setError(e.message); setBusy(false) })
  }

  return (
    <ModalShell
      title="New chat window"
      icon={<MessagesSquare size={16} style={{ color: CHAT_WINDOW_COLOR }} />}
      width={520}
      onClose={onClose}
      footer={
        <>
          <button onClick={onImport} className="mr-auto flex items-center gap-1 text-xs underline" style={muted}>
            <Upload size={11} /> Import a shared window
          </button>
          <button onClick={onClose} className="rounded px-3 py-1.5 text-sm" style={{ ...muted, border: '1px solid var(--color-border)' }}>
            Cancel
          </button>
          <button
            onClick={create}
            disabled={busy}
            className="rounded px-3 py-1.5 text-sm font-semibold"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)', opacity: busy ? 0.5 : 1 }}
          >
            {busy ? 'Creating…' : 'Create'}
          </button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-2">
        {CHAT_PRESETS.map((p) => (
          <button
            key={p.id}
            onClick={() => pick(p.id)}
            className="flex flex-col items-start gap-0.5 rounded p-2.5 text-left"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: `1px solid ${preset === p.id ? CHAT_WINDOW_COLOR : 'var(--color-border)'}`,
            }}
          >
            <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{p.label}</span>
            <span className="text-[11px]" style={muted}>{p.blurb}</span>
          </button>
        ))}
      </div>
      <label className="flex items-center gap-2 text-xs" style={muted}>
        Name
        <input
          value={name}
          onChange={(e) => { setName(e.target.value); setNameTouched(true) }}
          className="flex-1 rounded px-2 py-1 text-sm outline-none"
          style={secondaryBtn}
        />
      </label>
      <p className="text-[11px]" style={muted}>
        You can change channels, rules and looks afterwards. The window fills from your chat history.
      </p>
      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}
    </ModalShell>
  )
}

export function ImportChatWindowsModal({
  onImported,
  onClose,
}: {
  onImported: (defs: ChatWindowDef[]) => void
  onClose: () => void
}): React.ReactElement {
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<ChatWindowDef[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!text.trim()) { setPreview(null); setError(''); return }
    let cancelled = false
    const t = setTimeout(() => {
      importChatWindows(text, true)
        .then((r) => { if (!cancelled) { setPreview(r.windows); setError('') } })
        .catch((e: Error) => { if (!cancelled) { setPreview(null); setError(e.message) } })
    }, PREVIEW_DEBOUNCE_MS)
    return () => { cancelled = true; clearTimeout(t) }
  }, [text])

  const openFile = (e: React.ChangeEvent<HTMLInputElement>): void => {
    const f = e.target.files?.[0]
    e.target.value = ''
    if (!f) return
    f.text().then(setText).catch(() => setError('Couldn’t read that file.'))
  }

  const doImport = (): void => {
    setBusy(true)
    importChatWindows(text)
      .then((r) => onImported(r.windows))
      .catch((e: Error) => { setError(e.message); setBusy(false) })
  }

  return (
    <ModalShell
      title="Import chat windows"
      icon={<Upload size={16} style={{ color: CHAT_WINDOW_COLOR }} />}
      width={560}
      onClose={onClose}
      footer={
        <>
          <button onClick={onClose} className="rounded px-3 py-1.5 text-sm" style={{ ...muted, border: '1px solid var(--color-border)' }}>
            Cancel
          </button>
          <button
            onClick={doImport}
            disabled={!preview || busy}
            className="rounded px-3 py-1.5 text-sm font-semibold"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)', opacity: !preview || busy ? 0.5 : 1 }}
          >
            {busy ? 'Importing…' : preview && preview.length > 1 ? `Import ${preview.length} windows` : 'Import'}
          </button>
        </>
      }
    >
      <div className="flex items-center gap-2">
        <p className="flex-1 text-xs" style={muted}>Paste the shared text below, or open the .json file.</p>
        <button onClick={() => fileRef.current?.click()} className="flex items-center gap-1 rounded px-2 py-1 text-xs" style={secondaryBtn}>
          <FileText size={11} /> Open file…
        </button>
        <input ref={fileRef} type="file" accept=".json,.txt,application/json,text/plain" onChange={openFile} className="hidden" />
      </div>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={6}
        spellCheck={false}
        placeholder='{"format": "pq-companion-chat-windows", …}'
        aria-label="Shared chat window text"
        className="w-full resize-y rounded p-2 font-mono text-[11px] outline-none"
        style={secondaryBtn}
      />
      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}
      {preview && (
        <div className="flex flex-col gap-1.5 rounded p-3" style={secondaryBtn}>
          {preview.map((d, i) => (
            <div key={i} className="flex flex-col">
              <span className="flex items-center gap-1.5 text-sm font-semibold">
                <MessagesSquare size={13} style={{ color: CHAT_WINDOW_COLOR }} /> {d.name}
              </span>
              <span className="text-[11px]" style={muted}>
                {[...d.sources.channels.map(channelLabel), ...(d.sources.all_named ? ['all named'] : []), ...(d.sources.other ? ['other lines'] : [])].join(', ')}
                {' · '}
                {d.keep.length} keep, {d.hide.length} hide
              </span>
            </div>
          ))}
        </div>
      )}
    </ModalShell>
  )
}

function slugify(name: string): string {
  return name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '') || 'chat'
}

// Copy (for pasting in Discord) or save one window, or every window, as a
// share. Character filters stay out.
export function ChatShareButtons({ ids, name }: { ids: string[]; name: string }): React.ReactElement {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), COPIED_MS)
    return () => clearTimeout(t)
  }, [copied])

  const copy = (): void => {
    setError('')
    exportChatWindows(ids)
      .then((sh) => navigator.clipboard.writeText(JSON.stringify(sh)))
      .then(() => setCopied(true))
      .catch(() => setError('Couldn’t copy'))
  }
  const save = (): void => {
    setError('')
    exportChatWindows(ids)
      .then((sh) => {
        const blob = new Blob([JSON.stringify(sh, null, 2)], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `pq-chat-${slugify(name)}.json`
        a.click()
        URL.revokeObjectURL(url)
      })
      .catch(() => setError('Couldn’t export'))
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <button onClick={copy} className="flex items-center gap-1 rounded px-2 py-1 text-xs" style={secondaryBtn}
        title="Copy as text to paste in Discord or on another PC. Add it with New chat window → Import.">
        {copied ? <Check size={11} style={{ color: 'var(--color-success)' }} /> : <Copy size={11} />}
        {copied ? 'Copied' : 'Copy to share'}
      </button>
      <button onClick={save} className="flex items-center gap-1 rounded px-2 py-1 text-xs" style={secondaryBtn}
        title="Save as a .json file">
        <Download size={11} /> Save as file
      </button>
      {error && <span className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</span>}
    </div>
  )
}

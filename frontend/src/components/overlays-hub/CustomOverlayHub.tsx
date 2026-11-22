/**
 * Custom Overlay pieces for the Overlays hub: the "+ New custom overlay"
 * chooser (blank or a built-in template), the drawer's Rules section
 * (on/off and which characters it listens for) and the two-click reset of
 * an overlay's contents, plus sharing: export (copy or save a .json share)
 * and import (paste or open one, previewed before it's added). The editor
 * itself is customOverlayEditor/CustomOverlayEditor.
 */
import React, { useEffect, useRef, useState } from 'react'
import { Check, Copy, Download, FileText, LayoutTemplate, Plus, RotateCcw, Upload } from 'lucide-react'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import {
  createCustomOverlay,
  exportCustomOverlay,
  getCustomOverlayTemplates,
  importCustomOverlay,
  previewCustomOverlayImport,
  listCharacters,
  updateCustomOverlay,
} from '../../services/api'
import type {
  CustomOverlay,
  CustomOverlayDef,
  CustomOverlayTemplate,
} from '../../types/customOverlay'

export const CUSTOM_OVERLAY_INFO = {
  blurb: 'Your own overlay, filled in by rules that read the log.',
  details:
    'Rules watch the log (or one chat channel) for lines you describe and fill in this ' +
    'overlay’s widgets: fields, a ranked table, a feed of recent lines, a counter or a ' +
    'countdown. Edit it to change the rules and widgets; test them against pasted log lines ' +
    'before saving.',
}

// A blank overlay still needs one widget to validate; a status line is the
// most common first widget.
export function blankOverlayDef(): CustomOverlayDef {
  return {
    id: '',
    name: 'New overlay',
    enabled: true,
    widgets: [{ id: 'status', type: 'fields', fields: [{ key: 'status', label: 'Status' }] }],
    rules: [],
  }
}

export function NewCustomOverlayModal({
  onCreated,
  onImport,
  onClose,
}: {
  // Called with the new overlay; the hub selects it and opens the editor.
  onCreated: (o: CustomOverlay) => void
  // Switches to the import dialog.
  onImport: () => void
  onClose: () => void
}): React.ReactElement {
  useEscapeToClose(onClose)
  const [templates, setTemplates] = useState<CustomOverlayTemplate[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    getCustomOverlayTemplates()
      .then((r) => setTemplates(r.templates ?? []))
      .catch(() => {})
  }, [])

  const create = (def: CustomOverlayDef): void => {
    setBusy(true)
    setError('')
    createCustomOverlay(def)
      .then(onCreated)
      .catch((e: Error) => { setError(e.message); setBusy(false) })
  }

  const option = (
    key: string,
    icon: React.ReactNode,
    title: string,
    text: string,
    onPick: () => void,
  ): React.ReactElement => (
    <button
      key={key}
      disabled={busy}
      onClick={onPick}
      className="flex w-full items-start gap-3 rounded-lg p-3 text-left"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        opacity: busy ? 0.6 : 1,
      }}
    >
      <span className="mt-0.5 shrink-0" style={{ color: '#a78bfa' }}>{icon}</span>
      <span className="min-w-0">
        <span className="block text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</span>
        <span className="block text-xs leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>{text}</span>
      </span>
    </button>
  )

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 460,
        }}
      >
        <div className="flex items-center gap-2 border-b px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          <LayoutTemplate size={16} style={{ color: '#a78bfa' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>New custom overlay</span>
        </div>
        <div className="flex flex-col gap-2 px-4 py-3">
          {option('blank', <Plus size={16} />, 'Blank', 'Start with an empty status line and add your own widgets and rules.',
            () => create(blankOverlayDef()))}
          {templates.map((t) =>
            option(t.key, <FileText size={16} />, t.name, t.description, () => create(t.def)),
          )}
          {option('import', <Upload size={16} />, 'Import…', 'Add an overlay someone shared with you, from a .json file or pasted text.', onImport)}
          {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}
        </div>
        <div className="flex justify-end border-t px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onClose}
            className="rounded px-3 py-1.5 text-sm"
            style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}

// Rules on/off and the character filter. These are definition fields, saved
// straight away (unlike the editor's draft) since each is a single choice.
export function CustomOverlayRulesSection({
  overlay,
}: {
  overlay: CustomOverlay
}): React.ReactElement {
  const [characters, setCharacters] = useState<string[]>([])
  const [error, setError] = useState('')
  useEffect(() => {
    listCharacters()
      .then((r) => setCharacters((r.characters ?? []).map((c) => c.name)))
      .catch(() => {})
  }, [])

  const save = (patch: Partial<CustomOverlayDef>): void => {
    setError('')
    updateCustomOverlay({ ...overlay.def, ...patch }).catch((e: Error) => setError(e.message))
  }

  const chosen = overlay.def.characters ?? []
  const toggleChar = (name: string): void => {
    const next = chosen.includes(name) ? chosen.filter((c) => c !== name) : [...chosen, name]
    save({ characters: next })
  }

  const row = (title: string, hint: string, control: React.ReactNode): React.ReactElement => (
    <div className="flex items-center justify-between gap-3 py-1.5">
      <div className="min-w-0">
        <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>{title}</p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{hint}</p>
      </div>
      <div className="shrink-0 text-xs">{control}</div>
    </div>
  )

  return (
    <div className="flex flex-col">
      {row(
        'Rules running',
        overlay.def.enabled ? 'Reading the log and updating this overlay.' : 'Paused. The contents stay as they are.',
        <input
          type="checkbox"
          checked={overlay.def.enabled}
          onChange={(e) => save({ enabled: e.target.checked })}
          aria-label="Rules running"
        />,
      )}
      <div className="py-1.5">
        <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>Characters</p>
        <p className="mb-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {chosen.length === 0
            ? 'Listens on every character. Pick some to limit it.'
            : 'Only listens while one of these is logged in.'}{' '}
          Not included when you share the overlay.
        </p>
        <div className="flex flex-wrap gap-1.5">
          {characters.length === 0 && (
            <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>No characters found yet.</span>
          )}
          {characters.map((name) => {
            const on = chosen.includes(name)
            return (
              <button
                key={name}
                onClick={() => toggleChar(name)}
                className="rounded-full px-2 py-0.5 text-xs"
                style={{
                  border: `1px solid ${on ? 'var(--color-primary)' : 'var(--color-border)'}`,
                  backgroundColor: on ? 'color-mix(in srgb, var(--color-primary) 18%, transparent)' : 'transparent',
                  color: on ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                }}
                aria-pressed={on}
              >
                {name}
              </button>
            )
          })}
        </div>
      </div>
      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}
    </div>
  )
}

const RESET_CONFIRM_MS = 3000

// "Reset contents" — clears every widget. Needs a second click within 3s.
export function ResetContentsButton({ onReset }: { onReset: () => void }): React.ReactElement {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const t = setTimeout(() => setArmed(false), RESET_CONFIRM_MS)
    return () => clearTimeout(t)
  }, [armed])
  return (
    <button
      onClick={() => {
        if (!armed) { setArmed(true); return }
        setArmed(false)
        onReset()
      }}
      title={armed ? 'Click again to clear everything in this overlay' : 'Clear everything in this overlay'}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs"
      style={{
        backgroundColor: armed ? 'color-mix(in srgb, var(--color-danger) 15%, transparent)' : 'var(--color-surface-2)',
        color: armed ? 'var(--color-danger)' : 'var(--color-foreground)',
        border: `1px solid ${armed ? 'var(--color-danger)' : 'var(--color-border)'}`,
      }}
    >
      <RotateCcw size={11} />
      {armed ? 'Click again to clear' : 'Reset contents'}
    </button>
  )
}

export function ModalShell({
  title,
  icon,
  width,
  onClose,
  children,
  footer,
}: {
  title: string
  icon: React.ReactNode
  width: number
  onClose: () => void
  children: React.ReactNode
  footer: React.ReactNode
}): React.ReactElement {
  useEscapeToClose(onClose)
  return (
    <div
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        role="dialog"
        aria-label={title}
        className="flex max-h-full flex-col rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: width,
        }}
      >
        <div className="flex shrink-0 items-center gap-2 border-b px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          {icon}
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</span>
        </div>
        <div className="flex min-h-0 flex-col gap-3 overflow-y-auto px-4 py-3">{children}</div>
        <div className="flex shrink-0 justify-end gap-2 border-t px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          {footer}
        </div>
      </div>
    </div>
  )
}

const PREVIEW_DEBOUNCE_MS = 300

const CHANNEL_LABELS: Record<string, string> = {
  any: 'any chat',
  guild: 'guild',
  raid: 'raid',
  group: 'group',
  tell: 'tells',
  say: 'say',
  ooc: 'OOC',
  shout: 'shout',
  auction: 'auction',
}

// One line describing a rule for the import preview.
function ruleSummary(r: CustomOverlayDef['rules'][number]): string {
  const where = !r.channel ? 'any log line' : CHANNEL_LABELS[r.channel] ?? `channel “${r.channel}”`
  const match = r.simple
    ? `${r.simple.kind === 'starts' ? 'starts with' : r.simple.kind === 'exact' ? 'is' : 'contains'} “${r.simple.text}”`
    : r.pattern
      ? `regex ${r.pattern}`
      : 'every message'
  return `${where} · ${match}`
}

// Paste or open a share, see what it contains, then add it. The backend
// validates the share for the preview, so what's shown is what gets saved.
export function ImportCustomOverlayModal({
  existingNames,
  onImported,
  onClose,
}: {
  existingNames: string[]
  onImported: (o: CustomOverlay) => void
  onClose: () => void
}): React.ReactElement {
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<CustomOverlayDef | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!text.trim()) { setPreview(null); setError(''); return }
    let cancelled = false
    const t = setTimeout(() => {
      previewCustomOverlayImport(text)
        .then((r) => { if (!cancelled) { setPreview(r.def); setError('') } })
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
    importCustomOverlay(text)
      .then(onImported)
      .catch((e: Error) => { setError(e.message); setBusy(false) })
  }

  const muted = { color: 'var(--color-muted-foreground)' }
  const duplicate = preview && existingNames.some((n) => n.toLowerCase() === preview.name.toLowerCase())

  return (
    <ModalShell
      title="Import custom overlay"
      icon={<Upload size={16} style={{ color: '#a78bfa' }} />}
      width={560}
      onClose={onClose}
      footer={
        <>
          <button
            onClick={onClose}
            className="rounded px-3 py-1.5 text-sm"
            style={{ ...muted, border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={doImport}
            disabled={!preview || busy}
            className="rounded px-3 py-1.5 text-sm font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              opacity: !preview || busy ? 0.5 : 1,
            }}
          >
            {busy ? 'Importing…' : 'Import'}
          </button>
        </>
      }
    >
      <div className="flex items-center gap-2">
        <p className="flex-1 text-xs" style={muted}>Paste the shared text below, or open the .json file.</p>
        <button
          onClick={() => fileRef.current?.click()}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
        >
          <FileText size={11} /> Open file…
        </button>
        <input ref={fileRef} type="file" accept=".json,.txt,application/json,text/plain" onChange={openFile} className="hidden" />
      </div>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={6}
        spellCheck={false}
        placeholder='{"format": "pq-companion-custom-overlay", …}'
        aria-label="Shared overlay text"
        className="w-full resize-y rounded p-2 font-mono text-[11px] outline-none"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
      />
      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}
      {preview && (
        <div className="flex flex-col gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}>
          <div className="flex items-center gap-2">
            <LayoutTemplate size={14} style={{ color: '#a78bfa' }} />
            <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{preview.name}</span>
          </div>
          {duplicate && (
            <p className="text-[11px]" style={{ color: 'var(--color-warning)' }}>
              You already have an overlay with this name. Importing adds a second one.
            </p>
          )}
          <p className="text-[11px]" style={muted}>
            Widgets: {preview.widgets.map((w) => w.title || w.fields?.map((f) => f.label).join(' / ') || w.type).join(', ')}
          </p>
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px]" style={muted}>
              {preview.rules.length} {preview.rules.length === 1 ? 'rule' : 'rules'}:
            </span>
            {preview.rules.map((r, i) => (
              <span key={r.id} className="truncate text-[11px]" style={{ color: 'var(--color-foreground)' }} title={ruleSummary(r)}>
                {r.name || `Rule ${i + 1}`}
                <span style={muted}> · {ruleSummary(r)}</span>
              </span>
            ))}
          </div>
          <p className="text-[11px]" style={muted}>
            Custom overlays only read your log and show what their rules write. They can’t run
            code or send anything anywhere. It listens on every character until you limit it.
          </p>
        </div>
      )}
    </ModalShell>
  )
}

function slugify(name: string): string {
  return name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '') || 'overlay'
}

const COPIED_MS = 2000

// Export: copy the share text (compact, for pasting in Discord) or save it
// as a .json file. Neither includes the character filter.
export function ShareButtons({ overlay }: { overlay: CustomOverlay }): React.ReactElement {
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), COPIED_MS)
    return () => clearTimeout(t)
  }, [copied])

  const copy = (): void => {
    setError('')
    exportCustomOverlay(overlay.def.id)
      .then((sh) => navigator.clipboard.writeText(JSON.stringify(sh)))
      .then(() => setCopied(true))
      .catch(() => setError('Couldn’t copy'))
  }
  const save = (): void => {
    setError('')
    exportCustomOverlay(overlay.def.id)
      .then((sh) => {
        const blob = new Blob([JSON.stringify(sh, null, 2)], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `pq-overlay-${slugify(overlay.def.name)}.json`
        a.click()
        URL.revokeObjectURL(url)
      })
      .catch(() => setError('Couldn’t export'))
  }

  const btn = (icon: React.ReactNode, label: string, onClick: () => void, title: string): React.ReactElement => (
    <button
      onClick={onClick}
      title={title}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
    >
      {icon}
      {label}
    </button>
  )

  return (
    <div className="flex flex-wrap items-center gap-2">
      {btn(
        copied ? <Check size={11} style={{ color: 'var(--color-success)' }} /> : <Copy size={11} />,
        copied ? 'Copied' : 'Copy to share',
        copy,
        'Copy this overlay as text to paste in Discord. Others add it with New custom overlay → Import.',
      )}
      {btn(<Download size={11} />, 'Save as file', save, 'Save this overlay as a .json file to share')}
      {error && <span className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</span>}
    </div>
  )
}

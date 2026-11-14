/**
 * Custom Overlay pieces for the Overlays hub: the "+ New custom overlay"
 * chooser (blank or a built-in template), the drawer's Rules section
 * (on/off and which characters it listens for) and the two-click reset of
 * an overlay's contents. The editor itself is CustomOverlayEditor.
 */
import React, { useEffect, useState } from 'react'
import { FileText, LayoutTemplate, Plus, RotateCcw } from 'lucide-react'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import {
  createCustomOverlay,
  getCustomOverlayTemplates,
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
  onClose,
}: {
  // Called with the new overlay; the hub selects it and opens the editor.
  onCreated: (o: CustomOverlay) => void
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

import React, { useEffect, useState } from 'react'
import { AlertTriangle, ChevronDown, ChevronRight, Pencil, RotateCcw } from 'lucide-react'
import { getSpellEmote, putSpellEmote, revertSpellEmote } from '../services/api'
import type { EmoteText, SpellEmote } from '../types/emote'

// EmoteEditor is the per-spell click-to-edit widget for the developer-mode
// Spell Emote Customizer. Shared by the Dev hub's Spell Emotes panel and the
// inline editor on the Spells page detail pane, so both surfaces stay in
// sync on the same fetch/save logic.

const EFFECTIVE_FIELDS: { key: keyof EmoteText; label: string }[] = [
  { key: 'cast_on_you', label: 'Cast on you' },
  { key: 'cast_on_other', label: 'Cast on other' },
  { key: 'spell_fades', label: 'Fades' },
]

const ADVANCED_FIELDS: { key: keyof EmoteText; label: string }[] = [
  { key: 'you_cast', label: 'You cast' },
  { key: 'other_casts', label: 'Others cast' },
]

interface EditableFieldProps {
  label: string
  value: string
  overridden: boolean
  onSave: (value: string) => void
}

function EditableField({ label, value, overridden, onSave }: EditableFieldProps): React.ReactElement {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)

  useEffect(() => {
    if (!editing) setDraft(value)
  }, [value, editing])

  const commit = (): void => {
    setEditing(false)
    if (draft !== value) onSave(draft)
  }

  return (
    <div className="flex items-start gap-2 py-1 text-sm">
      <span className="w-28 shrink-0 pt-0.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        {label}
      </span>
      {editing ? (
        <input
          autoFocus
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commit()
            if (e.key === 'Escape') { setDraft(value); setEditing(false) }
          }}
          className="min-w-0 flex-1 rounded border px-1.5 py-0.5 text-sm outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            borderColor: 'var(--color-primary)',
            color: 'var(--color-foreground)',
          }}
        />
      ) : (
        <button
          type="button"
          onClick={() => setEditing(true)}
          className="group flex min-w-0 flex-1 items-start gap-1.5 rounded px-1.5 py-0.5 text-left italic"
          style={{ color: value ? 'var(--color-foreground)' : 'var(--color-muted)' }}
        >
          <span className="min-w-0 flex-1 break-words">{value || '(no emote)'}</span>
          <Pencil
            size={11}
            className="mt-0.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-60"
          />
          {overridden && (
            <span
              className="mt-0.5 shrink-0 rounded px-1 text-[9px] font-semibold not-italic uppercase tracking-wide"
              style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
            >
              custom
            </span>
          )}
        </button>
      )}
    </div>
  )
}

interface EmoteEditorProps {
  spellId: number
  onChanged?: (se: SpellEmote) => void
}

export default function EmoteEditor({ spellId, onChanged }: EmoteEditorProps): React.ReactElement {
  const [emote, setEmote] = useState<SpellEmote | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    setEmote(null)
    setError(null)
    getSpellEmote(spellId)
      .then((se) => { if (!cancelled) setEmote(se) })
      .catch((err: Error) => { if (!cancelled) setError(err.message) })
    return () => { cancelled = true }
  }, [spellId])

  const overriddenSet = new Set(emote?.overridden_fields ?? [])

  const save = (key: keyof EmoteText, value: string): void => {
    putSpellEmote(spellId, { [key]: value })
      .then((se) => { setEmote(se); onChanged?.(se) })
      .catch((err: Error) => setError(err.message))
  }

  const revert = (): void => {
    revertSpellEmote(spellId)
      .then((se) => { setEmote(se); onChanged?.(se) })
      .catch((err: Error) => setError(err.message))
  }

  if (error) {
    return <p className="text-xs" style={{ color: 'var(--color-destructive)' }}>{error}</p>
  }
  if (!emote) {
    return <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Loading emotes…</p>
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
          Emotes {emote.customized && <span style={{ color: 'var(--color-primary)' }}>· customized</span>}
        </span>
        {emote.customized && (
          <button
            type="button"
            onClick={revert}
            className="flex items-center gap-1 text-[11px]"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Revert this spell's emotes to default"
          >
            <RotateCcw size={11} />
            Revert to default
          </button>
        )}
      </div>
      {EFFECTIVE_FIELDS.map((f) => (
        <EditableField
          key={f.key}
          label={f.label}
          value={emote.current[f.key]}
          overridden={overriddenSet.has(f.key)}
          onSave={(v) => save(f.key, v)}
        />
      ))}

      <button
        type="button"
        onClick={() => setAdvancedOpen((o) => !o)}
        className="mt-1 flex items-center gap-1 text-[11px]"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        {advancedOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        Advanced (server-controlled)
      </button>
      {advancedOpen && (
        <div className="mt-1 rounded border px-2 py-1.5" style={{ borderColor: 'var(--color-border)' }}>
          <div className="mb-1 flex items-start gap-1.5 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
            <span>
              These &ldquo;casting&rdquo; emotes are controlled server-side — editing them here
              usually won&rsquo;t change what displays in-game.
            </span>
          </div>
          {ADVANCED_FIELDS.map((f) => (
            <EditableField
              key={f.key}
              label={f.label}
              value={emote.current[f.key]}
              overridden={overriddenSet.has(f.key)}
              onSave={(v) => save(f.key, v)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

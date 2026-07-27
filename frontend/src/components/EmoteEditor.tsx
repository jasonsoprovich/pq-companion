import React, { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, ChevronDown, ChevronRight, Pencil, RotateCcw, Zap } from 'lucide-react'
import {
  applyTriggerPatternUpdate,
  getSpellEmote,
  getTriggerEmoteSuggestions,
  putSpellEmote,
  revertSpellEmote,
  revertTriggerPatternUpdate,
} from '../services/api'
import type { EmoteText, SpellEmote } from '../types/emote'
import type { EmoteChange, PatternLocation, TriggerEmoteSuggestion } from '../types/trigger'

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

const LOCATION_LABELS: Record<PatternLocation, string> = {
  pattern: 'Pattern',
  worn_off_pattern: 'Worn-off pattern',
  extra_pattern: 'Additional pattern',
}

function matchKey(s: TriggerEmoteSuggestion, mi: number): string {
  return `${s.trigger_id}:${mi}`
}

interface RecentAction {
  key: string
  label: string
  auditId: string
}

interface LinkedTriggersProps {
  suggestions: TriggerEmoteSuggestion[]
  // Re-fetches suggestions from the server. Applying a match can change what
  // "current" reads for OTHER matches on the very same trigger field (e.g.
  // Caustic Mist / Putrefy Flesh share one alternation pattern with a match
  // for each spell) — refetching after every apply/revert is what keeps
  // those matches' precomputed current/suggested text valid instead of
  // stale, rather than trying to patch the list up client-side.
  onMutated: () => void
}

// LinkedTriggers flags triggers explicitly linked (via spell_id) to the
// spell being edited whose pattern still contains the OLD emote text, with
// a suggested replacement. This never rewrites anything on its own — a
// regex doesn't reliably embed the emote text as a clean literal substring,
// and a trigger deliberately matching several spells that share an emote
// (the exact case this whole feature exists for) must never be touched by a
// blind bulk edit. Each match is applied individually, on request; a brief
// "Undo" affordance covers the common change-my-mind case right after.
function LinkedTriggers({ suggestions, onMutated }: LinkedTriggersProps): React.ReactElement | null {
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [recent, setRecent] = useState<RecentAction[]>([])

  const apply = (s: TriggerEmoteSuggestion, mi: number): void => {
    const m = s.matches[mi]
    const key = matchKey(s, mi)
    setBusy(key)
    setError(null)
    // The backend re-verifies the trigger's live field still equals
    // m.current exactly before writing m.suggested — passing the whole
    // prior field value (not just the emote fragment) as "old" means ANY
    // change to the pattern since this suggestion was computed (not only to
    // the matched fragment) correctly invalidates a stale apply.
    applyTriggerPatternUpdate(s.trigger_id, m.location, m.extra_index, m.current, m.suggested)
      .then((res) => {
        setRecent((prev) => [...prev, { key, label: s.name, auditId: res.audit_id }])
        onMutated()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(null))
  }

  const undo = (action: RecentAction): void => {
    setBusy(action.key)
    setError(null)
    revertTriggerPatternUpdate(action.auditId)
      .then(() => {
        setRecent((prev) => prev.filter((a) => a.auditId !== action.auditId))
        onMutated()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(null))
  }

  if (suggestions.length === 0 && recent.length === 0) {
    return null
  }

  return (
    <div className="mt-2 rounded border px-2 py-1.5" style={{ borderColor: 'var(--color-primary)' }}>
      {recent.length > 0 && (
        <div className="mb-1.5 flex flex-col gap-0.5">
          {recent.map((a) => (
            <div key={a.auditId} className="flex items-center gap-2 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              <span>✓ Updated {a.label}&rsquo;s pattern.</span>
              <button
                type="button"
                disabled={busy === a.key}
                onClick={() => undo(a)}
                className="underline"
                style={{ color: 'var(--color-primary)' }}
              >
                Undo
              </button>
            </div>
          ))}
        </div>
      )}
      {suggestions.length > 0 && (
        <>
          <div className="mb-1 flex items-start gap-1.5 text-[11px]" style={{ color: 'var(--color-foreground)' }}>
            <Zap size={12} className="mt-0.5 shrink-0" style={{ color: 'var(--color-primary)' }} />
            <span>
              Linked trigger{suggestions.length === 1 ? '' : 's'} may need a pattern update to
              match this new emote text:
            </span>
          </div>
          {error && <p className="mb-1 text-[11px]" style={{ color: 'var(--color-destructive)' }}>{error}</p>}
          {suggestions.map((s) => (
            <div key={s.trigger_id} className="border-t py-1.5 first:border-t-0" style={{ borderColor: 'var(--color-border)' }}>
              <div className="text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                {s.name}
                {s.pack_name && (
                  <span className="ml-1 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                    ({s.pack_name})
                  </span>
                )}
              </div>
              {s.matches.map((m, mi) => {
                const key = matchKey(s, mi)
                return (
                  <div key={key} className="mt-1 flex items-start gap-2 pl-1">
                    <div className="min-w-0 flex-1 font-mono text-[11px] leading-tight">
                      <div className="mb-0.5 text-[10px] not-italic" style={{ color: 'var(--color-muted)' }}>
                        {LOCATION_LABELS[m.location]}
                        {m.location === 'extra_pattern' ? ` #${m.extra_index + 1}` : ''}
                      </div>
                      <div style={{ color: 'var(--color-destructive)' }}>- {m.current}</div>
                      <div style={{ color: 'var(--color-success, #22c55e)' }}>+ {m.suggested}</div>
                    </div>
                    <button
                      type="button"
                      disabled={busy === key}
                      onClick={() => apply(s, mi)}
                      className="shrink-0 rounded px-2 py-1 text-[11px] font-medium"
                      style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
                    >
                      Update
                    </button>
                  </div>
                )
              })}
            </div>
          ))}
        </>
      )}
    </div>
  )
}

interface EmoteEditorProps {
  spellId: number
  onChanged?: (se: SpellEmote) => void
}

const ALL_FIELDS: (keyof EmoteText)[] = [
  'you_cast', 'other_casts', 'cast_on_you', 'cast_on_other', 'spell_fades',
]

export default function EmoteEditor({ spellId, onChanged }: EmoteEditorProps): React.ReactElement {
  const [emote, setEmote] = useState<SpellEmote | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [suggestions, setSuggestions] = useState<TriggerEmoteSuggestion[]>([])

  useEffect(() => {
    let cancelled = false
    setEmote(null)
    setError(null)
    setSuggestions([])
    getSpellEmote(spellId)
      .then((se) => { if (!cancelled) setEmote(se) })
      .catch((err: Error) => { if (!cancelled) setError(err.message) })
    return () => { cancelled = true }
  }, [spellId])

  const refreshTriggerSuggestions = useCallback((se: SpellEmote) => {
    const changes: EmoteChange[] = ALL_FIELDS
      .filter((f) => se.default[f] !== se.current[f])
      .map((f) => ({ field: f, old: se.default[f], new: se.current[f] }))
    if (changes.length === 0) {
      setSuggestions([])
      return
    }
    getTriggerEmoteSuggestions(spellId, changes)
      .then(setSuggestions)
      .catch(() => setSuggestions([]))
  }, [spellId])

  useEffect(() => {
    if (emote) refreshTriggerSuggestions(emote)
  }, [emote, refreshTriggerSuggestions])

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

      <LinkedTriggers suggestions={suggestions} onMutated={() => refreshTriggerSuggestions(emote)} />
    </div>
  )
}

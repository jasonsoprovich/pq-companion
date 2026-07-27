import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  FlaskConical,
  RefreshCw,
  Search,
  X,
} from 'lucide-react'
import {
  getEmoteDiff,
  getEmoteOverrides,
  getEmoteStatus,
  ignoreEmoteExternalChange,
  reapplyEmotes,
  restoreEmoteDefaults,
  searchSpells,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import EmoteEditor from '../components/EmoteEditor'
import type { EmoteStatus, SpellEmote, SpellEmoteDiff } from '../types/emote'
import type { Spell } from '../types/spell'

const cardStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
}

// SpellEmotesPanel is the Settings > Developer > Spell Emotes hub: search any
// spell to customize its client-visible chat emotes, browse everything
// already customized, review a red/green diff against the pristine default,
// and restore defaults. Editing here writes directly into the user's real
// <EQPath>/spells_en.txt — bad edits can break other players' triggers or
// particle patches, which is why this whole feature lives behind Developer
// Mode rather than in the normal Spells page.
export default function SpellEmotesPanel(): React.ReactElement {
  const [status, setStatus] = useState<EmoteStatus | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refreshStatus = useCallback(() => {
    getEmoteStatus()
      .then(setStatus)
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(() => { refreshStatus() }, [refreshStatus])

  useWebSocket((msg) => {
    if (msg.type === 'emote:external-change') refreshStatus()
  })

  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Spell[]>([])
  const [searching, setSearching] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (!query.trim()) { setResults([]); return }
    setSearching(true)
    debounceRef.current = setTimeout(() => {
      searchSpells(query, 20)
        .then((r) => setResults(r.items ?? []))
        .catch(() => setResults([]))
        .finally(() => setSearching(false))
    }, 250)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [query])

  const [customizedOnly, setCustomizedOnly] = useState(false)
  const [customized, setCustomized] = useState<SpellEmote[]>([])

  const refreshCustomized = useCallback(() => {
    getEmoteOverrides()
      .then(setCustomized)
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(() => {
    if (customizedOnly) refreshCustomized()
  }, [customizedOnly, refreshCustomized])

  const [expandedId, setExpandedId] = useState<number | null>(null)

  const onEmoteChanged = (): void => {
    refreshStatus()
    if (customizedOnly) refreshCustomized()
  }

  // ── Diff ──────────────────────────────────────────────────────────────
  const [diffOpen, setDiffOpen] = useState(false)
  const [diffs, setDiffs] = useState<SpellEmoteDiff[]>([])
  const [diffIndex, setDiffIndex] = useState(0)
  const diffRefs = useRef<Record<number, HTMLDivElement | null>>({})

  const openDiff = (): void => {
    setDiffOpen(true)
    getEmoteDiff()
      .then((d) => { setDiffs(d); setDiffIndex(0) })
      .catch((err: Error) => setError(err.message))
  }

  const jumpTo = (index: number): void => {
    if (diffs.length === 0) return
    const next = ((index % diffs.length) + diffs.length) % diffs.length
    setDiffIndex(next)
    diffRefs.current[diffs[next].spell_id]?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  // ── Restore defaults ──────────────────────────────────────────────────
  const [confirmRestore, setConfirmRestore] = useState(false)
  const [restoring, setRestoring] = useState(false)

  const doRestore = (): void => {
    setRestoring(true)
    restoreEmoteDefaults()
      .then(() => {
        setConfirmRestore(false)
        refreshStatus()
        if (customizedOnly) refreshCustomized()
        if (diffOpen) openDiff()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setRestoring(false))
  }

  // ── External-change banner actions ─────────────────────────────────────
  const [applying, setApplying] = useState(false)

  const doReapply = (): void => {
    setApplying(true)
    reapplyEmotes().then(refreshStatus).catch((err: Error) => setError(err.message)).finally(() => setApplying(false))
  }
  const doIgnore = (): void => {
    setApplying(true)
    ignoreEmoteExternalChange().then(refreshStatus).catch((err: Error) => setError(err.message)).finally(() => setApplying(false))
  }

  const listedSpells = customizedOnly ? [] : results

  return (
    <div className="flex flex-col gap-4">
      <div
        className="flex items-start gap-2 rounded-lg px-4 py-3"
        style={{
          border: '1px solid var(--color-warning, #f59e0b)',
          backgroundColor: 'color-mix(in srgb, var(--color-warning, #f59e0b) 12%, transparent)',
        }}
      >
        <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: 'var(--color-warning, #f59e0b)' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Edits here write directly into your real <code>spells_en.txt</code>, changing what
          chat emotes you see in-game — useful for disambiguating shared emotes (many slows all
          say &ldquo; yawns.&rdquo;) or adding one to a spell that ships with none. A pristine
          default backup is kept automatically so you can always revert.
        </p>
      </div>

      {status?.pending_external_change && (
        <div
          className="flex items-center gap-3 rounded-lg px-4 py-3"
          style={{ border: '1px solid var(--color-primary)', backgroundColor: 'var(--color-surface-2)' }}
        >
          <RefreshCw size={16} className="shrink-0" style={{ color: 'var(--color-primary)' }} />
          <p className="flex-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
            <code>spells_en.txt</code> was replaced outside the app — most likely a server patch.
            Your customizations are safely backed up.
          </p>
          <button
            type="button"
            disabled={applying}
            onClick={doReapply}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
          >
            Re-apply now
          </button>
          <button
            type="button"
            disabled={applying}
            onClick={openDiff}
            className="shrink-0 rounded border px-3 py-1.5 text-xs font-medium"
            style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
          >
            Review Diff
          </button>
          <button
            type="button"
            disabled={applying}
            onClick={doIgnore}
            className="shrink-0 rounded px-2 py-1.5 text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            Ignore
          </button>
        </div>
      )}

      <section className="rounded-lg p-4" style={cardStyle}>
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
            <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Spell Emotes
            </h2>
          </div>
          <div className="ml-auto flex items-center gap-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {status && (
              <span>
                {status.configured
                  ? status.file_present
                    ? `${status.override_count} spell${status.override_count === 1 ? '' : 's'} customized`
                    : 'spells_en.txt not found'
                  : 'EverQuest directory not configured'}
              </span>
            )}
            <button type="button" onClick={openDiff} className="rounded border px-2 py-1" style={{ borderColor: 'var(--color-border)' }}>
              View Diff
            </button>
            {confirmRestore ? (
              <span className="flex items-center gap-1.5">
                <span style={{ color: 'var(--color-destructive)' }}>Restore ALL to defaults?</span>
                <button
                  type="button"
                  disabled={restoring}
                  onClick={doRestore}
                  className="rounded px-2 py-1 font-medium"
                  style={{ backgroundColor: 'var(--color-destructive)', color: 'var(--color-background)' }}
                >
                  Confirm
                </button>
                <button type="button" onClick={() => setConfirmRestore(false)} className="rounded border px-2 py-1" style={{ borderColor: 'var(--color-border)' }}>
                  Cancel
                </button>
              </span>
            ) : (
              <button type="button" onClick={() => setConfirmRestore(true)} className="rounded border px-2 py-1" style={{ borderColor: 'var(--color-border)' }}>
                Restore Defaults
              </button>
            )}
          </div>
        </div>

        {error && (
          <p className="mb-2 text-xs" style={{ color: 'var(--color-destructive)' }}>{error}</p>
        )}

        {diffOpen && (
          <div className="mb-4 rounded border p-3" style={{ borderColor: 'var(--color-border)' }}>
            <div className="mb-2 flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <span className="font-medium">
                {diffs.length} spell{diffs.length === 1 ? '' : 's'} changed
              </span>
              {diffs.length > 0 && (
                <span className="ml-auto flex items-center gap-1">
                  <button type="button" onClick={() => jumpTo(diffIndex - 1)} className="rounded border p-1" style={{ borderColor: 'var(--color-border)' }}>
                    <ChevronUp size={12} />
                  </button>
                  <span>{diffIndex + 1} / {diffs.length}</span>
                  <button type="button" onClick={() => jumpTo(diffIndex + 1)} className="rounded border p-1" style={{ borderColor: 'var(--color-border)' }}>
                    <ChevronDown size={12} />
                  </button>
                </span>
              )}
              <button type="button" onClick={() => setDiffOpen(false)} className="shrink-0">
                <X size={12} />
              </button>
            </div>
            <div className="max-h-80 overflow-y-auto">
              {diffs.length === 0 && (
                <p className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
                  No customizations differ from the default backup.
                </p>
              )}
              {diffs.map((d, i) => (
                <div
                  key={d.spell_id}
                  ref={(el) => { diffRefs.current[d.spell_id] = el }}
                  className="border-t py-2 first:border-t-0"
                  style={{
                    borderColor: 'var(--color-border)',
                    backgroundColor: i === diffIndex ? 'var(--color-surface-2)' : 'transparent',
                  }}
                >
                  <div className="mb-1 text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                    {d.name} <span style={{ color: 'var(--color-muted)' }}>#{d.spell_id}</span>
                  </div>
                  {d.fields.map((f) => (
                    <div key={f.field} className="pl-2 font-mono text-[11px] leading-tight">
                      <div style={{ color: 'var(--color-destructive)' }}>- {f.label}: {f.old || '(empty)'}</div>
                      <div style={{ color: 'var(--color-success, #22c55e)' }}>+ {f.label}: {f.new || '(empty)'}</div>
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="mb-3 flex items-center gap-3">
          <div
            className="flex flex-1 items-center gap-2 rounded border px-2 py-1.5"
            style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-2)' }}
          >
            <Search size={13} style={{ color: 'var(--color-muted)' }} />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search spells to customize…"
              disabled={customizedOnly}
              className="flex-1 bg-transparent text-sm outline-none disabled:opacity-40"
              style={{ color: 'var(--color-foreground)' }}
              spellCheck={false}
            />
            {query && (
              <button onClick={() => setQuery('')}>
                <X size={12} style={{ color: 'var(--color-muted)' }} />
              </button>
            )}
          </div>
          <label className="flex shrink-0 items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <input
              type="checkbox"
              checked={customizedOnly}
              onChange={(e) => setCustomizedOnly(e.target.checked)}
            />
            Customized only
          </label>
        </div>

        {customizedOnly ? (
          <div className="flex flex-col gap-1">
            {customized.length === 0 && (
              <p className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
                No spells customized yet.
              </p>
            )}
            {customized.map((se) => (
              <div key={se.spell_id} className="rounded border p-2" style={{ borderColor: 'var(--color-border)' }}>
                <button
                  type="button"
                  onClick={() => setExpandedId(expandedId === se.spell_id ? null : se.spell_id)}
                  className="flex w-full items-center justify-between text-left text-sm font-medium"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  <span>{se.name} <span className="text-xs" style={{ color: 'var(--color-muted)' }}>#{se.spell_id}</span></span>
                  {expandedId === se.spell_id ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
                </button>
                {expandedId === se.spell_id && (
                  <div className="mt-2">
                    <EmoteEditor spellId={se.spell_id} onChanged={onEmoteChanged} />
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-1">
            {searching && <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Searching…</p>}
            {!searching && query && listedSpells.length === 0 && (
              <p className="text-xs italic" style={{ color: 'var(--color-muted)' }}>No spells found.</p>
            )}
            {listedSpells.map((spell) => (
              <div key={spell.id} className="rounded border p-2" style={{ borderColor: 'var(--color-border)' }}>
                <button
                  type="button"
                  onClick={() => setExpandedId(expandedId === spell.id ? null : spell.id)}
                  className="flex w-full items-center justify-between text-left text-sm font-medium"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  <span>{spell.name} <span className="text-xs" style={{ color: 'var(--color-muted)' }}>#{spell.id}</span></span>
                  {expandedId === spell.id ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
                </button>
                {expandedId === spell.id && (
                  <div className="mt-2">
                    <EmoteEditor spellId={spell.id} onChanged={onEmoteChanged} />
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

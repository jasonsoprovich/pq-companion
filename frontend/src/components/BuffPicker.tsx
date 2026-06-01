import React, { useEffect, useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import { searchSpells } from '../services/api'
import type { Spell } from '../types/spell'
import { SpellIcon } from './Icon'
import { useEscapeToClose } from '../hooks/useEscapeToClose'

// BuffPicker is a search modal for selecting a beneficial buff spell. Used
// by the Raid Buffs panel on the character stats page to swap individual
// slots. Filters server-side to goodEffect=1 spells (no debuffs/nukes).
//
// `currentSpellId` and `existingSpellIDs` let the picker mark the current
// selection and grey out spells already chosen in other slots — the in-game
// stacking model doesn't let the same buff appear twice in the preset.

interface BuffPickerProps {
  currentSpellId: number
  existingSpellIDs: number[]
  onPick: (spell: Spell) => void
  onClear?: () => void
  onClose: () => void
}

const SEARCH_LIMIT = 60
const DEBOUNCE_MS = 200

export function BuffPicker({ currentSpellId, existingSpellIDs, onPick, onClear, onClose }: BuffPickerProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Spell[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEscapeToClose(onClose)

  // Debounce the search — every keystroke would hammer the API otherwise.
  useEffect(() => {
    let cancelled = false
    const t = window.setTimeout(async () => {
      setLoading(true)
      setError(null)
      try {
        const res = await searchSpells(query, SEARCH_LIMIT, 0, -1, 0, 0, true)
        if (!cancelled) setResults(res.items ?? [])
      } catch (e: unknown) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'search failed')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }, DEBOUNCE_MS)
    return () => {
      cancelled = true
      window.clearTimeout(t)
    }
  }, [query])

  const existingSet = useMemo(() => new Set(existingSpellIDs), [existingSpellIDs])

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
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
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 520,
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div className="flex items-center gap-2 px-3 py-2 border-b" style={{ borderColor: 'var(--color-border)' }}>
          <Search size={14} style={{ color: 'var(--color-muted)' }} />
          <div className="flex-1 min-w-0">
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>
              Pick a buff for this slot
            </div>
            <input
              type="text"
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search beneficial spells…"
              className="w-full bg-transparent text-sm outline-none"
              style={{ color: 'var(--color-foreground)' }}
            />
          </div>
          <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }} aria-label="Close">
            <X size={14} />
          </button>
        </div>

        <div style={{ flex: 1, overflowY: 'auto' }}>
          {loading && (
            <p className="px-4 py-6 text-sm text-center" style={{ color: 'var(--color-muted)' }}>
              Searching…
            </p>
          )}
          {error && (
            <p className="px-4 py-6 text-sm text-center" style={{ color: 'var(--color-destructive)' }}>
              {error}
            </p>
          )}
          {!loading && !error && results.length === 0 && (
            <p className="px-4 py-6 text-sm text-center" style={{ color: 'var(--color-muted)' }}>
              {query ? 'No beneficial spells match that name.' : 'Start typing to search beneficial spells.'}
            </p>
          )}
          {results.map((s) => {
            const isCurrent = s.id === currentSpellId
            const isAlreadyPicked = !isCurrent && existingSet.has(s.id)
            return (
              <button
                key={s.id}
                onClick={() => onPick(s)}
                disabled={isCurrent || isAlreadyPicked}
                className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors border-t hover:bg-(--color-surface-2) disabled:opacity-40"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <SpellIcon id={s.new_icon} name={s.name} size={24} />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium truncate" style={{ color: 'var(--color-foreground)' }}>
                    {s.name}
                  </div>
                  <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {isCurrent ? 'Current selection' : isAlreadyPicked ? 'Already in another slot' : 'Beneficial buff'}
                  </div>
                </div>
              </button>
            )
          })}
        </div>

        {onClear && (
          <div className="border-t px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
            <button
              onClick={onClear}
              className="text-xs underline decoration-dotted"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              Clear this slot
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

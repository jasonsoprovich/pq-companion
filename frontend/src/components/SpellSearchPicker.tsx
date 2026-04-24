import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Search, X, RefreshCw } from 'lucide-react'
import { searchSpells } from '../services/api'
import type { Spell } from '../types/spell'
import { castableClassesShort } from '../lib/spellHelpers'

interface SpellSearchPickerProps {
  onPick: (spell: Spell) => void
  onClose: () => void
}

/**
 * Modal dialog for searching spells and picking one. Used to drive the
 * "Add Timer" flow from the Spell Timer overlay tab.
 */
export default function SpellSearchPicker({
  onPick,
  onClose,
}: SpellSearchPickerProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Spell[]>([])
  const [loading, setLoading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const run = useCallback((q: string) => {
    setLoading(true)
    searchSpells(q, 30, 0)
      .then((r) => setResults(r.items ?? []))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => run(query), 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, run])

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
          maxWidth: 480,
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div
          className="flex items-center gap-2 px-3 py-2 border-b"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <Search size={14} style={{ color: 'var(--color-muted)' }} />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search spells to create a timer trigger…"
            className="flex-1 bg-transparent text-sm outline-none"
            style={{ color: 'var(--color-foreground)' }}
          />
          <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
            <X size={14} />
          </button>
        </div>

        {/* Results */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {loading && (
            <div className="flex items-center justify-center py-6">
              <RefreshCw size={14} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          )}
          {!loading && results.length === 0 && (
            <p className="px-4 py-6 text-sm text-center" style={{ color: 'var(--color-muted)' }}>
              {query ? 'No spells match that search.' : 'Type to search spells.'}
            </p>
          )}
          {!loading &&
            results.map((s) => (
              <button
                key={s.id}
                onClick={() => onPick(s)}
                className="w-full px-3 py-2 text-left transition-colors border-t"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <div className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
                  {s.name}
                </div>
                <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {castableClassesShort(s.class_levels)}
                </div>
              </button>
            ))}
        </div>
      </div>
    </div>
  )
}

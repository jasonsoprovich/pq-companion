import React, { useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import { searchRecipes } from '../services/api'
import type { RecipeSummary } from '../types/recipe'
import { ItemIcon } from './Icon'

interface RecipeSearchModalProps {
  open: boolean
  /** Optional title above the search box (e.g. "Add recipe"). */
  title?: string
  /** Restrict results to this tradeskill discipline's recipes. */
  tradeskill: number
  /** Recipe ids already in the path — filtered out of results. */
  excludeIds?: number[]
  onSelect: (recipe: RecipeSummary) => void
  onClose: () => void
}

/**
 * Recipe-only search modal for adding a recipe to a Custom tradeskill leveling
 * path. Mirrors ItemSearchModal: debounced text search with keyboard nav,
 * scoped to one discipline so results are always something the selected trade
 * can actually use.
 */
export default function RecipeSearchModal({
  open,
  title = 'Add recipe',
  tradeskill,
  excludeIds,
  onSelect,
  onClose,
}: RecipeSearchModalProps): React.ReactElement | null {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<RecipeSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [activeIdx, setActiveIdx] = useState(0)
  const inputRef = useRef<HTMLInputElement | null>(null)
  // Monotonic token so a slow earlier request can't clobber a newer one.
  const seqRef = useRef(0)

  // Reset on open.
  useEffect(() => {
    if (!open) return
    setQ('')
    setResults([])
    setActiveIdx(0)
    setLoading(false)
    setTimeout(() => inputRef.current?.focus(), 0)
  }, [open])

  // Debounced search, scoped to the selected discipline.
  useEffect(() => {
    if (!open) return
    if (q.trim().length < 2) {
      setResults([])
      setLoading(false)
      return
    }
    setLoading(true)
    const excluded = new Set(excludeIds ?? [])
    const handle = setTimeout(() => {
      const seq = ++seqRef.current
      searchRecipes(q, 20, 0, { tradeskill })
        .then((r) => {
          if (seq !== seqRef.current) return
          setResults((r.items ?? []).filter((rc) => !excluded.has(rc.id)))
          setActiveIdx(0)
        })
        .catch(() => { if (seq === seqRef.current) setResults([]) })
        .finally(() => { if (seq === seqRef.current) setLoading(false) })
    }, 200)
    return () => clearTimeout(handle)
  }, [q, open, tradeskill, excludeIds])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIdx((i) => Math.min(results.length - 1, i + 1))
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIdx((i) => Math.max(0, i - 1))
      }
      if (e.key === 'Enter' && results[activeIdx]) {
        e.preventDefault()
        onSelect(results[activeIdx])
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, results, activeIdx, onClose, onSelect])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center p-4 pt-20"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={(e) => { e.stopPropagation(); onClose() }}
    >
      <div
        className="w-full max-w-xl rounded-lg shadow-2xl"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          <Search size={16} style={{ color: 'var(--color-muted)' }} />
          <div className="flex-1">
            <div className="text-[10px] uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              {title}
            </div>
            <input
              ref={inputRef}
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Type a recipe name…"
              className="w-full bg-transparent text-sm outline-none"
              style={{ color: 'var(--color-foreground)' }}
            />
          </div>
          <button onClick={onClose} title="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>
        <div className="max-h-[50vh] overflow-y-auto">
          {loading && (
            <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              Searching…
            </div>
          )}
          {!loading && q.trim().length >= 2 && results.length === 0 && (
            <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No recipes match.
            </div>
          )}
          {!loading && q.trim().length < 2 && (
            <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              Type at least two characters to search.
            </div>
          )}
          {results.map((rc, i) => {
            const active = i === activeIdx
            return (
              <button
                key={rc.id}
                onMouseEnter={() => setActiveIdx(i)}
                onClick={() => onSelect(rc)}
                className="flex w-full items-center gap-3 px-3 py-2 text-left"
                style={{
                  backgroundColor: active ? 'var(--color-surface)' : 'transparent',
                  borderLeft: active ? '2px solid var(--color-primary)' : '2px solid transparent',
                }}
              >
                <ItemIcon id={rc.product_icon} name={rc.name} size={24} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm" style={{ color: 'var(--color-foreground)' }}>
                    {rc.name}
                  </div>
                  <div className="truncate text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    Trivial {rc.trivial}
                  </div>
                </div>
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

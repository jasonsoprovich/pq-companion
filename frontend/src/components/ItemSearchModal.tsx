import React, { useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import { searchItems } from '../services/api'
import type { Item } from '../types/item'
import { effectiveItemTypeLabel, slotsLabel } from '../lib/itemHelpers'
import { ItemIcon } from './Icon'

interface ItemSearchModalProps {
  open: boolean
  /** Optional title above the search box (e.g. "Add to Wishlist"). */
  title?: string
  /** Optional slot bitmask (see lib/itemSlots.ts) — restricts results to items that fit this slot. */
  slotFilter?: number
  onSelect: (item: Item) => void
  onClose: () => void
}

/**
 * Items-only search modal for picking a single item — used by the Wishlist
 * "Add item" button. Simpler than the global Cmd/K search: no spells/NPCs/
 * zones, no result categories, debounced text search with keyboard nav.
 */
export default function ItemSearchModal({
  open,
  title = 'Find an item',
  slotFilter,
  onSelect,
  onClose,
}: ItemSearchModalProps): React.ReactElement | null {
  const [q, setQ] = useState('')
  // Kept lenient (Item[] | null/undefined-tolerant) and normalised on read.
  // Some upstream paths historically set this to null when the backend
  // returned an empty result set, which would crash the render.
  const [results, setResults] = useState<Item[]>([])
  const safeResults: Item[] = results ?? []
  const [loading, setLoading] = useState(false)
  const [activeIdx, setActiveIdx] = useState(0)
  const inputRef = useRef<HTMLInputElement | null>(null)
  // Monotonic token so a slow earlier request can't clobber a newer one. The
  // debounce cancels not-yet-fired timeouts, but a fetch already in flight when
  // the query changes is not cancelled and could resolve out of order.
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

  // Debounced search.
  useEffect(() => {
    if (!open) return
    if (q.trim().length < 2) {
      setResults([])
      setLoading(false)
      return
    }
    setLoading(true)
    const handle = setTimeout(() => {
      const seq = ++seqRef.current
      searchItems(q, 20, 0, slotFilter ? { slot: slotFilter } : {})
        .then((r) => {
          if (seq !== seqRef.current) return
          setResults(r.items ?? [])
          setActiveIdx(0)
        })
        .catch(() => { if (seq === seqRef.current) setResults([]) })
        .finally(() => { if (seq === seqRef.current) setLoading(false) })
    }, 200)
    return () => clearTimeout(handle)
  }, [q, open, slotFilter])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIdx((i) => Math.min(safeResults.length - 1, i + 1))
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIdx((i) => Math.max(0, i - 1))
      }
      if (e.key === 'Enter' && safeResults[activeIdx]) {
        e.preventDefault()
        onSelect(safeResults[activeIdx])
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, safeResults, activeIdx, onClose, onSelect])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center p-4 pt-20"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      // Now sometimes opened from inside another modal's backdrop (e.g.
      // ItemCompareModal's "Add item"), so a click here must not bubble up and
      // close the parent modal too.
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
              placeholder="Type an item name…"
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
          {!loading && q.trim().length >= 2 && safeResults.length === 0 && (
            <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No items match.
            </div>
          )}
          {!loading && q.trim().length < 2 && (
            <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              Type at least two characters to search.
            </div>
          )}
          {safeResults.map((item, i) => {
            const active = i === activeIdx
            return (
              <button
                key={item.id}
                onMouseEnter={() => setActiveIdx(i)}
                onClick={() => onSelect(item)}
                className="flex w-full items-center gap-3 px-3 py-2 text-left"
                style={{
                  backgroundColor: active ? 'var(--color-surface)' : 'transparent',
                  borderLeft: active ? '2px solid var(--color-primary)' : '2px solid transparent',
                }}
              >
                <ItemIcon id={item.icon} name={item.name} size={24} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm" style={{ color: 'var(--color-foreground)' }}>
                    {item.name}
                  </div>
                  <div className="truncate text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {effectiveItemTypeLabel(item.item_class, item.item_type)}
                    {item.slots > 0 && ` · ${slotsLabel(item.slots)}`}
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

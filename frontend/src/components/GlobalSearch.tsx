import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, Sword, Sparkles, Skull, Map } from 'lucide-react'
import { globalSearch, type GlobalSearchResult } from '../services/api'
import type { Item } from '../types/item'
import type { Spell } from '../types/spell'
import type { NPC } from '../types/npc'
import type { Zone } from '../types/zone'
import { effectiveItemTypeLabel } from '../lib/itemHelpers'
import { castableClassesShort } from '../lib/spellHelpers'
import { npcDisplayName, className as npcClassName } from '../lib/npcHelpers'
import { ItemIcon, SpellIcon } from './Icon'

// ── Types ──────────────────────────────────────────────────────────────────────

type ResultEntry =
  | { kind: 'item'; item: Item }
  | { kind: 'spell'; spell: Spell }
  | { kind: 'npc'; npc: NPC }
  | { kind: 'zone'; zone: Zone }

interface Section {
  label: string
  icon: React.ReactNode
  entries: ResultEntry[]
}

// ── Result row ─────────────────────────────────────────────────────────────────

interface ResultRowProps {
  entry: ResultEntry
  active: boolean
  onHover: () => void
  onClick: () => void
}

function ResultRow({ entry, active, onHover, onClick }: ResultRowProps): React.ReactElement {
  let name = ''
  let subtitle = ''

  if (entry.kind === 'item') {
    name = entry.item.name
    subtitle = effectiveItemTypeLabel(entry.item.item_class, entry.item.item_type)
    if (entry.item.req_level > 0) subtitle += ` · Req ${entry.item.req_level}`
  } else if (entry.kind === 'spell') {
    name = entry.spell.name
    subtitle = castableClassesShort(entry.spell.class_levels)
  } else if (entry.kind === 'npc') {
    name = npcDisplayName(entry.npc)
    subtitle = `Level ${entry.npc.level} ${npcClassName(entry.npc.class)}`
  } else {
    name = entry.zone.long_name
    subtitle = entry.zone.short_name
  }

  let leadingIcon: React.ReactNode = null
  if (entry.kind === 'item') {
    leadingIcon = <ItemIcon id={entry.item.icon} name={entry.item.name} size={24} />
  } else if (entry.kind === 'spell') {
    leadingIcon = <SpellIcon id={entry.spell.new_icon} name={entry.spell.name} size={24} />
  }

  return (
    <button
      className="flex w-full items-center gap-3 px-4 py-2 text-left transition-none"
      style={{
        backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
        borderLeft: active ? '2px solid var(--color-primary)' : '2px solid transparent',
      }}
      onMouseEnter={onHover}
      onClick={onClick}
    >
      {leadingIcon}
      <div className="min-w-0 flex-1">
        <div
          className="truncate text-sm font-medium"
          style={{ color: active ? 'var(--color-primary)' : 'var(--color-foreground)' }}
        >
          {name}
        </div>
        <div className="mt-0.5 truncate text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {subtitle}
        </div>
      </div>
    </button>
  )
}

// ── Main component ─────────────────────────────────────────────────────────────

interface GlobalSearchProps {
  open: boolean
  onClose: () => void
}

export default function GlobalSearch({ open, onClose }: GlobalSearchProps): React.ReactElement | null {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<GlobalSearchResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Reset state when opened
  useEffect(() => {
    if (open) {
      setQuery('')
      setResults(null)
      setActiveIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open])

  // Debounced search
  const runSearch = useCallback((q: string) => {
    if (!q.trim()) {
      setResults(null)
      setLoading(false)
      return
    }
    setLoading(true)
    globalSearch(q, 5)
      .then((res) => {
        setResults(res)
        setActiveIndex(0)
      })
      .catch(() => setResults(null))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, runSearch])

  // Build flat list of entries across sections (for keyboard nav)
  const sections = useMemo<Section[]>(() => {
    if (!results) return []
    const out: Section[] = []
    if (results.items?.length) {
      out.push({
        label: 'Items',
        icon: <Sword size={11} />,
        entries: results.items.map((item) => ({ kind: 'item' as const, item })),
      })
    }
    if (results.spells?.length) {
      out.push({
        label: 'Spells',
        icon: <Sparkles size={11} />,
        entries: results.spells
          .filter((s) => s.name.trim() !== '')
          .map((spell) => ({ kind: 'spell' as const, spell })),
      })
    }
    if (results.npcs?.length) {
      out.push({
        label: 'NPCs',
        icon: <Skull size={11} />,
        entries: results.npcs.map((npc) => ({ kind: 'npc' as const, npc })),
      })
    }
    if (results.zones?.length) {
      out.push({
        label: 'Zones',
        icon: <Map size={11} />,
        entries: results.zones.map((zone) => ({ kind: 'zone' as const, zone })),
      })
    }
    return out
  }, [results])

  const flatEntries = useMemo<ResultEntry[]>(
    () => sections.flatMap((s) => s.entries),
    [sections],
  )

  const navigateTo = useCallback(
    (entry: ResultEntry) => {
      onClose()
      if (entry.kind === 'item') {
        navigate(`/items?select=${entry.item.id}`)
      } else if (entry.kind === 'spell') {
        navigate(`/spells?select=${entry.spell.id}`)
      } else if (entry.kind === 'npc') {
        navigate(`/npcs?select=${entry.npc.id}`)
      } else {
        navigate(`/zones?select=${entry.zone.id}`)
      }
    },
    [navigate, onClose],
  )

  // Keyboard handler inside the modal
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
        return
      }
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIndex((i) => Math.min(i + 1, flatEntries.length - 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIndex((i) => Math.max(i - 1, 0))
      } else if (e.key === 'Enter') {
        const entry = flatEntries[activeIndex]
        if (entry) navigateTo(entry)
      }
    },
    [flatEntries, activeIndex, navigateTo, onClose],
  )

  if (!open) return null

  // Track flat index across sections
  let globalIdx = 0

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-24"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      {/* Modal panel */}
      <div
        className="flex w-full max-w-xl flex-col overflow-hidden rounded-lg shadow-2xl"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
        }}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Input row */}
        <div
          className="flex items-center gap-3 border-b px-4 py-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <Search size={16} style={{ color: 'var(--color-muted)', flexShrink: 0 }} />
          <input
            ref={inputRef}
            type="text"
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
            style={{ color: 'var(--color-foreground)' }}
            placeholder="Search items, spells, NPCs, zones…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            spellCheck={false}
          />
          <kbd
            className="shrink-0 rounded px-1.5 py-0.5 text-[10px]"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted)',
              border: '1px solid var(--color-border)',
            }}
          >
            esc
          </kbd>
        </div>

        {/* Results */}
        <div className="max-h-96 overflow-y-auto">
          {loading && (
            <p className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              Searching…
            </p>
          )}

          {!loading && query.trim() && !flatEntries.length && (
            <p className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No results for &ldquo;{query}&rdquo;
            </p>
          )}

          {!loading &&
            sections.map((section) => {
              const sectionStart = globalIdx
              globalIdx += section.entries.length
              return (
                <div key={section.label}>
                  {/* Section header */}
                  <div
                    className="flex items-center gap-1.5 px-4 py-1.5 text-[10px] font-semibold uppercase tracking-widest"
                    style={{
                      color: 'var(--color-muted)',
                      backgroundColor: 'var(--color-background)',
                      borderBottom: '1px solid var(--color-border)',
                    }}
                  >
                    {section.icon}
                    {section.label}
                  </div>
                  {section.entries.map((entry, i) => {
                    const idx = sectionStart + i
                    return (
                      <ResultRow
                        key={idx}
                        entry={entry}
                        active={activeIndex === idx}
                        onHover={() => setActiveIndex(idx)}
                        onClick={() => navigateTo(entry)}
                      />
                    )
                  })}
                </div>
              )
            })}

          {/* Hint when empty */}
          {!query.trim() && (
            <div className="px-4 py-6 text-center">
              <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                Type to search across items, spells, NPCs, and zones
              </p>
            </div>
          )}
        </div>

        {/* Footer */}
        {flatEntries.length > 0 && (
          <div
            className="flex items-center gap-4 border-t px-4 py-2"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
              <kbd
                className="rounded px-1 py-0.5"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                ↑↓
              </kbd>{' '}
              navigate
            </span>
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
              <kbd
                className="rounded px-1 py-0.5"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                ↵
              </kbd>{' '}
              open
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

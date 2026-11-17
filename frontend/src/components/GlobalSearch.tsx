import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, Sword, Sparkles, Skull, Map, X, Compass, Settings2, LifeBuoy, Settings, LayoutGrid } from 'lucide-react'
import { getConfig, globalSearch, saveConfigChanges, type GlobalSearchResult } from '../services/api'
import { NAV_SECTIONS } from '../lib/sidebarNav'
import { searchSettings, type SettingEntry } from '../lib/settingsIndex'
import type { Item } from '../types/item'
import type { Spell } from '../types/spell'
import type { NPC } from '../types/npc'
import type { Zone } from '../types/zone'
import { effectiveItemTypeLabel } from '../lib/itemHelpers'
import { castableClassesShort } from '../lib/spellHelpers'
import { npcDisplayName, className as npcClassName, npcLevelLabel } from '../lib/npcHelpers'
import { zoneListLabel } from './NPCZoneLinks'
import { ItemIcon, SpellIcon } from './Icon'
import { WILDCARD_HINT } from '../lib/textSearch'

// ── Types ──────────────────────────────────────────────────────────────────────

type ResultEntry =
  | { kind: 'item'; item: Item }
  | { kind: 'spell'; spell: Spell }
  | { kind: 'npc'; npc: NPC }
  | { kind: 'zone'; zone: Zone }
  | { kind: 'page'; page: PageEntry }
  | { kind: 'setting'; entry: SettingEntry }
  | { kind: 'help'; slug: string; title: string; snippet: string }

interface PageEntry {
  label: string
  to: string
  section: string
  icon: React.ReactNode
}

// Every navigable page: the sidebar's sections plus the fixed Help and
// Settings links.
const PAGES: PageEntry[] = [
  ...NAV_SECTIONS.flatMap((s) =>
    s.items.map((i) => ({ label: i.label, to: i.to, section: s.label, icon: i.icon })),
  ),
  { label: 'Help', to: '/help', section: 'Help', icon: <LifeBuoy size={16} /> },
  { label: 'Settings', to: '/settings', section: 'Settings', icon: <Settings size={16} /> },
]

function searchPages(query: string, limit = 5): PageEntry[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return []
  return PAGES.map((p) => {
    const label = p.label.toLowerCase()
    const hay = `${label} ${p.section.toLowerCase()}`
    if (!terms.every((t) => hay.includes(t))) return null
    const score = terms.reduce((n, t) => n + (label.startsWith(t) ? 10 : label.includes(t) ? 5 : 1), 0)
    return { p, score }
  })
    .filter((x): x is { p: PageEntry; score: number } => x !== null)
    .sort((a, b) => b.score - a.score)
    .slice(0, limit)
    .map((x) => x.p)
}

// startsWithQuery: does a local result's name start with what was typed?
// Then pages/settings rank above game data ("lockouts", "audio").
function startsWithQuery(name: string, query: string): boolean {
  const q = query.trim().toLowerCase()
  return q.length > 0 && name.toLowerCase().startsWith(q)
}

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
    subtitle = `Level ${npcLevelLabel(entry.npc)} ${npcClassName(entry.npc.class)}`
    if (entry.npc.zones?.length) subtitle += ` · ${zoneListLabel(entry.npc.zones, 1)}`
  } else if (entry.kind === 'zone') {
    name = entry.zone.long_name
    subtitle = entry.zone.short_name
  } else if (entry.kind === 'page') {
    name = entry.page.label
    subtitle = `Page · ${entry.page.section}`
  } else if (entry.kind === 'setting') {
    name = entry.entry.label
    subtitle = entry.entry.where
  } else {
    name = entry.title
    subtitle = entry.snippet || 'Help article'
  }

  let leadingIcon: React.ReactNode = null
  if (entry.kind === 'item') {
    leadingIcon = <ItemIcon id={entry.item.icon} name={entry.item.name} size={24} />
  } else if (entry.kind === 'spell') {
    leadingIcon = <SpellIcon id={entry.spell.new_icon} name={entry.spell.name} size={24} />
  } else if (entry.kind === 'page') {
    leadingIcon = <span style={{ color: 'var(--color-muted-foreground)' }}>{entry.page.icon}</span>
  } else if (entry.kind === 'setting') {
    leadingIcon = (
      <span style={{ color: 'var(--color-muted-foreground)' }}>
        {entry.entry.to.path === '/overlays' ? <LayoutGrid size={16} /> : <Settings2 size={16} />}
      </span>
    )
  } else if (entry.kind === 'help') {
    leadingIcon = <LifeBuoy size={16} style={{ color: 'var(--color-muted-foreground)' }} />
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

// markSearchUsed records the Getting Started "tried Ctrl+K" milestone the
// first time search opens.
let searchUsedMarked = false
function markSearchUsed(): void {
  if (searchUsedMarked) return
  searchUsedMarked = true
  getConfig()
    .then((c) => {
      const gs = c.preferences?.getting_started ?? {}
      if (gs.used_search_at) return
      return saveConfigChanges(
        {
          ...c,
          preferences: {
            ...c.preferences,
            getting_started: { ...gs, used_search_at: new Date().toISOString() },
          },
        },
        c,
      )
    })
    .catch(() => {
      searchUsedMarked = false
    })
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
  // Monotonic token so a slow earlier request can't clobber a newer one (or
  // repopulate after the query was cleared). Each run bumps it; only the latest
  // applies.
  const seqRef = useRef(0)
  // Help articles load on first open so they stay out of the startup chunk.
  const [helpMod, setHelpMod] = useState<typeof import('../lib/help') | null>(null)

  // Reset state when opened
  useEffect(() => {
    if (open) {
      setQuery('')
      setResults(null)
      setActiveIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
      if (!helpMod) void import('../lib/help').then(setHelpMod)
      markSearchUsed()
    }
  }, [open, helpMod])

  // Debounced search
  const runSearch = useCallback((q: string) => {
    const seq = ++seqRef.current
    if (!q.trim()) {
      setResults(null)
      setLoading(false)
      return
    }
    setLoading(true)
    globalSearch(q, 5)
      .then((res) => {
        if (seq !== seqRef.current) return
        setResults(res)
        setActiveIndex(0)
      })
      .catch(() => { if (seq === seqRef.current) setResults(null) })
      .finally(() => { if (seq === seqRef.current) setLoading(false) })
  }, [])

  // Local results (pages, settings, help) change on every keystroke.
  useEffect(() => setActiveIndex(0), [query])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, runSearch])

  // Build flat list of entries across sections (for keyboard nav)
  const sections = useMemo<Section[]>(() => {
    const out: Section[] = []
    const q = query.trim()
    const pages = q ? searchPages(q) : []
    const settings = q ? searchSettings(q, 5) : []
    const help = q && helpMod ? helpMod.searchHelp(q, 4) : []
    const local: Section[] = []
    if (pages.length) {
      local.push({
        label: 'Pages',
        icon: <Compass size={11} />,
        entries: pages.map((page) => ({ kind: 'page' as const, page })),
      })
    }
    if (settings.length) {
      local.push({
        label: 'Settings',
        icon: <Settings2 size={11} />,
        entries: settings.map((entry) => ({ kind: 'setting' as const, entry })),
      })
    }
    const localFirst =
      pages.some((p) => startsWithQuery(p.label, q)) ||
      settings.some((e) => startsWithQuery(e.label, q))
    if (localFirst) out.push(...local)
    const helpSection: Section | null = help.length
      ? {
          label: 'Help',
          icon: <LifeBuoy size={11} />,
          entries: help.map((h) => ({
            kind: 'help' as const,
            slug: h.article.slug,
            title: h.article.title,
            snippet: h.snippet,
          })),
        }
      : null
    if (!results) {
      if (!localFirst) out.push(...local)
      if (helpSection) out.push(helpSection)
      return out
    }
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
    if (!localFirst) out.push(...local)
    if (helpSection) out.push(helpSection)
    return out
  }, [results, query, helpMod])

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
      } else if (entry.kind === 'zone') {
        navigate(`/zones?select=${entry.zone.id}`)
      } else if (entry.kind === 'page') {
        navigate(entry.page.to)
      } else if (entry.kind === 'setting') {
        navigate(entry.entry.to.path + (entry.entry.to.search ?? ''))
      } else {
        navigate(`/help/${entry.slug}`)
      }
    },
    [navigate, onClose],
  )

  // Document-level keyboard handler so Escape/arrows/Enter work regardless of
  // which element inside the modal currently has focus.
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIndex((i) => Math.min(i + 1, flatEntries.length - 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIndex((i) => Math.max(i - 1, 0))
      } else if (e.key === 'Enter') {
        const entry = flatEntries[activeIndex]
        if (entry) navigateTo(entry)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, flatEntries, activeIndex, navigateTo, onClose])

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
            placeholder="Search items, spells, NPCs, zones, pages, settings, help…"
            title={WILDCARD_HINT}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            spellCheck={false}
          />
          <button
            type="button"
            onClick={onClose}
            aria-label="Close search"
            className="flex shrink-0 items-center justify-center rounded p-1 transition-colors hover:bg-(--color-surface-2)"
            style={{ color: 'var(--color-muted)' }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Results */}
        <div className="max-h-96 overflow-y-auto">
          {loading && (
            <p className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              Searching…
            </p>
          )}

          {!loading && query.trim() && !flatEntries.length && results !== null && (
            <p className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No results for &ldquo;{query}&rdquo;
            </p>
          )}

          {sections.map((section) => {
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
                Search game data, or jump to any page, setting or help article
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

import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search, X } from 'lucide-react'
import { searchItems, getItem } from '../services/api'
import type { Item } from '../types/item'
import {
  BANE_BODY_OPTIONS,
  baneBodyLabel,
  classesLabel,
  effectiveItemTypeLabel,
  isLoreItem,
  itemTypeLabel,
  priceLabel,
  racesLabel,
  sizeLabel,
  slotsLabel,
  weightLabel,
} from '../lib/itemHelpers'

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (item: Item) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [baneBody, setBaneBody] = useState(0)
  const [items, setItems] = useState<Item[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string, body: number) => {
    setLoading(true)
    setError(null)
    searchItems(q, 50, 0, body)
      .then((res) => {
        setItems(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, baneBody), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, baneBody, runSearch])

  // Run initial search on mount
  useEffect(() => {
    runSearch('', 0)
  }, [runSearch])

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Search input */}
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search items…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          spellCheck={false}
        />
        {query && (
          <button onClick={() => setQuery('')} className="shrink-0">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        )}
      </div>

      {/* Bane filter */}
      <div
        className="flex items-center gap-2 border-b px-3 py-1.5"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <span className="shrink-0 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Bane vs
        </span>
        <select
          className="flex-1 bg-transparent text-[11px] outline-none"
          style={{ color: 'var(--color-foreground)' }}
          value={baneBody}
          onChange={(e) => setBaneBody(Number(e.target.value))}
        >
          {BANE_BODY_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} items`}
      </div>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          items.map((item) => (
            <button
              key={item.id}
              onClick={() => onSelect(item)}
              className="w-full px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === item.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === item.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <div
                className="truncate text-sm font-medium"
                style={{
                  color:
                    selectedId === item.id
                      ? 'var(--color-primary)'
                      : 'var(--color-foreground)',
                }}
              >
                {item.name}
              </div>
              <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {effectiveItemTypeLabel(item.item_class, item.item_type)}
                {item.req_level > 0 && ` · Req ${item.req_level}`}
              </div>
            </button>
          ))}
      </div>
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps): React.ReactElement {
  return (
    <div className="flex justify-between py-0.5 text-sm">
      <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps): React.ReactElement {
  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      <div
        className="rounded border px-3 py-1"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {children}
      </div>
    </div>
  )
}

interface DetailPanelProps {
  item: Item | null
}

function DetailPanel({ item }: DetailPanelProps): React.ReactElement {
  if (!item) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select an item to view details
        </p>
      </div>
    )
  }

  const flags: string[] = []
  if (item.magic) flags.push('MAGIC')
  if (isLoreItem(item.lore)) flags.push('LORE')
  if (item.nodrop === 0) flags.push('NO DROP')
  if (item.norent === 0) flags.push('NO RENT')

  const hasCombat = item.damage > 0 || item.ac > 0
  const hasBane = item.bane_amt > 0 || item.bane_body > 0 || item.bane_race > 0
  const hasStats =
    item.hp > 0 || item.mana > 0 || item.str > 0 || item.sta > 0 || item.agi > 0 ||
    item.dex > 0 || item.wis > 0 || item.int > 0 || item.cha > 0
  const hasResists = item.mr > 0 || item.cr > 0 || item.dr > 0 || item.fr > 0 || item.pr > 0
  const hasEffects =
    (item.click_effect > 0 && !!item.click_name) ||
    (item.proc_effect > 0 && !!item.proc_name) ||
    (item.worn_effect > 0 && !!item.worn_name) ||
    (item.focus_effect > 0 && !!item.focus_name)

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {/* Header */}
      <div className="mb-4">
        <h2
          className="text-xl font-bold leading-tight"
          style={{ color: 'var(--color-primary)' }}
        >
          {item.name}
        </h2>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            {effectiveItemTypeLabel(item.item_class, item.item_type)}
          </span>
          {flags.map((f) => (
            <span
              key={f}
              className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-primary)',
                border: '1px solid var(--color-border)',
              }}
            >
              {f}
            </span>
          ))}
        </div>
        {item.lore && (
          <p
            className="mt-1.5 text-xs italic"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            {item.lore.startsWith('*') ? item.lore.slice(1) : item.lore}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-3">
        {/* Combat */}
        {hasCombat && (
          <Section title="Combat">
            {item.damage > 0 && (
              <StatRow label="Damage / Delay" value={`${item.damage} / ${item.delay}`} />
            )}
            {item.range > 0 && <StatRow label="Range" value={item.range} />}
            {item.ac > 0 && <StatRow label="AC" value={item.ac} />}
          </Section>
        )}

        {/* Bane Damage */}
        {hasBane && (
          <Section title="Bane Damage">
            {item.bane_amt > 0 && <StatRow label="Bane Damage" value={`+${item.bane_amt}`} />}
            {item.bane_body > 0 && (
              <StatRow label="Bane vs Body" value={baneBodyLabel(item.bane_body)} />
            )}
            {item.bane_race > 0 && <StatRow label="Bane vs Race" value={item.bane_race} />}
          </Section>
        )}

        {/* Stats */}
        {hasStats && (
          <Section title="Stats">
            {item.hp > 0 && <StatRow label="HP" value={`+${item.hp}`} />}
            {item.mana > 0 && <StatRow label="Mana" value={`+${item.mana}`} />}
            {item.str > 0 && <StatRow label="STR" value={`+${item.str}`} />}
            {item.sta > 0 && <StatRow label="STA" value={`+${item.sta}`} />}
            {item.agi > 0 && <StatRow label="AGI" value={`+${item.agi}`} />}
            {item.dex > 0 && <StatRow label="DEX" value={`+${item.dex}`} />}
            {item.wis > 0 && <StatRow label="WIS" value={`+${item.wis}`} />}
            {item.int > 0 && <StatRow label="INT" value={`+${item.int}`} />}
            {item.cha > 0 && <StatRow label="CHA" value={`+${item.cha}`} />}
          </Section>
        )}

        {/* Resists */}
        {hasResists && (
          <Section title="Resists">
            {item.mr > 0 && <StatRow label="Magic" value={`+${item.mr}`} />}
            {item.cr > 0 && <StatRow label="Cold" value={`+${item.cr}`} />}
            {item.dr > 0 && <StatRow label="Disease" value={`+${item.dr}`} />}
            {item.fr > 0 && <StatRow label="Fire" value={`+${item.fr}`} />}
            {item.pr > 0 && <StatRow label="Poison" value={`+${item.pr}`} />}
          </Section>
        )}

        {/* Effects */}
        {hasEffects && (
          <Section title="Effects">
            {item.click_effect > 0 && item.click_name && (
              <StatRow label="Click" value={item.click_name} />
            )}
            {item.proc_effect > 0 && item.proc_name && (
              <StatRow label="Proc" value={item.proc_name} />
            )}
            {item.worn_effect > 0 && item.worn_name && (
              <StatRow label="Worn" value={item.worn_name} />
            )}
            {item.focus_effect > 0 && item.focus_name && (
              <StatRow label="Focus" value={item.focus_name} />
            )}
          </Section>
        )}

        {/* Restrictions */}
        <Section title="Restrictions">
          {(item.req_level > 0 || item.rec_level > 0) && (
            <>
              {item.req_level > 0 && (
                <StatRow label="Required Level" value={item.req_level} />
              )}
              {item.rec_level > 0 && (
                <StatRow label="Recommended Level" value={item.rec_level} />
              )}
            </>
          )}
          <StatRow label="Slots" value={slotsLabel(item.slots)} />
          <StatRow label="Classes" value={classesLabel(item.classes)} />
          <StatRow label="Races" value={racesLabel(item.races)} />
        </Section>

        {/* Info */}
        <Section title="Info">
          <StatRow label="Weight" value={weightLabel(item.weight)} />
          <StatRow label="Size" value={sizeLabel(item.size)} />
          {item.stackable > 0 && item.stack_size > 1 && (
            <StatRow label="Stack Size" value={item.stack_size} />
          )}
          {item.item_class === 1 && (
            <>
              <StatRow label="Bag Slots" value={item.bag_slots} />
              <StatRow label="Bag Size" value={sizeLabel(item.bag_size)} />
            </>
          )}
          {item.price > 0 && <StatRow label="Value" value={priceLabel(item.price)} />}
          <StatRow label="Item ID" value={item.id} />
        </Section>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function ItemsPage(): React.ReactElement {
  const [selected, setSelected] = useState<Item | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getItem(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel item={selected} />
    </div>
  )
}

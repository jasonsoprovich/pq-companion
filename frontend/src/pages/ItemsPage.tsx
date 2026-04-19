import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Search, X } from 'lucide-react'
import { searchItems, getItem, getItemSources } from '../services/api'
import type { Item, ItemSourceNPC, ItemSources } from '../types/item'
import {
  baneBodyLabel,
  baneRaceLabel,
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
  const [items, setItems] = useState<Item[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string) => {
    setLoading(true)
    setError(null)
    searchItems(q, 50, 0, 0)
      .then((res) => {
        setItems(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, runSearch])

  // Run initial search on mount
  useEffect(() => {
    runSearch('')
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
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span className="min-w-0 break-words text-right" style={{ color: 'var(--color-foreground)' }}>{value}</span>
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

function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

interface SpellEffectRowProps {
  label: string
  spellId: number
  name: string
}

function SpellEffectRow({ label, spellId, name }: SpellEffectRowProps): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <button
        onClick={() => navigate(`/spells?select=${spellId}`)}
        className="min-w-0 truncate text-right underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
        title="View spell details"
      >
        {name}
      </button>
    </div>
  )
}

interface SourceNPCLinkProps {
  npc: ItemSourceNPC
}

function SourceNPCLink({ npc }: SourceNPCLinkProps): React.ReactElement {
  const navigate = useNavigate()
  return (
    <button
      onClick={() => navigate(`/npcs?select=${npc.id}`)}
      className="flex w-full items-center justify-between gap-3 py-0.5 text-sm text-left"
    >
      <span
        className="truncate underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
      >
        {formatNPCName(npc.name)}
      </span>
      {npc.zone_name && (
        <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
          {npc.zone_name}
        </span>
      )}
    </button>
  )
}

interface DetailPanelProps {
  item: Item | null
}

function DetailPanel({ item }: DetailPanelProps): React.ReactElement {
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!item) { setSources(null); return }
    getItemSources(item.id)
      .then(setSources)
      .catch(() => setSources({ drops: [], merchants: [] }))
  }, [item?.id])

  function copyIngameLink() {
    if (!item) return
    const link = `\\aITEM ${item.id} 0 0 0 0 0:${item.name}\\a/`
    navigator.clipboard.writeText(link).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

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
        <div className="flex items-start justify-between gap-2">
          <h2
            className="text-xl font-bold leading-tight"
            style={{ color: 'var(--color-primary)' }}
          >
            {item.name}
          </h2>
          <button
            onClick={copyIngameLink}
            title="Copy In-Game Link"
            className="flex shrink-0 items-center gap-1.5 rounded border px-2 py-1 text-[11px] font-medium transition-colors"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: 'var(--color-border)',
              color: copied ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            }}
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
            {copied ? 'Copied!' : 'Copy Link'}
          </button>
        </div>
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
            {item.bane_race > 0 && <StatRow label="Bane vs Race" value={baneRaceLabel(item.bane_race)} />}
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
              <SpellEffectRow label="Click" spellId={item.click_effect} name={item.click_name} />
            )}
            {item.proc_effect > 0 && item.proc_name && (
              <SpellEffectRow label="Proc" spellId={item.proc_effect} name={item.proc_name} />
            )}
            {item.worn_effect > 0 && item.worn_name && (
              <SpellEffectRow label="Worn" spellId={item.worn_effect} name={item.worn_name} />
            )}
            {item.focus_effect > 0 && item.focus_name && (
              <SpellEffectRow label="Focus" spellId={item.focus_effect} name={item.focus_name} />
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

        {/* Sources */}
        {sources && (sources.drops.length > 0 || sources.merchants.length > 0) && (
          <Section title="Item Sources">
            {sources.drops.length > 0 && (
              <>
                <div className="pb-0.5 pt-1 text-[11px] font-medium" style={{ color: 'var(--color-muted)' }}>
                  Dropped by
                </div>
                {sources.drops.map((npc) => (
                  <SourceNPCLink key={npc.id} npc={npc} />
                ))}
              </>
            )}
            {sources.merchants.length > 0 && (
              <>
                <div className={`pb-0.5 text-[11px] font-medium ${sources.drops.length > 0 ? 'pt-2' : 'pt-1'}`} style={{ color: 'var(--color-muted)' }}>
                  Sold by
                </div>
                {sources.merchants.map((npc) => (
                  <SourceNPCLink key={npc.id} npc={npc} />
                ))}
              </>
            )}
          </Section>
        )}
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
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel item={selected} />
    </div>
  )
}

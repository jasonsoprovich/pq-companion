import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Search, X } from 'lucide-react'
import { searchItems, getItem, getItemSources } from '../services/api'
import type { Item, ItemSourceNPC, ItemSources, ItemForageZone, ItemGroundSpawnZone, ItemTradeskillEntry } from '../types/item'
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
  showRate?: boolean
}

function SourceNPCLink({ npc, showRate }: SourceNPCLinkProps): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex w-full items-center justify-between gap-3 py-0.5 text-sm">
      <button
        onClick={() => navigate(`/npcs?select=${npc.id}`)}
        className="min-w-0 truncate text-left underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
      >
        {formatNPCName(npc.name)}
      </button>
      <div className="flex shrink-0 items-center gap-2">
        {showRate && npc.drop_rate != null && npc.drop_rate > 0 && (
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {npc.drop_rate.toFixed(2)}%
          </span>
        )}
        {npc.zone_name && (
          <button
            onClick={() => navigate(`/zones?select=${npc.zone_short_name}`)}
            className="text-xs underline decoration-dotted"
            style={{ color: 'var(--color-muted)' }}
          >
            {npc.zone_name}
          </button>
        )}
      </div>
    </div>
  )
}

function EmptyTabMessage({ message }: { message: string }): React.ReactElement {
  return (
    <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>{message}</p>
  )
}

function tradeskillLabel(id: number): string {
  const labels: Record<number, string> = {
    0: 'No Tradeskill',
    55: 'Fishing',
    56: 'Make Poison',
    57: 'Smithing',
    58: 'Tailoring',
    59: 'Baking',
    60: 'Alchemy',
    61: 'Brewing',
    63: 'Research',
    64: 'Pottery',
    65: 'Fletching',
    68: 'Jewelry Making',
    69: 'Tinkering',
    75: 'Begging',
  }
  return labels[id] ?? `Tradeskill ${id}`
}

// ── Tab: Overview ──────────────────────────────────────────────────────────────

interface OverviewTabProps {
  item: Item
  copied: boolean
  onCopy: () => void
}

function OverviewTab({ item, copied, onCopy }: OverviewTabProps): React.ReactElement {
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
    <div className="flex flex-col gap-3">
      {/* Item header in tab */}
      <div>
        <div className="flex items-start justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
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
          <button
            onClick={onCopy}
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
        {item.lore && (
          <p className="mt-1.5 text-xs italic" style={{ color: 'var(--color-muted-foreground)' }}>
            {item.lore.startsWith('*') ? item.lore.slice(1) : item.lore}
          </p>
        )}
      </div>

      {hasCombat && (
        <Section title="Combat">
          {item.damage > 0 && (
            <StatRow label="Damage / Delay" value={`${item.damage} / ${item.delay}`} />
          )}
          {item.range > 0 && <StatRow label="Range" value={item.range} />}
          {item.ac > 0 && <StatRow label="AC" value={item.ac} />}
        </Section>
      )}

      {hasBane && (
        <Section title="Bane Damage">
          {item.bane_amt > 0 && <StatRow label="Bane Damage" value={`+${item.bane_amt}`} />}
          {item.bane_body > 0 && <StatRow label="Bane vs Body" value={baneBodyLabel(item.bane_body)} />}
          {item.bane_race > 0 && <StatRow label="Bane vs Race" value={baneRaceLabel(item.bane_race)} />}
        </Section>
      )}

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

      {hasResists && (
        <Section title="Resists">
          {item.mr > 0 && <StatRow label="Magic" value={`+${item.mr}`} />}
          {item.cr > 0 && <StatRow label="Cold" value={`+${item.cr}`} />}
          {item.dr > 0 && <StatRow label="Disease" value={`+${item.dr}`} />}
          {item.fr > 0 && <StatRow label="Fire" value={`+${item.fr}`} />}
          {item.pr > 0 && <StatRow label="Poison" value={`+${item.pr}`} />}
        </Section>
      )}

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

      <Section title="Restrictions">
        {item.req_level > 0 && <StatRow label="Required Level" value={item.req_level} />}
        {item.rec_level > 0 && <StatRow label="Recommended Level" value={item.rec_level} />}
        <StatRow label="Slots" value={slotsLabel(item.slots)} />
        <StatRow label="Classes" value={classesLabel(item.classes)} />
        <StatRow label="Races" value={racesLabel(item.races)} />
      </Section>

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
  )
}

// ── Tab: Drops From ────────────────────────────────────────────────────────────

function DropsFromTab({ drops }: { drops: ItemSourceNPC[] }): React.ReactElement {
  if (drops.length === 0) return <EmptyTabMessage message="No drop sources found." />
  return (
    <div>
      <div className="mb-1 flex justify-between text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        <span>NPC</span>
        <span>Drop Rate / Zone</span>
      </div>
      {drops.map((npc) => (
        <SourceNPCLink key={npc.id} npc={npc} showRate />
      ))}
    </div>
  )
}

// ── Tab: Purchased From ────────────────────────────────────────────────────────

function PurchasedFromTab({ merchants }: { merchants: ItemSourceNPC[] }): React.ReactElement {
  if (merchants.length === 0) return <EmptyTabMessage message="Not sold by any merchant." />
  return (
    <div>
      {merchants.map((npc) => (
        <SourceNPCLink key={npc.id} npc={npc} />
      ))}
    </div>
  )
}

// ── Tab: Foraged From ──────────────────────────────────────────────────────────

function ForagedFromTab({ zones }: { zones: ItemForageZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (zones.length === 0) return <EmptyTabMessage message="Not forageable in any zone." />
  return (
    <div>
      {zones.map((fz, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button
            onClick={() => navigate(`/zones?select=${fz.zone_short_name}`)}
            className="min-w-0 truncate text-left underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {fz.zone_name || fz.zone_short_name}
          </button>
          {fz.chance > 0 && (
            <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {fz.chance}%
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

// ── Tab: Ground Spawns ─────────────────────────────────────────────────────────

function GroundSpawnsTab({ spawns }: { spawns: ItemGroundSpawnZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (spawns.length === 0) return <EmptyTabMessage message="No ground spawns found." />
  return (
    <div>
      {spawns.map((gs, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button
            onClick={() => navigate(`/zones?select=${gs.zone_short_name}`)}
            className="min-w-0 truncate text-left underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {gs.zone_name || gs.zone_short_name}
          </button>
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
            {gs.name}
          </span>
        </div>
      ))}
    </div>
  )
}

// ── Tab: Tradeskills ───────────────────────────────────────────────────────────

function TradeskillsTab({ entries }: { entries: ItemTradeskillEntry[] }): React.ReactElement {
  if (entries.length === 0) return <EmptyTabMessage message="Not used in any tradeskill recipe." />
  const products = entries.filter((e) => e.role === 'product')
  const ingredients = entries.filter((e) => e.role === 'ingredient')
  return (
    <div className="flex flex-col gap-3">
      {products.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Produced by
          </div>
          {products.map((ts) => (
            <div key={ts.recipe_id} className="flex items-center justify-between gap-3 py-0.5 text-sm">
              <span style={{ color: 'var(--color-foreground)' }}>{ts.recipe_name}</span>
              <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                <span>{tradeskillLabel(ts.tradeskill)}</span>
                <span>Trivial {ts.trivial}</span>
              </div>
            </div>
          ))}
        </div>
      )}
      {ingredients.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Used as ingredient in
          </div>
          {ingredients.map((ts) => (
            <div key={ts.recipe_id} className="flex items-center justify-between gap-3 py-0.5 text-sm">
              <span style={{ color: 'var(--color-foreground)' }}>{ts.recipe_name}</span>
              <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                <span>{tradeskillLabel(ts.tradeskill)}</span>
                {ts.count > 1 && <span>×{ts.count}</span>}
                <span>Trivial {ts.trivial}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

type ItemTabKey = 'overview' | 'drops' | 'merchants' | 'forage' | 'ground-spawns' | 'tradeskills'

const ITEM_TABS: { key: ItemTabKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'drops', label: 'Drops From' },
  { key: 'merchants', label: 'Purchased From' },
  { key: 'forage', label: 'Foraged From' },
  { key: 'ground-spawns', label: 'Ground Spawns' },
  { key: 'tradeskills', label: 'Tradeskills' },
]

interface DetailPanelProps {
  item: Item | null
}

function DetailPanel({ item }: DetailPanelProps): React.ReactElement {
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [activeTab, setActiveTab] = useState<ItemTabKey>('overview')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setActiveTab('overview')
    if (!item) { setSources(null); return }
    getItemSources(item.id)
      .then(setSources)
      .catch(() => setSources({ drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] }))
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

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Header */}
      <div
        className="shrink-0 border-b px-5 pt-4 pb-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <h2 className="text-xl font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
          {item.name}
        </h2>

        {/* Tabs */}
        <div className="mt-3 flex gap-0 overflow-x-auto">
          {ITEM_TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className="shrink-0 px-3 py-1.5 text-xs font-medium transition-colors"
              style={{
                color: activeTab === tab.key ? 'var(--color-primary)' : 'var(--color-muted)',
                borderBottom:
                  activeTab === tab.key
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto px-5 py-4">
        {activeTab === 'overview' && (
          <OverviewTab item={item} copied={copied} onCopy={copyIngameLink} />
        )}
        {activeTab === 'drops' && (
          <DropsFromTab drops={sources?.drops ?? []} />
        )}
        {activeTab === 'merchants' && (
          <PurchasedFromTab merchants={sources?.merchants ?? []} />
        )}
        {activeTab === 'forage' && (
          <ForagedFromTab zones={sources?.forage_zones ?? []} />
        )}
        {activeTab === 'ground-spawns' && (
          <GroundSpawnsTab spawns={sources?.ground_spawns ?? []} />
        )}
        {activeTab === 'tradeskills' && (
          <TradeskillsTab entries={sources?.tradeskills ?? []} />
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

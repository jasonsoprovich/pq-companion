import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Check, Copy, X } from 'lucide-react'
import { getItemSources } from '../services/api'
import type { Item, ItemForageZone, ItemGroundSpawnZone, ItemSourceNPC, ItemSources, ItemTradeskillEntry } from '../types/item'
import {
  baneBodyLabel,
  baneRaceLabel,
  classesLabel,
  effectiveItemTypeLabel,
  inGameItemLink,
  isLoreItem,
  priceLabel,
  racesLabel,
  sizeLabel,
  slotsLabel,
  weightLabel,
} from '../lib/itemHelpers'
import { ItemIcon } from './Icon'

// ── Shared primitives ──────────────────────────────────────────────────────────

function StatRow({ label, value }: { label: string; value: string | number }): React.ReactElement {
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span className="min-w-0 break-words text-right" style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }): React.ReactElement {
  return (
    <div>
      <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        {title}
      </div>
      <div className="rounded border px-3 py-1" style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}>
        {children}
      </div>
    </div>
  )
}

function EmptyTabMessage({ message }: { message: string }): React.ReactElement {
  return <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>{message}</p>
}

function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

function tradeskillLabel(id: number): string {
  const labels: Record<number, string> = {
    0: 'No Tradeskill', 55: 'Fishing', 56: 'Make Poison', 57: 'Smithing',
    58: 'Tailoring', 59: 'Baking', 60: 'Alchemy', 61: 'Brewing',
    63: 'Research', 64: 'Pottery', 65: 'Fletching', 68: 'Jewelry Making',
    69: 'Tinkering', 75: 'Begging',
  }
  return labels[id] ?? `Tradeskill ${id}`
}

// ── Tab content ────────────────────────────────────────────────────────────────

function SpellEffectRow({ label, spellId, name }: { label: string; spellId: number; name: string }): React.ReactElement {
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

function SourceNPCLink({ npc, showRate }: { npc: ItemSourceNPC; showRate?: boolean }): React.ReactElement {
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

function OverviewTab({ item, copied, onCopy }: { item: Item; copied: boolean; onCopy: () => void }): React.ReactElement {
  const flags: string[] = []
  if (item.magic) flags.push('MAGIC')
  if (isLoreItem(item.lore)) flags.push('LORE')
  if (item.nodrop === 0) flags.push('NO DROP')
  if (item.norent === 0) flags.push('NO RENT')

  const hasCombat = item.damage > 0 || item.ac > 0
  const hasBane = item.bane_amt > 0 || item.bane_body > 0 || item.bane_race > 0
  const hasStats = item.hp > 0 || item.mana > 0 || item.str > 0 || item.sta > 0 || item.agi > 0 || item.dex > 0 || item.wis > 0 || item.int > 0 || item.cha > 0
  const hasResists = item.mr > 0 || item.cr > 0 || item.dr > 0 || item.fr > 0 || item.pr > 0
  const hasEffects =
    (item.click_effect > 0 && !!item.click_name) ||
    (item.proc_effect > 0 && !!item.proc_name) ||
    (item.worn_effect > 0 && !!item.worn_name) ||
    (item.focus_effect > 0 && !!item.focus_name)

  return (
    <div className="flex flex-col gap-3">
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
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)', border: '1px solid var(--color-border)' }}
              >
                {f}
              </span>
            ))}
          </div>
          <button
            onClick={onCopy}
            title="Copy In-Game Link"
            className="flex shrink-0 items-center gap-1.5 rounded border px-2 py-1 text-[11px] font-medium transition-colors"
            style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)', color: copied ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }}
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
          {item.damage > 0 && <StatRow label="Damage / Delay" value={`${item.damage} / ${item.delay}`} />}
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
          {item.click_effect > 0 && item.click_name && <SpellEffectRow label="Click" spellId={item.click_effect} name={item.click_name} />}
          {item.proc_effect > 0 && item.proc_name && <SpellEffectRow label="Proc" spellId={item.proc_effect} name={item.proc_name} />}
          {item.worn_effect > 0 && item.worn_name && <SpellEffectRow label="Worn" spellId={item.worn_effect} name={item.worn_name} />}
          {item.focus_effect > 0 && item.focus_name && <SpellEffectRow label="Focus" spellId={item.focus_effect} name={item.focus_name} />}
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
        {item.stackable > 0 && item.stack_size > 1 && <StatRow label="Stack Size" value={item.stack_size} />}
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

function DropsFromTab({ drops }: { drops: ItemSourceNPC[] }): React.ReactElement {
  if (drops.length === 0) return <EmptyTabMessage message="No drop sources found." />
  return (
    <div>
      <div className="mb-1 flex justify-between text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        <span>NPC</span>
        <span>Drop Rate / Zone</span>
      </div>
      {drops.map((npc) => <SourceNPCLink key={npc.id} npc={npc} showRate />)}
    </div>
  )
}

function PurchasedFromTab({ merchants }: { merchants: ItemSourceNPC[] }): React.ReactElement {
  if (merchants.length === 0) return <EmptyTabMessage message="Not sold by any merchant." />
  return <div>{merchants.map((npc) => <SourceNPCLink key={npc.id} npc={npc} />)}</div>
}

function ForagedFromTab({ zones }: { zones: ItemForageZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (zones.length === 0) return <EmptyTabMessage message="Not forageable in any zone." />
  return (
    <div>
      {zones.map((fz, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button onClick={() => navigate(`/zones?select=${fz.zone_short_name}`)} className="min-w-0 truncate text-left underline decoration-dotted" style={{ color: 'var(--color-primary)' }}>
            {fz.zone_name || fz.zone_short_name}
          </button>
          {fz.chance > 0 && <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{fz.chance}%</span>}
        </div>
      ))}
    </div>
  )
}

function GroundSpawnsTab({ spawns }: { spawns: ItemGroundSpawnZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (spawns.length === 0) return <EmptyTabMessage message="No ground spawns found." />
  return (
    <div>
      {spawns.map((gs, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button onClick={() => navigate(`/zones?select=${gs.zone_short_name}`)} className="min-w-0 truncate text-left underline decoration-dotted" style={{ color: 'var(--color-primary)' }}>
            {gs.zone_name || gs.zone_short_name}
          </button>
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>{gs.name}</span>
        </div>
      ))}
    </div>
  )
}

function TradeskillsTab({ entries }: { entries: ItemTradeskillEntry[] }): React.ReactElement {
  if (entries.length === 0) return <EmptyTabMessage message="Not used in any tradeskill recipe." />
  const products = entries.filter((e) => e.role === 'product')
  const ingredients = entries.filter((e) => e.role === 'ingredient')
  return (
    <div className="flex flex-col gap-3">
      {products.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Produced by</div>
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
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Used as ingredient in</div>
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

// ── Modal ──────────────────────────────────────────────────────────────────────

type TabKey = 'overview' | 'drops' | 'merchants' | 'forage' | 'ground-spawns' | 'tradeskills'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'drops', label: 'Drops From' },
  { key: 'merchants', label: 'Purchased From' },
  { key: 'forage', label: 'Foraged From' },
  { key: 'ground-spawns', label: 'Ground Spawns' },
  { key: 'tradeskills', label: 'Tradeskills' },
]

interface ItemDetailModalProps {
  item: Item | null
  open: boolean
  onClose: () => void
}

export default function ItemDetailModal({ item, open, onClose }: ItemDetailModalProps): React.ReactElement | null {
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [activeTab, setActiveTab] = useState<TabKey>('overview')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setActiveTab('overview')
    setSources(null)
    if (!item) return
    getItemSources(item.id)
      .then(setSources)
      .catch(() => setSources({ drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] }))
  }, [item?.id])

  useEffect(() => {
    if (!open) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open || !item) return null

  function copyIngameLink() {
    if (!item) return
    navigator.clipboard.writeText(inGameItemLink(item.id, item.name)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="flex flex-col rounded-lg shadow-2xl w-full max-w-xl max-h-[80vh]"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="shrink-0 border-b px-5 pt-4 pb-0" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex items-start justify-between gap-2 mb-2">
            <div className="flex items-start gap-3 min-w-0">
              <ItemIcon id={item.icon} name={item.name} size={36} />
              <h2 className="text-xl font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
                {item.name}
              </h2>
            </div>
            <button onClick={onClose} className="shrink-0 mt-0.5" title="Close">
              <X size={16} style={{ color: 'var(--color-muted)' }} />
            </button>
          </div>
          {/* Tabs */}
          <div className="flex gap-0 overflow-x-auto">
            {TABS.map((tab) => (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                className="shrink-0 px-3 py-1.5 text-xs font-medium transition-colors"
                style={{
                  color: activeTab === tab.key ? 'var(--color-primary)' : 'var(--color-muted)',
                  borderBottom: activeTab === tab.key ? '2px solid var(--color-primary)' : '2px solid transparent',
                }}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {activeTab === 'overview' && <OverviewTab item={item} copied={copied} onCopy={copyIngameLink} />}
          {activeTab === 'drops' && <DropsFromTab drops={sources?.drops ?? []} />}
          {activeTab === 'merchants' && <PurchasedFromTab merchants={sources?.merchants ?? []} />}
          {activeTab === 'forage' && <ForagedFromTab zones={sources?.forage_zones ?? []} />}
          {activeTab === 'ground-spawns' && <GroundSpawnsTab spawns={sources?.ground_spawns ?? []} />}
          {activeTab === 'tradeskills' && <TradeskillsTab entries={sources?.tradeskills ?? []} />}
        </div>
      </div>
    </div>
  )
}

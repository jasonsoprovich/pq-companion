import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, ExternalLink, AlertCircle, Search, X } from 'lucide-react'
import { getAllInventories, getItem } from '../services/api'
import type { AllInventoriesResponse, Inventory, InventoryEntry } from '../types/zeal'
import type { Item } from '../types/item'
import ItemDetailModal from '../components/ItemDetailModal'

// ── Types ──────────────────────────────────────────────────────────────────────

interface TaggedEntry extends InventoryEntry {
  character: string
}

// ── Slot ordering ──────────────────────────────────────────────────────────────

const EQUIPMENT_ORDER = [
  'Charm', 'Ear1', 'Ear2', 'Head', 'Face', 'Neck', 'Shoulders', 'Arms',
  'Wrists', 'Wrist', 'Hands', 'Primary', 'Secondary', 'Range', 'Ammo',
  'Finger1', 'Finger2', 'Ring1', 'Ring2', 'Chest', 'Back', 'Waist', 'Legs', 'Feet',
]

// ── Entry classifiers ──────────────────────────────────────────────────────────

function bagNumber(location: string): number | null {
  const m = location.match(/^General(\d+)/)
  return m ? parseInt(m[1], 10) : null
}

function isBagSlot(location: string): boolean {
  return bagNumber(location) !== null && location.includes(':')
}

function isBagContainer(location: string): boolean {
  return bagNumber(location) !== null && !location.includes(':')
}

function isBank(location: string): boolean {
  return location.startsWith('Bank')
}

function isEquipment(location: string): boolean {
  return !isBagSlot(location) && !isBagContainer(location) && !isBank(location) && location !== 'Cursor'
}

// ── Grouping ───────────────────────────────────────────────────────────────────

interface BagGroup {
  num: number
  character: string
  bag: TaggedEntry | null
  slots: TaggedEntry[]
}

function groupBags(entries: TaggedEntry[]): BagGroup[] {
  const map = new Map<string, BagGroup>()
  for (const e of entries) {
    const num = bagNumber(e.location)
    if (num === null) continue
    const key = `${e.character}::${num}`
    if (!map.has(key)) map.set(key, { num, character: e.character, bag: null, slots: [] })
    const g = map.get(key)!
    if (e.location.includes(':')) {
      g.slots.push(e)
    } else {
      g.bag = e
    }
  }
  // Sort slots within each bag.
  for (const g of map.values()) {
    g.slots.sort((a, b) => {
      const an = parseInt(a.location.split(':Slot')[1] ?? '0', 10)
      const bn = parseInt(b.location.split(':Slot')[1] ?? '0', 10)
      return an - bn
    })
  }
  return [...map.values()].sort((a, b) => {
    if (a.character !== b.character) return a.character.localeCompare(b.character)
    return a.num - b.num
  })
}

// ── Helpers ────────────────────────────────────────────────────────────────────

function tagEntries(inv: Inventory): TaggedEntry[] {
  return inv.entries.map((e) => ({ ...e, character: inv.character }))
}

function filterByQuery(entries: TaggedEntry[], query: string): TaggedEntry[] {
  if (!query) return entries
  const q = query.toLowerCase()
  return entries.filter((e) => e.name.toLowerCase().includes(q))
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface ItemRowProps {
  entry: TaggedEntry
  showCharBadge: boolean
  indent?: boolean
  onLookup: (id: number) => void
}

function ItemRow({ entry, showCharBadge, indent = false, onLookup }: ItemRowProps): React.ReactElement {
  return (
    <div
      className={`group flex items-center gap-2 px-3 py-1.5 ${indent ? 'pl-8' : ''}`}
      style={{ borderBottom: '1px solid var(--color-border)' }}
    >
      <div className="flex-1 min-w-0 flex items-center gap-2">
        <span className="text-sm truncate" style={{ color: 'var(--color-foreground)' }}>
          {entry.name}
        </span>
        {entry.count > 1 && (
          <span className="text-xs shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            ×{entry.count}
          </span>
        )}
        {showCharBadge && (
          <span
            className="shrink-0 text-[10px] px-1.5 py-0.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {entry.character}
          </span>
        )}
      </div>
      <button
        onClick={() => onLookup(entry.id)}
        className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
        title="View item details"
      >
        <ExternalLink size={12} style={{ color: 'var(--color-muted)' }} />
      </button>
    </div>
  )
}

interface SectionHeaderProps {
  label: string
  count: number
}

function SectionHeader({ label, count }: SectionHeaderProps): React.ReactElement {
  return (
    <div
      className="flex items-center justify-between px-3 py-1.5 sticky top-0 z-10"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderBottom: '1px solid var(--color-border)',
      }}
    >
      <span
        className="text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {label}
      </span>
      <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
        {count}
      </span>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function InventoryTrackerPage(): React.ReactElement {
  const [data, setData] = useState<AllInventoriesResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedChar, setSelectedChar] = useState<string>('all')
  const [query, setQuery] = useState('')
  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    getAllInventories()
      .then((res) => {
        setData(res)
        // If the previously selected character is no longer present, reset to All.
        setSelectedChar((prev) => {
          if (prev === 'all') return 'all'
          if (res.characters.some((c) => c.character === prev)) return prev
          return 'all'
        })
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  function handleLookup(id: number) {
    getItem(id)
      .then((item) => {
        setModalItem(item)
        setModalOpen(true)
      })
      .catch(() => {
        // Item not found in database — fall back to Items Explorer
        navigate(`/items?select=${id}`)
      })
  }

  // ── Derived data ─────────────────────────────────────────────────────────────

  const allTagged = useMemo<TaggedEntry[]>(() => {
    if (!data) return []
    if (selectedChar === 'all') {
      return data.characters.flatMap(tagEntries)
    }
    const inv = data.characters.find((c) => c.character === selectedChar)
    return inv ? tagEntries(inv) : []
  }, [data, selectedChar])

  const showCharBadge = selectedChar === 'all' && (data?.characters.length ?? 0) > 1

  const equipped = useMemo(() => {
    const items = filterByQuery(allTagged.filter((e) => isEquipment(e.location)), query)
    return items.sort((a, b) => {
      const ai = EQUIPMENT_ORDER.indexOf(a.location)
      const bi = EQUIPMENT_ORDER.indexOf(b.location)
      if (ai === -1 && bi === -1) return a.location.localeCompare(b.location)
      if (ai === -1) return 1
      if (bi === -1) return -1
      return ai - bi
    })
  }, [allTagged, query])

  const bagGroups = useMemo<BagGroup[]>(() => {
    const bagEntries = allTagged.filter((e) => isBagSlot(e.location) || isBagContainer(e.location))
    const groups = groupBags(bagEntries)
    if (!query) return groups
    const q = query.toLowerCase()
    return groups
      .map((g) => ({
        ...g,
        slots: g.slots.filter((s) => s.name.toLowerCase().includes(q)),
      }))
      .filter((g) => g.slots.length > 0 || (g.bag && g.bag.name.toLowerCase().includes(q)))
  }, [allTagged, query])

  const bankItems = useMemo(
    () => filterByQuery(allTagged.filter((e) => isBank(e.location)), query),
    [allTagged, query],
  )

  const sharedBankItems = useMemo(() => {
    if (!data) return []
    const tagged: TaggedEntry[] = data.shared_bank.map((e) => ({ ...e, character: '' }))
    return filterByQuery(tagged, query)
  }, [data, query])

  const totalBagItems = useMemo(
    () => bagGroups.reduce((sum, g) => sum + g.slots.length, 0),
    [bagGroups],
  )

  // ── Loading / error states ───────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
        <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
          {error}
        </p>
        <button
          onClick={load}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  if (!data?.configured) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Package size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          EQ Path Not Configured
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Set your EverQuest install path in{' '}
          <button
            className="underline"
            style={{ color: 'var(--color-primary)' }}
            onClick={() => navigate('/settings')}
          >
            Settings
          </button>
          , then log out of EverQuest to generate Zeal inventory exports.
        </p>
      </div>
    )
  }

  if (data.characters.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Package size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          No Inventory Exports Found
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Zeal writes inventory files named{' '}
          <code
            className="px-1 py-0.5 rounded text-xs"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
          >
            &lt;CharName&gt;_pq.proj-Inventory.txt
          </code>{' '}
          to your EQ directory when you log out. Log out of each character to generate their export.
        </p>
        <button
          onClick={load}
          className="flex items-center gap-2 text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <RefreshCw size={12} />
          Check Again
        </button>
      </div>
    )
  }

  // ── Main render ──────────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col">
      <ItemDetailModal
        item={modalItem}
        open={modalOpen}
        onClose={() => setModalOpen(false)}
      />
      {/* Header bar */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Package size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold shrink-0" style={{ color: 'var(--color-foreground)' }}>
          Inventory Tracker
        </span>

        {/* Search */}
        <div
          className="flex-1 flex items-center gap-2 rounded px-2.5 py-1"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            maxWidth: 320,
          }}
        >
          <Search size={12} style={{ color: 'var(--color-muted)', flexShrink: 0 }} />
          <input
            ref={searchRef}
            type="text"
            placeholder="Search items…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent text-xs outline-none"
            style={{ color: 'var(--color-foreground)' }}
          />
          {query && (
            <button onClick={() => setQuery('')} className="shrink-0">
              <X size={11} style={{ color: 'var(--color-muted)' }} />
            </button>
          )}
        </div>

        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={load}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
        </div>
      </div>

      {/* Character tabs */}
      <div
        className="flex items-center gap-1 border-b px-4 shrink-0"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        {[{ label: 'All', value: 'all' }, ...data.characters.map((c) => ({ label: c.character, value: c.character }))].map(
          ({ label, value }) => {
            const active = selectedChar === value
            return (
              <button
                key={value}
                onClick={() => setSelectedChar(value)}
                className="px-3 py-2 text-xs font-medium transition-colors"
                style={{
                  color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                  borderBottom: active ? '2px solid var(--color-primary)' : '2px solid transparent',
                }}
              >
                {label}
                {value !== 'all' && (
                  <span
                    className="ml-1.5 text-[10px]"
                    style={{ color: active ? 'var(--color-primary)' : 'var(--color-muted)' }}
                  >
                    {data.characters.find((c) => c.character === value)?.entries.length ?? 0}
                  </span>
                )}
              </button>
            )
          },
        )}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {/* Equipped */}
        {equipped.length > 0 && (
          <div>
            <SectionHeader label="Equipped" count={equipped.length} />
            {equipped.map((e, i) => (
              <ItemRow key={`eq-${i}`} entry={e} showCharBadge={showCharBadge} onLookup={handleLookup} />
            ))}
          </div>
        )}

        {/* Bags */}
        {bagGroups.length > 0 && (
          <div>
            <SectionHeader label="Bags" count={totalBagItems} />
            {bagGroups.map((g) => {
              const bagLabel = g.bag
                ? `${showCharBadge ? `${g.character} · ` : ''}General ${g.num} — ${g.bag.name}`
                : `${showCharBadge ? `${g.character} · ` : ''}General ${g.num}`
              return (
                <div key={`${g.character}::${g.num}`}>
                  <div
                    className="flex items-center justify-between px-3 py-1 pl-6"
                    style={{ borderBottom: '1px solid var(--color-border)' }}
                  >
                    <span className="text-[10px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                      {bagLabel}
                    </span>
                    <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
                      {g.slots.length}
                    </span>
                  </div>
                  {g.slots.length === 0 ? (
                    <p className="px-3 pl-8 py-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
                      Empty
                    </p>
                  ) : (
                    g.slots.map((e, i) => (
                      <ItemRow key={`bag-${g.character}-${g.num}-${i}`} entry={e} showCharBadge={false} indent onLookup={handleLookup} />
                    ))
                  )}
                </div>
              )
            })}
          </div>
        )}

        {/* Bank */}
        {bankItems.length > 0 && (
          <div>
            <SectionHeader label="Bank" count={bankItems.length} />
            {bankItems.map((e, i) => (
              <ItemRow key={`bank-${i}`} entry={e} showCharBadge={showCharBadge} onLookup={handleLookup} />
            ))}
          </div>
        )}

        {/* Shared Bank — always shown if non-empty */}
        {sharedBankItems.length > 0 && (
          <div>
            <SectionHeader label="Shared Bank" count={sharedBankItems.length} />
            {sharedBankItems.map((e, i) => (
              <ItemRow key={`sb-${i}`} entry={e} showCharBadge={false} onLookup={handleLookup} />
            ))}
          </div>
        )}

        {/* Empty state after search */}
        {equipped.length === 0 && bagGroups.length === 0 && bankItems.length === 0 && sharedBankItems.length === 0 && (
          <div className="flex h-full items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              {query ? `No items matching "${query}"` : 'No items'}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}

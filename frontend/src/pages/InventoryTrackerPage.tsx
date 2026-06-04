import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, AlertCircle, Search, X, ChevronRight, ChevronDown } from 'lucide-react'
import { getAllInventories, getItem } from '../services/api'
import type { AllInventoriesResponse, Inventory, InventoryEntry } from '../types/zeal'
import type { Item } from '../types/item'
import ItemDetailModal from '../components/ItemDetailModal'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'

// ── Types ──────────────────────────────────────────────────────────────────────

interface TaggedEntry extends InventoryEntry {
  character: string
}

// ── Entry classifiers ──────────────────────────────────────────────────────────

// Bag containers and slots use either ":" or "-" as separator depending on the
// Zeal version. Treat both consistently.
const BAG_SLOT_RE = /^(General|Bank|SharedBank)(\d+)[:\-]Slot(\d+)$/
const BAG_CONTAINER_RE = /^(General|Bank|SharedBank)(\d+)$/

function bagSlotInfo(location: string): { kind: string; bag: number; slot: number } | null {
  const m = location.match(BAG_SLOT_RE)
  if (!m) return null
  return { kind: m[1], bag: parseInt(m[2], 10), slot: parseInt(m[3], 10) }
}

function bagContainerInfo(location: string): { kind: string; bag: number } | null {
  const m = location.match(BAG_CONTAINER_RE)
  if (!m) return null
  return { kind: m[1], bag: parseInt(m[2], 10) }
}

function isBagOrBank(location: string, kind: string): boolean {
  const slot = bagSlotInfo(location)
  if (slot) return slot.kind === kind
  const cont = bagContainerInfo(location)
  return cont?.kind === kind
}

function isEquipmentSlot(location: string): boolean {
  if (bagSlotInfo(location) || bagContainerInfo(location)) return false
  if (location === 'Cursor' || location === 'Held') return false
  if (location === 'General-Coin' || location.endsWith('-Coin')) return false
  return true
}

// ── Grouping ───────────────────────────────────────────────────────────────────

interface BagGroup {
  kind: string // "General" | "Bank" | "SharedBank"
  num: number
  character: string
  bag: TaggedEntry | null
  slots: TaggedEntry[]
}

function groupBags(entries: TaggedEntry[], kind: string): BagGroup[] {
  const map = new Map<string, BagGroup>()
  for (const e of entries) {
    const slot = bagSlotInfo(e.location)
    const cont = bagContainerInfo(e.location)
    const num = slot?.bag ?? cont?.bag
    const matchKind = slot?.kind ?? cont?.kind
    if (num === undefined || matchKind !== kind) continue
    const key = `${e.character}::${num}`
    if (!map.has(key)) map.set(key, { kind, num, character: e.character, bag: null, slots: [] })
    const g = map.get(key)!
    if (slot) g.slots.push(e)
    else g.bag = e
  }
  for (const g of map.values()) {
    g.slots.sort((a, b) => {
      const an = bagSlotInfo(a.location)?.slot ?? 0
      const bn = bagSlotInfo(b.location)?.slot ?? 0
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

function filterByQuery<T extends { name: string }>(entries: T[], query: string): T[] {
  if (!query) return entries
  const q = query.toLowerCase()
  return entries.filter((e) => e.name.toLowerCase().includes(q))
}

function isEmptyEntry(e: InventoryEntry): boolean {
  return e.id === 0 || e.name.toLowerCase() === 'empty'
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface BagCardProps {
  group: BagGroup
  showCharBadge: boolean
  query: string
  onLookup: (id: number) => void
}

function BagCard({ group, showCharBadge, query, onLookup }: BagCardProps): React.ReactElement {
  const [open, setOpen] = useState(true)
  const q = query.trim().toLowerCase()
  const filledSlots = group.slots.filter((s) => !isEmptyEntry(s))
  const visibleSlots = q ? filledSlots.filter((s) => s.name.toLowerCase().includes(q)) : filledSlots
  const bagName = group.bag && !isEmptyEntry(group.bag) ? group.bag.name : '(empty bag)'
  const label = group.kind === 'General' ? `Bag ${group.num}` : `${group.kind} ${group.num}`

  return (
    <div
      className="rounded"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {label}
        </span>
        <span className="text-xs truncate" style={{ color: 'var(--color-muted-foreground)' }}>
          {bagName}
        </span>
        {showCharBadge && (
          <span
            className="shrink-0 text-[10px] px-1.5 py-0.5 rounded ml-1"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {group.character}
          </span>
        )}
        <span className="ml-auto text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {visibleSlots.length}/{group.slots.length}
        </span>
      </button>
      {open && (
        <div
          className="grid gap-1.5 px-3 pb-3 pt-1"
          style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))' }}
        >
          {visibleSlots.length === 0 ? (
            <p className="text-[11px] italic col-span-full" style={{ color: 'var(--color-muted)' }}>
              {q ? 'No matches' : 'Empty'}
            </p>
          ) : (
            visibleSlots.map((s, i) => (
              <button
                key={`${s.location}-${i}`}
                onClick={() => onLookup(s.id)}
                className="flex items-center gap-1.5 rounded px-2 py-1 text-left text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
                title={s.name}
              >
                <ItemIcon id={s.icon} name={s.name} size={18} />
                <span className="truncate flex-1">{s.name}</span>
                {s.count > 1 && (
                  <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                    ×{s.count}
                  </span>
                )}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}

interface SectionTitleProps {
  label: string
  count: number
}

function SectionTitle({ label, count }: SectionTitleProps): React.ReactElement {
  return (
    <div className="flex items-center justify-between mb-2 mt-4">
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
  // Empty string = "All"; a name = single-character view. Matches CharacterSubTabs.
  const [selectedChar, setSelectedChar] = useState<string>('')
  const [query, setQuery] = useState('')
  const [hideEmptyBags, setHideEmptyBags] = useState(false)
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
        setSelectedChar((prev) => {
          // CharacterSubTabs handles validation when characters are loaded; we
          // only collapse to a single character when there's exactly one.
          if (res.characters.length === 1) return res.characters[0].character
          if (prev === '') return ''
          return res.characters.some((c) => c.character === prev) ? prev : ''
        })
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  function handleLookup(id: number) {
    if (id === 0) return
    getItem(id)
      .then((item) => {
        setModalItem(item)
        setModalOpen(true)
      })
      .catch(() => {
        navigate(`/items?select=${id}`)
      })
  }

  // ── Derived data ─────────────────────────────────────────────────────────────

  const allTagged = useMemo<TaggedEntry[]>(() => {
    if (!data) return []
    if (selectedChar === '') return data.characters.flatMap(tagEntries)
    const inv = data.characters.find((c) => c.character === selectedChar)
    return inv ? tagEntries(inv) : []
  }, [data, selectedChar])

  const showCharBadge = selectedChar === '' && (data?.characters.length ?? 0) > 1

  const equipped = useMemo(
    () => allTagged.filter((e) => isEquipmentSlot(e.location) && !isEmptyEntry(e)),
    [allTagged],
  )

  const bagGroups = useMemo<BagGroup[]>(
    () => groupBags(allTagged.filter((e) => isBagOrBank(e.location, 'General')), 'General'),
    [allTagged],
  )

  const bankGroups = useMemo<BagGroup[]>(
    () => groupBags(allTagged.filter((e) => isBagOrBank(e.location, 'Bank')), 'Bank'),
    [allTagged],
  )

  const sharedBankGroups = useMemo<BagGroup[]>(() => {
    if (!data) return []
    const tagged: TaggedEntry[] = data.shared_bank.map((e) => ({ ...e, character: '' }))
    return groupBags(tagged.filter((e) => isBagOrBank(e.location, 'SharedBank')), 'SharedBank')
  }, [data])

  // Equipped items render as flat slot tiles (the in-game paper-doll grid lives
  // on Character Info → Gear), so a single filtered list serves every view.
  const equippedFiltered = useMemo(() => filterByQuery(equipped, query), [equipped, query])

  const hasEmptyBag = useMemo(
    () =>
      bagGroups.some((g) => g.slots.every(isEmptyEntry)) ||
      bankGroups.some((g) => g.slots.every(isEmptyEntry)) ||
      sharedBankGroups.some((g) => g.slots.every(isEmptyEntry)),
    [bagGroups, bankGroups, sharedBankGroups],
  )

  const visibleBagGroups = useMemo(
    () => (hideEmptyBags ? bagGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s))) : bagGroups),
    [bagGroups, hideEmptyBags],
  )
  const visibleBankGroups = useMemo(
    () => (hideEmptyBags ? bankGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s))) : bankGroups),
    [bankGroups, hideEmptyBags],
  )
  const visibleSharedBankGroups = useMemo(
    () =>
      hideEmptyBags
        ? sharedBankGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s)))
        : sharedBankGroups,
    [sharedBankGroups, hideEmptyBags],
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
            &lt;CharName&gt;-Inventory.txt
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

  const totalBagItems = bagGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )
  const totalBankItems = bankGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )
  const totalSharedBankItems = sharedBankGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )

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
          {hasEmptyBag && (
            <label
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded cursor-pointer select-none"
              style={{
                backgroundColor: hideEmptyBags ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title="Hide bags whose slots are all empty"
            >
              <input
                type="checkbox"
                checked={hideEmptyBags}
                onChange={(e) => setHideEmptyBags(e.target.checked)}
                className="h-3 w-3"
              />
              Hide empty bags
            </label>
          )}
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

      {/* Character sub-tabs */}
      <CharacterSubTabs
        value={selectedChar}
        onChange={setSelectedChar}
        allowAll
      />

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {/* Equipped — flat slot tiles, matching the bag-slot layout below. The
            in-game equipment grid lives on Character Info → Gear instead. */}
        <div>
          <SectionTitle label="Equipped" count={equippedFiltered.length} />
          {equippedFiltered.length === 0 ? (
            <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {query ? `No equipped items matching "${query}"` : 'No equipped items'}
            </p>
          ) : (
            <div
              className="grid gap-1.5"
              style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))' }}
            >
              {equippedFiltered.map((e, i) => (
                <button
                  key={`eq-${i}`}
                  onClick={() => handleLookup(e.id)}
                  className="flex items-center gap-1.5 rounded px-2 py-1.5 text-left text-xs"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                  }}
                  title={e.name}
                >
                  <ItemIcon id={e.icon} name={e.name} size={18} />
                  <span className="truncate flex-1">{e.name}</span>
                  <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                    {showCharBadge ? e.character : e.location}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Bags */}
        {visibleBagGroups.length > 0 && (
          <>
            <SectionTitle label="Bags" count={totalBagItems} />
            <div className="flex flex-col gap-2">
              {visibleBagGroups.map((g) => (
                <BagCard
                  key={`bag-${g.character}-${g.num}`}
                  group={g}
                  showCharBadge={showCharBadge}
                  query={query}
                  onLookup={handleLookup}
                />
              ))}
            </div>
          </>
        )}

        {/* Bank */}
        {visibleBankGroups.length > 0 && (
          <>
            <SectionTitle label="Bank" count={totalBankItems} />
            <div className="flex flex-col gap-2">
              {visibleBankGroups.map((g) => (
                <BagCard
                  key={`bank-${g.character}-${g.num}`}
                  group={g}
                  showCharBadge={showCharBadge}
                  query={query}
                  onLookup={handleLookup}
                />
              ))}
            </div>
          </>
        )}

        {/* Shared Bank */}
        {visibleSharedBankGroups.length > 0 && (
          <>
            <SectionTitle label="Shared Bank" count={totalSharedBankItems} />
            <div className="flex flex-col gap-2">
              {visibleSharedBankGroups.map((g) => (
                <BagCard
                  key={`sb-${g.num}`}
                  group={g}
                  showCharBadge={false}
                  query={query}
                  onLookup={handleLookup}
                />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

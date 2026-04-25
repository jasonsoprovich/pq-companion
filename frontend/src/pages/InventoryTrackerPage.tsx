import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, AlertCircle, Search, X, ChevronRight, ChevronDown } from 'lucide-react'
import { getAllInventories, getItem } from '../services/api'
import type { AllInventoriesResponse, Inventory, InventoryEntry } from '../types/zeal'
import type { Item } from '../types/item'
import ItemDetailModal from '../components/ItemDetailModal'

// ── Types ──────────────────────────────────────────────────────────────────────

interface TaggedEntry extends InventoryEntry {
  character: string
}

// Visual position in the paper-doll grid (1-indexed CSS grid coordinates).
interface SlotPos {
  /** Display label for empty slots. */
  label: string
  /** Underlying Zeal slot key. Multiple positions may share the same key (e.g. Ear, Wrist, Fingers). */
  key: string
  /** When two positions share a key, which occurrence (0 or 1) consumes the matching entry. */
  index?: number
  col: number
  row: number
}

// 5-column × 9-row grid loosely modeled on the EQ paper-doll inventory window.
const SLOTS: SlotPos[] = [
  { label: 'Charm',      key: 'Charm',     col: 1, row: 1 },
  { label: 'Ear',        key: 'Ear',       index: 0, col: 2, row: 1 },
  { label: 'Head',       key: 'Head',      col: 3, row: 1 },
  { label: 'Face',       key: 'Face',      col: 4, row: 1 },
  { label: 'Ear',        key: 'Ear',       index: 1, col: 5, row: 1 },

  { label: 'Neck',       key: 'Neck',      col: 3, row: 2 },

  { label: 'Shoulders',  key: 'Shoulders', col: 1, row: 3 },
  { label: 'Chest',      key: 'Chest',     col: 5, row: 3 },

  { label: 'Arms',       key: 'Arms',      col: 1, row: 4 },
  { label: 'Back',       key: 'Back',      col: 5, row: 4 },

  { label: 'Wrist',      key: 'Wrist',     index: 0, col: 1, row: 5 },
  { label: 'Wrist',      key: 'Wrist',     index: 1, col: 5, row: 5 },

  { label: 'Hands',      key: 'Hands',     col: 1, row: 6 },
  { label: 'Range',      key: 'Range',     col: 5, row: 6 },

  { label: 'Finger',     key: 'Fingers',   index: 0, col: 1, row: 7 },
  { label: 'Finger',     key: 'Fingers',   index: 1, col: 5, row: 7 },

  { label: 'Waist',      key: 'Waist',     col: 1, row: 8 },
  { label: 'Legs',       key: 'Legs',      col: 3, row: 8 },
  { label: 'Feet',       key: 'Feet',      col: 5, row: 8 },

  { label: 'Primary',    key: 'Primary',   col: 1, row: 9 },
  { label: 'Secondary',  key: 'Secondary', col: 3, row: 9 },
  { label: 'Ammo',       key: 'Ammo',      col: 5, row: 9 },
]

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

// Build a lookup: slot key → list of equipped entries in that key.
function indexEquipmentBySlot(equipped: TaggedEntry[]): Map<string, TaggedEntry[]> {
  const m = new Map<string, TaggedEntry[]>()
  for (const e of equipped) {
    if (isEmptyEntry(e)) continue
    if (!m.has(e.location)) m.set(e.location, [])
    m.get(e.location)!.push(e)
  }
  return m
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface SlotCellProps {
  pos: SlotPos
  entry: TaggedEntry | null
  highlight: boolean
  onLookup: (id: number) => void
}

function SlotCell({ pos, entry, highlight, onLookup }: SlotCellProps): React.ReactElement {
  const empty = !entry
  return (
    <div
      style={{
        gridColumn: pos.col,
        gridRow: pos.row,
        backgroundColor: empty
          ? 'var(--color-surface)'
          : highlight
            ? 'color-mix(in srgb, var(--color-primary) 15%, var(--color-surface-2))'
            : 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        borderColor: highlight ? 'var(--color-primary)' : 'var(--color-border)',
        borderRadius: 4,
        padding: '6px 8px',
        minHeight: 56,
        cursor: empty ? 'default' : 'pointer',
        transition: 'background-color 120ms, border-color 120ms',
      }}
      onClick={() => entry && onLookup(entry.id)}
      title={entry ? entry.name : pos.label}
    >
      <div
        className="text-[10px] font-semibold uppercase tracking-wider"
        style={{ color: empty ? 'var(--color-muted)' : 'var(--color-muted-foreground)' }}
      >
        {pos.label}
      </div>
      {entry ? (
        <div className="mt-0.5 flex items-center gap-1.5">
          <span className="text-xs leading-tight truncate" style={{ color: 'var(--color-foreground)' }}>
            {entry.name}
          </span>
          {entry.count > 1 && (
            <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
              ×{entry.count}
            </span>
          )}
        </div>
      ) : (
        <div className="mt-0.5 text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
          —
        </div>
      )}
    </div>
  )
}

interface PaperDollProps {
  equipped: TaggedEntry[]
  query: string
  onLookup: (id: number) => void
}

function PaperDoll({ equipped, query, onLookup }: PaperDollProps): React.ReactElement {
  const indexed = useMemo(() => indexEquipmentBySlot(equipped), [equipped])
  const q = query.trim().toLowerCase()

  return (
    <div
      className="grid gap-2"
      style={{
        gridTemplateColumns: 'repeat(5, minmax(0, 1fr))',
        gridAutoRows: 'minmax(56px, auto)',
      }}
    >
      {SLOTS.map((pos) => {
        const list = indexed.get(pos.key) ?? []
        const idx = pos.index ?? 0
        const entry = list[idx] ?? null
        const highlight = !!(entry && q && entry.name.toLowerCase().includes(q))
        return (
          <SlotCell
            key={`${pos.key}-${idx}-${pos.row}-${pos.col}`}
            pos={pos}
            entry={entry}
            highlight={highlight}
            onLookup={onLookup}
          />
        )
      })}
    </div>
  )
}

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
                className="flex items-center justify-between gap-1 rounded px-2 py-1 text-left text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
                title={s.name}
              >
                <span className="truncate">{s.name}</span>
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
        setSelectedChar((prev) => {
          if (prev === 'all') return res.characters.length === 1 ? res.characters[0].character : 'all'
          if (res.characters.some((c) => c.character === prev)) return prev
          return res.characters.length === 1 ? res.characters[0].character : 'all'
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
    if (selectedChar === 'all') return data.characters.flatMap(tagEntries)
    const inv = data.characters.find((c) => c.character === selectedChar)
    return inv ? tagEntries(inv) : []
  }, [data, selectedChar])

  const showCharBadge = selectedChar === 'all' && (data?.characters.length ?? 0) > 1
  const singleCharacter = selectedChar !== 'all'

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

  // For the "All" view, fall back to a flat equipped list since the paper-doll
  // can only display one character at a time.
  const equippedFiltered = useMemo(() => filterByQuery(equipped, query), [equipped, query])

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
        className="flex items-center gap-1 border-b px-4 shrink-0 overflow-x-auto"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        {[{ label: 'All', value: 'all' }, ...data.characters.map((c) => ({ label: c.character, value: c.character }))].map(
          ({ label, value }) => {
            const active = selectedChar === value
            return (
              <button
                key={value}
                onClick={() => setSelectedChar(value)}
                className="px-3 py-2 text-xs font-medium transition-colors whitespace-nowrap"
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
                    {data.characters.find((c) => c.character === value)?.entries.filter((e) => !isEmptyEntry(e)).length ?? 0}
                  </span>
                )}
              </button>
            )
          },
        )}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {/* Equipped — paper-doll for single-character view, flat list for All */}
        {singleCharacter ? (
          <div className="mx-auto" style={{ maxWidth: 720 }}>
            <SectionTitle label="Equipped" count={equipped.length} />
            <PaperDoll equipped={equipped} query={query} onLookup={handleLookup} />
          </div>
        ) : (
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
                    className="flex items-center justify-between gap-2 rounded px-2 py-1.5 text-left text-xs"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                    }}
                    title={e.name}
                  >
                    <span className="truncate">{e.name}</span>
                    <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                      {showCharBadge ? e.character : e.location}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Bags */}
        {bagGroups.length > 0 && (
          <>
            <SectionTitle label="Bags" count={totalBagItems} />
            <div className="flex flex-col gap-2">
              {bagGroups.map((g) => (
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
        {bankGroups.length > 0 && (
          <>
            <SectionTitle label="Bank" count={totalBankItems} />
            <div className="flex flex-col gap-2">
              {bankGroups.map((g) => (
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
        {sharedBankGroups.length > 0 && (
          <>
            <SectionTitle label="Shared Bank" count={totalSharedBankItems} />
            <div className="flex flex-col gap-2">
              {sharedBankGroups.map((g) => (
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

import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, ExternalLink, AlertCircle } from 'lucide-react'
import { getZealInventory } from '../services/api'
import type { Inventory, InventoryEntry } from '../types/zeal'

// ── Slot ordering for equipment display ───────────────────────────────────────

const EQUIPMENT_ORDER = [
  'Charm', 'Ear1', 'Ear2', 'Head', 'Face', 'Neck', 'Shoulders', 'Arms',
  'Wrists', 'Wrist', 'Hands', 'Primary', 'Secondary', 'Range', 'Ammo',
  'Finger1', 'Finger2', 'Ring1', 'Ring2', 'Chest', 'Back', 'Waist', 'Legs', 'Feet',
]

// Returns the bag number from a location like "General3" or "General3:Slot2".
function bagNumber(location: string): number | null {
  const m = location.match(/^General(\d+)/)
  return m ? parseInt(m[1], 10) : null
}

function isBankSlot(location: string): boolean {
  return location.startsWith('Bank') || location.startsWith('SharedBank')
}

function isEquipment(location: string): boolean {
  return !location.includes(':') && bagNumber(location) === null && !isBankSlot(location) && location !== 'Cursor'
}

// Group entries into equipment, bags (1–8), bank, and cursor.
function groupEntries(entries: InventoryEntry[]) {
  const equipment: InventoryEntry[] = []
  const bags: Map<number, { bag: InventoryEntry | null; slots: InventoryEntry[] }> = new Map()
  const bank: InventoryEntry[] = []
  const cursor: InventoryEntry[] = []

  for (const e of entries) {
    const bn = bagNumber(e.location)
    if (bn !== null) {
      if (!bags.has(bn)) bags.set(bn, { bag: null, slots: [] })
      const g = bags.get(bn)!
      if (e.location.includes(':')) {
        g.slots.push(e)
      } else {
        g.bag = e
      }
    } else if (isBankSlot(e.location)) {
      bank.push(e)
    } else if (e.location === 'Cursor') {
      cursor.push(e)
    } else {
      equipment.push(e)
    }
  }

  // Sort equipment by canonical slot order.
  equipment.sort((a, b) => {
    const ai = EQUIPMENT_ORDER.indexOf(a.location)
    const bi = EQUIPMENT_ORDER.indexOf(b.location)
    if (ai === -1 && bi === -1) return a.location.localeCompare(b.location)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  // Sort bag slots by their slot number within each bag.
  for (const g of bags.values()) {
    g.slots.sort((a, b) => {
      const aSlot = parseInt(a.location.split(':Slot')[1] ?? '0', 10)
      const bSlot = parseInt(b.location.split(':Slot')[1] ?? '0', 10)
      return aSlot - bSlot
    })
  }

  // Return bags sorted by bag number.
  const sortedBags = [...bags.entries()].sort(([a], [b]) => a - b).map(([num, g]) => ({ num, ...g }))

  return { equipment, bags: sortedBags, bank, cursor }
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface ItemRowProps {
  entry: InventoryEntry
  indent?: boolean
  onLookup: (id: number) => void
}

function ItemRow({ entry, indent = false, onLookup }: ItemRowProps): React.ReactElement {
  return (
    <div
      className={`group flex items-center gap-2 px-3 py-1.5 ${indent ? 'pl-8' : ''}`}
      style={{ borderBottom: '1px solid var(--color-border)' }}
    >
      <div className="flex-1 min-w-0">
        <span className="text-sm truncate" style={{ color: 'var(--color-foreground)' }}>
          {entry.name}
        </span>
        {entry.count > 1 && (
          <span
            className="ml-2 text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            ×{entry.count}
          </span>
        )}
      </div>
      <button
        onClick={() => onLookup(entry.id)}
        className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
        title="Look up in Item Explorer"
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

export default function InventoryPage(): React.ReactElement {
  const [inventory, setInventory] = useState<Inventory | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    getZealInventory()
      .then((res) => setInventory(res.inventory))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  function handleLookup(id: number) {
    navigate(`/items?select=${id}`)
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw
          size={20}
          className="animate-spin"
          style={{ color: 'var(--color-muted)' }}
        />
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

  if (!inventory) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Package size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          No Zeal Export Found
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Zeal writes your inventory to{' '}
          <code
            className="px-1 py-0.5 rounded text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
            }}
          >
            &lt;CharName&gt;_pq.proj-Inventory.txt
          </code>{' '}
          in your EverQuest directory when you log out.
        </p>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Make sure your EQ path and character name are set in{' '}
          <button
            className="underline"
            style={{ color: 'var(--color-primary)' }}
            onClick={() => navigate('/settings')}
          >
            Settings
          </button>
          , then log out of EverQuest to generate the export.
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

  const { equipment, bags, bank, cursor } = groupEntries(inventory.entries)
  const exportDate = new Date(inventory.exported_at)

  return (
    <div className="flex h-full flex-col">
      {/* Header bar */}
      <div
        className="flex items-center justify-between border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-3">
          <Package size={18} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {inventory.character}
          </span>
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {inventory.entries.length} items
          </span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
            Exported {exportDate.toLocaleString()}
          </span>
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

      {/* Content — two columns: equipment | bags */}
      <div className="flex flex-1 min-h-0">
        {/* Equipment column */}
        <div
          className="w-72 shrink-0 flex flex-col border-r overflow-y-auto"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <SectionHeader label="Equipped" count={equipment.length} />
          {equipment.length === 0 ? (
            <p className="px-3 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No equipment data
            </p>
          ) : (
            equipment.map((e) => (
              <ItemRow
                key={e.location}
                entry={e}
                onLookup={handleLookup}
              />
            ))
          )}

          {cursor.length > 0 && (
            <>
              <SectionHeader label="Cursor" count={cursor.length} />
              {cursor.map((e, i) => (
                <ItemRow key={i} entry={e} onLookup={handleLookup} />
              ))}
            </>
          )}

          {bank.length > 0 && (
            <>
              <SectionHeader label="Bank" count={bank.length} />
              {bank.map((e) => (
                <ItemRow key={e.location} entry={e} onLookup={handleLookup} />
              ))}
            </>
          )}
        </div>

        {/* Bags column */}
        <div className="flex-1 overflow-y-auto">
          {bags.length === 0 ? (
            <div className="flex h-full items-center justify-center">
              <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
                No bags
              </p>
            </div>
          ) : (
            bags.map(({ num, bag, slots }) => (
              <div key={num}>
                <SectionHeader
                  label={`General ${num}${bag ? ` — ${bag.name}` : ''}`}
                  count={slots.length}
                />
                {slots.length === 0 ? (
                  <p className="px-3 py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                    Empty
                  </p>
                ) : (
                  slots.map((e) => (
                    <ItemRow
                      key={e.location}
                      entry={e}
                      indent
                      onLookup={handleLookup}
                    />
                  ))
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

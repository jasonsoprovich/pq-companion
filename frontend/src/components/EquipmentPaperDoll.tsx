import React, { useMemo } from 'react'
import { ItemIcon } from './Icon'

// Minimal shape the paper-doll needs from an equipped item. Both the Zeal
// inventory entry (types/zeal.ts) and the quarmy export entry (api.ts) satisfy
// this, so the same grid renders the Inventory and Character Info → Gear views.
export interface EquipSlotEntry {
  location: string
  name: string
  id: number
  count: number
  icon?: number
}

// Visual position in the paper-doll grid (1-indexed CSS grid coordinates).
interface SlotPos {
  /** Display label for empty slots. */
  label: string
  /** Underlying slot key (matches an entry's location). Multiple positions may
   *  share the same key (e.g. Ear, Wrist, Fingers). */
  key: string
  /** When two positions share a key, which occurrence (0 or 1) consumes the
   *  matching entry. */
  index?: number
  col: number
  row: number
}

// 5-column × 9-row grid loosely modeled on the EQ paper-doll inventory window.
const SLOTS: SlotPos[] = [
  // No Charm slot — Project Quarm (TAKP/EQMac client) has no charm slot, so
  // col 1 / row 1 is intentionally left empty.
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

function isEmptyEntry(e: EquipSlotEntry): boolean {
  return e.id === 0 || e.name.toLowerCase() === 'empty'
}

// Build a lookup: slot key → list of equipped entries in that key.
function indexEquipmentBySlot(equipped: EquipSlotEntry[]): Map<string, EquipSlotEntry[]> {
  const m = new Map<string, EquipSlotEntry[]>()
  for (const e of equipped) {
    if (isEmptyEntry(e)) continue
    if (!m.has(e.location)) m.set(e.location, [])
    m.get(e.location)!.push(e)
  }
  return m
}

interface SlotCellProps {
  pos: SlotPos
  entry: EquipSlotEntry | null
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
          <ItemIcon id={entry.icon} name={entry.name} size={20} />
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

interface EquipmentPaperDollProps {
  equipped: EquipSlotEntry[]
  /** Optional search term; matching item cells are highlighted. */
  query?: string
  onLookup: (id: number) => void
}

// EquipmentPaperDoll renders equipped items in an in-game-style equipment
// grid, positioning each item by its slot.
export function EquipmentPaperDoll({
  equipped,
  query = '',
  onLookup,
}: EquipmentPaperDollProps): React.ReactElement {
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

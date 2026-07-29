import React, { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ItemSourceNPC } from '../types/item'
import { formatNPCName } from './SourceNPCLink'

// SourceNPCTable renders the "Drops From" / "Purchased From" lists as a real
// table: fixed columns, zebra rows, and sortable headers.
//
// The previous layout was a flex row with justify-between, which pushed the NPC
// name hard left and the zone hard right — readable in a narrow panel, but on a
// wide window it left a gulf between the two and made it hard to tell which NPC
// sat in which zone. Fixed column widths keep the pair together at any width.
//
// Kept separate from SourceNPCLink, which is still the right compact form for
// the spell acquisition panel and the gear-finder popovers.

export type SourceSortKey = 'name' | 'zone' | 'rate'
type SortState = { key: SourceSortKey; dir: 'asc' | 'desc' }

interface SourceNPCTableProps {
  npcs: ItemSourceNPC[]
  // showRate adds the drop-rate column (drops only; vendors have no rate).
  showRate?: boolean
  defaultSort?: SortState
}

export function SourceNPCTable({
  npcs,
  showRate,
  defaultSort,
}: SourceNPCTableProps): React.ReactElement {
  const navigate = useNavigate()
  const [sort, setSort] = useState<SortState>(
    defaultSort ?? { key: 'zone', dir: 'asc' },
  )

  const rows = useMemo(() => {
    const dir = sort.dir === 'asc' ? 1 : -1
    const byName = (a: ItemSourceNPC, b: ItemSourceNPC): number =>
      formatNPCName(a.name).localeCompare(formatNPCName(b.name))
    const cmp = (a: ItemSourceNPC, b: ItemSourceNPC): number => {
      switch (sort.key) {
        case 'name':
          return byName(a, b)
        case 'rate':
          return (a.drop_rate ?? 0) - (b.drop_rate ?? 0)
        default:
          // Within a zone keep NPCs alphabetical, so the grouping reads as a
          // block rather than an arbitrary order.
          return (a.zone_name || '').localeCompare(b.zone_name || '') || byName(a, b)
      }
    }
    return [...npcs].sort((a, b) => cmp(a, b) * dir)
  }, [npcs, sort])

  const toggleSort = (key: SourceSortKey): void => {
    setSort((prev) =>
      prev.key === key
        ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'rate' ? 'desc' : 'asc' },
    )
  }

  return (
    <table className="w-full table-fixed border-collapse text-sm">
      <thead>
        <tr
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          <Th label="NPC" sortKey="name" sort={sort} onSort={toggleSort} />
          <Th label="Zone" sortKey="zone" sort={sort} onSort={toggleSort} width="40%" />
          {showRate && (
            <Th label="Rate" sortKey="rate" sort={sort} onSort={toggleSort} width="4.5rem" align="right" />
          )}
        </tr>
      </thead>
      <tbody>
        {rows.map((npc, i) => (
          <tr
            key={`${npc.id}-${npc.zone_short_name}-${i}`}
            style={{
              backgroundColor: i % 2 === 1 ? 'var(--color-surface-2)' : 'transparent',
            }}
          >
            <td className="truncate px-1.5 py-1">
              <button
                onClick={() => navigate(`/npcs?select=${npc.id}`)}
                className="max-w-full truncate text-left underline decoration-dotted"
                style={{ color: 'var(--color-primary)' }}
              >
                {formatNPCName(npc.name)}
              </button>
            </td>
            <td className="truncate px-1.5 py-1">
              {npc.zone_name && (
                <button
                  onClick={() => navigate(`/zones?select=${npc.zone_short_name}`)}
                  className="max-w-full truncate text-left text-xs underline decoration-dotted"
                  style={{ color: 'var(--color-muted)' }}
                >
                  {npc.zone_name}
                </button>
              )}
            </td>
            {showRate && (
              <td
                className="px-1.5 py-1 text-right text-xs tabular-nums"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                {npc.drop_rate != null && npc.drop_rate > 0
                  ? `${npc.drop_rate.toFixed(2)}%`
                  : '—'}
              </td>
            )}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function Th({
  label,
  sortKey,
  sort,
  onSort,
  width,
  align,
}: {
  label: string
  sortKey: SourceSortKey
  sort: SortState
  onSort: (k: SourceSortKey) => void
  width?: string
  align?: 'right'
}): React.ReactElement {
  const active = sort.key === sortKey
  return (
    <th
      className={`px-1.5 pb-1 font-semibold ${align === 'right' ? 'text-right' : 'text-left'}`}
      style={{ width }}
    >
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className="inline-flex items-center gap-1 whitespace-nowrap hover:underline"
        style={{ color: active ? 'var(--color-foreground)' : 'inherit' }}
      >
        {label}
        {active && <span>{sort.dir === 'desc' ? '▼' : '▲'}</span>}
      </button>
    </th>
  )
}

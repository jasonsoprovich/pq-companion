import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Archive,
  ChevronDown,
  ChevronRight,
  Search,
  Trash2,
  RefreshCw,
  X,
} from 'lucide-react'
import {
  listCombatHistory,
  deleteCombatHistoryFight,
  clearCombatHistory,
} from '../services/api'
import type { EntityStats, HealerStats, HistoryListResponse, StoredFight } from '../types/combat'

// Page-level pagination size — matches the backend default; chosen so a
// raid night (~50–200 fights) fits in 1–2 pages without scrolling becoming
// the only way to navigate.
const PAGE_SIZE = 50

// ── small helpers ─────────────────────────────────────────────────────────────

function fmt(n: number): string {
  return n.toLocaleString()
}

function fmtDPS(n: number): string {
  return n.toFixed(1)
}

function fmtDuration(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = Math.floor(secs % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function fmtDateTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

// ── filter form ───────────────────────────────────────────────────────────────

interface UIFilter {
  npc: string
  character: string
  zone: string
  // YYYY-MM-DD strings from <input type=date>; converted to RFC3339 at fetch.
  startDate: string
  endDate: string
}

const EMPTY_FILTER: UIFilter = { npc: '', character: '', zone: '', startDate: '', endDate: '' }

function FilterBar({
  filter,
  onChange,
  onApply,
  onClear,
  onRefresh,
  onDeleteAll,
}: {
  filter: UIFilter
  onChange: (f: UIFilter) => void
  onApply: () => void
  onClear: () => void
  onRefresh: () => void
  onDeleteAll: () => void
}): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 14px',
        borderBottom: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-surface-2)',
        flexShrink: 0,
        flexWrap: 'wrap',
      }}
    >
      {/* NPC search */}
      <div style={{ position: 'relative', flex: '1 1 180px', minWidth: 140, maxWidth: 240 }}>
        <Search
          size={11}
          style={{ position: 'absolute', left: 7, top: '50%', transform: 'translateY(-50%)', color: 'var(--color-muted)', pointerEvents: 'none' }}
        />
        <input
          type="text"
          placeholder="NPC name…"
          value={filter.npc}
          onChange={(e) => onChange({ ...filter, npc: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Enter') onApply()
          }}
          style={{
            width: '100%',
            paddingLeft: 22,
            paddingRight: 8,
            paddingTop: 4,
            paddingBottom: 4,
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: 'var(--color-foreground)',
            outline: 'none',
            boxSizing: 'border-box',
          }}
        />
      </div>

      <input
        type="text"
        placeholder="Character"
        value={filter.character}
        onChange={(e) => onChange({ ...filter, character: e.target.value })}
        onKeyDown={(e) => {
          if (e.key === 'Enter') onApply()
        }}
        style={{
          width: 110,
          padding: '4px 8px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-foreground)',
        }}
      />

      <input
        type="text"
        placeholder="Zone"
        value={filter.zone}
        onChange={(e) => onChange({ ...filter, zone: e.target.value })}
        onKeyDown={(e) => {
          if (e.key === 'Enter') onApply()
        }}
        style={{
          width: 110,
          padding: '4px 8px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-foreground)',
        }}
      />

      <input
        type="date"
        value={filter.startDate}
        onChange={(e) => onChange({ ...filter, startDate: e.target.value })}
        title="Start date"
        style={{
          padding: '3px 6px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-foreground)',
        }}
      />
      <input
        type="date"
        value={filter.endDate}
        onChange={(e) => onChange({ ...filter, endDate: e.target.value })}
        title="End date"
        style={{
          padding: '3px 6px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-foreground)',
        }}
      />

      <button
        onClick={onApply}
        style={{
          padding: '4px 10px',
          fontSize: 11,
          background: 'var(--color-primary)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: '#000',
          cursor: 'pointer',
          fontWeight: 600,
        }}
      >
        Apply
      </button>

      <button
        onClick={onClear}
        title="Clear filters"
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          padding: '4px 8px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-muted)',
          cursor: 'pointer',
        }}
      >
        <X size={11} /> Reset
      </button>

      <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
        <button
          onClick={onRefresh}
          title="Refresh from server"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '4px 8px',
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: 'var(--color-muted)',
            cursor: 'pointer',
          }}
        >
          <RefreshCw size={11} /> Refresh
        </button>

        <button
          onClick={onDeleteAll}
          title="Delete ALL saved fights"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '4px 8px',
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: '#ef4444',
            cursor: 'pointer',
          }}
        >
          <Trash2 size={11} /> Clear all
        </button>
      </div>
    </div>
  )
}

// ── combatant breakdown table (collapsed view) ────────────────────────────────

function CombatantTable({
  combatants,
  totalDamage,
}: {
  combatants: EntityStats[]
  totalDamage: number
}): React.ReactElement {
  if (combatants.length === 0) {
    return (
      <div style={{ fontSize: 11, color: 'var(--color-muted)', fontStyle: 'italic' }}>
        No combatant data
      </div>
    )
  }
  return (
    <div>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 48px 80px 70px 60px',
          gap: '0 8px',
          fontSize: 10,
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: 'var(--color-muted)',
          padding: '3px 0',
          borderBottom: '1px solid var(--color-border)',
        }}
      >
        <span>Name</span>
        <span style={{ textAlign: 'right' }}>%</span>
        <span style={{ textAlign: 'right' }}>Damage</span>
        <span style={{ textAlign: 'right' }}>DPS</span>
        <span style={{ textAlign: 'right' }}>Crits</span>
      </div>
      {combatants.map((c) => (
        <div
          key={c.name}
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 48px 80px 70px 60px',
            gap: '0 8px',
            padding: '3px 0',
            fontSize: 11,
            borderBottom: '1px solid rgba(255,255,255,0.03)',
          }}
        >
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</span>
          <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>
            {pct(c.total_damage, totalDamage)}
          </span>
          <span style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{fmt(c.total_damage)}</span>
          <span style={{ textAlign: 'right', color: '#f97316', fontVariantNumeric: 'tabular-nums' }}>{fmtDPS(c.dps)}</span>
          <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>
            {c.crit_count > 0 ? c.crit_count : '—'}
          </span>
        </div>
      ))}
    </div>
  )
}

function HealerTable({ healers }: { healers: HealerStats[] }): React.ReactElement | null {
  if (healers.length === 0) return null
  return (
    <div style={{ marginTop: 10 }}>
      <div
        style={{
          fontSize: 10,
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: 'var(--color-muted)',
          marginBottom: 4,
        }}
      >
        Healers
      </div>
      {healers.map((h) => (
        <div
          key={h.name}
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 80px 70px',
            gap: '0 8px',
            padding: '3px 0',
            fontSize: 11,
            borderBottom: '1px solid rgba(255,255,255,0.03)',
          }}
        >
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{h.name}</span>
          <span style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{fmt(h.total_heal)}</span>
          <span style={{ textAlign: 'right', color: '#22c55e', fontVariantNumeric: 'tabular-nums' }}>{fmtDPS(h.hps)}</span>
        </div>
      ))}
    </div>
  )
}

// ── one row + expandable detail ───────────────────────────────────────────────

function FightRow({
  fight,
  onDelete,
}: {
  fight: StoredFight
  onDelete: () => void
}): React.ReactElement {
  const [expanded, setExpanded] = useState(false)
  return (
    <div style={{ borderBottom: '1px solid var(--color-border)' }}>
      <div
        onClick={() => setExpanded((v) => !v)}
        style={{
          display: 'grid',
          gridTemplateColumns: '20px 140px 1fr 60px 90px 90px 24px',
          gap: '0 12px',
          alignItems: 'center',
          padding: '7px 14px',
          cursor: 'pointer',
          fontSize: 12,
        }}
      >
        <span style={{ color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}>
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </span>
        <span style={{ color: 'var(--color-muted)', fontSize: 11, whiteSpace: 'nowrap' }}>
          {fmtDateTime(fight.start_time)}
        </span>
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontStyle: 'italic',
          }}
        >
          {fight.npc_name}
          {fight.zone && (
            <span style={{ color: 'var(--color-muted)', fontStyle: 'normal' }}>
              {' · '}
              {fight.zone}
            </span>
          )}
          {fight.character_name && (
            <span style={{ color: 'var(--color-muted)', fontStyle: 'normal' }}>
              {' · '}
              {fight.character_name}
            </span>
          )}
        </span>
        <span style={{ color: 'var(--color-muted)', fontSize: 11, textAlign: 'right' }}>
          {fmtDuration(fight.duration_seconds)}
        </span>
        <span style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{fmt(fight.total_damage)}</span>
        <span style={{ textAlign: 'right', color: '#f97316', fontVariantNumeric: 'tabular-nums' }}>
          {fmtDPS(fight.total_damage / Math.max(fight.duration_seconds, 0.001))} DPS
        </span>
        <button
          onClick={(e) => {
            e.stopPropagation()
            onDelete()
          }}
          title="Delete this fight"
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            padding: 2,
            color: 'var(--color-muted)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Trash2 size={12} />
        </button>
      </div>
      {expanded && (
        <div style={{ padding: '4px 14px 12px', backgroundColor: 'var(--color-surface-2)' }}>
          <CombatantTable combatants={fight.combatants} totalDamage={fight.total_damage} />
          <HealerTable healers={fight.healers} />
        </div>
      )}
    </div>
  )
}

// ── pagination ────────────────────────────────────────────────────────────────

function Pagination({
  total,
  offset,
  pageSize,
  onPage,
}: {
  total: number
  offset: number
  pageSize: number
  onPage: (newOffset: number) => void
}): React.ReactElement {
  const page = Math.floor(offset / pageSize) + 1
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div
      style={{
        padding: '6px 14px',
        fontSize: 11,
        color: 'var(--color-muted)',
        borderTop: '1px solid var(--color-border)',
        display: 'flex',
        gap: 12,
        alignItems: 'center',
        flexShrink: 0,
        backgroundColor: 'var(--color-surface-2)',
      }}
    >
      <span>
        Page {page} of {pages} · {total} total
      </span>
      <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
        <button
          onClick={() => onPage(Math.max(0, offset - pageSize))}
          disabled={offset === 0}
          style={{
            padding: '3px 10px',
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: offset === 0 ? 'var(--color-muted)' : 'var(--color-foreground)',
            cursor: offset === 0 ? 'default' : 'pointer',
          }}
        >
          Prev
        </button>
        <button
          onClick={() => onPage(offset + pageSize)}
          disabled={offset + pageSize >= total}
          style={{
            padding: '3px 10px',
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: offset + pageSize >= total ? 'var(--color-muted)' : 'var(--color-foreground)',
            cursor: offset + pageSize >= total ? 'default' : 'pointer',
          }}
        >
          Next
        </button>
      </div>
    </div>
  )
}

// ── page ──────────────────────────────────────────────────────────────────────

export default function CombatHistoryPage(): React.ReactElement {
  const [filter, setFilter] = useState<UIFilter>(EMPTY_FILTER)
  // appliedFilter is what's in flight to the backend; filter tracks the form
  // so typing doesn't refetch on every keystroke.
  const [appliedFilter, setAppliedFilter] = useState<UIFilter>(EMPTY_FILTER)
  const [offset, setOffset] = useState(0)
  const [data, setData] = useState<HistoryListResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const fetchPage = useCallback(() => {
    setLoading(true)
    listCombatHistory({
      npc: appliedFilter.npc || undefined,
      character: appliedFilter.character || undefined,
      zone: appliedFilter.zone || undefined,
      // <input type=date> gives YYYY-MM-DD; treat as the user's local
      // midnight and convert to RFC3339. End date is inclusive of the day.
      start: appliedFilter.startDate ? new Date(appliedFilter.startDate + 'T00:00:00').toISOString() : undefined,
      end: appliedFilter.endDate ? new Date(appliedFilter.endDate + 'T23:59:59').toISOString() : undefined,
      limit: PAGE_SIZE,
      offset,
    })
      .then((res) => {
        setData(res)
        setError(null)
      })
      .catch((e) => setError(e.message ?? String(e)))
      .finally(() => setLoading(false))
  }, [appliedFilter, offset])

  useEffect(() => {
    fetchPage()
  }, [fetchPage])

  const handleApply = useCallback(() => {
    setOffset(0)
    setAppliedFilter(filter)
  }, [filter])

  const handleClearFilters = useCallback(() => {
    setOffset(0)
    setFilter(EMPTY_FILTER)
    setAppliedFilter(EMPTY_FILTER)
  }, [])

  const handleDeleteRow = useCallback(
    (id: number) => {
      if (!window.confirm('Delete this fight from history?')) return
      deleteCombatHistoryFight(id).then(fetchPage).catch((e) => setError(e.message ?? String(e)))
    },
    [fetchPage],
  )

  const handleDeleteAll = useCallback(() => {
    if (!window.confirm('Delete ALL saved fights? This cannot be undone.')) return
    clearCombatHistory().then(fetchPage).catch((e) => setError(e.message ?? String(e)))
  }, [fetchPage])

  const fights = data?.fights ?? []
  const total = data?.total ?? 0

  const empty = useMemo(() => {
    if (loading || error) return null
    if (fights.length > 0) return null
    const filtered =
      appliedFilter.npc || appliedFilter.character || appliedFilter.zone || appliedFilter.startDate || appliedFilter.endDate
    return filtered ? 'No fights match your filters' : 'No saved fights yet — fight an NPC and they will appear here.'
  }, [loading, error, fights, appliedFilter])

  return (
    <div className="flex h-full flex-col overflow-hidden" style={{ backgroundColor: 'var(--color-background)' }}>
      <div
        className="flex shrink-0 items-center justify-between border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2">
          <Archive size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Combat History
          </h1>
          <span
            className="rounded px-1.5 py-0.5 text-[10px]"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
          >
            {total} saved
          </span>
        </div>
      </div>

      <FilterBar
        filter={filter}
        onChange={setFilter}
        onApply={handleApply}
        onClear={handleClearFilters}
        onRefresh={fetchPage}
        onDeleteAll={handleDeleteAll}
      />

      {error && (
        <div
          style={{
            padding: '8px 14px',
            fontSize: 11,
            color: '#f87171',
            backgroundColor: 'rgba(220,38,38,0.12)',
            borderBottom: '1px solid var(--color-border)',
          }}
        >
          {error}
        </div>
      )}

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '20px 140px 1fr 60px 90px 90px 24px',
          gap: '0 12px',
          padding: '5px 14px',
          fontSize: 10,
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: 'var(--color-muted)',
          borderBottom: '1px solid var(--color-border)',
          backgroundColor: 'var(--color-surface-2)',
          flexShrink: 0,
        }}
      >
        <span />
        <span>Date</span>
        <span>NPC · Zone · Character</span>
        <span style={{ textAlign: 'right' }}>Length</span>
        <span style={{ textAlign: 'right' }}>Total Dmg</span>
        <span style={{ textAlign: 'right' }}>DPS</span>
        <span />
      </div>

      <div style={{ flex: 1, overflowY: 'auto' }}>
        {loading && fights.length === 0 ? (
          <div
            style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: 24,
              color: 'var(--color-muted)',
              fontSize: 12,
            }}
          >
            Loading…
          </div>
        ) : empty ? (
          <div
            style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: 24,
              color: 'var(--color-muted)',
              fontSize: 12,
            }}
          >
            {empty}
          </div>
        ) : (
          fights.map((f) => <FightRow key={f.id} fight={f} onDelete={() => handleDeleteRow(f.id)} />)
        )}
      </div>

      {total > 0 && (
        <Pagination total={total} offset={offset} pageSize={PAGE_SIZE} onPage={setOffset} />
      )}
    </div>
  )
}

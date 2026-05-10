import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
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
  getCombatHistoryFacets,
} from '../services/api'
import type { EntityStats, HealerStats, HistoryFacets, HistoryListResponse, StoredFight } from '../types/combat'
import { groupBySession, fmtSessionGap } from '../lib/sessionGrouping'

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

// DatePreset drives the date-range filter. The named windows compute their
// bounds at query time relative to "now"; "custom" reads the date pickers;
// "all" sends no date bound. Default is "7d" — covers a typical week of
// raiding without hammering the user with thousands of trash fights, and
// fits inside the 30-day retention default without surprises.
type DatePreset = '24h' | '7d' | '30d' | 'custom' | 'all'

interface UIFilter {
  npc: string
  character: string
  zone: string
  preset: DatePreset
  // Only consulted when preset === 'custom'.
  startDate: string
  endDate: string
}

const EMPTY_FILTER: UIFilter = {
  npc: '',
  character: '',
  zone: '',
  preset: '7d',
  startDate: '',
  endDate: '',
}

// resolveDateRange converts a UIFilter into the start/end RFC3339 strings
// the backend expects. Returns undefined for missing bounds. For named
// presets, the start is "now minus N" and the end is left open so freshly-
// archived fights show up immediately.
function resolveDateRange(f: UIFilter): { start?: string; end?: string } {
  if (f.preset === 'all') return {}
  if (f.preset === 'custom') {
    return {
      start: f.startDate ? new Date(f.startDate + 'T00:00:00').toISOString() : undefined,
      end: f.endDate ? new Date(f.endDate + 'T23:59:59').toISOString() : undefined,
    }
  }
  const now = new Date()
  const back = new Date(now)
  switch (f.preset) {
    case '24h':
      back.setHours(back.getHours() - 24)
      break
    case '7d':
      back.setDate(back.getDate() - 7)
      break
    case '30d':
      back.setDate(back.getDate() - 30)
      break
  }
  return { start: back.toISOString() }
}

// PRESETS drives the date-range pill row. Order matters — narrowest to
// widest, with "all" as the escape hatch and "custom" enabling manual
// date pickers below.
const PRESETS: { value: DatePreset; label: string }[] = [
  { value: '24h', label: 'Last 24h' },
  { value: '7d', label: 'Last 7d' },
  { value: '30d', label: 'Last 30d' },
  { value: 'all', label: 'All' },
  { value: 'custom', label: 'Custom' },
]

function FilterBar({
  filter,
  facets,
  onChange,
  onApply,
  onClear,
  onRefresh,
  onDeleteAll,
}: {
  filter: UIFilter
  facets: HistoryFacets
  onChange: (f: UIFilter) => void
  onApply: () => void
  onClear: () => void
  onRefresh: () => void
  onDeleteAll: () => void
}): React.ReactElement {
  const inputStyle: React.CSSProperties = {
    padding: '4px 8px',
    fontSize: 11,
    background: 'var(--color-background)',
    border: '1px solid var(--color-border)',
    borderRadius: 4,
    color: 'var(--color-foreground)',
    cursor: 'pointer',
  }
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        padding: '8px 14px',
        borderBottom: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-surface-2)',
        flexShrink: 0,
      }}
    >
      {/* Row 1 — search + identity dropdowns + actions */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
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

        <select
          value={filter.character}
          onChange={(e) => onChange({ ...filter, character: e.target.value })}
          title="Character"
          style={{ ...inputStyle, minWidth: 130 }}
        >
          <option value="">All characters</option>
          {facets.characters.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>

        <select
          value={filter.zone}
          onChange={(e) => onChange({ ...filter, zone: e.target.value })}
          title="Zone"
          style={{ ...inputStyle, minWidth: 150 }}
        >
          <option value="">All zones</option>
          {facets.zones.map((z) => (
            <option key={z} value={z}>
              {z}
            </option>
          ))}
        </select>

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
          title="Reset all filters"
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

      {/* Row 2 — date-range presets + custom date pickers */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
        <span style={{ fontSize: 11, color: 'var(--color-muted)', marginRight: 4 }}>Range:</span>
        {PRESETS.map((p) => {
          const active = filter.preset === p.value
          return (
            <button
              key={p.value}
              onClick={() => onChange({ ...filter, preset: p.value })}
              style={{
                padding: '3px 10px',
                fontSize: 11,
                background: active ? 'var(--color-primary)' : 'var(--color-background)',
                border: '1px solid var(--color-border)',
                borderRadius: 4,
                color: active ? '#000' : 'var(--color-foreground)',
                cursor: 'pointer',
                fontWeight: active ? 600 : 400,
              }}
            >
              {p.label}
            </button>
          )
        })}
        {filter.preset === 'custom' && (
          <>
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
                marginLeft: 6,
              }}
            />
            <span style={{ fontSize: 11, color: 'var(--color-muted)' }}>→</span>
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
          </>
        )}
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

// ── session break divider ─────────────────────────────────────────────────────

// SessionBreak renders between two fights separated by more than
// SESSION_GAP_SECONDS of inactivity. Visual cue only — it does not change
// any DPS calculations or the ordering of the list.
function SessionBreak({ gapSeconds }: { gapSeconds: number }): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '6px 14px',
        backgroundColor: 'var(--color-surface-2)',
        borderTop: '1px solid var(--color-border)',
        borderBottom: '1px solid var(--color-border)',
        fontSize: 10,
        textTransform: 'uppercase',
        letterSpacing: '0.06em',
        color: 'var(--color-muted)',
      }}
    >
      <div style={{ flex: 1, height: 1, background: 'var(--color-border)' }} />
      <span style={{ whiteSpace: 'nowrap' }}>Session break · {fmtSessionGap(gapSeconds)}</span>
      <div style={{ flex: 1, height: 1, background: 'var(--color-border)' }} />
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

// ── confirm modal ─────────────────────────────────────────────────────────────

// ConfirmModal mirrors the themed confirmation pattern used elsewhere in
// the app (see TriggersPage). Backdrop closes; Escape isn't bound because
// React's modal patterns here don't use it consistently and adding only
// here would be inconsistent.
function ConfirmModal({
  title,
  body,
  confirmLabel,
  onCancel,
  onConfirm,
}: {
  title: string
  body: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  return (
    <div
      onClick={onCancel}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 420,
        }}
      >
        <div className="flex items-center gap-2">
          <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {title}
          </p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          {body}
        </p>
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onCancel}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-danger)',
              color: '#fff',
              border: '1px solid transparent',
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

// ConfirmAction discriminates the two destructive actions on this page —
// per-row delete (carries the target id and a label for the body) and
// clear-all (no payload). null means no modal showing.
type ConfirmAction =
  | { kind: 'deleteRow'; id: number; label: string }
  | { kind: 'clearAll' }
  | null

// ── page ──────────────────────────────────────────────────────────────────────

export default function CombatHistoryPage(): React.ReactElement {
  const [filter, setFilter] = useState<UIFilter>(EMPTY_FILTER)
  // appliedFilter is what's in flight to the backend; filter tracks the form
  // so typing doesn't refetch on every keystroke. Date-preset changes apply
  // immediately though — see the effect below.
  const [appliedFilter, setAppliedFilter] = useState<UIFilter>(EMPTY_FILTER)
  const [offset, setOffset] = useState(0)
  const [data, setData] = useState<HistoryListResponse | null>(null)
  const [facets, setFacets] = useState<HistoryFacets>({ characters: [], zones: [] })
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [confirm, setConfirm] = useState<ConfirmAction>(null)

  // Fetch the dropdown facets once on mount. They refresh after a destructive
  // action below so a now-empty character no longer shows up as an option.
  const refreshFacets = useCallback(() => {
    getCombatHistoryFacets()
      .then(setFacets)
      .catch(() => {
        // Non-fatal — dropdowns just stay empty.
      })
  }, [])
  useEffect(() => {
    refreshFacets()
  }, [refreshFacets])

  const fetchPage = useCallback(() => {
    setLoading(true)
    const range = resolveDateRange(appliedFilter)
    listCombatHistory({
      npc: appliedFilter.npc || undefined,
      character: appliedFilter.character || undefined,
      zone: appliedFilter.zone || undefined,
      start: range.start,
      end: range.end,
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

  // Pill clicks on the date-preset row apply immediately — they're a single
  // tap, no Apply button needed. Other fields still wait for Apply / Enter.
  useEffect(() => {
    if (filter.preset !== appliedFilter.preset) {
      setOffset(0)
      setAppliedFilter((prev) => ({ ...prev, preset: filter.preset, startDate: filter.startDate, endDate: filter.endDate }))
    }
    // Custom-mode date edits apply on Apply, not on every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter.preset])

  const handleApply = useCallback(() => {
    setOffset(0)
    setAppliedFilter(filter)
  }, [filter])

  const handleClearFilters = useCallback(() => {
    setOffset(0)
    setFilter(EMPTY_FILTER)
    setAppliedFilter(EMPTY_FILTER)
  }, [])

  const handleConfirm = useCallback(() => {
    if (!confirm) return
    const action = confirm
    setConfirm(null)
    const after = (): void => {
      fetchPage()
      refreshFacets()
    }
    if (action.kind === 'deleteRow') {
      deleteCombatHistoryFight(action.id).then(after).catch((e) => setError(e.message ?? String(e)))
    } else {
      clearCombatHistory().then(after).catch((e) => setError(e.message ?? String(e)))
    }
  }, [confirm, fetchPage, refreshFacets])

  const fights = data?.fights ?? []
  const total = data?.total ?? 0

  const empty = useMemo(() => {
    if (loading || error) return null
    if (fights.length > 0) return null
    const filtered =
      appliedFilter.npc ||
      appliedFilter.character ||
      appliedFilter.zone ||
      appliedFilter.preset !== 'all'
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
        facets={facets}
        onChange={setFilter}
        onApply={handleApply}
        onClear={handleClearFilters}
        onRefresh={() => {
          fetchPage()
          refreshFacets()
        }}
        onDeleteAll={() => setConfirm({ kind: 'clearAll' })}
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
          groupBySession(fights, (f) => String(f.id)).map((row) =>
            row.kind === 'gap' ? (
              <SessionBreak key={row.key} gapSeconds={row.gapSeconds} />
            ) : (
              <FightRow
                key={row.fight.id}
                fight={row.fight}
                onDelete={() =>
                  setConfirm({
                    kind: 'deleteRow',
                    id: row.fight.id,
                    label: `${row.fight.npc_name}${row.fight.zone ? ` in ${row.fight.zone}` : ''}`,
                  })
                }
              />
            ),
          )
        )}
      </div>

      {total > 0 && (
        <Pagination total={total} offset={offset} pageSize={PAGE_SIZE} onPage={setOffset} />
      )}

      {confirm && (
        <ConfirmModal
          title={confirm.kind === 'clearAll' ? 'Clear all combat history?' : 'Delete this fight?'}
          body={
            confirm.kind === 'clearAll'
              ? 'This permanently removes every saved fight from your local database. This cannot be undone.'
              : `Delete the saved fight against ${confirm.label}? This cannot be undone.`
          }
          confirmLabel={confirm.kind === 'clearAll' ? 'Clear all' : 'Delete'}
          onCancel={() => setConfirm(null)}
          onConfirm={handleConfirm}
        />
      )}
    </div>
  )
}

import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ScrollText,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Circle,
  CheckCircle2,
  Skull,
  Search,
  Download,
  Trash2,
  Clipboard,
  ClipboardCheck,
  Users,
} from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getCombatState, getLogStatus, resetCombatState } from '../services/api'
import type { CombatState, DeathRecord, EntityStats, FightSummary } from '../types/combat'
import type { LogTailerStatus } from '../types/logEvent'
import { rollupCombatants, useCombinePetWithOwner, petBadge, type RolledUpEntity } from '../lib/dpsRollup'

// ── Helpers ────────────────────────────────────────────────────────────────────

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

function fmtTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

// ── StatusBar ─────────────────────────────────────────────────────────────────

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  const base: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    padding: '6px 14px',
    fontSize: 11,
    borderBottom: '1px solid var(--color-border)',
    backgroundColor: 'var(--color-surface-2)',
    flexShrink: 0,
  }

  if (!status) {
    return (
      <div style={{ ...base, color: 'var(--color-muted)' }}>
        <Circle size={10} /> Loading…
      </div>
    )
  }
  if (!status.enabled) {
    return (
      <div style={{ ...base, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log parsing disabled — enable in Settings
      </div>
    )
  }
  if (!status.file_exists) {
    return (
      <div style={{ ...base, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log file not found
      </div>
    )
  }
  return (
    <div style={{ ...base, color: '#22c55e' }}>
      <CheckCircle2 size={11} /> Tailing log
    </div>
  )
}

// ── Combatant table ────────────────────────────────────────────────────────────

function CombatantTable({
  combatants,
  totalDamage,
  combine,
  fightDuration,
}: {
  combatants: EntityStats[]
  totalDamage: number
  combine: boolean
  fightDuration: number
}): React.ReactElement {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const rolled = rollupCombatants(combatants, combine, fightDuration)

  return (
    <div
      style={{
        margin: '0 0 0 24px',
        borderLeft: '2px solid var(--color-border)',
        paddingLeft: 12,
        marginBottom: 4,
      }}
    >
      {/* Header */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 48px 72px 60px 48px',
          gap: '0 8px',
          padding: '3px 8px 3px 0',
          fontSize: 10,
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: 'var(--color-muted)',
          borderBottom: '1px solid var(--color-border)',
        }}
      >
        <span>Name</span>
        <span style={{ textAlign: 'right' }}>%</span>
        <span style={{ textAlign: 'right' }}>Damage</span>
        <span style={{ textAlign: 'right' }}>DPS</span>
        <span style={{ textAlign: 'right' }}>Max</span>
      </div>

      {rolled.map((c) => renderRolledRow(c, totalDamage, expanded, setExpanded))}
    </div>
  )
}

function renderRolledRow(
  c: RolledUpEntity,
  totalDamage: number,
  expanded: Set<string>,
  setExpanded: React.Dispatch<React.SetStateAction<Set<string>>>,
): React.ReactNode {
  const isYou = c.name === 'You'
  const hasPets = c.pets.length > 0
  const isExpanded = expanded.has(c.name)
  return (
    <React.Fragment key={c.name}>
      <div
        onClick={hasPets ? () => setExpanded((prev) => {
          const next = new Set(prev)
          if (next.has(c.name)) next.delete(c.name)
          else next.add(c.name)
          return next
        }) : undefined}
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 48px 72px 60px 48px',
          gap: '0 8px',
          padding: '3px 8px 3px 0',
          borderBottom: '1px solid rgba(255,255,255,0.03)',
          fontSize: 11,
          cursor: hasPets ? 'pointer' : 'default',
        }}
      >
        <span
          style={{
            color: isYou ? 'var(--color-primary)' : 'var(--color-foreground)',
            fontWeight: isYou ? 600 : 400,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {c.name}
          {hasPets && <span style={{ color: 'var(--color-muted)', fontWeight: 400 }}>{petBadge(c.pets)}</span>}
        </span>
        <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>
          {pct(c.total_damage, totalDamage)}
        </span>
        <span style={{ textAlign: 'right', color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums' }}>
          {fmt(c.total_damage)}
        </span>
        <span style={{ textAlign: 'right', color: '#f97316', fontVariantNumeric: 'tabular-nums' }}>
          {fmtDPS(c.dps)}
        </span>
        <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>
          {fmt(c.max_hit)}
        </span>
      </div>
      {hasPets && isExpanded && c.pets.map((p) => (
        <div
          key={p.name}
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 48px 72px 60px 48px',
            gap: '0 8px',
            padding: '2px 8px 2px 14px',
            fontSize: 11,
            color: 'var(--color-muted-foreground)',
          }}
        >
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>↳ {p.name}</span>
          <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>{pct(p.total_damage, totalDamage)}</span>
          <span style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{fmt(p.total_damage)}</span>
          <span style={{ textAlign: 'right', color: '#f97316', fontVariantNumeric: 'tabular-nums' }}>{fmtDPS(p.dps)}</span>
          <span style={{ textAlign: 'right', color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>{fmt(p.max_hit)}</span>
        </div>
      ))}
    </React.Fragment>
  )
}

// ── Fight row ─────────────────────────────────────────────────────────────────

function FightRow({
  fight,
  index,
  total,
  combine,
}: {
  fight: FightSummary
  index: number
  total: number
  combine: boolean
}): React.ReactElement {
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)
  const fightNum = total - index

  function handleCopy(e: React.MouseEvent): void {
    e.stopPropagation()
    navigator.clipboard.writeText(buildFightText(fight, combine)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }).catch(() => {})
  }

  return (
    <div
      style={{
        borderBottom: '1px solid var(--color-border)',
      }}
    >
      {/* Summary row */}
      <div
        style={{
          width: '100%',
          display: 'grid',
          gridTemplateColumns: '20px 60px 1fr 90px 90px 90px 24px',
          gap: '0 12px',
          alignItems: 'center',
          padding: '7px 14px',
          cursor: 'pointer',
          color: 'var(--color-foreground)',
          fontSize: 12,
        }}
        onClick={() => setExpanded((v) => !v)}
      >
        {/* Chevron */}
        <span style={{ color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}>
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </span>

        {/* Fight # */}
        <span style={{ color: 'var(--color-muted)', fontSize: 11 }}>
          #{fightNum}
        </span>

        {/* Time + NPC target */}
        <span style={{ color: 'var(--color-muted)', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {fmtTime(fight.start_time)}
          <span style={{ color: 'var(--color-border)', margin: '0 4px' }}>·</span>
          {fmtDuration(fight.duration_seconds)}
          {fight.primary_target && (
            <>
              <span style={{ color: 'var(--color-border)', margin: '0 4px' }}>—</span>
              <span style={{ color: 'var(--color-foreground)', fontStyle: 'italic' }}>{fight.primary_target}</span>
            </>
          )}
        </span>

        {/* Total damage */}
        <span
          style={{
            textAlign: 'right',
            fontVariantNumeric: 'tabular-nums',
            color: 'var(--color-foreground)',
          }}
        >
          {fmt(fight.total_damage)}
        </span>

        {/* Total DPS */}
        <span
          style={{
            textAlign: 'right',
            fontVariantNumeric: 'tabular-nums',
            color: '#f97316',
          }}
        >
          {fmtDPS(fight.total_dps)} DPS
        </span>

        {/* Your DPS */}
        <span
          style={{
            textAlign: 'right',
            fontVariantNumeric: 'tabular-nums',
            color: 'var(--color-primary)',
            fontSize: 11,
          }}
        >
          {fight.you_damage > 0 ? `${fmtDPS(fight.you_dps)} me` : '—'}
        </span>

        {/* Copy fight summary */}
        <button
          onClick={handleCopy}
          title="Copy fight summary to clipboard"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            padding: 2,
            color: copied ? '#22c55e' : 'var(--color-muted)',
          }}
        >
          {copied ? <ClipboardCheck size={12} /> : <Clipboard size={12} />}
        </button>
      </div>

      {/* Expanded combatant breakdown */}
      {expanded && fight.combatants.length > 0 && (
        <div style={{ padding: '4px 14px 8px' }}>
          <CombatantTable
            combatants={fight.combatants}
            totalDamage={fight.total_damage}
            combine={combine}
            fightDuration={fight.duration_seconds}
          />
        </div>
      )}
      {expanded && fight.combatants.length === 0 && (
        <div
          style={{
            padding: '4px 14px 8px',
            fontSize: 11,
            color: 'var(--color-muted)',
          }}
        >
          No combatant data
        </div>
      )}
    </div>
  )
}

// ── Column headers ─────────────────────────────────────────────────────────────

function TableHeader(): React.ReactElement {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '20px 60px 1fr 90px 90px 90px 24px',
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
      <span>Fight</span>
      <span>Time · Duration</span>
      <span style={{ textAlign: 'right' }}>Total Dmg</span>
      <span style={{ textAlign: 'right' }}>Total DPS</span>
      <span style={{ textAlign: 'right' }}>My DPS</span>
      <span />
    </div>
  )
}

// ── Death log section ──────────────────────────────────────────────────────────

function DeathLogSection({ deaths }: { deaths: DeathRecord[] }): React.ReactElement {
  const [expanded, setExpanded] = useState(false)
  const count = deaths.length

  return (
    <div
      style={{
        borderTop: '1px solid var(--color-border)',
        flexShrink: 0,
        backgroundColor: 'var(--color-surface-2)',
      }}
    >
      <button
        onClick={() => setExpanded((v) => !v)}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 14px',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          textAlign: 'left',
          color: 'var(--color-foreground)',
          fontSize: 11,
        }}
      >
        <span style={{ color: '#ef4444', display: 'flex', alignItems: 'center' }}>
          <Skull size={12} />
        </span>
        <span style={{ color: '#ef4444', fontWeight: 600 }}>
          {count} death{count !== 1 ? 's' : ''} this session
        </span>
        <span style={{ color: 'var(--color-muted)', marginLeft: 'auto', display: 'flex', alignItems: 'center' }}>
          {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        </span>
      </button>

      {expanded && (
        <div style={{ maxHeight: 160, overflowY: 'auto', padding: '0 14px 8px' }}>
          {[...deaths].reverse().map((d, i) => (
            <div
              key={i}
              style={{
                display: 'grid',
                gridTemplateColumns: '70px 1fr 1fr',
                gap: '0 12px',
                padding: '3px 0',
                fontSize: 11,
                borderBottom: '1px solid rgba(255,255,255,0.03)',
              }}
            >
              <span style={{ color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>
                {fmtTime(d.timestamp)}
              </span>
              <span style={{ color: 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {d.zone || '—'}
              </span>
              <span style={{ color: '#ef4444', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {d.slain_by ? `by ${d.slain_by}` : 'unknown cause'}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Session footer ─────────────────────────────────────────────────────────────

function SessionFooter({ combat }: { combat: CombatState }): React.ReactElement {
  const fights = combat.recent_fights.length
  return (
    <div
      style={{
        padding: '6px 14px',
        fontSize: 11,
        color: 'var(--color-muted)',
        borderTop: '1px solid var(--color-border)',
        display: 'flex',
        gap: 14,
        flexShrink: 0,
        backgroundColor: 'var(--color-surface-2)',
        flexWrap: 'wrap',
      }}
    >
      <span>{fights} fight{fights !== 1 ? 's' : ''} (last 20)</span>
      <span style={{ color: 'var(--color-foreground)' }}>
        {fmt(combat.session_damage)} dmg (me)
      </span>
      <span style={{ color: '#f97316' }}>
        {fmtDPS(combat.session_dps)} DPS session avg (me)
      </span>
    </div>
  )
}

// ── Filter bar ─────────────────────────────────────────────────────────────────

type TimeRange = 'all' | '30m' | '1h' | '2h'

interface FilterState {
  search: string
  timeRange: TimeRange
  meOnly: boolean
}

function FilterBar({
  filters,
  onChange,
  onClear,
  onExport,
  onCopySession,
  sessionCopied,
  combine,
  onToggleCombine,
}: {
  filters: FilterState
  onChange: (f: FilterState) => void
  onClear: () => void
  onExport: () => void
  onCopySession: () => void
  sessionCopied: boolean
  combine: boolean
  onToggleCombine: () => void
}): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '6px 14px',
        borderBottom: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-surface-2)',
        flexShrink: 0,
        flexWrap: 'wrap',
      }}
    >
      {/* Search input */}
      <div style={{ position: 'relative', flex: '1 1 140px', minWidth: 120, maxWidth: 220 }}>
        <Search
          size={11}
          style={{
            position: 'absolute',
            left: 7,
            top: '50%',
            transform: 'translateY(-50%)',
            color: 'var(--color-muted)',
            pointerEvents: 'none',
          }}
        />
        <input
          type="text"
          placeholder="Search combatants…"
          value={filters.search}
          onChange={(e) => onChange({ ...filters, search: e.target.value })}
          style={{
            width: '100%',
            paddingLeft: 24,
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

      {/* Time range */}
      <select
        value={filters.timeRange}
        onChange={(e) => onChange({ ...filters, timeRange: e.target.value as TimeRange })}
        style={{
          padding: '4px 6px',
          fontSize: 11,
          background: 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: 'var(--color-foreground)',
          cursor: 'pointer',
        }}
      >
        <option value="all">All time</option>
        <option value="30m">Last 30m</option>
        <option value="1h">Last 1h</option>
        <option value="2h">Last 2h</option>
      </select>

      {/* Combine pets toggle */}
      <button
        onClick={onToggleCombine}
        title={combine ? 'Pet damage rolled up under owner — click to split' : 'Pets shown separately — click to combine'}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          padding: '4px 8px',
          fontSize: 11,
          background: combine ? 'var(--color-primary)' : 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: combine ? '#000' : 'var(--color-foreground)',
          cursor: 'pointer',
          whiteSpace: 'nowrap',
        }}
      >
        <Users size={11} /> Pets
      </button>

      {/* Me only toggle */}
      <button
        onClick={() => onChange({ ...filters, meOnly: !filters.meOnly })}
        style={{
          padding: '4px 8px',
          fontSize: 11,
          background: filters.meOnly ? 'var(--color-primary)' : 'var(--color-background)',
          border: '1px solid var(--color-border)',
          borderRadius: 4,
          color: filters.meOnly ? '#000' : 'var(--color-foreground)',
          cursor: 'pointer',
          whiteSpace: 'nowrap',
        }}
      >
        Me only
      </button>

      <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
        {/* Copy session summary */}
        <button
          onClick={onCopySession}
          title="Copy session summary to clipboard"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '4px 8px',
            fontSize: 11,
            background: 'var(--color-background)',
            border: '1px solid var(--color-border)',
            borderRadius: 4,
            color: sessionCopied ? '#22c55e' : 'var(--color-muted)',
            cursor: 'pointer',
          }}
        >
          {sessionCopied ? <ClipboardCheck size={11} /> : <Clipboard size={11} />} Copy
        </button>

        {/* Export */}
        <button
          onClick={onExport}
          title="Export visible fights as CSV"
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
          <Download size={11} /> Export
        </button>

        {/* Clear */}
        <button
          onClick={onClear}
          title="Clear combat log"
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
          <Trash2 size={11} /> Clear
        </button>
      </div>
    </div>
  )
}

// ── Filter logic ───────────────────────────────────────────────────────────────

function applyFilters(fights: FightSummary[], filters: FilterState): FightSummary[] {
  let result = fights

  if (filters.timeRange !== 'all') {
    const minutes = filters.timeRange === '30m' ? 30 : filters.timeRange === '1h' ? 60 : 120
    const cutoff = Date.now() - minutes * 60 * 1000
    result = result.filter((f) => new Date(f.start_time).getTime() >= cutoff)
  }

  if (filters.meOnly) {
    result = result.filter((f) => f.you_damage > 0)
  }

  const query = filters.search.trim().toLowerCase()
  if (query) {
    result = result.filter((f) =>
      f.combatants.some((c) => c.name.toLowerCase().includes(query))
    )
  }

  return result
}

// ── Clipboard helpers ──────────────────────────────────────────────────────────

function buildFightText(fight: FightSummary, combine: boolean): string {
  const target = fight.primary_target ?? 'Unknown'
  const dur = fmtDuration(fight.duration_seconds)
  const lines: string[] = [`[PQ Companion] Fight: ${target} (${dur})`]
  const rows = rollupCombatants(fight.combatants, combine, fight.duration_seconds)
  for (const c of rows) {
    lines.push(`${c.name}${petBadge(c.pets)}: ${fmtDPS(c.dps)} DPS (${fmt(c.total_damage)} total)`)
  }
  return lines.join('\n')
}

function buildSessionText(fights: FightSummary[], sessionDPS: number): string {
  const lines: string[] = [`[PQ Companion] Session: ${fights.length} fight${fights.length !== 1 ? 's' : ''} | ${fmtDPS(sessionDPS)} DPS avg (me)`]
  return lines.join('\n')
}

// ── Export helpers ─────────────────────────────────────────────────────────────

function exportFightsCSV(fights: FightSummary[]): void {
  const rows: string[] = [
    'Fight,Start Time,Duration (s),Total Damage,Total DPS,My Damage,My DPS,Combatant,Combatant Damage,Combatant DPS,Combatant Max Hit',
  ]

  fights.forEach((f, i) => {
    const fightNum = fights.length - i
    const baseRow = [
      fightNum,
      f.start_time,
      f.duration_seconds.toFixed(1),
      f.total_damage,
      f.total_dps.toFixed(1),
      f.you_damage,
      f.you_dps.toFixed(1),
    ]
    if (f.combatants.length === 0) {
      rows.push([...baseRow, '', '', '', ''].join(','))
    } else {
      f.combatants.forEach((c) => {
        rows.push([...baseRow, c.name, c.total_damage, c.dps.toFixed(1), c.max_hit].join(','))
      })
    }
  })

  const blob = new Blob([rows.join('\n')], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `combat-log-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function CombatLogPage(): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [filters, setFilters] = useState<FilterState>({ search: '', timeRange: 'all', meOnly: false })
  const [sessionCopied, setSessionCopied] = useState(false)
  const [combine, setCombine] = useCombinePetWithOwner()

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:combat') {
      setCombat(msg.data as CombatState)
    }
  }, [])

  useWebSocket(handleMessage)

  const allFights = combat?.recent_fights ?? []
  const deaths = combat?.deaths ?? []

  const visibleFights = useMemo(() => applyFilters(allFights, filters), [allFights, filters])

  const handleClear = useCallback(() => {
    resetCombatState().catch(() => {})
  }, [])

  const handleExport = useCallback(() => {
    exportFightsCSV(visibleFights)
  }, [visibleFights])

  const handleCopySession = useCallback(() => {
    if (!combat) return
    const text = buildSessionText(visibleFights, combat.session_dps)
    navigator.clipboard.writeText(text).then(() => {
      setSessionCopied(true)
      setTimeout(() => setSessionCopied(false), 1500)
    }).catch(() => {})
  }, [combat, visibleFights])

  const isFiltered =
    filters.search !== '' || filters.timeRange !== 'all' || filters.meOnly

  return (
    <div className="flex h-full flex-col overflow-hidden" style={{ backgroundColor: 'var(--color-background)' }}>
      {/* Header */}
      <div
        className="flex shrink-0 items-center justify-between border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2">
          <ScrollText size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Combat Log
          </h1>
          {combat && (
            <span
              className="rounded px-1.5 py-0.5 text-[10px]"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
            >
              {isFiltered
                ? `${visibleFights.length} / ${allFights.length} fight${allFights.length !== 1 ? 's' : ''}`
                : `${allFights.length} fight${allFights.length !== 1 ? 's' : ''}`}
            </span>
          )}
        </div>
      </div>

      <StatusBar status={status} />

      <FilterBar
        filters={filters}
        onChange={setFilters}
        onClear={handleClear}
        onExport={handleExport}
        onCopySession={handleCopySession}
        sessionCopied={sessionCopied}
        combine={combine}
        onToggleCombine={() => setCombine(!combine)}
      />

      {combat === null ? (
        <div
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <p style={{ fontSize: 12, color: 'var(--color-muted)' }}>Loading…</p>
        </div>
      ) : visibleFights.length === 0 ? (
        <>
          <div
            style={{
              flex: 1,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 8,
              color: 'var(--color-muted)',
            }}
          >
            <ScrollText size={32} style={{ opacity: 0.3 }} />
            <p style={{ fontSize: 12, margin: 0 }}>
              {isFiltered ? 'No fights match your filters' : 'No completed fights yet'}
            </p>
            <p style={{ fontSize: 11, margin: 0, opacity: 0.6 }}>
              {isFiltered
                ? 'Try adjusting the search or time range'
                : 'Fight history will appear here as you engage enemies'}
            </p>
          </div>
          {deaths.length > 0 && <DeathLogSection deaths={deaths} />}
        </>
      ) : (
        <>
          <TableHeader />

          <div style={{ flex: 1, overflowY: 'auto' }}>
            {visibleFights.map((fight, i) => (
              <FightRow
                key={fight.start_time}
                fight={fight}
                index={i}
                total={visibleFights.length}
                combine={combine}
              />
            ))}
          </div>

          <SessionFooter combat={combat} />
          {deaths.length > 0 && <DeathLogSection deaths={deaths} />}
        </>
      )}
    </div>
  )
}

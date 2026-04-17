import React, { useCallback, useEffect, useState } from 'react'
import {
  ScrollText,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Circle,
  CheckCircle2,
  Skull,
} from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getCombatState, getLogStatus } from '../services/api'
import type { CombatState, DeathRecord, EntityStats, FightSummary } from '../types/combat'
import type { LogTailerStatus } from '../types/logEvent'

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
}: {
  combatants: EntityStats[]
  totalDamage: number
}): React.ReactElement {
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

      {combatants.map((c) => {
        const isYou = c.name === 'You'
        return (
          <div
            key={c.name}
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 48px 72px 60px 48px',
              gap: '0 8px',
              padding: '3px 8px 3px 0',
              borderBottom: '1px solid rgba(255,255,255,0.03)',
              fontSize: 11,
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
            </span>
            <span
              style={{
                textAlign: 'right',
                color: 'var(--color-muted)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {pct(c.total_damage, totalDamage)}
            </span>
            <span
              style={{
                textAlign: 'right',
                color: 'var(--color-foreground)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {fmt(c.total_damage)}
            </span>
            <span
              style={{
                textAlign: 'right',
                color: '#f97316',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {fmtDPS(c.dps)}
            </span>
            <span
              style={{
                textAlign: 'right',
                color: 'var(--color-muted)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {fmt(c.max_hit)}
            </span>
          </div>
        )
      })}
    </div>
  )
}

// ── Fight row ─────────────────────────────────────────────────────────────────

function FightRow({
  fight,
  index,
  total,
}: {
  fight: FightSummary
  index: number
  total: number
}): React.ReactElement {
  const [expanded, setExpanded] = useState(false)
  const fightNum = total - index

  return (
    <div
      style={{
        borderBottom: '1px solid var(--color-border)',
      }}
    >
      {/* Summary row */}
      <button
        onClick={() => setExpanded((v) => !v)}
        style={{
          width: '100%',
          display: 'grid',
          gridTemplateColumns: '20px 60px 1fr 90px 90px 90px',
          gap: '0 12px',
          alignItems: 'center',
          padding: '7px 14px',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          textAlign: 'left',
          color: 'var(--color-foreground)',
          fontSize: 12,
        }}
      >
        {/* Chevron */}
        <span style={{ color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}>
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </span>

        {/* Fight # */}
        <span style={{ color: 'var(--color-muted)', fontSize: 11 }}>
          #{fightNum}
        </span>

        {/* Time */}
        <span style={{ color: 'var(--color-muted)', fontSize: 11 }}>
          {fmtTime(fight.start_time)}
          <span style={{ color: 'var(--color-border)', margin: '0 4px' }}>·</span>
          {fmtDuration(fight.duration_seconds)}
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
      </button>

      {/* Expanded combatant breakdown */}
      {expanded && fight.combatants.length > 0 && (
        <div style={{ padding: '4px 14px 8px' }}>
          <CombatantTable
            combatants={fight.combatants}
            totalDamage={fight.total_damage}
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
        gridTemplateColumns: '20px 60px 1fr 90px 90px 90px',
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

// ── Page ───────────────────────────────────────────────────────────────────────

export default function CombatLogPage(): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)

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

  const fights = combat?.recent_fights ?? []
  const deaths = combat?.deaths ?? []

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
              {fights.length} fight{fights.length !== 1 ? 's' : ''}
            </span>
          )}
        </div>
      </div>

      <StatusBar status={status} />

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
      ) : fights.length === 0 ? (
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
            <p style={{ fontSize: 12, margin: 0 }}>No completed fights yet</p>
            <p style={{ fontSize: 11, margin: 0, opacity: 0.6 }}>
              Fight history will appear here as you engage enemies
            </p>
          </div>
          {deaths.length > 0 && <DeathLogSection deaths={deaths} />}
        </>
      ) : (
        <>
          <TableHeader />

          <div style={{ flex: 1, overflowY: 'auto' }}>
            {fights.map((fight, i) => (
              <FightRow
                key={fight.start_time}
                fight={fight}
                index={i}
                total={fights.length}
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

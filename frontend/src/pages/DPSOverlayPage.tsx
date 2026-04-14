import React, { useCallback, useEffect, useState } from 'react'
import { Swords, Circle, CheckCircle2, AlertTriangle, ExternalLink } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getCombatState, getLogStatus } from '../services/api'
import OverlayWindow from '../components/OverlayWindow'
import type { CombatState, EntityStats, FightState } from '../types/combat'
import type { LogTailerStatus } from '../types/logEvent'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmt(n: number): string {
  return n.toLocaleString()
}

function fmtDPS(n: number): string {
  return n.toFixed(1)
}

function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

function fmtDuration(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = Math.floor(secs % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

// ── Sub-components ─────────────────────────────────────────────────────────────

function ConnPill({ state }: { state: string }): React.ReactElement {
  const color =
    state === 'open' ? '#22c55e' : state === 'connecting' ? '#f97316' : '#6b7280'
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: color, display: 'inline-block' }} />
      {state === 'open' ? 'Live' : state === 'connecting' ? 'Connecting' : 'Off'}
    </span>
  )
}

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  const style: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    padding: '6px 10px',
    fontSize: 11,
    borderBottom: '1px solid var(--color-border)',
    flexShrink: 0,
    backgroundColor: 'var(--color-surface-2)',
  }

  if (!status) {
    return (
      <div style={{ ...style, color: 'var(--color-muted)' }}>
        <Circle size={10} /> Loading…
      </div>
    )
  }
  if (!status.enabled) {
    return (
      <div style={{ ...style, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log parsing disabled — enable in Settings
      </div>
    )
  }
  if (!status.file_exists) {
    return (
      <div style={{ ...style, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log file not found
      </div>
    )
  }
  return (
    <div style={{ ...style, color: '#22c55e' }}>
      <CheckCircle2 size={11} /> Tailing log
    </div>
  )
}

// ── DPS row ────────────────────────────────────────────────────────────────────

function CombatantRow({
  stat,
  totalDamage,
  isYou,
}: {
  stat: EntityStats
  totalDamage: number
  isYou: boolean
}): React.ReactElement {
  const barPct = totalDamage > 0 ? (stat.total_damage / totalDamage) * 100 : 0

  return (
    <div
      style={{
        position: 'relative',
        padding: '5px 10px',
        display: 'grid',
        gridTemplateColumns: '1fr auto auto auto',
        gap: '0 10px',
        alignItems: 'center',
        borderBottom: '1px solid var(--color-border)',
        overflow: 'hidden',
      }}
    >
      {/* damage bar background */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${barPct}%`,
          backgroundColor: isYou ? 'rgba(99,102,241,0.18)' : 'rgba(255,255,255,0.05)',
          pointerEvents: 'none',
        }}
      />

      {/* Name */}
      <span
        style={{
          fontSize: 12,
          fontWeight: isYou ? 600 : 400,
          color: isYou ? 'var(--color-primary)' : 'var(--color-foreground)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          position: 'relative',
        }}
      >
        {stat.name}
      </span>

      {/* % share */}
      <span style={{ fontSize: 11, color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {pct(stat.total_damage, totalDamage)}
      </span>

      {/* Total dmg */}
      <span style={{ fontSize: 11, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {fmt(stat.total_damage)}
      </span>

      {/* DPS */}
      <span style={{ fontSize: 11, color: '#f97316', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
        {fmtDPS(stat.dps)}
      </span>
    </div>
  )
}

// ── Fight panel ────────────────────────────────────────────────────────────────

function FightPanel({
  fight,
  showAll,
}: {
  fight: FightState
  showAll: boolean
}): React.ReactElement {
  const rows = showAll
    ? fight.combatants
    : fight.combatants.filter((c) => c.name === 'You')

  const totalDmg = showAll ? fight.total_damage : fight.you_damage

  return (
    <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      {/* Column headers */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr auto auto auto',
          gap: '0 10px',
          padding: '4px 10px',
          fontSize: 10,
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: 'var(--color-muted)',
          borderBottom: '1px solid var(--color-border)',
          flexShrink: 0,
        }}
      >
        <span>Name</span>
        <span>%</span>
        <span>Dmg</span>
        <span style={{ textAlign: 'right' }}>DPS</span>
      </div>

      {rows.length === 0 ? (
        <div style={{ padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' }}>
          No damage data
        </div>
      ) : (
        rows.map((s) => (
          <CombatantRow key={s.name} stat={s} totalDamage={totalDmg} isYou={s.name === 'You'} />
        ))
      )}
    </div>
  )
}

// ── No-combat state ────────────────────────────────────────────────────────────

function NotInCombat(): React.ReactElement {
  return (
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
      <Swords size={32} style={{ opacity: 0.3 }} />
      <p style={{ fontSize: 12, margin: 0 }}>Not in combat</p>
    </div>
  )
}

// ── Session summary ────────────────────────────────────────────────────────────

function SessionBar({ combat }: { combat: CombatState }): React.ReactElement {
  const fights = combat.recent_fights.length
  return (
    <div
      style={{
        padding: '5px 10px',
        fontSize: 11,
        color: 'var(--color-muted)',
        borderTop: '1px solid var(--color-border)',
        display: 'flex',
        gap: 12,
        flexShrink: 0,
        backgroundColor: 'var(--color-surface-2)',
        flexWrap: 'wrap',
      }}
    >
      <span>{fights} fight{fights !== 1 ? 's' : ''}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{fmt(combat.session_damage)} dmg</span>
      <span style={{ color: '#f97316' }}>{fmtDPS(combat.session_dps)} DPS (session)</span>
    </div>
  )
}

// ── Filter toggle button ───────────────────────────────────────────────────────

function FilterButton({
  showAll,
  onToggle,
}: {
  showAll: boolean
  onToggle: () => void
}): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      style={{
        fontSize: 10,
        padding: '2px 7px',
        borderRadius: 4,
        border: '1px solid var(--color-border)',
        backgroundColor: showAll ? 'var(--color-primary)' : 'var(--color-surface)',
        color: showAll ? '#fff' : 'var(--color-muted-foreground)',
        cursor: 'pointer',
        fontWeight: 600,
        letterSpacing: '0.04em',
        textTransform: 'uppercase',
      }}
    >
      {showAll ? 'All' : 'Me'}
    </button>
  )
}

// ── Combat status strip ────────────────────────────────────────────────────────

function CombatStrip({ combat }: { combat: CombatState }): React.ReactElement {
  const fight = combat.current_fight
  return (
    <div
      style={{
        padding: '4px 10px',
        fontSize: 11,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        borderBottom: '1px solid var(--color-border)',
        flexShrink: 0,
        backgroundColor: combat.in_combat ? 'rgba(220,38,38,0.15)' : 'transparent',
        color: combat.in_combat ? '#f87171' : 'var(--color-muted)',
      }}
    >
      <span
        style={{
          width: 7,
          height: 7,
          borderRadius: '50%',
          backgroundColor: combat.in_combat ? '#ef4444' : '#6b7280',
          display: 'inline-block',
          flexShrink: 0,
        }}
      />
      {combat.in_combat && fight ? (
        <>
          <span style={{ fontWeight: 600, color: '#f87171' }}>In Combat</span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span>{fmtDuration(fight.duration_seconds)}</span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span style={{ color: '#f97316' }}>{fmtDPS(fight.total_dps)} DPS</span>
        </>
      ) : (
        <span>Not in combat</span>
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DPSOverlayPage(): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [showAll, setShowAll] = useState(true)

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:combat') {
      setCombat(msg.data as CombatState)
    }
  }, [])

  const wsState = useWebSocket(handleMessage)

  return (
    <div
      style={{
        position: 'relative',
        flex: 1,
        overflow: 'hidden',
        backgroundColor: 'var(--color-background)',
      }}
    >
      {/* Background hint */}
      <div
        style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          pointerEvents: 'none',
          userSelect: 'none',
        }}
      >
        <p style={{ fontSize: 12, color: 'var(--color-muted)', opacity: 0.4 }}>
          Drag title bar to move · Drag edges/corners to resize
        </p>
      </div>

      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Swords size={13} style={{ color: 'var(--color-primary)' }} />
            DPS Meter
          </span>
        }
        headerRight={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <FilterButton showAll={showAll} onToggle={() => setShowAll((v) => !v)} />
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleDPS()}
                title="Pop out as floating overlay"
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  padding: '1px 3px',
                  color: 'var(--color-muted)',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <ExternalLink size={12} />
              </button>
            )}
            <ConnPill state={wsState} />
          </div>
        }
        defaultWidth={400}
        defaultHeight={440}
        defaultX={24}
        defaultY={24}
      >
        <StatusBar status={status} />

        {combat === null ? (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <p style={{ fontSize: 12, color: 'var(--color-muted)' }}>Loading…</p>
          </div>
        ) : (
          <>
            <CombatStrip combat={combat} />

            {combat.in_combat && combat.current_fight ? (
              <FightPanel fight={combat.current_fight} showAll={showAll} />
            ) : (
              <NotInCombat />
            )}

            <SessionBar combat={combat} />
          </>
        )}
      </OverlayWindow>
    </div>
  )
}

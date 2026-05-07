/**
 * HPSOverlayWindowPage — renders in the dedicated always-on-top Electron overlay
 * window for the HPS meter. No sidebar, no title bar, transparent background.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { HeartPulse } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import { getCombatState } from '../services/api'
import type { CombatState, HealerStats, FightState } from '../types/combat'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmt(n: number): string {
  return n.toLocaleString()
}

function fmtHPS(n: number): string {
  return n.toFixed(1)
}

function fmtDur(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = Math.floor(secs % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

// ── Row ────────────────────────────────────────────────────────────────────────

function Row({ stat, totalHeal }: { stat: HealerStats; totalHeal: number }): React.ReactElement {
  const isYou = stat.name === 'You'
  const barPct = totalHeal > 0 ? (stat.total_heal / totalHeal) * 100 : 0

  return (
    <div
      style={{
        position: 'relative',
        display: 'grid',
        gridTemplateColumns: '1fr auto auto auto',
        gap: '0 8px',
        padding: '4px 8px',
        alignItems: 'center',
        borderBottom: '1px solid rgba(255,255,255,0.06)',
        overflow: 'hidden',
      }}
    >
      {/* bar */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${barPct}%`,
          backgroundColor: isYou ? 'rgba(34,197,94,0.25)' : 'rgba(255,255,255,0.06)',
          pointerEvents: 'none',
        }}
      />
      <span
        style={{
          fontSize: 12,
          fontWeight: isYou ? 700 : 400,
          color: isYou ? '#4ade80' : 'rgba(255,255,255,0.85)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          position: 'relative',
        }}
      >
        {stat.name}
      </span>
      <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {pct(stat.total_heal, totalHeal)}
      </span>
      <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.7)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {fmt(stat.total_heal)}
      </span>
      <span style={{ fontSize: 11, color: '#4ade80', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
        {fmtHPS(stat.hps)}
      </span>
    </div>
  )
}

// ── Heal table ─────────────────────────────────────────────────────────────────

function HealTable({ fight, showAll }: { fight: FightState; showAll: boolean }): React.ReactElement {
  const healers = fight.healers ?? []
  const rows = showAll ? healers : healers.filter((h) => h.name === 'You')
  const totalHeal = showAll ? fight.total_heal : fight.you_heal

  return (
    <div style={{ flex: 1, overflow: 'auto' }}>
      {/* Header */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr auto auto auto',
          gap: '0 8px',
          padding: '3px 8px',
          fontSize: 9,
          fontWeight: 700,
          textTransform: 'uppercase',
          letterSpacing: '0.06em',
          color: 'rgba(255,255,255,0.3)',
          borderBottom: '1px solid rgba(255,255,255,0.08)',
        }}
      >
        <span>Name</span>
        <span>%</span>
        <span>Heal</span>
        <span style={{ textAlign: 'right' }}>HPS</span>
      </div>

      {rows.length === 0 ? (
        <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>No data</p>
      ) : (
        rows.map((h) => <Row key={h.name} stat={h} totalHeal={totalHeal} />)
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function HPSOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [showAll, setShowAll] = useState(true)

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:combat') {
      setCombat(msg.data as CombatState)
    }
  }, [])

  useWebSocket(handleMessage)

  const fight = combat?.current_fight

  return (
    <div
      onMouseEnter={enableInteraction}
      onMouseLeave={enableClickThrough}
      style={{
        width: '100vw',
        height: '100vh',
        backgroundColor: `rgba(10,10,12,${opacity})`,
        border: '1px solid rgba(255,255,255,0.12)',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      {/* ── Drag handle / title bar ─────────────────────────────────────── */}
      <div
        className={locked ? 'no-drag' : 'drag-region'}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <HeartPulse size={11} style={{ color: '#4ade80' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>HPS</span>
          {fight && (
            <span style={{ fontSize: 10, color: '#4ade80', marginLeft: 4 }}>
              {fmtHPS(showAll ? fight.total_hps : fight.you_hps)}
            </span>
          )}
        </div>

        {/* Controls — no-drag zone. When locked, hover here re-enables clicks
            so the buttons remain interactive. */}
        <div
          className="no-drag"
          onMouseEnter={enableInteraction}
          onMouseLeave={enableClickThrough}
          style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        >
          {/* filter toggle */}
          <button
            onClick={() => setShowAll((v) => !v)}
            style={{
              fontSize: 9,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.15)',
              backgroundColor: showAll ? '#16a34a' : 'transparent',
              color: showAll ? '#fff' : 'rgba(255,255,255,0.5)',
              cursor: 'pointer',
              fontWeight: 700,
              letterSpacing: '0.05em',
              textTransform: 'uppercase',
            }}
          >
            {showAll ? 'All' : 'Me'}
          </button>
          {/* lock */}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          {/* close */}
          <button
            onClick={() => window.electron?.overlay?.closeHPS()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* ── Status strip ────────────────────────────────────────────────── */}
      <div
        style={{
          padding: '3px 8px',
          fontSize: 10,
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          borderBottom: '1px solid rgba(255,255,255,0.06)',
          flexShrink: 0,
          color: combat?.in_combat ? '#86efac' : 'rgba(255,255,255,0.3)',
        }}
      >
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: combat?.in_combat ? '#22c55e' : 'rgba(255,255,255,0.2)',
            display: 'inline-block',
          }}
        />
        {combat?.in_combat && fight ? (
          <span>{fmtDur(fight.duration_seconds)} · {fmt(fight.total_heal)} healed</span>
        ) : (
          <span>Not in combat</span>
        )}
      </div>

      {/* ── Heal data ────────────────────────────────────────────────────── */}
      {combat === null ? (
        <p style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, color: 'rgba(255,255,255,0.3)', margin: 0 }}>
          Connecting…
        </p>
      ) : combat.in_combat && fight ? (
        <HealTable fight={fight} showAll={showAll} />
      ) : (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
          <HeartPulse size={24} style={{ opacity: 0.15 }} />
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>Not in combat</p>
        </div>
      )}

      {/* ── Session footer ────────────────────────────────────────────────── */}
      {combat && (
        <div
          style={{
            padding: '3px 8px',
            fontSize: 10,
            color: 'rgba(255,255,255,0.3)',
            borderTop: '1px solid rgba(255,255,255,0.06)',
            display: 'flex',
            gap: 8,
            flexShrink: 0,
          }}
        >
          <span>{(combat.recent_fights ?? []).length} fights</span>
          <span>{fmt(combat.session_heal)} healed</span>
          <span style={{ color: '#4ade80' }}>{fmtHPS(combat.session_hps)} HPS</span>
        </div>
      )}
    </div>
  )
}

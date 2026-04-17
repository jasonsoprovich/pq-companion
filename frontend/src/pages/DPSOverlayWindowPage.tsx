/**
 * DPSOverlayWindowPage — renders in the dedicated always-on-top Electron overlay
 * window. No sidebar, no title bar, transparent background.
 *
 * The window frame is removed; the user drags via the OS title-bar area
 * (CSS -webkit-app-region: drag on the header strip).
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Swords } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { getCombatState } from '../services/api'
import type { CombatState, EntityStats, FightState } from '../types/combat'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmt(n: number): string {
  return n.toLocaleString()
}

function fmtDPS(n: number): string {
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

function Row({ stat, totalDmg }: { stat: EntityStats; totalDmg: number }): React.ReactElement {
  const isYou = stat.name === 'You'
  const barPct = totalDmg > 0 ? (stat.total_damage / totalDmg) * 100 : 0

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
          backgroundColor: isYou ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)',
          pointerEvents: 'none',
        }}
      />
      <span
        style={{
          fontSize: 12,
          fontWeight: isYou ? 700 : 400,
          color: isYou ? '#818cf8' : 'rgba(255,255,255,0.85)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          position: 'relative',
        }}
      >
        {stat.name}
      </span>
      <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {pct(stat.total_damage, totalDmg)}
      </span>
      <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.7)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {fmt(stat.total_damage)}
      </span>
      <span style={{ fontSize: 11, color: '#fb923c', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
        {fmtDPS(stat.dps)}
      </span>
    </div>
  )
}

// ── Fight table ────────────────────────────────────────────────────────────────

function FightTable({ fight, showAll }: { fight: FightState; showAll: boolean }): React.ReactElement {
  const combatants = fight.combatants ?? []
  const rows = showAll ? combatants : combatants.filter((c) => c.name === 'You')
  const totalDmg = showAll ? fight.total_damage : fight.you_damage

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
        <span>Dmg</span>
        <span style={{ textAlign: 'right' }}>DPS</span>
      </div>

      {rows.length === 0 ? (
        <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>No data</p>
      ) : (
        rows.map((s) => <Row key={s.name} stat={s} totalDmg={totalDmg} />)
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DPSOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
  }, [])

  // Tick every second while in combat so the fight timer advances in real-time.
  useEffect(() => {
    if (!combat?.in_combat) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [combat?.in_combat])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:combat') {
      setCombat(msg.data as CombatState)
    }
  }, [])

  useWebSocket(handleMessage)

  const fight = combat?.current_fight
  const liveSecs = fight
    ? Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
    : 0
  const liveTotalDPS = fight && liveSecs > 0 ? fight.total_damage / liveSecs : 0
  const liveYouDPS = fight && liveSecs > 0 ? fight.you_damage / liveSecs : 0

  return (
    <div
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
        className="drag-region"
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
          <Swords size={11} style={{ color: '#818cf8' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>DPS</span>
          {fight && (
            <span style={{ fontSize: 10, color: '#fb923c', marginLeft: 4 }}>
              {fmtDPS(showAll ? liveTotalDPS : liveYouDPS)}
            </span>
          )}
        </div>

        {/* Controls — no-drag zone */}
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {/* filter toggle */}
          <button
            onClick={() => setShowAll((v) => !v)}
            style={{
              fontSize: 9,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.15)',
              backgroundColor: showAll ? '#4f46e5' : 'transparent',
              color: showAll ? '#fff' : 'rgba(255,255,255,0.5)',
              cursor: 'pointer',
              fontWeight: 700,
              letterSpacing: '0.05em',
              textTransform: 'uppercase',
            }}
          >
            {showAll ? 'All' : 'Me'}
          </button>
          {/* close */}
          <button
            onClick={() => window.electron?.overlay?.closeDPS()}
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
          color: combat?.in_combat ? '#f87171' : 'rgba(255,255,255,0.3)',
        }}
      >
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: combat?.in_combat ? '#ef4444' : 'rgba(255,255,255,0.2)',
            display: 'inline-block',
          }}
        />
        {combat?.in_combat && fight ? (
          <span>{fmtDur(liveSecs)} · {fmt(fight.total_damage)} dmg</span>
        ) : (
          <span>Not in combat</span>
        )}
      </div>

      {/* ── Fight data ───────────────────────────────────────────────────── */}
      {combat === null ? (
        <p style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, color: 'rgba(255,255,255,0.3)', margin: 0 }}>
          Connecting…
        </p>
      ) : combat.in_combat && fight ? (
        <FightTable fight={fight} showAll={showAll} />
      ) : (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
          <Swords size={24} style={{ opacity: 0.15 }} />
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
          <span>{fmt(combat.session_damage)} dmg</span>
          <span style={{ color: '#fb923c' }}>{fmtDPS(combat.session_dps)} DPS</span>
        </div>
      )}
    </div>
  )
}

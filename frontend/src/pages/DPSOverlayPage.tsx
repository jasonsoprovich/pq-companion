import React, { useCallback, useEffect, useState } from 'react'
import { Swords, HeartPulse, Circle, CheckCircle2, AlertTriangle, ExternalLink } from 'lucide-react'
import { DEV_HPS } from '../lib/devFlags'
import { useWebSocket } from '../hooks/useWebSocket'
import { getCombatState, getLogStatus } from '../services/api'
import OverlayWindow from '../components/OverlayWindow'
import type { CombatState, EntityStats, HealerStats, FightState } from '../types/combat'
import type { LogTailerStatus } from '../types/logEvent'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmt(n: number): string {
  return n.toLocaleString()
}

function fmtRate(n: number): string {
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

// ── Connection pill ────────────────────────────────────────────────────────────

function ConnPill({
  state,
  status,
}: {
  state: string
  status: LogTailerStatus | null
}): React.ReactElement {
  let color: string
  let label: string
  if (state !== 'open') {
    color = state === 'connecting' ? '#f97316' : '#6b7280'
    label = state === 'connecting' ? 'Connecting' : 'Off'
  } else if (!status || !status.enabled || !status.file_exists) {
    color = '#f97316'
    label = 'No Log'
  } else {
    color = '#22c55e'
    label = 'Live'
  }
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: color, display: 'inline-block' }} />
      {label}
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

// ── Filter toggle ──────────────────────────────────────────────────────────────

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

function CombatStrip({
  combat,
  kind,
  now,
}: {
  combat: CombatState
  kind: 'dps' | 'hps'
  now: number
}): React.ReactElement {
  const fight = combat.current_fight

  // Compute live duration from wall-clock so the timer ticks every second even
  // between log events, then clamp to at least the backend-reported value.
  const liveSecs = fight
    ? Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
    : 0
  const liveTotalDPS = fight && liveSecs > 0 ? fight.total_damage / liveSecs : 0
  const liveTotalHPS = fight && liveSecs > 0 ? fight.total_heal / liveSecs : 0

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
          <span>{fmtDuration(liveSecs)}</span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          {kind === 'dps' ? (
            <span style={{ color: '#f97316' }}>{fmtRate(liveTotalDPS)} DPS</span>
          ) : (
            <span style={{ color: '#22c55e' }}>{fmtRate(liveTotalHPS)} HPS</span>
          )}
        </>
      ) : (
        <span>Not in combat</span>
      )}
    </div>
  )
}

// ── Column header row ──────────────────────────────────────────────────────────

function ColHeaders({ rateLabel }: { rateLabel: string }): React.ReactElement {
  return (
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
      <span>{rateLabel === 'DPS' ? 'Dmg' : 'Heal'}</span>
      <span style={{ textAlign: 'right' }}>{rateLabel}</span>
    </div>
  )
}

// ── DPS row ────────────────────────────────────────────────────────────────────

function DPSRow({
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
      <span style={{ fontSize: 11, color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {pct(stat.total_damage, totalDamage)}
      </span>
      <span style={{ fontSize: 11, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {fmt(stat.total_damage)}
      </span>
      <span style={{ fontSize: 11, color: '#f97316', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
        {fmtRate(stat.dps)}
      </span>
    </div>
  )
}

// ── HPS row ────────────────────────────────────────────────────────────────────

function HPSRow({
  stat,
  totalHeal,
  isYou,
}: {
  stat: HealerStats
  totalHeal: number
  isYou: boolean
}): React.ReactElement {
  const barPct = totalHeal > 0 ? (stat.total_heal / totalHeal) * 100 : 0

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
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${barPct}%`,
          backgroundColor: isYou ? 'rgba(34,197,94,0.15)' : 'rgba(255,255,255,0.05)',
          pointerEvents: 'none',
        }}
      />
      <span
        style={{
          fontSize: 12,
          fontWeight: isYou ? 600 : 400,
          color: isYou ? '#4ade80' : 'var(--color-foreground)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          position: 'relative',
        }}
      >
        {stat.name}
      </span>
      <span style={{ fontSize: 11, color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {pct(stat.total_heal, totalHeal)}
      </span>
      <span style={{ fontSize: 11, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
        {fmt(stat.total_heal)}
      </span>
      <span style={{ fontSize: 11, color: '#22c55e', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
        {fmtRate(stat.hps)}
      </span>
    </div>
  )
}

// ── DPS fight panel ────────────────────────────────────────────────────────────

function DPSPanel({ fight, showAll }: { fight: FightState; showAll: boolean }): React.ReactElement {
  const combatants = fight.combatants ?? []
  const rows = showAll ? combatants : combatants.filter((c) => c.name === 'You')
  const totalDmg = showAll ? fight.total_damage : fight.you_damage

  return (
    <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      <ColHeaders rateLabel="DPS" />
      {rows.length === 0 ? (
        <div style={{ padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' }}>
          No damage data
        </div>
      ) : (
        rows.map((s) => (
          <DPSRow key={s.name} stat={s} totalDamage={totalDmg} isYou={s.name === 'You'} />
        ))
      )}
    </div>
  )
}

// ── HPS fight panel ────────────────────────────────────────────────────────────

function HPSPanel({ fight, showAll }: { fight: FightState; showAll: boolean }): React.ReactElement {
  const healers = fight.healers ?? []
  const rows = showAll ? healers : healers.filter((h) => h.name === 'You')
  const totalHeal = showAll ? fight.total_heal : fight.you_heal

  return (
    <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      <ColHeaders rateLabel="HPS" />
      {rows.length === 0 ? (
        <div style={{ padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' }}>
          No healing data
        </div>
      ) : (
        rows.map((h) => (
          <HPSRow key={h.name} stat={h} totalHeal={totalHeal} isYou={h.name === 'You'} />
        ))
      )}
    </div>
  )
}

// ── Not-in-combat placeholder ──────────────────────────────────────────────────

function NotInCombat({ kind }: { kind: 'dps' | 'hps' }): React.ReactElement {
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
      {kind === 'dps'
        ? <Swords size={32} style={{ opacity: 0.3 }} />
        : <HeartPulse size={32} style={{ opacity: 0.3 }} />}
      <p style={{ fontSize: 12, margin: 0 }}>Not in combat</p>
    </div>
  )
}

// ── Session bar ────────────────────────────────────────────────────────────────

function SessionBar({ combat, kind }: { combat: CombatState; kind: 'dps' | 'hps' }): React.ReactElement {
  const fights = (combat.recent_fights ?? []).length
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
      {kind === 'dps' ? (
        <>
          <span style={{ color: 'var(--color-foreground)' }}>{fmt(combat.session_damage)} dmg</span>
          <span style={{ color: '#f97316' }}>{fmtRate(combat.session_dps)} DPS (session)</span>
        </>
      ) : (
        <>
          <span style={{ color: 'var(--color-foreground)' }}>{fmt(combat.session_heal)} healed</span>
          <span style={{ color: '#22c55e' }}>{fmtRate(combat.session_hps)} HPS (session)</span>
        </>
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DPSOverlayPage(): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [showAllDPS, setShowAllDPS] = useState(true)
  const [showAllHPS, setShowAllHPS] = useState(true) // only used when DEV_HPS
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
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

  const wsState = useWebSocket(handleMessage)

  return (
    <div
      style={{
        position: 'relative',
        height: '100%',
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
          Drag title bars to move · Drag edges/corners to resize
        </p>
      </div>

      {/* ── DPS panel ─────────────────────────────────────────────────── */}
      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Swords size={13} style={{ color: 'var(--color-primary)' }} />
            DPS Meter
          </span>
        }
        headerRight={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <FilterButton showAll={showAllDPS} onToggle={() => setShowAllDPS((v) => !v)} />
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleDPS()}
                title="Pop out DPS as floating overlay"
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
            <ConnPill state={wsState} status={status} />
          </div>
        }
        defaultWidth={380}
        defaultHeight={420}
        defaultX={24}
        defaultY={24}
        minWidth={260}
        minHeight={180}
      >
        <StatusBar status={status} />

        {combat === null ? (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <p style={{ fontSize: 12, color: 'var(--color-muted)' }}>Loading…</p>
          </div>
        ) : (
          <>
            <CombatStrip combat={combat} kind="dps" now={now} />
            {combat.in_combat && combat.current_fight ? (
              <DPSPanel fight={combat.current_fight} showAll={showAllDPS} />
            ) : (
              <NotInCombat kind="dps" />
            )}
            <SessionBar combat={combat} kind="dps" />
          </>
        )}
      </OverlayWindow>

      {/* ── HPS panel — hidden until EQ logs expose healing events ─────── */}
      {DEV_HPS && (
        <OverlayWindow
          title={
            <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
              <HeartPulse size={13} style={{ color: '#22c55e' }} />
              HPS Meter
            </span>
          }
          headerRight={
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <FilterButton showAll={showAllHPS} onToggle={() => setShowAllHPS((v) => !v)} />
              {window.electron?.overlay && (
                <button
                  onClick={() => window.electron.overlay.toggleHPS()}
                  title="Pop out HPS as floating overlay"
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
              <ConnPill state={wsState} status={status} />
            </div>
          }
          defaultWidth={380}
          defaultHeight={420}
          defaultX={420}
          defaultY={24}
          minWidth={260}
          minHeight={180}
        >
          <StatusBar status={status} />

          {combat === null ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <p style={{ fontSize: 12, color: 'var(--color-muted)' }}>Loading…</p>
            </div>
          ) : (
            <>
              <CombatStrip combat={combat} kind="hps" now={now} />
              {combat.in_combat && combat.current_fight ? (
                <HPSPanel fight={combat.current_fight} showAll={showAllHPS} />
              ) : (
                <NotInCombat kind="hps" />
              )}
              <SessionBar combat={combat} kind="hps" />
            </>
          )}
        </OverlayWindow>
      )}
    </div>
  )
}

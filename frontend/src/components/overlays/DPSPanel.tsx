import React, { useCallback, useEffect, useState } from 'react'
import { Swords, Circle, CheckCircle2, AlertTriangle, ExternalLink, Clipboard, ClipboardCheck } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { getCombatState, getLogStatus } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { CombatState, EntityStats, FightState } from '../../types/combat'
import type { LogTailerStatus } from '../../types/logEvent'

interface DPSPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
}

function fmt(n: number): string { return n.toLocaleString() }
function fmtRate(n: number): string { return n.toFixed(1) }
function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}
function fmtDuration(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = Math.floor(secs % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}
function truncateName(name: string, max = 28): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

function buildFightText(fight: FightState): string {
  const target = fight.primary_target ?? 'Unknown'
  const dur = fmtDuration(fight.duration_seconds)
  const lines: string[] = [`[PQ Companion] Fight: ${target} (${dur})`]
  for (const c of fight.combatants) {
    lines.push(`${c.name}: ${fmtRate(c.dps)} DPS (${fmt(c.total_damage)} total)`)
  }
  return lines.join('\n')
}

function CopyFightButton({ fight }: { fight: FightState | null }): React.ReactElement {
  const [copied, setCopied] = useState(false)

  function handleCopy(): void {
    if (!fight) return
    navigator.clipboard.writeText(buildFightText(fight)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }).catch(() => {})
  }

  return (
    <button
      onClick={handleCopy}
      disabled={!fight}
      title="Copy DPS summary to clipboard"
      style={{
        background: 'none', border: 'none',
        cursor: fight ? 'pointer' : 'default',
        padding: '1px 3px',
        color: copied ? '#22c55e' : 'var(--color-muted)',
        display: 'flex', alignItems: 'center', opacity: fight ? 1 : 0.4,
      }}
    >
      {copied ? <ClipboardCheck size={12} /> : <Clipboard size={12} />}
    </button>
  )
}

function ConnPill({ state, status }: { state: string; status: LogTailerStatus | null }): React.ReactElement {
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
    display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', fontSize: 11,
    borderBottom: '1px solid var(--color-border)', flexShrink: 0,
    backgroundColor: 'var(--color-surface-2)',
  }
  if (!status) return <div style={{ ...style, color: 'var(--color-muted)' }}><Circle size={10} /> Loading…</div>
  if (!status.enabled) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log parsing disabled — enable in Settings</div>
  if (!status.file_exists) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log file not found</div>
  return <div style={{ ...style, color: '#22c55e' }}><CheckCircle2 size={11} /> Tailing log</div>
}

function FilterButton({ showAll, onToggle }: { showAll: boolean; onToggle: () => void }): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      style={{
        fontSize: 10, padding: '2px 7px', borderRadius: 4,
        border: '1px solid var(--color-border)',
        backgroundColor: showAll ? 'var(--color-primary)' : 'var(--color-surface)',
        color: showAll ? '#fff' : 'var(--color-muted-foreground)',
        cursor: 'pointer', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      }}
    >
      {showAll ? 'All' : 'Me'}
    </button>
  )
}

function CombatStrip({ combat, now }: { combat: CombatState; now: number }): React.ReactElement {
  const fight = combat.current_fight
  const liveSecs = fight
    ? Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
    : 0
  const liveTotalDPS = fight && liveSecs > 0 ? fight.total_damage / liveSecs : 0

  return (
    <div
      style={{
        padding: '4px 10px', fontSize: 11, display: 'flex', alignItems: 'center', gap: 8,
        borderBottom: '1px solid var(--color-border)', flexShrink: 0,
        backgroundColor: combat.in_combat ? 'rgba(220,38,38,0.15)' : 'transparent',
        color: combat.in_combat ? '#f87171' : 'var(--color-muted)',
      }}
    >
      <span style={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: combat.in_combat ? '#ef4444' : '#6b7280', display: 'inline-block', flexShrink: 0 }} />
      {combat.in_combat && fight ? (
        <>
          <span style={{ fontWeight: 600, color: '#f87171', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }}>
            {fight.primary_target ? truncateName(fight.primary_target) : 'In Combat'}
          </span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span>{fmtDuration(liveSecs)}</span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span style={{ color: '#f97316' }}>{fmtRate(liveTotalDPS)} DPS</span>
        </>
      ) : (
        <span>Not in combat</span>
      )}
    </div>
  )
}

function ColHeaders(): React.ReactElement {
  return (
    <div
      style={{
        display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: '0 10px',
        padding: '4px 10px', fontSize: 10, fontWeight: 600,
        textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-muted)',
        borderBottom: '1px solid var(--color-border)', flexShrink: 0,
      }}
    >
      <span>Name</span>
      <span>%</span>
      <span>Dmg</span>
      <span style={{ textAlign: 'right' }}>DPS</span>
    </div>
  )
}

function DPSRow({ stat, totalDamage, isYou }: { stat: EntityStats; totalDamage: number; isYou: boolean }): React.ReactElement {
  const barPct = totalDamage > 0 ? (stat.total_damage / totalDamage) * 100 : 0
  return (
    <div
      style={{
        position: 'relative', padding: '5px 10px',
        display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: '0 10px',
        alignItems: 'center', borderBottom: '1px solid var(--color-border)', overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${barPct}%`,
          backgroundColor: isYou ? 'rgba(99,102,241,0.18)' : 'rgba(255,255,255,0.05)',
          pointerEvents: 'none',
        }}
      />
      <span style={{ fontSize: 12, fontWeight: isYou ? 600 : 400, color: isYou ? 'var(--color-primary)' : 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', position: 'relative' }}>{stat.name}</span>
      <span style={{ fontSize: 11, color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{pct(stat.total_damage, totalDamage)}</span>
      <span style={{ fontSize: 11, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{fmt(stat.total_damage)}</span>
      <span style={{ fontSize: 11, color: '#f97316', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>{fmtRate(stat.dps)}</span>
    </div>
  )
}

function DPSContent({ fight, showAll }: { fight: FightState; showAll: boolean }): React.ReactElement {
  const combatants = fight.combatants ?? []
  const rows = showAll ? combatants : combatants.filter((c) => c.name === 'You')
  const totalDmg = showAll ? fight.total_damage : fight.you_damage
  return (
    <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      <ColHeaders />
      {rows.length === 0 ? (
        <div style={{ padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' }}>No damage data</div>
      ) : (
        rows.map((s) => <DPSRow key={s.name} stat={s} totalDamage={totalDmg} isYou={s.name === 'You'} />)
      )}
    </div>
  )
}

function NotInCombat(): React.ReactElement {
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)' }}>
      <Swords size={32} style={{ opacity: 0.3 }} />
      <p style={{ fontSize: 12, margin: 0 }}>Not in combat</p>
    </div>
  )
}

function SessionBar({ combat }: { combat: CombatState }): React.ReactElement {
  const fights = (combat.recent_fights ?? []).length
  return (
    <div
      style={{
        padding: '5px 10px', fontSize: 11, color: 'var(--color-muted)',
        borderTop: '1px solid var(--color-border)',
        display: 'flex', gap: 12, flexShrink: 0,
        backgroundColor: 'var(--color-surface-2)', flexWrap: 'wrap',
      }}
    >
      <span>{fights} fight{fights !== 1 ? 's' : ''}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{fmt(combat.session_damage)} dmg</span>
      <span style={{ color: '#f97316' }}>{fmtRate(combat.session_dps)} DPS (session)</span>
    </div>
  )
}

export default function DPSPanel({
  defaultX = 24,
  defaultY = 420,
  defaultWidth = 380,
  defaultHeight = 420,
}: DPSPanelProps): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  useEffect(() => {
    if (!combat?.in_combat) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [combat?.in_combat])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:combat') setCombat(msg.data as CombatState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  return (
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
          <CopyFightButton fight={combat?.current_fight ?? null} />
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleDPS()}
              title="Pop out DPS as floating overlay"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <ExternalLink size={12} />
            </button>
          )}
          <ConnPill state={wsState} status={status} />
        </div>
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
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
          <CombatStrip combat={combat} now={now} />
          {combat.in_combat && combat.current_fight ? (
            <DPSContent fight={combat.current_fight} showAll={showAll} />
          ) : (
            <NotInCombat />
          )}
          <SessionBar combat={combat} />
        </>
      )}
    </OverlayWindow>
  )
}

import React, { useCallback, useEffect, useState } from 'react'
import { Swords, Circle, CheckCircle2, AlertTriangle, ExternalLink, Clipboard, ClipboardCheck, Users, Trash2, Activity, Hourglass, User } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { getCombatState, getLogStatus, resetCombatState } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { CombatState, FightState } from '../../types/combat'
import type { LogTailerStatus } from '../../types/logEvent'
import { rollupCombatants, useCombinePetWithOwner, petBadge, type RolledUpEntity } from '../../lib/dpsRollup'
import { combatantBarColor } from '../../lib/combatantColor'
import { useDPSClassColors } from '../../hooks/useDPSClassColors'
import type { DPSClassColors } from '../../types/config'
import { useDPSMode, dpsForMode, dpsModeAbbrev, dpsModeLabel, fightAggregateDPS, type DPSMode } from '../../hooks/useDPSMode'

// dpsModeIcon picks an icon for the current DPS mode that matches the
// metric's intuition: a single User for Personal, Users for Raid, an
// Hourglass for Encounter (wall-clock).
function dpsModeIcon(mode: DPSMode, size = 12): React.ReactElement {
  switch (mode) {
    case 'personal':
      return <User size={size} />
    case 'raid':
      return <Activity size={size} />
    case 'encounter':
      return <Hourglass size={size} />
  }
}

// dpsModeTooltip explains the current metric and that clicking cycles.
function dpsModeTooltip(mode: DPSMode): string {
  const meaning: Record<DPSMode, string> = {
    personal: 'Personal DPS — total damage / your first-to-last span. Fair to the individual; matches EQLogParser.',
    raid: 'Raid-relative DPS — total damage / the raid\'s active span. The right metric for ranking players within one fight.',
    encounter: 'Encounter DPS — total damage / fight wall-clock. Compare whole fights to each other.',
  }
  return `${meaning[mode]} Click to cycle (Personal → Raid → Encounter).`
}

interface DPSPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
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

function buildFightText(fight: FightState, combine: boolean, mode: DPSMode): string {
  const target = fight.primary_target ?? 'Unknown'
  const dur = fmtDuration(fight.duration_seconds)
  const label = dpsModeAbbrev(mode)
  const lines: string[] = [`[PQ Companion] Fight: ${target} (${dur}) — ${dpsModeLabel(mode)} DPS`]
  const rows = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  for (const c of rows) {
    const crit = c.crit_count > 0
      ? `, ${c.crit_count} crit${c.crit_count !== 1 ? 's' : ''} for ${fmt(c.crit_damage)}`
      : ''
    lines.push(`${c.name}${petBadge(c.pets)}: ${fmtRate(dpsForMode(c, mode))} ${label} (${fmt(c.total_damage)} total${crit})`)
  }
  return lines.join('\n')
}

function rowTooltip(stat: RolledUpEntity): string {
  const parts = [`${stat.hit_count} hit${stat.hit_count !== 1 ? 's' : ''}`, `max ${fmt(stat.max_hit)}`]
  if (stat.crit_count > 0) {
    parts.push(`${stat.crit_count} crit${stat.crit_count !== 1 ? 's' : ''} (${fmt(stat.crit_damage)} dmg)`)
  }
  return parts.join(' · ')
}

function CopyFightButton({ fight, combine, mode }: { fight: FightState | null; combine: boolean; mode: DPSMode }): React.ReactElement {
  const [copied, setCopied] = useState(false)

  function handleCopy(): void {
    if (!fight) return
    navigator.clipboard.writeText(buildFightText(fight, combine, mode)).then(() => {
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

function CombatStrip({ combat, now, mode }: { combat: CombatState; now: number; mode: DPSMode }): React.ReactElement {
  const fight = combat.current_fight
  const liveSecs = fight
    ? Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
    : 0
  // For the live strip, Encounter uses live wall-clock; Raid/Personal use
  // the same fight-aggregate denominator (raid_seconds from combatants).
  // Personal at the strip level collapses to Raid (a multi-player total
  // has no per-player "personal" interpretation).
  let liveTotalDPS = 0
  if (fight) {
    if (mode === 'encounter') {
      liveTotalDPS = liveSecs > 0 ? fight.total_damage / liveSecs : 0
    } else {
      liveTotalDPS = fightAggregateDPS(fight.total_damage, fight.duration_seconds, fight.combatants ?? [], mode)
    }
  }

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
          <span style={{ color: '#f97316' }}>{fmtRate(liveTotalDPS)} {dpsModeAbbrev(mode)}</span>
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

function DPSRow({ stat, totalDamage, isYou, expanded, onToggle, mode, palette }: { stat: RolledUpEntity; totalDamage: number; isYou: boolean; expanded: boolean; onToggle: () => void; mode: DPSMode; palette: DPSClassColors }): React.ReactElement {
  const barPct = totalDamage > 0 ? (stat.total_damage / totalDamage) * 100 : 0
  const hasPets = stat.pets.length > 0
  // flexShrink: 0 keeps each row at its natural height when the parent flex
  // column overflows — without it the column flex algorithm squeezes rows to
  // fit, making text and bars look tiny in raid-sized fights.
  return (
    <div style={{ flexShrink: 0 }}>
      <div
        onClick={hasPets ? onToggle : undefined}
        title={rowTooltip(stat)}
        style={{
          position: 'relative', padding: '5px 10px',
          display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: '0 10px',
          alignItems: 'center', borderBottom: '1px solid var(--color-border)', overflow: 'hidden',
          cursor: hasPets ? 'pointer' : 'default',
        }}
      >
        <div
          style={{
            position: 'absolute', left: 0, top: 0, bottom: 0,
            width: `${barPct}%`,
            backgroundColor: combatantBarColor(stat.class, palette),
            pointerEvents: 'none',
          }}
        />
        <span style={{ fontSize: 12, fontWeight: isYou ? 600 : 400, color: isYou ? 'var(--color-primary)' : 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', position: 'relative' }}>
          {stat.name}
          {hasPets && <span style={{ color: 'var(--color-muted)', fontWeight: 400 }}>{petBadge(stat.pets)}</span>}
        </span>
        <span style={{ fontSize: 11, color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{pct(stat.total_damage, totalDamage)}</span>
        <span style={{ fontSize: 11, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{fmt(stat.total_damage)}</span>
        <span style={{ fontSize: 11, color: '#f97316', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>{fmtRate(dpsForMode(stat, mode))}</span>
      </div>
      {hasPets && expanded && (
        <div style={{ borderBottom: '1px solid var(--color-border)' }}>
          {stat.pets.map((p) => (
            <div
              key={p.name}
              style={{
                padding: '3px 10px 3px 22px',
                display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: '0 10px',
                alignItems: 'center', fontSize: 11,
                color: 'var(--color-muted-foreground)',
              }}
            >
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>↳ {p.name}</span>
              <span style={{ color: 'var(--color-muted)', fontVariantNumeric: 'tabular-nums' }}>{pct(p.total_damage, totalDamage)}</span>
              <span style={{ fontVariantNumeric: 'tabular-nums' }}>{fmt(p.total_damage)}</span>
              <span style={{ color: '#f97316', fontVariantNumeric: 'tabular-nums', minWidth: 44, textAlign: 'right' }}>{fmtRate(dpsForMode(p, mode))}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function DPSContent({ fight, showAll, combine, mode, palette }: { fight: FightState; showAll: boolean; combine: boolean; mode: DPSMode; palette: DPSClassColors }): React.ReactElement {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const rolled = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  const rows = showAll ? rolled : rolled.filter((c) => c.name === 'You')
  const totalDmg = showAll ? fight.total_damage : fight.you_damage
  // min-height: 0 lets this flex child honor its allocated height instead of
  // growing to fit all rows (default min-height: auto on flex items), so
  // overflow: auto actually scrolls when there are many combatants.
  return (
    <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      <ColHeaders />
      {rows.length === 0 ? (
        <div style={{ padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' }}>No damage data</div>
      ) : (
        rows.map((s) => (
          <DPSRow
            key={s.name}
            stat={s}
            totalDamage={totalDmg}
            isYou={s.name === 'You'}
            mode={mode}
            palette={palette}
            expanded={expanded.has(s.name)}
            onToggle={() => setExpanded((prev) => {
              const next = new Set(prev)
              if (next.has(s.name)) next.delete(s.name)
              else next.add(s.name)
              return next
            })}
          />
        ))
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
  snapGridSize,
  onLayoutChange,
}: DPSPanelProps): React.ReactElement {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [combine, setCombine] = useCombinePetWithOwner()
  const { mode: dpsMode, toggle: toggleDPSMode } = useDPSMode()
  const [now, setNow] = useState(() => Date.now())
  const palette = useDPSClassColors()

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
    if (msg.type === WSEvent.OverlayCombat) setCombat(msg.data as CombatState)
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
          <button
            onClick={() => setCombine(!combine)}
            title={combine ? 'Pet damage rolled up under owner — click to split' : 'Pets shown as separate rows — click to combine'}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '1px 3px', display: 'flex', alignItems: 'center',
              color: combine ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            <Users size={12} />
          </button>
          <button
            onClick={toggleDPSMode}
            title={dpsModeTooltip(dpsMode)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '1px 4px', display: 'flex', alignItems: 'center', gap: 3,
              color: 'var(--color-primary)',
              fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em', fontWeight: 600,
            }}
          >
            {dpsModeIcon(dpsMode)}
            {dpsModeLabel(dpsMode)}
          </button>
          <CopyFightButton fight={combat?.current_fight ?? null} combine={combine} mode={dpsMode} />
          <button
            onClick={() => resetCombatState().catch(() => {})}
            title="Clear DPS — reset session damage, fight history, and the meter"
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '1px 3px', color: 'var(--color-muted)',
              display: 'flex', alignItems: 'center',
            }}
          >
            <Trash2 size={12} />
          </button>
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
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <StatusBar status={status} />

      {combat === null ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <p style={{ fontSize: 12, color: 'var(--color-muted)' }}>Loading…</p>
        </div>
      ) : (
        <>
          <CombatStrip combat={combat} now={now} mode={dpsMode} />
          {combat.in_combat && combat.current_fight ? (
            <DPSContent fight={combat.current_fight} showAll={showAll} combine={combine} mode={dpsMode} palette={palette} />
          ) : (
            <NotInCombat />
          )}
          <SessionBar combat={combat} />
        </>
      )}
    </OverlayWindow>
  )
}

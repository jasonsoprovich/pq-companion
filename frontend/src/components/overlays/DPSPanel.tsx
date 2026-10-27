import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Swords, Circle, ChartBar, ChartBarDecreasing, CheckCircle2, AlertTriangle, ExternalLink, Clipboard, ClipboardCheck, Users, Trash2, Activity, Hourglass, User, History, Crosshair, XCircle } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { getCombatState, getLogStatus, resetCombatState, endActiveFight } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { CombatState, FightState } from '../../types/combat'
import type { LogTailerStatus } from '../../types/logEvent'
import { useCombinePetWithOwner } from '../../lib/dpsRollup'
import { buildDpsFightSummary } from '../../lib/dpsClipboard'
import { useDPSClassColors } from '../../hooks/useDPSClassColors'
import type { DPSClassColors } from '../../types/config'
import { useDPSMode, dpsModeAbbrev, dpsModeLabel, fightAggregateDPS, type DPSMode } from '../../hooks/useDPSMode'
import { useDPSScope, dpsScopeLabel, type DPSScope } from '../../hooks/useDPSScope'
import { aggregateRecentFights } from '../../lib/rollingWindow'
import { useDPSBarScale, dpsBarScaleTooltip, type DPSBarScale } from '../../hooks/useDPSBarScale'
import DPSTable, { PANEL_THEME } from './DPSTable'

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
function fmtDuration(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = Math.floor(secs % 60)
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}
function truncateName(name: string, max = 28): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

function CopyFightButton({ fight, combine, mode }: { fight: FightState | null; combine: boolean; mode: DPSMode }): React.ReactElement {
  const [copied, setCopied] = useState(false)

  function handleCopy(): void {
    if (!fight) return
    const text = buildDpsFightSummary(fight.primary_target, fight, combine, mode)
    if (!text) return
    navigator.clipboard.writeText(text).then(() => {
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

// WindowStrip is the "Last N mobs" analogue of CombatStrip: a static summary
// of the pooled window rather than a live in-combat readout.
function WindowStrip({ fight, mode }: { fight: FightState | null; mode: DPSMode }): React.ReactElement {
  // Encounter pools over total wall-clock (fight.total_dps already is that);
  // Raid/Personal use the shared fight-aggregate denominator.
  const totalDPS = fight
    ? mode === 'encounter'
      ? fight.total_dps
      : fightAggregateDPS(fight.total_damage, fight.duration_seconds, fight.combatants ?? [], mode)
    : 0
  return (
    <div
      style={{
        padding: '4px 10px', fontSize: 11, display: 'flex', alignItems: 'center', gap: 8,
        borderBottom: '1px solid var(--color-border)', flexShrink: 0,
        backgroundColor: 'rgba(129,140,248,0.12)', color: '#a5b4fc',
      }}
    >
      <History size={12} style={{ flexShrink: 0 }} />
      {fight ? (
        <>
          <span style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 }}>
            {fight.primary_target}
          </span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span>{fmtDuration(fight.duration_seconds)} total</span>
          <span style={{ color: 'var(--color-muted)' }}>·</span>
          <span style={{ color: '#f97316' }}>{fmtRate(totalDPS)} {dpsModeAbbrev(mode)}</span>
        </>
      ) : (
        <span>No completed fights yet</span>
      )}
    </div>
  )
}

function DPSContent({ fight, showAll, combine, mode, palette, barScale }: { fight: FightState; showAll: boolean; combine: boolean; mode: DPSMode; palette: DPSClassColors; barScale: DPSBarScale }): React.ReactElement {
  return (
    <DPSTable
      fight={fight}
      showAll={showAll}
      combine={combine}
      mode={mode}
      palette={palette}
      barScale={barScale}
      theme={PANEL_THEME}
    />
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

function NoWindowData(): React.ReactElement {
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)' }}>
      <History size={32} style={{ opacity: 0.3 }} />
      <p style={{ fontSize: 12, margin: 0 }}>No completed fights yet</p>
      <p style={{ fontSize: 11, margin: 0, opacity: 0.7 }}>Kill a few mobs to build the average</p>
    </div>
  )
}

function ScopeButton({ scope, onToggle }: { scope: DPSScope; onToggle: () => void }): React.ReactElement {
  const isWindow = scope === 'window'
  return (
    <button
      onClick={onToggle}
      title={
        isWindow
          ? 'Showing a pooled average over the last 20 completed fights — click for the current fight only'
          : 'Showing the current fight — click for a pooled average over the last 20 mobs'
      }
      style={{
        background: 'none', border: 'none', cursor: 'pointer',
        padding: '1px 4px', display: 'flex', alignItems: 'center', gap: 3,
        color: isWindow ? '#a5b4fc' : 'var(--color-muted)',
        fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em', fontWeight: 600,
      }}
    >
      {isWindow ? <History size={12} /> : <Crosshair size={12} />}
      {dpsScopeLabel(scope)}
    </button>
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
  const { scope, toggle: toggleScope } = useDPSScope()
  const { barScale, toggle: toggleBarScale } = useDPSBarScale()
  const [now, setNow] = useState(() => Date.now())
  const palette = useDPSClassColors()

  // Pooled "last N mobs" fight, recomputed when the recent-fights list moves.
  // A synthetic FightState so DPSContent renders it with the same row/rollup/
  // mode code as a real fight.
  const windowFight = useMemo(
    () => aggregateRecentFights(combat?.recent_fights ?? []),
    [combat?.recent_fights],
  )

  // Once a WS broadcast has applied, ignore the initial REST snapshot if it
  // resolves later — otherwise the stale fetch can clobber a fresher live
  // update (mount-order race).
  const wsAppliedRef = useRef(false)
  useEffect(() => {
    getCombatState().then((s) => { if (!wsAppliedRef.current) setCombat(s) }).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  useEffect(() => {
    if (!combat?.in_combat) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [combat?.in_combat])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayCombat) {
      wsAppliedRef.current = true
      setCombat(msg.data as CombatState)
    }
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
            onClick={toggleBarScale}
            title={dpsBarScaleTooltip(barScale)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '1px 3px', display: 'flex', alignItems: 'center',
              color: barScale === 'leader' ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            {barScale === 'leader' ? <ChartBarDecreasing size={12} /> : <ChartBar size={12} />}
          </button>
          <ScopeButton scope={scope} onToggle={toggleScope} />
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
          <CopyFightButton fight={scope === 'window' ? windowFight : combat?.current_fight ?? null} combine={combine} mode={dpsMode} />
          <button
            onClick={() => { if (combat?.in_combat) endActiveFight().catch(() => {}) }}
            disabled={!combat?.in_combat}
            title={
              combat?.in_combat
                ? 'End the current parse — clears this fight from the meter and saves it to Combat History (use after evac/zone when you’re done with the mob)'
                : 'No active fight to end'
            }
            style={{
              background: 'none', border: 'none',
              cursor: combat?.in_combat ? 'pointer' : 'default',
              padding: '1px 3px', color: 'var(--color-muted)',
              display: 'flex', alignItems: 'center',
              opacity: combat?.in_combat ? 1 : 0.4,
            }}
          >
            <XCircle size={12} />
          </button>
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
          {scope === 'window' ? (
            <>
              <WindowStrip fight={windowFight} mode={dpsMode} />
              {windowFight ? (
                <DPSContent fight={windowFight} showAll={showAll} combine={combine} mode={dpsMode} palette={palette} barScale={barScale} />
              ) : (
                <NoWindowData />
              )}
            </>
          ) : (
            <>
              <CombatStrip combat={combat} now={now} mode={dpsMode} />
              {combat.in_combat && combat.current_fight ? (
                <DPSContent fight={combat.current_fight} showAll={showAll} combine={combine} mode={dpsMode} palette={palette} barScale={barScale} />
              ) : (
                <NotInCombat />
              )}
            </>
          )}
          <SessionBar combat={combat} />
        </>
      )}
    </OverlayWindow>
  )
}

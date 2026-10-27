/**
 * DPSOverlayWindowPage — renders in the dedicated always-on-top Electron overlay
 * window. No sidebar, no title bar, transparent background.
 *
 * Unlocked: the window is movable (OS drag region on the header) and resizable.
 * Locked: the entire window passes clicks through to the game except the
 * header strip — hovering the header temporarily disables passthrough so its
 * buttons (clear, lock, close, etc.) stay clickable.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Swords, ChartBar, ChartBarDecreasing, Clipboard, ClipboardCheck, Trash2, Users, Activity, Hourglass, User, History, Crosshair, XCircle } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { getCombatState, resetCombatState, endActiveFight } from '../services/api'
import type { CombatState, FightState } from '../types/combat'
import { useCombinePetWithOwner } from '../lib/dpsRollup'
import { buildDpsFightSummary } from '../lib/dpsClipboard'
import { useDPSClassColors } from '../hooks/useDPSClassColors'
import { useDPSMode, dpsModeAbbrev, dpsModeLabel, fightAggregateDPS, playerAggregateDPS, type DPSMode } from '../hooks/useDPSMode'
import { useDPSScope, dpsScopeLabel } from '../hooks/useDPSScope'
import { aggregateRecentFights } from '../lib/rollingWindow'
import { useDPSBarScale, dpsBarScaleTooltip } from '../hooks/useDPSBarScale'
import DPSTable, { OVERLAY_THEME } from '../components/overlays/DPSTable'
import { WSEvent } from '../lib/wsEvents'

// dpsModeIcon picks an icon matching the metric's intuition.
function dpsModeIcon(mode: DPSMode, size = 11): React.ReactElement {
  switch (mode) {
    case 'personal':
      return <User size={size} />
    case 'raid':
      return <Activity size={size} />
    case 'encounter':
      return <Hourglass size={size} />
  }
}

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

function truncateName(name: string, max = 24): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DPSOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('dps')
  const palette = useDPSClassColors()
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('dps')
  const chrome = useOverlayChromeFade('dps', mode === 'display-only')
  const onDragMouseDown = useWindowDrag()
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [combine, setCombine] = useCombinePetWithOwner()
  const { mode: dpsMode, toggle: toggleDPSMode } = useDPSMode()
  const { scope, toggle: toggleScope } = useDPSScope()
  const { barScale, toggle: toggleBarScale } = useDPSBarScale()
  const [now, setNow] = useState(() => Date.now())
  const [copied, setCopied] = useState(false)

  // Hold the most recent fight so the overlay keeps the numbers visible after
  // combat ends. Cleared when a new fight starts (replaced with current_fight)
  // or via the manual clear button.
  const [frozenFight, setFrozenFight] = useState<FightState | null>(null)

  // Skip the initial REST snapshot once a WS broadcast has applied, so a
  // late-resolving fetch can't clobber a fresher live update (mount-order race).
  const wsAppliedRef = useRef(false)
  useEffect(() => {
    getCombatState().then((s) => { if (!wsAppliedRef.current) setCombat(s) }).catch(() => {})
  }, [])

  // Capture the current fight while in combat; once combat ends we keep the
  // last frozenFight in state until a new fight begins or the user clears it.
  //
  // The out-of-combat branch adopts recent_fights[0] (the just-archived
  // fight) rather than relying on having seen an earlier in_combat=true
  // snapshot. The backend coalesces queued WS sends into one frame
  // (ws/client.go writePump drain loop), so the killing-hit broadcast and
  // the kill broadcast often arrive in the same onmessage callback. React
  // 18 batches the two setCombat calls into a single render, and the
  // in_combat=true intermediate state never reaches this effect — without
  // this fallback the panel would clear the moment the mob died.
  const latestFightRef = useRef<FightState | null>(null)
  useEffect(() => {
    if (!combat) return
    if (combat.in_combat && combat.current_fight) {
      // The render reads current_fight directly while in combat, so frozenFight
      // isn't displayed now — just stash it in a ref for the freeze-on-end,
      // avoiding a setState (and re-render) per combat message.
      latestFightRef.current = combat.current_fight
      return
    }
    const last = combat.recent_fights?.[0]
    if (last) {
      const { end_time: _endTime, ...state } = last
      setFrozenFight(state)
    } else if (latestFightRef.current) {
      setFrozenFight(latestFightRef.current)
    }
  }, [combat])

  // Tick every second while in combat so the live duration/DPS update.
  const inCombat = combat?.in_combat ?? false
  useEffect(() => {
    if (!inCombat) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [inCombat])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayCombat) {
      wsAppliedRef.current = true
      setCombat(msg.data as CombatState)
    }
  }, [])

  useWebSocket(handleMessage)

  // Pooled "last N mobs" fight — a synthetic FightState rendered by the same
  // DPSTable as a real fight. Stable/historical, so no freeze handling.
  const windowMode = scope === 'window'
  const windowFight = useMemo(
    () => aggregateRecentFights(combat?.recent_fights ?? []),
    [combat?.recent_fights],
  )

  const showingPostFight = !windowMode && !inCombat && frozenFight !== null
  const fight = windowMode
    ? windowFight
    : inCombat
      ? combat?.current_fight
      : showingPostFight
        ? frozenFight
        : null

  // In window scope the duration is the pooled wall-clock; otherwise it ticks
  // live while in combat and freezes to the fight's own duration afterwards.
  const liveSecs = fight
    ? windowMode || !inCombat
      ? fight.duration_seconds
      : Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
    : 0
  // Aggregate DPS respects the current mode. Encounter uses the wall-
  // clock liveSecs; Personal and Raid use the fight-level helpers.
  let liveTotalDPS = 0
  let liveYouDPS = 0
  if (fight) {
    if (dpsMode === 'encounter') {
      liveTotalDPS = liveSecs > 0 ? fight.total_damage / liveSecs : 0
      liveYouDPS = liveSecs > 0 ? fight.you_damage / liveSecs : 0
    } else {
      liveTotalDPS = fightAggregateDPS(fight.total_damage, fight.duration_seconds, fight.combatants ?? [], dpsMode)
      liveYouDPS = playerAggregateDPS(fight.you_damage, fight.duration_seconds, fight.combatants ?? [], 'You', dpsMode)
    }
  }

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100%',
        height: '100%',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
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
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          // Fade-when-inactive: hide the title bar with the rest of the
          // chrome; pointerEvents off so invisible buttons can't be clicked.
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Swords size={11} style={{ color: '#818cf8' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>{dpsModeAbbrev(dpsMode)}</span>
          {fight && (
            <span style={{ fontSize: 10, color: '#fb923c', marginLeft: 4 }}>
              {fmtDPS(showAll ? liveTotalDPS : liveYouDPS)}
            </span>
          )}
        </div>

        {/* Controls — no-drag zone. Hover handling lives on the header strip
            and/or the root depending on locked mode, so the controls inherit
            interactive mode while hovered. */}
        <div
          className="no-drag"
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
          {/* combine pets toggle */}
          <button
            onClick={() => setCombine(!combine)}
            title={combine ? 'Pet damage rolled up — click to split' : 'Pets shown separately — click to combine'}
            style={{
              display: 'flex',
              alignItems: 'center',
              background: 'none',
              border: 'none',
              padding: '1px 3px',
              cursor: 'pointer',
              color: combine ? '#818cf8' : 'rgba(255,255,255,0.4)',
            }}
          >
            <Users size={11} />
          </button>
          {/* Bar scale toggle: leader = full bar ↔ share of total. */}
          <button
            onClick={toggleBarScale}
            title={dpsBarScaleTooltip(barScale)}
            style={{
              display: 'flex',
              alignItems: 'center',
              background: 'none',
              border: 'none',
              padding: '1px 3px',
              cursor: 'pointer',
              color: barScale === 'leader' ? '#818cf8' : 'rgba(255,255,255,0.4)',
            }}
          >
            {barScale === 'leader' ? <ChartBarDecreasing size={11} /> : <ChartBar size={11} />}
          </button>
          {/* Scope toggle: current fight ↔ pooled last-20-mobs average. */}
          <button
            onClick={toggleScope}
            title={
              windowMode
                ? 'Showing a pooled average over the last 20 completed fights — click for the current fight only'
                : 'Showing the current fight — click for a pooled average over the last 20 mobs'
            }
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 3,
              background: 'none',
              border: 'none',
              padding: '1px 4px',
              cursor: 'pointer',
              color: windowMode ? '#a5b4fc' : 'rgba(255,255,255,0.4)',
              fontSize: 9,
              textTransform: 'uppercase',
              letterSpacing: '0.04em',
              fontWeight: 600,
            }}
          >
            {windowMode ? <History size={11} /> : <Crosshair size={11} />}
            {dpsScopeLabel(scope)}
          </button>
          {/* DPS mode toggle: cycles Personal → Raid → Encounter. */}
          <button
            onClick={toggleDPSMode}
            title={`${dpsModeLabel(dpsMode)} DPS — click to cycle (Personal → Raid → Encounter)`}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 3,
              background: 'none',
              border: 'none',
              padding: '1px 4px',
              cursor: 'pointer',
              color: '#818cf8',
              fontSize: 9,
              textTransform: 'uppercase',
              letterSpacing: '0.04em',
              fontWeight: 600,
            }}
          >
            {dpsModeIcon(dpsMode)}
            {dpsModeLabel(dpsMode)}
          </button>
          {/* copy fight summary */}
          <button
            onClick={() => {
              if (!fight) return
              const text = buildDpsFightSummary(fight.primary_target, fight, combine, dpsMode)
              if (!text) return
              navigator.clipboard.writeText(text).then(() => {
                setCopied(true)
                setTimeout(() => setCopied(false), 1500)
              }).catch(() => {})
            }}
            disabled={!fight}
            title="Copy DPS summary"
            style={{
              display: 'flex',
              alignItems: 'center',
              background: 'none',
              border: 'none',
              padding: '1px 3px',
              cursor: fight ? 'pointer' : 'default',
              color: copied ? '#4ade80' : 'rgba(255,255,255,0.4)',
              opacity: fight ? 1 : 0.3,
            }}
          >
            {copied ? <ClipboardCheck size={11} /> : <Clipboard size={11} />}
          </button>
          {/* end parse — finalises the current in-flight fight, saving it to
              Combat History, without wiping session totals. Only enabled
              while in combat. */}
          <button
            onClick={() => { if (inCombat) endActiveFight().catch(() => {}) }}
            disabled={!inCombat}
            title={
              inCombat
                ? 'End the current parse — clears this fight from the meter and saves it to Combat History (use after evac/zone when you’re done with the mob)'
                : 'No active fight to end'
            }
            style={{
              display: 'flex',
              alignItems: 'center',
              background: 'none',
              border: 'none',
              padding: '1px 3px',
              cursor: inCombat ? 'pointer' : 'default',
              color: 'rgba(255,255,255,0.4)',
              opacity: inCombat ? 1 : 0.4,
            }}
          >
            <XCircle size={11} />
          </button>
          {/* clear — resets the backend tracker (session damage, fight history,
              death log) and the local frozen-fight cache. Always enabled so the
              user can wipe the meter mid-fight too. */}
          <button
            onClick={() => {
              setFrozenFight(null)
              resetCombatState().catch(() => {})
            }}
            title="Clear DPS — reset session damage, fight history, and the meter"
            style={{
              display: 'flex',
              alignItems: 'center',
              background: 'none',
              border: 'none',
              padding: '1px 3px',
              cursor: 'pointer',
              color: 'rgba(255,255,255,0.4)',
            }}
          >
            <Trash2 size={11} />
          </button>
          {/* lock */}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
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
          color: windowMode ? '#a5b4fc' : inCombat ? '#f87171' : showingPostFight ? '#fb923c' : 'rgba(255,255,255,0.3)',
        }}
      >
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: windowMode ? '#818cf8' : inCombat ? '#ef4444' : showingPostFight ? '#fb923c' : 'rgba(255,255,255,0.2)',
            display: 'inline-block',
          }}
        />
        {fight ? (
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {fight.primary_target && (
              <span style={{ fontWeight: 700, color: 'rgba(255,255,255,0.85)' }}>
                {truncateName(fight.primary_target)}
              </span>
            )}
            {fight.primary_target && <span style={{ color: 'rgba(255,255,255,0.3)' }}> · </span>}
            {fmtDur(liveSecs)} · {fmt(fight.total_damage)} dmg
            {showingPostFight && (
              <span style={{ color: 'rgba(255,255,255,0.3)', marginLeft: 4 }}>
                (last fight)
              </span>
            )}
          </span>
        ) : (
          <span>Not in combat</span>
        )}
      </div>

      {/* ── Fight data ───────────────────────────────────────────────────── */}
      {combat === null ? (
        <p style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, color: 'rgba(255,255,255,0.3)', margin: 0 }}>
          Connecting…
        </p>
      ) : fight ? (
        <DPSTable
          fight={fight}
          showAll={showAll}
          combine={combine}
          mode={dpsMode}
          palette={palette}
          barScale={barScale}
          theme={OVERLAY_THEME}
        />
      ) : (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6, opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease' }}>
          {windowMode ? <History size={24} style={{ opacity: 0.15 }} /> : <Swords size={24} style={{ opacity: 0.15 }} />}
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
            {windowMode ? 'No completed fights yet' : 'Not in combat'}
          </p>
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

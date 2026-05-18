/**
 * DPSOverlayWindowPage — renders in the dedicated always-on-top Electron overlay
 * window. No sidebar, no title bar, transparent background.
 *
 * Unlocked: the window is movable (OS drag region on the header) and resizable.
 * Locked: the entire window passes clicks through to the game except the
 * header strip — hovering the header temporarily disables passthrough so its
 * buttons (clear, lock, close, etc.) stay clickable.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Swords, Clipboard, ClipboardCheck, Trash2, Users, Activity, Hourglass, User } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import { getCombatState, resetCombatState } from '../services/api'
import type { CombatState, FightState } from '../types/combat'
import { rollupCombatants, useCombinePetWithOwner, petBadge, type RolledUpEntity } from '../lib/dpsRollup'
import { combatantBarColor } from '../lib/combatantColor'
import { useDPSClassColors } from '../hooks/useDPSClassColors'
import type { DPSClassColors } from '../types/config'
import { useDPSMode, dpsForMode, dpsModeAbbrev, dpsModeLabel, fightAggregateDPS, playerAggregateDPS, type DPSMode } from '../hooks/useDPSMode'
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

function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

function truncateName(name: string, max = 24): string {
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

// ── Clipboard ──────────────────────────────────────────────────────────────────

function buildFightText(fight: FightState, combine: boolean, mode: DPSMode): string {
  const rolled = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  const label = dpsModeAbbrev(mode).toLowerCase()
  return rolled
    .slice(0, 10)
    .map((c, i) => `#${i + 1} ${c.name}${petBadge(c.pets)} ${Math.round(dpsForMode(c, mode))}${label} ${c.total_damage.toLocaleString()}dmg`)
    .join(' | ')
}

// ── Row ────────────────────────────────────────────────────────────────────────

function Row({ stat, totalDmg, expanded, onToggle, mode, palette }: { stat: RolledUpEntity; totalDmg: number; expanded: boolean; onToggle: () => void; mode: DPSMode; palette: DPSClassColors }): React.ReactElement {
  const isYou = stat.name === 'You'
  const barPct = totalDmg > 0 ? (stat.total_damage / totalDmg) * 100 : 0
  const hasPets = stat.pets.length > 0

  return (
    <div>
      <div
        onClick={hasPets ? onToggle : undefined}
        style={{
          position: 'relative',
          display: 'grid',
          gridTemplateColumns: '1fr auto auto auto',
          gap: '0 8px',
          padding: '4px 8px',
          alignItems: 'center',
          borderBottom: '1px solid rgba(255,255,255,0.06)',
          overflow: 'hidden',
          cursor: hasPets ? 'pointer' : 'default',
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
            backgroundColor: combatantBarColor(stat.class, palette, 0.28),
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
          {hasPets && <span style={{ color: 'rgba(255,255,255,0.45)', fontWeight: 400 }}>{petBadge(stat.pets)}</span>}
        </span>
        <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
          {pct(stat.total_damage, totalDmg)}
        </span>
        <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.7)', fontVariantNumeric: 'tabular-nums', position: 'relative' }}>
          {fmt(stat.total_damage)}
        </span>
        <span style={{ fontSize: 11, color: '#fb923c', fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>
          {fmtDPS(dpsForMode(stat, mode))}
        </span>
      </div>
      {hasPets && expanded && stat.pets.map((p) => (
        <div
          key={p.name}
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr auto auto auto',
            gap: '0 8px',
            padding: '2px 8px 2px 20px',
            alignItems: 'center',
            fontSize: 11,
            color: 'rgba(255,255,255,0.55)',
          }}
        >
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>↳ {p.name}</span>
          <span style={{ color: 'rgba(255,255,255,0.35)', fontVariantNumeric: 'tabular-nums' }}>{pct(p.total_damage, totalDmg)}</span>
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>{fmt(p.total_damage)}</span>
          <span style={{ color: '#fb923c', fontVariantNumeric: 'tabular-nums', minWidth: 44, textAlign: 'right' }}>{fmtDPS(dpsForMode(p, mode))}</span>
        </div>
      ))}
    </div>
  )
}

// ── Fight table ────────────────────────────────────────────────────────────────

function FightTable({ fight, showAll, combine, mode, palette }: { fight: FightState; showAll: boolean; combine: boolean; mode: DPSMode; palette: DPSClassColors }): React.ReactElement {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const rolled = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  const rows = showAll ? rolled : rolled.filter((c) => c.name === 'You')
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
        <span style={{ textAlign: 'right' }}>{dpsModeAbbrev(mode)}</span>
      </div>

      {rows.length === 0 ? (
        <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>No data</p>
      ) : (
        rows.map((s) => (
          <Row
            key={s.name}
            stat={s}
            totalDmg={totalDmg}
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

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DPSOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const palette = useDPSClassColors()
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const [combat, setCombat] = useState<CombatState | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [combine, setCombine] = useCombinePetWithOwner()
  const { mode: dpsMode, toggle: toggleDPSMode } = useDPSMode()
  const [now, setNow] = useState(() => Date.now())
  const [copied, setCopied] = useState(false)

  // Hold the most recent fight so the overlay keeps the numbers visible after
  // combat ends. Cleared when a new fight starts (replaced with current_fight)
  // or via the manual clear button.
  const [frozenFight, setFrozenFight] = useState<FightState | null>(null)

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
  }, [])

  // Capture the current fight while in combat; once combat ends we keep the
  // last frozenFight in state until a new fight begins or the user clears it.
  useEffect(() => {
    if (combat?.in_combat && combat.current_fight) {
      setFrozenFight(combat.current_fight)
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
      setCombat(msg.data as CombatState)
    }
  }, [])

  useWebSocket(handleMessage)

  const showingPostFight = !inCombat && frozenFight !== null
  const fight = inCombat ? combat?.current_fight : showingPostFight ? frozenFight : null

  const liveSecs = fight
    ? inCombat
      ? Math.max((now - new Date(fight.start_time).getTime()) / 1000, fight.duration_seconds)
      : fight.duration_seconds
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
        onMouseEnter={enableInteraction}
        onMouseLeave={enableClickThrough}
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
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>{dpsModeAbbrev(dpsMode)}</span>
          {fight && (
            <span style={{ fontSize: 10, color: '#fb923c', marginLeft: 4 }}>
              {fmtDPS(showAll ? liveTotalDPS : liveYouDPS)}
            </span>
          )}
        </div>

        {/* Controls — no-drag zone. Hover handling lives on the header strip
            (parent), so the controls inherit interactive mode while hovered. */}
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
              navigator.clipboard.writeText(buildFightText(fight, combine, dpsMode)).then(() => {
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
          color: inCombat ? '#f87171' : showingPostFight ? '#fb923c' : 'rgba(255,255,255,0.3)',
        }}
      >
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: inCombat ? '#ef4444' : showingPostFight ? '#fb923c' : 'rgba(255,255,255,0.2)',
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
        <FightTable fight={fight} showAll={showAll} combine={combine} mode={dpsMode} palette={palette} />
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

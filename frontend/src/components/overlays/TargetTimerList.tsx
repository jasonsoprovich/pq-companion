import React from 'react'
import { Swords } from 'lucide-react'
import { SpellIcon } from '../Icon'
import { useTimerAppearance } from '../../hooks/useTimerAppearance'
import { fmtRemaining } from '../../lib/timeFormat'
import type { ActiveTimer, TimerCategory } from '../../types/timer'

// Category accents mirror the dedicated buff/detrim overlays so a spell reads
// the same colour wherever it appears.
const CATEGORY_COLORS: Record<TimerCategory, string> = {
  buff: '#22c55e',
  debuff: '#f97316',
  dot: '#ef4444',
  mez: '#a855f7',
  stun: '#eab308',
  ch_chain: '#3b82f6',
  ch_chain_2: '#3b82f6',
  custom: '#38bdf8',
}

function TimerRow({ timer, showSeconds }: { timer: ActiveTimer; showSeconds: boolean }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const catColor = CATEGORY_COLORS[timer.category] ?? '#6b7280'
  const urgent = timer.duration_seconds > 0 && pct < 0.2
  const fill = urgent ? '#ef4444' : catColor
  // A same-named mob died elsewhere and the engine can't prove this is the
  // instance that died — see ActiveTimer.MaybeDead. Never auto-removed on
  // evidence this thin; just dimmed and marked so the player can judge for
  // themselves by retargeting.
  const maybeDead = timer.maybe_dead === true

  return (
    <div
      style={{
        position: 'relative',
        padding: '3px 6px',
        borderRadius: 3,
        overflow: 'hidden',
        backgroundColor: 'rgba(255,255,255,0.05)',
        opacity: maybeDead ? 0.45 : 1,
      }}
      title={maybeDead ? 'A mob with this name died — this may be it. Target it to confirm.' : undefined}
    >
      {/* depleting progress fill */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: fill,
          opacity: 0.3,
          pointerEvents: 'none',
          transition: 'width 1s linear',
        }}
      />
      {/* category accent line */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 2,
          backgroundColor: catColor,
          opacity: 0.85,
        }}
      />
      <div
        style={{
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 6,
          paddingLeft: 6,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0, flex: 1 }}>
          <SpellIcon id={timer.icon} name={timer.spell_name} size={15} loading="eager" />
          <span
            style={{
              fontSize: 11,
              color: urgent ? '#f87171' : 'rgba(255,255,255,0.92)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: urgent ? 600 : 500,
            }}
          >
            {maybeDead ? '? ' : ''}
            {timer.spell_name}
          </span>
          {timer.target_is_caster && (
            <span
              title="This NPC's own recast cooldown — not a debuff currently on it"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 2,
                flexShrink: 0,
                padding: '1px 4px',
                borderRadius: 3,
                fontSize: 9,
                fontWeight: 600,
                letterSpacing: 0.3,
                textTransform: 'uppercase',
                color: '#fbbf24',
                backgroundColor: 'rgba(251,191,36,0.12)',
              }}
            >
              <Swords size={9} />
              Cooldown
            </span>
          )}
        </div>
        <span
          style={{
            fontSize: 10,
            color: urgent ? '#f87171' : fill,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: 600,
          }}
        >
          {fmtRemaining(timer.remaining_seconds, showSeconds)}
        </span>
      </div>
    </div>
  )
}

/**
 * Read-only list of the active spell timers ticking on the overlay's current
 * target. Fed by useTargetTimers (threshold-ignoring), so it shows buffs and
 * detrimentals that are still hidden from the main timer overlays. Shared by
 * the dashboard NPC panel and the popped-out NPC overlay window.
 */
export default function TargetTimerList({ timers }: { timers: ActiveTimer[] }): React.ReactElement {
  const appearance = useTimerAppearance()
  if (timers.length === 0) {
    return (
      <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px' }}>
        No active timers on this target.
      </p>
    )
  }
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {timers.map((t) => (
        <TimerRow key={t.id} timer={t} showSeconds={appearance.showSeconds} />
      ))}
    </div>
  )
}

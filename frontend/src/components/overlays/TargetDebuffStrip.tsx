import React from 'react'
import { SpellIcon } from '../Icon'
import { useTargetTimers } from '../../hooks/useTargetTimers'
import { useTimerAppearance } from '../../hooks/useTimerAppearance'
import { fmtRemaining } from '../../lib/timeFormat'
import { DETRIM_CATEGORIES } from '../../lib/timerFilter'

/**
 * A row of the debuffs ticking on the current target, as icons with the time
 * left, so the Stats and Loot tabs show them without switching to Timers.
 * Renders nothing when there are none. Its own component so the ~1Hz timer
 * feed only re-renders the strip, not the whole stats tree.
 */
export default function TargetDebuffStrip({
  targetName,
  targetId,
  onOpen,
}: {
  targetName: string | undefined
  targetId?: number
  // Opens the Timers tab; the strip is a button when given.
  onOpen?: () => void
}): React.ReactElement | null {
  const appearance = useTimerAppearance()
  const debuffs = useTargetTimers(targetName, targetId).filter(
    (t) => DETRIM_CATEGORIES.has(t.category) && !t.target_is_caster,
  )
  if (debuffs.length === 0) return null
  return (
    <div
      onClick={onOpen}
      title={onOpen ? 'Debuffs on this target: open the Timers tab' : 'Debuffs on this target'}
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 4,
        marginTop: 5,
        cursor: onOpen ? 'pointer' : undefined,
      }}
    >
      {debuffs.map((t) => {
        const unproven = !!targetId && !t.target_id
        const urgent =
          t.duration_seconds > 0 && t.remaining_seconds / t.duration_seconds < 0.2
        return (
          <span
            key={t.id}
            title={
              `${t.spell_name}: ${fmtRemaining(t.remaining_seconds, true)} left` +
              (unproven ? '\nNot tied to this exact spawn; may be on another mob with the same name' : '')
            }
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 3,
              padding: '1px 4px 1px 1px',
              borderRadius: 3,
              backgroundColor: 'rgba(255,255,255,0.06)',
              opacity: unproven || t.maybe_dead ? 0.55 : 1,
            }}
          >
            <SpellIcon id={t.icon} name={t.spell_name} size={16} loading="eager" />
            <span
              style={{
                fontSize: 10,
                fontWeight: 600,
                fontVariantNumeric: 'tabular-nums',
                color: urgent ? '#f87171' : 'rgba(255,255,255,0.75)',
              }}
            >
              {fmtRemaining(t.remaining_seconds, appearance.showSeconds)}
              {unproven && '?'}
            </span>
          </span>
        )
      })}
    </div>
  )
}

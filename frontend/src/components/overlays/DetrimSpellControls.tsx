import React from 'react'
import { Bell, BellOff, EyeOff } from 'lucide-react'
import { removeTimer } from '../../services/api'
import { overrideFor, type DetrimOverrideMode, type DetrimOverrides } from '../../lib/detrimOverrides'

interface DetrimSpellControlsProps {
  timerId: string
  spellName: string
  overrides: DetrimOverrides
  setOverride: (spellName: string, mode: DetrimOverrideMode | null) => Promise<void>
  /** Idle icon colour — the row's muted text colour. */
  color: string
}

const MUTED_ACCENT = '#f59e0b'

// Two tiny per-spell buttons for a Detrimental timer row: silence this spell's
// "fading soon" alert, or stop showing the spell at all (e.g. Tashanian).
// Both persist to config and are listed in the Overlays hub settings so they can be
// reversed. Hiding also dismisses the timer that's currently up.
export function DetrimSpellControls({
  timerId, spellName, overrides, setOverride, color,
}: DetrimSpellControlsProps): React.ReactElement {
  const muted = overrideFor(overrides, spellName) === 'mute'
  const btn: React.CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', padding: 0,
    display: 'flex', alignItems: 'center', flexShrink: 0, lineHeight: 0,
  }
  return (
    <>
      <button
        onClick={() => setOverride(spellName, muted ? null : 'mute').catch(() => {})}
        title={muted ? `Alert muted for ${spellName} — click to unmute` : `Mute the fading-soon alert for ${spellName}`}
        style={{ ...btn, color: muted ? MUTED_ACCENT : color }}
      >
        {muted ? <BellOff size={11} /> : <Bell size={11} />}
      </button>
      <button
        onClick={() => {
          setOverride(spellName, 'hide').catch(() => {})
          removeTimer(timerId).catch(() => {})
        }}
        title={`Never show ${spellName} timers (undo in Settings → Overlays)`}
        style={{ ...btn, color }}
      >
        <EyeOff size={11} />
      </button>
    </>
  )
}

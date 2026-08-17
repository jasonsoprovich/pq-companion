import React from 'react'
import { ShieldAlert, Shield } from 'lucide-react'

interface OverlayInstanceModeButtonProps {
  enabled: boolean
  onToggle: () => void
  size?: number
}

/**
 * Shield toggle for the respawn overlay headers. Forces newly started timers
 * to use the raw (unreduced) respawn time instead of Quarm's fast-respawn
 * reduction — for guild/raid-locked instances (e.g. Sebilis, Howling
 * Stones), which run with the reduction disabled server-side but are
 * otherwise indistinguishable from the open-world zone (LIMITATIONS.md
 * §4.1). Manual because there is no data source to detect this
 * automatically.
 */
export default function OverlayInstanceModeButton({
  enabled,
  onToggle,
  size = 11,
}: OverlayInstanceModeButtonProps): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      title={
        enabled
          ? 'Normal timers — new kills use the full unreduced respawn time (click to use fast-respawn timers)'
          : 'Fast timers — Quarm\'s reduction is applied (click to use normal/unreduced timers for a guild/raid-locked instance)'
      }
      aria-pressed={enabled}
      style={{
        display: 'flex',
        alignItems: 'center',
        padding: '1px 5px',
        borderRadius: 3,
        border: '1px solid rgba(255,255,255,0.1)',
        backgroundColor: 'transparent',
        color: enabled ? '#f97316' : 'rgba(255,255,255,0.4)',
        cursor: 'pointer',
        lineHeight: 1,
      }}
    >
      {enabled ? <ShieldAlert size={size} /> : <Shield size={size} />}
    </button>
  )
}

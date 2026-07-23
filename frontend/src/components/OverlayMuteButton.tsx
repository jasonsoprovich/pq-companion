import React from 'react'
import { Bell, BellOff } from 'lucide-react'

interface OverlayMuteButtonProps {
  enabled: boolean
  onToggle: () => void
  size?: number
}

/**
 * Bell toggle for overlay popout headers. Mutes/unmutes this overlay's
 * audio/TTS alerts without touching the underlying preference in Settings.
 */
export default function OverlayMuteButton({
  enabled,
  onToggle,
  size = 11,
}: OverlayMuteButtonProps): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      title={enabled ? 'Alerts on (click to mute)' : 'Alerts muted (click to unmute)'}
      aria-pressed={enabled}
      style={{
        display: 'flex',
        alignItems: 'center',
        padding: '1px 5px',
        borderRadius: 3,
        border: '1px solid rgba(255,255,255,0.1)',
        backgroundColor: 'transparent',
        color: enabled ? '#93c5fd' : 'rgba(255,255,255,0.4)',
        cursor: 'pointer',
        lineHeight: 1,
      }}
    >
      {enabled ? <Bell size={size} /> : <BellOff size={size} />}
    </button>
  )
}

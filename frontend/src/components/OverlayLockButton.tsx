import React from 'react'
import { Lock, LockOpen } from 'lucide-react'

interface OverlayLockButtonProps {
  locked: boolean
  onToggle: () => void
  size?: number
}

/**
 * Padlock toggle for overlay popout headers. Closed = locked (click-through),
 * open = unlocked (movable/resizable).
 */
export default function OverlayLockButton({
  locked,
  onToggle,
  size = 11,
}: OverlayLockButtonProps): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      title={locked ? 'Unlock (allow move/resize)' : 'Lock (click-through)'}
      style={{
        display: 'flex',
        alignItems: 'center',
        background: 'none',
        border: '1px solid rgba(255,255,255,0.1)',
        borderRadius: 3,
        padding: '1px 5px',
        cursor: 'pointer',
        color: locked ? '#c9a84c' : 'rgba(255,255,255,0.4)',
        lineHeight: 1,
      }}
    >
      {locked ? <Lock size={size} /> : <LockOpen size={size} />}
    </button>
  )
}

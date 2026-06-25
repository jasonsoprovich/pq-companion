import React from 'react'
import { Pin, PinOff } from 'lucide-react'

interface TargetPinButtonProps {
  pinned: boolean
  onToggle: () => void
  size?: number
}

/**
 * Pin toggle for the NPC overlay header. When pinned, the overlay holds onto
 * the current target and ignores target swaps until unpinned — handy for
 * watching a boss while clicking other mobs during a fight.
 */
export default function TargetPinButton({
  pinned,
  onToggle,
  size = 12,
}: TargetPinButtonProps): React.ReactElement {
  return (
    <button
      onClick={onToggle}
      title={
        pinned
          ? 'Unpin target (follow current target again)'
          : 'Pin target (keep this NPC while you swap targets)'
      }
      style={{
        display: 'flex',
        alignItems: 'center',
        background: pinned ? 'rgba(201,168,76,0.15)' : 'none',
        border: '1px solid rgba(255,255,255,0.1)',
        borderRadius: 3,
        padding: '1px 5px',
        cursor: 'pointer',
        color: pinned ? '#c9a84c' : 'rgba(255,255,255,0.4)',
        lineHeight: 1,
      }}
    >
      {pinned ? <Pin size={size} /> : <PinOff size={size} />}
    </button>
  )
}

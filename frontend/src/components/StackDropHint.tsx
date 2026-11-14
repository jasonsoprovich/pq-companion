/**
 * StackDropHint — shown in an overlay window while another one is being
 * dragged onto it to stack (main decides when; see "Drag to stack" in
 * electron/main). The target gets a gold outline, and the dragged window
 * says what letting go will do. Never takes the mouse.
 */
import React, { useEffect, useState } from 'react'
import { Layers } from 'lucide-react'
import { overlayWindowLabel } from '../lib/overlayCatalog'
import type { StackDropHint as Hint } from '../types/electron'
import { instanceKind } from '../lib/instanceKeys'

function label(hint: NonNullable<Hint>): string {
  if (hint.key.startsWith('stack:')) return 'the stack'
  const kind = instanceKind(hint.key)
  if (kind) return hint.name || (kind === 'customTimer' ? 'the timer window' : 'the custom overlay')
  return overlayWindowLabel(hint.key, [])
}

export default function StackDropHint(): React.ReactElement | null {
  const [hint, setHint] = useState<Hint>(null)
  useEffect(() => window.electron?.overlay?.onStackDropHint?.(setHint), [])
  if (!hint) return null
  const text = hint.role === 'source' ? `Release to stack with ${label(hint)}` : `Drop ${label(hint)} here to stack`
  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 10000,
        pointerEvents: 'none',
        borderRadius: 8,
        border: hint.role === 'target' ? '2px solid #c9a84c' : '2px dashed rgba(201,168,76,0.7)',
        backgroundColor: hint.role === 'target' ? 'rgba(201,168,76,0.12)' : 'transparent',
        display: 'flex',
        alignItems: hint.role === 'target' ? 'center' : 'flex-end',
        justifyContent: 'center',
        padding: 8,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '4px 10px',
          borderRadius: 6,
          backgroundColor: 'rgba(10,10,12,0.9)',
          border: '1px solid rgba(201,168,76,0.6)',
          color: '#c9a84c',
          fontSize: 11,
          fontWeight: 600,
          fontFamily: 'system-ui, -apple-system, sans-serif',
          whiteSpace: 'nowrap',
        }}
      >
        <Layers size={12} />
        {text}
      </div>
    </div>
  )
}

import React from 'react'
import { Layers } from 'lucide-react'
import { useOverlayPopouts } from '../hooks/useOverlayPopouts'

// Sidebar quick-toggle for popping every visible overlay out (or closing them
// all again) without leaving the current page. Sits next to the character
// switcher — that area of the sidebar is always visible — and mirrors the
// "Pop Out All / Close All Popouts" action from the Overlays page's Manage
// overlays menu (same IPC calls via useOverlayPopouts, so both stay in sync).
export default function OverlayPopoutToggle(): React.ReactElement | null {
  const { anyPopoutOpen, supported, toggle } = useOverlayPopouts()

  if (!supported) return null

  return (
    <button
      onClick={toggle}
      className="flex h-[30px] w-7 shrink-0 items-center justify-center rounded transition-colors"
      style={{
        border: '1px solid var(--color-border)',
        backgroundColor: anyPopoutOpen ? 'var(--color-primary)' : 'var(--color-surface-2)',
        color: anyPopoutOpen ? 'var(--color-primary-foreground)' : 'var(--color-muted)',
      }}
      title={anyPopoutOpen ? 'Close all pop-out overlay windows' : 'Pop out all overlay windows'}
    >
      <Layers size={13} />
    </button>
  )
}

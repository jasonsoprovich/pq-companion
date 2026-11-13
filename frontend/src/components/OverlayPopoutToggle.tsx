import React from 'react'
import { EyeOff, Layers } from 'lucide-react'
import { useOverlayPopouts } from '../hooks/useOverlayPopouts'

// Sidebar quick-toggle for overlays, next to the search bar — that area of
// the sidebar is always visible. One click shows hidden overlays, hides open
// ones (they stay alive and in place; trigger alerts keep showing), or pops
// out the saved set when nothing is open. Same action as the global
// show/hide hotkey (see useOverlayPopouts).
export default function OverlayPopoutToggle(): React.ReactElement | null {
  const { anyPopoutOpen, anyHidden, supported, toggle } = useOverlayPopouts()

  if (!supported) return null

  const showing = anyPopoutOpen && !anyHidden
  return (
    <button
      onClick={toggle}
      className="flex h-[30px] w-7 shrink-0 items-center justify-center rounded transition-colors"
      style={{
        border: `1px solid ${anyHidden ? 'var(--color-primary)' : 'var(--color-border)'}`,
        backgroundColor: showing ? 'var(--color-primary)' : 'var(--color-surface-2)',
        color: showing
          ? 'var(--color-primary-foreground)'
          : anyHidden
            ? 'var(--color-primary)'
            : 'var(--color-muted)',
      }}
      title={
        anyHidden
          ? 'Overlays hidden — click to show them again'
          : anyPopoutOpen
            ? 'Hide all overlays (they stay in place; trigger alerts keep showing)'
            : 'Pop out your overlays'
      }
    >
      {anyHidden ? <EyeOff size={13} /> : <Layers size={13} />}
    </button>
  )
}

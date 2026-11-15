import React, { useEffect, useRef, useState } from 'react'
import { Box, EyeOff } from 'lucide-react'
import { useModelPreviewPrefs } from '../../hooks/useModelPreviewPrefs'

// ModelPreviewCard frames a 3D model preview on the item and NPC pages.
// The viewer inside is only mounted once the card is on screen, and not at
// all while previews are turned off, so a page that isn't showing one
// never starts WebGL.

export default function ModelPreviewCard({ controls, children }: {
  /** Pickers shown over the model (race, gender, era). */
  controls?: React.ReactNode
  children: React.ReactNode
}): React.ReactElement {
  const [prefs, setPrefs] = useModelPreviewPrefs()
  const host = useRef<HTMLDivElement>(null)
  const [seen, setSeen] = useState(false)

  useEffect(() => {
    const el = host.current
    if (!el || seen) return
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) setSeen(true)
    })
    io.observe(el)
    return () => io.disconnect()
  }, [seen, prefs.show])

  if (!prefs.show) {
    return (
      <button
        type="button"
        onClick={() => setPrefs({ show: true })}
        className="flex items-center gap-1.5 self-start rounded px-2 py-1 text-[11px]"
        style={{ border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)' }}
        title="Show 3D models on item and NPC pages"
      >
        <Box size={12} /> Show 3D model
      </button>
    )
  }
  return (
    <div
      ref={host}
      className="relative h-72 w-64 shrink-0 overflow-hidden rounded-lg"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      {seen && children}
      <div className="absolute left-2 right-2 top-2 flex items-start gap-1.5">
        <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">{controls}</div>
        <button
          type="button"
          onClick={() => setPrefs({ show: false })}
          className="rounded p-1"
          style={{ color: 'var(--color-muted)' }}
          title="Hide 3D models on item and NPC pages"
        >
          <EyeOff size={13} />
        </button>
      </div>
    </div>
  )
}

/** PreviewToggle is a small two-way switch for the card's controls. */
export function PreviewToggle({ options, value, onChange }: {
  options: { value: number; label: string }[]
  value: number
  onChange: (v: number) => void
}): React.ReactElement {
  return (
    <div className="flex overflow-hidden rounded text-[10px]" style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className="px-1.5 py-0.5"
          style={o.value === value
            ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }
            : { color: 'var(--color-muted-foreground)' }}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

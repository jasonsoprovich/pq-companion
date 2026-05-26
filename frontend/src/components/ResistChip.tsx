import React from 'react'

// Per-resist colour scheme used wherever NPC resists are rendered (NPC
// overlay window, dashboard NPC panel). Matches EQ convention: red for
// fire, blue for cold, green for poison, lime for disease, off-white for
// magic. Background uses the same hue at low opacity so the chip reads as
// a tinted bubble rather than a solid block.

export type ResistType = 'fire' | 'cold' | 'magic' | 'poison' | 'disease'

const RESIST_PALETTE: Record<ResistType, { bg: string; fg: string; label: string }> = {
  fire:    { bg: 'rgba(239, 68, 68, 0.22)',   fg: '#fca5a5', label: 'FR' },
  cold:    { bg: 'rgba(59, 130, 246, 0.22)',  fg: '#93c5fd', label: 'CR' },
  poison:  { bg: 'rgba(22, 101, 52, 0.45)',   fg: '#86efac', label: 'PR' },
  disease: { bg: 'rgba(132, 204, 22, 0.22)',  fg: '#bef264', label: 'DR' },
  magic:   { bg: 'rgba(229, 231, 235, 0.18)', fg: '#f3f4f6', label: 'MR' },
}

export function ResistChip({ type, value }: { type: ResistType; value: number }): React.ReactElement {
  const c = RESIST_PALETTE[type]
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        gap: 4,
        backgroundColor: c.bg,
        borderRadius: 3,
        padding: '3px 7px',
        fontSize: 12,
        lineHeight: 1.4,
      }}
    >
      <span
        style={{
          color: c.fg,
          opacity: 0.7,
          fontWeight: 600,
          fontSize: 10,
          textTransform: 'uppercase',
          letterSpacing: '0.04em',
        }}
      >
        {c.label}
      </span>
      <span style={{ color: c.fg, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
        {value}
      </span>
    </span>
  )
}

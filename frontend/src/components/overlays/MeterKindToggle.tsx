/**
 * DPS | HPS switch for the DPS meter's title bar (popout and dashboard).
 */
import React from 'react'
import { HeartPulse, Swords } from 'lucide-react'
import type { MeterKind } from '../../hooks/useMeterKind'
import { HPS_COLOR } from '../../lib/hpsMeter'

const DPS_COLOR = '#818cf8'

export default function MeterKindToggle({
  kind,
  onChange,
  size = 11,
  borderColor = 'rgba(255,255,255,0.15)',
  idleColor = 'rgba(255,255,255,0.45)',
  dpsLabel = 'DPS',
  hpsLabel = 'HPS',
}: {
  kind: MeterKind
  onChange: (k: MeterKind) => void
  size?: number
  borderColor?: string
  idleColor?: string
  // Mode-specific abbreviations (pDPS, rHPS…) for the active side.
  dpsLabel?: string
  hpsLabel?: string
}): React.ReactElement {
  const option = (k: MeterKind, label: string, icon: React.ReactNode, color: string, title: string): React.ReactElement => {
    const on = kind === k
    return (
      <button
        onClick={() => onChange(k)}
        title={title}
        aria-pressed={on}
        style={{
          display: 'flex', alignItems: 'center', gap: 3,
          padding: '1px 5px', border: 'none', cursor: 'pointer',
          backgroundColor: on ? `${color}2e` : 'transparent',
          color: on ? color : idleColor,
          fontSize: size, fontWeight: 700,
        }}
      >
        {icon}
        {label}
      </button>
    )
  }
  return (
    <span
      className="no-drag"
      // Keep a click here from starting the window drag on its title bar.
      onMouseDown={(e) => e.stopPropagation()}
      style={{ display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: `1px solid ${borderColor}` }}
    >
      {option('dps', kind === 'dps' ? dpsLabel : 'DPS', <Swords size={size} />, DPS_COLOR, 'Damage meter')}
      {option('hps', kind === 'hps' ? hpsLabel : 'HPS', <HeartPulse size={size} />, HPS_COLOR,
        'Healing meter: your heals, split by who you healed')}
    </span>
  )
}

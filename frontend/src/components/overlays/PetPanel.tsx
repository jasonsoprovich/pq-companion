import React from 'react'
import { ExternalLink, PawPrint } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import PetView from './PetView'
import { usePetState } from '../../hooks/usePetState'

interface PetPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Dashboard-embedded Pet panel — same live state as the popout window
// (PetWindowPage), both fed by pet:state broadcasts.
export default function PetPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 304,
  defaultHeight = 432,
  snapGridSize,
  onLayoutChange,
}: PetPanelProps): React.ReactElement {
  const state = usePetState()
  const btn: React.CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px',
    color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
  }

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <PawPrint size={13} style={{ color: '#a855f7' }} />
          Pet
        </span>
      }
      headerRight={
        window.electron?.overlay ? (
          <button
            onClick={() => window.electron.overlay.togglePet()}
            title="Pop out as floating overlay"
            style={btn}
          >
            <ExternalLink size={12} />
          </button>
        ) : undefined
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={220}
      minHeight={160}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <PetView state={state} />
    </OverlayWindow>
  )
}

import React from 'react'
import { ExternalLink, Skull } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import DeathRecapView from './DeathRecapView'
import { useLatestDeath } from '../../hooks/useLatestDeath'

interface DeathRecapPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Dashboard-embedded Death Recap panel — same data as the popout window
// (DeathRecapWindowPage): the most recent death and its recap.
export default function DeathRecapPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 352,
  defaultHeight = 368,
  snapGridSize,
  onLayoutChange,
}: DeathRecapPanelProps): React.ReactElement {
  const death = useLatestDeath()
  const btn: React.CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px',
    color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
  }

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Skull size={13} style={{ color: '#ef4444' }} />
          Death Recap
        </span>
      }
      headerRight={
        window.electron?.overlay ? (
          <button
            onClick={() => window.electron.overlay.toggleDeathRecap()}
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
      minWidth={240}
      minHeight={120}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div style={{ height: '100%', overflowY: 'auto', padding: '6px 10px' }}>
        {death ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div style={{ fontSize: 10, color: 'var(--color-muted)' }}>
              {new Date(death.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
              {death.zone ? ` · ${death.zone}` : ''}
              {death.character ? ` · ${death.character}` : ''}
            </div>
            <DeathRecapView death={death} theme="panel" />
          </div>
        ) : (
          <div style={{ fontSize: 11, color: 'var(--color-muted)' }}>
            No deaths yet. The recap of your next death shows here.
          </div>
        )}
      </div>
    </OverlayWindow>
  )
}

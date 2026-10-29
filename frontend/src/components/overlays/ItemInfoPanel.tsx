import React, { useState } from 'react'
import { ExternalLink, FlaskConical, Info } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import ItemInfoView from './ItemInfoView'
import ItemSearchModal from '../ItemSearchModal'
import { useItemDisplay } from '../../hooks/useItemDisplay'
import { useDeveloperMode } from '../../hooks/useDeveloperMode'
import { simulateItemDisplay } from '../../services/api'

interface ItemInfoPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Dashboard-embedded Item Info panel — same live item as the popout window
// (ItemInfoWindowPage), both fed by item:display broadcasts. In Developer
// Mode the header gets a "simulate item click" picker, which fakes the Zeal
// event through the backend so every open view updates as it would in game.
export default function ItemInfoPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 320,
  defaultHeight = 464,
  snapGridSize,
  onLayoutChange,
}: ItemInfoPanelProps): React.ReactElement {
  const display = useItemDisplay()
  const devMode = useDeveloperMode()
  const [pickerOpen, setPickerOpen] = useState(false)
  const btn: React.CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px',
    color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
  }

  return (
    <>
      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Info size={13} style={{ color: '#38bdf8' }} />
            Item Info
          </span>
        }
        headerRight={
          <span style={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            {devMode && (
              <button
                onClick={() => setPickerOpen(true)}
                title="Simulate an in-game item click (Developer Mode)"
                style={btn}
              >
                <FlaskConical size={12} />
              </button>
            )}
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleItemInfo()}
                title="Pop out as floating overlay"
                style={btn}
              >
                <ExternalLink size={12} />
              </button>
            )}
          </span>
        }
        defaultWidth={defaultWidth}
        defaultHeight={defaultHeight}
        defaultX={defaultX}
        defaultY={defaultY}
        minWidth={260}
        minHeight={160}
        snapGridSize={snapGridSize}
        onLayoutChange={onLayoutChange}
      >
        <div className="flex h-full min-h-0 flex-col">
          <ItemInfoView {...display} surface="dashboard" />
        </div>
      </OverlayWindow>
      <ItemSearchModal
        open={pickerOpen}
        title="Simulate item click"
        onSelect={(it) => {
          setPickerOpen(false)
          simulateItemDisplay(it.id).catch(() => {})
        }}
        onClose={() => setPickerOpen(false)}
      />
    </>
  )
}

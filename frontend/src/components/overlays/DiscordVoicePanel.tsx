import React from 'react'
import { Headphones, ExternalLink } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'

interface DiscordVoicePanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// DiscordVoicePanel — dashboard-docked placeholder for the Discord Voice
// overlay (issue #150). Unlike every other dashboard panel, this one can't
// show live content in-app: the real roster is Discord's own StreamKit page,
// embedded via a native Electron WebContentsView that only exists on the
// popout overlay window (electron/main/index.ts createDiscordVoiceOverlay) —
// there's no equivalent for docking a live native view inside the main
// window's React canvas. This panel exists so Discord Voice still shows up
// consistently in "Manage overlays" with working Dash/Pop/Move/Reset
// controls, with a one-click shortcut to the real pop-out.
export default function DiscordVoicePanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 240,
  defaultHeight = 200,
  snapGridSize,
  onLayoutChange,
}: DiscordVoicePanelProps): React.ReactElement {
  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Headphones size={13} style={{ color: '#5865f2' }} />
          Discord Voice
        </span>
      }
      headerRight={
        window.electron?.overlay && (
          <button
            onClick={() => window.electron.overlay.toggleDiscordVoice()}
            title="Pop out as floating overlay"
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              padding: '1px 3px',
              color: 'var(--color-muted)',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <ExternalLink size={12} />
          </button>
        )
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={200}
      minHeight={140}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 8,
          padding: 12,
          textAlign: 'center',
        }}
      >
        <p style={{ fontSize: 11, color: 'var(--color-muted-foreground)', lineHeight: 1.5 }}>
          The live roster only renders in its own pop-out window — it embeds
          Discord's own page, which can't be docked inline here.
        </p>
        <button
          onClick={() => window.electron?.overlay?.toggleDiscordVoice()}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '4px 10px',
            borderRadius: 4,
            border: '1px solid var(--color-border)',
            backgroundColor: 'var(--color-primary)',
            color: '#fff',
            fontSize: 11,
            fontWeight: 600,
            cursor: 'pointer',
          }}
        >
          <ExternalLink size={12} /> Pop out
        </button>
      </div>
    </OverlayWindow>
  )
}

import React from 'react'
import { ArrowUpCircle, ExternalLink, Circle, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import OverlayWindow from '../OverlayWindow'
import { GearUpgradeList, useZoneGearUpgrades, type ZoneGearUpgrades } from './gearUpgradesShared'

interface GearUpgradesPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

const iconButton: React.CSSProperties = {
  background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px',
  color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
}

export default function GearUpgradesPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 300,
  defaultHeight = 380,
  snapGridSize,
  onLayoutChange,
}: GearUpgradesPanelProps): React.ReactElement {
  const navigate = useNavigate()
  const g = useZoneGearUpgrades()

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <ArrowUpCircle size={13} style={{ color: '#22c55e' }} />
          Gear Upgrades
        </span>
      }
      headerRight={
        <span style={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <button onClick={g.reload} title="Check again" style={iconButton}>
            <RefreshCw size={12} />
          </button>
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleGearUpgrades()}
              title="Pop out as floating overlay"
              style={iconButton}
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
      minWidth={220}
      minHeight={140}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', fontSize: 11, borderBottom: '1px solid var(--color-border)', flexShrink: 0, backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
        <Circle size={10} style={{ color: g.zone ? '#22c55e' : '#6b7280' }} />
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {g.zone ? g.zoneName : 'Zone unknown'}
          {g.character && ` · ${g.character}`}
        </span>
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        <GearUpgradesBody
          g={g}
          muted="var(--color-muted)"
          onOpenFinder={() => navigate('/characters/upgrades')}
        >
          <GearUpgradeList
            rows={g.rows}
            variant="panel"
            onItem={(id) => navigate(`/items?select=${id}`)}
            onNpc={(id) => navigate(`/npcs?select=${id}`)}
          />
        </GearUpgradesBody>
      </div>
    </OverlayWindow>
  )
}

// GearUpgradesBody shows the overlay's waiting, empty and error states, or
// the list once there is one.
export function GearUpgradesBody({
  g,
  muted,
  onOpenFinder,
  children,
}: {
  g: ZoneGearUpgrades
  muted: string
  onOpenFinder?: () => void
  children: React.ReactNode
}): React.ReactElement {
  const message = (text: string, link?: boolean): React.ReactElement => (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, padding: 16, color: muted }}>
      <ArrowUpCircle size={24} style={{ opacity: 0.2 }} />
      <p style={{ fontSize: 11, margin: 0, textAlign: 'center' }}>{text}</p>
      {link && onOpenFinder && (
        <button
          type="button"
          onClick={onOpenFinder}
          style={{ background: 'none', border: 'none', padding: 0, fontSize: 11, color: 'var(--color-primary)', cursor: 'pointer', textDecoration: 'underline' }}
        >
          Open the Gear Upgrade Finder
        </button>
      )}
    </div>
  )
  if (!g.character) return message('Waiting for a character…')
  if (g.noCharacter) return message(`Set ${g.character}'s class and level on the Characters page to see upgrades.`)
  if (!g.zone) return message('Waiting for zone…')
  if (g.error) return message(`Couldn't check upgrades: ${g.error}`)
  if (!g.data) return message('Checking…')
  if (g.rows.length === 0) return message('No upgrades drop in this zone.', true)
  if (!g.data.has_current_gear) {
    return (
      <>
        <p style={{ margin: 0, padding: '5px 10px', fontSize: 10, color: '#f59e0b' }}>
          No Zeal export found, so these are ranked by stats alone, not against your gear.
        </p>
        {children}
      </>
    )
  }
  return <>{children}</>
}

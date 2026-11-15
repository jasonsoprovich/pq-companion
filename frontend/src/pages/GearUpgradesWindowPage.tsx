/**
 * GearUpgradesWindowPage — transparent always-on-top overlay listing the
 * active character's best gear upgrades that drop in the current zone, by
 * slot, with the NPCs here that drop them. Renders in a dedicated frameless
 * Electron window.
 */
import React from 'react'
import { ArrowUpCircle, RefreshCw } from 'lucide-react'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { openEntityInMain, openRouteInMain } from '../lib/overlayNav'
import {
  GearUpgradeList,
  gearUpgradesSummary,
  useZoneGearUpgrades,
} from '../components/overlays/gearUpgradesShared'
import { GearUpgradesBody } from '../components/overlays/GearUpgradesPanel'

const headerButton: React.CSSProperties = {
  fontSize: 11,
  lineHeight: 1,
  padding: '1px 5px',
  borderRadius: 3,
  border: '1px solid rgba(255,255,255,0.1)',
  backgroundColor: 'transparent',
  color: 'rgba(255,255,255,0.4)',
  cursor: 'pointer',
}

export default function GearUpgradesWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('gearUpgrades')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('gearUpgrades')
  const chromeFade = useOverlayChromeFade('gearUpgrades', mode === 'display-only')
  // A collapsed window is only its title bar, so keep that visible through
  // the inactive fade.
  const { collapsed, toggleCollapsed, headerRef } = useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const g = useZoneGearUpgrades()

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100%',
        height: '100%',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      <div
        ref={headerRef}
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0 }}>
          <ArrowUpCircle size={11} style={{ color: '#22c55e', flexShrink: 0 }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)', whiteSpace: 'nowrap' }}>
            Gear Upgrades
          </span>
          {g.zone && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {g.zoneName}
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={gearUpgradesSummary(g.rows)} />
          <button onClick={g.reload} style={{ ...headerButton, display: 'flex', alignItems: 'center', padding: '2px 4px' }} title="Check again">
            <RefreshCw size={10} />
          </button>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeGearUpgrades()}
            style={headerButton}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        <GearUpgradesBody
          g={g}
          muted="rgba(255,255,255,0.3)"
          onOpenFinder={() => openRouteInMain('/characters/upgrades')}
        >
          <GearUpgradeList
            rows={g.rows}
            variant="window"
            onItem={(id) => openEntityInMain('item', id)}
            onNpc={(id) => openEntityInMain('npc', id)}
          />
        </GearUpgradesBody>
      </div>
    </div>
  )
}

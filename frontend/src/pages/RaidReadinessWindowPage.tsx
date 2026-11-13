/**
 * RaidReadinessWindowPage — transparent always-on-top overlay showing which
 * MIN-level raid roles are still short, for the encounter detected from the
 * live Zeal roster. Renders in a dedicated frameless Electron window.
 */
import React from 'react'
import { ShieldAlert, ShieldCheck, Circle } from 'lucide-react'
import { useRaidReadiness } from '../hooks/useRaidReadiness'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { deriveGapRows, RaidGapRow } from '../components/overlays/raidReadinessShared'

export default function RaidReadinessWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('raidReadiness')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('raidReadiness')
  const chromeFade = useOverlayChromeFade('raidReadiness', mode === 'display-only')
  // Collapse to title bar. A collapsed window is only its title bar, so keep
  // that visible through the inactive fade.
  const { collapsed, toggleCollapsed, headerRef } = useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const { roster, report, encounters, selectedId } = useRaidReadiness()
  const encounter = encounters.find((e) => e.id === selectedId)
  const gapRows = deriveGapRows(report, 'min')
  const ok = report ? gapRows.length === 0 : null

  // "Missing: Enchanter, Shaman" or "Ready".
  const collapsedSummary =
    ok === null ? null : ok ? 'Ready' : `Missing: ${gapRows.map((r) => r.label).join(', ')}`

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
      {/* ── Drag handle / title bar ─────────────────────────────────────── */}
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
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          {ok === false ? (
            <ShieldAlert size={11} style={{ color: '#ef4444' }} />
          ) : (
            <ShieldCheck size={11} style={{ color: '#a855f7' }} />
          )}
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Raid Readiness
          </span>
          {encounter && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {encounter.name}
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={collapsedSummary} />
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeRaidReadiness()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* ── Gap list ─────────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {!report ? (
          <div
            style={{
              flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
              justifyContent: 'center', gap: 6, padding: 16,
              opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease',
            }}
          >
            <Circle size={22} style={{ opacity: 0.15 }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0, textAlign: 'center' }}>
              Pick an encounter on the Raid Composition page
            </p>
          </div>
        ) : gapRows.length === 0 ? (
          <div
            style={{
              flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
              justifyContent: 'center', gap: 6, padding: 16,
              opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease',
            }}
          >
            <ShieldCheck size={22} style={{ color: '#22c55e', opacity: 0.7 }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', margin: 0, textAlign: 'center' }}>
              Every MIN role is staffed
            </p>
          </div>
        ) : (
          gapRows.map((row) => <RaidGapRow key={row.path} row={row} variant="window" />)
        )}
      </div>
    </div>
  )
}

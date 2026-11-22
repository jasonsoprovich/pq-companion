import React, { useEffect, useState, lazy, Suspense } from 'react'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { ErrorBoundary } from './components/ErrorBoundary'
import StackDropHint from './components/StackDropHint'
import { loadEnums } from './lib/enumsCache'
import { useOverlayZoom } from './hooks/useOverlayZoom'

// The main window's UI (sidebar, pages, wizards, audio/alert hooks) lives in
// MainApp and is lazy-loaded, so overlay windows never load it. Overlay pages
// are lazy too: each window loads only its own page.
const MainApp = lazy(() => import('./MainApp'))
const LiveMapWindowPage = lazy(() => import('./pages/LiveMapWindowPage'))
const DPSOverlayWindowPage = lazy(() => import('./pages/DPSOverlayWindowPage'))
const BuffTimerWindowPage = lazy(() => import('./pages/BuffTimerWindowPage'))
const FilteredTimerWindowPage = lazy(() => import('./pages/FilteredTimerWindowPage'))
const KillCounterWindowPage = lazy(() => import('./pages/KillCounterWindowPage'))
const PetWindowPage = lazy(() => import('./pages/PetWindowPage'))
const DeathRecapWindowPage = lazy(() => import('./pages/DeathRecapWindowPage'))
const GearUpgradesWindowPage = lazy(() => import('./pages/GearUpgradesWindowPage'))
const ItemInfoWindowPage = lazy(() => import('./pages/ItemInfoWindowPage'))
const CHChainOverlayWindowPage = lazy(() => import('./pages/CHChainOverlayWindowPage'))
const CHMetronomeOverlayWindowPage = lazy(() => import('./pages/CHMetronomeOverlayWindowPage'))
const DetrimTimerWindowPage = lazy(() => import('./pages/DetrimTimerWindowPage'))
const CustomTimerWindowPage = lazy(() => import('./pages/CustomTimerWindowPage'))
const CustomOverlayWindowPage = lazy(() => import('./pages/CustomOverlayWindowPage'))
const ChatWindowPage = lazy(() => import('./pages/ChatWindowPage'))
const RollTrackerWindowPage = lazy(() => import('./pages/RollTrackerWindowPage'))
const RespawnTimerWindowPage = lazy(() => import('./pages/RespawnTimerWindowPage'))
const ZoneLockoutsWindowPage = lazy(() => import('./pages/ZoneLockoutsWindowPage'))
const RaidReadinessWindowPage = lazy(() => import('./pages/RaidReadinessWindowPage'))
const TriggerOverlayWindowPage = lazy(() => import('./pages/TriggerOverlayWindowPage'))
const NPCOverlayWindowPage = lazy(() => import('./pages/NPCOverlayWindowPage'))
const DiscordVoiceOverlayWindowPage = lazy(() => import('./pages/DiscordVoiceOverlayWindowPage'))
const ThreatOverlayWindowPage = lazy(() => import('./pages/ThreatOverlayWindowPage'))
const StackWindowPage = lazy(() => import('./pages/StackWindowPage'))

function OverlayPage({
  children,
  overlayKey,
}: {
  children: React.ReactNode
  // Canonical overlay name (see lib/overlays.ts); drives this window's own
  // per-overlay zoom. Omitted for overlays without a zoom control (trigger).
  overlayKey?: string
}): React.ReactElement {
  useEffect(() => {
    document.body.style.backgroundColor = 'transparent'
    return () => {
      // Only hand the opaque app background back when this window really has
      // navigated off the overlay route. A React tree can also unmount because
      // it threw during render — reverting there repaints the whole frameless
      // window solid #0a0a0a, which is the "overlay turned into a black box,
      // gone only after a restart" bug. The boundary below should now catch
      // those, but the window must never be able to go opaque either way.
      if (!window.location.hash.includes('-window')) {
        document.body.style.backgroundColor = ''
      }
    }
  }, [])
  // While main has this window hidden, render nothing: a hidden overlay kept
  // rendering live data and holding its WebSocket. Remounting on show reloads
  // the current state from the backend.
  const [paused, setPaused] = useState(false)
  useEffect(() => window.electron?.overlay?.onPausedChanged?.(setPaused), [])
  if (paused) return <></>
  // Overlay pages are lazy too; a null fallback keeps the window fully
  // transparent while the (small) chunk loads instead of flashing a backdrop.
  //
  // The boundary keeps one bad payload (an unexpected null in a WebSocket
  // target/timer push, say) from unmounting the window's entire tree — an
  // overlay has no navigation to recover through, so an uncaught render throw
  // left users with a dead window until they restarted the app.
  return (
    <>
      {overlayKey && <OverlayZoom overlayKey={overlayKey} />}
      <ErrorBoundary label={`${overlayKey ?? 'Overlay'} overlay`}>
        <Suspense fallback={null}>{children}</Suspense>
        <StackDropHint />
      </ErrorBoundary>
    </>
  )
}

// Applies this window's per-overlay zoom. Its own component, rendered only
// while the overlay isn't paused, because useOverlayZoom subscribes to the
// WebSocket for config:updated: mounted above the paused check it kept a
// hidden window's socket open (and parsing every broadcast) indefinitely.
// The CSS zoom it set stays on <html> while paused; remounting re-reads it.
function OverlayZoom({ overlayKey }: { overlayKey: string }): null {
  useOverlayZoom(overlayKey)
  return null
}

export default function App(): React.ReactElement {
  // Load the backend enums catalog (raw-code → label maps) once at app
  // boot so synchronous label helpers like tradeskillLabel() have data
  // available before any consumer renders. On fetch failure we still
  // proceed — helpers fall back to "Tradeskill 75"-style stubs.
  const [enumsReady, setEnumsReady] = useState(false)
  useEffect(() => {
    let cancelled = false
    loadEnums()
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setEnumsReady(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (!enumsReady) return <></>

  return (
    // useTransitions={false} makes route changes synchronous. React Router v7
    // wraps location updates in React.startTransition by default, but pages
    // built on an external store that can't be deferred (xyflow / Zustand —
    // the Schema Graph and PoP Flags graph) make that concurrent commit land
    // one navigation late: a sidebar click appears to do nothing until the
    // NEXT click, which then shows the previously-clicked page. Opting out
    // restores immediate, predictable navigation everywhere.
    <HashRouter useTransitions={false}>
      <Routes>
        {/* Standalone overlay windows — no sidebar/titlebar Layout */}
        <Route path="dps-overlay-window" element={<OverlayPage overlayKey="dps"><DPSOverlayWindowPage /></OverlayPage>} />
        <Route path="buff-timer-window" element={<OverlayPage overlayKey="buffTimer"><BuffTimerWindowPage /></OverlayPage>} />
        <Route path="filtered-timer-window" element={<OverlayPage overlayKey="filteredTimer"><FilteredTimerWindowPage /></OverlayPage>} />
        <Route path="kill-counter-window" element={<OverlayPage overlayKey="killCounter"><KillCounterWindowPage /></OverlayPage>} />
        <Route path="pet-window" element={<OverlayPage overlayKey="pet"><PetWindowPage /></OverlayPage>} />
        <Route path="death-recap-window" element={<OverlayPage overlayKey="deathRecap"><DeathRecapWindowPage /></OverlayPage>} />
        <Route path="gear-upgrades-window" element={<OverlayPage overlayKey="gearUpgrades"><GearUpgradesWindowPage /></OverlayPage>} />
        <Route path="item-info-window" element={<OverlayPage overlayKey="itemInfo"><ItemInfoWindowPage /></OverlayPage>} />
        <Route path="detrim-timer-window" element={<OverlayPage overlayKey="detrimTimer"><DetrimTimerWindowPage /></OverlayPage>} />
        <Route path="custom-timer-window" element={<OverlayPage overlayKey="customTimer"><CustomTimerWindowPage /></OverlayPage>} />
        <Route path="custom-overlay-window" element={<OverlayPage overlayKey="customOverlay"><CustomOverlayWindowPage /></OverlayPage>} />
        {/* No overlayKey: each chat window sizes its own text (font scale). */}
        <Route path="chat-window-window" element={<OverlayPage><ChatWindowPage /></OverlayPage>} />
        <Route path="trigger-overlay-window" element={<OverlayPage><TriggerOverlayWindowPage /></OverlayPage>} />
        <Route path="npc-overlay-window" element={<OverlayPage overlayKey="npc"><NPCOverlayWindowPage /></OverlayPage>} />
        <Route path="discord-voice-overlay-window" element={<OverlayPage overlayKey="discordVoice"><DiscordVoiceOverlayWindowPage /></OverlayPage>} />
        <Route path="threat-overlay-window" element={<OverlayPage overlayKey="threat"><ThreatOverlayWindowPage /></OverlayPage>} />
        <Route path="roll-tracker-window" element={<OverlayPage overlayKey="rollTracker"><RollTrackerWindowPage /></OverlayPage>} />
        <Route path="respawn-timer-window" element={<OverlayPage overlayKey="respawnTimer"><RespawnTimerWindowPage /></OverlayPage>} />
        <Route path="zone-lockouts-window" element={<OverlayPage overlayKey="zoneLockouts"><ZoneLockoutsWindowPage /></OverlayPage>} />
        <Route path="raid-readiness-window" element={<OverlayPage overlayKey="raidReadiness"><RaidReadinessWindowPage /></OverlayPage>} />
        <Route path="live-map-window" element={<OverlayPage overlayKey="liveMap"><LiveMapWindowPage /></OverlayPage>} />
        <Route path="ch-chain-window" element={<OverlayPage overlayKey="chChain"><CHChainOverlayWindowPage /></OverlayPage>} />
        <Route path="ch-metronome-window" element={<OverlayPage overlayKey="chMetronome"><CHMetronomeOverlayWindowPage /></OverlayPage>} />
        <Route path="stack-window" element={<OverlayPage><StackWindowPage /></OverlayPage>} />

        {/* Main window: sidebar Layout, pages, wizards, alert hooks. */}
        <Route
          path="/*"
          element={<Suspense fallback={null}><MainApp /></Suspense>}
        />
      </Routes>
    </HashRouter>
  )
}

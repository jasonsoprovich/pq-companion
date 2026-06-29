import React, { useEffect, useState, lazy, Suspense } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import OnboardingWizard from './components/OnboardingWizard'
import { getConfig } from './services/api'
import { loadEnums } from './lib/enumsCache'
import { useAudioEngine } from './hooks/useAudioEngine'
import { useTriggerClipboard } from './hooks/useTriggerClipboard'
import { useTimerAlerts } from './hooks/useTimerAlerts'
import { useRespawnAlerts } from './hooks/useRespawnAlerts'
import { useAudioPrefs } from './hooks/useAudioPrefs'
import { useHighContrast } from './hooks/useHighContrast'
import { useZoom } from './hooks/useZoom'
import { useOverlayZoom } from './hooks/useOverlayZoom'
import { useLogFeedSubscriber } from './hooks/useLogFeed'
// ItemsPage is the default landing route, so it stays in the initial bundle
// for an instant first paint. Every other page is code-split below so it loads
// on demand — this keeps the boot bundle small. The structural layout
// components (Layout/CombatLayout/CharactersLayout) also stay eager since the
// shell renders immediately.
import ItemsPage from './pages/ItemsPage'
import CombatLayout from './components/CombatLayout'
import CharactersLayout from './components/CharactersLayout'
import { DEV_HPS } from './lib/devFlags'
import { ActiveCharacterProvider } from './contexts/ActiveCharacterContext'
import { BackfillProvider } from './contexts/BackfillContext'

const SpellsPage = lazy(() => import('./pages/SpellsPage'))
const NpcsPage = lazy(() => import('./pages/NpcsPage'))
const ResistCalcPage = lazy(() => import('./pages/ResistCalcPage'))
const TraderTrackerPage = lazy(() => import('./pages/TraderTrackerPage'))
const CharmPetFinderPage = lazy(() => import('./pages/CharmPetFinderPage'))
const PoPFlaggingPage = lazy(() => import('./pages/PoPFlaggingPage'))
const ZonesPage = lazy(() => import('./pages/ZonesPage'))
const RecipesPage = lazy(() => import('./pages/RecipesPage'))
const QuestsPage = lazy(() => import('./pages/QuestsPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const InventoryPage = lazy(() => import('./pages/InventoryPage'))
const SpellChecklistPage = lazy(() => import('./pages/SpellChecklistPage'))
const InventoryTrackerPage = lazy(() => import('./pages/InventoryTrackerPage'))
const KeyTrackerPage = lazy(() => import('./pages/KeyTrackerPage'))
const LockoutTrackerPage = lazy(() => import('./pages/LockoutTrackerPage'))
const WishlistPage = lazy(() => import('./pages/WishlistPage'))
const LogFeedPage = lazy(() => import('./pages/LogFeedPage'))
const DPSOverlayWindowPage = lazy(() => import('./pages/DPSOverlayWindowPage'))
const HPSOverlayWindowPage = lazy(() => import('./pages/HPSOverlayWindowPage'))
const BuffTimerWindowPage = lazy(() => import('./pages/BuffTimerWindowPage'))
const CHChainOverlayWindowPage = lazy(() => import('./pages/CHChainOverlayWindowPage'))
const CHMetronomeOverlayWindowPage = lazy(() => import('./pages/CHMetronomeOverlayWindowPage'))
const DetrimTimerWindowPage = lazy(() => import('./pages/DetrimTimerWindowPage'))
const CustomTimerWindowPage = lazy(() => import('./pages/CustomTimerWindowPage'))
const OverlaysDashboard = lazy(() => import('./pages/OverlaysDashboard'))
const CombatLogPage = lazy(() => import('./pages/CombatLogPage'))
const CombatHistoryPage = lazy(() => import('./pages/CombatHistoryPage'))
const TriggersPage = lazy(() => import('./pages/TriggersPage'))
const RollTrackerPage = lazy(() => import('./pages/RollTrackerPage'))
const RollTrackerWindowPage = lazy(() => import('./pages/RollTrackerWindowPage'))
const RespawnTimerWindowPage = lazy(() => import('./pages/RespawnTimerWindowPage'))
const PlayersPage = lazy(() => import('./pages/PlayersPage'))
const ChatHistoryPage = lazy(() => import('./pages/ChatHistoryPage'))
const LootTrackerPage = lazy(() => import('./pages/LootTrackerPage'))
const TriggerOverlayWindowPage = lazy(() => import('./pages/TriggerOverlayWindowPage'))
const NPCOverlayWindowPage = lazy(() => import('./pages/NPCOverlayWindowPage'))
const ThreatOverlayWindowPage = lazy(() => import('./pages/ThreatOverlayWindowPage'))
const CharactersPage = lazy(() => import('./pages/CharactersPage'))
const CharacterProgressPage = lazy(() => import('./pages/CharacterProgressPage'))
const CharacterTasksPage = lazy(() => import('./pages/CharacterTasksPage'))
const GearUpgradeFinderPage = lazy(() => import('./pages/GearUpgradeFinderPage'))
const CharacterSpellsetsPage = lazy(() => import('./pages/CharacterSpellsetsPage'))

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
    return () => { document.body.style.backgroundColor = '' }
  }, [])
  // Apply this window's per-overlay zoom (no-op default of 1.0 when unset).
  // Called unconditionally with '' for keyless overlays so hook order is stable.
  useOverlayZoom(overlayKey ?? '')
  // Overlay pages are lazy too; a null fallback keeps the window fully
  // transparent while the (small) chunk loads instead of flashing a backdrop.
  return <Suspense fallback={null}>{children}</Suspense>
}

// MainWindowLayout mounts the audio/alert hooks and renders the main Layout.
// It is only rendered for the "/" route so overlay windows never run these hooks
// and never fire duplicate TTS or sound alerts.
function MainWindowLayout(): React.ReactElement {
  useAudioPrefs()
  useHighContrast()
  useZoom()
  useAudioEngine()
  useTriggerClipboard()
  useTimerAlerts()
  useRespawnAlerts()
  // Keep the Log Feed populating in the background so it persists across tab
  // navigation. Clearing only happens via the user's Trash button or restart.
  useLogFeedSubscriber()

  // 'unknown' until config has been loaded; then either true (skip wizard)
  // or false (show wizard before mounting the main Layout).
  const [onboardingDone, setOnboardingDone] = useState<boolean | 'unknown'>('unknown')

  useEffect(() => {
    getConfig()
      .then((c) => {
        setOnboardingDone(Boolean(c.onboarding_completed))
        // Mirror the persisted "Minimize to Tray" preference to the main
        // process so the close ('X') behaviour and tray icon match the saved
        // setting from the moment the app loads.
        void window.electron?.window?.setMinimizeToTray(
          Boolean(c.preferences?.minimize_to_tray)
        )
      })
      // If the backend is briefly unreachable on first launch, default to
      // showing the wizard rather than the main UI in an unconfigured state.
      .catch(() => setOnboardingDone(false))

    function handleReopen(): void {
      setOnboardingDone(false)
    }
    window.addEventListener('pq:open-onboarding', handleReopen)
    return () => window.removeEventListener('pq:open-onboarding', handleReopen)
  }, [])

  if (onboardingDone === 'unknown') return <></>

  return (
    <ActiveCharacterProvider>
      <BackfillProvider>
        {onboardingDone === false && (
          <OnboardingWizard
            allowCancel
            onCancel={() => setOnboardingDone(true)}
            onComplete={() => setOnboardingDone(true)}
          />
        )}
        <Layout />
      </BackfillProvider>
    </ActiveCharacterProvider>
  )
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
        {DEV_HPS && <Route path="hps-overlay-window" element={<OverlayPage overlayKey="hps"><HPSOverlayWindowPage /></OverlayPage>} />}
        <Route path="buff-timer-window" element={<OverlayPage overlayKey="buffTimer"><BuffTimerWindowPage /></OverlayPage>} />
        <Route path="detrim-timer-window" element={<OverlayPage overlayKey="detrimTimer"><DetrimTimerWindowPage /></OverlayPage>} />
        <Route path="custom-timer-window" element={<OverlayPage overlayKey="customTimer"><CustomTimerWindowPage /></OverlayPage>} />
        <Route path="trigger-overlay-window" element={<OverlayPage><TriggerOverlayWindowPage /></OverlayPage>} />
        <Route path="npc-overlay-window" element={<OverlayPage overlayKey="npc"><NPCOverlayWindowPage /></OverlayPage>} />
        <Route path="threat-overlay-window" element={<OverlayPage overlayKey="threat"><ThreatOverlayWindowPage /></OverlayPage>} />
        <Route path="roll-tracker-window" element={<OverlayPage overlayKey="rollTracker"><RollTrackerWindowPage /></OverlayPage>} />
        <Route path="respawn-timer-window" element={<OverlayPage overlayKey="respawnTimer"><RespawnTimerWindowPage /></OverlayPage>} />
        <Route path="ch-chain-window" element={<OverlayPage overlayKey="chChain"><CHChainOverlayWindowPage /></OverlayPage>} />
        <Route path="ch-metronome-window" element={<OverlayPage overlayKey="chMetronome"><CHMetronomeOverlayWindowPage /></OverlayPage>} />

        {/* Main app routes — wrapped in full Layout with audio/alert hooks */}
        <Route path="/" element={<MainWindowLayout />}>
          <Route index element={<Navigate to="/items" replace />} />
          <Route path="items" element={<ItemsPage />} />
          <Route path="spells" element={<SpellsPage />} />
          <Route path="npcs" element={<NpcsPage />} />
          <Route path="resist-calc" element={<ResistCalcPage />} />
          <Route path="trader-tracker" element={<TraderTrackerPage />} />
          <Route path="charm-pet-finder" element={<CharmPetFinderPage />} />
          <Route path="pop-flags" element={<PoPFlaggingPage />} />
          <Route path="zones" element={<ZonesPage />} />
          <Route path="recipes" element={<RecipesPage />} />
          <Route path="quests" element={<QuestsPage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="backup-manager" element={<Navigate to="/settings" replace />} />
          <Route path="log-feed" element={<LogFeedPage />} />
          <Route path="overlays" element={<OverlaysDashboard />} />
          <Route path="overlays/npc" element={<Navigate to="/overlays" replace />} />
          <Route path="overlays/dps" element={<Navigate to="/overlays" replace />} />
          <Route path="overlays/timers" element={<Navigate to="/overlays" replace />} />
          <Route path="npc-overlay" element={<Navigate to="/overlays" replace />} />
          <Route path="dps-overlay" element={<Navigate to="/overlays" replace />} />
          <Route path="spell-timers" element={<Navigate to="/overlays" replace />} />
          <Route path="combat" element={<CombatLayout />}>
            <Route index element={<Navigate to="/combat/log" replace />} />
            <Route path="log" element={<CombatLogPage />} />
            <Route path="history" element={<CombatHistoryPage />} />
          </Route>
          <Route path="combat-log" element={<Navigate to="/combat/log" replace />} />
          <Route path="combat-history" element={<Navigate to="/combat/history" replace />} />
          <Route path="triggers" element={<TriggersPage />} />
          <Route path="rolls" element={<RollTrackerPage />} />
          <Route path="players" element={<PlayersPage />} />
          <Route path="chat" element={<ChatHistoryPage />} />
          <Route path="loot" element={<LootTrackerPage />} />
          <Route path="characters" element={<CharactersLayout />}>
            <Route index element={<Navigate to="/characters/overview" replace />} />
            <Route path="overview" element={<CharactersPage />} />
            <Route path="progress" element={<CharacterProgressPage />} />
            <Route path="inventory" element={<InventoryTrackerPage />} />
            <Route path="spells" element={<SpellChecklistPage />} />
            <Route path="spellsets" element={<CharacterSpellsetsPage />} />
            <Route path="keys" element={<KeyTrackerPage />} />
            <Route path="lockouts" element={<LockoutTrackerPage />} />
            <Route path="wishlist" element={<WishlistPage />} />
            <Route path="upgrades" element={<GearUpgradeFinderPage />} />
            <Route path="tasks" element={<CharacterTasksPage />} />
          </Route>
          <Route path="character-progress" element={<Navigate to="/characters/progress" replace />} />
          <Route path="inventory-tracker" element={<Navigate to="/characters/inventory" replace />} />
          <Route path="spell-checklist" element={<Navigate to="/characters/spells" replace />} />
          <Route path="key-tracker" element={<Navigate to="/characters/keys" replace />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}

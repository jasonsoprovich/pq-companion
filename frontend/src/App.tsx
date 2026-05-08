import React, { useEffect, useState } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import OnboardingWizard from './components/OnboardingWizard'
import { getConfig } from './services/api'
import { useAudioEngine } from './hooks/useAudioEngine'
import { useTimerAlerts } from './hooks/useTimerAlerts'
import { useEventAlerts } from './hooks/useEventAlerts'
import { useMasterVolume } from './hooks/useMasterVolume'
import { useLogFeedSubscriber } from './hooks/useLogFeed'
import ItemsPage from './pages/ItemsPage'
import SpellsPage from './pages/SpellsPage'
import NpcsPage from './pages/NpcsPage'
import ZonesPage from './pages/ZonesPage'
import SettingsPage from './pages/SettingsPage'
import InventoryPage from './pages/InventoryPage'
import SpellChecklistPage from './pages/SpellChecklistPage'
import InventoryTrackerPage from './pages/InventoryTrackerPage'
import KeyTrackerPage from './pages/KeyTrackerPage'
import LogFeedPage from './pages/LogFeedPage'
import DPSOverlayWindowPage from './pages/DPSOverlayWindowPage'
import HPSOverlayWindowPage from './pages/HPSOverlayWindowPage'
import { DEV_HPS } from './lib/devFlags'
import BuffTimerWindowPage from './pages/BuffTimerWindowPage'
import DetrimTimerWindowPage from './pages/DetrimTimerWindowPage'
import OverlaysDashboard from './pages/OverlaysDashboard'
import CombatLogPage from './pages/CombatLogPage'
import TriggersPage from './pages/TriggersPage'
import TriggerOverlayWindowPage from './pages/TriggerOverlayWindowPage'
import NPCOverlayWindowPage from './pages/NPCOverlayWindowPage'
import CharactersPage from './pages/CharactersPage'
import CharacterProgressPage from './pages/CharacterProgressPage'
import CharacterTasksPage from './pages/CharacterTasksPage'
import CharactersLayout from './components/CharactersLayout'
import { ActiveCharacterProvider } from './contexts/ActiveCharacterContext'

function OverlayPage({ children }: { children: React.ReactNode }): React.ReactElement {
  useEffect(() => {
    document.body.style.backgroundColor = 'transparent'
    return () => { document.body.style.backgroundColor = '' }
  }, [])
  return <>{children}</>
}

// MainWindowLayout mounts the audio/alert hooks and renders the main Layout.
// It is only rendered for the "/" route so overlay windows never run these hooks
// and never fire duplicate TTS or sound alerts.
function MainWindowLayout(): React.ReactElement {
  useMasterVolume()
  useAudioEngine()
  useTimerAlerts()
  useEventAlerts()
  // Keep the Log Feed populating in the background so it persists across tab
  // navigation. Clearing only happens via the user's Trash button or restart.
  useLogFeedSubscriber()

  // 'unknown' until config has been loaded; then either true (skip wizard)
  // or false (show wizard before mounting the main Layout).
  const [onboardingDone, setOnboardingDone] = useState<boolean | 'unknown'>('unknown')

  useEffect(() => {
    getConfig()
      .then((c) => setOnboardingDone(Boolean(c.onboarding_completed)))
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
      {onboardingDone === false && (
        <OnboardingWizard
          allowCancel
          onCancel={() => setOnboardingDone(true)}
          onComplete={() => setOnboardingDone(true)}
        />
      )}
      <Layout />
    </ActiveCharacterProvider>
  )
}

export default function App(): React.ReactElement {
  return (
    <HashRouter>
      <Routes>
        {/* Standalone overlay windows — no sidebar/titlebar Layout */}
        <Route path="dps-overlay-window" element={<OverlayPage><DPSOverlayWindowPage /></OverlayPage>} />
        {DEV_HPS && <Route path="hps-overlay-window" element={<OverlayPage><HPSOverlayWindowPage /></OverlayPage>} />}
        <Route path="buff-timer-window" element={<OverlayPage><BuffTimerWindowPage /></OverlayPage>} />
        <Route path="detrim-timer-window" element={<OverlayPage><DetrimTimerWindowPage /></OverlayPage>} />
        <Route path="trigger-overlay-window" element={<OverlayPage><TriggerOverlayWindowPage /></OverlayPage>} />
        <Route path="npc-overlay-window" element={<OverlayPage><NPCOverlayWindowPage /></OverlayPage>} />

        {/* Main app routes — wrapped in full Layout with audio/alert hooks */}
        <Route path="/" element={<MainWindowLayout />}>
          <Route index element={<Navigate to="/items" replace />} />
          <Route path="items" element={<ItemsPage />} />
          <Route path="spells" element={<SpellsPage />} />
          <Route path="npcs" element={<NpcsPage />} />
          <Route path="zones" element={<ZonesPage />} />
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
          <Route path="combat-log" element={<CombatLogPage />} />
          <Route path="triggers" element={<TriggersPage />} />
          <Route path="characters" element={<CharactersLayout />}>
            <Route index element={<Navigate to="/characters/overview" replace />} />
            <Route path="overview" element={<CharactersPage />} />
            <Route path="progress" element={<CharacterProgressPage />} />
            <Route path="inventory" element={<InventoryTrackerPage />} />
            <Route path="spells" element={<SpellChecklistPage />} />
            <Route path="keys" element={<KeyTrackerPage />} />
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

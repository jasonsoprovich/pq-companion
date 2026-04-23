import React, { useEffect } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import { useAudioEngine } from './hooks/useAudioEngine'
import { useTimerAlerts } from './hooks/useTimerAlerts'
import { useEventAlerts } from './hooks/useEventAlerts'
import ItemsPage from './pages/ItemsPage'
import SpellsPage from './pages/SpellsPage'
import NpcsPage from './pages/NpcsPage'
import ZonesPage from './pages/ZonesPage'
import SettingsPage from './pages/SettingsPage'
import InventoryPage from './pages/InventoryPage'
import SpellChecklistPage from './pages/SpellChecklistPage'
import InventoryTrackerPage from './pages/InventoryTrackerPage'
import KeyTrackerPage from './pages/KeyTrackerPage'
import BackupManagerPage from './pages/BackupManagerPage'
import LogFeedPage from './pages/LogFeedPage'
import NPCOverlayPage from './pages/NPCOverlayPage'
import DPSOverlayPage from './pages/DPSOverlayPage'
import DPSOverlayWindowPage from './pages/DPSOverlayWindowPage'
import HPSOverlayWindowPage from './pages/HPSOverlayWindowPage'
import { DEV_HPS } from './lib/devFlags'
import BuffTimerWindowPage from './pages/BuffTimerWindowPage'
import DetrimTimerWindowPage from './pages/DetrimTimerWindowPage'
import SpellTimerPage from './pages/SpellTimerPage'
import CombatLogPage from './pages/CombatLogPage'
import TriggersPage from './pages/TriggersPage'
import TriggerOverlayWindowPage from './pages/TriggerOverlayWindowPage'
import NPCOverlayWindowPage from './pages/NPCOverlayWindowPage'
import CharactersPage from './pages/CharactersPage'

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
  useAudioEngine()
  useTimerAlerts()
  useEventAlerts()
  return <Layout />
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
          <Route path="inventory-tracker" element={<InventoryTrackerPage />} />
          <Route path="spell-checklist" element={<SpellChecklistPage />} />
          <Route path="key-tracker" element={<KeyTrackerPage />} />
          <Route path="backup-manager" element={<BackupManagerPage />} />
          <Route path="log-feed" element={<LogFeedPage />} />
          <Route path="npc-overlay" element={<NPCOverlayPage />} />
          <Route path="dps-overlay" element={<DPSOverlayPage />} />
          <Route path="spell-timers" element={<SpellTimerPage />} />
          <Route path="combat-log" element={<CombatLogPage />} />
          <Route path="triggers" element={<TriggersPage />} />
          <Route path="characters" element={<CharactersPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}

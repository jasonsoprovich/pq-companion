import React from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import { useAudioEngine } from './hooks/useAudioEngine'
import { useTimerAlerts } from './hooks/useTimerAlerts'
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
import BuffTimerWindowPage from './pages/BuffTimerWindowPage'
import DetrimTimerWindowPage from './pages/DetrimTimerWindowPage'
import SpellTimerPage from './pages/SpellTimerPage'
import CombatLogPage from './pages/CombatLogPage'
import TriggersPage from './pages/TriggersPage'
import TriggerOverlayWindowPage from './pages/TriggerOverlayWindowPage'

export default function App(): React.ReactElement {
  useAudioEngine()
  useTimerAlerts()

  return (
    <HashRouter>
      <Routes>
        {/* Standalone overlay windows — no sidebar/titlebar Layout */}
        <Route path="dps-overlay-window" element={<DPSOverlayWindowPage />} />
        <Route path="hps-overlay-window" element={<HPSOverlayWindowPage />} />
        <Route path="buff-timer-window" element={<BuffTimerWindowPage />} />
        <Route path="detrim-timer-window" element={<DetrimTimerWindowPage />} />
        <Route path="trigger-overlay-window" element={<TriggerOverlayWindowPage />} />

        {/* Main app routes — wrapped in full Layout */}
        <Route path="/" element={<Layout />}>
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
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}

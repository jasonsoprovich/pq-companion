import React from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import ItemsPage from './pages/ItemsPage'
import SpellsPage from './pages/SpellsPage'
import NpcsPage from './pages/NpcsPage'
import ZonesPage from './pages/ZonesPage'
import SettingsPage from './pages/SettingsPage'
import InventoryPage from './pages/InventoryPage'
import SpellChecklistPage from './pages/SpellChecklistPage'
import InventoryTrackerPage from './pages/InventoryTrackerPage'

export default function App(): React.ReactElement {
  return (
    <HashRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/items" replace />} />
          <Route path="items" element={<ItemsPage />} />
          <Route path="spells" element={<SpellsPage />} />
          <Route path="npcs" element={<NpcsPage />} />
          <Route path="zones" element={<ZonesPage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="inventory-tracker" element={<InventoryTrackerPage />} />
          <Route path="spell-checklist" element={<SpellChecklistPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}

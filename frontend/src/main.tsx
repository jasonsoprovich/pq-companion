import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'
import { restorePendingLocalStorage } from './services/clientState'

async function bootstrap(): Promise<void> {
  // A staged App Backup/Restore import writes its localStorage half to a
  // pending file that only this (post-relaunch) process can consume — pull
  // it in before mounting so every localStorage-backed hook reads the
  // restored values on its very first render. Reloading once is cheap
  // insurance against any module that reads a key at import time rather than
  // at mount; take-pending only ever returns data once (the main process
  // deletes its pending file on the first read), so this can't loop.
  const restored = await restorePendingLocalStorage()
  if (restored) {
    location.reload()
    return
  }

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}

// Overlays are shown once and stay up, so Chromium marking one hidden means
// it has stopped painting it (e.g. native occlusion tracking deciding it's
// covered) — the "overlays went blank" reports. console.warn reaches
// electron.log, which timestamps the edge. The trigger overlay is natively
// hidden between alerts, so its edges are noise and skipped.
function logOverlayVisibility(): void {
  const route = window.location.hash
  if (!route.includes('-window') || route.includes('trigger-overlay-window')) {
    return
  }
  document.addEventListener('visibilitychange', () => {
    console.warn(
      `[overlay-visibility] ${route} ${document.visibilityState}`,
    )
  })
}

logOverlayVisibility()
void bootstrap()

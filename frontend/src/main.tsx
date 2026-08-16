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

void bootstrap()

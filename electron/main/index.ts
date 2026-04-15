import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog } from 'electron'
import { join } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { existsSync } from 'fs'
import { autoUpdater } from 'electron-updater'

const isDev = !app.isPackaged

let mainWindow: BrowserWindow | null = null
let dpsOverlayWindow: BrowserWindow | null = null
let hpsOverlayWindow: BrowserWindow | null = null
let buffTimerWindow: BrowserWindow | null = null
let detrimTimerWindow: BrowserWindow | null = null
let triggerOverlayWindow: BrowserWindow | null = null
let sidecarProcess: ChildProcess | null = null

// ── Sidecar (Go backend) lifecycle ────────────────────────────────────────────

function getSidecarPath(): string | null {
  if (isDev) {
    // In dev the Go server is started separately with `go run ./cmd/server`
    return null
  }
  const ext = process.platform === 'win32' ? '.exe' : ''
  const candidate = join(process.resourcesPath, 'bin', `pq-companion-server${ext}`)
  return existsSync(candidate) ? candidate : null
}

function startSidecar(): void {
  const sidecarPath = getSidecarPath()
  if (!sidecarPath) {
    console.log('[main] Sidecar not found — assuming backend is running separately in dev mode')
    return
  }

  sidecarProcess = spawn(sidecarPath, [], { stdio: 'pipe' })

  sidecarProcess.stdout?.on('data', (data: Buffer) => {
    process.stdout.write(`[backend] ${data.toString()}`)
  })

  sidecarProcess.stderr?.on('data', (data: Buffer) => {
    process.stderr.write(`[backend:err] ${data.toString()}`)
  })

  sidecarProcess.on('exit', (code) => {
    console.log(`[main] Backend exited with code ${code}`)
    sidecarProcess = null
  })

  console.log(`[main] Backend sidecar started (pid ${sidecarProcess.pid})`)
}

function stopSidecar(): void {
  if (sidecarProcess) {
    console.log('[main] Stopping backend sidecar…')
    sidecarProcess.kill()
    sidecarProcess = null
  }
}

// ── Auto-updater ──────────────────────────────────────────────────────────────

function setupAutoUpdater(): void {
  if (isDev) return // not applicable outside a packaged build

  autoUpdater.autoDownload = true
  autoUpdater.autoInstallOnAppQuit = true

  autoUpdater.on('checking-for-update', () => {
    console.log('[updater] Checking for updates…')
  })

  autoUpdater.on('update-available', (info) => {
    console.log(`[updater] Update available: ${info.version}`)
    mainWindow?.webContents.send('updater:available', { version: info.version })
  })

  autoUpdater.on('update-not-available', () => {
    console.log('[updater] Already up to date')
  })

  autoUpdater.on('download-progress', (progress) => {
    mainWindow?.webContents.send('updater:progress', {
      percent: Math.floor(progress.percent),
      transferred: progress.transferred,
      total: progress.total,
    })
  })

  autoUpdater.on('update-downloaded', (info) => {
    console.log(`[updater] Update downloaded: ${info.version}`)
    mainWindow?.webContents.send('updater:downloaded', { version: info.version })
  })

  autoUpdater.on('error', (err) => {
    console.error('[updater] Error:', err.message)
    mainWindow?.webContents.send('updater:error', err.message)
  })

  // Delay first check so the app finishes launching before hitting the network.
  setTimeout(() => autoUpdater.checkForUpdates(), 5_000)
}

// ── Window management ─────────────────────────────────────────────────────────

function createMainWindow(): void {
  nativeTheme.themeSource = 'dark'

  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: '#0a0a0a',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'hidden',
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
    show: false // show after ready-to-show to avoid flash
  })

  mainWindow.once('ready-to-show', () => {
    mainWindow?.show()
  })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    mainWindow.loadURL(rendererUrl)
    mainWindow.webContents.openDevTools({ mode: 'detach' })
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }

  // Open external links in the system browser, not in Electron
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })
}

// ── DPS Overlay window ────────────────────────────────────────────────────────

function createDPSOverlay(): void {
  if (dpsOverlayWindow && !dpsOverlayWindow.isDestroyed()) {
    dpsOverlayWindow.focus()
    return
  }

  dpsOverlayWindow = new BrowserWindow({
    width: 420,
    height: 460,
    minWidth: 260,
    minHeight: 180,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  // Keep it above fullscreen apps on macOS/Windows.
  dpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  dpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    dpsOverlayWindow.loadURL(`${rendererUrl}/#/dps-overlay-window`)
  } else {
    dpsOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/dps-overlay-window',
    })
  }

  dpsOverlayWindow.on('closed', () => {
    dpsOverlayWindow = null
  })
}

// ── HPS Overlay window ────────────────────────────────────────────────────────

function createHPSOverlay(): void {
  if (hpsOverlayWindow && !hpsOverlayWindow.isDestroyed()) {
    hpsOverlayWindow.focus()
    return
  }

  hpsOverlayWindow = new BrowserWindow({
    width: 420,
    height: 460,
    minWidth: 260,
    minHeight: 180,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  hpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  hpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    hpsOverlayWindow.loadURL(`${rendererUrl}/#/hps-overlay-window`)
  } else {
    hpsOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/hps-overlay-window',
    })
  }

  hpsOverlayWindow.on('closed', () => {
    hpsOverlayWindow = null
  })
}

// ── Buff Timer overlay window ─────────────────────────────────────────────────

function createBuffTimerOverlay(): void {
  if (buffTimerWindow && !buffTimerWindow.isDestroyed()) {
    buffTimerWindow.focus()
    return
  }

  buffTimerWindow = new BrowserWindow({
    width: 280,
    height: 380,
    minWidth: 200,
    minHeight: 140,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  buffTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  buffTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    buffTimerWindow.loadURL(`${rendererUrl}/#/buff-timer-window`)
  } else {
    buffTimerWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/buff-timer-window',
    })
  }

  buffTimerWindow.on('closed', () => {
    buffTimerWindow = null
  })
}

// ── Detrimental Timer overlay window ─────────────────────────────────────────

function createDetrimTimerOverlay(): void {
  if (detrimTimerWindow && !detrimTimerWindow.isDestroyed()) {
    detrimTimerWindow.focus()
    return
  }

  detrimTimerWindow = new BrowserWindow({
    width: 300,
    height: 320,
    minWidth: 200,
    minHeight: 140,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  detrimTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  detrimTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    detrimTimerWindow.loadURL(`${rendererUrl}/#/detrim-timer-window`)
  } else {
    detrimTimerWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/detrim-timer-window',
    })
  }

  detrimTimerWindow.on('closed', () => {
    detrimTimerWindow = null
  })
}

// ── Trigger Overlay window ────────────────────────────────────────────────────

function createTriggerOverlay(): void {
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) {
    triggerOverlayWindow.focus()
    return
  }

  triggerOverlayWindow = new BrowserWindow({
    width: 340,
    height: 360,
    minWidth: 240,
    minHeight: 100,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  triggerOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  triggerOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    triggerOverlayWindow.loadURL(`${rendererUrl}/#/trigger-overlay-window`)
  } else {
    triggerOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/trigger-overlay-window',
    })
  }

  triggerOverlayWindow.on('closed', () => {
    triggerOverlayWindow = null
  })
}

// ── IPC handlers — window controls ───────────────────────────────────────────

ipcMain.handle('window:minimize', () => mainWindow?.minimize())
ipcMain.handle('window:maximize', () => {
  if (mainWindow?.isMaximized()) {
    mainWindow.unmaximize()
  } else {
    mainWindow?.maximize()
  }
})
ipcMain.handle('window:close', () => mainWindow?.close())
ipcMain.handle('window:is-maximized', () => mainWindow?.isMaximized() ?? false)

// ── IPC handlers — overlay windows ───────────────────────────────────────────

ipcMain.handle('overlay:dps:open', () => {
  createDPSOverlay()
})

ipcMain.handle('overlay:dps:close', () => {
  if (dpsOverlayWindow && !dpsOverlayWindow.isDestroyed()) {
    dpsOverlayWindow.close()
  }
})

ipcMain.handle('overlay:dps:toggle', () => {
  if (dpsOverlayWindow && !dpsOverlayWindow.isDestroyed()) {
    dpsOverlayWindow.close()
  } else {
    createDPSOverlay()
  }
})

ipcMain.handle('overlay:hps:open', () => {
  createHPSOverlay()
})

ipcMain.handle('overlay:hps:close', () => {
  if (hpsOverlayWindow && !hpsOverlayWindow.isDestroyed()) {
    hpsOverlayWindow.close()
  }
})

ipcMain.handle('overlay:hps:toggle', () => {
  if (hpsOverlayWindow && !hpsOverlayWindow.isDestroyed()) {
    hpsOverlayWindow.close()
  } else {
    createHPSOverlay()
  }
})

ipcMain.handle('overlay:bufftimer:open', () => {
  createBuffTimerOverlay()
})

ipcMain.handle('overlay:bufftimer:close', () => {
  if (buffTimerWindow && !buffTimerWindow.isDestroyed()) {
    buffTimerWindow.close()
  }
})

ipcMain.handle('overlay:bufftimer:toggle', () => {
  if (buffTimerWindow && !buffTimerWindow.isDestroyed()) {
    buffTimerWindow.close()
  } else {
    createBuffTimerOverlay()
  }
})

ipcMain.handle('overlay:detrimtimer:open', () => {
  createDetrimTimerOverlay()
})

ipcMain.handle('overlay:detrimtimer:close', () => {
  if (detrimTimerWindow && !detrimTimerWindow.isDestroyed()) {
    detrimTimerWindow.close()
  }
})

ipcMain.handle('overlay:detrimtimer:toggle', () => {
  if (detrimTimerWindow && !detrimTimerWindow.isDestroyed()) {
    detrimTimerWindow.close()
  } else {
    createDetrimTimerOverlay()
  }
})

ipcMain.handle('overlay:trigger:open', () => {
  createTriggerOverlay()
})

ipcMain.handle('overlay:trigger:close', () => {
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) {
    triggerOverlayWindow.close()
  }
})

ipcMain.handle('overlay:trigger:toggle', () => {
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) {
    triggerOverlayWindow.close()
  } else {
    createTriggerOverlay()
  }
})

// ── IPC handlers — dialogs ────────────────────────────────────────────────────

ipcMain.handle('dialog:select-folder', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openDirectory'],
    title: 'Select EverQuest Installation Folder',
  })
  return result.canceled ? null : result.filePaths[0]
})

// ── IPC handlers — auto-updater ───────────────────────────────────────────────

ipcMain.handle('updater:check', () => {
  if (!isDev) autoUpdater.checkForUpdates()
})

ipcMain.handle('updater:quit-and-install', () => {
  autoUpdater.quitAndInstall()
})

// ── App lifecycle ─────────────────────────────────────────────────────────────

app.whenReady().then(() => {
  startSidecar()
  createMainWindow()
  setupAutoUpdater()

  // macOS: re-create window when dock icon is clicked and no windows are open
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createMainWindow()
    }
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    stopSidecar()
    app.quit()
  }
})

app.on('before-quit', () => {
  stopSidecar()
})

import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog } from 'electron'
import { join } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { existsSync } from 'fs'

const isDev = !app.isPackaged

let mainWindow: BrowserWindow | null = null
let dpsOverlayWindow: BrowserWindow | null = null
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

// ── IPC handlers — dialogs ────────────────────────────────────────────────────

ipcMain.handle('dialog:select-folder', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openDirectory'],
    title: 'Select EverQuest Installation Folder',
  })
  return result.canceled ? null : result.filePaths[0]
})

// ── App lifecycle ─────────────────────────────────────────────────────────────

app.whenReady().then(() => {
  startSidecar()
  createMainWindow()

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

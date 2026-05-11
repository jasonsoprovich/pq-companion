import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog, screen, protocol } from 'electron'
import { join, extname } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { existsSync, readFileSync, writeFileSync } from 'fs'
import { readFile } from 'fs/promises'
import { autoUpdater } from 'electron-updater'

const isDev = !app.isPackaged

// pq-audio:// is a custom protocol used by the renderer's Audio elements to
// load arbitrary local sound files (trigger sounds, alert sounds, etc.) under
// the default webSecurity:true sandbox. file:// is blocked from non-file://
// origins, so we map pq-audio:///<absolute-path> to the file system here.
// Must be registered as privileged BEFORE app.ready so HTMLMediaElement can
// stream from it.
protocol.registerSchemesAsPrivileged([
  {
    scheme: 'pq-audio',
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      stream: true,
      bypassCSP: true,
    },
  },
])

// Allow trigger sounds and TTS to play before the user has clicked anywhere
// in the window. Without this Chromium silently blocks Audio.play() and
// SpeechSynthesisUtterance for the rest of the session if the very first
// alert lands before the first user gesture — exactly when audio matters
// most (combat starts before the user touches the companion app). Must be
// set before app is ready.
app.commandLine.appendSwitch('autoplay-policy', 'no-user-gesture-required')

// Resolve the app icon for runtime BrowserWindow use.
// In packaged Windows builds the exe already embeds build/icon.ico via
// electron-builder, so the taskbar/title-bar icon comes from there. We only
// need an explicit icon path during `npm run dev` so the dev taskbar entry
// doesn't fall back to the default Electron icon.
const appIconPath = isDev
  ? join(__dirname, '../../build/icon.png')
  : join(process.resourcesPath, 'icon.png')

// MIME type for the pq-audio:// handler. Chromium needs a Content-Type that
// matches a media type it can decode; without this it silently rejects the
// response and Audio.play() never starts.
function audioMimeType(ext: string): string {
  switch (ext) {
    case '.mp3':
      return 'audio/mpeg'
    case '.wav':
      return 'audio/wav'
    case '.ogg':
    case '.oga':
      return 'audio/ogg'
    case '.m4a':
    case '.mp4':
      return 'audio/mp4'
    case '.aac':
      return 'audio/aac'
    case '.flac':
      return 'audio/flac'
    case '.opus':
      return 'audio/opus'
    case '.webm':
      return 'audio/webm'
    default:
      return 'application/octet-stream'
  }
}

// ── Overlay bounds persistence ────────────────────────────────────────────────

type OverlayName = 'dps' | 'hps' | 'buffTimer' | 'detrimTimer' | 'trigger' | 'npc' | 'rollTracker'
type Bounds = { x: number; y: number; width: number; height: number }

type BoundsStore = Partial<Record<OverlayName, Bounds>>
type LockStore = Partial<Record<OverlayName, boolean>>

function boundsFilePath(): string {
  return join(app.getPath('userData'), 'overlayBounds.json')
}

function lockFilePath(): string {
  return join(app.getPath('userData'), 'overlayLockState.json')
}

function loadBoundsStore(): BoundsStore {
  try {
    const raw = readFileSync(boundsFilePath(), 'utf8')
    return JSON.parse(raw) as BoundsStore
  } catch {
    return {}
  }
}

function saveBoundsStore(store: BoundsStore): void {
  try {
    writeFileSync(boundsFilePath(), JSON.stringify(store, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write overlay bounds:', err)
  }
}

function loadLockStore(): LockStore {
  try {
    const raw = readFileSync(lockFilePath(), 'utf8')
    return JSON.parse(raw) as LockStore
  } catch {
    return {}
  }
}

function saveLockStore(store: LockStore): void {
  try {
    writeFileSync(lockFilePath(), JSON.stringify(store, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write overlay lock state:', err)
  }
}

function getOverlayLocked(name: OverlayName): boolean {
  return loadLockStore()[name] === true
}

function setOverlayLocked(name: OverlayName, locked: boolean): void {
  const store = loadLockStore()
  store[name] = locked
  saveLockStore(store)
}

// Map a BrowserWindow back to its overlay name so IPC handlers can look up
// which overlay is sending lock state changes.
const windowToOverlayName = new WeakMap<BrowserWindow, OverlayName>()

const boundsDebounceTimers = new Map<OverlayName, ReturnType<typeof setTimeout>>()

function persistBounds(name: OverlayName, win: BrowserWindow): void {
  const existing = boundsDebounceTimers.get(name)
  if (existing) clearTimeout(existing)
  boundsDebounceTimers.set(
    name,
    setTimeout(() => {
      if (win.isDestroyed()) return
      const b = win.getBounds()
      const store = loadBoundsStore()
      store[name] = { x: b.x, y: b.y, width: b.width, height: b.height }
      saveBoundsStore(store)
      boundsDebounceTimers.delete(name)
    }, 500),
  )
}

function isOnScreen(bounds: Bounds): boolean {
  const displays = screen.getAllDisplays()
  return displays.some((d) => {
    const wa = d.workArea
    return (
      bounds.x < wa.x + wa.width &&
      bounds.x + bounds.width > wa.x &&
      bounds.y < wa.y + wa.height &&
      bounds.y + bounds.height > wa.y
    )
  })
}

function getRestoredBounds(name: OverlayName, defaults: Bounds): Bounds {
  const store = loadBoundsStore()
  const saved = store[name]
  if (saved && isOnScreen(saved)) return saved
  return defaults
}

function trackOverlayBounds(name: OverlayName, win: BrowserWindow): void {
  win.on('move', () => persistBounds(name, win))
  win.on('resize', () => persistBounds(name, win))
  win.on('close', () => persistBounds(name, win))
}

let mainWindow: BrowserWindow | null = null
let dpsOverlayWindow: BrowserWindow | null = null
let hpsOverlayWindow: BrowserWindow | null = null
let buffTimerWindow: BrowserWindow | null = null
let detrimTimerWindow: BrowserWindow | null = null
let triggerOverlayWindow: BrowserWindow | null = null
let npcOverlayWindow: BrowserWindow | null = null
let rollTrackerWindow: BrowserWindow | null = null
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

function stopSidecar(): Promise<void> {
  const proc = sidecarProcess
  if (!proc || proc.exitCode !== null || !proc.pid) {
    sidecarProcess = null
    return Promise.resolve()
  }

  const pid = proc.pid
  sidecarProcess = null
  console.log(`[main] Stopping backend sidecar (pid ${pid})…`)

  const exited = new Promise<void>((resolve) => {
    if (proc.exitCode !== null) {
      resolve()
      return
    }
    proc.once('exit', () => resolve())
  })

  if (process.platform === 'win32') {
    // child.kill() on Windows is unreliable for cross-compiled Go binaries:
    // it can leave the process alive, holding file locks on the installed
    // .exe. That blocks the NSIS installer/uninstaller and auto-updater
    // ("application is still open"). taskkill /F /T terminates the process
    // tree synchronously at the OS level.
    spawn('taskkill', ['/F', '/T', '/PID', String(pid)], { stdio: 'ignore' })
      .on('error', (err) => {
        console.error('[main] taskkill failed:', err.message)
        try { proc.kill('SIGKILL') } catch { /* already gone */ }
      })
  } else {
    proc.kill('SIGTERM')
  }

  // Bound the wait — if the child really is wedged we still let Electron exit
  // rather than hang forever.
  return Promise.race([
    exited,
    new Promise<void>((resolve) => setTimeout(resolve, 5000)),
  ])
}

// ── Auto-updater ──────────────────────────────────────────────────────────────

function setupAutoUpdater(): void {
  if (isDev) return // not applicable outside a packaged build

  autoUpdater.autoDownload = false
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

function closeAllOverlays(): void {
  for (const win of [dpsOverlayWindow, hpsOverlayWindow, buffTimerWindow, detrimTimerWindow, triggerOverlayWindow, npcOverlayWindow, rollTrackerWindow]) {
    if (win && !win.isDestroyed()) win.destroy()
  }
}

function createMainWindow(): void {
  nativeTheme.themeSource = 'dark'

  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: '#0a0a0a',
    icon: appIconPath,
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
    closeAllOverlays()
    mainWindow = null
  })
}

// ── DPS Overlay window ────────────────────────────────────────────────────────

function createDPSOverlay(): void {
  if (dpsOverlayWindow && !dpsOverlayWindow.isDestroyed()) {
    dpsOverlayWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('dps', { x: 0, y: 0, width: 420, height: 460 })
  dpsOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
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
  windowToOverlayName.set(dpsOverlayWindow, 'dps')
  if (getOverlayLocked('dps')) {
    dpsOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('dps', dpsOverlayWindow)

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

  const { x, y, width, height } = getRestoredBounds('hps', { x: 0, y: 0, width: 420, height: 460 })
  hpsOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
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
  windowToOverlayName.set(hpsOverlayWindow, 'hps')
  if (getOverlayLocked('hps')) {
    hpsOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('hps', hpsOverlayWindow)

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

  const { x, y, width, height } = getRestoredBounds('buffTimer', { x: 0, y: 0, width: 280, height: 380 })
  buffTimerWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
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
  windowToOverlayName.set(buffTimerWindow, 'buffTimer')
  if (getOverlayLocked('buffTimer')) {
    buffTimerWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('buffTimer', buffTimerWindow)

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

  const { x, y, width, height } = getRestoredBounds('detrimTimer', { x: 0, y: 0, width: 300, height: 320 })
  detrimTimerWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
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
  windowToOverlayName.set(detrimTimerWindow, 'detrimTimer')
  if (getOverlayLocked('detrimTimer')) {
    detrimTimerWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('detrimTimer', detrimTimerWindow)

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

  // The trigger overlay is the invisible click-through canvas that real-fire
  // alerts and the positioning test card render into. Default to the full
  // primary work area so trigger text can be pinned anywhere on screen — the
  // window has no chrome, isn't visible outside a positioning session, and
  // the renderer toggles per-region click-through over just the test card,
  // so a screen-spanning size never blocks the underlying app/game.
  const primary = screen.getPrimaryDisplay().workArea
  const triggerDefaults = {
    x: primary.x,
    y: primary.y,
    width: primary.width,
    height: primary.height,
  }
  // One-time migration: clear out saved bounds from the old chrome-bearing
  // positioning model — the centered 90%-of-screen box and the even older
  // 340×360 popup. The new full-workArea default replaces both.
  const store = loadBoundsStore()
  if (store.trigger) {
    const old90W = Math.round(primary.width * 0.9)
    const old90H = Math.round(primary.height * 0.9)
    const isOld90 = store.trigger.width === old90W && store.trigger.height === old90H
    const isOldPopup = store.trigger.width === 340 && store.trigger.height === 360
    if (isOld90 || isOldPopup) {
      delete store.trigger
      saveBoundsStore(store)
    }
  }
  const { x, y, width, height } = getRestoredBounds('trigger', triggerDefaults)
  triggerOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
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
  windowToOverlayName.set(triggerOverlayWindow, 'trigger')
  // Default the trigger overlay to click-through so the chrome-less canvas
  // doesn't intercept clicks intended for the game underneath. The header
  // (only shown while positioning) re-enables interaction on hover.
  if (!getOverlayLocked('trigger')) {
    setOverlayLocked('trigger', true)
  }
  triggerOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
  trackOverlayBounds('trigger', triggerOverlayWindow)

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

// ── NPC Overlay window ────────────────────────────────────────────────────────

function createNPCOverlay(): void {
  if (npcOverlayWindow && !npcOverlayWindow.isDestroyed()) {
    npcOverlayWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('npc', { x: 0, y: 0, width: 360, height: 480 })
  npcOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 280,
    minHeight: 200,
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

  npcOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  npcOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(npcOverlayWindow, 'npc')
  if (getOverlayLocked('npc')) {
    npcOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('npc', npcOverlayWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    npcOverlayWindow.loadURL(`${rendererUrl}/#/npc-overlay-window`)
  } else {
    npcOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/npc-overlay-window',
    })
  }

  npcOverlayWindow.on('closed', () => {
    npcOverlayWindow = null
  })
}

// ── Roll Tracker overlay window ──────────────────────────────────────────────

function createRollTrackerOverlay(): void {
  if (rollTrackerWindow && !rollTrackerWindow.isDestroyed()) {
    rollTrackerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('rollTracker', { x: 0, y: 0, width: 320, height: 360 })
  rollTrackerWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 240,
    minHeight: 160,
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

  rollTrackerWindow.setAlwaysOnTop(true, 'screen-saver')
  rollTrackerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(rollTrackerWindow, 'rollTracker')
  if (getOverlayLocked('rollTracker')) {
    rollTrackerWindow.setIgnoreMouseEvents(true, { forward: true })
  }
  trackOverlayBounds('rollTracker', rollTrackerWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    rollTrackerWindow.loadURL(`${rendererUrl}/#/roll-tracker-window`)
  } else {
    rollTrackerWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/roll-tracker-window',
    })
  }

  rollTrackerWindow.on('closed', () => {
    rollTrackerWindow = null
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

ipcMain.handle('overlay:npc:open', () => {
  createNPCOverlay()
})

ipcMain.handle('overlay:npc:close', () => {
  if (npcOverlayWindow && !npcOverlayWindow.isDestroyed()) {
    npcOverlayWindow.close()
  }
})

ipcMain.handle('overlay:npc:toggle', () => {
  if (npcOverlayWindow && !npcOverlayWindow.isDestroyed()) {
    npcOverlayWindow.close()
  } else {
    createNPCOverlay()
  }
})

ipcMain.handle('overlay:rolltracker:open', () => {
  createRollTrackerOverlay()
})

ipcMain.handle('overlay:rolltracker:close', () => {
  if (rollTrackerWindow && !rollTrackerWindow.isDestroyed()) {
    rollTrackerWindow.close()
  }
})

ipcMain.handle('overlay:rolltracker:toggle', () => {
  if (rollTrackerWindow && !rollTrackerWindow.isDestroyed()) {
    rollTrackerWindow.close()
  } else {
    createRollTrackerOverlay()
  }
})

// ── IPC handlers — bulk popout control ───────────────────────────────────────

function popoutWindows(): BrowserWindow[] {
  return [
    dpsOverlayWindow,
    hpsOverlayWindow,
    buffTimerWindow,
    detrimTimerWindow,
    triggerOverlayWindow,
    npcOverlayWindow,
    rollTrackerWindow,
  ].filter((w): w is BrowserWindow => !!w && !w.isDestroyed())
}

ipcMain.handle('overlay:popouts:any-open', () => popoutWindows().length > 0)

ipcMain.handle('overlay:popouts:open-all', () => {
  if (!dpsOverlayWindow || dpsOverlayWindow.isDestroyed()) createDPSOverlay()
  if (!buffTimerWindow || buffTimerWindow.isDestroyed()) createBuffTimerOverlay()
  if (!detrimTimerWindow || detrimTimerWindow.isDestroyed()) createDetrimTimerOverlay()
  if (!npcOverlayWindow || npcOverlayWindow.isDestroyed()) createNPCOverlay()
  if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) createTriggerOverlay()
  if (!rollTrackerWindow || rollTrackerWindow.isDestroyed()) createRollTrackerOverlay()
})

ipcMain.handle('overlay:popouts:close-all', () => {
  // Use close() (not destroy()) so each window's 'close' handler persists its bounds.
  for (const win of popoutWindows()) win.close()
})

// ── IPC handlers — click-through ─────────────────────────────────────────────

ipcMain.handle('overlay:set-ignore-mouse-events', (event, ignore: boolean) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  win?.setIgnoreMouseEvents(ignore, { forward: true })
})

// ── IPC handlers — overlay lock state ────────────────────────────────────────

ipcMain.handle('overlay:lock:get', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win) return false
  const name = windowToOverlayName.get(win)
  return name ? getOverlayLocked(name) : false
})

ipcMain.handle('overlay:lock:set', (event, locked: boolean) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win) return
  const name = windowToOverlayName.get(win)
  if (!name) return
  setOverlayLocked(name, locked)
  win.setIgnoreMouseEvents(locked, { forward: true })
})

// ── IPC handlers — dialogs ────────────────────────────────────────────────────

ipcMain.handle('dialog:select-folder', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openDirectory'],
    title: 'Select EverQuest Installation Folder',
  })
  return result.canceled ? null : result.filePaths[0]
})

ipcMain.handle('dialog:select-sound-file', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Select Sound File',
    filters: [
      { name: 'Audio Files', extensions: ['wav', 'mp3', 'ogg', 'flac', 'm4a', 'aac', 'opus'] },
      { name: 'All Files', extensions: ['*'] },
    ],
  })
  return result.canceled ? null : result.filePaths[0]
})

// ── IPC handlers — auto-updater ───────────────────────────────────────────────

ipcMain.handle('app:version', () => app.getVersion())

ipcMain.handle('updater:check', () => {
  if (!isDev) autoUpdater.checkForUpdates()
})

ipcMain.handle('updater:download', () => {
  if (!isDev) autoUpdater.downloadUpdate()
})

ipcMain.handle('updater:quit-and-install', async () => {
  // Kill the sidecar before handing off to the updater. quitAndInstall exits
  // the main process quickly; if the sidecar is still alive the NSIS updater
  // cannot replace pq-companion-server.exe and the install wedges.
  await stopSidecar()
  autoUpdater.quitAndInstall(true, true)
})

// ── App lifecycle ─────────────────────────────────────────────────────────────

app.whenReady().then(() => {
  // Map pq-audio:///<absolute-path> to a local file. The leading `/` after the
  // scheme is the empty host part, so on macOS pq-audio:///Users/x/foo.wav
  // resolves to /Users/x/foo.wav, and on Windows pq-audio:///C:/x/foo.wav
  // resolves to C:/x/foo.wav (we strip the host slash before the drive letter).
  protocol.handle('pq-audio', async (request) => {
    // The renderer formats URLs as pq-audio://local/<absolute-path>. We pull
    // the path back out via URL.pathname (which keeps it percent-encoded and
    // case-preserved) and decode it once.
    let p: string
    try {
      p = decodeURIComponent(new URL(request.url).pathname)
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[pq-audio] bad url', { url: request.url, err: String(err) })
      return new Response('bad url', { status: 400 })
    }
    if (process.platform === 'win32' && /^\/[a-zA-Z]:/.test(p)) {
      p = p.substring(1)
    }
    try {
      const data = await readFile(p)
      const mime = audioMimeType(extname(p).toLowerCase())
      // eslint-disable-next-line no-console
      console.log('[pq-audio] served', { path: p, mime, bytes: data.byteLength })
      return new Response(data, { headers: { 'Content-Type': mime } })
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[pq-audio] failed to read', { path: p, err: String(err) })
      return new Response('not found', { status: 404 })
    }
  })

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

let isGracefulQuit = false

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

app.on('before-quit', (event) => {
  if (isGracefulQuit || !sidecarProcess) return
  // Electron only waits for before-quit synchronously. To stop the sidecar
  // and confirm exit before we really quit, cancel this pass, run the async
  // cleanup, then quit again with the flag set.
  event.preventDefault()
  isGracefulQuit = true
  stopSidecar().finally(() => app.quit())
})

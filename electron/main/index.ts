import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog, screen, protocol } from 'electron'
import { join, extname, dirname } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { existsSync, readFileSync, writeFileSync, statSync } from 'fs'
import { readFile, access, open as fsOpen } from 'fs/promises'
import { constants as fsConstants } from 'fs'
import { homedir } from 'os'
import { autoUpdater } from 'electron-updater'
import { appendLine, closeLogger, initLogger } from './logger'

const isDev = !app.isPackaged

// Initialize file logging as early as possible so any errors during the
// rest of main-process bootstrap are captured. Packaged Windows builds
// have no attached console — without this we lose every console.* call,
// every [backend] line piped from the sidecar, and every unhandledRejection.
const loggerInit = initLogger(app.getVersion())
if (loggerInit.error) {
  console.warn(`[main] file logging disabled: ${loggerInit.error.message}`)
} else if (loggerInit.logPath) {
  console.log(`[main] electron log: ${loggerInit.logPath}`)
}
app.on('before-quit', () => closeLogger())

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

type OverlayName = 'dps' | 'hps' | 'buffTimer' | 'detrimTimer' | 'trigger' | 'npc' | 'rollTracker' | 'respawnTimer' | 'chChain'
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

// virtualDesktopBounds returns the rectangle that covers every connected
// display (the full virtual desktop). Used to size the trigger overlay so its
// notification text can be positioned on any monitor. Uses full display bounds
// (not workArea) so text can sit anywhere, including over taskbars.
function virtualDesktopBounds(): Bounds {
  const displays = screen.getAllDisplays()
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const d of displays) {
    const b = d.bounds
    minX = Math.min(minX, b.x)
    minY = Math.min(minY, b.y)
    maxX = Math.max(maxX, b.x + b.width)
    maxY = Math.max(maxY, b.y + b.height)
  }
  if (!Number.isFinite(minX)) {
    // No displays reported (shouldn't happen); fall back to the primary.
    const p = screen.getPrimaryDisplay().bounds
    return { x: p.x, y: p.y, width: p.width, height: p.height }
  }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
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

// ── Main window state persistence ─────────────────────────────────────────────
// The main window gets its own state file (separate from overlayBounds.json)
// because it also tracks a `maximized` flag, which Bounds can't express. We
// restore the pre-maximize size/position AND the maximized state so reopening
// lands exactly where the user left it — including on a second monitor, since
// isOnScreen() validates against every display.

type MainWindowState = { bounds?: Bounds; maximized?: boolean }

function mainWindowStateFilePath(): string {
  return join(app.getPath('userData'), 'mainWindowState.json')
}

function loadMainWindowState(): MainWindowState {
  try {
    return JSON.parse(readFileSync(mainWindowStateFilePath(), 'utf8')) as MainWindowState
  } catch {
    return {}
  }
}

function saveMainWindowState(state: MainWindowState): void {
  try {
    writeFileSync(mainWindowStateFilePath(), JSON.stringify(state, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write main window state:', err)
  }
}

let mainStateDebounce: ReturnType<typeof setTimeout> | null = null

// Persist the main window's normal (non-maximized) bounds plus its maximized
// flag. While maximized we deliberately keep the previously-saved bounds so
// the restore size survives — getBounds() during maximize returns the
// maximized rect, which we don't want to treat as the restore size.
function persistMainWindowState(win: BrowserWindow): void {
  if (mainStateDebounce) clearTimeout(mainStateDebounce)
  mainStateDebounce = setTimeout(() => {
    mainStateDebounce = null
    if (win.isDestroyed()) return
    const state = loadMainWindowState()
    state.maximized = win.isMaximized()
    if (!state.maximized) {
      const b = win.getBounds()
      state.bounds = { x: b.x, y: b.y, width: b.width, height: b.height }
    }
    saveMainWindowState(state)
  }, 500)
}

function trackMainWindowBounds(win: BrowserWindow): void {
  win.on('move', () => persistMainWindowState(win))
  win.on('resize', () => persistMainWindowState(win))
  win.on('maximize', () => persistMainWindowState(win))
  win.on('unmaximize', () => persistMainWindowState(win))
  win.on('close', () => persistMainWindowState(win))
}

let mainWindow: BrowserWindow | null = null
let dpsOverlayWindow: BrowserWindow | null = null
let hpsOverlayWindow: BrowserWindow | null = null
let buffTimerWindow: BrowserWindow | null = null
let chChainWindow: BrowserWindow | null = null
let detrimTimerWindow: BrowserWindow | null = null
let triggerOverlayWindow: BrowserWindow | null = null
let npcOverlayWindow: BrowserWindow | null = null
let rollTrackerWindow: BrowserWindow | null = null
let respawnTimerWindow: BrowserWindow | null = null
let sidecarProcess: ChildProcess | null = null

// Backend port is discovered at runtime: the Go sidecar tries its preferred
// port first, falls back to an OS-assigned port if busy, then prints
// `BACKEND_PORT=N` to stdout. We capture it here and resolve any pending
// renderer IPC calls. In dev (`npm run dev`) there's no sidecar — the
// developer runs the backend on its default port — so resolve to 17654
// after a short grace period if no BACKEND_PORT line arrives.
let backendPort: number | null = null
let backendPortResolvers: Array<(port: number) => void> = []
const DEV_FALLBACK_PORT = 17654

function setBackendPort(port: number): void {
  if (backendPort === port) return
  backendPort = port
  console.log(`[main] Backend port resolved: ${port}`)
  const pending = backendPortResolvers
  backendPortResolvers = []
  pending.forEach((r) => r(port))
}

function getBackendPort(): Promise<number> {
  if (backendPort !== null) return Promise.resolve(backendPort)
  return new Promise<number>((resolve) => {
    backendPortResolvers.push(resolve)
  })
}

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

// Resolve where the bundled EQ database should live in a packaged build.
// Mirrors the `extraResources` mapping in electron-builder.yml (bin/data/quarm.db).
// Returns null in dev — the Go server is run separately and points at backend/data/.
function getQuarmDbPath(): string | null {
  if (isDev) return null
  return join(process.resourcesPath, 'bin', 'data', 'quarm.db')
}

// probeDbReadable checks that quarm.db exists AND is openable for read.
// Returns ok:true with stats on success, or ok:false with a one-line reason
// suitable for logging. We open and read a tiny prefix because some failure
// modes (OneDrive Files-on-Demand placeholders, AV ACL holds) pass stat()
// but fail at first read.
async function probeDbReadable(
  path: string,
): Promise<{ ok: true; reason: string } | { ok: false; reason: string }> {
  try {
    const st = statSync(path)
    if (!st.isFile()) return { ok: false, reason: 'path is not a regular file' }
    if (st.size === 0) return { ok: false, reason: 'file is 0 bytes (truncated)' }
    await access(path, fsConstants.R_OK)
    const fh = await fsOpen(path, 'r')
    try {
      const buf = Buffer.alloc(16)
      await fh.read(buf, 0, 16, 0)
      // SQLite files start with "SQLite format 3\0". A non-matching header
      // means the file was clobbered (partial AV scrub, bad download, etc.).
      const header = buf.toString('utf8', 0, 15)
      if (header !== 'SQLite format 3') {
        return { ok: false, reason: `bad SQLite header: ${JSON.stringify(header)}` }
      }
    } finally {
      await fh.close()
    }
    return { ok: true, reason: `${st.size} bytes, mtime ${st.mtime.toISOString()}` }
  } catch (err) {
    const e = err as NodeJS.ErrnoException
    return { ok: false, reason: `${e.code || 'ERR'}: ${e.message}` }
  }
}

// Show a blocking error dialog when quarm.db is missing OR unreadable.
// The file is bundled in resources/bin/data/, so the most common causes are
// (1) an AV silently quarantining or ACL-locking it post-install, (2) a
// OneDrive Files-on-Demand placeholder that hasn't been hydrated, or (3) a
// partial download leaving a 0-byte stub. We guide the user through manual
// recovery rather than auto-downloading. Returns when the user picks "Quit".
async function showDbMissingDialog(expectedPath: string): Promise<void> {
  const downloadUrl =
    'https://github.com/jasonsoprovich/pq-companion/releases/tag/data-latest'
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const { response } = await dialog.showMessageBox({
      type: 'error',
      title: 'Game database missing or unreadable',
      message: "PQ Companion can't read the game database (quarm.db).",
      detail:
        `The database file should be located at:\n${expectedPath}\n\n` +
        'The file is either missing, empty, or your system is blocking ' +
        'this app from opening it. Common causes:\n\n' +
        '1. Antivirus quarantine or ACL hold. Open Windows Security → ' +
        'Virus & threat protection → Protection history and look for ' +
        '"quarm.db" or "pq-companion-server.exe". Restore the file and ' +
        'add an exclusion for the install folder, then relaunch.\n\n' +
        '2. OneDrive Files-on-Demand. If the install folder lives under ' +
        'OneDrive, right-click the install folder and choose "Always ' +
        'keep on this device" so the .db file is actually downloaded.\n\n' +
        '3. Manual restore. Download quarm.db from the "data-latest" ' +
        'GitHub release and drop it into the folder shown above (create ' +
        'the data folder if it does not exist). The file should be ' +
        'around 84 MB — anything noticeably smaller is incomplete.\n\n' +
        'Relaunch PQ Companion once the file is in place. See ' +
        '~/.pq-companion/logs/electron.log for the exact failure reason.',
      buttons: ['Quit', 'Open install folder', 'Open download page'],
      defaultId: 0,
      cancelId: 0,
      noLink: true,
    })
    if (response === 1) {
      await shell.openPath(dirname(expectedPath))
    } else if (response === 2) {
      await shell.openExternal(downloadUrl)
    } else {
      return
    }
  }
}

// resolveDevBackendPort polls ~/.pq-companion/server-port (written by the Go
// server on bind) so the dev renderer learns whichever port the standalone
// `go run` chose — which is whatever the user's config.yaml says, not the
// hardcoded fallback. Polls for up to 8 s so we tolerate the user starting
// Electron before `go run` is fully up. Falls back to DEV_FALLBACK_PORT only
// if the file genuinely never appears, with a loud console warning.
async function resolveDevBackendPort(): Promise<void> {
  const portFile = join(homedir(), '.pq-companion', 'server-port')
  const deadline = Date.now() + 8000
  while (Date.now() < deadline) {
    try {
      const text = await readFile(portFile, 'utf8')
      const port = Number(text.trim())
      if (Number.isFinite(port) && port > 0 && port < 65536) {
        console.log(`[main] Dev backend port discovered from ${portFile}: ${port}`)
        setBackendPort(port)
        return
      }
    } catch {
      // file not written yet — keep polling
    }
    await new Promise((r) => setTimeout(r, 200))
  }
  console.warn(
    `[main] Dev port discovery timed out (${portFile} not found / unreadable). ` +
      `Falling back to ${DEV_FALLBACK_PORT}. Make sure the Go backend is running.`,
  )
  setBackendPort(DEV_FALLBACK_PORT)
}

function startSidecar(): void {
  const sidecarPath = getSidecarPath()
  if (!sidecarPath) {
    console.log('[main] Sidecar not found — dev mode; discovering backend port from file')
    void resolveDevBackendPort()
    return
  }

  sidecarProcess = spawn(sidecarPath, [], {
    stdio: 'pipe',
    env: {
      ...process.env,
      // Pass the app version through so manifest.json in exported .pqcb
      // bundles is stamped with the producing version. Read at startup by
      // runtimeAppVersion() in cmd/server/main.go.
      PQ_APP_VERSION: app.getVersion(),
    },
  })

  // Buffer for parsing the BACKEND_PORT=N line. The line may arrive in the
  // middle of a chunk so we accumulate until we see a newline.
  let stdoutBuf = ''
  sidecarProcess.stdout?.on('data', (data: Buffer) => {
    const chunk = data.toString()
    process.stdout.write(`[backend] ${chunk}`)
    // Tee directly via appendLine instead of console.log so the file
    // keeps the sidecar's own ISO timestamps and level markers intact
    // (rather than wrapping every line in another [main] prefix).
    appendLine('BACKEND', chunk)
    if (backendPort === null) {
      stdoutBuf += chunk
      const match = stdoutBuf.match(/BACKEND_PORT=(\d+)/)
      if (match) {
        const port = Number(match[1])
        if (Number.isFinite(port) && port > 0 && port < 65536) {
          setBackendPort(port)
        }
        stdoutBuf = '' // line consumed
      } else if (stdoutBuf.length > 4096) {
        // Don't grow unbounded if something is wrong upstream.
        stdoutBuf = stdoutBuf.slice(-1024)
      }
    }
  })

  sidecarProcess.stderr?.on('data', (data: Buffer) => {
    const chunk = data.toString()
    process.stderr.write(`[backend:err] ${chunk}`)
    appendLine('BACKEND-ERR', chunk)
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
  for (const win of [dpsOverlayWindow, hpsOverlayWindow, buffTimerWindow, detrimTimerWindow, triggerOverlayWindow, npcOverlayWindow, rollTrackerWindow, respawnTimerWindow, chChainWindow]) {
    if (win && !win.isDestroyed()) win.destroy()
  }
}

function createMainWindow(): void {
  nativeTheme.themeSource = 'dark'

  // Restore the last-used size/position when it still falls on a connected
  // display; otherwise fall back to the first-run default. isOnScreen()
  // checks every display, so a window left on a second monitor reopens there.
  const savedState = loadMainWindowState()
  const restored = savedState.bounds && isOnScreen(savedState.bounds) ? savedState.bounds : null

  mainWindow = new BrowserWindow({
    width: restored?.width ?? 1280,
    height: restored?.height ?? 860,
    ...(restored ? { x: restored.x, y: restored.y } : {}),
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
    if (savedState.maximized) mainWindow?.maximize()
    mainWindow?.show()
  })

  trackMainWindowBounds(mainWindow)

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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  dpsOverlayWindow.once('ready-to-show', () => {
    dpsOverlayWindow?.show()
  })

  // Keep it above fullscreen apps on macOS/Windows.
  dpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  dpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(dpsOverlayWindow, 'dps')
  if (getOverlayLocked('dps')) {
    dpsOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
    dpsOverlayWindow.setResizable(false)
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  hpsOverlayWindow.once('ready-to-show', () => {
    hpsOverlayWindow?.show()
  })

  hpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  hpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(hpsOverlayWindow, 'hps')
  if (getOverlayLocked('hps')) {
    hpsOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
    hpsOverlayWindow.setResizable(false)
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  buffTimerWindow.once('ready-to-show', () => {
    buffTimerWindow?.show()
  })

  buffTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  buffTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(buffTimerWindow, 'buffTimer')
  if (getOverlayLocked('buffTimer')) {
    buffTimerWindow.setIgnoreMouseEvents(true, { forward: true })
    buffTimerWindow.setResizable(false)
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

function createCHChainOverlay(): void {
  if (chChainWindow && !chChainWindow.isDestroyed()) {
    chChainWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('chChain', { x: 0, y: 0, width: 260, height: 320 })
  chChainWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 180,
    minHeight: 120,
    transparent: true,
    backgroundColor: '#00000000',
    frame: false,
    resizable: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  chChainWindow.once('ready-to-show', () => {
    chChainWindow?.show()
  })

  chChainWindow.setAlwaysOnTop(true, 'screen-saver')
  chChainWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(chChainWindow, 'chChain')
  if (getOverlayLocked('chChain')) {
    chChainWindow.setIgnoreMouseEvents(true, { forward: true })
    chChainWindow.setResizable(false)
  }
  trackOverlayBounds('chChain', chChainWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    chChainWindow.loadURL(`${rendererUrl}/#/ch-chain-window`)
  } else {
    chChainWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/ch-chain-window',
    })
  }

  chChainWindow.on('closed', () => {
    chChainWindow = null
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  detrimTimerWindow.once('ready-to-show', () => {
    detrimTimerWindow?.show()
  })

  detrimTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  detrimTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(detrimTimerWindow, 'detrimTimer')
  if (getOverlayLocked('detrimTimer')) {
    detrimTimerWindow.setIgnoreMouseEvents(true, { forward: true })
    detrimTimerWindow.setResizable(false)
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

// ── Respawn (death) Timer overlay window ─────────────────────────────────────

function createRespawnTimerOverlay(): void {
  if (respawnTimerWindow && !respawnTimerWindow.isDestroyed()) {
    respawnTimerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('respawnTimer', { x: 0, y: 0, width: 300, height: 320 })
  respawnTimerWindow = new BrowserWindow({
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  respawnTimerWindow.once('ready-to-show', () => {
    respawnTimerWindow?.show()
  })

  respawnTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  respawnTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(respawnTimerWindow, 'respawnTimer')
  if (getOverlayLocked('respawnTimer')) {
    respawnTimerWindow.setIgnoreMouseEvents(true, { forward: true })
    respawnTimerWindow.setResizable(false)
  }
  trackOverlayBounds('respawnTimer', respawnTimerWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    respawnTimerWindow.loadURL(`${rendererUrl}/#/respawn-timer-window`)
  } else {
    respawnTimerWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/respawn-timer-window',
    })
  }

  respawnTimerWindow.on('closed', () => {
    respawnTimerWindow = null
  })
}

// ── Trigger Overlay window ────────────────────────────────────────────────────

function createTriggerOverlay(): void {
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) {
    triggerOverlayWindow.focus()
    return
  }

  // The trigger overlay is the invisible click-through canvas that real-fire
  // alerts and the positioning test card render into. It spans the entire
  // virtual desktop (every monitor) so notification text can be positioned on
  // any display. The window has no chrome, isn't visible outside a positioning
  // session, and the renderer toggles click-through over just the test card,
  // so a desktop-spanning size never blocks the underlying app/game.
  //
  // Because it must always cover the full desktop, we do NOT persist or restore
  // its bounds (unlike the other overlays) — they're recomputed from the
  // current display layout every time. Drop any stale saved bounds left over
  // from older single-display versions.
  const store = loadBoundsStore()
  if (store.trigger) {
    delete store.trigger
    saveBoundsStore(store)
  }
  const { x, y, width, height } = virtualDesktopBounds()
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  // Do NOT auto-show on ready-to-show. The trigger overlay is hidden whenever
  // it's idle (no positioning session, no live alert) and only shown on demand
  // by the renderer via overlay:trigger:set-mode. A hidden window cannot
  // capture mouse input, which is the only fully reliable way to guarantee the
  // desktop-spanning overlay never locks the app out — setIgnoreMouseEvents
  // alone proved unreliable on some multi-monitor Windows setups.

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
  // Note: no trackOverlayBounds here — the trigger overlay always spans the
  // full virtual desktop and is resized on display changes instead.

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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  npcOverlayWindow.once('ready-to-show', () => {
    npcOverlayWindow?.show()
  })

  npcOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  npcOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(npcOverlayWindow, 'npc')
  if (getOverlayLocked('npc')) {
    npcOverlayWindow.setIgnoreMouseEvents(true, { forward: true })
    npcOverlayWindow.setResizable(false)
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
    show: false, // show after ready-to-show to avoid blank-frame flash
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
    },
  })

  rollTrackerWindow.once('ready-to-show', () => {
    rollTrackerWindow?.show()
  })

  rollTrackerWindow.setAlwaysOnTop(true, 'screen-saver')
  rollTrackerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(rollTrackerWindow, 'rollTracker')
  if (getOverlayLocked('rollTracker')) {
    rollTrackerWindow.setIgnoreMouseEvents(true, { forward: true })
    rollTrackerWindow.setResizable(false)
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

// Scales the whole main-window UI like a browser zoom (issue #130). Clamped to
// a sane range so the UI can't be zoomed into uselessness. Applied to the
// calling window's webContents so it works regardless of which window asks.
ipcMain.handle('window:set-zoom', (event, factor: number) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win || win.isDestroyed()) return
  const clamped = Math.max(0.7, Math.min(2.0, factor || 1))
  win.webContents.setZoomFactor(clamped)
})

// ── IPC handlers — synthetic window dragging ─────────────────────────────────
// Frameless/custom-titlebar windows using CSS `-webkit-app-region: drag`
// cannot be dragged across monitor boundaries on Windows — Chromium clamps the
// drag to the monitor it started on. We replace that with a main-process drag
// loop driven by the global cursor position, which is free to span the whole
// virtual desktop. The renderer signals drag start/end on the title bar.
const dragLoops = new WeakMap<BrowserWindow, ReturnType<typeof setInterval>>()

function stopDrag(win: BrowserWindow): void {
  const loop = dragLoops.get(win)
  if (loop) {
    clearInterval(loop)
    dragLoops.delete(win)
  }
}

ipcMain.handle('window:drag:start', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win || win.isDestroyed() || win.isMaximized()) return
  stopDrag(win) // clear any prior loop (e.g. a missed drag:end)
  const start = win.getBounds()
  const cursorStart = screen.getCursorScreenPoint()
  // Free the window's own 'closed' handler from a dangling loop.
  win.once('closed', () => stopDrag(win))
  const loop = setInterval(() => {
    if (win.isDestroyed()) {
      stopDrag(win)
      return
    }
    const cur = screen.getCursorScreenPoint()
    win.setBounds({
      x: start.x + (cur.x - cursorStart.x),
      y: start.y + (cur.y - cursorStart.y),
      width: start.width,
      height: start.height,
    })
  }, 16)
  dragLoops.set(win, loop)
})

ipcMain.handle('window:drag:end', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (win) stopDrag(win)
})

// Returns the center of the primary display expressed in coordinates local to
// the trigger overlay window (which spans the whole virtual desktop). The
// trigger positioner uses this as its default spawn point so the test card
// appears on the primary monitor rather than at the seam between displays.
ipcMain.handle('screen:trigger-default-center', () => {
  const v = virtualDesktopBounds()
  const p = screen.getPrimaryDisplay().bounds
  return {
    x: Math.round(p.x - v.x + p.width / 2),
    y: Math.round(p.y - v.y + p.height / 2),
  }
})

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

ipcMain.handle('overlay:chchain:open', () => {
  createCHChainOverlay()
})

ipcMain.handle('overlay:chchain:close', () => {
  if (chChainWindow && !chChainWindow.isDestroyed()) {
    chChainWindow.close()
  }
})

ipcMain.handle('overlay:chchain:toggle', () => {
  if (chChainWindow && !chChainWindow.isDestroyed()) {
    chChainWindow.close()
  } else {
    createCHChainOverlay()
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

ipcMain.handle('overlay:respawntimer:open', () => {
  createRespawnTimerOverlay()
})

ipcMain.handle('overlay:respawntimer:close', () => {
  if (respawnTimerWindow && !respawnTimerWindow.isDestroyed()) {
    respawnTimerWindow.close()
  }
})

ipcMain.handle('overlay:respawntimer:toggle', () => {
  if (respawnTimerWindow && !respawnTimerWindow.isDestroyed()) {
    respawnTimerWindow.close()
  } else {
    createRespawnTimerOverlay()
  }
})

ipcMain.handle('overlay:trigger:open', () => {
  createTriggerOverlay()
})

// Drives the trigger overlay's visibility + input behaviour from the renderer,
// which knows whether a positioning session or a live alert is active:
//   'interactive' — positioning: visible and capturing mouse (card is draggable)
//   'passthrough' — live alert only: visible but click-through (text overlay)
//   'hidden'      — idle: hidden entirely so it can never capture input
// Hiding when idle is the authoritative lockout fix; click-through alone was
// unreliable on some setups.
ipcMain.handle('overlay:trigger:set-mode', (_event, mode: 'interactive' | 'passthrough' | 'hidden') => {
  const win = triggerOverlayWindow
  if (!win || win.isDestroyed()) return
  if (mode === 'interactive') {
    win.setIgnoreMouseEvents(false)
    if (!win.isVisible()) win.showInactive()
    return
  }
  if (mode === 'passthrough') {
    win.setIgnoreMouseEvents(true, { forward: true })
    if (!win.isVisible()) win.showInactive()
    return
  }
  // hidden
  win.setIgnoreMouseEvents(true, { forward: true })
  // Only hand focus back to the main window if the overlay actually HELD focus
  // — i.e. an interactive positioning session the user clicked into. A live
  // alert is shown with showInactive() and never takes focus, so hiding it when
  // it expires must NOT pull focus off the game window.
  const overlayHadFocus = win.isFocused()
  if (win.isVisible()) win.hide()
  if (overlayHadFocus && mainWindow && !mainWindow.isDestroyed()) mainWindow.focus()
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
    respawnTimerWindow,
    chChainWindow,
  ].filter((w): w is BrowserWindow => !!w && !w.isDestroyed())
}

ipcMain.handle('overlay:popouts:any-open', () => popoutWindows().length > 0)

ipcMain.handle('overlay:popouts:open-all', (_event, panels?: string[]) => {
  // `panels` is the set of dashboard panel keys the user has toggled visible
  // (buff/detrim/dps/npc/rolls/respawn). Only those overlays pop out, so the
  // button respects the dashboard view instead of opening everything. When the
  // argument is omitted (legacy callers), fall back to opening all panels.
  const all = !Array.isArray(panels)
  const want = new Set(panels ?? [])
  const wants = (key: string): boolean => all || want.has(key)

  if (wants('dps') && (!dpsOverlayWindow || dpsOverlayWindow.isDestroyed())) createDPSOverlay()
  if (wants('buff') && (!buffTimerWindow || buffTimerWindow.isDestroyed())) createBuffTimerOverlay()
  if (wants('detrim') && (!detrimTimerWindow || detrimTimerWindow.isDestroyed())) createDetrimTimerOverlay()
  if (wants('npc') && (!npcOverlayWindow || npcOverlayWindow.isDestroyed())) createNPCOverlay()
  if (wants('rolls') && (!rollTrackerWindow || rollTrackerWindow.isDestroyed())) createRollTrackerOverlay()
  if (wants('respawn') && (!respawnTimerWindow || respawnTimerWindow.isDestroyed())) createRespawnTimerOverlay()

  // Trigger Alerts and CH Chain have no in-dashboard panel or visibility
  // toggle, so they aren't something the user can "disable in the dashboard
  // view". They always pop out (and stay invisible until a trigger fires /
  // a chain is called).
  if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) createTriggerOverlay()
  if (!chChainWindow || chChainWindow.isDestroyed()) createCHChainOverlay()
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
  // While locked the overlay still goes interactive on hover (so its list can
  // be scrolled and per-row controls clicked — issue #127), which would also
  // re-enable OS edge-resize. Disable resize while locked so the window can't
  // be moved or resized; dragging is already gated by the no-drag class.
  win.setResizable(!locked)
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

ipcMain.handle('dialog:save-export-bundle', async (_event, suggestedName?: string) => {
  const result = await dialog.showSaveDialog({
    title: 'Export App Data',
    defaultPath: suggestedName ?? `pq-companion-export-${new Date().toISOString().slice(0, 10)}.pqcb`,
    filters: [
      { name: 'PQ Companion Backup', extensions: ['pqcb'] },
    ],
  })
  return result.canceled ? null : result.filePath
})

ipcMain.handle('dialog:open-import-bundle', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Import App Data',
    filters: [
      { name: 'PQ Companion Backup', extensions: ['pqcb'] },
    ],
  })
  return result.canceled ? null : result.filePaths[0]
})

ipcMain.handle('dialog:open-spellsets-file', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Import Spellsets from .ini',
    filters: [
      { name: 'EverQuest Spellsets', extensions: ['ini'] },
    ],
  })
  return result.canceled ? null : result.filePaths[0]
})

// ── IPC handlers — restart for import application ────────────────────────────

ipcMain.handle('app:relaunch', async () => {
  // Stop sidecar cleanly, then relaunch the Electron app. On next startup
  // the Go server's ApplyPendingImport runs before user.db opens, swaps the
  // staged files into place, and the renderer reconnects.
  await stopSidecar()
  app.relaunch()
  app.exit(0)
})

// ── IPC handlers — auto-updater ───────────────────────────────────────────────

ipcMain.handle('app:version', () => app.getVersion())
ipcMain.handle('backend:port', () => getBackendPort())

// Opens ~/.pq-companion in the OS file manager. Used by the Settings page
// error screen so a user whose backend never came up (AV quarantine, port
// stuck, etc.) can still reach their config.yaml manually. We open the
// folder rather than the file so a missing config.yaml — e.g. brand-new
// install where the sidecar died before writing one — still lands somewhere
// useful.
ipcMain.handle('shell:open-config-folder', async () => {
  const dir = join(homedir(), '.pq-companion')
  await shell.openPath(dir)
  return dir
})

ipcMain.handle('config:folder-path', () => join(homedir(), '.pq-companion'))

// Opens ~/.pq-companion/logs in the OS file manager so the user can attach
// electron.log / server.log to a bug report without hunting through hidden
// directories. The folder is created on first launch by initLogger; if it
// somehow doesn't exist yet we fall back to opening ~/.pq-companion.
ipcMain.handle('shell:open-logs-folder', async () => {
  const logsDir = join(homedir(), '.pq-companion', 'logs')
  const target = existsSync(logsDir) ? logsDir : join(homedir(), '.pq-companion')
  await shell.openPath(target)
  return target
})

// Opens ~/.pq-companion/backups in the OS file manager so the user can grab
// individual EQ-config backup zips (e.g. to move one to another machine
// outside the full app-export flow). Created lazily by the backend on first
// backup, so if it doesn't exist yet we fall back to ~/.pq-companion.
ipcMain.handle('shell:open-backups-folder', async () => {
  const backupsDir = join(homedir(), '.pq-companion', 'backups')
  const target = existsSync(backupsDir) ? backupsDir : join(homedir(), '.pq-companion')
  await shell.openPath(target)
  return target
})

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

app.whenReady().then(async () => {
  // Bail early if the bundled EQ database is missing OR unreadable.
  // existsSync alone isn't enough: an AV that ACL-locks the file post-install
  // (without quarantining it) leaves stat() succeeding while open-for-read
  // fails. Same for OneDrive cloud-only placeholders. The Go sidecar will hit
  // SQLite CANTOPEN on every query in that state, so it's worth confirming
  // read access up front and showing the same recovery dialog if we can't.
  const dbPath = getQuarmDbPath()
  if (dbPath) {
    const readable = await probeDbReadable(dbPath)
    if (!readable.ok) {
      console.error(`[main] quarm.db not usable at ${dbPath}: ${readable.reason}`)
      await showDbMissingDialog(dbPath)
      app.quit()
      return
    }
    console.log(`[main] quarm.db check passed (${readable.reason})`)
  }

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

  // Keep the trigger overlay spanning the full virtual desktop when the
  // monitor layout changes (plug/unplug a display, resolution or DPI change).
  const resizeTriggerOverlay = (): void => {
    if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) return
    triggerOverlayWindow.setBounds(virtualDesktopBounds())
  }
  screen.on('display-added', resizeTriggerOverlay)
  screen.on('display-removed', resizeTriggerOverlay)
  screen.on('display-metrics-changed', resizeTriggerOverlay)

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

import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog, screen, protocol, globalShortcut, Tray, Menu, nativeImage } from 'electron'
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
// Close the logger on 'will-quit', not 'before-quit'. The quit sequence
// preventDefault()s the first before-quit pass and runs up to 5s of async
// teardown (taskkill, overlay snapshots) — closing the fd that early would
// silently no-op every appendLine() during exactly the phase where update-wedge
// evidence gets written. 'will-quit' fires after teardown, right before exit.
app.on('will-quit', () => closeLogger())

// isExternalHttpUrl reports whether a URL is a normal web link we're willing to
// hand to the system browser. Anything that isn't http/https (file://, custom
// schemes, javascript:, etc.) is refused.
function isExternalHttpUrl(url: string): boolean {
  try {
    const proto = new URL(url).protocol
    return proto === 'http:' || proto === 'https:'
  } catch {
    return false
  }
}

// isAppUrl reports whether url is the app's own renderer content: the Vite dev
// server origin in development, or a packaged file:// URL in production. Routing
// is hash-based, so in-app navigation only changes the hash (fires
// did-navigate-in-page, not will-navigate) — only full navigations reach the
// guard, and anything that isn't our own content is treated as untrusted.
function isAppUrl(url: string): boolean {
  if (url.startsWith('file://')) return true
  const devBase = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
  try {
    return new URL(url).origin === new URL(devBase).origin
  } catch {
    return false
  }
}

// hardenWebContents applies navigation defenses to every window:
//  - window.open / target=_blank: external http(s) links open in the system
//    browser; every other scheme is denied. No child window is ever created, so
//    a compromised renderer can't spawn one that inherits the preload bridge.
//  - will-navigate: confine top-level navigation to the app's own content; a
//    phished or injected navigation elsewhere is cancelled (and, if it's a web
//    link, handed to the system browser instead).
function hardenWebContents(contents: Electron.WebContents): void {
  contents.setWindowOpenHandler(({ url }) => {
    if (isExternalHttpUrl(url)) void shell.openExternal(url)
    return { action: 'deny' }
  })
  contents.on('will-navigate', (event, url) => {
    if (isAppUrl(url)) return
    event.preventDefault()
    if (isExternalHttpUrl(url)) void shell.openExternal(url)
  })
}

// Mirror renderer console warnings/errors from every window (main + all
// overlays) into electron.log. DevTools only shows them per-window and only
// while open; this gives one persistent stream that survives into packaged
// builds (where there is no DevTools at all) and can be read after the fact.
app.on('web-contents-created', (_event, contents) => {
  hardenWebContents(contents)
  contents.on('console-message', (details) => {
    if (details.level !== 'warning' && details.level !== 'error') return
    const win = BrowserWindow.fromWebContents(contents)
    const title = win && !win.isDestroyed() ? win.getTitle() : `webcontents-${contents.id}`
    const where = details.sourceId ? ` (${details.sourceId}:${details.lineNumber})` : ''
    appendLine(details.level === 'error' ? 'RENDERER-ERR' : 'RENDERER-WARN', `[${title}] ${details.message}${where}`)
  })
  contents.on('render-process-gone', (_e, details) => {
    const win = BrowserWindow.fromWebContents(contents)
    const title = win && !win.isDestroyed() ? win.getTitle() : `webcontents-${contents.id}`
    appendLine('RENDERER-ERR', `[${title}] render process gone: ${details.reason} (exitCode=${details.exitCode})`)
  })
})

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

// Single-instance lock. Without this, launching the shortcut again while the
// app is already running (e.g. it's hidden in the tray and the user forgets
// it's open) spins up a whole second copy — sidecar, windows and all — instead
// of surfacing the existing one. Users have reported 3–5 instances stacking up
// this way. We grab the lock before app.ready: if another instance already
// holds it we quit immediately, and Electron delivers our launch to that
// instance via the 'second-instance' event, where we restore + focus the
// existing window (the same path the tray uses). On macOS the OS already
// enforces single-instance for packaged .app bundles, but the lock is harmless
// there and keeps `npm run dev` honest too.
const gotSingleInstanceLock = app.requestSingleInstanceLock()
if (!gotSingleInstanceLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    // A second launch was attempted; bring our window back to the foreground
    // instead of letting a duplicate start. showMainWindow() restores from the
    // tray / un-minimizes / re-creates the window as needed.
    showMainWindow()
  })
}

// Resolve the app icon for runtime BrowserWindow use.
// In packaged Windows builds the exe already embeds build/icon.ico via
// electron-builder, so the taskbar/title-bar icon comes from there. We only
// need an explicit icon path during `npm run dev` so the dev taskbar entry
// doesn't fall back to the default Electron icon.
const appIconPath = isDev
  ? join(__dirname, '../../build/icon.png')
  : join(process.resourcesPath, 'icon.png')

// ── System tray / minimize-to-tray ─────────────────────────────────────────
// The renderer pushes the user's "Minimize to Tray" preference here via the
// `window:set-minimize-to-tray` IPC (see App.tsx + SettingsPage). When on, the
// window's close ('X') hides to a tray icon instead of quitting; when off there
// is no tray icon and 'X' quits as before. `isQuittingApp` lets the tray's Quit
// item — and any real quit — bypass the hide-on-close interception.
let tray: Tray | null = null
let minimizeToTray = false
let isQuittingApp = false

// Bring the main window back from the tray (or recreate it if it was torn
// down). Used by the tray click handler and the tray menu's "Show" item.
function showMainWindow(): void {
  if (!mainWindow || mainWindow.isDestroyed()) {
    createMainWindow()
    return
  }
  if (mainWindow.isMinimized()) mainWindow.restore()
  mainWindow.show()
  mainWindow.focus()
}

function createTray(): void {
  if (tray && !tray.isDestroyed()) return
  // A nativeImage from the same PNG the window uses. If the file is missing
  // for any reason, fall back to the path string so Electron still renders
  // something rather than throwing.
  const image = nativeImage.createFromPath(appIconPath)
  tray = new Tray(image.isEmpty() ? appIconPath : image)
  tray.setToolTip('PQ Companion')
  const menu = Menu.buildFromTemplate([
    { label: 'Show PQ Companion', click: () => showMainWindow() },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => {
        isQuittingApp = true
        app.quit()
      }
    }
  ])
  tray.setContextMenu(menu)
  // Left-click (Windows) restores the window; on other platforms the context
  // menu covers it, but wiring click everywhere is harmless.
  tray.on('click', () => showMainWindow())
}

function destroyTray(): void {
  if (tray && !tray.isDestroyed()) tray.destroy()
  tray = null
}

// Apply the latest preference value. Creating/destroying the tray here keeps
// the icon present only while the feature is enabled.
function setMinimizeToTray(enabled: boolean): void {
  minimizeToTray = enabled
  if (enabled) createTray()
  else destroyTray()
}

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

type OverlayName = 'dps' | 'hps' | 'buffTimer' | 'detrimTimer' | 'customTimer' | 'trigger' | 'npc' | 'threat' | 'rollTracker' | 'respawnTimer' | 'chChain' | 'chMetronome'
type Bounds = { x: number; y: number; width: number; height: number }

type BoundsStore = Partial<Record<OverlayName, Bounds>>
type LockStore = Partial<Record<OverlayName, boolean>>

// Default bounds for each popout overlay — used both when an overlay first
// opens with no saved position and when the user resets an overlay's position.
// The screen-spanning "trigger" overlay computes its own bounds and so is
// excluded. Keep this the single source of truth: the create*Overlay functions
// pass these straight into getRestoredBounds().
const OVERLAY_DEFAULTS: Record<Exclude<OverlayName, 'trigger'>, Bounds> = {
  dps: { x: 0, y: 0, width: 420, height: 460 },
  hps: { x: 0, y: 0, width: 420, height: 460 },
  buffTimer: { x: 0, y: 0, width: 280, height: 380 },
  detrimTimer: { x: 0, y: 0, width: 300, height: 320 },
  customTimer: { x: 0, y: 0, width: 280, height: 280 },
  npc: { x: 0, y: 0, width: 360, height: 480 },
  threat: { x: 0, y: 0, width: 288, height: 384 },
  rollTracker: { x: 0, y: 0, width: 320, height: 360 },
  respawnTimer: { x: 0, y: 0, width: 300, height: 320 },
  chChain: { x: 0, y: 0, width: 260, height: 320 },
  chMetronome: { x: 0, y: 0, width: 220, height: 220 },
}

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

// ── Restore-overlays-on-launch persistence ───────────────────────────────────
// When enabled, the set of popout overlays the user had open is remembered and
// re-opened (in their saved positions) on the next launch. Stored here in
// Electron's userData rather than the Go config because the decision has to be
// made at boot, before the backend port is even resolved. `overlays` is the
// last-known open set (canonical overlay names, excluding the always-on
// trigger overlay); it's refreshed when the toggle is enabled and again on quit.
type AutoOpenStore = { enabled: boolean; overlays: OverlayName[] }

function autoOpenFilePath(): string {
  return join(app.getPath('userData'), 'overlayAutoOpen.json')
}

function loadAutoOpenStore(): AutoOpenStore {
  try {
    const raw = readFileSync(autoOpenFilePath(), 'utf8')
    const parsed = JSON.parse(raw) as Partial<AutoOpenStore>
    return {
      enabled: parsed.enabled === true,
      overlays: Array.isArray(parsed.overlays) ? parsed.overlays : [],
    }
  } catch {
    return { enabled: false, overlays: [] }
  }
}

function saveAutoOpenStore(store: AutoOpenStore): void {
  try {
    writeFileSync(autoOpenFilePath(), JSON.stringify(store, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write overlay auto-open state:', err)
  }
}

// Global "Position overlays" mode: a transient flag (off on every launch) that
// makes every popout overlay fully interactive at once so the user can drag
// each one into place regardless of its locked mode — the recovery path for
// "display-only" overlays, which otherwise never capture the mouse. Reverting
// restores each overlay to its persisted lock state.
let overlayPositionMode = false

// Apply the correct mouse-input state to a freshly created overlay window:
// fully interactive while positioning, otherwise click-through + non-resizable
// when locked. (Unlocked overlays stay interactive, the window default.)
function applyInitialOverlayInput(win: BrowserWindow, name: OverlayName): void {
  if (overlayPositionMode) {
    win.setIgnoreMouseEvents(false)
    win.setResizable(true)
    return
  }
  if (getOverlayLocked(name)) {
    win.setIgnoreMouseEvents(true, { forward: true })
    win.setResizable(false)
  }
}

// Map a BrowserWindow back to its overlay name so IPC handlers can look up
// which overlay is sending lock state changes.
const windowToOverlayName = new WeakMap<BrowserWindow, OverlayName>()

const boundsDebounceTimers = new Map<OverlayName, ReturnType<typeof setTimeout>>()

// writeBoundsNow captures and persists the overlay's bounds synchronously.
// Called from the 'close' handler: the debounced persistBounds would schedule a
// timer that fires ~500ms later, by which point the window is destroyed and the
// timer bails on isDestroyed() — losing a move-then-close within that window.
function writeBoundsNow(name: OverlayName, win: BrowserWindow): void {
  const existing = boundsDebounceTimers.get(name)
  if (existing) {
    clearTimeout(existing)
    boundsDebounceTimers.delete(name)
  }
  if (win.isDestroyed()) return
  const b = win.getBounds()
  const store = loadBoundsStore()
  store[name] = { x: b.x, y: b.y, width: b.width, height: b.height }
  saveBoundsStore(store)
}

function persistBounds(name: OverlayName, win: BrowserWindow): void {
  const existing = boundsDebounceTimers.get(name)
  if (existing) clearTimeout(existing)
  boundsDebounceTimers.set(name, setTimeout(() => writeBoundsNow(name, win), 500))
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

// ── Trigger overlay target display ────────────────────────────────────────────
// The trigger overlay (the invisible canvas that trigger alert text and the
// positioning card render into) covers exactly ONE monitor. A window spanning
// the whole virtual desktop is unreliable on multi-monitor Windows: Chromium
// applies a single scale factor to the entire window, so its CSS pixels line up
// with screen coordinates only on the monitor that owns that scale factor. On a
// differently-scaled monitor the positioning card drifts into a region that
// maps to no visible pixels — to the user it simply "vanishes." Confining the
// overlay to one monitor makes CSS px == screen DIP everywhere inside it, which
// kills that whole class of multi-monitor positioning bugs.
//
// The chosen monitor is persisted by display id, with an identical-bounds
// fallback in case ids shift across a reboot/replug. Default (no saved choice):
// the monitor EQ / the main window is on, else the primary.
type OverlayDisplayPref = { id: number; bounds: Bounds }

function overlayDisplayFilePath(): string {
  return join(app.getPath('userData'), 'overlayDisplay.json')
}

function loadOverlayDisplayPref(): OverlayDisplayPref | null {
  try {
    return JSON.parse(readFileSync(overlayDisplayFilePath(), 'utf8')) as OverlayDisplayPref
  } catch {
    return null
  }
}

function saveOverlayDisplayPref(pref: OverlayDisplayPref): void {
  try {
    writeFileSync(overlayDisplayFilePath(), JSON.stringify(pref, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write overlay display pref:', err)
  }
}

function resolveOverlayDisplay() {
  const displays = screen.getAllDisplays()
  const pref = loadOverlayDisplayPref()
  if (pref) {
    const byId = displays.find((d) => d.id === pref.id)
    if (byId) return byId
    // Display ids can change across a reboot/replug; fall back to a display in
    // the same spot so the user's choice survives an unchanged layout.
    const byBounds = displays.find(
      (d) =>
        d.bounds.x === pref.bounds.x &&
        d.bounds.y === pref.bounds.y &&
        d.bounds.width === pref.bounds.width &&
        d.bounds.height === pref.bounds.height,
    )
    if (byBounds) return byBounds
  }
  if (mainWindow && !mainWindow.isDestroyed()) {
    return screen.getDisplayMatching(mainWindow.getBounds())
  }
  return screen.getPrimaryDisplay()
}

// Full bounds (not workArea) of the chosen overlay monitor, so trigger text can
// sit anywhere on it, including over the taskbar.
function overlayDisplayBounds(): Bounds {
  const b = resolveOverlayDisplay().bounds
  return { x: b.x, y: b.y, width: b.width, height: b.height }
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
  win.on('close', () => {
    writeBoundsNow(name, win) // synchronous: window is about to be destroyed
    // Don't leave a closed overlay armed to re-enter placing mode next open.
    placingOverlays.delete(name)
  })
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
// writeMainWindowStateNow captures and persists the main window's state
// synchronously — used from 'close', where the debounced version's timer would
// fire after the window is destroyed and bail (losing a move-then-close).
function writeMainWindowStateNow(win: BrowserWindow): void {
  if (mainStateDebounce) {
    clearTimeout(mainStateDebounce)
    mainStateDebounce = null
  }
  if (win.isDestroyed()) return
  const state = loadMainWindowState()
  state.maximized = win.isMaximized()
  if (!state.maximized) {
    const b = win.getBounds()
    state.bounds = { x: b.x, y: b.y, width: b.width, height: b.height }
  }
  saveMainWindowState(state)
}

function persistMainWindowState(win: BrowserWindow): void {
  if (mainStateDebounce) clearTimeout(mainStateDebounce)
  mainStateDebounce = setTimeout(() => writeMainWindowStateNow(win), 500)
}

function trackMainWindowBounds(win: BrowserWindow): void {
  win.on('move', () => persistMainWindowState(win))
  win.on('resize', () => persistMainWindowState(win))
  win.on('maximize', () => persistMainWindowState(win))
  win.on('unmaximize', () => persistMainWindowState(win))
  win.on('close', () => writeMainWindowStateNow(win)) // synchronous before destroy
}

let mainWindow: BrowserWindow | null = null
let dpsOverlayWindow: BrowserWindow | null = null
let hpsOverlayWindow: BrowserWindow | null = null
let buffTimerWindow: BrowserWindow | null = null
let chChainWindow: BrowserWindow | null = null
let chMetronomeWindow: BrowserWindow | null = null
let detrimTimerWindow: BrowserWindow | null = null
let customTimerWindow: BrowserWindow | null = null
let triggerOverlayWindow: BrowserWindow | null = null
let npcOverlayWindow: BrowserWindow | null = null
let threatOverlayWindow: BrowserWindow | null = null
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
// isBackendAlive probes a candidate dev port to confirm our backend is actually
// listening there (not a stale port-file entry). Short timeout so a dead port
// fails fast and polling continues.
async function isBackendAlive(port: number): Promise<boolean> {
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), 400)
    const res = await fetch(`http://127.0.0.1:${port}/api/enums`, { signal: controller.signal })
    clearTimeout(timer)
    return res.ok
  } catch {
    return false
  }
}

async function resolveDevBackendPort(): Promise<void> {
  const portFile = join(homedir(), '.pq-companion', 'server-port')
  const deadline = Date.now() + 8000
  while (Date.now() < deadline) {
    try {
      const text = await readFile(portFile, 'utf8')
      const port = Number(text.trim())
      // The Go server writes this file on bind but never deletes it on exit, so
      // a stale file from a previous `go run` would otherwise latch us onto a
      // dead port. Health-probe before accepting; if it's stale we keep polling
      // until a fresh server rebinds and (re)writes the file.
      if (Number.isFinite(port) && port > 0 && port < 65536 && (await isBackendAlive(port))) {
        console.log(`[main] Dev backend port discovered from ${portFile}: ${port}`)
        setBackendPort(port)
        return
      }
    } catch {
      // file not written yet — keep polling
    }
    await new Promise((r) => setTimeout(r, 50))
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

  // If the process can't even be spawned (ENOENT, EACCES, AV quarantine), the
  // 'error' event fires and no BACKEND_PORT ever arrives — without this the
  // renderer's backend:port IPC hangs forever on a black screen.
  sidecarProcess.on('error', (err) => {
    console.error('[main] Backend spawn error:', err)
    appendLine('MAIN', `backend spawn error: ${err.message}`)
    void handleBackendStartFailure(`could not launch the backend: ${err.message}`)
  })

  sidecarProcess.on('exit', (code) => {
    console.log(`[main] Backend exited with code ${code}`)
    sidecarProcess = null
    // Exiting before a port was ever resolved is a startup failure (the normal
    // case has backendPort set and this is a shutdown). Surface it instead of
    // leaving the window black.
    if (backendPort === null) {
      void handleBackendStartFailure(`the backend exited (code ${code}) before it was ready`)
    }
  })

  console.log(`[main] Backend sidecar started (pid ${sidecarProcess.pid})`)
}

// handleBackendStartFailure shows a recovery dialog (mirrors showDbMissingDialog)
// when the sidecar fails to launch or exits before signalling its port, then
// quits — rather than hanging on a permanent black screen with no diagnostics.
let backendFailureHandled = false
async function handleBackendStartFailure(reason: string): Promise<void> {
  if (backendFailureHandled || isQuittingApp) return
  backendFailureHandled = true
  const logsDir = join(homedir(), '.pq-companion', 'logs')
  const { response } = await dialog.showMessageBox({
    type: 'error',
    title: 'Backend failed to start',
    message: "PQ Companion's backend service could not start.",
    detail:
      `The helper process (pq-companion-server.exe) ${reason}.\n\n` +
      'The most common cause is antivirus quarantining or blocking the file. ' +
      'Open Windows Security → Virus & threat protection → Protection history, ' +
      'look for "pq-companion-server.exe", restore it, and add an exclusion for ' +
      'the install folder. Then relaunch PQ Companion.\n\n' +
      `See the logs at:\n${logsDir}`,
    buttons: ['Quit', 'Open logs folder'],
    defaultId: 0,
  })
  if (response === 1) {
    void shell.openPath(logsDir)
  }
  isQuittingApp = true
  app.quit()
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
  // Capture the open set for "restore on launch" before we tear the windows
  // down — once destroyed, currentlyOpenOverlayNames() would report nothing.
  snapshotAutoOpenOverlays()
  for (const win of [dpsOverlayWindow, hpsOverlayWindow, buffTimerWindow, detrimTimerWindow, customTimerWindow, triggerOverlayWindow, npcOverlayWindow, threatOverlayWindow, rollTrackerWindow, respawnTimerWindow, chChainWindow, chMetronomeWindow]) {
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
      nodeIntegration: false
    },
    show: false // show after ready-to-show to avoid flash
  })

  // Mixed-DPI multi-monitor correction. When the window is constructed with an
  // x/y on a monitor whose scale factor differs from the primary display,
  // Chromium sizes it using the wrong display's scale, so width/height come
  // back wrong (and then get re-saved on the resize, never converging). Once
  // the window physically lives on the target display we re-apply the saved
  // rect: setBounds() now interprets the dimensions in THAT monitor's scale
  // factor, correcting the size. On single-monitor / same-DPI setups the bounds
  // already match, so this is a no-op — existing users are unaffected.
  if (restored) mainWindow.setBounds(restored)

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

  // External-link handling + navigation confinement are applied globally to
  // every window via hardenWebContents (see the web-contents-created hook), so
  // no per-window setWindowOpenHandler is needed here.

  // Minimize-to-tray: when the preference is on, the 'X' hides the window to
  // the tray instead of closing it. A real quit (tray "Quit", app.quit, OS
  // shutdown) sets isQuittingApp first so this doesn't trap the exit.
  mainWindow.on('close', (event) => {
    if (minimizeToTray && !isQuittingApp) {
      event.preventDefault()
      mainWindow?.hide()
    }
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

  const { x, y, width, height } = getRestoredBounds('dps', OVERLAY_DEFAULTS.dps)
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
      nodeIntegration: false
    },
  })

  dpsOverlayWindow.once('ready-to-show', () => {
    dpsOverlayWindow?.show()
  })

  // Keep it above fullscreen apps on macOS/Windows.
  dpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  dpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(dpsOverlayWindow, 'dps')
  applyInitialOverlayInput(dpsOverlayWindow, 'dps')
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

  const { x, y, width, height } = getRestoredBounds('hps', OVERLAY_DEFAULTS.hps)
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
      nodeIntegration: false
    },
  })

  hpsOverlayWindow.once('ready-to-show', () => {
    hpsOverlayWindow?.show()
  })

  hpsOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  hpsOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(hpsOverlayWindow, 'hps')
  applyInitialOverlayInput(hpsOverlayWindow, 'hps')
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

  const { x, y, width, height } = getRestoredBounds('buffTimer', OVERLAY_DEFAULTS.buffTimer)
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
      nodeIntegration: false
    },
  })

  buffTimerWindow.once('ready-to-show', () => {
    buffTimerWindow?.show()
  })

  buffTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  buffTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(buffTimerWindow, 'buffTimer')
  applyInitialOverlayInput(buffTimerWindow, 'buffTimer')
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

  const { x, y, width, height } = getRestoredBounds('chChain', OVERLAY_DEFAULTS.chChain)
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
      nodeIntegration: false
    },
  })

  chChainWindow.once('ready-to-show', () => {
    chChainWindow?.show()
  })

  chChainWindow.setAlwaysOnTop(true, 'screen-saver')
  chChainWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(chChainWindow, 'chChain')
  applyInitialOverlayInput(chChainWindow, 'chChain')
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

function createCHMetronomeOverlay(): void {
  if (chMetronomeWindow && !chMetronomeWindow.isDestroyed()) {
    chMetronomeWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('chMetronome', OVERLAY_DEFAULTS.chMetronome)
  chMetronomeWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 180,
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
      nodeIntegration: false
    },
  })

  chMetronomeWindow.once('ready-to-show', () => {
    chMetronomeWindow?.show()
  })

  chMetronomeWindow.setAlwaysOnTop(true, 'screen-saver')
  chMetronomeWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(chMetronomeWindow, 'chMetronome')
  applyInitialOverlayInput(chMetronomeWindow, 'chMetronome')
  trackOverlayBounds('chMetronome', chMetronomeWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    chMetronomeWindow.loadURL(`${rendererUrl}/#/ch-metronome-window`)
  } else {
    chMetronomeWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/ch-metronome-window',
    })
  }

  chMetronomeWindow.on('closed', () => {
    chMetronomeWindow = null
  })
}

// ── Detrimental Timer overlay window ─────────────────────────────────────────

function createDetrimTimerOverlay(): void {
  if (detrimTimerWindow && !detrimTimerWindow.isDestroyed()) {
    detrimTimerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('detrimTimer', OVERLAY_DEFAULTS.detrimTimer)
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
      nodeIntegration: false
    },
  })

  detrimTimerWindow.once('ready-to-show', () => {
    detrimTimerWindow?.show()
  })

  detrimTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  detrimTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(detrimTimerWindow, 'detrimTimer')
  applyInitialOverlayInput(detrimTimerWindow, 'detrimTimer')
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

// ── Custom Timer overlay window ──────────────────────────────────────────────

function createCustomTimerOverlay(): void {
  if (customTimerWindow && !customTimerWindow.isDestroyed()) {
    customTimerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('customTimer', OVERLAY_DEFAULTS.customTimer)
  customTimerWindow = new BrowserWindow({
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
      nodeIntegration: false
    },
  })

  customTimerWindow.once('ready-to-show', () => {
    customTimerWindow?.show()
  })

  customTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  customTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(customTimerWindow, 'customTimer')
  applyInitialOverlayInput(customTimerWindow, 'customTimer')
  trackOverlayBounds('customTimer', customTimerWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    customTimerWindow.loadURL(`${rendererUrl}/#/custom-timer-window`)
  } else {
    customTimerWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/custom-timer-window',
    })
  }

  customTimerWindow.on('closed', () => {
    customTimerWindow = null
  })
}

// ── Respawn (death) Timer overlay window ─────────────────────────────────────

function createRespawnTimerOverlay(): void {
  if (respawnTimerWindow && !respawnTimerWindow.isDestroyed()) {
    respawnTimerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('respawnTimer', OVERLAY_DEFAULTS.respawnTimer)
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
      nodeIntegration: false
    },
  })

  respawnTimerWindow.once('ready-to-show', () => {
    respawnTimerWindow?.show()
  })

  respawnTimerWindow.setAlwaysOnTop(true, 'screen-saver')
  respawnTimerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(respawnTimerWindow, 'respawnTimer')
  applyInitialOverlayInput(respawnTimerWindow, 'respawnTimer')
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
  // alerts and the positioning test card render into. It covers exactly one
  // monitor — the user's chosen overlay display (see resolveOverlayDisplay) —
  // so notification text and the positioning card share a clean, single-DPI
  // coordinate space. The window has no chrome, isn't visible outside a
  // positioning session, and the renderer toggles click-through over just the
  // test card, so a full-monitor size never blocks the underlying app/game.
  //
  // Because it's always pinned to the chosen monitor, we do NOT persist or
  // restore its bounds (unlike the other overlays) — they're recomputed from
  // the current display layout every time. Drop any stale saved bounds left
  // over from older versions.
  const store = loadBoundsStore()
  if (store.trigger) {
    delete store.trigger
    saveBoundsStore(store)
  }
  const { x, y, width, height } = overlayDisplayBounds()
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
      nodeIntegration: false
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
    // Don't leave a global Escape capture dangling if the window goes away
    // mid-session — it would silently swallow Escape app-wide.
    setTriggerEscapeShortcut(false)
  })
}

// ── NPC Overlay window ────────────────────────────────────────────────────────

function createNPCOverlay(): void {
  if (npcOverlayWindow && !npcOverlayWindow.isDestroyed()) {
    npcOverlayWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('npc', OVERLAY_DEFAULTS.npc)
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
      nodeIntegration: false
    },
  })

  npcOverlayWindow.once('ready-to-show', () => {
    npcOverlayWindow?.show()
  })

  npcOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  npcOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(npcOverlayWindow, 'npc')
  applyInitialOverlayInput(npcOverlayWindow, 'npc')
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

// ── Threat meter overlay window ──────────────────────────────────────────────

function createThreatOverlay(): void {
  if (threatOverlayWindow && !threatOverlayWindow.isDestroyed()) {
    threatOverlayWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('threat', OVERLAY_DEFAULTS.threat)
  threatOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 200,
    minHeight: 150,
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
      nodeIntegration: false
    },
  })

  threatOverlayWindow.once('ready-to-show', () => {
    threatOverlayWindow?.show()
  })

  threatOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  threatOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(threatOverlayWindow, 'threat')
  applyInitialOverlayInput(threatOverlayWindow, 'threat')
  trackOverlayBounds('threat', threatOverlayWindow)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    threatOverlayWindow.loadURL(`${rendererUrl}/#/threat-overlay-window`)
  } else {
    threatOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/threat-overlay-window',
    })
  }

  threatOverlayWindow.on('closed', () => {
    threatOverlayWindow = null
  })
}

// ── Roll Tracker overlay window ──────────────────────────────────────────────

function createRollTrackerOverlay(): void {
  if (rollTrackerWindow && !rollTrackerWindow.isDestroyed()) {
    rollTrackerWindow.focus()
    return
  }

  const { x, y, width, height } = getRestoredBounds('rollTracker', OVERLAY_DEFAULTS.rollTracker)
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
      nodeIntegration: false
    },
  })

  rollTrackerWindow.once('ready-to-show', () => {
    rollTrackerWindow?.show()
  })

  rollTrackerWindow.setAlwaysOnTop(true, 'screen-saver')
  rollTrackerWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(rollTrackerWindow, 'rollTracker')
  applyInitialOverlayInput(rollTrackerWindow, 'rollTracker')
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

// True only for the one canonical main window's webContents. Trigger/alert
// audio is meant to play in exactly one place, but audio duplication reports
// ("the tell beep played 3×") point to more than one renderer running the
// audio engine at once — the per-renderer dedup in the frontend can't collapse
// plays across separate renderer processes. Renderers gate playback on this so
// only the current mainWindow ever emits sound; any stray/duplicate main-route
// renderer (a ghost window that outlived its replacement) stays silent. Overlay
// windows never mount the audio hooks, so they're unaffected either way.
ipcMain.handle('window:is-primary', (event) => {
  return (
    !!mainWindow &&
    !mainWindow.isDestroyed() &&
    event.sender.id === mainWindow.webContents.id
  )
})

// The renderer mirrors the persisted "Minimize to Tray" preference here on
// startup and whenever the user toggles it, so the close handler and tray
// icon stay in sync without the main process needing to parse config.yaml.
ipcMain.handle('window:set-minimize-to-tray', (_event, enabled: boolean) => {
  setMinimizeToTray(Boolean(enabled))
})

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
// Windows that already have a 'closed' cleanup handler, so drag:start doesn't
// register a fresh once('closed') on every drag — that leaked one listener per
// drag (MaxListenersExceededWarning after ~10, accumulating closures).
const dragClosedGuarded = new WeakSet<BrowserWindow>()

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
  // Free any dangling loop if the window closes mid-drag. Registered once per
  // window (not per drag) so the listener count stays bounded.
  if (!dragClosedGuarded.has(win)) {
    dragClosedGuarded.add(win)
    win.once('closed', () => {
      stopDrag(win)
      dragClosedGuarded.delete(win)
    })
  }
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

// Default spawn point for the positioning card, in coordinates local to the
// trigger overlay window. The overlay now covers exactly one monitor, so its
// window-local center is simply half its size — no virtual-desktop offset math.
ipcMain.handle('screen:trigger-default-center', () => {
  const b = overlayDisplayBounds()
  return { x: Math.round(b.width / 2), y: Math.round(b.height / 2) }
})

// The overlay covers exactly one monitor, so the only "display" in window-local
// coordinates is the whole window. Returned as a one-element list so the
// renderer's clamp logic stays unchanged.
ipcMain.handle('screen:trigger-displays', () => {
  const b = overlayDisplayBounds()
  return [{ x: 0, y: 0, width: b.width, height: b.height }]
})

// Lists every connected display for the overlay-monitor picker, with a
// human-readable label plus the id used to persist the choice.
ipcMain.handle('screen:list-displays', () => {
  const primaryId = screen.getPrimaryDisplay().id
  const chosenId = resolveOverlayDisplay().id
  return screen.getAllDisplays().map((d, i) => ({
    id: d.id,
    label: d.label && d.label.trim().length > 0 ? d.label : `Display ${i + 1}`,
    width: d.bounds.width,
    height: d.bounds.height,
    isPrimary: d.id === primaryId,
    isCurrent: d.id === chosenId,
  }))
})

// Persists which monitor the trigger overlay covers and, if the overlay window
// already exists, moves/resizes it to the new monitor immediately.
ipcMain.handle('overlay:set-display', (_event, id: number) => {
  const target = screen.getAllDisplays().find((d) => d.id === id)
  if (!target) return
  const b = target.bounds
  const bounds = { x: b.x, y: b.y, width: b.width, height: b.height }
  saveOverlayDisplayPref({ id: target.id, bounds })
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) {
    triggerOverlayWindow.setBounds(bounds)
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

ipcMain.handle('overlay:chmetronome:open', () => {
  createCHMetronomeOverlay()
})

ipcMain.handle('overlay:chmetronome:close', () => {
  if (chMetronomeWindow && !chMetronomeWindow.isDestroyed()) {
    chMetronomeWindow.close()
  }
})

ipcMain.handle('overlay:chmetronome:toggle', () => {
  if (chMetronomeWindow && !chMetronomeWindow.isDestroyed()) {
    chMetronomeWindow.close()
  } else {
    createCHMetronomeOverlay()
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

ipcMain.handle('overlay:customtimer:open', () => {
  createCustomTimerOverlay()
})

ipcMain.handle('overlay:customtimer:close', () => {
  if (customTimerWindow && !customTimerWindow.isDestroyed()) {
    customTimerWindow.close()
  }
})

ipcMain.handle('overlay:customtimer:toggle', () => {
  if (customTimerWindow && !customTimerWindow.isDestroyed()) {
    customTimerWindow.close()
  } else {
    createCustomTimerOverlay()
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
// While an interactive positioning session is live the trigger overlay spans
// every monitor and captures ALL mouse input, so the ONLY way to interact with
// anything is the test card. If that card lands on a monitor the user isn't
// looking at (or focus is on a fullscreen game), the desktop appears frozen.
// A global Escape is an always-available bail-out: it fires regardless of which
// window/app currently holds focus, where the renderer-local Escape handlers
// cannot. Registered only for the duration of a session.
let triggerEscapeRegistered = false

function forceEndTriggerPositioning(): void {
  const win = triggerOverlayWindow
  if (!win || win.isDestroyed()) return
  // Break the input lockout immediately from the main process — don't wait on a
  // renderer round-trip — then ask the renderer to tear the session down cleanly
  // (revert position, reset the editor button, clear the backend session).
  win.setIgnoreMouseEvents(true, { forward: true })
  const overlayHadFocus = win.isFocused()
  if (win.isVisible()) win.hide()
  if (overlayHadFocus && mainWindow && !mainWindow.isDestroyed()) mainWindow.focus()
  win.webContents.send('overlay:trigger:escape')
  // Drop the capture immediately. The session is over from the main process's
  // point of view; if the renderer later re-enters interactive it re-registers.
  // Without this, a dead/slow renderer that never sends set-mode('hidden') would
  // leave Escape swallowed app-wide.
  setTriggerEscapeShortcut(false)
}

function setTriggerEscapeShortcut(active: boolean): void {
  if (active === triggerEscapeRegistered) return
  if (active) {
    triggerEscapeRegistered = globalShortcut.register('Escape', forceEndTriggerPositioning)
  } else {
    globalShortcut.unregister('Escape')
    triggerEscapeRegistered = false
  }
}

ipcMain.handle('overlay:trigger:set-mode', (_event, mode: 'interactive' | 'passthrough' | 'hidden') => {
  const win = triggerOverlayWindow
  if (!win || win.isDestroyed()) return
  // The global-Escape bail-out is only needed while the overlay is capturing the
  // whole desktop (interactive). Drop it the moment we leave that mode.
  setTriggerEscapeShortcut(mode === 'interactive')
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

ipcMain.handle('overlay:threat:open', () => {
  createThreatOverlay()
})

ipcMain.handle('overlay:threat:close', () => {
  if (threatOverlayWindow && !threatOverlayWindow.isDestroyed()) {
    threatOverlayWindow.close()
  }
})

ipcMain.handle('overlay:threat:toggle', () => {
  if (threatOverlayWindow && !threatOverlayWindow.isDestroyed()) {
    threatOverlayWindow.close()
  } else {
    createThreatOverlay()
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

// User-managed popout windows. Excludes the trigger alert overlay: it's created
// at startup and always present (just hidden until an alert fires), so counting
// it would make "any popout open" perpetually true — wrongly starting the bulk
// button on "Close All Popouts" with nothing actually popped out. Closing it
// would also stop trigger alerts from rendering, so it's left alone by the
// bulk close as well.
function userPopoutWindows(): BrowserWindow[] {
  return [
    dpsOverlayWindow,
    hpsOverlayWindow,
    buffTimerWindow,
    detrimTimerWindow,
    customTimerWindow,
    npcOverlayWindow,
    threatOverlayWindow,
    rollTrackerWindow,
    respawnTimerWindow,
    chChainWindow,
    chMetronomeWindow,
  ].filter((w): w is BrowserWindow => !!w && !w.isDestroyed())
}

ipcMain.handle('overlay:popouts:any-open', () => userPopoutWindows().length > 0)

ipcMain.handle('overlay:popouts:open-all', (_event, panels?: string[]) => {
  // `panels` is the set of dashboard panel keys the user has toggled visible
  // (buff/detrim/dps/npc/rolls/respawn/chChain/chMetronome/custom). Only those
  // overlays pop out, so the
  // button respects the dashboard view instead of opening everything. When the
  // argument is omitted (legacy callers), fall back to opening all panels.
  const all = !Array.isArray(panels)
  const want = new Set(panels ?? [])
  const wants = (key: string): boolean => all || want.has(key)

  if (wants('dps') && (!dpsOverlayWindow || dpsOverlayWindow.isDestroyed())) createDPSOverlay()
  if (wants('buff') && (!buffTimerWindow || buffTimerWindow.isDestroyed())) createBuffTimerOverlay()
  if (wants('detrim') && (!detrimTimerWindow || detrimTimerWindow.isDestroyed())) createDetrimTimerOverlay()
  if (wants('custom') && (!customTimerWindow || customTimerWindow.isDestroyed())) createCustomTimerOverlay()
  if (wants('npc') && (!npcOverlayWindow || npcOverlayWindow.isDestroyed())) createNPCOverlay()
  if (wants('threat') && (!threatOverlayWindow || threatOverlayWindow.isDestroyed())) createThreatOverlay()
  if (wants('rolls') && (!rollTrackerWindow || rollTrackerWindow.isDestroyed())) createRollTrackerOverlay()
  if (wants('respawn') && (!respawnTimerWindow || respawnTimerWindow.isDestroyed())) createRespawnTimerOverlay()
  if (wants('chChain') && (!chChainWindow || chChainWindow.isDestroyed())) createCHChainOverlay()
  if (wants('chMetronome') && (!chMetronomeWindow || chMetronomeWindow.isDestroyed())) createCHMetronomeOverlay()

  // Trigger Alerts has no in-dashboard panel or visibility toggle, so it isn't
  // something the user can "disable in the dashboard view". It always pops out
  // (and stays invisible until a trigger fires).
  if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) createTriggerOverlay()
})

ipcMain.handle('overlay:popouts:close-all', () => {
  // Use close() (not destroy()) so each window's 'close' handler persists its
  // bounds. The always-on trigger overlay is intentionally left open so trigger
  // alerts keep rendering after a bulk close.
  for (const win of userPopoutWindows()) win.close()
})

// Per-overlay popout open state, keyed by canonical overlay name (excludes the
// always-on trigger overlay). Lets the dashboard show which windows are open.
ipcMain.handle('overlay:popouts:states', () => {
  const states: Record<string, boolean> = {}
  for (const name of RESETTABLE_OVERLAYS) {
    const win = overlayWindowByName(name)
    states[name] = !!win && !win.isDestroyed()
  }
  return states
})

// ── IPC handlers — restore overlays on launch ────────────────────────────────

ipcMain.handle('overlay:auto-open:get', () => loadAutoOpenStore())

ipcMain.handle('overlay:auto-open:set-enabled', (_event, enabled: boolean) => {
  const store = loadAutoOpenStore()
  store.enabled = enabled === true
  // Snapshot the currently-open set the moment the user turns this on, so even
  // an abrupt exit (no before-quit) still restores something sensible. The set
  // is refreshed again on quit to track windows opened/closed afterwards.
  if (store.enabled) store.overlays = currentlyOpenOverlayNames()
  saveAutoOpenStore(store)
})

// ── IPC handlers — overlay position reset ────────────────────────────────────
// Recovers an overlay that has wandered (or been pushed by a layout/update)
// off-screen. Because a locked overlay is click-through and its unlock button
// may be off-screen, the reset is driven from the main app window — never from
// the stuck overlay itself.

// Every overlay with a saved/movable position. Excludes the screen-spanning
// "trigger" overlay, which has no free position to reset.
const RESETTABLE_OVERLAYS = Object.keys(OVERLAY_DEFAULTS) as Array<Exclude<OverlayName, 'trigger'>>

function overlayWindowByName(name: OverlayName): BrowserWindow | null {
  switch (name) {
    case 'dps': return dpsOverlayWindow
    case 'hps': return hpsOverlayWindow
    case 'buffTimer': return buffTimerWindow
    case 'detrimTimer': return detrimTimerWindow
    case 'customTimer': return customTimerWindow
    case 'trigger': return triggerOverlayWindow
    case 'npc': return npcOverlayWindow
    case 'threat': return threatOverlayWindow
    case 'rollTracker': return rollTrackerWindow
    case 'respawnTimer': return respawnTimerWindow
    case 'chChain': return chChainWindow
    case 'chMetronome': return chMetronomeWindow
    default: return null
  }
}

// Open one overlay's popout window by canonical name (the trigger overlay is
// created separately at startup and so is intentionally absent here).
function createOverlayByName(name: OverlayName): void {
  switch (name) {
    case 'dps': createDPSOverlay(); break
    case 'hps': createHPSOverlay(); break
    case 'buffTimer': createBuffTimerOverlay(); break
    case 'detrimTimer': createDetrimTimerOverlay(); break
    case 'customTimer': createCustomTimerOverlay(); break
    case 'npc': createNPCOverlay(); break
    case 'threat': createThreatOverlay(); break
    case 'rollTracker': createRollTrackerOverlay(); break
    case 'respawnTimer': createRespawnTimerOverlay(); break
    case 'chChain': createCHChainOverlay(); break
    case 'chMetronome': createCHMetronomeOverlay(); break
    default: break
  }
}

// Canonical names of every popout overlay currently open (excludes the
// always-on trigger overlay, which isn't user-toggled). Used to snapshot what
// to restore on the next launch.
function currentlyOpenOverlayNames(): OverlayName[] {
  return RESETTABLE_OVERLAYS.filter((name) => {
    const win = overlayWindowByName(name)
    return !!win && !win.isDestroyed()
  })
}

// Snapshot which popout overlays are open so "restore overlays on launch" can
// re-open exactly that set next time. Must run while the overlay windows are
// still alive — on the X-to-quit path closeAllOverlays() destroys them before
// before-quit fires, so this is called from BOTH closeAllOverlays() (X path)
// and before-quit (tray/app.quit path). The one-shot guard means whichever
// fires first captures the live set and the other becomes a no-op, so a late
// run can never clobber a good snapshot with an empty list.
let didSnapshotAutoOpen = false
function snapshotAutoOpenOverlays(): void {
  if (didSnapshotAutoOpen) return
  const store = loadAutoOpenStore()
  if (!store.enabled) return
  didSnapshotAutoOpen = true
  store.overlays = currentlyOpenOverlayNames()
  saveAutoOpenStore(store)
}

// Center a window of the given size on the PRIMARY display's work area. We
// deliberately use the primary monitor (not the overlay's last-known monitor,
// which may be the very screen it disappeared off of) so a reset always lands
// somewhere visible.
function centeredOnPrimary(size: Bounds): Bounds {
  const wa = screen.getPrimaryDisplay().workArea
  return {
    x: Math.round(wa.x + (wa.width - size.width) / 2),
    y: Math.round(wa.y + (wa.height - size.height) / 2),
    width: size.width,
    height: size.height,
  }
}

// Recenter one overlay on the primary monitor at its default size AND clear its
// locked state, so a stuck "locked + off-screen" overlay becomes immediately
// movable again. Updates the saved bounds too, so an overlay that's currently
// closed also re-opens centered.
function resetOverlayPosition(name: Exclude<OverlayName, 'trigger'>): void {
  const target = centeredOnPrimary(OVERLAY_DEFAULTS[name])

  // Persist centered bounds and clear the lock regardless of open/closed state.
  const store = loadBoundsStore()
  store[name] = target
  saveBoundsStore(store)
  setOverlayLocked(name, false)

  const win = overlayWindowByName(name)
  if (win && !win.isDestroyed()) {
    win.setResizable(true)
    win.setIgnoreMouseEvents(false)
    win.setBounds(target)
    // The lock hook only reads lock state on mount, so push the cleared state
    // to the overlay's renderer to keep its padlock button in sync.
    win.webContents.send('overlay:lock-changed', false)
  }
}

ipcMain.handle('overlay:reset-position', (_event, name: string) => {
  if ((RESETTABLE_OVERLAYS as string[]).includes(name)) {
    resetOverlayPosition(name as Exclude<OverlayName, 'trigger'>)
  }
})

ipcMain.handle('overlay:reset-all-positions', () => {
  for (const name of RESETTABLE_OVERLAYS) resetOverlayPosition(name)
})

// Center a window of the given size on a specific display's work area.
function centeredOnDisplay(display: Electron.Display, size: { width: number; height: number }): Bounds {
  const wa = display.workArea
  return {
    x: Math.round(wa.x + (wa.width - size.width) / 2),
    y: Math.round(wa.y + (wa.height - size.height) / 2),
    width: size.width,
    height: size.height,
  }
}

// The display id each resettable overlay currently lives on — by its live
// window bounds when open, else its saved (or default) bounds. Powers the
// per-overlay monitor picker in Settings so the dropdown shows where each
// overlay is now.
ipcMain.handle('overlay:display-ids', () => {
  const out: Record<string, number> = {}
  const store = loadBoundsStore()
  for (const name of RESETTABLE_OVERLAYS) {
    const win = overlayWindowByName(name)
    const b = win && !win.isDestroyed() ? win.getBounds() : store[name] ?? OVERLAY_DEFAULTS[name]
    out[name] = screen.getDisplayMatching(b).id
  }
  return out
})

// ── Per-overlay "Move" (placing) mode ───────────────────────────────────────
// Scoped, visible replacement for the old global "Position overlays" toggle.
// While an overlay is "placing" it's made interactive and draggable regardless
// of its lock mode (so a chromeless Display-only HUD can be grabbed), with a
// dashed outline + "Drag to place / Done" banner drawn by the renderer
// (useOverlayLock). The renderer owns input state; the main process just tracks
// which overlays are placing and notifies their windows.
const placingOverlays = new Set<string>()

function setPlacing(name: string, on: boolean): void {
  if (on) placingOverlays.add(name)
  else placingOverlays.delete(name)
  const win = overlayWindowByName(name as OverlayName)
  if (win && !win.isDestroyed()) {
    if (on) {
      // Re-home an off-screen overlay onto the primary monitor first so its
      // placing banner is actually visible to grab.
      if (!isOnScreen(win.getBounds())) win.setBounds(centeredOnPrimary(win.getBounds()))
      win.show()
    }
    win.webContents.send('overlay:placing-changed', on)
  }
}

ipcMain.handle('overlay:place:start', (_event, name: string) => {
  if ((RESETTABLE_OVERLAYS as string[]).includes(name)) setPlacing(name, true)
})
ipcMain.handle('overlay:place:stop', (_event, name: string) => {
  if ((RESETTABLE_OVERLAYS as string[]).includes(name)) setPlacing(name, false)
})
// Called by the overlay's own "Done" button — resolve the window to its name.
ipcMain.handle('overlay:place:stop-self', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  const name = win ? windowToOverlayName.get(win) : undefined
  if (name) setPlacing(name, false)
})
// Read on mount so a freshly-opened overlay window picks up that it should be
// placing even if it missed the placing-changed broadcast.
ipcMain.handle('overlay:place:am-i-placing', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  const name = win ? windowToOverlayName.get(win) : undefined
  return name ? placingOverlays.has(name) : false
})
ipcMain.handle('overlay:place:names', () => Array.from(placingOverlays))

// Send one overlay to a chosen monitor, centered on it. Persists the new
// bounds so a currently-closed overlay also re-opens there. This is the
// reliable multi-monitor path — dragging frameless always-on-top windows
// across monitors is unreliable on Windows, so users pick a monitor instead.
ipcMain.handle('overlay:move-to-display', (_event, name: string, displayId: number) => {
  if (!(RESETTABLE_OVERLAYS as string[]).includes(name)) return
  const target = screen.getAllDisplays().find((d) => d.id === displayId)
  if (!target) return
  const oname = name as Exclude<OverlayName, 'trigger'>
  const win = overlayWindowByName(oname)
  const size = win && !win.isDestroyed() ? win.getBounds() : loadBoundsStore()[oname] ?? OVERLAY_DEFAULTS[oname]
  const bounds = centeredOnDisplay(target, { width: size.width, height: size.height })
  const store = loadBoundsStore()
  store[oname] = bounds
  saveBoundsStore(store)
  if (win && !win.isDestroyed()) win.setBounds(bounds)
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

// ── IPC handlers — global position mode ──────────────────────────────────────

// Push the current position-mode flag to every overlay window and the main
// window so their renderers (chrome fade, drag gating, lock hover) stay in sync.
function broadcastPositionMode(): void {
  const targets: Array<BrowserWindow | null> = [mainWindow]
  for (const name of RESETTABLE_OVERLAYS) targets.push(overlayWindowByName(name))
  for (const win of targets) {
    if (win && !win.isDestroyed()) {
      win.webContents.send('overlay:position-mode-changed', overlayPositionMode)
    }
  }
}

function setOverlayPositionMode(enabled: boolean): void {
  overlayPositionMode = enabled
  // Flip every open overlay's window-level input state: fully interactive while
  // positioning, back to its persisted lock state when done.
  for (const name of RESETTABLE_OVERLAYS) {
    const win = overlayWindowByName(name)
    if (!win || win.isDestroyed()) continue
    if (enabled) {
      win.setIgnoreMouseEvents(false)
      win.setResizable(true)
    } else {
      const locked = getOverlayLocked(name)
      win.setIgnoreMouseEvents(locked, { forward: true })
      win.setResizable(!locked)
    }
  }
  broadcastPositionMode()
}

ipcMain.handle('overlay:position-mode:get', () => overlayPositionMode)
ipcMain.handle('overlay:position-mode:set', (_event, enabled: boolean) => {
  setOverlayPositionMode(Boolean(enabled))
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

ipcMain.handle('dialog:open-macros-file', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Import Macros from _pq.proj.ini',
    filters: [
      { name: 'EverQuest Character Config', extensions: ['ini'] },
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

// Drive the MAIN window to a deep link from another window (e.g. an overlay
// popout whose loot/spell rows link into the database explorer). Overlays have
// their own router, so they can't navigate the main window directly — they ask
// the main process to focus the main window and hand it the hash route.
ipcMain.handle('app:navigate-main', (_event, route: string) => {
  if (typeof route !== 'string' || !route.startsWith('/')) return
  showMainWindow()
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('app:navigate', route)
  }
})

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
    // Reject anything that isn't a recognized audio extension BEFORE reading.
    // Otherwise this handler is an arbitrary local-file read primitive
    // reachable from any of the app's renderers — a compromised renderer could
    // pull user.db, SSH keys, cookies, etc. Confining reads to audio types
    // keeps the surface to files the user could already play.
    const mime = audioMimeType(extname(p).toLowerCase())
    if (mime === 'application/octet-stream') {
      // eslint-disable-next-line no-console
      console.warn('[pq-audio] refused non-audio path', { path: p })
      return new Response('unsupported media type', { status: 415 })
    }
    try {
      const data = await readFile(p)
      const total = data.byteLength

      // Chromium's media element (HTMLAudioElement) requests media resources
      // with a `Range` header and expects proper HTTP range semantics back.
      // When it gets a bare 200 with no Accept-Ranges / Content-Range, its
      // resource loader re-issues the request and RESTARTS playback from the
      // top — the same file was served 2–3× for a single Audio().play() and
      // the user heard the sound stutter/double "then cut off". This was the
      // long-standing "audio duplication" bug for custom-sound users (TTS,
      // which never touches this protocol, was immune). Honor Range with a
      // 206 + Content-Range, and always advertise Accept-Ranges + Content-
      // Length so the media stack fetches once and plays once.
      const baseHeaders: Record<string, string> = {
        'Content-Type': mime,
        'Accept-Ranges': 'bytes',
        // These files are immutable for the life of a play; let Chromium reuse
        // the buffered resource instead of re-fetching mid-playback.
        'Cache-Control': 'no-cache',
      }

      const range = request.headers.get('Range')
      const match = range ? /^bytes=(\d*)-(\d*)$/.exec(range.trim()) : null
      if (match) {
        // Parse `bytes=start-end`; either side may be omitted (suffix/open).
        let start = match[1] === '' ? NaN : parseInt(match[1], 10)
        let end = match[2] === '' ? NaN : parseInt(match[2], 10)
        if (Number.isNaN(start)) {
          // Suffix range: bytes=-N → last N bytes.
          const suffix = Number.isNaN(end) ? total : end
          start = Math.max(0, total - suffix)
          end = total - 1
        } else if (Number.isNaN(end)) {
          end = total - 1
        }
        // Clamp and validate. An unsatisfiable range gets a 416 per spec.
        if (start > end || start >= total) {
          return new Response('range not satisfiable', {
            status: 416,
            headers: { 'Content-Range': `bytes */${total}`, 'Accept-Ranges': 'bytes' },
          })
        }
        end = Math.min(end, total - 1)
        const chunk = data.subarray(start, end + 1)
        // eslint-disable-next-line no-console
        console.log('[pq-audio] served (range)', {
          path: p,
          mime,
          range: `${start}-${end}/${total}`,
          bytes: chunk.byteLength,
        })
        return new Response(chunk, {
          status: 206,
          headers: {
            ...baseHeaders,
            'Content-Range': `bytes ${start}-${end}/${total}`,
            'Content-Length': String(chunk.byteLength),
          },
        })
      }

      // eslint-disable-next-line no-console
      console.log('[pq-audio] served', { path: p, mime, bytes: total })
      return new Response(data, {
        status: 200,
        headers: { ...baseHeaders, 'Content-Length': String(total) },
      })
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[pq-audio] failed to read', { path: p, err: String(err) })
      return new Response('not found', { status: 404 })
    }
  })

  startSidecar()
  createMainWindow()
  // The trigger overlay window must exist for trigger alert text to render —
  // its renderer is what subscribes to trigger:fired broadcasts. It used to be
  // created only by a positioning session or the overlays pop-out button, so
  // after a restart fired triggers showed no text until the user re-opened a
  // trigger editor. Create it at startup; it stays hidden until an alert or
  // positioning session needs it (see overlay:trigger:set-mode).
  createTriggerOverlay()

  // Restore overlays on launch: if the user enabled it, re-open the popout
  // windows that were open when they last quit. Each restores to its saved
  // bounds/lock state via the normal create path, so they come back in place.
  const autoOpen = loadAutoOpenStore()
  if (autoOpen.enabled) {
    for (const name of autoOpen.overlays) {
      if (name === 'trigger') continue
      const win = overlayWindowByName(name)
      if (!win || win.isDestroyed()) createOverlayByName(name)
    }
  }

  setupAutoUpdater()

  // Keep the trigger overlay pinned to its chosen monitor when the layout
  // changes (plug/unplug a display, resolution or DPI change). If the chosen
  // monitor was unplugged, resolveOverlayDisplay() re-homes the overlay to a
  // still-connected one so it never lands on a now-missing display.
  const resizeTriggerOverlay = (): void => {
    if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) return
    triggerOverlayWindow.setBounds(overlayDisplayBounds())
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

// Snapshot which popout overlays are open as the app quits, so "restore
// overlays on launch" can re-open exactly that set next time. Covers the
// tray-Quit / app.quit() path, where before-quit fires while the overlays are
// still alive. The X-to-quit path is covered by closeAllOverlays(); the shared
// guard makes whichever fires first authoritative.
app.on('before-quit', () => {
  snapshotAutoOpenOverlays()
})

app.on('before-quit', (event) => {
  // We're genuinely quitting now — let the main window's close handler through
  // instead of hiding it to the tray.
  isQuittingApp = true
  if (isGracefulQuit || !sidecarProcess) return
  // Electron only waits for before-quit synchronously. To stop the sidecar
  // and confirm exit before we really quit, cancel this pass, run the async
  // cleanup, then quit again with the flag set.
  event.preventDefault()
  isGracefulQuit = true
  stopSidecar().finally(() => app.quit())
})

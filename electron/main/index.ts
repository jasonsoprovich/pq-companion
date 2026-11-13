import { app, BrowserWindow, shell, ipcMain, nativeTheme, dialog, screen, protocol, globalShortcut, Tray, Menu, nativeImage, WebContentsView, session, powerMonitor } from 'electron'
import { join, extname, dirname } from 'path'
import { spawn, ChildProcess } from 'child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync, statSync, unlinkSync } from 'fs'
import { readFile, access, open as fsOpen } from 'fs/promises'
import { constants as fsConstants } from 'fs'
import { homedir } from 'os'
import { autoUpdater } from 'electron-updater'
import { appendLine, closeLogger, initLogger } from './logger'
import { findOtherRunningCopies, otherCopyMessage } from './otherCopy'
import {
  SKIP_ENV,
  findManagedInstall,
  isManagedCopy,
  isWarningSilenced,
  silenceWarning,
  unmanagedCopyMessage,
  unmanagedUpdateMessage,
} from './installCheck'

const isDev = !app.isPackaged

// Some Windows systems hit a known Electron/Chromium bug where hardware-
// accelerated transparent windows render solid black instead of transparent
// (electron/electron#40515, #37088, #10069 — see LIMITATIONS.md §13.2). It's
// most visible on the trigger overlay, which is natively shown/hidden on
// every alert. There's no per-window GPU toggle, so the only mitigation is
// the app-wide switch below — opt-in via Settings > Advanced, persisted to
// ~/.pq-companion (not Electron's userData) because this must run before
// 'ready', before the backend sidecar even starts, and app.getPath('userData')
// is unreliable that early (same reason logger.ts uses homedir() directly).
function hardwareAccelFilePath(): string {
  return join(homedir(), '.pq-companion', 'hardwareAcceleration.json')
}

function loadHardwareAccelDisabled(): boolean {
  try {
    const raw = JSON.parse(readFileSync(hardwareAccelFilePath(), 'utf8')) as { disabled?: boolean }
    return raw.disabled === true
  } catch {
    return false
  }
}

function saveHardwareAccelDisabled(disabled: boolean): void {
  try {
    const dir = join(homedir(), '.pq-companion')
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true })
    writeFileSync(hardwareAccelFilePath(), JSON.stringify({ disabled }, null, 2), 'utf8')
  } catch (err) {
    console.error('[main] Failed to write hardware acceleration pref:', err)
  }
}

if (loadHardwareAccelDisabled()) {
  app.disableHardwareAcceleration()
}

// Initialize file logging as early as possible so any errors during the
// rest of main-process bootstrap are captured. Packaged Windows builds
// have no attached console — without this we lose every console.* call,
// every [backend] line piped from the sidecar, and every unhandledRejection.
// The single-instance lock is taken first so a second launch appends to the
// running instance's log rather than rotating it away (see initLogger).
const gotSingleInstanceLock = app.requestSingleInstanceLock()
const loggerInit = initLogger(app.getVersion(), gotSingleInstanceLock)
if (loggerInit.error) {
  console.warn(`[main] file logging disabled: ${loggerInit.error.message}`)
} else if (loggerInit.logPath) {
  console.log(`[main] electron log: ${loggerInit.logPath}`)
}
console.log(`[main] hardware acceleration: ${loadHardwareAccelDisabled() ? 'disabled' : 'enabled'}`)
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

// Non-renderer helper processes (GPU, utility, network). A GPU-process crash is
// invisible otherwise, yet it's a known way for transparent overlay windows to
// stop painting until reopened, so log it for "overlays disappeared" reports.
app.on('child-process-gone', (_event, details) => {
  const name = details.name ? ` name=${details.name}` : ''
  console.warn(`[main] child process gone: type=${details.type}${name} reason=${details.reason} (exitCode=${details.exitCode})`)
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

// Chromium's native occlusion tracking on Windows marks a window hidden when
// it decides other windows cover it, and stops painting it. Transparent
// overlays then go blank while staying visible/topmost/on-screen, until
// toggled off/on — which matches the "overlays disappeared" reports. The
// overlays are always on top, so they never need occlusion throttling.
app.commandLine.appendSwitch('disable-features', 'CalculateNativeWinOcclusion')

// Under Wine/Proton, Chromium picks up a non-1 DPI scale (e.g. from KDE's
// fractional scaling) that Wine's mouse coordinates don't follow, so clicks
// drift away from the cursor. Pinning the scale to 1 fixed this for a Linux
// reporter; the in-app zoom setting can enlarge the UI instead. An explicit
// --force-device-scale-factor wins; PQC_SCALE_FACTOR=<n> picks another value
// and PQC_SCALE_FACTOR=auto leaves Chromium's own choice alone.
if (isRunningUnderWine() && !app.commandLine.hasSwitch('force-device-scale-factor')) {
  const scale = process.env['PQC_SCALE_FACTOR'] ?? '1'
  if (scale !== 'auto') app.commandLine.appendSwitch('force-device-scale-factor', scale)
}

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
//
// The lock holder can also be a different copy of the app — e.g. an old
// extracted setup folder in Downloads. Then the copy the auto-updater just
// installed quits on every launch while the stale one keeps running (issue
// #160). For packaged Windows builds we check for that before quitting and
// warn the user (see otherCopy.ts). app.exit() rather than app.quit() so the
// before-quit handlers (overlay auto-open snapshot, sidecar stop) never run
// in this instance and can't clobber the running copy's saved state. The lock
// itself is requested up top, before initLogger.
if (!gotSingleInstanceLock) {
  if (process.platform === 'win32' && app.isPackaged) {
    app.whenReady()
      .then(warnIfOtherCopyRunning)
      .catch((err) => appendLine('WARN', `[main] other-copy check failed: ${err}`))
      .finally(() => app.exit(0))
  } else {
    app.quit()
  }
} else {
  app.on('second-instance', () => {
    // A second launch was attempted; bring our window back to the foreground
    // instead of letting a duplicate start. showMainWindow() restores from the
    // tray / un-minimizes / re-creates the window as needed.
    showMainWindow()
  })
}

// warnIfOtherCopyRunning shows a warning when the single-instance lock is held
// by a copy of the app installed in a different folder. A second launch of
// the SAME install finds nothing and stays silent, as before.
async function warnIfOtherCopyRunning(): Promise<void> {
  const others = await findOtherRunningCopies()
  if (others.length === 0) return
  const other = others[0]
  appendLine('WARN', `[main] single-instance lock held by another copy: path=${other.path} version=${other.version || 'unknown'}`)
  const { message, detail } = otherCopyMessage(other, app.getVersion())
  const { response } = await dialog.showMessageBox({
    type: 'warning',
    title: 'PQ Companion',
    message,
    detail,
    buttons: ['OK', 'Show Old Copy in Explorer'],
    defaultId: 0,
    noLink: true,
  })
  if (response === 1) shell.showItemInFolder(other.path)
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
  console.log('[main] tray icon created')
}

function destroyTray(): void {
  if (tray && !tray.isDestroyed()) {
    tray.destroy()
    console.log('[main] tray icon destroyed')
  }
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

type OverlayName = 'dps' | 'hps' | 'buffTimer' | 'detrimTimer' | 'customTimer' | 'trigger' | 'npc' | 'threat' | 'rollTracker' | 'respawnTimer' | 'chChain' | 'chMetronome' | 'discordVoice' | 'liveMap' | 'zoneLockouts' | 'raidReadiness' | 'filteredTimer' | 'killCounter' | 'itemInfo' | 'pet' | 'deathRecap'
type Bounds = { x: number; y: number; width: number; height: number }

// A window's identity for bounds/lock persistence and the generic per-window
// IPC handlers (lock get/set, drag-resize bounds tracking). Every overlay
// type maps 1:1 to its OverlayName except Custom Timers, which can have
// multiple instances — one per user-created timer group — keyed as
// `customTimer:<groupId>`. The bare 'customTimer' key is unchanged: it's
// still the original/default window, so existing users' saved position and
// lock state carry over untouched. Named-group windows are additive and
// deliberately excluded from the OverlayName-keyed systems below (auto-open
// on launch, move-to-display, reset-position) — see createCustomTimerGroupOverlay.
type WindowKey = OverlayName | `customTimer:${string}`

type BoundsStore = Partial<Record<WindowKey, Bounds>>
type LockStore = Partial<Record<WindowKey, boolean>>

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
  // Squarer and larger than the other overlays: this one draws a map, and a
  // narrow strip of a zone footprint is unreadable.
  liveMap: { x: 0, y: 0, width: 380, height: 380 },
  discordVoice: { x: 0, y: 0, width: 220, height: 340 },
  zoneLockouts: { x: 0, y: 0, width: 300, height: 320 },
  raidReadiness: { x: 0, y: 0, width: 280, height: 260 },
  filteredTimer: { x: 0, y: 0, width: 300, height: 380 },
  killCounter: { x: 0, y: 0, width: 280, height: 220 },
  itemInfo: { x: 0, y: 0, width: 380, height: 460 },
  pet: { x: 0, y: 0, width: 300, height: 420 },
  deathRecap: { x: 0, y: 0, width: 340, height: 360 },
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

function getOverlayLocked(name: WindowKey): boolean {
  return loadLockStore()[name] === true
}

function setOverlayLocked(name: WindowKey, locked: boolean): void {
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
//
// A named Custom Timers window can't be represented as a bare OverlayName (its
// identity is a groupId, and re-creating it needs a display name too, which the
// backend — not reachable this early in boot — would otherwise have to supply).
// So an entry is either a plain OverlayName (unchanged, existing behavior) or an
// object carrying the group's WindowKey + the name last seen when it was open.
type AutoOpenGroupEntry = { key: `customTimer:${string}`; groupName: string }
type AutoOpenEntry = OverlayName | AutoOpenGroupEntry
type AutoOpenStore = { enabled: boolean; overlays: AutoOpenEntry[] }

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
function applyInitialOverlayInput(win: BrowserWindow, name: WindowKey): void {
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
const windowToOverlayName = new WeakMap<BrowserWindow, WindowKey>()

const boundsDebounceTimers = new Map<WindowKey, ReturnType<typeof setTimeout>>()

// writeBoundsNow captures and persists the overlay's bounds synchronously.
// Called from the 'close' handler: the debounced persistBounds would schedule a
// timer that fires ~500ms later, by which point the window is destroyed and the
// timer bails on isDestroyed() — losing a move-then-close within that window.
function writeBoundsNow(name: WindowKey, win: BrowserWindow): void {
  const existing = boundsDebounceTimers.get(name)
  if (existing) {
    clearTimeout(existing)
    boundsDebounceTimers.delete(name)
  }
  if (win.isDestroyed()) return
  const b = win.getBounds()
  // A collapsed window is title-bar height; persist its expanded height.
  const height = collapsedEntry(name)?.expandedHeight ?? b.height
  const store = loadBoundsStore()
  store[name] = { x: b.x, y: b.y, width: b.width, height }
  saveBoundsStore(store)
  layoutBoundsChanged(name, store[name] as Bounds)
}

function persistBounds(name: WindowKey, win: BrowserWindow): void {
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

function getRestoredBounds(name: WindowKey, defaults: Bounds): Bounds {
  const store = loadBoundsStore()
  const saved = store[name]
  if (saved && isOnScreen(saved)) return saved
  return defaults
}

function trackOverlayBounds(name: WindowKey, win: BrowserWindow): void {
  win.on('move', () => persistBounds(name, win))
  win.on('resize', () => persistBounds(name, win))
  win.on('close', () => {
    // Users recover a vanished overlay by toggling it off/on, so the window's
    // state right before that close is the best evidence of how it vanished.
    console.log(`[main] overlay ${name} closing: ${describeOverlayState(win)}`)
    writeBoundsNow(name, win) // synchronous: window is about to be destroyed
    // Don't leave a closed overlay armed to re-enter placing mode next open.
    placingOverlays.delete(name)
  })
  watchOverlayVisibility(name, win)
  layoutWindowOpened(name, win)
}

// One-line snapshot of the window properties that decide whether an overlay
// is actually visible on screen.
function describeOverlayState(win: BrowserWindow): string {
  if (win.isDestroyed()) return 'destroyed'
  const b = win.getBounds()
  return [
    `visible=${win.isVisible()}`,
    `minimized=${win.isMinimized()}`,
    `topmost=${win.isAlwaysOnTop()}`,
    `onScreen=${isOnScreen(b)}`,
    `opacity=${win.getOpacity()}`,
    `bounds=${b.x},${b.y} ${b.width}x${b.height}`,
  ].join(' ')
}

// Overlays have skipTaskbar set, so once Windows minimizes or hides one
// (Win+D / Win+M, a game's display-mode switch) there's no taskbar entry to
// bring it back — it just vanishes while its dashboard toggle stays on. An
// overlay should never be minimized, so drop the minimize box (Win+D/Win+M
// skip such windows), restore immediately if it happens anyway, and log every
// visibility edge so the next "overlays disappeared" report shows which one.
function watchOverlayVisibility(name: WindowKey, win: BrowserWindow): void {
  win.setMinimizable(false)
  win.on('minimize', () => {
    console.warn(`[main] overlay ${name} minimized; restoring`)
    win.restore()
  })
  win.on('restore', () => console.log(`[main] overlay ${name} restored`))
  win.on('hide', () => {
    // Hide-all hides on purpose; only unexpected hides are worth a warning.
    if (userHiddenOverlays.has(win)) console.log(`[main] overlay ${name} hidden by hide-all`)
    else console.warn(`[main] overlay ${name} hidden: ${describeOverlayState(win)}`)
  })
  win.on('always-on-top-changed', (_e, isOnTop) => {
    console.log(`[main] overlay ${name} always-on-top-changed: ${isOnTop}`)
  })
  watchCompositorMessages(`overlay ${name}`, win)
}

// Win32 messages Windows broadcasts to every top-level window.
const WM_DISPLAYCHANGE = 0x007e
const WM_THEMECHANGED = 0x031a
const WM_DWMCOMPOSITIONCHANGED = 0x031e

// Logs system-wide redraw events: the DWM compositor restarting (or toggling)
// and display-mode switches (e.g. a game entering/leaving exclusive
// fullscreen). Either can leave transparent windows blank in place while
// Electron still reports them visible — the shape of the "every overlay
// vanished at once" reports. Explorer's TaskbarCreated is a registered message
// whose id can't be resolved without native code, so it isn't covered.
function watchCompositorMessages(label: string, win: BrowserWindow): void {
  if (process.platform !== 'win32') return
  win.hookWindowMessage(WM_DWMCOMPOSITIONCHANGED, () => {
    console.warn(`[main] ${label}: WM_DWMCOMPOSITIONCHANGED`)
  })
  win.hookWindowMessage(WM_THEMECHANGED, () => {
    console.warn(`[main] ${label}: WM_THEMECHANGED`)
  })
  win.hookWindowMessage(WM_DISPLAYCHANGE, (wParam, lParam) => {
    // lParam packs the new resolution (low word width, high word height);
    // wParam is the color depth in bits per pixel.
    const packed = lParam.readUInt32LE(0)
    const bpp = wParam.readUInt32LE(0)
    console.warn(
      `[main] ${label}: WM_DISPLAYCHANGE ${packed & 0xffff}x${packed >>> 16} ${bpp}bpp`,
    )
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

// ── App Backup/Restore: Electron-side window & overlay state ────────────────
// The five small userData JSON files above (bounds, lock, auto-open, display
// pin, main window state) hold everything about *how the desktop shell is
// laid out*. None of it lives in the Go backend's user.db/config.yaml, so the
// App Backup/Restore feature (BackupManagerPage.tsx) can't include it without
// this bridge to the renderer. 'app:client-state:get' is read on demand when
// the user clicks Export; the import path writes a pending-client-state.json
// next to user.db that this process reads BEFORE creating any window on its
// next boot (see the whenReady handler) — Electron rewrites these files on
// every quit, so applying them any later would just get clobbered by the
// relaunching process's own shutdown writes.
type ClientStateElectron = {
  overlayBounds?: BoundsStore
  overlayLockState?: LockStore
  overlayAutoOpen?: AutoOpenStore
  overlayDisplay?: OverlayDisplayPref
  mainWindowState?: MainWindowState
  overlayHotkeys?: HotkeyStore
  overlayCollapsed?: CollapseStore
  overlayLayouts?: LayoutStore
}

const CLIENT_STATE_FILES: { [K in keyof ClientStateElectron]-?: () => string } = {
  overlayBounds: boundsFilePath,
  overlayLockState: lockFilePath,
  overlayAutoOpen: autoOpenFilePath,
  overlayDisplay: overlayDisplayFilePath,
  mainWindowState: mainWindowStateFilePath,
  overlayHotkeys: hotkeysFilePath,
  overlayCollapsed: collapseFilePath,
  overlayLayouts: layoutsFilePath,
}

// Matches appbackup.Manager.pendingClientStatePath() on the Go side exactly —
// both read/write the same file. Lives directly under ~/.pq-companion (like
// hardwareAccelFilePath above) rather than userData, because it must be
// readable before app.getPath('userData') is reliable this early in boot.
function pendingClientStatePath(): string {
  return join(homedir(), '.pq-companion', 'pending-client-state.json')
}

// Collects the current Electron-side state for an export. Refreshes the
// auto-open set from what's actually open right now (mirroring
// snapshotAutoOpenOverlays below) so an export captures live state rather
// than whatever was true at the app's last quit.
function collectClientStateElectron(): ClientStateElectron {
  const autoOpen = loadAutoOpenStore()
  if (autoOpen.enabled) autoOpen.overlays = currentlyOpenAutoOpenEntries()
  const displayPref = loadOverlayDisplayPref()
  return {
    overlayBounds: loadBoundsStore(),
    overlayLockState: loadLockStore(),
    overlayAutoOpen: autoOpen,
    ...(displayPref ? { overlayDisplay: displayPref } : {}),
    mainWindowState: loadMainWindowState(),
    overlayHotkeys: loadHotkeyStore(),
    overlayCollapsed: loadCollapseStore(),
    overlayLayouts: loadLayoutStore(),
  }
}

// Applies a staged import's Electron-side state, if any. Called once at boot
// before any window is created. Deliberately does NOT delete the pending
// file — 'app:client-state:take-pending' (called by the renderer after
// mount, for the localStorage half) owns that, so this can run first without
// racing the renderer's later read.
function applyPendingClientState(): void {
  try {
    const raw = readFileSync(pendingClientStatePath(), 'utf8')
    const parsed = JSON.parse(raw) as { electron?: ClientStateElectron }
    if (!parsed.electron) return
    for (const key of Object.keys(CLIENT_STATE_FILES) as (keyof ClientStateElectron)[]) {
      const value = parsed.electron[key]
      if (value === undefined) continue
      writeFileSync(CLIENT_STATE_FILES[key](), JSON.stringify(value, null, 2), 'utf8')
    }
  } catch {
    // No pending import — the normal case.
  }
}

// Matches appbackup.Manager.resetSentinelPath() on the Go side — read-only
// here, never written or deleted by this process. The Go backend owns
// consuming it (ApplyPendingReset, run when the sidecar starts moments after
// this) so its own aside/recovery bookkeeping stays authoritative; this just
// needs to know "is a factory reset about to happen" before creating any
// window that would otherwise read the stale overlay/window state.
function resetSentinelPath(): string {
  return join(homedir(), '.pq-companion', '.reset-pending')
}

// A factory reset (unlike a "data" reset, which keeps config.yaml and
// preferences) puts the app back to "as if freshly installed" — including
// the overlay layout and window positions Electron owns. Neither user.db nor
// config.yaml lives in this process, so there's nothing here for a plain
// "data" reset to do. Called once at boot, before any window is created,
// same as applyPendingClientState.
async function applyPendingFactoryResetClientState(): Promise<void> {
  let mode: string
  try {
    mode = readFileSync(resetSentinelPath(), 'utf8').trim()
  } catch {
    return // no pending reset — the normal case
  }
  if (mode !== 'factory') return

  for (const filePathFn of Object.values(CLIENT_STATE_FILES)) {
    try {
      unlinkSync(filePathFn())
    } catch {
      // Already gone / never existed — fine.
    }
  }
  try {
    await session.defaultSession.clearStorageData({ storages: ['localstorage'] })
  } catch (err) {
    console.error('[main] Failed to clear localStorage for factory reset:', err)
  }
}

ipcMain.handle('app:client-state:get', () => collectClientStateElectron())

ipcMain.handle('app:client-state:take-pending', (): Record<string, string> => {
  try {
    const raw = readFileSync(pendingClientStatePath(), 'utf8')
    const parsed = JSON.parse(raw) as { local_storage?: Record<string, string> }
    try {
      unlinkSync(pendingClientStatePath())
    } catch {
      // Already gone / never existed — fine, nothing left to consume.
    }
    return parsed.local_storage ?? {}
  } catch {
    return {}
  }
})

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

// ── Standard popout overlays ─────────────────────────────────────────────────
// Every popout except the screen-spanning trigger alert overlay and Discord
// Voice (which hosts a child web view) is the same frameless, transparent,
// always-on-top window with its own route and minimum size, so one table and
// one create function cover them all.

type StandardOverlay = Exclude<OverlayName, 'trigger' | 'discordVoice'>
type StandardOverlaySpec = {
  route: string // renderer hash route
  minWidth: number
  minHeight: number
  ipc: string // slug in the overlay:<slug>:open|close|toggle channels
}

const STANDARD_OVERLAYS: Record<StandardOverlay, StandardOverlaySpec> = {
  dps: { route: 'dps-overlay-window', minWidth: 260, minHeight: 180, ipc: 'dps' },
  hps: { route: 'hps-overlay-window', minWidth: 260, minHeight: 180, ipc: 'hps' },
  buffTimer: { route: 'buff-timer-window', minWidth: 200, minHeight: 140, ipc: 'bufftimer' },
  filteredTimer: { route: 'filtered-timer-window', minWidth: 240, minHeight: 140, ipc: 'filteredtimer' },
  killCounter: { route: 'kill-counter-window', minWidth: 200, minHeight: 100, ipc: 'killcounter' },
  pet: { route: 'pet-window', minWidth: 220, minHeight: 140, ipc: 'pet' },
  deathRecap: { route: 'death-recap-window', minWidth: 240, minHeight: 120, ipc: 'deathrecap' },
  itemInfo: { route: 'item-info-window', minWidth: 260, minHeight: 160, ipc: 'iteminfo' },
  chChain: { route: 'ch-chain-window', minWidth: 180, minHeight: 120, ipc: 'chchain' },
  chMetronome: { route: 'ch-metronome-window', minWidth: 180, minHeight: 180, ipc: 'chmetronome' },
  detrimTimer: { route: 'detrim-timer-window', minWidth: 200, minHeight: 140, ipc: 'detrimtimer' },
  customTimer: { route: 'custom-timer-window', minWidth: 200, minHeight: 140, ipc: 'customtimer' },
  // Higher floor than the list overlays: below roughly this size the zone
  // outline stops being recognisable and the window is just noise.
  liveMap: { route: 'live-map-window', minWidth: 220, minHeight: 220, ipc: 'livemap' },
  respawnTimer: { route: 'respawn-timer-window', minWidth: 200, minHeight: 140, ipc: 'respawntimer' },
  zoneLockouts: { route: 'zone-lockouts-window', minWidth: 200, minHeight: 140, ipc: 'zonelockouts' },
  raidReadiness: { route: 'raid-readiness-window', minWidth: 200, minHeight: 140, ipc: 'raidreadiness' },
  npc: { route: 'npc-overlay-window', minWidth: 280, minHeight: 200, ipc: 'npc' },
  threat: { route: 'threat-overlay-window', minWidth: 200, minHeight: 150, ipc: 'threat' },
  rollTracker: { route: 'roll-tracker-window', minWidth: 240, minHeight: 160, ipc: 'rolltracker' },
}

const standardWindows = new Map<StandardOverlay, BrowserWindow>()

function isStandardOverlay(name: string): name is StandardOverlay {
  return Object.prototype.hasOwnProperty.call(STANDARD_OVERLAYS, name)
}

// The live window for a standard overlay, or null when it isn't open.
function stdWin(name: StandardOverlay): BrowserWindow | null {
  const win = standardWindows.get(name)
  return win && !win.isDestroyed() ? win : null
}

function loadOverlayRoute(win: BrowserWindow, route: string): void {
  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    win.loadURL(`${rendererUrl}/#/${route}`)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'), { hash: `/${route}` })
  }
}

// Opens a standard overlay, or brings an open one forward. Never takes focus:
// these open from hotkeys and in-game events while EQ is the active window,
// and stealing focus mid-fight is worse than any blank-frame flash.
// showInactive()/moveTop() raise the window without activating it.
function createStandardOverlay(name: StandardOverlay): void {
  const existing = stdWin(name)
  if (existing) {
    showOverlayWindow(existing)
    return
  }

  const spec = STANDARD_OVERLAYS[name]
  const { x, y, width, height } = getRestoredBounds(name, OVERLAY_DEFAULTS[name])
  const win = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: spec.minWidth,
    minHeight: spec.minHeight,
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
  standardWindows.set(name, win)

  win.once('ready-to-show', () => {
    if (!win.isDestroyed()) win.showInactive()
  })

  // Keep it above fullscreen apps on macOS/Windows.
  win.setAlwaysOnTop(true, 'screen-saver')
  win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(win, name)
  applyInitialOverlayInput(win, name)
  trackOverlayBounds(name, win)
  restoreCollapsedState(win, name)
  loadOverlayRoute(win, spec.route)

  win.on('closed', () => {
    if (standardWindows.get(name) === win) standardWindows.delete(name)
  })
}

// Named Custom Timers windows (one per user-created timer group), keyed by
// TimerGroup.id. Separate from the standard 'customTimer' window (the original/default
// window) so its own behavior stays untouched by this additive feature.
// Named windows get full parity with every other overlay — auto-open on
// launch, move-to-display, reset-position — via the WindowKey-aware
// versions of those systems below (see overlayWindowByName, isValidWindowKey).
const customTimerGroupWindows = new Map<string, BrowserWindow>()
// Group name for each currently-open group window, tracked alongside the
// window itself so "restore on launch" can re-create it (createCustomTimerGroupOverlay
// needs a display name, not just the id) without querying the Go backend —
// which isn't up yet this early in boot. Refreshed every time the window is
// (re)opened, so it's current as of the last open even across a rename.
const customTimerGroupNames = new Map<string, string>()
let triggerOverlayWindow: BrowserWindow | null = null
// Periodically reasserts the trigger overlay's z-order while it's visible
// (passthrough or interactive mode). Unlike every other overlay — which is
// shown once and left up for the whole session, so a single setAlwaysOnTop
// at creation sticks — the trigger overlay is natively hidden/shown on every
// single alert fire, and Windows' topmost enforcement isn't reliably durable
// across that, especially when the game is fullscreen on a non-primary
// monitor. Same fix and reasoning as discordVoiceTopInterval below (issue
// #150): setAlwaysOnTop's relativeLevel only affects macOS, so a plain
// moveTop() interval is what actually holds topmost on Windows.
let triggerOverlayTopInterval: ReturnType<typeof setInterval> | null = null
let discordVoiceOverlayWindow: BrowserWindow | null = null
// The Discord StreamKit page embedded inside discordVoiceOverlayWindow, added
// as a child view above the window's own React chrome (see createDiscordVoiceOverlay
// and issue #150). null until the renderer supplies a valid URL — there's
// nothing to embed until then. lastDiscordVoiceUrl avoids a needless reload
// when the renderer re-sends the same URL (e.g. after a config:updated poll).
let discordVoiceChildView: WebContentsView | null = null
let lastDiscordVoiceUrl: string | null = null
// Periodically reasserts the Discord Voice window's z-order above every
// other overlay window (issue #150 feedback: the roster should stay visible
// even if another overlay is dragged on top of it). setAlwaysOnTop's
// relativeLevel argument only affects macOS window levels, so a plain
// interval calling moveTop() is the mechanism that actually works on Windows,
// which is the only platform this app ships a build for.
let discordVoiceTopInterval: ReturnType<typeof setInterval> | null = null
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

// False for a copy the NSIS installer doesn't manage (portable zip, copied
// folder). Its updater would run the installer into the default folder and
// leave a second copy behind, so it only reports updates — see installCheck.ts.
let canSelfUpdate = true

function setupAutoUpdater(): void {
  if (isDev) return // not applicable outside a packaged build

  autoUpdater.autoDownload = false
  autoUpdater.autoInstallOnAppQuit = canSelfUpdate

  autoUpdater.on('checking-for-update', () => {
    console.log('[updater] Checking for updates…')
  })

  autoUpdater.on('update-available', (info) => {
    console.log(`[updater] Update available: ${info.version}`)
    if (!canSelfUpdate) {
      mainWindow?.webContents.send('updater:error', unmanagedUpdateMessage(info.version))
      return
    }
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
  // down — once destroyed, currentlyOpenAutoOpenEntries() would report nothing.
  snapshotAutoOpenOverlays()
  // Quitting isn't the user closing windows; keep the active layout intact.
  layoutsFrozen = true
  // Driven by RESETTABLE_OVERLAYS/overlayWindowByName (not a hand-maintained
  // array) so every overlay type gets closed automatically, including ones
  // added after this function was written — discordVoice was previously
  // omitted from a manual list and its window leaked past app quit.
  for (const name of RESETTABLE_OVERLAYS) {
    const win = overlayWindowByName(name)
    if (win && !win.isDestroyed()) win.destroy()
  }
  if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed()) triggerOverlayWindow.destroy()
  for (const win of customTimerGroupWindows.values()) {
    if (win && !win.isDestroyed()) win.destroy()
  }
}

// Linux users run the Windows build under Wine/Proton. There the custom title
// bar (titleBarStyle 'hidden') breaks: Electron's non-client-area override
// isn't honoured the way real Windows honours it, so the window manager draws
// its own title bar on top and mouse hit-testing ends up offset from what's
// rendered by roughly the resize-border height (~10px). Under Wine we fall
// back to a native frame and the renderer hides its own TitleBar.
// PQC_NATIVE_FRAME=1 / =0 forces it either way (e.g. via Steam launch options).
function isRunningUnderWine(): boolean {
  if (process.platform !== 'win32') return false
  // Wine passes the Unix environment through to Windows processes.
  if (process.env['WINEPREFIX'] || process.env['STEAM_COMPAT_DATA_PATH'] || process.env['WINELOADER']) {
    return true
  }
  // Every Wine prefix ships builtin program stubs in system32.
  const sysRoot = process.env['SystemRoot'] ?? 'C:\\windows'
  return existsSync(join(sysRoot, 'system32', 'winecfg.exe'))
}

function useNativeMainFrame(): boolean {
  const override = process.env['PQC_NATIVE_FRAME']
  if (override === '1') return true
  if (override === '0') return false
  return isRunningUnderWine()
}

function createMainWindow(): void {
  nativeTheme.themeSource = 'dark'

  const nativeFrame = useNativeMainFrame()
  if (nativeFrame) console.log('[main] using native window frame (Wine detected or PQC_NATIVE_FRAME=1)')

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
    // A native frame on Windows also brings Electron's default File/Edit/View
    // menu bar; auto-hide keeps its accelerators without showing it.
    ...(nativeFrame
      ? { autoHideMenuBar: true }
      : { titleBarStyle: process.platform === 'darwin' ? ('hiddenInset' as const) : ('hidden' as const) }),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      // Read synchronously by the preload so TitleBar never flashes.
      additionalArguments: nativeFrame ? ['--pqc-native-frame'] : []
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
      console.log('[main] main window hidden to tray')
    }
  })

  // A main-process or renderer stall shows up to the user as a frozen/ghosted
  // taskbar entry; log both edges so a log export shows when and how long.
  mainWindow.on('unresponsive', () => console.warn('[main] main window unresponsive'))
  watchCompositorMessages('main', mainWindow)
  mainWindow.on('responsive', () => console.log('[main] main window responsive again'))

  mainWindow.on('closed', () => {
    closeAllOverlays()
    mainWindow = null
  })
}

// customTimerWindowKey returns the bounds/lock persistence key for a named
// timer group's window. Empty groupId is intentionally not routed here —
// callers use createStandardOverlay('customTimer')/'customTimer' for the default window.
function customTimerWindowKey(groupId: string): `customTimer:${string}` {
  return `customTimer:${groupId}`
}

// Opens (or focuses) a named Custom Timers window for the given TimerGroup.
// Mirrors createStandardOverlay('customTimer') but keyed by groupId so multiple can
// coexist alongside the original/default window. groupName is passed through
// to the renderer via the URL only for a window-title-ish display; the
// renderer fetches the authoritative group list itself.
function createCustomTimerGroupOverlay(groupId: string, groupName: string): void {
  if (!groupId) return
  customTimerGroupNames.set(groupId, groupName)
  const existing = customTimerGroupWindows.get(groupId)
  if (existing && !existing.isDestroyed()) {
    // Bring forward without taking focus (see createStandardOverlay).
    showOverlayWindow(existing)
    return
  }

  const key = customTimerWindowKey(groupId)
  const { x, y, width, height } = getRestoredBounds(key, OVERLAY_DEFAULTS.customTimer)
  const win = new BrowserWindow({
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
  customTimerGroupWindows.set(groupId, win)

  win.once('ready-to-show', () => {
    if (!win.isDestroyed()) win.showInactive()
  })

  win.setAlwaysOnTop(true, 'screen-saver')
  win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(win, key)
  applyInitialOverlayInput(win, key)
  trackOverlayBounds(key, win)
  restoreCollapsedState(win, key)

  const query = `group=${encodeURIComponent(groupId)}&name=${encodeURIComponent(groupName)}`
  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    win.loadURL(`${rendererUrl}/#/custom-timer-window?${query}`)
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: `/custom-timer-window?${query}`,
    })
  }

  win.on('closed', () => {
    customTimerGroupWindows.delete(groupId)
    // Name is kept even after close — resetOverlayPosition/move-to-display
    // work on closed overlays too (matching every other overlay type), and
    // a closed-but-still-in-the-auto-open-snapshot group needs its name to
    // survive until the next explicit open/snapshot overwrites it.
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
    stopTriggerOverlayTopInterval()
  })
}

// ── Discord Voice overlay window ─────────────────────────────────────────────
// Embeds Discord's own hosted StreamKit Overlay page instead of talking to
// Discord's API ourselves — see issue #150. The renderer
// (DiscordVoiceOverlayWindowPage) draws the usual title-bar chrome; this
// additionally attaches a WebContentsView as a child of the window, positioned
// below the header, that renders the live Discord content. setIgnoreMouseEvents
// (lock mode) is a native window-level property, so it click-throughs the
// child view too without any special-casing.

const DISCORD_VOICE_HEADER_HEIGHT = 30
const STREAMKIT_VOICE_URL_RE = /^https:\/\/streamkit\.discord\.com\/overlay\/voice\/\d+\/\d+(?:[/?#].*)?$/

function isValidStreamKitVoiceUrl(url: string): boolean {
  return STREAMKIT_VOICE_URL_RE.test(url)
}

function repositionDiscordVoiceChildView(): void {
  if (!discordVoiceOverlayWindow || discordVoiceOverlayWindow.isDestroyed() || !discordVoiceChildView) return
  const { width, height } = discordVoiceOverlayWindow.getContentBounds()
  discordVoiceChildView.setBounds({
    x: 0,
    y: DISCORD_VOICE_HEADER_HEIGHT,
    width,
    height: Math.max(0, height - DISCORD_VOICE_HEADER_HEIGHT),
  })
}

// Attach (or update) the embedded Discord content for the given window. Skips
// the reload if the URL hasn't changed from what's already loaded — the
// renderer re-sends this on every config poll, not just on real changes.
function setDiscordVoiceContent(win: BrowserWindow, url: string): void {
  if (!isValidStreamKitVoiceUrl(url)) return
  if (!discordVoiceChildView) {
    discordVoiceChildView = new WebContentsView({
      webPreferences: { contextIsolation: true, nodeIntegration: false },
    })
    // A WebContentsView has its own background color, independent of the
    // parent BrowserWindow's `transparent: true` — it defaults to opaque
    // white. StreamKit's page already renders its own body as transparent
    // (that's the whole point — it's built for OBS/XSplit browser sources),
    // but that only shows through if the embedder's compositor background is
    // transparent too. Without this, the overlay above the game shows a solid
    // white rectangle instead of letting the game show through around the
    // roster.
    discordVoiceChildView.setBackgroundColor('#00000000')
    win.contentView.addChildView(discordVoiceChildView)
    repositionDiscordVoiceChildView()
  }
  if (url !== lastDiscordVoiceUrl) {
    lastDiscordVoiceUrl = url
    discordVoiceChildView.webContents.loadURL(url)
  }
}

// Detach the embedded Discord content (feature disabled or URL cleared while
// the popout is open) so stale voice data doesn't linger on screen.
function clearDiscordVoiceContent(win: BrowserWindow): void {
  if (discordVoiceChildView) {
    win.contentView.removeChildView(discordVoiceChildView)
    discordVoiceChildView.webContents.close()
  }
  discordVoiceChildView = null
  lastDiscordVoiceUrl = null
}

function createDiscordVoiceOverlay(): void {
  if (discordVoiceOverlayWindow && !discordVoiceOverlayWindow.isDestroyed()) {
    // Bring forward without taking focus (see createStandardOverlay).
    showOverlayWindow(discordVoiceOverlayWindow)
    return
  }

  const { x, y, width, height } = getRestoredBounds('discordVoice', OVERLAY_DEFAULTS.discordVoice)
  discordVoiceOverlayWindow = new BrowserWindow({
    x,
    y,
    width,
    height,
    minWidth: 180,
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

  discordVoiceOverlayWindow.once('ready-to-show', () => {
    discordVoiceOverlayWindow?.showInactive()
    discordVoiceOverlayWindow?.moveTop()
  })

  discordVoiceOverlayWindow.setAlwaysOnTop(true, 'screen-saver')
  if (discordVoiceTopInterval) clearInterval(discordVoiceTopInterval)
  discordVoiceTopInterval = setInterval(() => {
    if (discordVoiceOverlayWindow && !discordVoiceOverlayWindow.isDestroyed()) {
      discordVoiceOverlayWindow.moveTop()
    }
  }, 2000)
  discordVoiceOverlayWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
  windowToOverlayName.set(discordVoiceOverlayWindow, 'discordVoice')
  applyInitialOverlayInput(discordVoiceOverlayWindow, 'discordVoice')
  trackOverlayBounds('discordVoice', discordVoiceOverlayWindow)
  discordVoiceOverlayWindow.on('resize', repositionDiscordVoiceChildView)

  if (isDev) {
    const rendererUrl = process.env['ELECTRON_RENDERER_URL'] ?? 'http://localhost:5173'
    discordVoiceOverlayWindow.loadURL(`${rendererUrl}/#/discord-voice-overlay-window`)
  } else {
    discordVoiceOverlayWindow.loadFile(join(__dirname, '../renderer/index.html'), {
      hash: '/discord-voice-overlay-window',
    })
  }

  discordVoiceOverlayWindow.on('closed', () => {
    // Destroying the parent window doesn't tear down an attached child
    // WebContentsView's own renderer — without this its webContents (and the
    // Discord page loaded into it) leaks as an orphaned target.
    discordVoiceChildView?.webContents.close()
    if (discordVoiceTopInterval) {
      clearInterval(discordVoiceTopInterval)
      discordVoiceTopInterval = null
    }
    discordVoiceOverlayWindow = null
    discordVoiceChildView = null
    lastDiscordVoiceUrl = null
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
//
// Chromium keys zoom by URL, and for file:// (packaged builds) that key is the
// full URL including the #route — so every in-app navigation silently dropped
// the main window back to 100%. Remember the requested factor per webContents
// and re-assert it after each same-document navigation.
const requestedZoom = new WeakMap<Electron.WebContents, number>()
ipcMain.handle('window:set-zoom', (event, factor: number) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win || win.isDestroyed()) return
  const clamped = Math.max(0.7, Math.min(2.0, factor || 1))
  const wc = win.webContents
  if (!requestedZoom.has(wc)) {
    wc.on('did-navigate-in-page', () => {
      const f = requestedZoom.get(wc)
      if (f !== undefined && wc.getZoomFactor() !== f) wc.setZoomFactor(f)
    })
  }
  requestedZoom.set(wc, clamped)
  wc.setZoomFactor(clamped)
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

// Open/close/toggle channels for every standard overlay
// (overlay:<slug>:open|close|toggle), generated from STANDARD_OVERLAYS.
for (const [name, spec] of Object.entries(STANDARD_OVERLAYS) as Array<[StandardOverlay, StandardOverlaySpec]>) {
  ipcMain.handle(`overlay:${spec.ipc}:open`, () => {
    createStandardOverlay(name)
  })
  ipcMain.handle(`overlay:${spec.ipc}:close`, () => {
    stdWin(name)?.close()
  })
  ipcMain.handle(`overlay:${spec.ipc}:toggle`, () => {
    toggleOverlayWindow(stdWin(name), () => createStandardOverlay(name))
  })
}

// Item Info's auto-show on an in-game item click. Same as open (which never
// takes focus either), kept as its own channel for the existing caller.
ipcMain.handle('overlay:iteminfo:show', () => {
  createStandardOverlay('itemInfo')
})

// Named timer-group Custom Timers windows — parameterized by groupId so a
// single set of channels covers any number of groups, rather than one
// channel triple per group (which isn't knowable ahead of time anyway,
// since groups are user-created).
ipcMain.handle('overlay:customtimer:group:open', (_event, groupId: string, groupName: string) => {
  createCustomTimerGroupOverlay(groupId, groupName)
})

ipcMain.handle('overlay:customtimer:group:close', (_event, groupId: string) => {
  const win = customTimerGroupWindows.get(groupId)
  if (win && !win.isDestroyed()) win.close()
})

ipcMain.handle('overlay:customtimer:group:toggle', (_event, groupId: string, groupName: string) => {
  toggleOverlayWindow(customTimerGroupWindows.get(groupId) ?? null, () =>
    createCustomTimerGroupOverlay(groupId, groupName),
  )
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

// Starts/stops the periodic moveTop() reassertion (see triggerOverlayTopInterval
// above) to match the window's actual visibility — no point reasserting z-order
// on a hidden window, and leaving the interval running app-wide would outlive
// nearly every session (the overlay is hidden the vast majority of the time).
function startTriggerOverlayTopInterval(): void {
  if (triggerOverlayTopInterval) return
  triggerOverlayTopInterval = setInterval(() => {
    if (triggerOverlayWindow && !triggerOverlayWindow.isDestroyed() && triggerOverlayWindow.isVisible()) {
      triggerOverlayWindow.moveTop()
    }
  }, 2000)
}

function stopTriggerOverlayTopInterval(): void {
  if (triggerOverlayTopInterval) {
    clearInterval(triggerOverlayTopInterval)
    triggerOverlayTopInterval = null
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
    win.moveTop()
    startTriggerOverlayTopInterval()
    return
  }
  if (mode === 'passthrough') {
    win.setIgnoreMouseEvents(true, { forward: true })
    if (!win.isVisible()) win.showInactive()
    win.moveTop()
    startTriggerOverlayTopInterval()
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
  stopTriggerOverlayTopInterval()
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

ipcMain.handle('overlay:discordvoice:open', () => {
  createDiscordVoiceOverlay()
})

ipcMain.handle('overlay:discordvoice:close', () => {
  if (discordVoiceOverlayWindow && !discordVoiceOverlayWindow.isDestroyed()) {
    discordVoiceOverlayWindow.close()
  }
})

ipcMain.handle('overlay:discordvoice:toggle', () => {
  toggleOverlayWindow(discordVoiceOverlayWindow, createDiscordVoiceOverlay)
})

// Called by DiscordVoiceOverlayWindowPage once it has read the saved
// StreamKit URL out of Config — main process never talks to the Go backend's
// config store itself, so it has to be handed the URL rather than fetching it.
ipcMain.handle('overlay:discordvoice:set-url', (event, url: string) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (win) setDiscordVoiceContent(win, url)
})

ipcMain.handle('overlay:discordvoice:clear', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (win) clearDiscordVoiceContent(win)
})

// ── IPC handlers — bulk popout control ───────────────────────────────────────

// User-managed popout windows. Excludes the trigger alert overlay: it's created
// at startup and always present (just hidden until an alert fires), so counting
// it would make "any popout open" perpetually true — wrongly starting the bulk
// button on "Close All Popouts" with nothing actually popped out. Closing it
// would also stop trigger alerts from rendering, so it's left alone by the
// bulk close as well.
// Re-applies the topmost flag to every open overlay. Windows can drop a
// window out of the topmost band (display wake, resume, Explorer restart),
// and a one-time setAlwaysOnTop at creation never recovers from that. Never
// shows a hidden window — the trigger overlay is hidden whenever it's idle.
// Discord Voice goes last so it stays above the rest (issue #150).
function reassertOverlaysOnTop(): void {
  for (const win of overlaysInStackOrder()) {
    win.setAlwaysOnTop(true, 'screen-saver')
    if (win.isVisible()) win.moveTop()
  }
}

// Every overlay bottom-to-top: popouts, then the trigger overlay so alerts
// sit above them, then Discord Voice so its roster stays above everything
// (issue #150).
function overlaysInStackOrder(): BrowserWindow[] {
  const wins = userPopoutWindows().filter((w) => w !== discordVoiceOverlayWindow)
  if (triggerOverlayWindow) wins.push(triggerOverlayWindow)
  if (discordVoiceOverlayWindow) wins.push(discordVoiceOverlayWindow)
  return wins.filter((w) => !w.isDestroyed())
}

// Re-raises every visible overlay to the top of the topmost band on an
// interval. Being topmost isn't enough: another topmost window (the game, via
// its windowed-mode wrapper) can be stacked above ours later, burying every
// overlay while each still reports visible + topmost — the Grimrose reports,
// where toggling an overlay off/on (a fresh window lands on top) is the only
// recovery. The trigger and Discord Voice overlays already did this for the
// same reason. moveTop() is SetWindowPos(HWND_TOP) with SWP_NOACTIVATE, so it
// never takes focus from the game. It also passes SWP_SHOWWINDOW, hence the
// isVisible() guard: hidden overlays (the idle trigger overlay) stay hidden.
const OVERLAY_RAISE_INTERVAL_MS = 2000

function startOverlayRaiseInterval(): void {
  setInterval(() => {
    for (const win of overlaysInStackOrder()) {
      if (win.isVisible() && !win.isMinimized()) win.moveTop()
    }
  }, OVERLAY_RAISE_INTERVAL_MS)
}

function userPopoutWindows(): BrowserWindow[] {
  const fixed = [
    ...standardWindows.values(),
    discordVoiceOverlayWindow,
  ].filter((w): w is BrowserWindow => !!w && !w.isDestroyed())
  return [...fixed, ...customTimerGroupWindows.values()].filter((w) => !w.isDestroyed())
}

ipcMain.handle('overlay:popouts:any-open', () => userPopoutWindows().length > 0)

// ── Hide / show all ──────────────────────────────────────────────────────────
// "Hide all" hides the open popouts without closing them, so they come back
// instantly in place. The trigger alert overlay is never in userPopoutWindows,
// so alerts keep firing while everything else is hidden. Windows hidden this
// way are tracked so a per-overlay toggle can bring one back, and so the
// state survives windows opening or closing in between.
const userHiddenOverlays = new WeakSet<BrowserWindow>()

type PopoutVisibility = { open: number; hidden: number }

function popoutVisibility(): PopoutVisibility {
  const wins = userPopoutWindows()
  return {
    open: wins.length,
    hidden: wins.filter((w) => userHiddenOverlays.has(w) && !w.isVisible()).length,
  }
}

function showOverlayWindow(win: BrowserWindow): void {
  userHiddenOverlays.delete(win)
  if (!win.isVisible()) win.showInactive()
  win.moveTop()
}

function hideAllPopouts(): void {
  for (const win of userPopoutWindows()) {
    if (!win.isVisible()) continue
    userHiddenOverlays.add(win)
    win.hide()
  }
}

function showAllPopouts(): void {
  for (const win of userPopoutWindows()) {
    if (userHiddenOverlays.has(win)) showOverlayWindow(win)
  }
}

// Shared by the sidebar button, the Manage overlays menu and the global
// hotkey: show hidden overlays if any, else hide the open ones, else open the
// active layout.
function toggleAllPopoutsVisibility(): void {
  const { open, hidden } = popoutVisibility()
  if (hidden > 0) showAllPopouts()
  else if (open > 0) hideAllPopouts()
  else openLayoutWindows(activeLayout())
}

// Per-overlay toggle: an overlay hidden by "hide all" comes back rather than
// closing, so toggling one overlay never closes something you can't see.
function toggleOverlayWindow(win: BrowserWindow | null, create: () => void): void {
  if (!win || win.isDestroyed()) create()
  else if (userHiddenOverlays.has(win) && !win.isVisible()) showOverlayWindow(win)
  else win.close()
}

ipcMain.handle('overlay:popouts:visibility', () => popoutVisibility())

// ── Collapse to title bar ────────────────────────────────────────────────────
// A popout can fold down to just its title bar (chevron in its header). The
// top edge stays put and the window shrinks to the header's height; vertical
// resizing is pinned while collapsed. The state, the header height and the
// expanded height persist per window, and the bounds store always keeps the
// expanded height, so turning the feature off or resetting never strands a
// window at title-bar size. Discord Voice (child web view) doesn't collapse.

type CollapseEntry = { collapsed: boolean; headerHeight: number; expandedHeight: number }
type CollapseStore = Partial<Record<WindowKey, CollapseEntry>>

function collapseFilePath(): string {
  return join(app.getPath('userData'), 'overlay-collapsed.json')
}

let collapseStoreCache: CollapseStore | null = null

function loadCollapseStore(): CollapseStore {
  if (collapseStoreCache) return collapseStoreCache
  try {
    collapseStoreCache = JSON.parse(readFileSync(collapseFilePath(), 'utf-8')) as CollapseStore
  } catch {
    collapseStoreCache = {}
  }
  return collapseStoreCache
}

function saveCollapseStore(): void {
  try {
    writeFileSync(collapseFilePath(), JSON.stringify(loadCollapseStore(), null, 2), 'utf-8')
  } catch (err) {
    console.warn('[main] failed to save overlay collapse state', err)
  }
}

function collapsedEntry(name: WindowKey): CollapseEntry | null {
  const e = loadCollapseStore()[name]
  return e?.collapsed ? e : null
}

function minSizeFor(name: WindowKey): { minWidth: number; minHeight: number } {
  const base = name.startsWith('customTimer:') ? 'customTimer' : name
  return isStandardOverlay(base) ? STANDARD_OVERLAYS[base] : { minWidth: 180, minHeight: 120 }
}

function applyCollapsedSize(win: BrowserWindow, name: WindowKey, headerHeight: number): void {
  const { width } = win.getBounds()
  win.setMinimumSize(minSizeFor(name).minWidth, headerHeight)
  win.setMaximumSize(100_000, headerHeight)
  win.setSize(width, headerHeight)
}

function applyExpandedSize(win: BrowserWindow, name: WindowKey, height: number): void {
  const { width } = win.getBounds()
  const min = minSizeFor(name)
  win.setMaximumSize(0, 0) // 0 = no limit
  win.setMinimumSize(min.minWidth, min.minHeight)
  win.setSize(width, Math.max(height, min.minHeight))
}

// Called right after a popout is created, before it shows, so a window that
// was collapsed last session reopens collapsed without a flash.
function restoreCollapsedState(win: BrowserWindow, name: WindowKey): void {
  const e = collapsedEntry(name)
  if (e) applyCollapsedSize(win, name, e.headerHeight)
}

function setOverlayCollapsed(win: BrowserWindow, name: WindowKey, collapsed: boolean, headerHeight: number): void {
  const store = loadCollapseStore()
  const cur = store[name]
  if (collapsed) {
    const expandedHeight = cur?.collapsed ? cur.expandedHeight : win.getBounds().height
    store[name] = { collapsed: true, headerHeight, expandedHeight }
    applyCollapsedSize(win, name, headerHeight)
  } else {
    const height = cur?.collapsed ? cur.expandedHeight : win.getBounds().height
    if (cur) store[name] = { ...cur, collapsed: false }
    applyExpandedSize(win, name, height)
  }
  saveCollapseStore()
  layoutCollapseChanged(name)
  writeBoundsNow(name, win)
}

// Clears collapse for a window being reset to default bounds.
function clearOverlayCollapsed(name: WindowKey, win: BrowserWindow | null): void {
  const store = loadCollapseStore()
  if (!store[name]?.collapsed) return
  store[name] = { ...(store[name] as CollapseEntry), collapsed: false }
  saveCollapseStore()
  layoutCollapseChanged(name)
  if (win && !win.isDestroyed()) {
    const min = minSizeFor(name)
    win.setMaximumSize(0, 0)
    win.setMinimumSize(min.minWidth, min.minHeight)
    win.webContents.send('overlay:collapse-changed', false)
  }
}

ipcMain.handle('overlay:collapse:get', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  const name = win ? windowToOverlayName.get(win) : undefined
  return name ? collapsedEntry(name) !== null : false
})

// headerFraction = title bar height / window content height, measured by the
// renderer. Sent as a fraction so CSS zoom and display scaling cancel out.
ipcMain.handle('overlay:collapse:set', (event, collapsed: boolean, headerFraction: number) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  const name = win ? windowToOverlayName.get(win) : undefined
  if (!win || !name || name === 'discordVoice' || name === 'trigger') return false
  const contentHeight = win.getContentBounds().height
  const frac = Number.isFinite(headerFraction) && headerFraction > 0 ? Math.min(headerFraction, 1) : 0
  const headerHeight = Math.max(16, Math.min(96, Math.round(frac * contentHeight) || 28))
  setOverlayCollapsed(win, name, collapsed === true, headerHeight)
  return collapsed === true
})

// ── Global overlay hotkeys ───────────────────────────────────────────────────
// User-bound key combos registered with the OS via globalShortcut, so they
// fire while EverQuest has focus — and never take focus from it: every action
// below shows windows with showInactive(). The trade-off (explained in
// Settings → Hotkeys): while a combo is bound here, EQ doesn't receive it.
// Nothing is bound by default. Stored in userData like the other overlay
// window prefs, since main must register them before any renderer loads.

// 'toggleAll' = the show/hide-all action; 'overlay:<name>' = show/hide one;
// 'layout:<id>' = switch to a layout (see runLayoutHotkey).
type HotkeyStore = { bindings: Record<string, string> }
type HotkeyStatus = 'ok' | 'failed'

function hotkeysFilePath(): string {
  return join(app.getPath('userData'), 'overlay-hotkeys.json')
}

function loadHotkeyStore(): HotkeyStore {
  try {
    const parsed = JSON.parse(readFileSync(hotkeysFilePath(), 'utf-8')) as Partial<HotkeyStore>
    const bindings: Record<string, string> = {}
    for (const [action, acc] of Object.entries(parsed.bindings ?? {})) {
      if (typeof acc === 'string' && acc && isHotkeyAction(action)) bindings[action] = acc
    }
    return { bindings }
  } catch {
    return { bindings: {} }
  }
}

function saveHotkeyStore(store: HotkeyStore): void {
  try {
    writeFileSync(hotkeysFilePath(), JSON.stringify(store, null, 2), 'utf-8')
  } catch (err) {
    console.warn('[main] failed to save overlay hotkeys', err)
  }
}

function isHotkeyAction(action: string): boolean {
  if (action === 'toggleAll') return true
  if (action.startsWith('layout:')) {
    const id = action.slice('layout:'.length)
    return loadLayoutStore().layouts.some((l) => l.id === id)
  }
  if (!action.startsWith('overlay:')) return false
  const name = action.slice('overlay:'.length)
  return name === 'discordVoice' || isStandardOverlay(name)
}

// Show/hide one overlay: open it if closed, show it if hidden, else hide it
// (not close — it comes back instantly, and show-all brings it back too).
function toggleOverlayFromHotkey(name: OverlayName): void {
  const win = overlayWindowByName(name)
  if (!win || win.isDestroyed()) {
    createOverlayByName(name)
  } else if (!win.isVisible()) {
    showOverlayWindow(win)
  } else {
    userHiddenOverlays.add(win)
    win.hide()
  }
}

function runHotkeyAction(action: string): void {
  if (action === 'toggleAll') toggleAllPopoutsVisibility()
  else if (action.startsWith('layout:')) runLayoutHotkey(action.slice('layout:'.length))
  else if (action.startsWith('overlay:')) toggleOverlayFromHotkey(action.slice('overlay:'.length) as OverlayName)
}

let hotkeyStore: HotkeyStore = { bindings: {} }
const registeredHotkeys = new Map<string, string>() // action -> accelerator
let hotkeyStatus: Record<string, HotkeyStatus> = {}
let hotkeysSuspendTimer: ReturnType<typeof setTimeout> | null = null

// (Re)registers every binding. Only our own accelerators are unregistered —
// the trigger overlay's Escape shortcut is left alone.
function applyHotkeys(): void {
  for (const acc of registeredHotkeys.values()) globalShortcut.unregister(acc)
  registeredHotkeys.clear()
  hotkeyStatus = {}
  if (hotkeysSuspendTimer) return
  for (const [action, acc] of Object.entries(hotkeyStore.bindings)) {
    let ok = false
    try {
      // Fails (false, or throws on a malformed combo) when another program
      // — or another of our bindings — already owns the combo.
      ok = globalShortcut.register(acc, () => runHotkeyAction(action))
    } catch {
      ok = false
    }
    if (ok) registeredHotkeys.set(action, acc)
    hotkeyStatus[action] = ok ? 'ok' : 'failed'
  }
}

function hotkeyState(): { bindings: Record<string, string>; status: Record<string, HotkeyStatus> } {
  return { bindings: { ...hotkeyStore.bindings }, status: { ...hotkeyStatus } }
}

function initHotkeys(): void {
  hotkeyStore = loadHotkeyStore()
  applyHotkeys()
}

ipcMain.handle('hotkeys:get', () => hotkeyState())

// accelerator null/empty clears the binding.
ipcMain.handle('hotkeys:set', (_event, action: string, accelerator: string | null) => {
  if (!isHotkeyAction(action)) return hotkeyState()
  if (accelerator) hotkeyStore.bindings[action] = accelerator
  else delete hotkeyStore.bindings[action]
  saveHotkeyStore(hotkeyStore)
  applyHotkeys()
  return hotkeyState()
})

ipcMain.handle('hotkeys:set-many', (_event, bindings: Record<string, string>) => {
  for (const [action, acc] of Object.entries(bindings ?? {})) {
    if (isHotkeyAction(action) && typeof acc === 'string' && acc) hotkeyStore.bindings[action] = acc
  }
  saveHotkeyStore(hotkeyStore)
  applyHotkeys()
  return hotkeyState()
})

// While Settings records a new combo, every hotkey is released so the key
// press reaches the page instead of firing an action. Auto-resumes after 30s
// in case the page goes away mid-recording.
ipcMain.handle('hotkeys:suspend', (_event, suspend: boolean) => {
  if (hotkeysSuspendTimer) clearTimeout(hotkeysSuspendTimer)
  hotkeysSuspendTimer = suspend
    ? setTimeout(() => {
        hotkeysSuspendTimer = null
        applyHotkeys()
      }, 30_000)
    : null
  applyHotkeys()
})
ipcMain.handle('overlay:popouts:toggle-visibility', () => toggleAllPopoutsVisibility())

// ── Overlay layouts ──────────────────────────────────────────────────────────
// A layout is a named set of popout windows with each one's position, size and
// collapse state. Exactly one is active, and it saves itself: opening or
// closing a window, moving, resizing or collapsing it updates the active
// layout. Bulk closes (Close All, quitting, a layout switch) don't, so the set
// survives them. Switching makes the screen match the target: windows not in
// it close, and its windows open or move into place. The per-window stores
// (overlayBounds.json, overlay-collapsed.json) stay what each window opens
// from; a switch writes the target's entries into them first. The trigger
// alert overlay is never part of a layout.
//
// Each layout also keeps the in-app dashboard's arrangement (panel and timer
// window positions, sizes, which are shown). Main only stores it: the
// renderer reads the active layout's and writes back by layout id.

type LayoutWindow = { key: WindowKey; groupName?: string; bounds?: Bounds; collapse?: CollapseEntry }
type DashboardPanel = { x: number; y: number; width: number; height: number; visible: boolean }
type LayoutDashboard = { panels: Record<string, DashboardPanel>; groups: Record<string, DashboardPanel> }
type OverlayLayout = { id: string; name: string; windows: LayoutWindow[]; dashboard?: LayoutDashboard }
// selectionMigrated: the renderer's old "Pop Out All" set (localStorage) has
// been folded into the Default layout, or wasn't needed. dashboardMigrated:
// the same for the dashboard arrangement the renderer kept in localStorage.
type LayoutStore = {
  activeId: string
  layouts: OverlayLayout[]
  selectionMigrated: boolean
  dashboardMigrated: boolean
}
type LayoutSummary = { activeId: string; layouts: Array<{ id: string; name: string; windows: WindowKey[] }> }

// Dashboard panel key → popout window. The old saved selection and the
// dashboard's Pop Out All speak in panel keys.
const PANEL_TO_OVERLAY: Record<string, Exclude<OverlayName, 'trigger'>> = {
  dps: 'dps',
  hps: 'hps',
  buff: 'buffTimer',
  detrim: 'detrimTimer',
  custom: 'customTimer',
  npc: 'npc',
  threat: 'threat',
  rolls: 'rollTracker',
  respawn: 'respawnTimer',
  chChain: 'chChain',
  chMetronome: 'chMetronome',
  discordVoice: 'discordVoice',
  liveMap: 'liveMap',
  zoneLockouts: 'zoneLockouts',
  raidReadiness: 'raidReadiness',
  filtered: 'filteredTimer',
  kills: 'killCounter',
  itemInfo: 'itemInfo',
  pet: 'pet',
  deathRecap: 'deathRecap',
}

const MAX_LAYOUT_NAME = 40

// A window as the renderer names it: a dashboard panel key, or a named timer
// group's window key plus its display name (main has no other way to know it).
type PopoutRequestEntry = string | { key: `customTimer:${string}`; name: string }

function layoutsFilePath(): string {
  return join(app.getPath('userData'), 'overlay-layouts.json')
}

function newLayoutId(): string {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
}

function cleanLayoutName(name: unknown): string {
  const s = typeof name === 'string' ? name.trim().slice(0, MAX_LAYOUT_NAME) : ''
  return s || 'Layout'
}

// A layout entry carrying the window's current saved position and collapse.
function withLiveState(w: LayoutWindow): LayoutWindow {
  const bounds = loadBoundsStore()[w.key]
  const collapse = loadCollapseStore()[w.key]
  return {
    ...w,
    ...(bounds ? { bounds: { ...bounds } } : {}),
    ...(collapse ? { collapse: { ...collapse } } : {}),
  }
}

function isDashboardPanel(v: unknown): v is DashboardPanel {
  if (!v || typeof v !== 'object') return false
  const o = v as Record<string, unknown>
  return ['x', 'y', 'width', 'height'].every((k) => typeof o[k] === 'number' && Number.isFinite(o[k]))
    && typeof o.visible === 'boolean'
}

function cleanPanelMap(v: unknown): Record<string, DashboardPanel> {
  const out: Record<string, DashboardPanel> = {}
  if (!v || typeof v !== 'object') return out
  for (const [k, p] of Object.entries(v as Record<string, unknown>)) {
    if (k.length <= 64 && isDashboardPanel(p)) {
      out[k] = { x: p.x, y: p.y, width: p.width, height: p.height, visible: p.visible }
    }
  }
  return out
}

function cleanDashboard(v: unknown): LayoutDashboard | undefined {
  if (!v || typeof v !== 'object') return undefined
  const o = v as Record<string, unknown>
  return { panels: cleanPanelMap(o.panels), groups: cleanPanelMap(o.groups) }
}

function copyDashboard(d: LayoutDashboard | undefined): LayoutDashboard | undefined {
  return d ? { panels: cleanPanelMap(d.panels), groups: cleanPanelMap(d.groups) } : undefined
}

function layoutWindowFromRequest(entry: PopoutRequestEntry): LayoutWindow | null {
  if (typeof entry === 'string') {
    const name = PANEL_TO_OVERLAY[entry] ?? entry
    // isValidWindowKey already excludes the trigger overlay.
    return isValidWindowKey(name) ? { key: name } : null
  }
  return entry?.key?.startsWith('customTimer:') ? { key: entry.key, groupName: entry.name ?? '' } : null
}

// First run with layouts: seed "Default" from the restore-on-launch set when
// that was on (it's what's on screen). Otherwise the renderer migrates its
// saved Pop Out All set (layouts:migrate-selection).
function initialLayoutStore(): LayoutStore {
  const autoOpen = loadAutoOpenStore()
  const fromAutoOpen = autoOpen.enabled && autoOpen.overlays.length > 0
  const windows: LayoutWindow[] = []
  if (fromAutoOpen) {
    for (const entry of autoOpen.overlays) {
      const w = typeof entry === 'string'
        ? layoutWindowFromRequest(entry)
        : layoutWindowFromRequest({ key: entry.key, name: entry.groupName })
      if (w) windows.push(withLiveState(w))
    }
  }
  return {
    activeId: 'default',
    layouts: [{ id: 'default', name: 'Default', windows }],
    selectionMigrated: fromAutoOpen,
    dashboardMigrated: false,
  }
}

function parseLayoutStore(raw: unknown): LayoutStore | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Partial<LayoutStore>
  if (!Array.isArray(o.layouts)) return null
  const layouts: OverlayLayout[] = []
  for (const l of o.layouts) {
    if (!l || typeof l.id !== 'string' || !l.id || layouts.some((x) => x.id === l.id)) continue
    const windows = (Array.isArray(l.windows) ? l.windows : []).filter(
      (w): w is LayoutWindow => !!w && typeof w.key === 'string' && isValidWindowKey(w.key) && w.key !== 'trigger',
    )
    const dashboard = cleanDashboard(l.dashboard)
    layouts.push({ id: l.id, name: cleanLayoutName(l.name), windows, ...(dashboard ? { dashboard } : {}) })
  }
  if (layouts.length === 0) return null
  const activeId = layouts.some((l) => l.id === o.activeId) ? (o.activeId as string) : layouts[0].id
  return {
    activeId,
    layouts,
    selectionMigrated: o.selectionMigrated === true,
    dashboardMigrated: o.dashboardMigrated === true,
  }
}

let layoutStoreCache: LayoutStore | null = null

function loadLayoutStore(): LayoutStore {
  if (layoutStoreCache) return layoutStoreCache
  let parsed: LayoutStore | null = null
  try {
    parsed = parseLayoutStore(JSON.parse(readFileSync(layoutsFilePath(), 'utf-8')))
  } catch {
    parsed = null
  }
  layoutStoreCache = parsed ?? initialLayoutStore()
  if (!parsed) saveLayoutStore()
  return layoutStoreCache
}

function saveLayoutStore(): void {
  try {
    writeFileSync(layoutsFilePath(), JSON.stringify(loadLayoutStore(), null, 2), 'utf-8')
  } catch (err) {
    console.warn('[main] failed to save overlay layouts', err)
  }
}

function activeLayout(): OverlayLayout {
  const store = loadLayoutStore()
  return store.layouts.find((l) => l.id === store.activeId) ?? store.layouts[0]
}

function layoutSummary(): LayoutSummary {
  const store = loadLayoutStore()
  return {
    activeId: store.activeId,
    layouts: store.layouts.map((l) => ({ id: l.id, name: l.name, windows: l.windows.map((w) => w.key) })),
  }
}

function broadcastLayouts(): void {
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('layouts:changed', layoutSummary())
}

// Windows closing as part of a bulk close, so their 'closed' event doesn't
// drop them from the active layout. layoutsFrozen does the same while quitting.
const bulkClosingWindows = new WeakSet<BrowserWindow>()
let layoutsFrozen = false

function closePopoutsInBulk(wins: BrowserWindow[]): void {
  for (const win of wins) {
    bulkClosingWindows.add(win)
    win.close()
  }
}

function groupNameFor(key: WindowKey): string | undefined {
  return key.startsWith('customTimer:') ? customTimerGroupNames.get(key.slice('customTimer:'.length)) : undefined
}

// Called for every popout as it's created (from trackOverlayBounds).
function layoutWindowOpened(key: WindowKey, win: BrowserWindow): void {
  win.on('closed', () => {
    if (!layoutsFrozen && !bulkClosingWindows.has(win)) layoutWindowClosed(key)
  })
  if (layoutsFrozen) return
  const layout = activeLayout()
  const groupName = groupNameFor(key)
  const existing = layout.windows.find((w) => w.key === key)
  if (existing) {
    if (groupName === undefined || existing.groupName === groupName) return
    existing.groupName = groupName
  } else {
    layout.windows.push(withLiveState({ key, ...(groupName !== undefined ? { groupName } : {}) }))
  }
  saveLayoutStore()
  broadcastLayouts()
}

function layoutWindowClosed(key: WindowKey): void {
  const layout = activeLayout()
  const before = layout.windows.length
  layout.windows = layout.windows.filter((w) => w.key !== key)
  if (layout.windows.length === before) return
  saveLayoutStore()
  broadcastLayouts()
}

function layoutBoundsChanged(key: WindowKey, bounds: Bounds): void {
  const entry = activeLayout().windows.find((w) => w.key === key)
  if (!entry) return
  entry.bounds = { ...bounds }
  saveLayoutStore()
}

function layoutCollapseChanged(key: WindowKey): void {
  const entry = activeLayout().windows.find((w) => w.key === key)
  const collapse = loadCollapseStore()[key]
  if (!entry || !collapse) return
  entry.collapse = { ...collapse }
  saveLayoutStore()
}

// Moves an already-open window to its spot in a layout and applies the
// layout's collapse state (already written to the collapse store).
function placeWindowForLayout(win: BrowserWindow, w: LayoutWindow): void {
  const min = minSizeFor(w.key)
  win.setMaximumSize(0, 0)
  win.setMinimumSize(min.minWidth, min.minHeight)
  if (w.bounds && isOnScreen(w.bounds)) win.setBounds(w.bounds)
  const c = collapsedEntry(w.key)
  if (c) applyCollapsedSize(win, w.key, c.headerHeight)
  win.webContents.send('overlay:collapse-changed', !!c)
  showOverlayWindow(win)
}

// Opens every window in a layout (or brings it back if hidden), at its saved
// spot. Also makes sure the trigger alert overlay exists.
function openLayoutWindows(layout: OverlayLayout, place = false): void {
  for (const w of [...layout.windows]) {
    const win = overlayWindowByName(w.key)
    if (win && !win.isDestroyed()) {
      if (place) placeWindowForLayout(win, w)
      else showOverlayWindow(win)
    } else if (w.key.startsWith('customTimer:')) {
      createCustomTimerGroupOverlay(w.key.slice('customTimer:'.length), w.groupName ?? '')
    } else {
      createOverlayByName(w.key as OverlayName)
    }
  }
  if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) createTriggerOverlay()
}

// flush=false when the outgoing layout no longer exists (it was deleted):
// pending moves are dropped instead of saved into whichever layout is next.
function switchLayout(id: string, flush = true): void {
  const store = loadLayoutStore()
  const target = store.layouts.find((l) => l.id === id)
  if (!target) return
  // Save pending moves into the outgoing layout before it stops being active.
  for (const win of userPopoutWindows()) {
    const key = windowToOverlayName.get(win)
    if (!key) continue
    if (flush) {
      writeBoundsNow(key, win)
    } else {
      const t = boundsDebounceTimers.get(key)
      if (t) clearTimeout(t)
      boundsDebounceTimers.delete(key)
    }
  }
  store.activeId = id
  const wanted = new Set(target.windows.map((w) => w.key))
  closePopoutsInBulk(userPopoutWindows().filter((win) => {
    const key = windowToOverlayName.get(win)
    return !key || !wanted.has(key)
  }))
  // Windows open from the per-window stores, so seed them with the target's
  // positions and collapse states. No collapse entry means expanded.
  const bounds = loadBoundsStore()
  const collapse = loadCollapseStore()
  for (const w of target.windows) {
    if (w.bounds) bounds[w.key] = { ...w.bounds }
    const cur = collapse[w.key]
    if (w.collapse) collapse[w.key] = { ...w.collapse }
    else if (cur?.collapsed) collapse[w.key] = { ...cur, collapsed: false }
  }
  saveBoundsStore(bounds)
  saveCollapseStore()
  saveLayoutStore()
  openLayoutWindows(target, true)
  broadcastLayouts()
}

// A layout hotkey switches to that layout, or shows/hides everything when it's
// already the active one.
function runLayoutHotkey(id: string): void {
  if (loadLayoutStore().activeId === id) toggleAllPopoutsVisibility()
  else switchLayout(id)
}

ipcMain.handle('layouts:get', () => layoutSummary())

ipcMain.handle('layouts:switch', (_event, id: string) => {
  switchLayout(id)
  return layoutSummary()
})

// Copies the active layout (its window set, with current positions) under a
// new name and makes the copy active. Nothing on screen changes.
ipcMain.handle('layouts:create', (_event, name: string) => {
  const store = loadLayoutStore()
  for (const win of userPopoutWindows()) {
    const key = windowToOverlayName.get(win)
    if (key) writeBoundsNow(key, win)
  }
  const copy: OverlayLayout = {
    id: newLayoutId(),
    name: cleanLayoutName(name),
    windows: activeLayout().windows.map((w) => withLiveState({ key: w.key, ...(w.groupName !== undefined ? { groupName: w.groupName } : {}) })),
  }
  const dashboard = copyDashboard(activeLayout().dashboard)
  if (dashboard) copy.dashboard = dashboard
  store.layouts.push(copy)
  store.activeId = copy.id
  saveLayoutStore()
  broadcastLayouts()
  return layoutSummary()
})

ipcMain.handle('layouts:rename', (_event, id: string, name: string) => {
  const layout = loadLayoutStore().layouts.find((l) => l.id === id)
  if (layout) {
    layout.name = cleanLayoutName(name)
    saveLayoutStore()
    broadcastLayouts()
  }
  return layoutSummary()
})

// The last layout can't be deleted. Deleting the active one switches to the
// first remaining layout; its hotkey binding goes with it.
ipcMain.handle('layouts:delete', (_event, id: string) => {
  const store = loadLayoutStore()
  if (store.layouts.length <= 1 || !store.layouts.some((l) => l.id === id)) return layoutSummary()
  const wasActive = store.activeId === id
  store.layouts = store.layouts.filter((l) => l.id !== id)
  if (hotkeyStore.bindings[`layout:${id}`]) {
    delete hotkeyStore.bindings[`layout:${id}`]
    saveHotkeyStore(hotkeyStore)
    applyHotkeys()
  }
  if (wasActive) {
    switchLayout(store.layouts[0].id, false)
  } else {
    saveLayoutStore()
    broadcastLayouts()
  }
  return layoutSummary()
})

ipcMain.handle('layouts:open-active', () => openLayoutWindows(activeLayout()))

// One-time migration of the renderer's old saved Pop Out All set into the
// Default layout, sent on startup with group names resolved.
ipcMain.handle('layouts:migrate-selection', (_event, entries: PopoutRequestEntry[]) => {
  const store = loadLayoutStore()
  if (store.selectionMigrated) return layoutSummary()
  store.selectionMigrated = true
  const layout = activeLayout()
  if (layout.windows.length === 0 && Array.isArray(entries)) {
    for (const entry of entries) {
      const w = layoutWindowFromRequest(entry)
      if (w && !layout.windows.some((x) => x.key === w.key)) layout.windows.push(withLiveState(w))
    }
  }
  saveLayoutStore()
  broadcastLayouts()
  return layoutSummary()
})

// The active layout's dashboard arrangement; null means the defaults.
// migrated=false asks the renderer to send its old localStorage arrangement.
function dashboardState(): { id: string; dashboard: LayoutDashboard | null; migrated: boolean } {
  const store = loadLayoutStore()
  const layout = activeLayout()
  return { id: layout.id, dashboard: layout.dashboard ?? null, migrated: store.dashboardMigrated }
}

ipcMain.handle('layouts:dashboard-get', () => dashboardState())

// Saved by layout id, so a save in flight across a switch lands in the layout
// it was made in.
ipcMain.handle('layouts:dashboard-set', (_event, id: string, dashboard: unknown) => {
  const layout = loadLayoutStore().layouts.find((l) => l.id === id)
  const clean = cleanDashboard(dashboard)
  if (!layout || !clean) return
  layout.dashboard = clean
  saveLayoutStore()
})

// One-time: the renderer's old localStorage arrangement becomes every
// existing layout's (they were all made before layouts had their own).
ipcMain.handle('layouts:migrate-dashboard', (_event, dashboard: unknown) => {
  const store = loadLayoutStore()
  if (!store.dashboardMigrated) {
    store.dashboardMigrated = true
    const clean = cleanDashboard(dashboard)
    if (clean) {
      for (const l of store.layouts) if (!l.dashboard) l.dashboard = copyDashboard(clean)
    }
    saveLayoutStore()
  }
  return dashboardState()
})

ipcMain.handle('overlay:popouts:close-all', () => {
  // Use close() (not destroy()) so each window's 'close' handler persists its
  // bounds. The always-on trigger overlay is intentionally left open so trigger
  // alerts keep rendering after a bulk close. A bulk close leaves the active
  // layout's window set alone, so toggling back on reopens the same windows.
  closePopoutsInBulk(userPopoutWindows())
})

// Per-overlay popout open state, keyed by canonical overlay name (excludes the
// always-on trigger overlay). Lets the dashboard show which windows are open.
ipcMain.handle('overlay:popouts:states', () => {
  const states: Record<string, boolean> = {}
  for (const name of RESETTABLE_OVERLAYS) {
    const win = overlayWindowByName(name)
    // A window hidden by "hide all" reports closed, so its Pop toggle (which
    // brings it back) reads as "pop out" rather than "close".
    states[name] = !!win && !win.isDestroyed() && !userHiddenOverlays.has(win)
  }
  // Only currently-open named groups can be reported — Electron doesn't know
  // the full group list (that's backend/renderer state), just which windows
  // it has actually created this session.
  for (const [groupId, win] of customTimerGroupWindows) {
    states[customTimerWindowKey(groupId)] = !win.isDestroyed() && !userHiddenOverlays.has(win)
  }
  return states
})

// Open popouts by window key, as 'open' or 'hidden' (hidden by hide-all or a
// hotkey). Closed windows are absent. The Overlays hub shows all three states.
ipcMain.handle('overlay:popouts:window-states', () => {
  const states: Record<string, 'open' | 'hidden'> = {}
  for (const win of userPopoutWindows()) {
    const key = windowToOverlayName.get(win)
    if (key) states[key] = userHiddenOverlays.has(win) && !win.isVisible() ? 'hidden' : 'open'
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
  if (store.enabled) store.overlays = currentlyOpenAutoOpenEntries()
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

// True for any fixed OverlayName in RESETTABLE_OVERLAYS, or a named Custom
// Timers group's WindowKey (`customTimer:<groupId>`). Gates every IPC handler
// below (reset-position, move-to-display, placing mode) so a bogus/unrelated
// name string can't reach overlayWindowByName.
function isValidWindowKey(name: string): name is WindowKey {
  return (RESETTABLE_OVERLAYS as string[]).includes(name) || name.startsWith('customTimer:')
}

// Default bounds for a fresh/reset window. Named groups all share the plain
// 'customTimer' default size — there's no per-group default to look up.
function defaultBoundsFor(name: WindowKey): Bounds {
  if (name.startsWith('customTimer:')) return OVERLAY_DEFAULTS.customTimer
  return OVERLAY_DEFAULTS[name as Exclude<OverlayName, 'trigger'>]
}

function overlayWindowByName(name: WindowKey): BrowserWindow | null {
  if (name.startsWith('customTimer:')) {
    return customTimerGroupWindows.get(name.slice('customTimer:'.length)) ?? null
  }
  if (isStandardOverlay(name)) return standardWindows.get(name) ?? null
  switch (name) {
    case 'trigger': return triggerOverlayWindow
    case 'discordVoice': return discordVoiceOverlayWindow
    default: return null
  }
}

// Open one overlay's popout window by canonical name (the trigger overlay is
// created separately at startup and so is intentionally absent here).
function createOverlayByName(name: OverlayName): void {
  if (isStandardOverlay(name)) {
    createStandardOverlay(name)
    return
  }
  switch (name) {
    case 'discordVoice': createDiscordVoiceOverlay(); break
    default: break
  }
}

// Every popout overlay currently open (excludes the always-on trigger
// overlay, which isn't user-toggled) — fixed overlay types plus any open
// named Custom Timers group windows. Used to snapshot what to restore on
// the next launch.
function currentlyOpenAutoOpenEntries(): AutoOpenEntry[] {
  const fixed: AutoOpenEntry[] = RESETTABLE_OVERLAYS.filter((name) => {
    const win = overlayWindowByName(name)
    return !!win && !win.isDestroyed()
  })
  const groups: AutoOpenEntry[] = []
  for (const [groupId, win] of customTimerGroupWindows) {
    if (win && !win.isDestroyed()) {
      groups.push({ key: customTimerWindowKey(groupId), groupName: customTimerGroupNames.get(groupId) ?? '' })
    }
  }
  return [...fixed, ...groups]
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
  store.overlays = currentlyOpenAutoOpenEntries()
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
function resetOverlayPosition(name: WindowKey): void {
  const target = centeredOnPrimary(defaultBoundsFor(name))

  // Persist centered bounds and clear the lock regardless of open/closed state.
  const store = loadBoundsStore()
  store[name] = target
  saveBoundsStore(store)
  setOverlayLocked(name, false)

  const win = overlayWindowByName(name)
  clearOverlayCollapsed(name, win)
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
  if (isValidWindowKey(name)) resetOverlayPosition(name)
})

ipcMain.handle('overlay:reset-all-positions', () => {
  for (const name of RESETTABLE_OVERLAYS) resetOverlayPosition(name)
  // Only currently-open named groups — Electron has no record of a group
  // that's never been opened this session (that list lives in the Go
  // backend). A closed group's stale position can still be reset
  // individually once it's known to the frontend (Timer Groups screen).
  for (const groupId of customTimerGroupWindows.keys()) {
    resetOverlayPosition(customTimerWindowKey(groupId))
  }
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
  // Named Custom Timers windows: report for every key ever positioned (open
  // or closed), same as fixed overlays above, so a closed group's monitor
  // picker still shows where it'll reopen.
  for (const key of Object.keys(store)) {
    if (!key.startsWith('customTimer:')) continue
    const windowKey = key as WindowKey
    const win = overlayWindowByName(windowKey)
    const b = win && !win.isDestroyed() ? win.getBounds() : store[windowKey] ?? OVERLAY_DEFAULTS.customTimer
    out[key] = screen.getDisplayMatching(b).id
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

function setPlacing(name: WindowKey, on: boolean): void {
  if (on) placingOverlays.add(name)
  else placingOverlays.delete(name)
  const win = overlayWindowByName(name)
  if (win && !win.isDestroyed()) {
    if (on) {
      // Re-home an off-screen overlay onto the primary monitor first so its
      // placing banner is actually visible to grab.
      if (!isOnScreen(win.getBounds())) win.setBounds(centeredOnPrimary(win.getBounds()))
      userHiddenOverlays.delete(win)
      win.show()
    }
    win.webContents.send('overlay:placing-changed', on)
  }
}

ipcMain.handle('overlay:place:start', (_event, name: string) => {
  if (isValidWindowKey(name)) setPlacing(name, true)
})
ipcMain.handle('overlay:place:stop', (_event, name: string) => {
  if (isValidWindowKey(name)) setPlacing(name, false)
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
  if (!isValidWindowKey(name)) return
  const target = screen.getAllDisplays().find((d) => d.id === displayId)
  if (!target) return
  const win = overlayWindowByName(name)
  const size = win && !win.isDestroyed() ? win.getBounds() : loadBoundsStore()[name] ?? defaultBoundsFor(name)
  const bounds = centeredOnDisplay(target, { width: size.width, height: size.height })
  const store = loadBoundsStore()
  store[name] = bounds
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
  for (const win of customTimerGroupWindows.values()) targets.push(win)
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

// ── IPC handlers — raid selection relay ──────────────────────────────────────

// The Raid Composition check page is the only surface with an encounter
// picker; both Raid Readiness views (dashboard panel + popout overlay)
// render the same useRaidReadiness hook in their own window and must mirror
// whatever it selects. Relay manual picks to every window that renders the
// hook, and remember the last pick so a popout opened later can adopt it.
let lastRaidSelection = ''

function broadcastRaidSelection(id: string): void {
  lastRaidSelection = id
  const targets: Array<BrowserWindow | null> = [mainWindow, stdWin('raidReadiness')]
  for (const win of customTimerGroupWindows.values()) targets.push(win)
  for (const win of targets) {
    if (win && !win.isDestroyed()) {
      win.webContents.send('overlay:raid-selection-changed', id)
    }
  }
}

ipcMain.handle('overlay:raid-selection:set', (_event, id: string) => {
  if (typeof id === 'string') broadcastRaidSelection(id)
})

ipcMain.handle('overlay:raid-selection:get', () => lastRaidSelection)

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

ipcMain.handle('dialog:save-debug-logs', async () => {
  const result = await dialog.showSaveDialog({
    title: 'Export Debug Logs',
    defaultPath: `pq-companion-debug-logs-${new Date().toISOString().slice(0, 10)}.zip`,
    filters: [
      { name: 'Zip Archive', extensions: ['zip'] },
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

ipcMain.handle('dialog:select-piper-exe', async () => {
  // The piper program: a frozen standalone piper.exe or a pip console script.
  // No extension filter on non-Windows; on Windows default to .exe but allow
  // All Files so a batch/Python wrapper can be picked.
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Select Piper Executable',
    filters:
      process.platform === 'win32'
        ? [
            { name: 'Executable', extensions: ['exe', 'bat', 'cmd'] },
            { name: 'All Files', extensions: ['*'] },
          ]
        : [{ name: 'All Files', extensions: ['*'] }],
  })
  return result.canceled ? null : result.filePaths[0]
})

ipcMain.handle('dialog:select-piper-model', async () => {
  const result = await dialog.showOpenDialog({
    properties: ['openFile'],
    title: 'Select Piper Voice Model (.onnx)',
    filters: [
      { name: 'Piper Voice Model', extensions: ['onnx'] },
      { name: 'All Files', extensions: ['*'] },
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

// ── IPC handlers — hardware acceleration toggle ───────────────────────────────
// See the pre-ready gate near the top of this file and LIMITATIONS.md §13.2.
// Only takes effect after a restart — disableHardwareAcceleration() must run
// before 'ready', so writing the flag here just arms it for next launch.

ipcMain.handle('settings:hardware-acceleration:get', () => ({
  disabled: loadHardwareAccelDisabled(),
}))

ipcMain.handle('settings:hardware-acceleration:set', (_event, disabled: boolean) => {
  saveHardwareAccelDisabled(disabled)
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
  if (!isDev && canSelfUpdate) autoUpdater.downloadUpdate()
})

ipcMain.handle('updater:quit-and-install', async () => {
  // Kill the sidecar before handing off to the updater. quitAndInstall exits
  // the main process quickly; if the sidecar is still alive the NSIS updater
  // cannot replace pq-companion-server.exe and the install wedges.
  if (!canSelfUpdate) return
  await stopSidecar()
  autoUpdater.quitAndInstall(true, true)
})

// checkInstallLocation runs before anything starts. On a copy the installer
// doesn't manage it disables self-update, and if an installed copy exists
// elsewhere it warns and offers to switch. Returns true when this instance is
// handing off to the installed copy and must start nothing.
async function checkInstallLocation(): Promise<boolean> {
  if (process.platform !== 'win32' || isDev || isManagedCopy()) return false
  canSelfUpdate = false
  const managed = await findManagedInstall()
  console.log(`[main] unmanaged copy (self-update disabled); installed copy: ${managed ? `${managed.exePath} v${managed.version || '?'}` : 'none'}`)
  if (!managed || process.env[SKIP_ENV]) return false
  const userData = app.getPath('userData')
  if (isWarningSilenced(userData)) return false

  const { message, detail } = unmanagedCopyMessage(managed, app.getVersion())
  const { response, checkboxChecked } = await dialog.showMessageBox({
    type: 'warning',
    title: 'PQ Companion',
    message,
    detail,
    buttons: ['Open Installed Copy', 'Keep Using This Copy', 'Show This Folder in Explorer'],
    defaultId: 0,
    cancelId: 1,
    checkboxLabel: "Don't warn me about this folder again",
    noLink: true,
  })
  if (checkboxChecked) silenceWarning(userData)
  if (response === 0) {
    // Nothing has started yet, so exiting skips no cleanup; the relaunch
    // fires once this process (and its single-instance lock) is gone.
    app.relaunch({ execPath: managed.exePath, args: [] })
    app.exit(0)
    return true
  }
  if (response === 2) shell.showItemInFolder(process.execPath)
  return false
}

// ── App lifecycle ─────────────────────────────────────────────────────────────

app.whenReady().then(async () => {
  // Lost the single-instance lock: this instance only exists to hand off to
  // (or warn about) the running copy and then exit. Start nothing.
  if (!gotSingleInstanceLock) return
  if (await checkInstallLocation()) return
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

  // Apply any pending App Backup/Restore import's Electron-side state before
  // anything reads overlayBounds.json et al. — createMainWindow,
  // createTriggerOverlay, and the restore-overlays-on-launch block just below
  // all read those files, and this process itself rewrites them on quit, so
  // there's no later point this could safely run. A pending factory reset
  // (mutually exclusive with a pending import — see the Settings UI) clears
  // the same files instead of restoring them.
  applyPendingClientState()
  await applyPendingFactoryResetClientState()

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
  // A named group entry carries its own display name (see AutoOpenGroupEntry)
  // so this needs no backend round-trip — the backend isn't reachable yet.
  //
  // The active layout is the set that was open at quit (it tracks every open
  // and close), so restoring means opening it.
  if (loadAutoOpenStore().enabled) openLayoutWindows(activeLayout())

  setupAutoUpdater()

  // Keep the trigger overlay pinned to its chosen monitor when the layout
  // changes (plug/unplug a display, resolution or DPI change). If the chosen
  // monitor was unplugged, resolveOverlayDisplay() re-homes the overlay to a
  // still-connected one so it never lands on a now-missing display.
  const resizeTriggerOverlay = (): void => {
    if (!triggerOverlayWindow || triggerOverlayWindow.isDestroyed()) return
    triggerOverlayWindow.setBounds(overlayDisplayBounds())
  }
  // A display change (resolution/DPI change, a monitor waking) and a resume
  // or unlock are plausible points where Windows resets the topmost z-order
  // band, leaving overlays open but buried behind the game. The raise
  // interval only re-stacks; these also re-apply the topmost flag itself.
  startOverlayRaiseInterval()
  initHotkeys()
  const onDisplayChanged = (): void => {
    console.log('[main] display layout changed; reasserting overlays')
    resizeTriggerOverlay()
    reassertOverlaysOnTop()
  }
  screen.on('display-added', onDisplayChanged)
  screen.on('display-removed', onDisplayChanged)
  screen.on('display-metrics-changed', onDisplayChanged)

  // Log power/session transitions so an "overlays vanished" report can be
  // lined up against a sleep/wake or lock/unlock in the exported logs.
  powerMonitor.on('suspend', () => console.log('[main] power: suspend'))
  powerMonitor.on('resume', () => {
    console.log('[main] power: resume')
    reassertOverlaysOnTop()
  })
  powerMonitor.on('lock-screen', () => console.log('[main] power: lock-screen'))
  powerMonitor.on('unlock-screen', () => {
    console.log('[main] power: unlock-screen')
    reassertOverlaysOnTop()
  })

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
  if (!gotSingleInstanceLock) return
  snapshotAutoOpenOverlays()
  layoutsFrozen = true
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

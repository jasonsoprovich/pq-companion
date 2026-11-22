// overlayHost runs overlay windows in one shared renderer process.
//
// Every BrowserWindow normally gets its own Chromium renderer process (~75 MB
// on Windows, mostly fixed overhead), so a dozen overlays cost ~1 GB. Chromium
// does keep a window opened with window.open() in its opener's process when
// both are same-site and share a sandbox setting, so the host is a hidden,
// empty window that opens every shared overlay with window.open(). Each
// overlay is still a real BrowserWindow with its own route, React tree,
// preload bridge, bounds and monitor; only the process is shared.
//
// Because creation goes through the host's renderer, it's asynchronous: an
// open() call returns before the window exists. Requests are tracked by a
// unique id (also the window.open frame name), so a cancelled or timed-out
// request whose window arrives late is destroyed instead of being mistaken for
// a newer request for the same overlay. Callers dedupe through isPending().
//
// Electron 27+ closes window.open() children when their opener closes, so
// destroying the host also closes every shared overlay.
import { BrowserWindow, type BrowserWindowConstructorOptions } from 'electron'

const OPEN_TIMEOUT_MS = 5000
// With no windows left the host is closed after this long, so its process
// (and the memory Chromium kept from closed overlays) goes away.
const IDLE_CLOSE_MS = 30_000

type Pending = {
  id: string
  key: string
  url: string
  options: BrowserWindowConstructorOptions
  onCreated: (win: BrowserWindow, startHidden: boolean) => void
  // Hide/Show All while the window is still on its way.
  visible: boolean
  timer: ReturnType<typeof setTimeout>
}

export type OverlayHostDeps = {
  // Full URL of the host page and of an overlay route (same origin).
  hostUrl: string
  routeUrl: (route: string) => string
  // The host's renderer died (crash, OOM, killed). Every shared overlay died
  // with it; the caller tears down and reopens.
  onCrash: (reason: string) => void
  log: (msg: string) => void
}

export class OverlayHost {
  private deps: OverlayHostDeps
  private host: BrowserWindow | null = null
  private ready = false
  private queue: Pending[] = []
  private pending = new Map<string, Pending>()
  private pendingByKey = new Map<string, string>()
  private seq = 0
  private children = new Set<BrowserWindow>()
  private idleTimer: ReturnType<typeof setTimeout> | null = null
  private loggedPidMismatch = false

  constructor(deps: OverlayHostDeps) {
    this.deps = deps
  }

  window(): BrowserWindow | null {
    return this.host && !this.host.isDestroyed() ? this.host : null
  }

  isHost(win: BrowserWindow): boolean {
    return win === this.host
  }

  isPending(key: string): boolean {
    return this.pendingByKey.has(key)
  }

  pendingKeys(): string[] {
    return [...this.pendingByKey.keys()]
  }

  // open asks the host to create an overlay window. onCreated runs once the
  // BrowserWindow exists (before its page has painted), unless the request
  // was cancelled first. A second open() for a key already in flight just
  // marks it visible again.
  open(key: string, route: string, options: BrowserWindowConstructorOptions, onCreated: (win: BrowserWindow, startHidden: boolean) => void): void {
    const inFlight = this.pendingByKey.get(key)
    if (inFlight) {
      const p = this.pending.get(inFlight)
      if (p) p.visible = true
      return
    }
    this.clearIdle()
    const id = `pqc-overlay-${++this.seq}`
    const p: Pending = {
      id, key, url: this.deps.routeUrl(route), options, onCreated, visible: true,
      timer: setTimeout(() => this.fail(id, 'timed out'), OPEN_TIMEOUT_MS),
    }
    this.pending.set(id, p)
    this.pendingByKey.set(key, id)
    this.ensureHost()
    if (this.ready) this.request(p)
    else this.queue.push(p)
  }

  // cancel drops in-flight requests matching the predicate; their windows are
  // destroyed if they still arrive.
  cancelWhere(match: (key: string) => boolean): void {
    for (const p of [...this.pending.values()]) if (match(p.key)) this.drop(p.id)
    this.scheduleIdle()
  }

  cancel(key: string): void {
    const id = this.pendingByKey.get(key)
    if (id) this.drop(id)
  }

  setPendingVisible(visible: boolean): void {
    for (const p of this.pending.values()) p.visible = visible
  }

  // destroy closes the host (and with it every shared overlay) and drops all
  // in-flight requests.
  destroy(): void {
    this.children.clear()
    this.cancelWhere(() => true)
    this.clearIdle()
    const h = this.host
    this.host = null
    this.ready = false
    this.queue = []
    if (h && !h.isDestroyed()) h.destroy()
  }

  pid(): number | null {
    const h = this.window()
    return h ? h.webContents.getOSProcessId() : null
  }

  private ensureHost(): void {
    if (this.window()) return
    const h = new BrowserWindow({
      show: false,
      width: 1,
      height: 1,
      skipTaskbar: true,
      // Same webPreferences shape as the overlays (and the same default
      // sandbox), so the children qualify for the opener's process.
      webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false },
    })
    this.host = h
    this.ready = false
    // Overrides the global deny-all handler from hardenWebContents: only our
    // own pending requests may open, and only to the exact URL requested.
    h.webContents.setWindowOpenHandler(({ url, frameName }) => {
      const p = this.pending.get(frameName)
      if (!p || p.url !== url) {
        this.deps.log(`[overlay-host] denied window.open frame=${frameName}`)
        return { action: 'deny' }
      }
      return { action: 'allow', overrideBrowserWindowOptions: p.options }
    })
    h.webContents.on('did-create-window', (win, details) => this.arrived(win, details.frameName))
    h.webContents.on('did-finish-load', () => {
      if (h !== this.host) return
      this.ready = true
      const q = this.queue
      this.queue = []
      for (const p of q) if (this.pending.has(p.id)) this.request(p)
    })
    h.webContents.on('did-fail-load', (_e, code, desc) => {
      if (h !== this.host) return
      this.deps.log(`[overlay-host] host page failed to load: ${code} ${desc}`)
      for (const p of [...this.pending.values()]) this.fail(p.id, 'host failed to load')
    })
    h.webContents.on('render-process-gone', (_e, details) => {
      if (h !== this.host) return
      this.deps.onCrash(details.reason)
    })
    h.on('closed', () => {
      if (h === this.host) {
        this.host = null
        this.ready = false
        for (const p of [...this.pending.values()]) this.fail(p.id, 'host closed')
      }
    })
    void h.loadURL(this.deps.hostUrl)
  }

  private request(p: Pending): void {
    const h = this.window()
    if (!h) return this.fail(p.id, 'no host')
    const js = `!!window.open(${JSON.stringify(p.url)}, ${JSON.stringify(p.id)})`
    h.webContents.executeJavaScript(js, true)
      .then((ok) => { if (!ok) this.fail(p.id, 'window.open returned null') })
      .catch((err) => this.fail(p.id, `window.open failed: ${String(err)}`))
  }

  private arrived(win: BrowserWindow, frameName: string): void {
    const p = this.pending.get(frameName)
    if (!p) {
      // Cancelled, timed out, or not ours: never let a stray window live.
      this.deps.log(`[overlay-host] dropping late/unknown window frame=${frameName}`)
      if (!win.isDestroyed()) win.destroy()
      return
    }
    this.drop(p.id)
    const hostPid = this.pid()
    const childPid = win.webContents.getOSProcessId()
    if (hostPid && childPid && hostPid !== childPid && !this.loggedPidMismatch) {
      this.loggedPidMismatch = true
      this.deps.log(`[overlay-host] ${p.key} did not share the host process (host=${hostPid} child=${childPid})`)
    }
    win.webContents.once('did-fail-load', (_e, code, desc) => {
      this.deps.log(`[overlay-host] ${p.key} failed to load: ${code} ${desc}`)
    })
    this.children.add(win)
    win.once('closed', () => {
      this.children.delete(win)
      this.scheduleIdle()
    })
    p.onCreated(win, !p.visible)
  }

  private clearIdle(): void {
    if (this.idleTimer) clearTimeout(this.idleTimer)
    this.idleTimer = null
  }

  private scheduleIdle(): void {
    this.clearIdle()
    if (this.children.size || this.pending.size || !this.window()) return
    this.idleTimer = setTimeout(() => {
      this.idleTimer = null
      if (this.children.size || this.pending.size) return
      this.deps.log('[overlay-host] no shared overlays open; closing the host')
      this.destroy()
    }, IDLE_CLOSE_MS)
  }

  private fail(id: string, why: string): void {
    const p = this.pending.get(id)
    if (!p) return
    this.deps.log(`[overlay-host] open ${p.key} ${why}`)
    this.drop(id)
    this.scheduleIdle()
  }

  // drop removes a request; only clears the key index if it still points at
  // this request, so a replacement for the same key survives.
  private drop(id: string): void {
    const p = this.pending.get(id)
    if (!p) return
    clearTimeout(p.timer)
    this.pending.delete(id)
    if (this.pendingByKey.get(p.key) === id) this.pendingByKey.delete(p.key)
  }
}

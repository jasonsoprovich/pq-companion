import { contextBridge, ipcRenderer, webFrame } from 'electron'

type HotkeyState = { bindings: Record<string, string>; status: Record<string, 'ok' | 'failed'> }
type LayoutState = {
  activeId: string
  layouts: Array<{ id: string; name: string; windows: string[]; characters: string[] }>
}
type DashboardPanel = { x: number; y: number; width: number; height: number; visible: boolean }
type LayoutDashboard = { panels: Record<string, DashboardPanel>; groups: Record<string, DashboardPanel> }
type LayoutDashboardState = { id: string; dashboard: LayoutDashboard | null; migrated: boolean }
type StackInfo = { id: string; key: string; tabs: string[]; active: string; groupNames: Record<string, string> }
type StackDropHint = { role: 'source' | 'target'; key: string; name?: string } | null
type ProcessRow = {
  pid: number
  role: 'overlay' | 'main' | 'trigger' | 'streamkit' | 'helper'
  keys: string[]
  state?: 'open' | 'hidden'
  type: string
  memoryMB: number
  privateMB: number | null
  cpu: number
}

// Expose a safe, typed API to the renderer — no direct Node/Electron access
contextBridge.exposeInMainWorld('electron', {
  app: {
    getVersion: (): Promise<string> => ipcRenderer.invoke('app:version'),
    relaunch: (): Promise<void> => ipcRenderer.invoke('app:relaunch'),
    // Ask the main process to focus the main window and navigate it to a hash
    // route. Called from overlay windows so their entity links open in the
    // main database explorer.
    navigateMain: (route: string): Promise<void> =>
      ipcRenderer.invoke('app:navigate-main', route),
    // Main-window-only: subscribe to deep-link routes pushed from other windows.
    onNavigate: (cb: (route: string) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, route: string): void => cb(route)
      ipcRenderer.on('app:navigate', listener)
      return () => ipcRenderer.removeListener('app:navigate', listener)
    },
    // App Backup/Restore: Electron-side window/overlay state (bounds, lock,
    // auto-open set, display pin, main window state) the Go backend has no
    // visibility into. getClientState is read on export; takeClientState
    // pulls the localStorage half of a staged import — call once, since the
    // main process deletes the pending file the moment it's read.
    getClientState: (): Promise<unknown> => ipcRenderer.invoke('app:client-state:get'),
    takeClientState: (): Promise<Record<string, string>> =>
      ipcRenderer.invoke('app:client-state:take-pending'),
  },
  backend: {
    getPort: (): Promise<number> => ipcRenderer.invoke('backend:port'),
  },
  settings: {
    // Opt-in workaround for a known Windows Electron/Chromium bug where
    // hardware-accelerated transparent windows render solid black (see
    // LIMITATIONS.md §13.2). Takes effect on next launch only.
    getHardwareAcceleration: (): Promise<{ disabled: boolean }> =>
      ipcRenderer.invoke('settings:hardware-acceleration:get'),
    setHardwareAcceleration: (disabled: boolean): Promise<void> =>
      ipcRenderer.invoke('settings:hardware-acceleration:set', disabled),
  },
  versions: {
    node: process.versions.node,
    chrome: process.versions.chrome,
    electron: process.versions.electron
  },
  window: {
    minimize: (): Promise<void> => ipcRenderer.invoke('window:minimize'),
    maximize: (): Promise<void> => ipcRenderer.invoke('window:maximize'),
    close: (): Promise<void> => ipcRenderer.invoke('window:close'),
    isMaximized: (): Promise<boolean> => ipcRenderer.invoke('window:is-maximized'),
    isPrimary: (): Promise<boolean> => ipcRenderer.invoke('window:is-primary'),
    dragStart: (): Promise<void> => ipcRenderer.invoke('window:drag:start'),
    dragEnd: (): Promise<void> => ipcRenderer.invoke('window:drag:end'),
    setZoom: (factor: number): Promise<void> => ipcRenderer.invoke('window:set-zoom', factor),
    // This window's current Chromium (Electron) zoom factor. Popout overlays
    // read it to cancel out zoom that leaks in from same-origin windows (see
    // useOverlayZoom).
    getZoom: (): number => webFrame.getZoomFactor(),
    // True when the main window uses the OS/window-manager frame instead of
    // our custom TitleBar (Wine/Proton on Linux — see useNativeMainFrame).
    nativeFrame: process.argv.includes('--pqc-native-frame'),
    setMinimizeToTray: (enabled: boolean): Promise<void> =>
      ipcRenderer.invoke('window:set-minimize-to-tray', enabled)
  },
  overlay: {
    openDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:open'),
    closeDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:close'),
    toggleDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:toggle'),
    openHPS: (): Promise<void> => ipcRenderer.invoke('overlay:hps:open'),
    closeHPS: (): Promise<void> => ipcRenderer.invoke('overlay:hps:close'),
    toggleHPS: (): Promise<void> => ipcRenderer.invoke('overlay:hps:toggle'),
    openBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:open'),
    closeBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:close'),
    toggleBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:toggle'),
    openFilteredTimer: (): Promise<void> => ipcRenderer.invoke('overlay:filteredtimer:open'),
    closeFilteredTimer: (): Promise<void> => ipcRenderer.invoke('overlay:filteredtimer:close'),
    toggleFilteredTimer: (): Promise<void> => ipcRenderer.invoke('overlay:filteredtimer:toggle'),
    openKillCounter: (): Promise<void> => ipcRenderer.invoke('overlay:killcounter:open'),
    closeKillCounter: (): Promise<void> => ipcRenderer.invoke('overlay:killcounter:close'),
    toggleKillCounter: (): Promise<void> => ipcRenderer.invoke('overlay:killcounter:toggle'),
    openPet: (): Promise<void> => ipcRenderer.invoke('overlay:pet:open'),
    closePet: (): Promise<void> => ipcRenderer.invoke('overlay:pet:close'),
    togglePet: (): Promise<void> => ipcRenderer.invoke('overlay:pet:toggle'),
    openDeathRecap: (): Promise<void> => ipcRenderer.invoke('overlay:deathrecap:open'),
    closeDeathRecap: (): Promise<void> => ipcRenderer.invoke('overlay:deathrecap:close'),
    toggleDeathRecap: (): Promise<void> => ipcRenderer.invoke('overlay:deathrecap:toggle'),
    openItemInfo: (): Promise<void> => ipcRenderer.invoke('overlay:iteminfo:open'),
    showItemInfo: (): Promise<void> => ipcRenderer.invoke('overlay:iteminfo:show'),
    closeItemInfo: (): Promise<void> => ipcRenderer.invoke('overlay:iteminfo:close'),
    toggleItemInfo: (): Promise<void> => ipcRenderer.invoke('overlay:iteminfo:toggle'),
    openCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:open'),
    closeCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:close'),
    toggleCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:toggle'),
    openLiveMap: (): Promise<void> => ipcRenderer.invoke('overlay:livemap:open'),
    closeLiveMap: (): Promise<void> => ipcRenderer.invoke('overlay:livemap:close'),
    toggleLiveMap: (): Promise<void> => ipcRenderer.invoke('overlay:livemap:toggle'),
    openCHMetronome: (): Promise<void> => ipcRenderer.invoke('overlay:chmetronome:open'),
    closeCHMetronome: (): Promise<void> => ipcRenderer.invoke('overlay:chmetronome:close'),
    toggleCHMetronome: (): Promise<void> => ipcRenderer.invoke('overlay:chmetronome:toggle'),
    openDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:open'),
    closeDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:close'),
    toggleDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:toggle'),
    openCustomTimer: (): Promise<void> => ipcRenderer.invoke('overlay:customtimer:open'),
    closeCustomTimer: (): Promise<void> => ipcRenderer.invoke('overlay:customtimer:close'),
    toggleCustomTimer: (): Promise<void> => ipcRenderer.invoke('overlay:customtimer:toggle'),
    // Named timer-group Custom Timers windows (one per user-created group,
    // alongside the original/default window above).
    openCustomTimerGroup: (groupId: string, groupName: string): Promise<void> =>
      ipcRenderer.invoke('overlay:customtimer:group:open', groupId, groupName),
    closeCustomTimerGroup: (groupId: string): Promise<void> =>
      ipcRenderer.invoke('overlay:customtimer:group:close', groupId),
    toggleCustomTimerGroup: (groupId: string, groupName: string): Promise<void> =>
      ipcRenderer.invoke('overlay:customtimer:group:toggle', groupId, groupName),
    openTrigger: (): Promise<void> => ipcRenderer.invoke('overlay:trigger:open'),
    closeTrigger: (): Promise<void> => ipcRenderer.invoke('overlay:trigger:close'),
    toggleTrigger: (): Promise<void> => ipcRenderer.invoke('overlay:trigger:toggle'),
    openNPC: (): Promise<void> => ipcRenderer.invoke('overlay:npc:open'),
    closeNPC: (): Promise<void> => ipcRenderer.invoke('overlay:npc:close'),
    toggleNPC: (): Promise<void> => ipcRenderer.invoke('overlay:npc:toggle'),
    openDiscordVoice: (): Promise<void> => ipcRenderer.invoke('overlay:discordvoice:open'),
    closeDiscordVoice: (): Promise<void> => ipcRenderer.invoke('overlay:discordvoice:close'),
    toggleDiscordVoice: (): Promise<void> => ipcRenderer.invoke('overlay:discordvoice:toggle'),
    // Hands the main process the saved StreamKit URL so it can attach/update
    // the embedded content view — main never talks to the Go config store.
    setDiscordVoiceUrl: (url: string): Promise<void> =>
      ipcRenderer.invoke('overlay:discordvoice:set-url', url),
    clearDiscordVoice: (): Promise<void> => ipcRenderer.invoke('overlay:discordvoice:clear'),
    openThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:open'),
    closeThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:close'),
    toggleThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:toggle'),
    openRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:open'),
    closeRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:close'),
    toggleRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:toggle'),
    openRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:open'),
    closeRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:close'),
    toggleRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:toggle'),
    openZoneLockouts: (): Promise<void> => ipcRenderer.invoke('overlay:zonelockouts:open'),
    closeZoneLockouts: (): Promise<void> => ipcRenderer.invoke('overlay:zonelockouts:close'),
    toggleZoneLockouts: (): Promise<void> => ipcRenderer.invoke('overlay:zonelockouts:toggle'),
    openRaidReadiness: (): Promise<void> => ipcRenderer.invoke('overlay:raidreadiness:open'),
    closeRaidReadiness: (): Promise<void> => ipcRenderer.invoke('overlay:raidreadiness:close'),
    toggleRaidReadiness: (): Promise<void> => ipcRenderer.invoke('overlay:raidreadiness:toggle'),
    anyPopoutOpen: (): Promise<boolean> => ipcRenderer.invoke('overlay:popouts:any-open'),
    closeAllPopouts: (): Promise<void> => ipcRenderer.invoke('overlay:popouts:close-all'),
    // Hide/show all: open popout count and how many are hidden by hide-all.
    popoutVisibility: (): Promise<{ open: number; hidden: number }> =>
      ipcRenderer.invoke('overlay:popouts:visibility'),
    // Show hidden overlays, else hide open ones, else (nothing open) open
    // the active layout.
    togglePopoutVisibility: (): Promise<void> =>
      ipcRenderer.invoke('overlay:popouts:toggle-visibility'),
    // Sidebar button: show hidden overlays, else close open ones, else open
    // the active layout.
    togglePopoutsOpen: (): Promise<void> => ipcRenderer.invoke('overlay:popouts:toggle-open'),
    // Global overlay hotkeys (Settings → Hotkeys). Action ids: 'toggleAll',
    // 'overlay:<name>' or 'layout:<id>'. Accelerators use Electron's syntax (Ctrl+Alt+H).
    hotkeysGet: (): Promise<HotkeyState> => ipcRenderer.invoke('hotkeys:get'),
    hotkeysSet: (action: string, accelerator: string | null): Promise<HotkeyState> =>
      ipcRenderer.invoke('hotkeys:set', action, accelerator),
    hotkeysSetMany: (bindings: Record<string, string>): Promise<HotkeyState> =>
      ipcRenderer.invoke('hotkeys:set-many', bindings),
    hotkeysSuspend: (suspend: boolean): Promise<void> => ipcRenderer.invoke('hotkeys:suspend', suspend),
    // Collapse this overlay window to its title bar. headerFraction = header
    // height / window content height (zoom-independent).
    collapseGet: (): Promise<boolean> => ipcRenderer.invoke('overlay:collapse:get'),
    collapseSet: (collapsed: boolean, headerFraction: number): Promise<boolean> =>
      ipcRenderer.invoke('overlay:collapse:set', collapsed, headerFraction),
    onCollapseChanged: (cb: (collapsed: boolean) => void): (() => void) => {
      const listener = (_e: unknown, v: boolean): void => cb(v)
      ipcRenderer.on('overlay:collapse-changed', listener)
      return () => ipcRenderer.removeListener('overlay:collapse-changed', listener)
    },
    // Main pauses a window it hides (Hide All, the hotkey) and resumes it on
    // show: a hidden overlay drops its content and data feed until then.
    // Stacks: several overlays as tabs in one window. A stack window reads
    // its tabs and picks one; anyone can merge windows or take one out.
    stackGet: (): Promise<StackInfo | null> => ipcRenderer.invoke('overlay:stack:get'),
    stackSelect: (tab: string): Promise<void> => ipcRenderer.invoke('overlay:stack:select', tab),
    stackMerge: (target: string, source: string): Promise<boolean> =>
      ipcRenderer.invoke('overlay:stack:merge', target, source),
    stackRemove: (key: string, popOut: boolean): Promise<boolean> =>
      ipcRenderer.invoke('overlay:stack:remove', key, popOut),
    stackClose: (): Promise<void> => ipcRenderer.invoke('overlay:stack:close'),
    onStackChanged: (cb: (info: StackInfo) => void): (() => void) => {
      const listener = (_e: unknown, v: StackInfo): void => cb(v)
      ipcRenderer.on('overlay:stack:changed', listener)
      return () => ipcRenderer.removeListener('overlay:stack:changed', listener)
    },
    onStackDropHint: (cb: (hint: StackDropHint) => void): (() => void) => {
      const listener = (_e: unknown, v: StackDropHint): void => cb(v)
      ipcRenderer.on('overlay:stack:drop-hint', listener)
      return () => ipcRenderer.removeListener('overlay:stack:drop-hint', listener)
    },
    onPausedChanged: (cb: (paused: boolean) => void): (() => void) => {
      const listener = (_e: unknown, v: boolean): void => cb(v)
      ipcRenderer.on('overlay:paused', listener)
      return () => ipcRenderer.removeListener('overlay:paused', listener)
    },
    // Overlay layouts: named sets of popouts with their positions. The
    // active one saves itself as windows open, close, move or collapse.
    layoutsGet: (): Promise<LayoutState> => ipcRenderer.invoke('layouts:get'),
    layoutsSwitch: (id: string): Promise<LayoutState> => ipcRenderer.invoke('layouts:switch', id),
    // Copies the active layout under a new name and makes the copy active.
    layoutsCreate: (name: string): Promise<LayoutState> => ipcRenderer.invoke('layouts:create', name),
    layoutsRename: (id: string, name: string): Promise<LayoutState> =>
      ipcRenderer.invoke('layouts:rename', id, name),
    layoutsDelete: (id: string): Promise<LayoutState> => ipcRenderer.invoke('layouts:delete', id),
    layoutsOpenActive: (): Promise<void> => ipcRenderer.invoke('layouts:open-active'),
    layoutsSetCharacters: (id: string, names: string[]): Promise<LayoutState> =>
      ipcRenderer.invoke('layouts:set-characters', id, names),
    layoutsSwitchForCharacter: (name: string): Promise<LayoutState> =>
      ipcRenderer.invoke('layouts:switch-for-character', name),
    // One-time move of the old saved Pop Out All set into the Default layout.
    layoutsMigrateSelection: (entries: Array<string | { key: string; name: string }>): Promise<LayoutState> =>
      ipcRenderer.invoke('layouts:migrate-selection', entries),
    // The active layout's dashboard arrangement (see dashboardLayout.ts).
    layoutsDashboardGet: (): Promise<LayoutDashboardState> => ipcRenderer.invoke('layouts:dashboard-get'),
    layoutsDashboardSet: (id: string, dashboard: LayoutDashboard): Promise<void> =>
      ipcRenderer.invoke('layouts:dashboard-set', id, dashboard),
    layoutsMigrateDashboard: (dashboard: LayoutDashboard | null): Promise<LayoutDashboardState> =>
      ipcRenderer.invoke('layouts:migrate-dashboard', dashboard),
    onLayoutsChanged: (cb: (state: LayoutState) => void): (() => void) => {
      const listener = (_e: unknown, s: LayoutState): void => cb(s)
      ipcRenderer.on('layouts:changed', listener)
      return () => ipcRenderer.removeListener('layouts:changed', listener)
    },
    popoutStates: (): Promise<Record<string, boolean>> =>
      ipcRenderer.invoke('overlay:popouts:states'),
    // Open popouts by window key: 'open', or 'hidden' by hide-all/a hotkey.
    popoutWindowStates: (): Promise<Record<string, 'open' | 'hidden'>> =>
      ipcRenderer.invoke('overlay:popouts:window-states'),
    // Memory and CPU per app process, matched to windows (Developer tab).
    processMetrics: (): Promise<ProcessRow[]> => ipcRenderer.invoke('overlay:process-metrics'),
    // "Restore overlays on launch": whether to re-open the last open popout set
    // on startup, plus that remembered set. Persisted in Electron userData.
    autoOpenGet: (): Promise<{ enabled: boolean; overlays: string[] }> =>
      ipcRenderer.invoke('overlay:auto-open:get'),
    autoOpenSetEnabled: (enabled: boolean): Promise<void> =>
      ipcRenderer.invoke('overlay:auto-open:set-enabled', enabled),
    setIgnoreMouseEvents: (ignore: boolean): Promise<void> =>
      ipcRenderer.invoke('overlay:set-ignore-mouse-events', ignore),
    setTriggerMode: (mode: 'interactive' | 'passthrough' | 'hidden'): Promise<void> =>
      ipcRenderer.invoke('overlay:trigger:set-mode', mode),
    // Fired by the main process when the global Escape bail-out is hit during a
    // positioning session — the renderer ends the session cleanly in response.
    onTriggerEscape: (cb: () => void): (() => void) => {
      const listener = (): void => cb()
      ipcRenderer.on('overlay:trigger:escape', listener)
      return () => ipcRenderer.removeListener('overlay:trigger:escape', listener)
    },
    getLocked: (): Promise<boolean> => ipcRenderer.invoke('overlay:lock:get'),
    setLocked: (locked: boolean): Promise<void> =>
      ipcRenderer.invoke('overlay:lock:set', locked),
    // Fired by the main process when an overlay's lock state is changed from
    // outside its own window (e.g. a position reset auto-unlocks it), so the
    // padlock button can stay in sync without a reload.
    onLockChanged: (cb: (locked: boolean) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, locked: boolean): void => cb(locked)
      ipcRenderer.on('overlay:lock-changed', listener)
      return () => ipcRenderer.removeListener('overlay:lock-changed', listener)
    },
    // Recenter overlays on the primary monitor at default size and unlock them.
    // Driven from the main app window so an off-screen/locked overlay is still
    // recoverable.
    resetPosition: (name: string): Promise<void> =>
      ipcRenderer.invoke('overlay:reset-position', name),
    resetAllPositions: (): Promise<void> =>
      ipcRenderer.invoke('overlay:reset-all-positions'),
    // Global "Position overlays" mode: temporarily make every overlay
    // interactive so they can be dragged into place regardless of locked mode.
    getPositionMode: (): Promise<boolean> =>
      ipcRenderer.invoke('overlay:position-mode:get'),
    setPositionMode: (enabled: boolean): Promise<void> =>
      ipcRenderer.invoke('overlay:position-mode:set', enabled),
    onPositionModeChanged: (cb: (enabled: boolean) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, enabled: boolean): void => cb(enabled)
      ipcRenderer.on('overlay:position-mode-changed', listener)
      return () => ipcRenderer.removeListener('overlay:position-mode-changed', listener)
    },
    // Pin trigger alert text (and the positioning card) to one monitor.
    setDisplay: (id: number): Promise<void> =>
      ipcRenderer.invoke('overlay:set-display', id),
    // Per-overlay monitor targeting for the data overlays (DPS, timers, NPC,
    // …): which monitor each one is on, and a way to send one to a chosen
    // monitor centered on it.
    displayIds: (): Promise<Record<string, number>> =>
      ipcRenderer.invoke('overlay:display-ids'),
    moveToDisplay: (name: string, id: number): Promise<void> =>
      ipcRenderer.invoke('overlay:move-to-display', name, id),
    // Per-overlay "Move" (placing) mode: drop one overlay into a marked,
    // draggable state regardless of its lock mode, then restore it.
    place: (name: string, on: boolean): Promise<void> =>
      ipcRenderer.invoke(on ? 'overlay:place:start' : 'overlay:place:stop', name),
    placeDoneSelf: (): Promise<void> => ipcRenderer.invoke('overlay:place:stop-self'),
    amIPlacing: (): Promise<boolean> => ipcRenderer.invoke('overlay:place:am-i-placing'),
    placingNames: (): Promise<string[]> => ipcRenderer.invoke('overlay:place:names'),
    onPlacing: (cb: (placing: boolean) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, placing: boolean): void => cb(placing)
      ipcRenderer.on('overlay:placing-changed', listener)
      return () => ipcRenderer.removeListener('overlay:placing-changed', listener)
    },
    // Raid Composition ↔ Raid Readiness sync: publish a manual encounter pick
    // so every surface rendering the shared raid-readiness hook (check page,
    // dashboard panel, popout overlay) mirrors the same selection.
    setRaidSelection: (id: string): Promise<void> =>
      ipcRenderer.invoke('overlay:raid-selection:set', id),
    getRaidSelection: (): Promise<string> =>
      ipcRenderer.invoke('overlay:raid-selection:get'),
    onRaidSelectionChanged: (cb: (id: string) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, id: string): void => cb(id)
      ipcRenderer.on('overlay:raid-selection-changed', listener)
      return () => ipcRenderer.removeListener('overlay:raid-selection-changed', listener)
    },
  },
  screen: {
    triggerDefaultCenter: (): Promise<{ x: number; y: number }> =>
      ipcRenderer.invoke('screen:trigger-default-center'),
    triggerDisplays: (): Promise<Array<{ x: number; y: number; width: number; height: number }>> =>
      ipcRenderer.invoke('screen:trigger-displays'),
    listDisplays: (): Promise<
      Array<{ id: number; label: string; width: number; height: number; isPrimary: boolean; isCurrent: boolean }>
    > => ipcRenderer.invoke('screen:list-displays'),
  },
  dialog: {
    selectFolder: (): Promise<string | null> => ipcRenderer.invoke('dialog:select-folder'),
    selectSoundFile: (): Promise<string | null> => ipcRenderer.invoke('dialog:select-sound-file'),
    saveExportBundle: (suggestedName?: string): Promise<string | null> =>
      ipcRenderer.invoke('dialog:save-export-bundle', suggestedName),
    saveDebugLogs: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:save-debug-logs'),
    openImportBundle: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:open-import-bundle'),
    openSpellsetsFile: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:open-spellsets-file'),
    openMacrosFile: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:open-macros-file'),
    selectPiperExe: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:select-piper-exe'),
    selectPiperModel: (): Promise<string | null> =>
      ipcRenderer.invoke('dialog:select-piper-model'),
  },
  shell: {
    openConfigFolder: (): Promise<string> => ipcRenderer.invoke('shell:open-config-folder'),
    openLogsFolder: (): Promise<string> => ipcRenderer.invoke('shell:open-logs-folder'),
    openBackupsFolder: (): Promise<string> => ipcRenderer.invoke('shell:open-backups-folder'),
    getConfigFolderPath: (): Promise<string> => ipcRenderer.invoke('config:folder-path'),
  },
  updater: {
    check: (): Promise<void> => ipcRenderer.invoke('updater:check'),
    download: (): Promise<void> => ipcRenderer.invoke('updater:download'),
    quitAndInstall: (): Promise<void> => ipcRenderer.invoke('updater:quit-and-install'),
    onAvailable: (cb: (info: { version: string }) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, info: { version: string }) => cb(info)
      ipcRenderer.on('updater:available', listener)
      return () => ipcRenderer.removeListener('updater:available', listener)
    },
    onProgress: (cb: (p: { percent: number; transferred: number; total: number }) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, p: { percent: number; transferred: number; total: number }) => cb(p)
      ipcRenderer.on('updater:progress', listener)
      return () => ipcRenderer.removeListener('updater:progress', listener)
    },
    onDownloaded: (cb: (info: { version: string }) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, info: { version: string }) => cb(info)
      ipcRenderer.on('updater:downloaded', listener)
      return () => ipcRenderer.removeListener('updater:downloaded', listener)
    },
    onError: (cb: (message: string) => void): (() => void) => {
      const listener = (_e: Electron.IpcRendererEvent, message: string) => cb(message)
      ipcRenderer.on('updater:error', listener)
      return () => ipcRenderer.removeListener('updater:error', listener)
    },
  }
})

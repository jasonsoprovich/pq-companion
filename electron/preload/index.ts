import { contextBridge, ipcRenderer } from 'electron'

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
  },
  backend: {
    getPort: (): Promise<number> => ipcRenderer.invoke('backend:port'),
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
    openCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:open'),
    closeCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:close'),
    toggleCHChain: (): Promise<void> => ipcRenderer.invoke('overlay:chchain:toggle'),
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
    openThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:open'),
    closeThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:close'),
    toggleThreat: (): Promise<void> => ipcRenderer.invoke('overlay:threat:toggle'),
    openRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:open'),
    closeRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:close'),
    toggleRollTracker: (): Promise<void> => ipcRenderer.invoke('overlay:rolltracker:toggle'),
    openRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:open'),
    closeRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:close'),
    toggleRespawnTimer: (): Promise<void> => ipcRenderer.invoke('overlay:respawntimer:toggle'),
    anyPopoutOpen: (): Promise<boolean> => ipcRenderer.invoke('overlay:popouts:any-open'),
    // A panel entry is a plain dashboard panel key, or {key, name} for a
    // named timer-group panel — main process needs the group's display name
    // to create its window and has no independent way to know it.
    openAllPopouts: (panels?: Array<string | { key: string; name: string }>): Promise<void> =>
      ipcRenderer.invoke('overlay:popouts:open-all', panels),
    closeAllPopouts: (): Promise<void> => ipcRenderer.invoke('overlay:popouts:close-all'),
    popoutStates: (): Promise<Record<string, boolean>> =>
      ipcRenderer.invoke('overlay:popouts:states'),
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

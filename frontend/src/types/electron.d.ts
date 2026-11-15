// Type definitions for the Electron contextBridge API exposed in preload/index.ts

// Global overlay hotkeys: bound accelerators by action id, and whether each
// registered with the OS ('failed' = another program owns the combo).
export interface HotkeyState {
  bindings: Record<string, string>
  status: Record<string, 'ok' | 'failed'>
}

// Overlay layouts: named sets of popout windows. `windows` are window keys
// (overlay names, or customTimer:<groupId>).
export interface OverlayLayoutSummary {
  id: string
  name: string
  windows: string[]
  // Characters whose detected log-in switches to this layout.
  characters: string[]
  // Stacks in the layout (their tabs are in `windows` too).
  stacks: OverlayStackSummary[]
}

// A stack window: several overlays as tabs. key = `stack:<id>`.
export interface OverlayStackSummary {
  id: string
  key: string
  tabs: string[]
  groupNames: Record<string, string>
}

export interface LayoutState {
  activeId: string
  layouts: OverlayLayoutSummary[]
}

// A layout's in-app dashboard arrangement: fixed panels by panel key, named
// timer windows by group id.
export interface LayoutDashboardPanel {
  x: number
  y: number
  width: number
  height: number
  visible: boolean
}

export interface LayoutDashboard {
  panels: Record<string, LayoutDashboardPanel>
  groups: Record<string, LayoutDashboardPanel>
}

// dashboard null = defaults; migrated false = the old localStorage
// arrangement hasn't been handed over yet.
export interface LayoutDashboardState {
  id: string
  dashboard: LayoutDashboard | null
  migrated: boolean
}

// One app process with its memory (MB) and CPU (% since the last call). role
// says what it renders; keys are the overlay window keys for 'overlay'.
export interface ProcessMetricRow {
  pid: number
  role: 'overlay' | 'main' | 'trigger' | 'streamkit' | 'helper'
  keys: string[]
  state?: 'open' | 'hidden'
  type: string
  memoryMB: number
  // Windows only.
  privateMB: number | null
  cpu: number
}

// A stack window's tabs (member window keys), the one showing, and the
// display names of named timer windows among them.
export interface StackInfo {
  id: string
  key: string
  tabs: string[]
  active: string
  groupNames: Record<string, string>
}

// While dragging one overlay onto another to stack: in the target, the
// window being dropped ('target'); in the dragged one, where it would land
// ('source'). name = a timer window's display name. null = no drop armed.
export type StackDropHint = { role: 'source' | 'target'; key: string; name?: string } | null

// The hub's Arrange tab: every monitor, and every window in the active
// layout at its current spot (open/hidden) or where it will open (closed).
export interface ArrangeRect {
  x: number
  y: number
  width: number
  height: number
}

export interface ArrangeDisplay {
  id: number
  label: string
  bounds: ArrangeRect
  isPrimary: boolean
}

export interface ArrangeWindow {
  key: string
  bounds: ArrangeRect
  state: 'open' | 'hidden' | 'closed'
  collapsed: boolean
  groupName?: string
  // A stack's tabs.
  tabs?: string[]
  groupNames?: Record<string, string>
}

export interface ArrangeState {
  displays: ArrangeDisplay[]
  windows: ArrangeWindow[]
}

export interface ElectronAPI {
  app: {
    getVersion: () => Promise<string>
    relaunch: () => Promise<void>
    navigateMain: (route: string) => Promise<void>
    onNavigate: (cb: (route: string) => void) => () => void
    // App Backup/Restore: Electron-side window/overlay state the Go backend
    // can't see. See preload/index.ts for details.
    getClientState: () => Promise<unknown>
    takeClientState: () => Promise<Record<string, string>>
  }
  backend: {
    getPort: () => Promise<number>
  }
  settings: {
    getHardwareAcceleration: () => Promise<{ disabled: boolean }>
    setHardwareAcceleration: (disabled: boolean) => Promise<void>
  }
  versions: {
    node: string
    chrome: string
    electron: string
  }
  window: {
    minimize: () => Promise<void>
    maximize: () => Promise<void>
    close: () => Promise<void>
    isMaximized: () => Promise<boolean>
    isPrimary: () => Promise<boolean>
    dragStart: () => Promise<void>
    dragEnd: () => Promise<void>
    setZoom: (factor: number) => Promise<void>
    getZoom: () => number
    nativeFrame: boolean
    setMinimizeToTray: (enabled: boolean) => Promise<void>
  }
  overlay: {
    openDPS: () => Promise<void>
    closeDPS: () => Promise<void>
    toggleDPS: () => Promise<void>
    openBuffTimer: () => Promise<void>
    closeBuffTimer: () => Promise<void>
    toggleBuffTimer: () => Promise<void>
    openFilteredTimer: () => Promise<void>
    closeFilteredTimer: () => Promise<void>
    toggleFilteredTimer: () => Promise<void>
    openKillCounter: () => Promise<void>
    closeKillCounter: () => Promise<void>
    toggleKillCounter: () => Promise<void>
    openPet: () => Promise<void>
    closePet: () => Promise<void>
    togglePet: () => Promise<void>
    openDeathRecap: () => Promise<void>
    closeDeathRecap: () => Promise<void>
    toggleDeathRecap: () => Promise<void>
    openItemInfo: () => Promise<void>
    // Opens without taking focus; a no-op when already open.
    showItemInfo: () => Promise<void>
    closeItemInfo: () => Promise<void>
    toggleItemInfo: () => Promise<void>
    openCHChain: () => Promise<void>
    closeCHChain: () => Promise<void>
    toggleCHChain: () => Promise<void>
    openLiveMap: () => Promise<void>
    closeLiveMap: () => Promise<void>
    toggleLiveMap: () => Promise<void>
    openCHMetronome: () => Promise<void>
    closeCHMetronome: () => Promise<void>
    toggleCHMetronome: () => Promise<void>
    openDetrimTimer: () => Promise<void>
    closeDetrimTimer: () => Promise<void>
    toggleDetrimTimer: () => Promise<void>
    openCustomTimer: () => Promise<void>
    closeCustomTimer: () => Promise<void>
    toggleCustomTimer: () => Promise<void>
    openCustomTimerGroup: (groupId: string, groupName: string) => Promise<void>
    closeCustomTimerGroup: (groupId: string) => Promise<void>
    toggleCustomTimerGroup: (groupId: string, groupName: string) => Promise<void>
    openCustomOverlay: (id: string, name: string) => Promise<void>
    closeCustomOverlay: (id: string) => Promise<void>
    toggleCustomOverlay: (id: string, name: string) => Promise<void>
    openTrigger: () => Promise<void>
    closeTrigger: () => Promise<void>
    toggleTrigger: () => Promise<void>
    openNPC: () => Promise<void>
    closeNPC: () => Promise<void>
    toggleNPC: () => Promise<void>
    openDiscordVoice: () => Promise<void>
    closeDiscordVoice: () => Promise<void>
    toggleDiscordVoice: () => Promise<void>
    setDiscordVoiceUrl: (url: string) => Promise<void>
    clearDiscordVoice: () => Promise<void>
    openThreat: () => Promise<void>
    closeThreat: () => Promise<void>
    toggleThreat: () => Promise<void>
    openRollTracker: () => Promise<void>
    closeRollTracker: () => Promise<void>
    toggleRollTracker: () => Promise<void>
    openRespawnTimer: () => Promise<void>
    closeRespawnTimer: () => Promise<void>
    toggleRespawnTimer: () => Promise<void>
    openZoneLockouts: () => Promise<void>
    closeZoneLockouts: () => Promise<void>
    toggleZoneLockouts: () => Promise<void>
    openRaidReadiness: () => Promise<void>
    closeRaidReadiness: () => Promise<void>
    toggleRaidReadiness: () => Promise<void>
    anyPopoutOpen: () => Promise<boolean>
    closeAllPopouts: () => Promise<void>
    popoutVisibility: () => Promise<{ open: number; hidden: number }>
    togglePopoutVisibility: () => Promise<void>
    togglePopoutsOpen: () => Promise<void>
    collapseGet: () => Promise<boolean>
    collapseSet: (collapsed: boolean, headerFraction: number) => Promise<boolean>
    onCollapseChanged: (cb: (collapsed: boolean) => void) => () => void
    // Hidden windows are paused (content unmounted) until shown again.
    onPausedChanged: (cb: (paused: boolean) => void) => () => void
    stackGet: () => Promise<StackInfo | null>
    stackSelect: (tab: string) => Promise<void>
    stackMerge: (target: string, source: string) => Promise<boolean>
    // popOut with x/y: the window opens with its top-left there.
    stackRemove: (key: string, popOut: boolean, x?: number, y?: number) => Promise<boolean>
    stackClose: () => Promise<void>
    stackToggle: (key: string) => Promise<void>
    arrangeGet: () => Promise<ArrangeState>
    arrangeOpen: (key: string) => Promise<void>
    arrangeMove: (key: string, x: number, y: number) => Promise<void>
    onStackChanged: (cb: (info: StackInfo) => void) => () => void
    onStackDropHint: (cb: (hint: StackDropHint) => void) => () => void
    hotkeysGet: () => Promise<HotkeyState>
    hotkeysSet: (action: string, accelerator: string | null) => Promise<HotkeyState>
    hotkeysSetMany: (bindings: Record<string, string>) => Promise<HotkeyState>
    hotkeysSuspend: (suspend: boolean) => Promise<void>
    layoutsGet: () => Promise<LayoutState>
    setupApply: (opts: { keys: string[]; displayId: number | null; mode: 'new' | 'replace'; name?: string }) => Promise<LayoutState>
    layoutsSwitch: (id: string) => Promise<LayoutState>
    layoutsCreate: (name: string) => Promise<LayoutState>
    layoutsRename: (id: string, name: string) => Promise<LayoutState>
    layoutsDelete: (id: string) => Promise<LayoutState>
    layoutsOpenActive: () => Promise<void>
    layoutsSetCharacters: (id: string, names: string[]) => Promise<LayoutState>
    layoutsSwitchForCharacter: (name: string) => Promise<LayoutState>
    layoutsMigrateSelection: (entries: Array<string | { key: string; name: string }>) => Promise<LayoutState>
    layoutsDashboardGet: () => Promise<LayoutDashboardState>
    layoutsDashboardSet: (id: string, dashboard: LayoutDashboard) => Promise<void>
    layoutsMigrateDashboard: (dashboard: LayoutDashboard | null) => Promise<LayoutDashboardState>
    onLayoutsChanged: (cb: (state: LayoutState) => void) => () => void
    setIgnoreMouseEvents: (ignore: boolean) => Promise<void>
    setTriggerMode: (mode: 'interactive' | 'passthrough' | 'hidden') => Promise<void>
    onTriggerEscape: (cb: () => void) => () => void
    getLocked: () => Promise<boolean>
    setLocked: (locked: boolean) => Promise<void>
    onLockChanged: (cb: (locked: boolean) => void) => () => void
    resetPosition: (name: string) => Promise<void>
    resetAllPositions: () => Promise<void>
    popoutStates: () => Promise<Record<string, boolean>>
    popoutWindowStates: () => Promise<Record<string, 'open' | 'hidden'>>
    processMetrics: () => Promise<ProcessMetricRow[]>
    autoOpenGet: () => Promise<{ enabled: boolean; overlays: string[] }>
    autoOpenSetEnabled: (enabled: boolean) => Promise<void>
    getPositionMode: () => Promise<boolean>
    setPositionMode: (enabled: boolean) => Promise<void>
    onPositionModeChanged: (cb: (enabled: boolean) => void) => () => void
    setDisplay: (id: number) => Promise<void>
    displayIds: () => Promise<Record<string, number>>
    moveToDisplay: (name: string, id: number) => Promise<void>
    place: (name: string, on: boolean) => Promise<void>
    placeDoneSelf: () => Promise<void>
    amIPlacing: () => Promise<boolean>
    placingNames: () => Promise<string[]>
    onPlacing: (cb: (placing: boolean) => void) => () => void
    // Raid Composition ↔ Raid Readiness selection sync (see useRaidReadiness).
    setRaidSelection: (id: string) => Promise<void>
    getRaidSelection: () => Promise<string>
    onRaidSelectionChanged: (cb: (id: string) => void) => () => void
  }
  screen: {
    triggerDefaultCenter: () => Promise<{ x: number; y: number }>
    triggerDisplays: () => Promise<Array<{ x: number; y: number; width: number; height: number }>>
    listDisplays: () => Promise<
      Array<{ id: number; label: string; width: number; height: number; isPrimary: boolean; isCurrent: boolean }>
    >
  }
  dialog: {
    selectFolder: () => Promise<string | null>
    selectSoundFile: () => Promise<string | null>
    saveExportBundle: (suggestedName?: string) => Promise<string | null>
    saveDebugLogs: () => Promise<string | null>
    openImportBundle: () => Promise<string | null>
    openSpellsetsFile: () => Promise<string | null>
    openMacrosFile: () => Promise<string | null>
    selectPiperExe: () => Promise<string | null>
    selectPiperModel: () => Promise<string | null>
  }
  shell: {
    openConfigFolder: () => Promise<string>
    openLogsFolder: () => Promise<string>
    openBackupsFolder: () => Promise<string>
    getConfigFolderPath: () => Promise<string>
  }
  updater: {
    check: () => Promise<void>
    download: () => Promise<void>
    quitAndInstall: () => Promise<void>
    onAvailable: (cb: (info: { version: string }) => void) => () => void
    onProgress: (cb: (p: { percent: number; transferred: number; total: number }) => void) => () => void
    onDownloaded: (cb: (info: { version: string }) => void) => () => void
    onError: (cb: (message: string) => void) => () => void
  }
}

declare global {
  interface Window {
    electron: ElectronAPI
  }
}

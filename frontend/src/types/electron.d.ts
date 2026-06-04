// Type definitions for the Electron contextBridge API exposed in preload/index.ts

export interface ElectronAPI {
  app: {
    getVersion: () => Promise<string>
    relaunch: () => Promise<void>
  }
  backend: {
    getPort: () => Promise<number>
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
    dragStart: () => Promise<void>
    dragEnd: () => Promise<void>
    setZoom: (factor: number) => Promise<void>
  }
  overlay: {
    openDPS: () => Promise<void>
    closeDPS: () => Promise<void>
    toggleDPS: () => Promise<void>
    openHPS: () => Promise<void>
    closeHPS: () => Promise<void>
    toggleHPS: () => Promise<void>
    openBuffTimer: () => Promise<void>
    closeBuffTimer: () => Promise<void>
    toggleBuffTimer: () => Promise<void>
    openCHChain: () => Promise<void>
    closeCHChain: () => Promise<void>
    toggleCHChain: () => Promise<void>
    openDetrimTimer: () => Promise<void>
    closeDetrimTimer: () => Promise<void>
    toggleDetrimTimer: () => Promise<void>
    openTrigger: () => Promise<void>
    closeTrigger: () => Promise<void>
    toggleTrigger: () => Promise<void>
    openNPC: () => Promise<void>
    closeNPC: () => Promise<void>
    toggleNPC: () => Promise<void>
    openRollTracker: () => Promise<void>
    closeRollTracker: () => Promise<void>
    toggleRollTracker: () => Promise<void>
    openRespawnTimer: () => Promise<void>
    closeRespawnTimer: () => Promise<void>
    toggleRespawnTimer: () => Promise<void>
    anyPopoutOpen: () => Promise<boolean>
    openAllPopouts: (panels?: string[]) => Promise<void>
    closeAllPopouts: () => Promise<void>
    setIgnoreMouseEvents: (ignore: boolean) => Promise<void>
    setTriggerMode: (mode: 'interactive' | 'passthrough' | 'hidden') => Promise<void>
    getLocked: () => Promise<boolean>
    setLocked: (locked: boolean) => Promise<void>
  }
  screen: {
    triggerDefaultCenter: () => Promise<{ x: number; y: number }>
  }
  dialog: {
    selectFolder: () => Promise<string | null>
    selectSoundFile: () => Promise<string | null>
    saveExportBundle: (suggestedName?: string) => Promise<string | null>
    openImportBundle: () => Promise<string | null>
    openSpellsetsFile: () => Promise<string | null>
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

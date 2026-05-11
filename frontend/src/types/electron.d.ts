// Type definitions for the Electron contextBridge API exposed in preload/index.ts

export interface ElectronAPI {
  app: {
    getVersion: () => Promise<string>
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
    anyPopoutOpen: () => Promise<boolean>
    openAllPopouts: () => Promise<void>
    closeAllPopouts: () => Promise<void>
    setIgnoreMouseEvents: (ignore: boolean) => Promise<void>
    getLocked: () => Promise<boolean>
    setLocked: (locked: boolean) => Promise<void>
  }
  dialog: {
    selectFolder: () => Promise<string | null>
    selectSoundFile: () => Promise<string | null>
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

// Type definitions for the Electron contextBridge API exposed in preload/index.ts

export interface ElectronAPI {
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
  }
}

declare global {
  interface Window {
    electron: ElectronAPI
  }
}

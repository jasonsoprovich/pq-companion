import { contextBridge, ipcRenderer } from 'electron'

// Expose a safe, typed API to the renderer — no direct Node/Electron access
contextBridge.exposeInMainWorld('electron', {
  versions: {
    node: process.versions.node,
    chrome: process.versions.chrome,
    electron: process.versions.electron
  },
  window: {
    minimize: (): Promise<void> => ipcRenderer.invoke('window:minimize'),
    maximize: (): Promise<void> => ipcRenderer.invoke('window:maximize'),
    close: (): Promise<void> => ipcRenderer.invoke('window:close'),
    isMaximized: (): Promise<boolean> => ipcRenderer.invoke('window:is-maximized')
  },
  overlay: {
    openDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:open'),
    closeDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:close'),
    toggleDPS: (): Promise<void> => ipcRenderer.invoke('overlay:dps:toggle'),
    openBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:open'),
    closeBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:close'),
    toggleBuffTimer: (): Promise<void> => ipcRenderer.invoke('overlay:bufftimer:toggle'),
    openDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:open'),
    closeDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:close'),
    toggleDetrimTimer: (): Promise<void> => ipcRenderer.invoke('overlay:detrimtimer:toggle'),
  },
  dialog: {
    selectFolder: (): Promise<string | null> => ipcRenderer.invoke('dialog:select-folder'),
  },
  updater: {
    check: (): Promise<void> => ipcRenderer.invoke('updater:check'),
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

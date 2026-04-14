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
  },
  dialog: {
    selectFolder: (): Promise<string | null> => ipcRenderer.invoke('dialog:select-folder'),
  }
})

# Electron Conventions
- Main process is thin — just window management and sidecar lifecycle
- All business logic lives in Go, not in Electron main process
- Overlay windows: `transparent: true`, `alwaysOnTop: true`, frameless
- IPC only for window management commands (show/hide/resize overlays)

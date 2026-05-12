// Resolves the backend HTTP / WebSocket URLs at runtime instead of hardcoding
// localhost:8080. The Go sidecar prefers a configured port at startup but
// falls back to an OS-assigned one when that's busy; the Electron main
// process captures the actual port via stdout and exposes it through preload
// IPC. Renderer code calls into here so a port change requires no per-call
// edits anywhere else.

// Fallback used in browser-only dev (no Electron). Must match the dev
// default port in electron/main/index.ts and backend/internal/config defaults.
const DEV_FALLBACK_PORT = 17654

let cachedPort: number | null = null
let portPromise: Promise<number> | null = null

function fetchPort(): Promise<number> {
  if (typeof window === 'undefined' || !window.electron?.backend) {
    return Promise.resolve(DEV_FALLBACK_PORT)
  }
  return window.electron.backend.getPort()
}

export function getBackendPort(): Promise<number> {
  if (cachedPort !== null) return Promise.resolve(cachedPort)
  if (!portPromise) {
    portPromise = fetchPort().then((port) => {
      cachedPort = port
      return port
    })
  }
  return portPromise
}

export async function getBackendBaseUrl(): Promise<string> {
  return `http://localhost:${await getBackendPort()}`
}

export async function getBackendWsUrl(): Promise<string> {
  return `ws://localhost:${await getBackendPort()}/ws`
}

// Test-only / Settings UI helper to force a re-read after a port change.
export function clearBackendPortCache(): void {
  cachedPort = null
  portPromise = null
}

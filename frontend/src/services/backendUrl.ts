// Resolves the backend HTTP / WebSocket URLs at runtime instead of hardcoding
// a port. The Go sidecar prefers a configured port at startup but falls back
// to an OS-assigned one when that's busy; the Electron main process captures
// the actual port via stdout and exposes it through preload IPC. Renderer
// code calls into here so a port change requires no per-call edits anywhere
// else.
//
// We target 127.0.0.1 explicitly rather than "localhost" because the backend
// binds 127.0.0.1 (IPv4 only) for reliable cross-platform port conflict
// detection. On some Windows boxes "localhost" resolves to ::1 first and
// Chromium's happy-eyeballs fallback can fail under load — fetch then
// reports a generic "failed to fetch" with no clue that resolution is the
// problem. Using the literal v4 address eliminates the resolver from the
// path entirely.

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
  return `http://127.0.0.1:${await getBackendPort()}`
}

export async function getBackendWsUrl(): Promise<string> {
  return `ws://127.0.0.1:${await getBackendPort()}/ws`
}

// Test-only / Settings UI helper to force a re-read after a port change.
export function clearBackendPortCache(): void {
  cachedPort = null
  portPromise = null
}

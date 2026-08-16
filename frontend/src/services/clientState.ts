/**
 * clientState — collects and restores the renderer/Electron-side state that
 * App Backup/Restore (BackupManagerPage.tsx) bundles alongside user.db and
 * config.yaml. None of this lives in the Go backend: it's either
 * localStorage (dashboard layout, popout selection, alert mute toggles, DPS
 * mode, sidebar collapse, ...) or Electron's own userData JSON files (window
 * bounds, overlay lock state, auto-open set, display pin) — see
 * electron/main/index.ts's CLIENT_STATE_FILES.
 *
 * Capture is deliberately "everything except a denylist" rather than an
 * allowlist of known keys: a new localStorage-backed preference is covered
 * automatically the moment it's added, with no second place to remember to
 * update. That's the gap that caused this feature to under-transfer in the
 * first place.
 */

// Keys intentionally left out of an export — session-scoped or self-expiring
// state that wouldn't mean anything on a different machine/session anyway.
const TRANSIENT_KEY_PREFIXES = [
  'sql-sandbox.', // SQL Sandbox scratch history/query — a debugging aid, not a setting
  'pq.replayPrefs', // Replay panel form state; self-expires after 30 min idle
  'chMetronome:seen:', // learned per-raid call-number map, tied to the current pull
  'chMetronome:chainTarget:', // learned per-raid tank-target assignment
]

function isTransientKey(key: string): boolean {
  return TRANSIENT_KEY_PREFIXES.some((prefix) => key.startsWith(prefix))
}

export function collectLocalStorage(): Record<string, string> {
  const out: Record<string, string> = {}
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (!key || isTransientKey(key)) continue
    const value = localStorage.getItem(key)
    if (value !== null) out[key] = value
  }
  return out
}

export interface ClientStateBundle {
  local_storage: Record<string, string>
  electron: unknown
}

// Gathers the full payload for an export. electron is opaque here — it's
// whatever window.electron.app.getClientState() returns — and omitted
// entirely outside the desktop app (browser preview has no bridge).
export async function collectClientState(): Promise<ClientStateBundle> {
  const local_storage = collectLocalStorage()
  const electron = window.electron?.app?.getClientState ? await window.electron.app.getClientState() : undefined
  return { local_storage, electron }
}

// Pulls the localStorage half of a staged import (the Electron half was
// already applied at boot, before this window existed — see
// applyPendingClientState in electron/main/index.ts) and writes it in.
// Returns true if anything was restored, so the caller knows whether a
// reload is warranted.
export async function restorePendingLocalStorage(): Promise<boolean> {
  if (!window.electron?.app?.takeClientState) return false
  const restored = await window.electron.app.takeClientState()
  const keys = Object.keys(restored)
  if (keys.length === 0) return false
  for (const key of keys) {
    try {
      localStorage.setItem(key, restored[key])
    } catch {
      // localStorage full/unavailable — skip; not worth failing the whole restore.
    }
  }
  return true
}

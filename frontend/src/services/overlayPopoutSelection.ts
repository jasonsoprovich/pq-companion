/**
 * overlayPopoutSelection — the old localStorage memory of which overlays
 * "Pop Out All" reopened. Overlay layouts (Electron main process) replaced
 * it; this only remains to move a saved set into the Default layout once,
 * after which the key is removed.
 */
import { listTimerGroups } from './api'
import type { TimerGroup } from '../types/trigger'

const STORAGE_KEY = 'pq-overlay-popout-selection'

interface PopoutSelection {
  panels: string[]
  groups: string[]
}

function loadSavedSelection(): PopoutSelection | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PopoutSelection>
    return {
      panels: Array.isArray(parsed.panels) ? parsed.panels : [],
      groups: Array.isArray(parsed.groups) ? parsed.groups : [],
    }
  } catch {
    return null
  }
}

// Main ignores this once it has migrated (or seeded Default from the
// restore-on-launch set), so it's safe to call on every startup. Group
// windows need their display names, which only the backend has.
export function migratePopoutSelection(): void {
  const o = window.electron?.overlay
  if (!o?.layoutsMigrateSelection) return
  const sel = loadSavedSelection()
  const send = (groups: TimerGroup[]): void => {
    const entries: Array<string | { key: string; name: string }> = [...(sel?.panels ?? [])]
    for (const id of sel?.groups ?? []) {
      const g = groups.find((x) => x.id === id)
      if (g) entries.push({ key: `customTimer:${id}`, name: g.name })
    }
    o.layoutsMigrateSelection(entries)
      .then(() => {
        try {
          localStorage.removeItem(STORAGE_KEY)
        } catch {
          // localStorage unavailable — harmless, main won't migrate twice.
        }
      })
      .catch(() => {})
  }
  if (sel && sel.groups.length > 0) {
    listTimerGroups().then(send).catch(() => send([]))
  } else {
    send([])
  }
}

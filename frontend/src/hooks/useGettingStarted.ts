// useGettingStarted computes the Getting Started checklist. Items come from
// durable state (EQ folder, overlay layouts, triggers, Zeal) or from saved
// milestones in preferences.getting_started (log seen, search used, Zeal
// skipped), so a finished checklist stays finished after a restart even
// with EQ closed. Nothing here depends on live freshness except recording
// the log milestone the first time it's seen.
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  detectZeal,
  getConfig,
  getLogStatus,
  listTriggers,
  saveConfigChanges,
  validateEQPath,
} from '../services/api'
import type { GettingStartedState } from '../types/config'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { saveLogSeen } from './useSetupHealth'

export type ChecklistId = 'eq' | 'log' | 'zeal' | 'overlays' | 'triggers' | 'search'

export interface ChecklistItem {
  id: ChecklistId
  label: string
  detail: string
  done: boolean
}

export interface GettingStarted {
  items: ChecklistItem[]
  doneCount: number
  total: number
  dismissed: boolean
  loaded: boolean
  setDismissed: (v: boolean) => void
  skipZeal: () => void
  refresh: () => void
}

const FRESH_MS = 10 * 60 * 1000

async function patchGettingStarted(patch: Partial<GettingStartedState>): Promise<void> {
  const c = await getConfig()
  await saveConfigChanges(
    {
      ...c,
      preferences: {
        ...c.preferences,
        getting_started: { ...(c.preferences?.getting_started ?? {}), ...patch },
      },
    },
    c,
  )
}

async function overlaysConfigured(): Promise<boolean> {
  const api = window.electron?.overlay
  const layouts = await api?.layoutsGet?.().catch(() => null)
  return !!layouts?.layouts.some((l) => l.windows.length > 0)
}

async function compute(
  skipWhenDismissed: boolean,
): Promise<{ items: ChecklistItem[]; dismissed: boolean }> {
  const cfg = await getConfig()
  const gs = cfg.preferences?.getting_started ?? {}
  // The sidebar pill has nothing to show once the list is dismissed (or was
  // auto-dismissed on completion), so skip the folder, Zeal, layout and
  // trigger checks entirely.
  if (skipWhenDismissed && gs.dismissed) return { items: [], dismissed: true }
  const eqPath = cfg.eq_path?.trim() ?? ''
  const [valid, zeal, overlays, triggers, log] = await Promise.all([
    eqPath ? validateEQPath(eqPath).then((v) => v.valid).catch(() => false) : Promise.resolve(false),
    eqPath ? detectZeal().catch(() => null) : Promise.resolve(null),
    overlaysConfigured(),
    listTriggers().then((t) => t.length).catch(() => 0),
    gs.log_seen_at ? Promise.resolve(null) : getLogStatus().catch(() => null),
  ])
  let logSeen = !!gs.log_seen_at
  if (!logSeen && log?.last_write_at && Date.now() - new Date(log.last_write_at).getTime() < FRESH_MS) {
    logSeen = true
    saveLogSeen()
  }
  const items: ChecklistItem[] = [
    {
      id: 'eq',
      label: 'Set your EverQuest folder',
      detail: 'Tells PQ Companion where your log and Zeal files are.',
      done: valid,
    },
    {
      id: 'log',
      label: 'Turn on logging',
      detail: 'Type /log on in game. Live features read your log.',
      done: logSeen,
    },
    {
      id: 'zeal',
      label: 'Install Zeal (optional)',
      detail: 'Adds spellbook, inventory and live target data.',
      done: !!zeal?.installed || !!gs.zeal_skipped,
    },
    {
      id: 'overlays',
      label: 'Set up overlays',
      detail: 'Pick timers, meters and more to show over the game.',
      done: overlays,
    },
    {
      id: 'triggers',
      label: 'Add triggers',
      detail: 'Install a pack or make your own alerts.',
      done: triggers > 0,
    },
    {
      id: 'search',
      label: 'Try Ctrl+K search',
      detail: 'Find items, spells, pages, settings and help from anywhere.',
      done: !!gs.used_search_at,
    },
  ]
  return { items, dismissed: !!gs.dismissed }
}

// Focus refreshes (every alt-tab back from EQ) are throttled to this.
const FOCUS_REFRESH_MS = 60 * 1000

export function useGettingStarted(
  opts: { delayMs?: number; skipWhenDismissed?: boolean } = {},
): GettingStarted {
  const skipWhenDismissed = !!opts.skipWhenDismissed
  const [items, setItems] = useState<ChecklistItem[]>([])
  const [dismissed, setDismissedState] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const seq = useRef(0)
  const lastRun = useRef(0)

  const refresh = useCallback(() => {
    const n = ++seq.current
    lastRun.current = Date.now()
    compute(skipWhenDismissed)
      .then((r) => {
        if (n !== seq.current) return
        setItems(r.items)
        setDismissedState(r.dismissed)
        setLoaded(true)
        // Finishing everything retires the sidebar pill for good.
        if (!r.dismissed && r.items.every((i) => i.done)) {
          setDismissedState(true)
          void patchGettingStarted({ dismissed: true }).catch(() => undefined)
        }
      })
      .catch(() => undefined)
  }, [skipWhenDismissed])

  useEffect(() => {
    // The sidebar pill delays its first check so it stays off the startup
    // critical path.
    const t = window.setTimeout(refresh, opts.delayMs ?? 0)
    const onFocus = (): void => {
      if (Date.now() - lastRun.current >= FOCUS_REFRESH_MS) refresh()
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.clearTimeout(t)
      window.removeEventListener('focus', onFocus)
    }
  }, [refresh, opts.delayMs])

  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (msg.type === WSEvent.ConfigUpdated) refresh()
      },
      [refresh],
    ),
  )

  const setDismissed = useCallback((v: boolean) => {
    setDismissedState(v)
    void patchGettingStarted({ dismissed: v }).catch(() => undefined)
  }, [])

  const skipZeal = useCallback(() => {
    void patchGettingStarted({ zeal_skipped: true }).catch(() => undefined)
  }, [])

  const doneCount = items.filter((i) => i.done).length
  return { items, doneCount, total: items.length, dismissed, loaded, setDismissed, skipZeal, refresh }
}

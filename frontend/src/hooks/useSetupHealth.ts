// useSetupHealth combines the existing status sources (config, EQ path
// validation, log tailer status, Zeal detection and pipe status) into a
// list of green / amber / red checks with a "Fix" link each. Used by the
// Feature health section in Settings and the Getting Started checklist.
//
// Rules:
// - The character check uses the tailer's resolved character, so a blank
//   config character (auto-detect) with a live log is healthy.
// - Log activity is neutral (grey), not a warning, when the file exists but
//   hasn't been written recently: that's normal while EQ is closed. Red is
//   only for confirmed problems (no folder, no log file, parser off).
// - The hook also returns the eqclient.ini / zeal.ini diagnostics and the
//   Zeal settings checklist, so the Settings health section and the Getting
//   Started checklist read the same state. It refreshes on a poll, on
//   ConfigUpdated / ZealSettings events, and on the window event
//   SETUP_HEALTH_CHANGED (fired after an ini toggle).
// - The first time fresh log activity is seen, the Getting Started
//   log_seen_at milestone is saved so the checklist stays ticked.
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  detectZeal,
  getConfig,
  getEqDiagnostics,
  getLogStatus,
  getZealPipeStatus,
  getZealSettings,
  saveConfigChanges,
  validateEQPath,
  type EqDiagnostics,
} from '../services/api'
import type { ZealSettings } from '../types/zeal'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export type HealthLevel = 'ok' | 'warn' | 'error' | 'off'

export interface HealthCheck {
  id: 'eq-folder' | 'parser' | 'character' | 'log-file' | 'log-activity' | 'zeal' | 'zeal-pipe'
  label: string
  level: HealthLevel
  detail: string
  fix?: { label: string; to: string }
}

export interface SetupHealth {
  checks: HealthCheck[]
  // eqclient.ini Log / zeal.ini ExportOnCamp state; null until loaded or
  // when the EQ folder isn't usable.
  diag: EqDiagnostics | null
  zealSettings: ZealSettings | null
  parserOn: boolean
  loading: boolean
  refresh: () => void
  // Apply a diagnostics response from a toggle right away (then refresh).
  applyDiag: (d: EqDiagnostics) => void
}

// Fired on window after anything changes setup state outside a config save
// (the eqclient.ini / zeal.ini toggles), so every health consumer refreshes.
export const SETUP_HEALTH_CHANGED = 'pq:setup-health-changed'

const FRESH_MS = 10 * 60 * 1000
const POLL_MS = 15000

const settingsLink = (anchor: string): string => `/settings?tab=general&focus=${anchor}`

export function ago(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (!Number.isFinite(ms) || ms < 0) return 'just now'
  const m = Math.round(ms / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m} min ago`
  const h = Math.round(m / 60)
  if (h < 48) return `${h} h ago`
  return `${Math.round(h / 24)} days ago`
}

interface Built {
  checks: HealthCheck[]
  fresh: boolean
  diag: EqDiagnostics | null
  zealSettings: ZealSettings | null
  parserOn: boolean
}

async function buildChecks(): Promise<Built> {
  const cfg = await getConfig()
  const checks: HealthCheck[] = []
  const eqPath = cfg.eq_path?.trim() ?? ''

  // EQ folder
  let folderOk = false
  if (!eqPath) {
    checks.push({
      id: 'eq-folder',
      label: 'EverQuest folder',
      level: 'error',
      detail: 'Not set. Choose the folder that contains eqgame.exe.',
      fix: { label: 'Set folder', to: settingsLink('eq-install') },
    })
  } else {
    const v = await validateEQPath(eqPath).catch(() => null)
    folderOk = !!v?.valid
    checks.push({
      id: 'eq-folder',
      label: 'EverQuest folder',
      level: folderOk ? 'ok' : 'error',
      detail: folderOk ? eqPath : v?.error || `Can't use ${eqPath}.`,
      fix: folderOk ? undefined : { label: 'Fix folder', to: settingsLink('eq-install') },
    })
  }

  // Parser
  const parserOn = !!cfg.preferences?.parse_combat_log
  checks.push({
    id: 'parser',
    label: 'Log parsing',
    level: parserOn ? 'ok' : 'error',
    detail: parserOn
      ? 'On'
      : 'Off. Live features (DPS, timers, triggers, overlays) need it.',
    fix: parserOn ? undefined : { label: 'Turn on', to: settingsLink('preferences') },
  })

  // Character, log file, activity
  const log = folderOk ? await getLogStatus().catch(() => null) : null
  const auto = !cfg.character?.trim()
  if (!folderOk) {
    checks.push({ id: 'character', label: 'Character', level: 'off', detail: 'Set the EverQuest folder first.' })
    checks.push({ id: 'log-file', label: 'Log file', level: 'off', detail: 'Set the EverQuest folder first.' })
    checks.push({ id: 'log-activity', label: 'Log activity', level: 'off', detail: 'Set the EverQuest folder first.' })
  } else if (!log?.character) {
    checks.push({
      id: 'character',
      label: 'Character',
      level: 'warn',
      detail: 'No character log yet. Log in and type /log on in game.',
      fix: { label: 'Choose', to: settingsLink('eq-install') },
    })
    checks.push({ id: 'log-file', label: 'Log file', level: 'off', detail: 'Waiting for a character.' })
    checks.push({ id: 'log-activity', label: 'Log activity', level: 'off', detail: 'Waiting for a character.' })
  } else {
    checks.push({
      id: 'character',
      label: 'Character',
      level: 'ok',
      detail: auto ? `${log.character} (auto-detected)` : log.character,
    })
    checks.push({
      id: 'log-file',
      label: 'Log file',
      level: log.file_exists ? 'ok' : 'error',
      detail: log.file_exists ? log.file_path : `Not found: ${log.file_path}. Type /log on in game.`,
      fix: log.file_exists ? undefined : { label: 'Logging', to: settingsLink('eq-logging') },
    })
    if (!log.file_exists || !log.last_write_at) {
      checks.push({ id: 'log-activity', label: 'Log activity', level: 'off', detail: 'No log file yet.' })
    } else {
      const fresh = Date.now() - new Date(log.last_write_at).getTime() < FRESH_MS
      checks.push({
        id: 'log-activity',
        label: 'Log activity',
        level: fresh ? 'ok' : 'off',
        detail: fresh
          ? `Written ${ago(log.last_write_at)}`
          : `Last written ${ago(log.last_write_at)}. Normal while EverQuest is closed.`,
      })
    }
  }

  // Zeal (optional)
  const zeal = folderOk ? await detectZeal().catch(() => null) : null
  if (!folderOk) {
    checks.push({ id: 'zeal', label: 'Zeal', level: 'off', detail: 'Set the EverQuest folder first.' })
    checks.push({ id: 'zeal-pipe', label: 'Zeal live data', level: 'off', detail: 'Set the EverQuest folder first.' })
  } else if (!zeal?.installed) {
    checks.push({
      id: 'zeal',
      label: 'Zeal',
      level: 'warn',
      detail: 'Not installed (optional). Adds spellbook, inventory and live target data.',
      fix: { label: 'Install', to: settingsLink('zeal') },
    })
    checks.push({ id: 'zeal-pipe', label: 'Zeal live data', level: 'off', detail: 'Needs Zeal.' })
  } else {
    const level: HealthLevel = !zeal.version_ok ? 'error' : zeal.update_available ? 'warn' : 'ok'
    checks.push({
      id: 'zeal',
      label: 'Zeal',
      level,
      detail: !zeal.version_ok
        ? `Version ${zeal.version ?? '?'} is too old (needs ${zeal.min_version ?? 'newer'}).`
        : zeal.update_available
          ? `Version ${zeal.version}; ${zeal.latest_version} is available.`
          : `Version ${zeal.version ?? 'installed'}`,
      fix: level === 'ok' ? undefined : { label: 'Update', to: settingsLink('zeal') },
    })
    const pipe = await getZealPipeStatus().catch(() => null)
    const connected = pipe?.state === 'connected'
    checks.push({
      id: 'zeal-pipe',
      label: 'Zeal live data',
      level: pipe?.state === 'unsupported' ? 'off' : connected ? 'ok' : 'warn',
      detail: connected
        ? `Connected${pipe?.character ? ` (${pipe.character})` : ''}`
        : pipe?.state === 'unsupported'
          ? 'Not available on this system.'
          : 'Not connected. Start EverQuest with Zeal loaded.',
    })
  }

  const fresh = checks.some((c) => c.id === 'log-activity' && c.level === 'ok')
  const [diag, zealSettings] = folderOk
    ? await Promise.all([
        getEqDiagnostics().catch(() => null),
        zeal?.installed ? getZealSettings().catch(() => null) : Promise.resolve(null),
      ])
    : [null, null]
  return { checks, fresh, diag, zealSettings, parserOn }
}

let logSeenSaved = false
export function saveLogSeen(): void {
  if (logSeenSaved) return
  logSeenSaved = true
  getConfig()
    .then((c) => {
      const gs = c.preferences?.getting_started ?? {}
      if (gs.log_seen_at) return
      return saveConfigChanges(
        {
          ...c,
          preferences: {
            ...c.preferences,
            getting_started: { ...gs, log_seen_at: new Date().toISOString() },
          },
        },
        c,
      )
    })
    .catch(() => {
      logSeenSaved = false
    })
}

export function useSetupHealth(): SetupHealth {
  const [checks, setChecks] = useState<HealthCheck[]>([])
  const [diag, setDiag] = useState<EqDiagnostics | null>(null)
  const [zealSettings, setZealSettings] = useState<ZealSettings | null>(null)
  const [parserOn, setParserOn] = useState(false)
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)

  const refresh = useCallback(() => {
    const n = ++seq.current
    buildChecks()
      .then((b) => {
        if (n !== seq.current) return
        setChecks(b.checks)
        setDiag(b.diag)
        setZealSettings(b.zealSettings)
        setParserOn(b.parserOn)
        if (b.fresh) saveLogSeen()
      })
      .catch(() => undefined)
      .finally(() => {
        if (n === seq.current) setLoading(false)
      })
  }, [])

  const applyDiag = useCallback(
    (d: EqDiagnostics) => {
      setDiag(d)
      refresh()
    },
    [refresh],
  )

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, POLL_MS)
    window.addEventListener(SETUP_HEALTH_CHANGED, refresh)
    return () => {
      window.clearInterval(t)
      window.removeEventListener(SETUP_HEALTH_CHANGED, refresh)
    }
  }, [refresh])

  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (
          msg.type === WSEvent.ConfigUpdated ||
          msg.type === WSEvent.ZealSettings ||
          msg.type === WSEvent.ZealConnected ||
          msg.type === WSEvent.ZealDisconnected
        ) {
          refresh()
        }
      },
      [refresh],
    ),
  )

  return { checks, diag, zealSettings, parserOn, loading, refresh, applyDiag }
}

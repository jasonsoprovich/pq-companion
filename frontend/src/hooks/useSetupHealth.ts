// useSetupHealth combines the existing status sources (config, EQ path
// validation, log tailer status, Zeal detection and pipe status) into a
// list of green / amber / red checks with a "Fix" link each. Used by the
// Setup health card in Settings and the Getting Started checklist.
//
// Rules:
// - The character check uses the tailer's resolved character, so a blank
//   config character (auto-detect) with a live log is healthy.
// - Log activity is amber, not red, when the file exists but hasn't been
//   written recently: EQ may simply be closed. Red is only for confirmed
//   problems (no folder, no log file, parser off).
// - The first time fresh log activity is seen, the Getting Started
//   log_seen_at milestone is saved so the checklist stays ticked.
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  detectZeal,
  getConfig,
  getLogStatus,
  getZealPipeStatus,
  saveConfigChanges,
  validateEQPath,
} from '../services/api'
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
  loading: boolean
  refresh: () => void
}

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

async function buildChecks(): Promise<{ checks: HealthCheck[]; fresh: boolean }> {
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
        level: fresh ? 'ok' : 'warn',
        detail: fresh
          ? `Written ${ago(log.last_write_at)}`
          : `Last written ${ago(log.last_write_at)}. EQ may be closed, or type /log on.`,
        fix: fresh ? undefined : { label: 'Logging', to: settingsLink('eq-logging') },
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
  return { checks, fresh }
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
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)

  const refresh = useCallback(() => {
    const n = ++seq.current
    buildChecks()
      .then(({ checks: c, fresh }) => {
        if (n !== seq.current) return
        setChecks(c)
        if (fresh) saveLogSeen()
      })
      .catch(() => undefined)
      .finally(() => {
        if (n === seq.current) setLoading(false)
      })
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, POLL_MS)
    return () => window.clearInterval(t)
  }, [refresh])

  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (msg.type === WSEvent.ConfigUpdated) refresh()
      },
      [refresh],
    ),
  )

  return { checks, loading, refresh }
}

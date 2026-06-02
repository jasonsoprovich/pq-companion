import React, { useEffect, useRef, useState } from 'react'
import { Settings, FolderOpen, Save, AlertTriangle, CheckCircle2, Loader2, X, RefreshCw, Trash2, HardDrive, Sparkles, Volume2, VolumeX, Wifi, Layers, FileText, Palette, Code2 } from 'lucide-react'
import { getConfig, updateConfig, getLogStatus, getLogFileInfo, cleanupLog, getServerInfo, testPortAvailability, detectZeal, getZealPipeStatus, getQuarmClientStatus, type ServerInfo, type TestPortResult } from '../services/api'
import type { Config, DPSClassColors, NPCOverlaySections } from '../types/config'
import { DEFAULT_DPS_CLASS_COLORS, DEFAULT_NPC_OVERLAY_SECTIONS } from '../types/config'
import { OVERLAY_DEFS, resolveLockedMode } from '../lib/overlays'
import type { OverlayName, LockedMode } from '../lib/overlays'
import type { LogFileInfo } from '../types/logEvent'
import { applyContrast } from '../hooks/useHighContrast'
import { applyZoom } from '../hooks/useZoom'
import type { ZealInstallStatus, ZealPipeStatus } from '../types/zeal'
import type { QuarmClientStatus, QuarmFileStatus } from '../types/quarm'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

const ZEAL_RELEASE_URL = 'https://github.com/CoastalRedwood/Zeal/releases/latest'
const QUARM_PATCHER_RELEASE_URL = 'https://github.com/Pkelly668/QuarmPatcher/releases/latest'

// formatManifestDate converts a YYYYMMDD manifest date into a readable form.
// Returns the original string if it doesn't match the expected layout — the
// upstream manifest format is stable but we'd rather show the raw value than
// throw away information on an unexpected shape.
function formatManifestDate(d: string): string {
  if (!/^\d{8}$/.test(d)) return d
  return `${d.slice(0, 4)}-${d.slice(4, 6)}-${d.slice(6, 8)}`
}

import BackupManagerPage from './BackupManagerPage'
import DeveloperTab from './DeveloperTab'


type SaveState = 'idle' | 'saving' | 'saved' | 'discarded' | 'error'
type UpdateState = 'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'downloaded' | 'error'
type Tab = 'general' | 'overlays' | 'spelltimers' | 'dpscolors' | 'logs' | 'backups' | 'advanced' | 'developer'

interface TabBarProps {
  tabs: { id: Tab; label: string; icon: React.ReactNode }[]
  active: Tab
  onChange: (t: Tab) => void
}

function TabBar({ tabs, active, onChange }: TabBarProps): React.ReactElement {
  return (
    <div
      className="flex shrink-0 border-b"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {tabs.map(({ id, label, icon }) => (
        <button
          key={id}
          onClick={() => onChange(id)}
          className="flex items-center gap-1.5 border-b-2 px-4 py-2.5 text-xs font-medium transition-colors"
          style={{
            borderBottomColor: active === id ? 'var(--color-primary)' : 'transparent',
            color: active === id ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            backgroundColor: 'transparent',
          }}
        >
          {icon}
          {label}
        </button>
      ))}
    </div>
  )
}

// QuarmFileRow renders one client DLL's status: a colored verdict pill plus
// detail lines for local version/build-date and the matching manifest entry.
// Used by the "EQ Client Version" section.
function QuarmFileRow({ file }: { file: QuarmFileStatus }): React.ReactElement {
  const verdict = (() => {
    switch (file.status) {
      case 'match':
        return { label: 'Up to date', color: '#22c55e', icon: <CheckCircle2 size={12} /> }
      case 'mismatch':
        return { label: 'Out of date', color: '#f59e0b', icon: <AlertTriangle size={12} /> }
      case 'missing':
        return { label: 'Missing', color: '#f87171', icon: <AlertTriangle size={12} /> }
      case 'unknown':
      default:
        return { label: 'Unknown', color: 'var(--color-muted)', icon: <FileText size={12} /> }
    }
  })()

  return (
    <div
      className="mb-2 rounded px-3 py-2 text-xs last:mb-0"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="flex items-center justify-between">
        <code style={{ color: 'var(--color-foreground)' }}>{file.name}</code>
        <span
          className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-[11px]"
          style={{ color: verdict.color, border: `1px solid ${verdict.color}` }}
        >
          {verdict.icon}
          {verdict.label}
        </span>
      </div>

      {file.local && (
        <div className="mt-1 space-y-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
          {file.local.file_version && (
            <div>
              FileVersion: <span style={{ color: 'var(--color-foreground)' }}>{file.local.file_version}</span>
            </div>
          )}
          <div>
            Built: <span style={{ color: 'var(--color-foreground)' }}>
              {new Date(file.local.compiled_at).toLocaleDateString()}
            </span>
            {' · '}
            Size: <span style={{ color: 'var(--color-foreground)' }}>{file.local.size.toLocaleString()} bytes</span>
          </div>
          <div className="font-mono">
            MD5: <span style={{ color: 'var(--color-foreground)' }}>{file.local.md5}</span>
          </div>
        </div>
      )}

      {file.manifest && (
        <div className="mt-1 space-y-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
          <div>
            Manifest expects size{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.size.toLocaleString()}</span>
            , dated{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{formatManifestDate(file.manifest.date)}</span>
          </div>
          {file.manifest.ref_file_version && (
            <div>
              Manifest FileVersion:{' '}
              <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.ref_file_version}</span>
            </div>
          )}
          {file.local && file.local.md5 !== file.manifest.md5 && (
            <div className="font-mono">
              Manifest MD5: <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.md5}</span>
            </div>
          )}
        </div>
      )}

      {file.reason && (
        <p className="mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
          {file.reason}
        </p>
      )}
    </div>
  )
}

export default function SettingsPage(): React.ReactElement {
  const [tab, setTab] = useState<Tab>('general')
  const [config, setConfig] = useState<Config | null>(null)
  const [originalConfig, setOriginalConfig] = useState<Config | null>(null)
  // Latest saved config, mirrored into a ref so the unmount cleanup below can
  // read it without re-subscribing.
  const originalConfigRef = useRef<Config | null>(null)
  useEffect(() => {
    originalConfigRef.current = originalConfig
  }, [originalConfig])
  // Leaving Settings discards unsaved changes; revert any unsaved high-contrast
  // or zoom preview to the saved value so the preview doesn't leak past this
  // page.
  useEffect(() => {
    return () => {
      const oc = originalConfigRef.current
      if (oc) {
        applyContrast(Boolean(oc.preferences.high_contrast))
        applyZoom(oc.preferences.zoom_factor)
      }
    }
  }, [])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const [updateState, setUpdateState] = useState<UpdateState>('idle')
  const [updateVersion, setUpdateVersion] = useState<string | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)

  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null)
  const [portTestState, setPortTestState] = useState<'idle' | 'testing'>('idle')
  const [portTestResult, setPortTestResult] = useState<TestPortResult | null>(null)
  const [portTestPort, setPortTestPort] = useState<number | null>(null)
  const [configFolder, setConfigFolder] = useState<string | null>(null)

  const [zealStatus, setZealStatus] = useState<ZealInstallStatus | null>(null)
  const [zealChecking, setZealChecking] = useState(false)
  const [zealError, setZealError] = useState<string | null>(null)
  const [pipeStatus, setPipeStatus] = useState<ZealPipeStatus | null>(null)

  const [quarmStatus, setQuarmStatus] = useState<QuarmClientStatus | null>(null)
  const [quarmChecking, setQuarmChecking] = useState(false)
  const [quarmError, setQuarmError] = useState<string | null>(null)

  const [logLargeFile, setLogLargeFile] = useState(false)
  const [logFileInfo, setLogFileInfo] = useState<LogFileInfo | null>(null)
  const [logInfoLoading, setLogInfoLoading] = useState(false)
  const [cleanupState, setCleanupState] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const [cleanupResult, setCleanupResult] = useState<string | null>(null)
  const logPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const saveStateClearRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
    }
  }, [])

  async function checkZeal(): Promise<void> {
    setZealChecking(true)
    setZealError(null)
    try {
      const [install, pipe] = await Promise.all([
        detectZeal(),
        getZealPipeStatus().catch(() => null),
      ])
      setZealStatus(install)
      setPipeStatus(pipe)
    } catch (err) {
      setZealError((err as Error).message)
      setZealStatus(null)
    } finally {
      setZealChecking(false)
    }
  }

  async function checkQuarmClient(): Promise<void> {
    setQuarmChecking(true)
    setQuarmError(null)
    try {
      const status = await getQuarmClientStatus()
      setQuarmStatus(status)
    } catch (err) {
      setQuarmError((err as Error).message)
      setQuarmStatus(null)
    } finally {
      setQuarmChecking(false)
    }
  }

  // Subscribe to backend pipe state changes so the status row reflects
  // connect/disconnect without the user having to click Re-check. The Re-check
  // button is still useful for re-running the filesystem detection when the
  // user has just installed Zeal — that signal doesn't come over the wire.
  useWebSocket((msg: WsMessage) => {
    if (msg.type !== WSEvent.ZealConnected && msg.type !== WSEvent.ZealDisconnected) return
    getZealPipeStatus().then(setPipeStatus).catch(() => null)
  })

  function flashSaveState(state: SaveState, ms = 2500): void {
    if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
    setSaveState(state)
    saveStateClearRef.current = setTimeout(() => {
      setSaveState('idle')
      setSaveError(null)
    }, ms)
  }


  useEffect(() => {
    getConfig()
      .then((c) => {
        setConfig(c)
        setOriginalConfig(c)
      })
      .catch((err: Error) => setLoadError(err.message))

    if (window.electron?.app) {
      window.electron.app.getVersion().then(setAppVersion).catch(() => null)
    }

    // Pre-fetch the config folder path so the error fallback can show it
    // immediately if the backend fetch above fails.
    if (window.electron?.shell) {
      window.electron.shell.getConfigFolderPath().then(setConfigFolder).catch(() => null)
    }

    getServerInfo().then(setServerInfo).catch(() => null)

    void checkZeal()
    void checkQuarmClient()

    const pollLogSize = () => {
      getLogStatus()
        .then((s) => setLogLargeFile(s.large_file))
        .catch(() => null)
    }
    pollLogSize()
    logPollRef.current = setInterval(pollLogSize, 10 * 60 * 1000)
    return () => {
      if (logPollRef.current) clearInterval(logPollRef.current)
    }
  }, [])

  useEffect(() => {
    if (!window.electron?.updater) return
    const offAvailable = window.electron.updater.onAvailable((info) => {
      setUpdateState('available')
      setUpdateVersion(info.version)
    })
    const offProgress = window.electron.updater.onProgress(() => {
      setUpdateState('downloading')
    })
    const offDownloaded = window.electron.updater.onDownloaded((info) => {
      setUpdateState('downloaded')
      setUpdateVersion(info.version)
    })
    const offError = window.electron.updater.onError((msg) => {
      setUpdateState('error')
      setUpdateError(msg)
    })
    return () => {
      offAvailable()
      offProgress()
      offDownloaded()
      offError()
    }
  }, [])

  // Parses a configured listen address (e.g. ":17654", "127.0.0.1:17654") into
  // a port number. Returns 0 when the addr is the auto-assign sentinel ":0".
  function parsePortFromAddr(addr: string): number {
    const m = addr.match(/:(\d+)$/)
    if (m) return Number(m[1])
    const n = Number(addr)
    return Number.isFinite(n) ? n : 0
  }

  async function handlePortTest(): Promise<void> {
    if (!config) return
    const port = parsePortFromAddr(config.server_addr)
    if (port === 0) {
      // No specific port to test — auto-assign will always succeed.
      setPortTestResult({ available: true })
      setPortTestPort(0)
      return
    }
    setPortTestState('testing')
    setPortTestResult(null)
    try {
      const result = await testPortAvailability(port)
      setPortTestResult(result)
      setPortTestPort(port)
    } catch (err) {
      setPortTestResult({ available: false, error: (err as Error).message })
      setPortTestPort(port)
    } finally {
      setPortTestState('idle')
    }
  }

  async function handleBrowse(): Promise<void> {
    if (!window.electron?.dialog) return
    const folder = await window.electron.dialog.selectFolder()
    if (folder && config) {
      setConfig({ ...config, eq_path: folder })
    }
  }

  function handleCancel(): void {
    if (originalConfig) {
      setConfig(originalConfig)
      // Revert any optimistic high-contrast / zoom preview.
      applyContrast(Boolean(originalConfig.preferences.high_contrast))
      applyZoom(originalConfig.preferences.zoom_factor)
    }
    flashSaveState('discarded')
  }

  async function handleSave(): Promise<void> {
    if (!config) return
    if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
    setSaveState('saving')
    setSaveError(null)
    try {
      const saved = await updateConfig(config)
      setConfig(saved)
      setOriginalConfig(saved)
      flashSaveState('saved')
    } catch (err) {
      setSaveError((err as Error).message)
      setSaveState('error')
    }
  }

  async function handleCheckForUpdates(): Promise<void> {
    if (!window.electron?.updater) return
    setUpdateState('checking')
    setUpdateError(null)
    await window.electron.updater.check()
    // If no event fires within 4s, assume up to date
    setTimeout(() => {
      setUpdateState((prev) => (prev === 'checking' ? 'up-to-date' : prev))
    }, 4_000)
  }

  function handleQuitAndInstall(): void {
    window.electron?.updater?.quitAndInstall()
  }

  function handleLoadLogInfo(): void {
    setLogInfoLoading(true)
    getLogFileInfo()
      .then((fi) => setLogFileInfo(fi))
      .catch(() => null)
      .finally(() => setLogInfoLoading(false))
  }

  async function handleCleanupLog(): Promise<void> {
    setCleanupState('running')
    setCleanupResult(null)
    try {
      const result = await cleanupLog()
      setCleanupResult(result.backup_path)
      setCleanupState('done')
      setLogLargeFile(false)
      setLogFileInfo(null)
      // Re-poll size after purge
      getLogStatus()
        .then((s) => setLogLargeFile(s.large_file))
        .catch(() => null)
    } catch (err) {
      setCleanupResult((err as Error).message)
      setCleanupState('error')
    }
  }

  const developerMode = config?.preferences?.developer_mode ?? false

  // Ctrl+Shift+D toggles the hidden Developer tab while the Settings page is
  // focused. Persists via the preferences PUT so it survives restarts.
  // Deliberately a chord (not a single key) so it can't fire by accident.
  useEffect(() => {
    if (!config) return
    const handler = (e: KeyboardEvent): void => {
      if (!(e.ctrlKey || e.metaKey) || !e.shiftKey) return
      if (e.key !== 'D' && e.key !== 'd') return
      e.preventDefault()
      const next: Config = {
        ...config,
        preferences: { ...config.preferences, developer_mode: !developerMode },
      }
      setConfig(next)
      updateConfig(next)
        .then((saved) => setOriginalConfig(saved))
        .catch(() => null)
      // Hop straight to the tab the first time the user reveals it so the
      // unlock isn't silent. When hiding, fall back to General.
      setTab(developerMode ? 'general' : 'developer')
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [config, developerMode])

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'general', label: 'General', icon: <Settings size={13} /> },
    { id: 'overlays', label: 'Overlays', icon: <Layers size={13} /> },
    { id: 'spelltimers', label: 'Spell Timers', icon: <Sparkles size={13} /> },
    { id: 'dpscolors', label: 'DPS Class Colors', icon: <Palette size={13} /> },
    { id: 'logs', label: 'Logs', icon: <FileText size={13} /> },
    { id: 'backups', label: 'EQ Config Backups', icon: <HardDrive size={13} /> },
    { id: 'advanced', label: 'Advanced', icon: <Wifi size={13} /> },
    ...(developerMode
      ? [{ id: 'developer' as Tab, label: 'Developer', icon: <Code2 size={13} /> }]
      : []),
  ]

  if (loadError && tab !== 'backups') {
    const configPath = configFolder ? `${configFolder}\\config.yaml` : null
    return (
      <div className="flex h-full flex-col">
        <TabBar tabs={tabs} active={tab} onChange={setTab} />
        <div className="flex flex-1 items-start justify-center overflow-y-auto p-6">
          <div className="flex w-full max-w-xl flex-col gap-4">
            <div className="flex items-center gap-3">
              <AlertTriangle size={22} style={{ color: '#f97316' }} />
              <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Can't reach the backend
              </h2>
            </div>

            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Settings failed to load: <span className="font-mono">{loadError}</span>.
              The PQ Companion backend service isn't responding on its local port.
            </p>

            <div
              className="rounded border p-3 text-xs"
              style={{
                borderColor: 'var(--color-border)',
                backgroundColor: 'var(--color-muted-background, rgba(255,255,255,0.02))',
                color: 'var(--color-muted-foreground)',
              }}
            >
              <p className="mb-2 font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Most common cause: antivirus
              </p>
              <p className="mb-2">
                Windows Defender or another antivirus may have quarantined
                <span className="font-mono"> pq-companion-server.exe</span>.
                Check Windows Security → Virus &amp; threat protection →
                Protection history. If the file is listed, restore it and add
                an exclusion, then relaunch PQ Companion.
              </p>
              <p className="mb-2">
                Other possibilities: a stuck listener on the configured port,
                or a firewall blocking loopback traffic. As a workaround you
                can change the port manually in <span className="font-mono">config.yaml</span>:
              </p>
              {configPath && (
                <p className="font-mono break-all" style={{ color: 'var(--color-foreground)' }}>
                  {configPath}
                </p>
              )}
            </div>

            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => {
                  if (window.electron?.shell) {
                    window.electron.shell.openConfigFolder().catch(() => null)
                  }
                }}
                disabled={!window.electron?.shell}
                className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-primary-foreground, #000)',
                }}
              >
                <FolderOpen size={13} />
                Open config folder
              </button>
              <button
                type="button"
                onClick={() => {
                  if (window.electron?.shell) {
                    window.electron.shell.openLogsFolder().catch(() => null)
                  }
                }}
                disabled={!window.electron?.shell}
                className="inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
                style={{
                  borderColor: 'var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <FileText size={13} />
                Open logs folder
              </button>
              <button
                type="button"
                onClick={() => window.location.reload()}
                className="inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium transition-colors"
                style={{
                  borderColor: 'var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <RefreshCw size={13} />
                Retry
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (!config && tab !== 'backups') {
    return (
      <div className="flex h-full flex-col">
        <TabBar tabs={tabs} active={tab} onChange={setTab} />
        <div className="flex flex-1 flex-col items-center justify-center gap-3">
          <Loader2 size={24} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Loading settings…
          </p>
        </div>
      </div>
    )
  }

  if (tab === 'backups') {
    return (
      <div className="flex h-full flex-col">
        <TabBar tabs={tabs} active={tab} onChange={setTab} />
        <div className="min-h-0 flex-1">
          <BackupManagerPage />
        </div>
      </div>
    )
  }

  if (tab === 'developer') {
    return (
      <div className="flex h-full flex-col">
        <TabBar tabs={tabs} active={tab} onChange={setTab} />
        <div className="min-h-0 flex-1 overflow-y-auto">
          <DeveloperTab />
        </div>
      </div>
    )
  }

  if (!config) return <></>


  const hasElectronDialog = Boolean(window.electron?.dialog)
  const hasElectronUpdater = Boolean(window.electron?.updater)

  return (
    <div className="flex h-full flex-col">
      <TabBar tabs={tabs} active={tab} onChange={setTab} />
      <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-xl p-6">
      {/* Page header */}
      <div className="mb-6 flex items-center gap-3">
        <Settings size={20} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Settings
        </h1>
      </div>

      <div className="flex flex-col gap-6">
        {/* ── App ──────────────────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-3 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            App
          </h2>

          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Version
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {appVersion ? `v${appVersion}` : 'Unknown'}
              </p>
            </div>

            {hasElectronUpdater && (
              <div className="flex items-center gap-2">
                {updateState === 'downloaded' && (
                  <button
                    onClick={handleQuitAndInstall}
                    className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
                    style={{
                      backgroundColor: '#22c55e',
                      color: '#fff',
                      border: 'none',
                      cursor: 'pointer',
                    }}
                  >
                    Install v{updateVersion} &amp; Restart
                  </button>
                )}
                {updateState !== 'downloaded' && (
                  <button
                    onClick={handleCheckForUpdates}
                    disabled={updateState === 'checking' || updateState === 'downloading'}
                    className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                      cursor: updateState === 'checking' || updateState === 'downloading' ? 'not-allowed' : 'pointer',
                      opacity: updateState === 'checking' || updateState === 'downloading' ? 0.7 : 1,
                    }}
                  >
                    {updateState === 'checking' || updateState === 'downloading' ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : (
                      <RefreshCw size={12} />
                    )}
                    {updateState === 'checking'
                      ? 'Checking…'
                      : updateState === 'downloading'
                        ? 'Downloading…'
                        : 'Check for Updates'}
                  </button>
                )}
              </div>
            )}
          </div>

          {updateState === 'up-to-date' && (
            <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#22c55e' }}>
              <CheckCircle2 size={12} />
              Up to date
            </p>
          )}
          {updateState === 'available' && (
            <p className="mt-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              v{updateVersion} available — downloading…
            </p>
          )}
          {updateState === 'error' && updateError && (
            <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} />
              {updateError}
            </p>
          )}
        </section>
        )}

        {/* ── Backend Network ────────────────────────────────────────────── */}
        {tab === 'advanced' && (
        <>
        <BackendNetworkSection
          config={config}
          setConfig={setConfig}
          originalServerAddr={originalConfig?.server_addr ?? config.server_addr}
          serverInfo={serverInfo}
          parsePortFromAddr={parsePortFromAddr}
          portTestState={portTestState}
          portTestResult={portTestResult}
          portTestPort={portTestPort}
          onTest={handlePortTest}
          onReset={() => {
            setConfig({ ...config, server_addr: ':0' })
            setPortTestResult(null)
          }}
          onSave={handleSave}
          saving={saveState === 'saving'}
        />
        <DiagnosticsSection />
        </>
        )}

        {/* ── EverQuest Path ─────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            EverQuest Installation
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Path to your EverQuest folder — used to find log files and Zeal exports.
          </p>

          <div className="flex gap-2">
            <input
              type="text"
              value={config.eq_path}
              onChange={(e) => setConfig({ ...config, eq_path: e.target.value })}
              placeholder="e.g. C:\EverQuest"
              className="flex-1 rounded px-3 py-2 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                outline: 'none',
              }}
            />
            {hasElectronDialog && (
              <button
                onClick={handleBrowse}
                className="flex items-center gap-1.5 rounded px-3 py-2 text-sm font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                  whiteSpace: 'nowrap',
                }}
              >
                <FolderOpen size={14} />
                Browse
              </button>
            )}
          </div>

          {config.eq_path && (
            <p className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
              Log file: <code style={{ color: 'var(--color-foreground)' }}>
                {config.eq_path}/eqlog_{config.character || '<auto>'}_pq.proj.txt
              </code>
            </p>
          )}

          <div className="mt-3 flex items-center justify-between gap-2">
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Want to redo first-launch setup?
            </p>
            <button
              onClick={() => window.dispatchEvent(new Event('pq:open-onboarding'))}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
            >
              <Sparkles size={12} />
              Run Setup Wizard
            </button>
          </div>
        </section>
        )}

        {/* ── Zeal integration ───────────────────────────────────────────── */}
        {tab === 'general' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-1 flex items-center justify-between">
            <h2
              className="text-sm font-semibold uppercase tracking-wide"
              style={{ color: 'var(--color-muted)' }}
            >
              Zeal Integration
            </h2>
            <button
              onClick={() => void checkZeal()}
              disabled={zealChecking}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: zealChecking ? 'not-allowed' : 'pointer',
                opacity: zealChecking ? 0.5 : 1,
              }}
            >
              {zealChecking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {zealChecking ? 'Checking…' : 'Re-check'}
            </button>
          </div>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Zeal is a community EverQuest add-on that exposes live target, HP,
            buff, and group state. PQ Companion uses it for real-time overlays
            when installed and falls back to log parsing when not.
          </p>

          {zealStatus?.installed && (
            <div className="space-y-2">
              <div className="flex items-start gap-2 text-sm" style={{ color: '#22c55e' }}>
                <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                <div>
                  <p>
                    Zeal is installed
                    {zealStatus.version ? ` (v${zealStatus.version})` : ''}.
                  </p>
                  {zealStatus.asi_path && (
                    <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      Found <code>{zealStatus.asi_path}</code>
                    </p>
                  )}
                </div>
              </div>

              {zealStatus.update_available && zealStatus.latest_version && (
                <div
                  className="flex items-start gap-2 rounded px-3 py-2 text-xs"
                  style={{
                    backgroundColor: 'rgba(245, 158, 11, 0.10)',
                    border: '1px solid rgba(245, 158, 11, 0.35)',
                    color: 'var(--color-text)',
                  }}
                >
                  <AlertTriangle size={12} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
                  <div className="flex-1">
                    A newer Zeal is available:{' '}
                    <span style={{ color: '#fcd34d' }}>v{zealStatus.latest_version}</span>{' '}
                    (you have v{zealStatus.version}). Pq-companion still works, but updating
                    keeps you on the latest fixes.
                  </div>
                  <a
                    href={ZEAL_RELEASE_URL}
                    target="_blank"
                    rel="noreferrer noopener"
                    style={{ color: '#fcd34d', textDecoration: 'underline' }}
                  >
                    Release notes
                  </a>
                </div>
              )}

              {zealStatus.export_on_camp_found && !zealStatus.export_on_camp && (
                <div
                  className="flex items-start gap-2 rounded px-3 py-2 text-xs"
                  style={{
                    backgroundColor: 'rgba(245, 158, 11, 0.10)',
                    border: '1px solid rgba(245, 158, 11, 0.35)',
                    color: 'var(--color-text)',
                  }}
                >
                  <AlertTriangle size={12} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
                  <div className="flex-1">
                    Zeal&apos;s <code>ExportOnCamp</code> is disabled. Character inventory,
                    spellbook, and stats will not refresh on /camp.{' '}
                    <span style={{ color: 'var(--color-muted-foreground)' }}>
                      Fix: edit <code>zeal.ini</code>, set{' '}
                      <code>ExportOnCamp=TRUE</code> under <code>[Zeal]</code>, then relaunch EQ.
                    </span>
                  </div>
                </div>
              )}

              {zealStatus.export_on_camp_found && zealStatus.export_on_camp && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  <code>ExportOnCamp</code> is enabled — character data refreshes when you /camp.
                </p>
              )}
            </div>
          )}

          {zealStatus && !zealStatus.installed && !zealError && (
            <div className="space-y-2">
              <p className="text-sm font-medium" style={{ color: '#f87171' }}>
                Zeal is not installed in your configured EverQuest folder
                {zealStatus.eqgame_present ? '' : ' (eqgame.exe also not found — verify the path above)'}.
              </p>
              <p
                className="text-xs font-semibold uppercase tracking-wide"
                style={{ color: 'var(--color-muted)' }}
              >
                Installing Zeal unlocks
              </p>
              <ul
                className="ml-4 list-disc space-y-0.5 text-xs"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                <li>Real-time target detection without <code>/con</code></li>
                <li>Live target HP bar + pet-owner attribution</li>
                <li>Authoritative damage attribution in the DPS meter</li>
                <li>Trigger conditions on target HP, buff slots, and <code>/pipe</code> alerts</li>
              </ul>
              <a
                href={ZEAL_RELEASE_URL}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: '#fff',
                  textDecoration: 'none',
                }}
              >
                Get Zeal (GitHub releases)
              </a>
            </div>
          )}

          {zealError && (
            <div className="flex items-start gap-2 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <p>Couldn&apos;t check for Zeal: {zealError}</p>
            </div>
          )}

          {!zealStatus && !zealError && !zealChecking && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Set your EverQuest folder above, then click Re-check.
            </p>
          )}

          {pipeStatus && pipeStatus.state !== 'unsupported' && (
            <div
              className="mt-3 space-y-2 rounded px-3 py-2 text-xs"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span
                    className="inline-block h-2 w-2 rounded-full"
                    style={{
                      backgroundColor:
                        pipeStatus.state === 'connected'
                          ? '#22c55e'
                          : pipeStatus.state === 'disconnected'
                            ? '#f87171'
                            : 'var(--color-muted)',
                    }}
                  />
                  <span style={{ color: 'var(--color-foreground)' }}>
                    Live pipe:{' '}
                    {pipeStatus.state === 'connected'
                      ? `connected${pipeStatus.character ? ` (${pipeStatus.character})` : ''}`
                      : pipeStatus.state === 'disconnected'
                        ? 'disconnected'
                        : 'waiting for EverQuest'}
                  </span>
                </div>
                {pipeStatus.state === 'connected' && pipeStatus.pid ? (
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    PID {pipeStatus.pid}
                  </span>
                ) : null}
              </div>
              {pipeStatus.state !== 'connected' && pipeStatus.last_error && (
                <pre
                  className="whitespace-pre-wrap break-words font-mono"
                  style={{ color: 'var(--color-muted-foreground)', fontSize: '11px' }}
                >
                  {pipeStatus.last_error}
                </pre>
              )}
            </div>
          )}
        </section>
        )}

        {/* ── EQ client version ──────────────────────────────────────────── */}
        {tab === 'general' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-1 flex items-center justify-between">
            <h2
              className="text-sm font-semibold uppercase tracking-wide"
              style={{ color: 'var(--color-muted)' }}
            >
              EQ Client Version
            </h2>
            <button
              onClick={() => void checkQuarmClient()}
              disabled={quarmChecking}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: quarmChecking ? 'not-allowed' : 'pointer',
                opacity: quarmChecking ? 0.5 : 1,
              }}
            >
              {quarmChecking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {quarmChecking ? 'Checking…' : 'Re-check'}
            </button>
          </div>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Compares <code>eqgame.dll</code> in your EverQuest folder against the
            public{' '}
            <a
              href={QUARM_PATCHER_RELEASE_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="underline"
              style={{ color: 'var(--color-primary)' }}
            >
              Quarm Patcher manifest
            </a>
            . Informational only — PQ Companion does not patch game files.
          </p>

          {quarmError && (
            <div className="flex items-start gap-2 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <p>Couldn&apos;t check EQ client: {quarmError}</p>
            </div>
          )}

          {!quarmError && quarmStatus && quarmStatus.files.map((f) => (
            <QuarmFileRow key={f.name} file={f} />
          ))}

          {!quarmStatus && !quarmError && !quarmChecking && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Set your EverQuest folder above, then click Re-check.
            </p>
          )}

          {!quarmError && quarmStatus?.manifest_error && (
            <p
              className="mt-2 text-xs"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              Manifest unreachable ({quarmStatus.manifest_error}). Local DLL info
              is still shown below.
            </p>
          )}
        </section>
        )}

        {/* ── Preferences ────────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-3 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Preferences
          </h2>

          <label className="flex cursor-pointer items-center justify-between py-1">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Parse Combat Log
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Enable real-time log parsing for DPS meter and overlays.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    parse_combat_log: !config.preferences.parse_combat_log,
                  },
                })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.parse_combat_log
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.parse_combat_log ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          <div className="mt-4">
            <div className="mb-1 flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Master Volume
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Scales all trigger sounds and TTS alerts. Each trigger keeps its own volume — this dampens everything together.
                </p>
              </div>
              <span className="text-xs font-mono shrink-0 ml-3" style={{ color: 'var(--color-muted-foreground)' }}>
                {config.preferences.master_volume ?? 100}%
              </span>
            </div>
            <div className="flex items-center gap-2 mt-2">
              {(config.preferences.master_volume ?? 100) === 0 ? (
                <VolumeX size={14} style={{ color: 'var(--color-muted)' }} />
              ) : (
                <Volume2 size={14} style={{ color: 'var(--color-muted)' }} />
              )}
              <input
                type="range"
                min={0}
                max={100}
                step={1}
                value={config.preferences.master_volume ?? 100}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    preferences: {
                      ...config.preferences,
                      master_volume: Number(e.target.value),
                    },
                  })
                }
                style={{ flex: 1, accentColor: 'var(--color-primary)', cursor: 'pointer' }}
              />
            </div>
          </div>

          <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Minimize to Tray
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Hide to system tray instead of closing when the window is dismissed.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    minimize_to_tray: !config.preferences.minimize_to_tray,
                  },
                })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.minimize_to_tray
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.minimize_to_tray ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                High Contrast Text
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Brighten muted text and borders for easier reading on
                high-resolution displays.
              </p>
            </div>
            <div
              onClick={() => {
                const next = !config.preferences.high_contrast
                // Apply immediately for instant feedback; the hook keeps it in
                // sync after the change is saved.
                applyContrast(next)
                setConfig({
                  ...config,
                  preferences: { ...config.preferences, high_contrast: next },
                })
              }}
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.high_contrast
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.high_contrast ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          <div className="py-1 mt-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Zoom
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Scale the whole app up or down for readability on
                  high-resolution displays.
                </p>
              </div>
              <span className="text-sm font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
                {Math.round((config.preferences.zoom_factor || 1) * 100)}%
              </span>
            </div>
            <input
              type="range"
              min={80}
              max={150}
              step={5}
              value={Math.round((config.preferences.zoom_factor || 1) * 100)}
              onChange={(e) => {
                const factor = (parseInt(e.target.value) || 100) / 100
                // Apply immediately for live feedback; the hook keeps it in
                // sync after saving.
                applyZoom(factor)
                setConfig({
                  ...config,
                  preferences: { ...config.preferences, zoom_factor: factor },
                })
              }}
              className="w-full mt-2"
            />
          </div>
        </section>
        )}

        {/* ── Overlays ───────────────────────────────────────────────────── */}
        {tab === 'overlays' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Overlays
          </h2>
          <p className="mb-4 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Controls transparency for all popout overlay windows (DPS, Buff Timers, NPC, Triggers).
          </p>

          <div className="flex items-center gap-4">
            <div style={{ flex: 1 }}>
              <div className="mb-1 flex items-center justify-between">
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Opacity
                </p>
                <span className="text-xs font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
                  {Math.round(config.preferences.overlay_opacity * 100)}%
                </span>
              </div>
              <input
                type="range"
                min={10}
                max={100}
                step={1}
                value={Math.round(config.preferences.overlay_opacity * 100)}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    preferences: {
                      ...config.preferences,
                      overlay_opacity: Number(e.target.value) / 100,
                    },
                  })
                }
                style={{ width: '100%', accentColor: 'var(--color-primary)', cursor: 'pointer' }}
              />
              <div className="mt-1 flex justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
                <span>10%</span>
                <span>100%</span>
              </div>
            </div>

            {/* Preview swatch */}
            <div
              style={{
                width: 48,
                height: 48,
                borderRadius: 6,
                border: '1px solid rgba(255,255,255,0.15)',
                backgroundColor: `rgba(10,10,12,${config.preferences.overlay_opacity})`,
                flexShrink: 0,
              }}
              title="Overlay background preview"
            />
          </div>
        </section>
        )}

        {/* ── Overlay lock behaviour ─────────────────────────────────────── */}
        {tab === 'overlays' && (
        <OverlayLockModeCard
          modes={config.preferences.overlay_locked_modes ?? {}}
          onChange={(next) =>
            setConfig({
              ...config,
              preferences: {
                ...config.preferences,
                overlay_locked_modes: next,
              },
            })
          }
        />
        )}

        {/* ── NPC Overlay sections ───────────────────────────────────────── */}
        {tab === 'overlays' && (
        <NPCOverlaySectionsCard
          dashboard={
            config.preferences.npc_overlay_dashboard_sections ??
            DEFAULT_NPC_OVERLAY_SECTIONS
          }
          popout={
            config.preferences.npc_overlay_popout_sections ??
            DEFAULT_NPC_OVERLAY_SECTIONS
          }
          onChangeDashboard={(next) =>
            setConfig({
              ...config,
              preferences: {
                ...config.preferences,
                npc_overlay_dashboard_sections: next,
              },
            })
          }
          onChangePopout={(next) =>
            setConfig({
              ...config,
              preferences: {
                ...config.preferences,
                npc_overlay_popout_sections: next,
              },
            })
          }
        />
        )}

        {/* ── Spell Timers ───────────────────────────────────────────────── */}
        {tab === 'spelltimers' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Spell Timers
          </h2>
          <p className="mb-4 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Controls which spells the timer overlays track.
          </p>

          <div>
            <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Tracking mode
            </p>
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <b>Auto</b> (default) creates a timer for every recognised spell landing; triggers/packs add alerts and thresholds on top. <b>Triggers only</b> shows just the spells you've curated through trigger packs or custom triggers — useful if you want a focused overlay instead of full auto-coverage.
            </p>
            <div className="mb-4 flex gap-2">
              {([
                { value: 'auto' as const, label: 'Auto' },
                { value: 'triggers_only' as const, label: 'Triggers only' },
              ]).map(({ value, label }) => {
                const active = (config.spell_timer?.tracking_mode ?? 'auto') === value
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() =>
                      setConfig({
                        ...config,
                        spell_timer: { ...config.spell_timer, tracking_mode: value },
                      })
                    }
                    className="rounded px-3 py-1.5 text-xs font-medium"
                    style={{
                      backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                      color: active ? '#000' : 'var(--color-foreground)',
                      border: '1px solid var(--color-border)',
                      cursor: 'pointer',
                      minWidth: 110,
                    }}
                  >
                    {label}
                  </button>
                )
              })}
            </div>

            <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Tracking scope
            </p>
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <b>Self only</b> tracks just buffs landing on you. <b>Cast by me</b> (default) also includes anything you cast on others — without the noise of other players buffing each other. <b>Anyone</b> tracks every recognised land, useful when you want to see e.g. another enchanter's debuff on a raid mob. (Ignored in <b>Triggers only</b> mode.)
            </p>
            <div className="flex gap-2">
              {([
                { value: 'self' as const, label: 'Self only' },
                { value: 'cast_by_me' as const, label: 'Cast by me' },
                { value: 'anyone' as const, label: 'Anyone' },
              ]).map(({ value, label }) => {
                const active = (config.spell_timer?.tracking_scope ?? 'cast_by_me') === value
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() =>
                      setConfig({
                        ...config,
                        spell_timer: { ...config.spell_timer, tracking_scope: value },
                      })
                    }
                    className="rounded px-3 py-1.5 text-xs font-medium"
                    style={{
                      backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                      color: active ? '#000' : 'var(--color-foreground)',
                      border: '1px solid var(--color-border)',
                      cursor: 'pointer',
                      minWidth: 90,
                    }}
                  >
                    {label}
                  </button>
                )
              })}
            </div>
          </div>

          <div className="mt-4">
            <label className="flex items-start gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={config.spell_timer?.class_filter ?? false}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    spell_timer: { ...config.spell_timer, class_filter: e.target.checked },
                  })
                }
                style={{ marginTop: 3 }}
              />
              <span>
                <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Filter buffs to my class
                </span>
                <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Drops buffs your class can't cast (e.g. paladin Spiritual Purity, shaman Talisman, bard songs) so an enchanter's overlay isn't cluttered with raid buffs from other classes. Detrimentals you cast are always tracked. Combine with <b>Anyone</b> scope to see other same-class casters' buffs across the raid.
                </span>
              </span>
            </label>
          </div>

          <div className="mt-4">
            <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Display thresholds
            </p>
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Hide overlay rows whose remaining time exceeds the threshold (in seconds). Set to <b>0</b> to always show — useful for keeping long-duration raid buffs out of view until they're close to expiring. Per-trigger overrides take precedence.
            </p>
            <div className="flex gap-3">
              {([
                { label: 'Buff', key: 'buff_display_threshold_secs' as const },
                { label: 'Detrimental', key: 'detrim_display_threshold_secs' as const },
              ]).map(({ label, key }) => (
                <label key={key} className="flex flex-col gap-1" style={{ flex: 1 }}>
                  <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
                  <input
                    type="number"
                    min={0}
                    step={1}
                    value={config.spell_timer?.[key] ?? 0}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        spell_timer: {
                          ...config.spell_timer,
                          [key]: Math.max(0, Number(e.target.value) || 0),
                        },
                      })
                    }
                    className="rounded px-2 py-1 text-sm"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      color: 'var(--color-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  />
                </label>
              ))}
            </div>
          </div>
        </section>
        )}

        {/* ── DPS Class Colors ───────────────────────────────────────────── */}
        {tab === 'dpscolors' && (
        <DPSClassColorsSection
          value={config.dps_class_colors ?? DEFAULT_DPS_CLASS_COLORS}
          onChange={(next) => setConfig({ ...config, dps_class_colors: next })}
        />
        )}

        {/* ── Log Files ──────────────────────────────────────────────────── */}
        {tab === 'logs' && (
        <section
          className="rounded-lg p-4"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: logLargeFile
              ? '1px solid #f97316'
              : '1px solid var(--color-border)',
          }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide flex items-center gap-2"
            style={{ color: 'var(--color-muted)' }}
          >
            Log Files
            {logLargeFile && (
              <span
                className="rounded-full px-2 py-0.5 text-[10px] font-semibold"
                style={{ backgroundColor: '#f97316', color: '#fff' }}
              >
                Large file detected
              </span>
            )}
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Archives the current EverQuest log file to a <code className="font-mono">.bak.txt</code> next to it, then trims the live log to the last 7 days of entries. Files over 75 MB are flagged for cleanup. Unrelated to the EQ Config Backups tab — that one protects your <code className="font-mono">.ini</code> files.
          </p>

          {/* Load file info */}
          {!logFileInfo && cleanupState === 'idle' && (
            <button
              onClick={handleLoadLogInfo}
              disabled={logInfoLoading}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: logInfoLoading ? 'not-allowed' : 'pointer',
                opacity: logInfoLoading ? 0.7 : 1,
              }}
            >
              {logInfoLoading ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {logInfoLoading ? 'Loading…' : 'Check Log File'}
            </button>
          )}

          {/* File info display */}
          {logFileInfo && cleanupState === 'idle' && (
            <div className="mb-3 space-y-1">
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Size: <span style={{ color: 'var(--color-foreground)' }}>{(logFileInfo.size_bytes / 1024 / 1024).toFixed(1)} MB</span>
              </p>
              {logFileInfo.oldest_entry && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Oldest entry: <span style={{ color: 'var(--color-foreground)' }}>{new Date(logFileInfo.oldest_entry).toLocaleDateString()}</span>
                </p>
              )}
              {logFileInfo.newest_entry && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Newest entry: <span style={{ color: 'var(--color-foreground)' }}>{new Date(logFileInfo.newest_entry).toLocaleDateString()}</span>
                </p>
              )}
              <div className="flex gap-2 pt-2">
                <button
                  onClick={handleCleanupLog}
                  className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
                  style={{
                    backgroundColor: '#f97316',
                    color: '#fff',
                    border: 'none',
                    cursor: 'pointer',
                  }}
                >
                  <Trash2 size={12} />
                  Archive &amp; Trim
                </button>
                <button
                  onClick={() => setLogFileInfo(null)}
                  className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    cursor: 'pointer',
                  }}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {/* Running */}
          {cleanupState === 'running' && (
            <p className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <Loader2 size={12} className="animate-spin" />
              Backing up and purging log…
            </p>
          )}

          {/* Done */}
          {cleanupState === 'done' && cleanupResult && (
            <div className="space-y-1">
              <p className="flex items-center gap-1.5 text-xs" style={{ color: '#22c55e' }}>
                <CheckCircle2 size={12} />
                Archive complete. Saved to:
              </p>
              <p className="text-xs font-mono break-all" style={{ color: 'var(--color-muted-foreground)' }}>
                {cleanupResult}
              </p>
              <button
                onClick={() => { setCleanupState('idle'); setCleanupResult(null) }}
                className="mt-2 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                }}
              >
                <RefreshCw size={12} />
                Check Again
              </button>
            </div>
          )}

          {/* Error */}
          {cleanupState === 'error' && cleanupResult && (
            <div className="space-y-1">
              <p className="flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
                <AlertTriangle size={12} />
                {cleanupResult}
              </p>
              <button
                onClick={() => { setCleanupState('idle'); setCleanupResult(null) }}
                className="mt-2 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                }}
              >
                Try Again
              </button>
            </div>
          )}
        </section>
        )}

        {/* ── Save / Discard buttons ─────────────────────────────────────── */}
        <div className="flex items-center gap-3">
          <button
            onClick={handleSave}
            disabled={saveState === 'saving'}
            className="flex items-center gap-2 rounded px-4 py-2 text-sm font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: saveState === 'saving' ? 'not-allowed' : 'pointer',
              opacity: saveState === 'saving' ? 0.7 : 1,
            }}
          >
            {saveState === 'saving' ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <Save size={14} />
            )}
            {saveState === 'saving' ? 'Saving…' : 'Save Settings'}
          </button>

          <button
            onClick={handleCancel}
            disabled={saveState === 'saving'}
            className="flex items-center gap-2 rounded px-4 py-2 text-sm font-semibold"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              cursor: saveState === 'saving' ? 'not-allowed' : 'pointer',
              opacity: saveState === 'saving' ? 0.7 : 1,
            }}
          >
            <X size={14} />
            Discard
          </button>

          {saveState === 'saved' && (
            <span className="flex items-center gap-1.5 text-sm" style={{ color: '#22c55e' }}>
              <CheckCircle2 size={14} />
              Settings saved
            </span>
          )}

          {saveState === 'discarded' && (
            <span className="flex items-center gap-1.5 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              <X size={14} />
              Changes discarded
            </span>
          )}

          {saveState === 'error' && saveError && (
            <span className="flex items-center gap-1.5 text-sm" style={{ color: '#f87171' }}>
              <AlertTriangle size={14} />
              {saveError}
            </span>
          )}
        </div>

        {/* ── Config file location note ──────────────────────────────────── */}
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Settings are stored at{' '}
          <code style={{ color: 'var(--color-foreground)' }}>~/.pq-companion/config.yaml</code>
        </p>
      </div>
      </div>
      </div>
    </div>
  )
}

// ── Backend Network section ──────────────────────────────────────────────────
// Shows the port the local API server is actually listening on, lets the user
// override the preferred port (used at next launch), and probes availability
// before they commit a change. `actual_port` reflects the running server;
// `preferred_addr` is whatever's saved in config — they only differ when the
// last startup fell back from a busy port to an OS-assigned one.

interface BackendNetworkSectionProps {
  config: Config
  setConfig: (c: Config) => void
  originalServerAddr: string
  serverInfo: ServerInfo | null
  parsePortFromAddr: (addr: string) => number
  portTestState: 'idle' | 'testing'
  portTestResult: TestPortResult | null
  portTestPort: number | null
  onTest: () => void
  onReset: () => void
  onSave: () => void
  saving: boolean
}

function BackendNetworkSection(props: BackendNetworkSectionProps): React.ReactElement {
  const {
    config, setConfig, originalServerAddr, serverInfo, parsePortFromAddr,
    portTestState, portTestResult, portTestPort, onTest, onReset, onSave, saving,
  } = props

  const preferredPort = parsePortFromAddr(config.server_addr)
  const isAuto = preferredPort === 0
  const actualPort = serverInfo?.actual_port ?? null
  const fellBack = serverInfo !== null
    && !isAuto
    && actualPort !== null
    && preferredPort !== actualPort
  const dirty = config.server_addr !== originalServerAddr

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        <Wifi size={13} />
        Backend Network
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        The companion app talks to its own local backend over a loopback port.
        The default is fine for most users — only change this if another local
        service is taking the port and the app can&rsquo;t start.
      </p>

      <div className="mb-3 grid grid-cols-2 gap-2 rounded p-3 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
      >
        <span style={{ color: 'var(--color-muted-foreground)' }}>Current port</span>
        <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>
          {actualPort ?? '—'}
        </span>
        <span style={{ color: 'var(--color-muted-foreground)' }}>Preferred</span>
        <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>
          {isAuto ? 'auto-assign' : preferredPort}
        </span>
      </div>

      {fellBack && (
        <p className="mb-3 flex items-start gap-1.5 text-xs" style={{ color: '#f97316' }}>
          <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          <span>
            Preferred port <b>{preferredPort}</b> was unavailable at startup —
            another process on your machine is using it. The app fell back to
            port {actualPort}. To make this stable, either stop the conflicting
            service or pick a different preferred port below.
          </span>
        </p>
      )}

      <div className="mb-2">
        <label className="mb-1 block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Preferred port (used at next app launch)
        </label>
        <div className="flex gap-2">
          <input
            type="number"
            min={0}
            max={65535}
            step={1}
            value={preferredPort}
            onChange={(e) => {
              const n = Math.max(0, Math.min(65535, Number(e.target.value) || 0))
              setConfig({ ...config, server_addr: `:${n}` })
            }}
            placeholder="0 = auto"
            className="w-28 rounded px-2 py-1 text-sm font-mono"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              outline: 'none',
            }}
          />
          <button
            onClick={onTest}
            disabled={portTestState === 'testing'}
            className="flex items-center gap-1.5 rounded px-3 py-1 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: portTestState === 'testing' ? 'not-allowed' : 'pointer',
              opacity: portTestState === 'testing' ? 0.7 : 1,
            }}
          >
            {portTestState === 'testing' ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
            {portTestState === 'testing' ? 'Testing…' : 'Test availability'}
          </button>
          {!isAuto && (
            <button
              onClick={onReset}
              className="flex items-center gap-1.5 rounded px-3 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
            >
              Reset to auto
            </button>
          )}
        </div>
        <p className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>
          Set to <code>0</code> (or click Reset to auto) to let the OS pick any
          free port at startup. Changes take effect the next time the app
          launches.
        </p>
      </div>

      {portTestResult && (
        <p
          className="mt-2 flex items-start gap-1.5 text-xs"
          style={{ color: portTestResult.available ? '#22c55e' : '#f87171' }}
        >
          {portTestResult.available ? (
            <CheckCircle2 size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          ) : (
            <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          )}
          <span>
            {portTestPort === 0 ? (
              <>Auto-assign always succeeds — the OS picks any free port.</>
            ) : portTestResult.available ? (
              portTestResult.in_use_by === 'pq-companion'
                ? <>Port {portTestPort} is currently used by this app — that&rsquo;s expected and fine.</>
                : <>Port {portTestPort} is available.</>
            ) : (
              <>Port {portTestPort} is unavailable{portTestResult.error ? <>: {portTestResult.error}</> : null}. Pick a different port or use auto-assign.</>
            )}
          </span>
        </p>
      )}

      {dirty && (
        <div
          className="mt-3 rounded p-3"
          style={{ backgroundColor: 'color-mix(in srgb, #f59e0b 18%, transparent)', border: '1px solid #f59e0b' }}
        >
          <p className="mb-2 flex items-start gap-1.5 text-xs" style={{ color: '#f59e0b' }}>
            <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
            <span>
              <b>Unsaved port change.</b> Testing alone does not save the change — click
              the button below (or <b>Save Settings</b> at the bottom of the page) to
              persist it, then restart the app for the new port to take effect.
            </span>
          </p>
          <button
            onClick={onSave}
            disabled={saving}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: saving ? 'not-allowed' : 'pointer',
              opacity: saving ? 0.7 : 1,
            }}
          >
            {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
            {saving ? 'Saving…' : 'Save port change'}
          </button>
        </div>
      )}
    </section>
  )
}

const DPS_CLASS_ROWS: { key: keyof DPSClassColors; label: string }[] = [
  { key: 'warrior', label: 'Warrior' },
  { key: 'cleric', label: 'Cleric' },
  { key: 'paladin', label: 'Paladin' },
  { key: 'ranger', label: 'Ranger' },
  { key: 'shadow_knight', label: 'Shadow Knight' },
  { key: 'druid', label: 'Druid' },
  { key: 'monk', label: 'Monk' },
  { key: 'bard', label: 'Bard' },
  { key: 'rogue', label: 'Rogue' },
  { key: 'shaman', label: 'Shaman' },
  { key: 'necromancer', label: 'Necromancer' },
  { key: 'wizard', label: 'Wizard' },
  { key: 'magician', label: 'Magician' },
  { key: 'enchanter', label: 'Enchanter' },
  { key: 'beastlord', label: 'Beastlord' },
  { key: 'unknown', label: 'Unknown / Default' },
]

function normalizeHex(input: string): string {
  let v = input.trim()
  if (!v.startsWith('#')) v = '#' + v
  return v.toUpperCase()
}

function isValidHex(input: string): boolean {
  return /^#([0-9A-Fa-f]{6})$/.test(input.trim())
}

function DPSClassColorsSection({
  value,
  onChange,
}: {
  value: DPSClassColors
  onChange: (next: DPSClassColors) => void
}): React.ReactElement {
  // Local mirror of the typed hex string per class so users can type freely
  // (e.g. while halfway through "#69CC") without the upper-case normaliser
  // jumping in mid-keystroke. We push to the parent config on blur or when
  // the value parses as a valid hex; the color picker writes through
  // immediately because it always produces a valid colour.
  const [drafts, setDrafts] = React.useState<Record<string, string>>(() => {
    const out: Record<string, string> = {}
    for (const r of DPS_CLASS_ROWS) out[r.key] = value[r.key] ?? ''
    return out
  })

  // Re-sync when the parent value changes (e.g. after Reset to defaults or a
  // load from the API). Without this, the local draft state would shadow
  // resets.
  useEffect(() => {
    const next: Record<string, string> = {}
    for (const r of DPS_CLASS_ROWS) next[r.key] = value[r.key] ?? ''
    setDrafts(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])

  function setOne(key: keyof DPSClassColors, hex: string): void {
    onChange({ ...value, [key]: hex })
  }

  function handleTextChange(key: keyof DPSClassColors, raw: string): void {
    setDrafts((d) => ({ ...d, [key]: raw }))
    const norm = normalizeHex(raw)
    if (isValidHex(norm)) setOne(key, norm)
  }

  function handleTextBlur(key: keyof DPSClassColors): void {
    const norm = normalizeHex(drafts[key] ?? '')
    if (isValidHex(norm)) {
      setDrafts((d) => ({ ...d, [key]: norm }))
      setOne(key, norm)
    } else {
      // Revert to the last valid persisted value if the user typed garbage.
      setDrafts((d) => ({ ...d, [key]: value[key] }))
    }
  }

  function resetOne(key: keyof DPSClassColors): void {
    const def = DEFAULT_DPS_CLASS_COLORS[key]
    setDrafts((d) => ({ ...d, [key]: def }))
    setOne(key, def)
  }

  function resetAll(): void {
    setDrafts(() => {
      const out: Record<string, string> = {}
      for (const r of DPS_CLASS_ROWS) out[r.key] = DEFAULT_DPS_CLASS_COLORS[r.key]
      return out
    })
    onChange({ ...DEFAULT_DPS_CLASS_COLORS })
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            DPS Class Colors
          </h2>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Per-class bar colour for the DPS meter and combat history. Pets
            inherit their owner's colour. Combatants whose class can't be
            resolved fall back to the Unknown / Default colour.
          </p>
        </div>
        <button
          type="button"
          onClick={resetAll}
          className="flex shrink-0 items-center gap-1.5 rounded border px-2.5 py-1 text-xs"
          style={{
            borderColor: 'var(--color-border)',
            color: 'var(--color-foreground)',
            backgroundColor: 'var(--color-surface-2)',
            cursor: 'pointer',
          }}
        >
          <RefreshCw size={11} />
          Reset all to defaults
        </button>
      </div>

      <div
        className="grid gap-2"
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}
      >
        {DPS_CLASS_ROWS.map((row) => {
          const draft = drafts[row.key] ?? ''
          const persisted = value[row.key] ?? ''
          const invalid = !isValidHex(normalizeHex(draft))
          const isDefault = persisted.toUpperCase() === DEFAULT_DPS_CLASS_COLORS[row.key].toUpperCase()
          return (
            <div
              key={row.key}
              className="flex items-center gap-2 rounded border p-2"
              style={{
                borderColor: 'var(--color-border)',
                backgroundColor: 'var(--color-surface-2)',
              }}
            >
              <input
                type="color"
                value={isValidHex(persisted) ? persisted : DEFAULT_DPS_CLASS_COLORS[row.key]}
                onChange={(e) => {
                  const hex = normalizeHex(e.target.value)
                  setDrafts((d) => ({ ...d, [row.key]: hex }))
                  setOne(row.key, hex)
                }}
                title={`Pick ${row.label} colour`}
                style={{
                  width: 32,
                  height: 24,
                  padding: 0,
                  border: '1px solid var(--color-border)',
                  borderRadius: 4,
                  cursor: 'pointer',
                  flexShrink: 0,
                  backgroundColor: 'transparent',
                }}
              />
              <div className="flex min-w-0 flex-1 flex-col">
                <span
                  className="truncate text-xs font-medium"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  {row.label}
                </span>
                <input
                  type="text"
                  value={draft}
                  onChange={(e) => handleTextChange(row.key, e.target.value)}
                  onBlur={() => handleTextBlur(row.key)}
                  spellCheck={false}
                  maxLength={7}
                  className="mt-0.5 rounded border px-1.5 py-0.5 text-xs font-mono"
                  style={{
                    borderColor: invalid ? '#f97316' : 'var(--color-border)',
                    backgroundColor: 'var(--color-surface)',
                    color: 'var(--color-foreground)',
                  }}
                />
              </div>
              <button
                type="button"
                onClick={() => resetOne(row.key)}
                disabled={isDefault}
                title={isDefault ? 'Already at default' : 'Reset to default'}
                className="flex shrink-0 items-center justify-center rounded p-1"
                style={{
                  border: '1px solid var(--color-border)',
                  backgroundColor: 'transparent',
                  color: isDefault ? 'var(--color-muted)' : 'var(--color-muted-foreground)',
                  cursor: isDefault ? 'not-allowed' : 'pointer',
                  opacity: isDefault ? 0.4 : 1,
                }}
              >
                <RefreshCw size={11} />
              </button>
            </div>
          )
        })}
      </div>
    </section>
  )
}

// OverlayLockModeCard lets the user pick, per popout overlay, how it behaves
// while locked. The two modes trade off "click reaches the game" against
// "scroll / clear rows from the overlay" — see lib/overlays.ts. Rows are
// generated from OVERLAY_DEFS, so a newly registered overlay shows up here
// automatically with both options.
const LOCK_MODE_OPTIONS: ReadonlyArray<{
  value: LockedMode
  label: string
  hint: string
}> = [
  {
    value: 'interactive',
    label: 'Interactive on hover',
    hint: 'Hover the overlay to scroll and clear individual rows; move away and clicks pass through to the game.',
  },
  {
    value: 'clickthrough',
    label: 'Click-through',
    hint: 'Only the title-bar buttons are clickable; scrolling and clicks everywhere else pass through to the game.',
  },
]

function OverlayLockModeCard({
  modes,
  onChange,
}: {
  modes: Partial<Record<OverlayName, LockedMode>>
  onChange: (next: Partial<Record<OverlayName, LockedMode>>) => void
}): React.ReactElement {
  const setAll = (mode: LockedMode): void => {
    const next: Partial<Record<OverlayName, LockedMode>> = {}
    for (const def of OVERLAY_DEFS) next[def.name] = mode
    onChange(next)
  }

  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        Overlay Lock Behaviour
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        When an overlay is locked it can&rsquo;t be moved or resized. Choose how
        each one reacts to the mouse while locked:
      </p>

      {/* Mode legend */}
      <div className="mb-3 flex flex-col gap-1">
        {LOCK_MODE_OPTIONS.map((opt) => (
          <p key={opt.value} className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {opt.label}:
            </span>{' '}
            {opt.hint}
          </p>
        ))}
      </div>

      {/* Set-all shortcut */}
      <div className="mb-3 flex items-center gap-2">
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Set all to
        </span>
        {LOCK_MODE_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            onClick={() => setAll(opt.value)}
            className="rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: 'pointer',
            }}
          >
            {opt.label}
          </button>
        ))}
      </div>

      {/* Per-overlay rows */}
      <div className="flex flex-col gap-2">
        {OVERLAY_DEFS.map((def) => {
          const mode = resolveLockedMode(modes, def.name)
          return (
            <div
              key={def.name}
              className="flex items-center justify-between rounded p-2"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
            >
              <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                {def.label}
              </span>
              <LockModeToggle
                value={mode}
                onChange={(next) => onChange({ ...modes, [def.name]: next })}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}

// LockModeToggle is a two-segment control choosing between the locked modes.
function LockModeToggle({
  value,
  onChange,
}: {
  value: LockedMode
  onChange: (next: LockedMode) => void
}): React.ReactElement {
  return (
    <div
      className="inline-flex overflow-hidden rounded"
      style={{ border: '1px solid var(--color-border)' }}
    >
      {LOCK_MODE_OPTIONS.map((opt) => {
        const active = value === opt.value
        return (
          <button
            key={opt.value}
            onClick={() => onChange(opt.value)}
            className="px-2 py-1 text-xs"
            style={{
              backgroundColor: active ? 'var(--color-primary)' : 'transparent',
              color: active ? '#fff' : 'var(--color-muted-foreground)',
              cursor: 'pointer',
            }}
          >
            {opt.label}
          </button>
        )
      })}
    </div>
  )
}

// NPCOverlaySectionsCard renders two parallel checklists controlling which
// information sections appear on each NPC overlay surface (the embedded
// dashboard panel and the floating popout window). Name, zone, pet owner,
// raid/rare badges, and the HP bar are always shown — they're the
// minimum-viable readout that justifies the overlay existing.
const NPC_SECTION_ROWS: ReadonlyArray<{
  key: keyof NPCOverlaySections
  label: string
  hint: string
}> = [
  { key: 'identity', label: 'Identity', hint: 'Level, class, race, body type' },
  { key: 'combat', label: 'Combat', hint: 'HP, mana, AC, damage, attacks/round' },
  { key: 'resists', label: 'Resists', hint: 'MR, CR, FR, DR, PR chips' },
  { key: 'attributes', label: 'Attributes', hint: 'STR / STA / DEX / AGI / INT / WIS / CHA' },
  { key: 'special_abilities', label: 'Special Abilities', hint: 'Summon, rampage, immunities, etc.' },
]

function NPCOverlaySectionsCard({
  dashboard,
  popout,
  onChangeDashboard,
  onChangePopout,
}: {
  dashboard: NPCOverlaySections
  popout: NPCOverlaySections
  onChangeDashboard: (next: NPCOverlaySections) => void
  onChangePopout: (next: NPCOverlaySections) => void
}): React.ReactElement {
  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        NPC Overlay Sections
      </h2>
      <p className="mb-4 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Choose which sections appear on each NPC overlay surface. The target name and HP bar are always shown.
      </p>

      <div className="grid gap-4" style={{ gridTemplateColumns: '1fr 1fr' }}>
        <NPCSectionList
          title="Dashboard panel"
          value={dashboard}
          onChange={onChangeDashboard}
        />
        <NPCSectionList
          title="Popout overlay"
          value={popout}
          onChange={onChangePopout}
        />
      </div>
    </section>
  )
}

function NPCSectionList({
  title,
  value,
  onChange,
}: {
  title: string
  value: NPCOverlaySections
  onChange: (next: NPCOverlaySections) => void
}): React.ReactElement {
  return (
    <div
      className="rounded p-3"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <p
        className="mb-2 text-xs font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        {title}
      </p>
      <div className="flex flex-col gap-2">
        {NPC_SECTION_ROWS.map((row) => (
          <label key={row.key} className="flex cursor-pointer items-start gap-2">
            <input
              type="checkbox"
              checked={value[row.key]}
              onChange={(e) => onChange({ ...value, [row.key]: e.target.checked })}
              style={{ marginTop: 3 }}
            />
            <span>
              <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>{row.label}</span>
              <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{row.hint}</span>
            </span>
          </label>
        ))}
      </div>
    </div>
  )
}

// DiagnosticsSection exposes the log folder so a user reporting an issue can
// hand back ~/.pq-companion/logs/ contents (server.log + electron.log, with
// rotated siblings). Lives on the Advanced tab — same place a power user
// would look when "the items page won't load."
function DiagnosticsSection(): React.ReactElement {
  const hasShell = typeof window !== 'undefined' && !!window.electron?.shell
  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        <FileText size={13} />
        Diagnostics
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        If something isn&rsquo;t working, the log folder contains the last few
        sessions of <code>server.log</code> (backend) and <code>electron.log</code>{' '}
        (UI shell). Attach those to a bug report so the cause can be diagnosed.
      </p>
      <button
        type="button"
        onClick={() => {
          if (window.electron?.shell) {
            window.electron.shell.openLogsFolder().catch(() => null)
          }
        }}
        disabled={!hasShell}
        className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
        }}
      >
        <FolderOpen size={13} />
        Open logs folder
      </button>
    </section>
  )
}

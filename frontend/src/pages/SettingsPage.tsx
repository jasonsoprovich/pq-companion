import React, { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Settings, FolderOpen, Save, AlertTriangle, CheckCircle2, Loader2, X, RefreshCw, Trash2, HardDrive, Sparkles } from 'lucide-react'
import { getConfig, updateConfig, getLogStatus, getLogFileInfo, cleanupLog } from '../services/api'
import type { Config } from '../types/config'
import type { LogFileInfo } from '../types/logEvent'
import BackupManagerPage from './BackupManagerPage'


type SaveState = 'idle' | 'saving' | 'saved' | 'error'
type UpdateState = 'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'downloaded' | 'error'
type Tab = 'settings' | 'backups'

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

export default function SettingsPage(): React.ReactElement {
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('settings')
  const [config, setConfig] = useState<Config | null>(null)
  const [originalConfig, setOriginalConfig] = useState<Config | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const [updateState, setUpdateState] = useState<UpdateState>('idle')
  const [updateVersion, setUpdateVersion] = useState<string | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)

  const [logLargeFile, setLogLargeFile] = useState(false)
  const [logFileInfo, setLogFileInfo] = useState<LogFileInfo | null>(null)
  const [logInfoLoading, setLogInfoLoading] = useState(false)
  const [cleanupState, setCleanupState] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const [cleanupResult, setCleanupResult] = useState<string | null>(null)
  const logPollRef = useRef<ReturnType<typeof setInterval> | null>(null)


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
    }
    navigate(-1)
  }

  async function handleSave(): Promise<void> {
    if (!config) return
    setSaveState('saving')
    setSaveError(null)
    try {
      const saved = await updateConfig(config)
      setConfig(saved)
      setOriginalConfig(saved)
      setSaveState('saved')
      setTimeout(() => navigate(-1), 800)
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

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'settings', label: 'Settings', icon: <Settings size={13} /> },
    { id: 'backups', label: 'Backup Manager', icon: <HardDrive size={13} /> },
  ]

  if (loadError && tab === 'settings') {
    return (
      <div className="flex h-full flex-col">
        <TabBar tabs={tabs} active={tab} onChange={setTab} />
        <div className="flex flex-1 flex-col items-center justify-center gap-3">
          <AlertTriangle size={28} style={{ color: '#f97316' }} />
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Failed to load settings: {loadError}
          </p>
        </div>
      </div>
    )
  }

  if (!config && tab === 'settings') {
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

        {/* ── EverQuest Path ─────────────────────────────────────────────── */}
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

        {/* ── Preferences ────────────────────────────────────────────────── */}
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

          <label className="flex cursor-pointer items-center justify-between py-1 mt-2">
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
        </section>

        {/* ── Overlays ───────────────────────────────────────────────────── */}
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

        {/* ── Log Files ──────────────────────────────────────────────────── */}
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
            Back up and purge your EverQuest log file, keeping only the most recent 7 days of entries. Files over 75 MB are flagged for cleanup.
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
                  Backup &amp; Purge
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
                Purge complete. Backup saved to:
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
              Saved
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

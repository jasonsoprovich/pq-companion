import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Settings, FolderOpen, Save, AlertTriangle, CheckCircle2, Loader2, X } from 'lucide-react'
import { getConfig, updateConfig } from '../services/api'
import type { Config } from '../types/config'

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

export default function SettingsPage(): React.ReactElement {
  const navigate = useNavigate()
  const [config, setConfig] = useState<Config | null>(null)
  const [originalConfig, setOriginalConfig] = useState<Config | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)

  useEffect(() => {
    getConfig()
      .then((c) => {
        setConfig(c)
        setOriginalConfig(c)
      })
      .catch((err: Error) => setLoadError(err.message))
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

  if (loadError) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <AlertTriangle size={28} style={{ color: '#f97316' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Failed to load settings: {loadError}
        </p>
      </div>
    )
  }

  if (!config) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <Loader2 size={24} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Loading settings…
        </p>
      </div>
    )
  }

  const hasElectronDialog = Boolean(window.electron?.dialog)

  return (
    <div className="mx-auto max-w-xl p-6">
      {/* Page header */}
      <div className="mb-6 flex items-center gap-3">
        <Settings size={20} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Settings
        </h1>
      </div>

      <div className="flex flex-col gap-6">
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
              placeholder="e.g. C:\EverQuest or /Applications/EverQuest"
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
        </section>

        {/* ── Character ──────────────────────────────────────────────────── */}
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Character
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Optional. Leave blank to auto-select the most recently active log file. Set a name to override auto-selection (useful for testing).
          </p>

          <input
            type="text"
            value={config.character}
            onChange={(e) => setConfig({ ...config, character: e.target.value })}
            placeholder="e.g. Firiona"
            className="w-full rounded px-3 py-2 text-sm"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              outline: 'none',
            }}
          />
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

        {/* ── Overlay Toggles ─────────────────────────────────────────────── */}
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
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Enable or disable individual floating overlay windows.
          </p>

          <label className="flex cursor-pointer items-center justify-between py-1">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                DPS Overlay
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Show the floating damage-per-second meter overlay.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    overlay_dps_enabled: !config.preferences.overlay_dps_enabled,
                  },
                })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.overlay_dps_enabled
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
                  left: config.preferences.overlay_dps_enabled ? 20 : 2,
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
                HPS Overlay
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Show the floating heals-per-second meter overlay.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    overlay_hps_enabled: !config.preferences.overlay_hps_enabled,
                  },
                })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.overlay_hps_enabled
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
                  left: config.preferences.overlay_hps_enabled ? 20 : 2,
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
  )
}

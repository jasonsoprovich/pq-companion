import React, { useState } from 'react'
import {
  Volume2,
  FolderOpen,
  Play,
  Square,
  Trash2,
  CheckCircle2,
  AlertTriangle,
  ExternalLink,
} from 'lucide-react'

import type { Config, Preferences } from '../../types/config'
import { updateConfig, piperSynthesize, clearPiperCache } from '../../services/api'
import { usePiperStatus } from '../../hooks/usePiperStatus'
import { playSoundForTest, stopTestPlayback, isTestPlaying } from '../../services/audio'

// External resources we link to (we host neither Piper nor voice models).
const INSTALL_DOCS = 'https://github.com/OHF-Voice/piper1-gpl'
const STANDALONE_RELEASE = 'https://github.com/rhasspy/piper/releases/tag/2023.11.14-2'
const VOICE_CATALOG = 'https://huggingface.co/rhasspy/piper-voices/tree/main'

const TEST_PHRASE = 'Piper text to speech is working.'

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

// A colored status dot: green = ok, red = problem, gray = unknown/not set.
function Dot({ state }: { state: 'on' | 'off' | 'unknown' }): React.ReactElement {
  const color =
    state === 'on' ? '#22c55e' : state === 'off' ? '#ef4444' : 'var(--color-muted)'
  return (
    <span
      className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
      style={{ backgroundColor: color }}
    />
  )
}

interface PiperTtsSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

/**
 * PiperTtsSettings is the "Local TTS (Piper)" settings card. Piper is a
 * user-installed external program (PQC bundles nothing) — this card enables it,
 * captures the executable + voice-model paths, shows live detection status, and
 * offers Test Voice / Clear Cache. Once enabled + valid, the "🔊 Piper (local)"
 * voice appears in every alert/trigger voice dropdown. See docs/piper-tts-plan.md.
 *
 * Unlike most settings (debounced autosave in the parent), path/toggle changes
 * here persist immediately via updateConfig so the backend re-detects and the
 * status + Test Voice reflect the just-entered paths without a lag.
 */
export default function PiperTtsSettings({
  config,
  setConfig,
}: PiperTtsSettingsProps): React.ReactElement {
  const { status, refresh: refreshStatus } = usePiperStatus()
  const prefs = config.preferences
  const enabled = prefs.piper_enabled ?? false
  const mode = prefs.piper_mode === 'warm' ? 'warm' : 'spawn'

  const [testing, setTesting] = useState(false)
  const [testError, setTestError] = useState<string | null>(null)
  const [cacheMsg, setCacheMsg] = useState<string | null>(null)

  const canBrowse =
    typeof window !== 'undefined' && !!window.electron?.dialog?.selectPiperExe

  // Stage an edit into the parent's draft; it debounce-autosaves (600ms). Used
  // for the path text inputs so typing doesn't fire a PUT — and a re-detect
  // (which spawns a piper --version probe) — on every keystroke.
  function stage(patch: Partial<Preferences>): void {
    setConfig({ ...config, preferences: { ...config.preferences, ...patch } })
  }
  // Persist immediately for discrete actions (toggle, file-picker result) so
  // the backend re-detects and usePiperStatus refetches on config:updated
  // without waiting for the debounce.
  function saveNow(patch: Partial<Preferences>): void {
    const next: Config = { ...config, preferences: { ...config.preferences, ...patch } }
    setConfig(next)
    void updateConfig(next)
  }

  async function browseExe(): Promise<void> {
    const p = await window.electron?.dialog?.selectPiperExe?.()
    if (p) saveNow({ piper_exe_path: p })
  }
  async function browseModel(): Promise<void> {
    const p = await window.electron?.dialog?.selectPiperModel?.()
    if (p) saveNow({ piper_model_path: p })
  }

  async function testVoice(): Promise<void> {
    if (isTestPlaying()) {
      stopTestPlayback()
      setTesting(false)
      return
    }
    setTestError(null)
    setTesting(true)
    try {
      // force: true bypasses the cache so this genuinely exercises the
      // currently selected mode's live synthesis path — the cache key is
      // mode-independent, so a normal (non-forced) call would just replay
      // whatever's already cached and never actually start a warm worker.
      const { path } = await piperSynthesize(TEST_PHRASE, true)
      playSoundForTest(path, 1.0, () => setTesting(false))
      refreshStatus() // update the warm-worker health dot without waiting for a config change
    } catch (e) {
      setTesting(false)
      setTestError((e as Error).message)
    }
  }

  async function clearCache(): Promise<void> {
    setCacheMsg(null)
    try {
      const { removed } = await clearPiperCache()
      setCacheMsg(`Cleared ${removed} cached file${removed === 1 ? '' : 's'}.`)
    } catch (e) {
      setCacheMsg((e as Error).message)
    }
  }

  const PathRow = ({
    label,
    value,
    placeholder,
    onChange,
    onBrowse,
    ok,
    note,
  }: {
    label: string
    value: string
    placeholder: string
    onChange: (v: string) => void
    onBrowse: () => void
    ok: boolean
    note?: string
  }): React.ReactElement => (
    <div>
      <div className="mb-1 flex items-center gap-2">
        <Dot state={value ? (ok ? 'on' : 'off') : 'unknown'} />
        <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
          {label}
        </span>
        {value && note && (
          <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            {note}
          </span>
        )}
      </div>
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          className="min-w-0 flex-1 rounded px-2 py-1 text-xs font-mono outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        />
        {canBrowse && (
          <button
            onClick={onBrowse}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-muted-foreground)',
            }}
          >
            <FolderOpen size={12} /> Browse
          </button>
        )}
      </div>
    </div>
  )

  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-1 flex items-center justify-between">
        <h2
          className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          <Volume2 size={13} /> Local TTS (Piper)
        </h2>
        <label className="flex cursor-pointer items-center gap-2">
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {enabled ? 'Enabled' : 'Disabled'}
          </span>
          <div
            onClick={() => saveNow({ piper_enabled: !enabled })}
            className="relative h-5 w-9 rounded-full transition-colors"
            style={{ backgroundColor: enabled ? 'var(--color-primary)' : 'var(--color-surface-2)' }}
          >
            <span
              className="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-transform"
              style={{ transform: enabled ? 'translateX(18px)' : 'translateX(2px)' }}
            />
          </div>
        </label>
      </div>

      <p className="mb-3 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
        Piper is a free, high-quality neural voice that runs locally — a better
        alternative to the built-in robotic voices. You install it yourself; PQ
        Companion bundles nothing.{' '}
        <a
          href={INSTALL_DOCS}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 underline"
          style={{ color: 'var(--color-primary)' }}
        >
          Install guide <ExternalLink size={10} />
        </a>{' '}
        (the{' '}
        <a
          href={STANDALONE_RELEASE}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 underline"
          style={{ color: 'var(--color-primary)' }}
        >
          standalone exe <ExternalLink size={10} />
        </a>{' '}
        needs no Python), then grab a voice from the{' '}
        <a
          href={VOICE_CATALOG}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 underline"
          style={{ color: 'var(--color-primary)' }}
        >
          voice catalog <ExternalLink size={10} />
        </a>{' '}
        (a <code className="font-mono">.onnx</code> plus its{' '}
        <code className="font-mono">.onnx.json</code>). The Piper engine (GPL) and
        voice models (usually CC-BY) are separately licensed.
      </p>

      {enabled && (
        <div className="flex flex-col gap-3">
          <PathRow
            label="Piper executable"
            value={prefs.piper_exe_path ?? ''}
            placeholder="C:\piper\piper.exe"
            onChange={(v) => stage({ piper_exe_path: v })}
            onBrowse={browseExe}
            ok={status?.exe_found ?? false}
            note={
              status?.exe_found
                ? status.version
                  ? `found — ${status.version}`
                  : 'found'
                : 'not found'
            }
          />
          <PathRow
            label="Voice model (.onnx)"
            value={prefs.piper_model_path ?? ''}
            placeholder="C:\piper\en_US-amy-medium.onnx"
            onChange={(v) => stage({ piper_model_path: v })}
            onBrowse={browseModel}
            ok={(status?.model_found ?? false) && (status?.model_config_found ?? false)}
            note={
              !status?.model_found
                ? 'not found'
                : !status?.model_config_found
                  ? '.onnx.json missing'
                  : status?.voice_name
                    ? status.voice_name
                    : 'found'
            }
          />

          {/* Overall readiness + error hint */}
          {status && (prefs.piper_exe_path || prefs.piper_model_path) && (
            <div
              className="flex items-start gap-2 rounded px-2 py-1.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)' }}
            >
              {status.ready ? (
                <>
                  <CheckCircle2 size={13} style={{ color: '#22c55e' }} />
                  <span style={{ color: 'var(--color-foreground)' }}>
                    Ready — the “🔊 Piper (local)” voice is available in every alert
                    and trigger voice dropdown.
                  </span>
                </>
              ) : (
                <>
                  <AlertTriangle size={13} style={{ color: '#f59e0b' }} />
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    {status.error || 'Finish configuring the executable and voice model.'}
                  </span>
                </>
              )}
            </div>
          )}

          {/* Synthesis mode */}
          <div>
            <div className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Synthesis mode
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => saveNow({ piper_mode: 'spawn' })}
                className="flex-1 rounded px-2.5 py-1.5 text-left text-xs font-medium"
                style={{
                  backgroundColor: mode === 'spawn' ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: mode === 'spawn' ? '#fff' : 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                Spawn per phrase
                <div className="text-[10px] font-normal opacity-80">
                  Simple; a new phrase may lag briefly
                </div>
              </button>
              <button
                onClick={() => saveNow({ piper_mode: 'warm' })}
                className="flex-1 rounded px-2.5 py-1.5 text-left text-xs font-medium"
                style={{
                  backgroundColor: mode === 'warm' ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: mode === 'warm' ? '#fff' : 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                Keep Piper running (warm)
                <div className="text-[10px] font-normal opacity-80">
                  Faster; uses a bit of background memory
                </div>
              </button>
            </div>
          </div>

          {/* Warm worker health — only meaningful in warm mode */}
          {mode === 'warm' && status?.ready && (
            <div
              className="flex items-center gap-2 rounded px-2 py-1.5"
              style={{ backgroundColor: 'var(--color-surface-2)' }}
            >
              <Dot state={status.warm_running ? 'on' : status.warm_error ? 'off' : 'unknown'} />
              <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
                Warm worker
                <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {status.warm_running
                    ? 'running'
                    : status.warm_error || 'starts on first use'}
                </span>
              </span>
            </div>
          )}

          {/* Actions */}
          <div className="flex flex-wrap items-center gap-2">
            <button
              onClick={testVoice}
              disabled={!status?.ready && !testing}
              className="flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: '#fff',
                border: '1px solid var(--color-border)',
              }}
            >
              {testing ? <Square size={11} /> : <Play size={11} />}
              {testing ? 'Stop' : 'Test voice'}
            </button>
            <button
              onClick={clearCache}
              className="flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <Trash2 size={11} /> Clear TTS cache
            </button>
            {status && (status.cache_files ?? 0) > 0 && (
              <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {status.cache_files} file{status.cache_files === 1 ? '' : 's'},{' '}
                {formatBytes(status.cache_bytes ?? 0)}
              </span>
            )}
            {cacheMsg && (
              <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {cacheMsg}
              </span>
            )}
          </div>

          {testError && (
            <div className="flex items-start gap-2 text-xs" style={{ color: 'var(--color-danger)' }}>
              <AlertTriangle size={13} /> {testError}
            </div>
          )}

          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            {mode === 'warm'
              ? 'Warm mode keeps Piper loaded in the background, so even brand-new phrases synthesize almost instantly — only the very first phrase after enabling (or after a restart) pays a one-time model-load cost. Unused cached files are automatically cleared after 30 days.'
              : 'Generated speech is cached, and callouts you save are pre-generated so they fire instantly. The first time a brand-new phrase is spoken it may lag briefly on slow hardware while Piper loads the model. Unused cached files are automatically cleared after 30 days.'}
          </p>
        </div>
      )}
    </section>
  )
}

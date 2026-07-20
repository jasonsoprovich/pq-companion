import React, { useState } from 'react'
import { Volume2, Play, Square, Trash2, CheckCircle2, AlertTriangle, ExternalLink } from 'lucide-react'

import type { Config, Preferences } from '../../types/config'
import { updateConfig, kokoroSynthesize, clearKokoroCache } from '../../services/api'
import { useKokoroStatus } from '../../hooks/useKokoroStatus'
import { playSoundForTest, stopTestPlayback, isTestPlaying } from '../../services/audio'

// The project we talk to — a self-hosted OpenAI-compatible TTS server, not
// something PQ Companion bundles or runs for the user.
const PROJECT_DOCS = 'https://github.com/remsky/Kokoro-FastAPI'

const TEST_PHRASE = 'Kokoro text to speech is working.'

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

interface KokoroTtsSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

/**
 * KokoroTtsSettings is the "Local TTS (Kokoro)" settings card — the sibling
 * of PiperTtsSettings for a second local neural voice. Unlike Piper (a
 * spawned executable), Kokoro here is a self-hosted Kokoro-FastAPI service
 * (typically `docker run`) reached over plain HTTP — so setup is just "enter
 * the service's URL," with the voice catalog fetched live and offered as a
 * dropdown, no file paths to hunt down. Once enabled + reachable, the
 * "🔊 Kokoro (local)" voice appears in every alert and trigger voice
 * dropdown.
 *
 * Unlike most settings (debounced autosave in the parent), the URL/toggle
 * changes here persist immediately via updateConfig so the backend
 * re-detects and the status + voice list reflect the just-entered URL
 * without a lag.
 */
export default function KokoroTtsSettings({
  config,
  setConfig,
}: KokoroTtsSettingsProps): React.ReactElement {
  const { status, refresh: refreshStatus } = useKokoroStatus()
  const prefs = config.preferences
  const enabled = prefs.kokoro_enabled ?? false

  const [testing, setTesting] = useState(false)
  const [testError, setTestError] = useState<string | null>(null)
  const [cacheMsg, setCacheMsg] = useState<string | null>(null)

  // Stage an edit into the parent's draft; it debounce-autosaves (600ms) —
  // used for the URL text input so typing doesn't fire a PUT (and a
  // re-detect, which hits the configured service) on every keystroke.
  function stage(patch: Partial<Preferences>): void {
    setConfig({ ...config, preferences: { ...config.preferences, ...patch } })
  }
  // Persist immediately for discrete actions (toggle, voice pick) so the
  // backend re-detects and useKokoroStatus refetches on config:updated
  // without waiting for the debounce.
  function saveNow(patch: Partial<Preferences>): void {
    const next: Config = { ...config, preferences: { ...config.preferences, ...patch } }
    setConfig(next)
    void updateConfig(next)
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
      // force: true bypasses the cache so this genuinely round-trips to the
      // service right now, instead of just replaying whatever's cached.
      const { path } = await kokoroSynthesize(TEST_PHRASE, true)
      playSoundForTest(path, 1.0, () => setTesting(false))
      refreshStatus()
    } catch (e) {
      setTesting(false)
      setTestError((e as Error).message)
    }
  }

  async function clearCache(): Promise<void> {
    setCacheMsg(null)
    try {
      const { removed } = await clearKokoroCache()
      setCacheMsg(`Cleared ${removed} cached file${removed === 1 ? '' : 's'}.`)
    } catch (e) {
      setCacheMsg((e as Error).message)
    }
  }

  const voices = status?.voices ?? []
  const selectedVoice = prefs.kokoro_voice || voices[0] || ''

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
          <Volume2 size={13} /> Local TTS (Kokoro)
        </h2>
        <label className="flex cursor-pointer items-center gap-2">
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {enabled ? 'Enabled' : 'Disabled'}
          </span>
          <div
            onClick={() => saveNow({ kokoro_enabled: !enabled })}
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
        Kokoro is a second free, high-quality neural voice — an alternative to
        Piper with its own voice catalog. Unlike Piper, it isn't a program
        this app spawns: point it at a running{' '}
        <a
          href={PROJECT_DOCS}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 underline"
          style={{ color: 'var(--color-primary)' }}
        >
          Kokoro-FastAPI <ExternalLink size={10} />
        </a>{' '}
        server (e.g. via Docker) and this app talks to it over HTTP — no
        files to download or install into PQ Companion itself.
      </p>

      {enabled && (
        <div className="flex flex-col gap-3">
          <div>
            <div className="mb-1 flex items-center gap-2">
              <Dot state={prefs.kokoro_base_url ? (status?.reachable ? 'on' : 'off') : 'unknown'} />
              <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Service URL
              </span>
            </div>
            <input
              type="text"
              value={prefs.kokoro_base_url ?? ''}
              placeholder="http://localhost:8880"
              onChange={(e) => stage({ kokoro_base_url: e.target.value })}
              onBlur={() => saveNow({ kokoro_base_url: prefs.kokoro_base_url })}
              className="w-full rounded px-2 py-1 text-xs font-mono outline-none"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            />
          </div>

          {/* Overall readiness + error hint */}
          {status && prefs.kokoro_base_url && (
            <div
              className="flex items-start gap-2 rounded px-2 py-1.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)' }}
            >
              {status.ready ? (
                <>
                  <CheckCircle2 size={13} style={{ color: '#22c55e' }} />
                  <span style={{ color: 'var(--color-foreground)' }}>
                    Connected — {voices.length} voice{voices.length === 1 ? '' : 's'} available. The
                    “🔊 Kokoro (local)” voice is available in every alert and trigger voice dropdown.
                  </span>
                </>
              ) : (
                <>
                  <AlertTriangle size={13} style={{ color: '#f59e0b' }} />
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    {status.error || 'Finish configuring the service URL.'}
                  </span>
                </>
              )}
            </div>
          )}

          {/* Voice */}
          {voices.length > 0 && (
            <div>
              <div className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
                Voice
              </div>
              <select
                value={selectedVoice}
                onChange={(e) => saveNow({ kokoro_voice: e.target.value })}
                className="w-full rounded px-2 py-1.5 text-xs outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                {voices.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
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
            Generated speech is cached (shared with Piper's cache — “Clear TTS
            cache” here clears both), and callouts you save are pre-generated
            so they fire instantly. Unused cached files are automatically
            cleared after 30 days.
          </p>
        </div>
      )}
    </section>
  )
}

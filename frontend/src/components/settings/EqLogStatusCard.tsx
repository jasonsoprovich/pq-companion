import React, { useEffect, useState } from 'react'
import { FileText, RefreshCw, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { getEqDiagnostics, setLogging, setExportOnCamp, type EqDiagnostics } from '../../services/api'

// A colored status dot: green = on, red = off, gray = unknown/not found.
function Dot({ state }: { state: 'on' | 'off' | 'unknown' }): React.ReactElement {
  const color = state === 'on' ? '#22c55e' : state === 'off' ? '#ef4444' : 'var(--color-muted)'
  return <span className="inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: color }} />
}

/**
 * EqLogStatusCard shows whether EverQuest logging and Zeal "output on camp" are
 * enabled (green/red), and lets the user flip them. Writing rewrites EQ's own
 * config files (a backup is taken server-side first), so it warns that EQ must
 * be closed. Used in Settings and the onboarding wizard.
 */
export default function EqLogStatusCard({
  onChange,
}: {
  onChange?: (d: EqDiagnostics) => void
}): React.ReactElement {
  const [diag, setDiag] = useState<EqDiagnostics | null>(null)
  const [busy, setBusy] = useState<'log' | 'camp' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const apply = (d: EqDiagnostics) => { setDiag(d); onChange?.(d) }

  const refresh = () => {
    getEqDiagnostics().then(apply).catch((e: Error) => setError(e.message))
  }
  useEffect(() => { refresh() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  async function toggleLog() {
    if (!diag) return
    setBusy('log'); setError(null)
    try { apply(await setLogging(!diag.log_enabled)) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(null) }
  }
  async function toggleCamp() {
    if (!diag) return
    setBusy('camp'); setError(null)
    try { apply(await setExportOnCamp(!diag.export_on_camp)) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(null) }
  }

  const ToggleBtn = ({ on, busyKey, onClick }: { on: boolean; busyKey: 'log' | 'camp'; onClick: () => void }) => (
    <button
      onClick={onClick}
      disabled={busy !== null}
      className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium disabled:opacity-50"
      style={{
        backgroundColor: on ? 'var(--color-surface-2)' : 'var(--color-primary)',
        color: on ? 'var(--color-muted-foreground)' : '#fff',
        border: '1px solid var(--color-border)',
      }}
    >
      {busy === busyKey && <RefreshCw size={11} className="animate-spin" />}
      {on ? 'Turn off' : 'Turn on'}
    </button>
  )

  return (
    <section className="rounded-lg p-4" style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
      <div className="mb-1 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          <FileText size={13} /> EverQuest logging &amp; Zeal
        </h2>
        <button onClick={refresh} className="rounded p-1" style={{ color: 'var(--color-muted-foreground)' }} title="Re-check">
          <RefreshCw size={13} />
        </button>
      </div>
      <p className="mb-3 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
        PQ Companion needs EverQuest logging on to read your log file, and Zeal's "output on camp" to export
        character data. Toggling these rewrites <code className="font-mono">eqclient.ini</code> /{' '}
        <code className="font-mono">zeal.ini</code> — <strong>close EverQuest first</strong>, since the client
        overwrites these on exit. A backup is saved automatically.
      </p>

      {error && (
        <div className="mb-3 flex items-start gap-2 rounded p-2" style={{ backgroundColor: 'var(--color-surface-2)' }}>
          <AlertTriangle size={13} style={{ color: 'var(--color-danger)' }} />
          <span className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</span>
        </div>
      )}

      {!diag ? (
        <div className="flex items-center gap-2 py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
          <RefreshCw size={13} className="animate-spin" /> Checking…
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {/* EQ logging */}
          <div className="flex items-center gap-2 rounded px-2 py-1.5" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <Dot state={diag.log_enabled ? 'on' : diag.log_found ? 'off' : 'unknown'} />
            <span className="flex-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              EverQuest logging
              <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {diag.log_enabled ? 'on' : diag.log_found ? 'off' : 'not set'}
              </span>
            </span>
            <ToggleBtn on={diag.log_enabled} busyKey="log" onClick={toggleLog} />
          </div>

          {/* Zeal output on camp */}
          <div className="flex items-center gap-2 rounded px-2 py-1.5" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <Dot state={diag.export_on_camp ? 'on' : diag.export_on_camp_found || diag.zeal_installed ? 'off' : 'unknown'} />
            <span className="flex-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Zeal output on camp
              <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {!diag.zeal_installed ? 'Zeal not detected' : diag.export_on_camp ? 'on' : 'off'}
              </span>
            </span>
            <ToggleBtn on={diag.export_on_camp} busyKey="camp" onClick={toggleCamp} />
          </div>

          {/* Zeal version (informational) */}
          {diag.zeal_installed && (
            <div className="flex items-center gap-2 px-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {diag.zeal_version_ok
                ? <CheckCircle2 size={12} style={{ color: '#22c55e' }} />
                : <AlertTriangle size={12} style={{ color: '#ef4444' }} />}
              Zeal {diag.zeal_version || 'version unknown'}
              {!diag.zeal_version_ok && ' — update recommended'}
            </div>
          )}
        </div>
      )}
    </section>
  )
}

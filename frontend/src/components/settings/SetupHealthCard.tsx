/**
 * SetupHealthCard — top of Settings → General. One row per setup check
 * (useSetupHealth) with a coloured dot and a Fix link that jumps to the
 * setting that resolves it.
 */
import React from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, Loader2, RefreshCw } from 'lucide-react'
import { useSetupHealth, type HealthLevel } from '../../hooks/useSetupHealth'

const DOT: Record<HealthLevel, string> = {
  ok: '#22c55e',
  warn: '#f59e0b',
  error: '#ef4444',
  off: 'var(--color-border)',
}

export default function SetupHealthCard(): React.ReactElement {
  const { checks, loading, refresh } = useSetupHealth()
  const navigate = useNavigate()
  const problems = checks.filter((c) => c.level === 'error').length
  const warnings = checks.filter((c) => c.level === 'warn').length

  return (
    <section
      data-setting="setup-health"
      data-tour="settings-health"
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-center gap-2">
        <Activity size={14} style={{ color: 'var(--color-primary)' }} />
        <h2
          className="text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          Setup health
        </h2>
        <span className="ml-auto text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {loading
            ? ''
            : problems > 0
              ? `${problems} problem${problems === 1 ? '' : 's'}`
              : warnings > 0
                ? `${warnings} to check`
                : 'All good'}
        </span>
        <button
          onClick={refresh}
          className="rounded p-1"
          style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
          title="Check again"
        >
          <RefreshCw size={12} />
        </button>
      </div>
      {loading && checks.length === 0 ? (
        <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <Loader2 size={12} className="animate-spin" /> Checking…
        </div>
      ) : (
        <ul className="space-y-1.5">
          {checks.map((c) => (
            <li key={c.id} className="flex items-start gap-2 text-xs">
              <span
                className="mt-1 inline-block h-2 w-2 shrink-0 rounded-full"
                style={{ backgroundColor: DOT[c.level] }}
              />
              <span className="w-28 shrink-0 font-medium" style={{ color: 'var(--color-foreground)' }}>
                {c.label}
              </span>
              <span
                className="min-w-0 flex-1 break-words"
                style={{ color: c.level === 'off' ? 'var(--color-muted)' : 'var(--color-muted-foreground)' }}
              >
                {c.detail}
              </span>
              {c.fix && (
                <button
                  onClick={() => navigate(c.fix!.to)}
                  className="shrink-0 rounded px-2 py-0.5 text-[11px] font-medium"
                  style={{
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-primary)',
                    cursor: 'pointer',
                  }}
                >
                  {c.fix.label}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

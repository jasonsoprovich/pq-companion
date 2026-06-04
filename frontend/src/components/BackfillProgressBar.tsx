import React from 'react'
import { RefreshCw, CheckCircle2, X } from 'lucide-react'
import { useBackfill } from '../contexts/BackfillContext'

function fmtDuration(sec: number): string {
  if (!isFinite(sec) || sec < 0) return '—'
  const s = Math.round(sec)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return rem ? `${m}m ${rem}s` : `${m}m`
}

// BackfillProgressBar is a persistent bottom bar (same slot/style as the app
// update bar) showing live Log Backfill progress while it runs in the
// background, then a brief dismissible completion summary. Rendered at the app
// root so it's visible from any page while the backfill keeps going.
export default function BackfillProgressBar(): React.ReactElement | null {
  const { running, runChars, prog, elapsed, results, dismissResults } = useBackfill()

  if (running) {
    const idx = prog ? runChars.indexOf(prog.character) : -1
    const fraction = prog && prog.total > 0 ? Math.min(1, prog.done / prog.total) : 0
    const pct = Math.round(fraction * 100)
    const eta = fraction > 0.03 ? (elapsed * (1 - fraction)) / fraction : null

    return (
      <div
        className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}
      >
        <RefreshCw size={13} className="animate-spin" style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
        <span style={{ color: 'var(--color-muted)' }}>
          Backfilling <span style={{ color: 'var(--color-foreground)' }}>{prog?.character ?? '…'}</span>
          {runChars.length > 1 && idx >= 0 && (
            <span> ({idx + 1}/{runChars.length})</span>
          )}
        </span>
        <div className="flex-1 mx-2 rounded-full overflow-hidden" style={{ height: 4, backgroundColor: 'var(--color-border)' }}>
          <div className="h-full rounded-full transition-all" style={{ width: `${pct}%`, backgroundColor: 'var(--color-primary)' }} />
        </div>
        <span className="tabular-nums" style={{ color: 'var(--color-muted)', minWidth: '2.5rem', textAlign: 'right' }}>
          {pct}%
        </span>
        <span className="tabular-nums" style={{ color: 'var(--color-muted)' }}>
          {fmtDuration(elapsed)}{eta !== null && <> · ~{fmtDuration(eta)} left</>}
        </span>
      </div>
    )
  }

  // Finished: show a one-line summary until dismissed.
  if (results && results.length > 0) {
    const added = results.reduce(
      (sum, r) => sum + Object.values(r.results).reduce((a, b) => a + b, 0),
      0,
    )
    const failed = results.filter((r) => r.error).length
    return (
      <div
        className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}
      >
        <CheckCircle2 size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
        <span style={{ color: 'var(--color-muted)' }}>
          Backfill complete — <span style={{ color: 'var(--color-foreground)' }}>{added}</span> new
          {added === 1 ? ' entry' : ' entries'} across {results.length} character
          {results.length === 1 ? '' : 's'}
          {failed > 0 && <span style={{ color: 'var(--color-danger)' }}> · {failed} failed</span>}
          {'. See Settings → Logs for details.'}
        </span>
        <button onClick={dismissResults} className="ml-auto" aria-label="Dismiss" style={{ color: 'var(--color-muted)' }}>
          <X size={12} />
        </button>
      </div>
    )
  }

  return null
}

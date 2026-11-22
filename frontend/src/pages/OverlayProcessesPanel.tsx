/**
 * OverlayProcessesPanel — Developer tab: memory and CPU for every app
 * process, matched to the window it renders. Each overlay window is its own
 * renderer process, so this shows what an open, hidden (by the hotkey) or closed
 * overlay costs. Polls main every 2s; CPU is the average since the last poll.
 * Memory is the working set (RAM in use); Windows also reports private bytes
 * (memory only that process holds), the better measure of what closing a
 * window would free. The Go backend runs outside Electron and isn't listed.
 */
import React, { useEffect, useState } from 'react'
import { Activity } from 'lucide-react'
import type { ProcessMetricRow } from '../types/electron'
import type { TimerGroup } from '../types/trigger'
import { overlayWindowLabel } from '../lib/overlayCatalog'
import { listTimerGroups } from '../services/api'

const POLL_MS = 2000

const ROLE_ORDER: Record<ProcessMetricRow['role'], number> = {
  main: 0, overlay: 1, streamkit: 2, trigger: 3, host: 4, helper: 5,
}

function rowLabel(r: ProcessMetricRow, groups: TimerGroup[]): string {
  switch (r.role) {
    case 'main': return 'Main window'
    case 'trigger': return 'Trigger alerts (always on)'
    case 'streamkit': return 'Discord Voice (StreamKit page)'
    case 'host': return 'Shared overlay process (idle)'
    case 'overlay': {
      const hidden = new Set(r.hiddenKeys ?? [])
      const names = r.keys.map((k) => overlayWindowLabel(k, groups) + (hidden.has(k) ? ' (hidden)' : ''))
      if (r.trigger) names.push('Trigger alerts')
      const shared = r.keys.length > 1 || r.trigger || r.host
      return shared ? `Shared: ${names.join(', ')}` : names.join(', ')
    }
    default: return r.type
  }
}

function mb(v: number | null): string {
  return v === null ? '—' : `${v.toFixed(1)} MB`
}

export default function OverlayProcessesPanel(): React.ReactElement {
  const supported = typeof window.electron?.overlay?.processMetrics === 'function'
  const [rows, setRows] = useState<ProcessMetricRow[]>([])
  const [groups, setGroups] = useState<TimerGroup[]>([])

  useEffect(() => {
    listTimerGroups().then(setGroups).catch(() => {})
  }, [])

  useEffect(() => {
    if (!supported) return
    let cancelled = false
    const poll = (): void => {
      window.electron.overlay.processMetrics()
        .then((r) => { if (!cancelled) setRows(r) })
        .catch(() => {})
    }
    poll()
    const id = setInterval(poll, POLL_MS)
    return () => { cancelled = true; clearInterval(id) }
  }, [supported])

  if (!supported) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Process metrics are only available in the desktop app.
      </p>
    )
  }

  const sorted = [...rows].sort((a, b) => ROLE_ORDER[a.role] - ROLE_ORDER[b.role] || b.memoryMB - a.memoryMB)
  const hasPrivate = rows.some((r) => r.privateMB !== null)
  const sum = (pick: (r: ProcessMetricRow) => boolean, f: (r: ProcessMetricRow) => number): number =>
    rows.filter(pick).reduce((t, r) => t + f(r), 0)
  const isOverlay = (r: ProcessMetricRow): boolean => r.role === 'overlay' || r.role === 'streamkit'
  const totals = [
    { label: 'Open overlays', pick: (r: ProcessMetricRow) => isOverlay(r) && r.state !== 'hidden' },
    { label: 'Hidden overlays', pick: (r: ProcessMetricRow) => r.role === 'overlay' && r.state === 'hidden' },
    { label: 'All processes', pick: () => true },
  ]

  const cell = 'px-2 py-1 text-right tabular-nums'
  const head = 'px-2 py-1 text-[10px] font-semibold uppercase tracking-wide'

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-1 flex items-center gap-2">
        <Activity size={14} style={{ color: 'var(--color-primary)' }} />
        <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Overlay processes
        </h2>
      </div>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Overlay windows share one process unless Overlay process mode is set to
        Separate (Live Map always runs on its own). Memory is RAM in use
        {hasPrivate ? '; private is memory only that process holds' : ''}. CPU
        is the average over the last {POLL_MS / 1000} seconds. Hidden overlays
        are ones put away with the show/hide-all hotkey; closed ones don&rsquo;t appear.
      </p>

      <div className="mb-3 flex flex-wrap gap-2">
        {totals.map((t) => (
          <div
            key={t.label}
            className="rounded px-3 py-1.5 text-xs"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
          >
            <span style={{ color: 'var(--color-muted-foreground)' }}>{t.label}: </span>
            <span className="tabular-nums" style={{ color: 'var(--color-foreground)' }}>
              {rows.filter(t.pick).length} · {mb(sum(t.pick, (r) => r.memoryMB))}
              {hasPrivate && ` (${mb(sum(t.pick, (r) => r.privateMB ?? 0))} private)`}
              {' · '}{sum(t.pick, (r) => r.cpu).toFixed(1)}% CPU
            </span>
          </div>
        ))}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-xs" style={{ color: 'var(--color-foreground)' }}>
          <thead style={{ color: 'var(--color-muted)' }}>
            <tr className="border-b" style={{ borderColor: 'var(--color-border)' }}>
              <th className={`${head} text-left`}>Process</th>
              <th className={`${head} text-left`}>State</th>
              <th className={`${head} text-right`}>Memory</th>
              {hasPrivate && <th className={`${head} text-right`}>Private</th>}
              <th className={`${head} text-right`}>CPU</th>
              <th className={`${head} text-right`}>PID</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((r) => (
              <tr key={r.pid} className="border-b" style={{ borderColor: 'var(--color-border)' }}>
                <td className="px-2 py-1">
                  {rowLabel(r, groups)}
                  {r.role !== 'helper' && (
                    <span className="ml-1.5 text-[10px]" style={{ color: 'var(--color-muted)' }}>{r.type}</span>
                  )}
                </td>
                <td
                  className="px-2 py-1"
                  style={{ color: r.state === 'hidden' ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }}
                >
                  {r.state === 'hidden' ? 'Hidden' : r.state === 'open' ? 'Open' : ''}
                </td>
                <td className={cell}>{mb(r.memoryMB)}</td>
                {hasPrivate && <td className={cell}>{mb(r.privateMB)}</td>}
                <td className={cell}>{r.cpu.toFixed(1)}%</td>
                <td className={cell} style={{ color: 'var(--color-muted)' }}>{r.pid}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

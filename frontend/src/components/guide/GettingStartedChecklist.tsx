/**
 * GettingStartedChecklist — Help → Quick Start. Six first steps with an
 * action button each; finished items tick off from durable state, so the
 * list survives restarts. "Hide" retires the sidebar pill too.
 */
import React from 'react'
import { useNavigate } from 'react-router-dom'
import { CheckCircle2, Circle, Rocket } from 'lucide-react'
import { useGettingStarted, type ChecklistId } from '../../hooks/useGettingStarted'

export default function GettingStartedChecklist(): React.ReactElement | null {
  const gs = useGettingStarted()
  const navigate = useNavigate()
  if (!gs.loaded) return null

  if (gs.dismissed) {
    return (
      <p className="mb-4 text-xs" style={{ color: 'var(--color-muted)' }}>
        Getting started: {gs.doneCount} of {gs.total} done.{' '}
        <button
          onClick={() => gs.setDismissed(false)}
          className="underline"
          style={{ color: 'var(--color-primary)', cursor: 'pointer' }}
        >
          Show checklist
        </button>
      </p>
    )
  }

  const actions: Record<ChecklistId, { label: string; run: () => void }[]> = {
    eq: [{ label: 'Setup wizard', run: () => window.dispatchEvent(new Event('pq:open-onboarding')) }],
    log: [{ label: 'Check status', run: () => navigate('/settings?tab=general&focus=setup-health') }],
    zeal: [
      { label: 'Install', run: () => navigate('/settings?tab=general&focus=zeal') },
      { label: 'Skip', run: gs.skipZeal },
    ],
    overlays: [{ label: 'Overlay setup', run: () => window.dispatchEvent(new Event('pq:open-overlay-setup')) }],
    triggers: [{ label: 'Open Triggers', run: () => navigate('/triggers') }],
    search: [{ label: 'Open search', run: () => window.dispatchEvent(new Event('pq:open-search')) }],
  }

  return (
    <section
      data-tour="getting-started"
      className="mb-6 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-center gap-2">
        <Rocket size={14} style={{ color: 'var(--color-primary)' }} />
        <h2 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Getting started
        </h2>
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {gs.doneCount} of {gs.total}
        </span>
        <button
          onClick={() => gs.setDismissed(true)}
          className="ml-auto text-xs"
          style={{ color: 'var(--color-muted)', cursor: 'pointer' }}
          title="Hide this checklist and the sidebar reminder"
        >
          Hide
        </button>
      </div>
      <div className="mb-3 h-1 overflow-hidden rounded-full" style={{ backgroundColor: 'var(--color-surface-2)' }}>
        <div
          className="h-full rounded-full transition-all"
          style={{ width: `${(gs.doneCount / Math.max(1, gs.total)) * 100}%`, backgroundColor: 'var(--color-primary)' }}
        />
      </div>
      <ul className="space-y-2">
        {gs.items.map((item) => (
          <li key={item.id} className="flex items-start gap-2.5">
            {item.done ? (
              <CheckCircle2 size={16} className="mt-0.5 shrink-0" style={{ color: '#22c55e' }} />
            ) : (
              <Circle size={16} className="mt-0.5 shrink-0" style={{ color: 'var(--color-muted)' }} />
            )}
            <div className="min-w-0 flex-1">
              <p
                className="text-sm"
                style={{
                  color: item.done ? 'var(--color-muted-foreground)' : 'var(--color-foreground)',
                  textDecoration: item.done ? 'line-through' : 'none',
                }}
              >
                {item.label}
              </p>
              {!item.done && (
                <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  {item.detail}
                </p>
              )}
            </div>
            {!item.done &&
              actions[item.id].map((a) => (
                <button
                  key={a.label}
                  onClick={a.run}
                  className="shrink-0 rounded px-2 py-0.5 text-[11px] font-medium"
                  style={{ border: '1px solid var(--color-border)', color: 'var(--color-primary)', cursor: 'pointer' }}
                >
                  {a.label}
                </button>
              ))}
          </li>
        ))}
      </ul>
    </section>
  )
}

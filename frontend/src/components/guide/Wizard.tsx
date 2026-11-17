/**
 * Wizard — the shared modal frame for guided setup flows: a header with an
 * icon, title and optional close button, a numbered step indicator, a
 * scrolling body, and a Back / primary-action footer.
 *
 * It is a controlled, presentational shell: callers own which step is
 * showing, which steps apply, and what Next/Finish do. That keeps each
 * flow's logic (validation, saving, skipping steps that don't apply) in
 * the flow itself.
 */
import React from 'react'
import { ArrowLeft, ArrowRight, CheckCircle2, Loader2, X } from 'lucide-react'

export interface WizardStep {
  id: string
  label: string
}

export interface WizardPrimary {
  label: string
  onClick: () => void
  disabled?: boolean
  busy?: boolean
  // 'next' shows a primary-coloured button with a right arrow; 'finish'
  // shows a green button with a check mark.
  variant?: 'next' | 'finish'
}

export interface WizardProps {
  icon: React.ReactNode
  title: string
  steps: WizardStep[]
  currentStep: string | null
  // Show each step's label beside its number in the indicator.
  showStepLabels?: boolean
  onClose?: () => void
  closeTitle?: string
  closeDisabled?: boolean
  wide?: boolean
  // Replaces the default padded, scrolling body wrapper (e.g. for a step
  // that lays out its own full-bleed content).
  bodyClassName?: string
  onBack?: () => void
  backDisabled?: boolean
  backHidden?: boolean
  footerCenter?: React.ReactNode
  primary: WizardPrimary
  children: React.ReactNode
}

export default function Wizard({
  icon,
  title,
  steps,
  currentStep,
  showStepLabels = false,
  onClose,
  closeTitle = 'Close',
  closeDisabled = false,
  wide = false,
  bodyClassName = 'flex-1 overflow-y-auto px-6 py-6',
  onBack,
  backDisabled = false,
  backHidden = false,
  footerCenter,
  primary,
  children,
}: WizardProps): React.ReactElement {
  const stepIndex = Math.max(0, steps.findIndex((s) => s.id === currentStep))
  const backOff = backDisabled || stepIndex === 0 || !onBack
  const finish = primary.variant === 'finish'
  const primaryOff = !!primary.disabled || !!primary.busy

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(2px)' }}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className={`flex w-full flex-col rounded-lg shadow-2xl ${wide ? 'max-w-5xl' : 'max-w-2xl'}`}
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          maxHeight: '90vh',
        }}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between border-b px-6 py-4"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <div className="flex items-center gap-3">
            <span style={{ color: 'var(--color-primary)' }}>{icon}</span>
            <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {title}
            </h2>
          </div>
          {onClose && (
            <button
              onClick={onClose}
              disabled={closeDisabled}
              className="rounded p-1"
              style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
              title={closeTitle}
            >
              <X size={16} />
            </button>
          )}
        </div>

        {/* Step indicator */}
        {steps.length > 1 && (
          <div
            className="flex items-center gap-2 border-b px-6 py-3"
            style={{ borderColor: 'var(--color-border)' }}
          >
            {steps.map((s, i) => (
              <React.Fragment key={s.id}>
                <div className="flex items-center gap-1.5">
                  <div
                    className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                    style={{
                      backgroundColor:
                        i <= stepIndex ? 'var(--color-primary)' : 'var(--color-surface-2)',
                      color: i <= stepIndex ? '#fff' : 'var(--color-muted-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  >
                    {i + 1}
                  </div>
                  {showStepLabels && (
                    <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      {s.label}
                    </span>
                  )}
                </div>
                {i < steps.length - 1 && (
                  <div
                    className="h-px flex-1"
                    style={{
                      backgroundColor:
                        i < stepIndex ? 'var(--color-primary)' : 'var(--color-border)',
                    }}
                  />
                )}
              </React.Fragment>
            ))}
          </div>
        )}

        {/* Body */}
        <div className={bodyClassName}>{children}</div>

        {/* Footer */}
        <div
          className="flex items-center justify-between border-t px-6 py-4"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <button
            onClick={onBack}
            disabled={backOff}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: backOff ? 'not-allowed' : 'pointer',
              opacity: backOff ? 0.4 : 1,
              visibility: backHidden ? 'hidden' : 'visible',
            }}
          >
            <ArrowLeft size={14} />
            Back
          </button>
          {footerCenter ?? <span />}
          <button
            onClick={primary.onClick}
            disabled={primaryOff}
            className="flex items-center gap-1.5 rounded px-4 py-1.5 text-sm font-semibold"
            style={{
              backgroundColor: finish ? '#22c55e' : 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: primaryOff ? 'not-allowed' : 'pointer',
              opacity: primary.busy ? 0.7 : primary.disabled ? 0.5 : 1,
            }}
          >
            {primary.busy ? (
              <Loader2 size={14} className="animate-spin" />
            ) : finish ? (
              <CheckCircle2 size={14} />
            ) : null}
            {primary.label}
            {!finish && !primary.busy && <ArrowRight size={14} />}
          </button>
        </div>
      </div>
    </div>
  )
}

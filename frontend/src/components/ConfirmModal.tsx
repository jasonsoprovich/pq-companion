import React from 'react'
import { AlertTriangle } from 'lucide-react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'

// ConfirmModal is the shared "are you sure?" prompt used wherever the user
// is about to mutate persisted state. Styled to match CharacterSpellsetsPage's
// inline ConfirmModal so the visual language stays consistent.

export interface ConfirmModalProps {
  title: string
  message: React.ReactNode
  confirmLabel?: string
  // Tone affects the confirm button color. 'danger' for destructive ops
  // (remove / delete), 'primary' (default) for routine actions.
  tone?: 'primary' | 'danger'
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmModal({
  title,
  message,
  confirmLabel = 'Confirm',
  tone = 'primary',
  onConfirm,
  onCancel,
}: ConfirmModalProps): React.ReactElement {
  useEscapeToClose(onCancel)
  const confirmBg = tone === 'danger' ? 'var(--color-danger, #ef4444)' : 'var(--color-primary)'
  return (
    <div
      onClick={onCancel}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 440,
        }}
      >
        <div className="flex items-center gap-2 px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
          <AlertTriangle size={16} style={{ color: 'var(--color-warning, #f59e0b)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {title}
          </span>
        </div>
        <div className="px-4 py-3 text-sm" style={{ color: 'var(--color-foreground)' }}>
          {message}
        </div>
        <div className="flex justify-end gap-2 px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-sm rounded"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-sm font-medium rounded"
            style={{
              backgroundColor: confirmBg,
              color: 'var(--color-primary-foreground, #fff)',
              border: 'none',
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

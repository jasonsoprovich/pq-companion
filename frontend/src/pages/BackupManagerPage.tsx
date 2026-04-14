import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Archive,
  RefreshCw,
  AlertCircle,
  Plus,
  Trash2,
  RotateCcw,
  CheckCircle2,
  X,
  HardDrive,
} from 'lucide-react'
import { listBackups, createBackup, deleteBackup, restoreBackup } from '../services/api'
import type { Backup } from '../types/backup'

// ── Helpers ────────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }) + ' at ' + d.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  })
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

// ── Create form ────────────────────────────────────────────────────────────────

interface CreateFormProps {
  onCreated: (b: Backup) => void
  onCancel: () => void
}

function CreateForm({ onCreated, onCancel }: CreateFormProps): React.ReactElement {
  const [name, setName] = useState('')
  const [notes, setNotes] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => { nameRef.current?.focus() }, [])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitting(true)
    setError(null)
    createBackup(name.trim(), notes.trim())
      .then((b) => onCreated(b))
      .catch((err: Error) => {
        setError(err.message)
        setSubmitting(false)
      })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-lg p-4 space-y-3"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-primary)',
      }}
    >
      <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
        New Backup
      </p>

      <div className="space-y-2">
        <input
          ref={nameRef}
          type="text"
          placeholder="Backup name (required)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded px-3 py-1.5 text-sm outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          disabled={submitting}
        />
        <input
          type="text"
          placeholder="Notes (optional)"
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          className="w-full rounded px-3 py-1.5 text-sm outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          disabled={submitting}
        />
      </div>

      {error && (
        <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}

      <div className="flex items-center gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          disabled={submitting}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !name.trim()}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: name.trim() && !submitting ? 'var(--color-primary)' : 'var(--color-surface-2)',
            color: name.trim() && !submitting ? 'var(--color-background)' : 'var(--color-muted)',
            border: '1px solid transparent',
            cursor: name.trim() && !submitting ? 'pointer' : 'not-allowed',
          }}
        >
          {submitting ? (
            <RefreshCw size={11} className="animate-spin" />
          ) : (
            <Archive size={11} />
          )}
          {submitting ? 'Creating…' : 'Create Backup'}
        </button>
      </div>
    </form>
  )
}

// ── Backup card ────────────────────────────────────────────────────────────────

type CardAction = 'none' | 'confirm-delete' | 'confirm-restore' | 'restoring' | 'deleting' | 'restored'

interface BackupCardProps {
  backup: Backup
  onDeleted: (id: string) => void
  onRestored: (id: string) => void
}

function BackupCard({ backup, onDeleted, onRestored }: BackupCardProps): React.ReactElement {
  const [action, setAction] = useState<CardAction>('none')
  const [error, setError] = useState<string | null>(null)

  const handleDelete = () => {
    setAction('deleting')
    setError(null)
    deleteBackup(backup.id)
      .then(() => onDeleted(backup.id))
      .catch((err: Error) => {
        setError(err.message)
        setAction('none')
      })
  }

  const handleRestore = () => {
    setAction('restoring')
    setError(null)
    restoreBackup(backup.id)
      .then(() => {
        setAction('restored')
        onRestored(backup.id)
        // Auto-reset after 3 s
        setTimeout(() => setAction('none'), 3000)
      })
      .catch((err: Error) => {
        setError(err.message)
        setAction('none')
      })
  }

  return (
    <div
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${action === 'restored' ? 'var(--color-success)' : 'var(--color-border)'}`,
      }}
    >
      {/* Top row: name + actions */}
      <div className="flex items-start gap-3">
        <Archive
          size={16}
          className="shrink-0 mt-0.5"
          style={{ color: 'var(--color-primary)' }}
        />
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate" style={{ color: 'var(--color-foreground)' }}>
            {backup.name}
          </p>
          {backup.notes && (
            <p className="text-xs mt-0.5 truncate" style={{ color: 'var(--color-muted-foreground)' }}>
              {backup.notes}
            </p>
          )}
        </div>

        {/* Action buttons — right side */}
        {action === 'none' && (
          <div className="flex items-center gap-1.5 shrink-0">
            <button
              title="Restore this backup"
              onClick={() => setAction('confirm-restore')}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <RotateCcw size={11} />
              Restore
            </button>
            <button
              title="Delete this backup"
              onClick={() => setAction('confirm-delete')}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <Trash2 size={11} />
            </button>
          </div>
        )}

        {action === 'restored' && (
          <div className="flex items-center gap-1 text-xs shrink-0" style={{ color: 'var(--color-success)' }}>
            <CheckCircle2 size={13} />
            Restored
          </div>
        )}

        {(action === 'restoring' || action === 'deleting') && (
          <RefreshCw
            size={14}
            className="animate-spin shrink-0"
            style={{ color: 'var(--color-muted)' }}
          />
        )}
      </div>

      {/* Meta row */}
      <div className="flex items-center gap-3 mt-2 ml-7">
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {formatDate(backup.created_at)}
        </span>
        <span
          className="text-[10px] px-1.5 py-0.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          {backup.file_count} file{backup.file_count !== 1 ? 's' : ''}
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {formatBytes(backup.size_bytes)}
        </span>
      </div>

      {/* Inline confirmation — delete */}
      {action === 'confirm-delete' && (
        <div
          className="mt-3 ml-7 flex items-center gap-2 rounded px-3 py-2"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <AlertCircle size={13} style={{ color: 'var(--color-danger)' }} />
          <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
            Delete this backup permanently?
          </span>
          <button
            onClick={handleDelete}
            className="text-xs px-2 py-0.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-danger)',
              color: '#fff',
              border: '1px solid transparent',
            }}
          >
            Delete
          </button>
          <button
            onClick={() => setAction('none')}
            className="text-xs px-1.5 py-0.5 rounded"
            style={{
              color: 'var(--color-muted-foreground)',
            }}
          >
            <X size={13} />
          </button>
        </div>
      )}

      {/* Inline confirmation — restore */}
      {action === 'confirm-restore' && (
        <div
          className="mt-3 ml-7 flex items-center gap-2 rounded px-3 py-2"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <RotateCcw size={13} style={{ color: 'var(--color-primary)' }} />
          <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
            Overwrite current EQ config files with this backup?
          </span>
          <button
            onClick={handleRestore}
            className="text-xs px-2 py-0.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              border: '1px solid transparent',
            }}
          >
            Restore
          </button>
          <button
            onClick={() => setAction('none')}
            className="text-xs px-1.5 py-0.5 rounded"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            <X size={13} />
          </button>
        </div>
      )}

      {/* Error */}
      {error && (
        <p className="mt-2 ml-7 text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function BackupManagerPage(): React.ReactElement {
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const navigate = useNavigate()

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listBackups()
      .then((resp) => setBackups(resp.backups))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreated = (b: Backup) => {
    setBackups((prev) => [b, ...prev])
    setShowCreate(false)
  }

  const handleDeleted = (id: string) => {
    setBackups((prev) => prev.filter((b) => b.id !== id))
  }

  // ── Loading ────────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
        <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
          {error}
        </p>
        <button
          onClick={load}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  // ── Main render ────────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-3 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <HardDrive size={16} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          Config Backup Manager
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={load}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
          <button
            onClick={() => setShowCreate((v) => !v)}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: showCreate ? 'var(--color-surface-2)' : 'var(--color-primary)',
              color: showCreate ? 'var(--color-muted-foreground)' : 'var(--color-background)',
              border: `1px solid ${showCreate ? 'var(--color-border)' : 'transparent'}`,
            }}
          >
            <Plus size={11} />
            New Backup
          </button>
        </div>
      </div>

      {/* Info banner */}
      <div
        className="flex items-start gap-2 border-b px-4 py-2 shrink-0"
        style={{
          borderColor: 'var(--color-border)',
          backgroundColor: 'var(--color-surface)',
        }}
      >
        <Archive size={12} className="shrink-0 mt-0.5" style={{ color: 'var(--color-muted)' }} />
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Backs up all <code className="font-mono">*.ini</code> files from your EverQuest directory
          (eqclient.ini, per-character UI and hotkey settings). Backups are stored in{' '}
          <code className="font-mono">~/.pq-companion/backups/</code>.
        </p>
      </div>

      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">

        {/* Create form (inline) */}
        {showCreate && (
          <CreateForm
            onCreated={handleCreated}
            onCancel={() => setShowCreate(false)}
          />
        )}

        {/* Empty state */}
        {backups.length === 0 && !showCreate && (
          <div className="flex h-40 flex-col items-center justify-center gap-3">
            <Archive size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No backups yet.
            </p>
            <button
              onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <Plus size={11} />
              Create your first backup
            </button>
            <p className="text-[11px] text-center max-w-xs" style={{ color: 'var(--color-muted)' }}>
              Make sure your EQ path is set in{' '}
              <button
                className="underline"
                style={{ color: 'var(--color-primary)' }}
                onClick={() => navigate('/settings')}
              >
                Settings
              </button>{' '}
              before creating a backup.
            </p>
          </div>
        )}

        {/* Backup list */}
        {backups.map((b) => (
          <BackupCard
            key={b.id}
            backup={b}
            onDeleted={handleDeleted}
            onRestored={() => {/* success shown inline in card */}}
          />
        ))}
      </div>
    </div>
  )
}

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
  Lock,
  LockOpen,
  Clock,
  Settings,
  Zap,
} from 'lucide-react'
import {
  listBackups,
  createBackup,
  deleteBackup,
  restoreBackup,
  lockBackup,
  unlockBackup,
  pruneBackups,
  getConfig,
  updateConfig,
} from '../services/api'
import type { Backup } from '../types/backup'
import type { Config, BackupSettings } from '../types/config'

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

// ── Trigger badge ──────────────────────────────────────────────────────────────

function TriggerBadge({ reason }: { reason: string }): React.ReactElement | null {
  if (reason === 'manual' || !reason) return null
  const label = reason === 'auto' ? 'auto' : 'scheduled'
  const Icon = reason === 'auto' ? Zap : Clock
  return (
    <span
      className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.5 rounded"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        color: 'var(--color-muted-foreground)',
        border: '1px solid var(--color-border)',
      }}
    >
      <Icon size={9} />
      {label}
    </span>
  )
}

// ── Backup card ────────────────────────────────────────────────────────────────

type CardAction = 'none' | 'confirm-delete' | 'confirm-restore' | 'restoring' | 'deleting' | 'restored'

interface BackupCardProps {
  backup: Backup
  onDeleted: (id: string) => void
  onRestored: (id: string) => void
  onLockToggled: (id: string, locked: boolean) => void
}

function BackupCard({ backup, onDeleted, onRestored, onLockToggled }: BackupCardProps): React.ReactElement {
  const [action, setAction] = useState<CardAction>('none')
  const [error, setError] = useState<string | null>(null)
  const [lockBusy, setLockBusy] = useState(false)

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
        setTimeout(() => setAction('none'), 3000)
      })
      .catch((err: Error) => {
        setError(err.message)
        setAction('none')
      })
  }

  const handleLockToggle = () => {
    setLockBusy(true)
    const fn = backup.locked ? unlockBackup : lockBackup
    fn(backup.id)
      .then(() => onLockToggled(backup.id, !backup.locked))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLockBusy(false))
  }

  return (
    <div
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${action === 'restored' ? 'var(--color-success)' : backup.locked ? 'var(--color-primary)' : 'var(--color-border)'}`,
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
              title={backup.locked ? 'Unlock this backup' : 'Lock this backup (protect from auto-cleanup)'}
              onClick={handleLockToggle}
              disabled={lockBusy}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: backup.locked ? 'var(--color-primary)' : 'var(--color-surface-2)',
                color: backup.locked ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                border: `1px solid ${backup.locked ? 'transparent' : 'var(--color-border)'}`,
              }}
            >
              {lockBusy ? <RefreshCw size={11} className="animate-spin" /> : backup.locked ? <Lock size={11} /> : <LockOpen size={11} />}
            </button>
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
      <div className="flex items-center gap-3 mt-2 ml-7 flex-wrap">
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
        <TriggerBadge reason={backup.trigger_reason} />
        {backup.locked && (
          <span className="flex items-center gap-0.5 text-[10px]" style={{ color: 'var(--color-primary)' }}>
            <Lock size={9} /> locked
          </span>
        )}
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

// ── Settings panel ─────────────────────────────────────────────────────────────

interface SettingsPanelProps {
  onClose: () => void
  onPruned: () => void
}

function SettingsPanel({ onClose, onPruned }: SettingsPanelProps): React.ReactElement {
  const [cfg, setCfg] = useState<Config | null>(null)
  const [saving, setSaving] = useState(false)
  const [pruning, setPruning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    getConfig().then(setCfg).catch((err: Error) => setError(err.message))
  }, [])

  const bs = cfg?.backup ?? { auto_backup: false, schedule: 'off' as const, max_backups: 10 }

  const patch = (update: Partial<BackupSettings>) => {
    if (!cfg) return
    setCfg({ ...cfg, backup: { ...bs, ...update } })
  }

  const handleSave = () => {
    if (!cfg) return
    setSaving(true)
    setError(null)
    updateConfig(cfg)
      .then(() => {
        setSaved(true)
        setTimeout(() => setSaved(false), 2000)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const handlePrune = () => {
    const max = bs.max_backups
    if (!max || max <= 0) return
    setPruning(true)
    pruneBackups(max)
      .then(() => { onPruned(); onClose() })
      .catch((err: Error) => setError(err.message))
      .finally(() => setPruning(false))
  }

  if (!cfg) {
    return (
      <div
        className="rounded-lg p-4 space-y-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <RefreshCw size={14} className="animate-spin mx-auto" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  return (
    <div
      className="rounded-lg p-4 space-y-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-primary)' }}
    >
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Backup Settings
        </p>
        <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
          <X size={13} />
        </button>
      </div>

      {/* Auto-backup toggle */}
      <label className="flex items-center justify-between gap-3 cursor-pointer">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
          Auto-backup on file change
        </span>
        <input
          type="checkbox"
          checked={bs.auto_backup}
          onChange={(e) => patch({ auto_backup: e.target.checked })}
          className="w-4 h-4 accent-primary"
        />
      </label>

      {/* Schedule */}
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>Schedule</span>
        <select
          value={bs.schedule}
          onChange={(e) => patch({ schedule: e.target.value as BackupSettings['schedule'] })}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <option value="off">Off</option>
          <option value="hourly">Hourly</option>
          <option value="daily">Daily</option>
        </select>
      </div>

      {/* Max backups */}
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
          Max backups <span style={{ color: 'var(--color-muted)' }}>(0 = unlimited)</span>
        </span>
        <input
          type="number"
          min={0}
          value={bs.max_backups}
          onChange={(e) => patch({ max_backups: Math.max(0, parseInt(e.target.value) || 0) })}
          className="w-16 text-xs rounded px-2 py-1 outline-none text-right"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        />
      </div>

      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}

      <div className="flex items-center gap-2 justify-between pt-1">
        <button
          onClick={handlePrune}
          disabled={pruning || !bs.max_backups}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
            opacity: !bs.max_backups ? 0.5 : 1,
          }}
        >
          {pruning ? <RefreshCw size={11} className="animate-spin" /> : <Trash2 size={11} />}
          Prune now
        </button>
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: saved ? 'var(--color-success)' : 'var(--color-primary)',
            color: 'var(--color-background)',
            border: '1px solid transparent',
          }}
        >
          {saving ? <RefreshCw size={11} className="animate-spin" /> : saved ? <CheckCircle2 size={11} /> : null}
          {saved ? 'Saved' : 'Save'}
        </button>
      </div>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function BackupManagerPage(): React.ReactElement {
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
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

  const handleLockToggled = (id: string, locked: boolean) => {
    setBackups((prev) => prev.map((b) => b.id === id ? { ...b, locked } : b))
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
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <HardDrive size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
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
            onClick={() => { setShowSettings((v) => !v); setShowCreate(false) }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: showSettings ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: showSettings ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: `1px solid ${showSettings ? 'transparent' : 'var(--color-border)'}`,
            }}
          >
            <Settings size={11} />
          </button>
          <button
            onClick={() => { setShowCreate((v) => !v); setShowSettings(false) }}
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
          Backs up all <code className="font-mono">*.ini</code> files from your EverQuest directory.
          Lock a backup to protect it from automatic cleanup.
        </p>
      </div>

      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">

        {/* Settings panel (inline) */}
        {showSettings && (
          <SettingsPanel
            onClose={() => setShowSettings(false)}
            onPruned={load}
          />
        )}

        {/* Create form (inline) */}
        {showCreate && (
          <CreateForm
            onCreated={handleCreated}
            onCancel={() => setShowCreate(false)}
          />
        )}

        {/* Empty state */}
        {backups.length === 0 && !showCreate && !showSettings && (
          <div className="flex h-full flex-col items-center justify-center gap-3">
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
            onLockToggled={handleLockToggled}
          />
        ))}
      </div>
    </div>
  )
}

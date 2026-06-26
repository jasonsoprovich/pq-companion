import React, { useEffect, useMemo, useState } from 'react'
import {
  X,
  Upload,
  RefreshCw,
  AlertTriangle,
  CheckCircle2,
  CheckSquare,
  Square,
} from 'lucide-react'
import {
  previewTriggerImport,
  commitTriggerImport,
} from '../services/api'
import {
  IMPORT_FORMAT_LABELS,
  type ImportPreview,
  type ImportedTrigger,
} from '../types/trigger'

interface ImportTriggersModalProps {
  // The file the user picked. Previewed on mount.
  file: File
  onClose: () => void
  // Called after a successful import so the Triggers list / categories refresh.
  onImported: (category: string, count: number) => void
}

/**
 * Wizard for importing triggers from another app. Detects the source format
 * (PQ Companion / GINA / EQNag / EQLogParser) on the backend, previews the
 * mapped triggers with per-trigger warnings, lets the user pick which to keep
 * and name a destination category, then commits the selection.
 */
export default function ImportTriggersModal({
  file,
  onClose,
  onImported,
}: ImportTriggersModalProps): React.ReactElement {
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [category, setCategory] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [committing, setCommitting] = useState(false)
  const [commitError, setCommitError] = useState<string | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !committing) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, committing])

  // Preview the picked file once on mount.
  useEffect(() => {
    let cancelled = false
    previewTriggerImport(file)
      .then((p) => {
        if (cancelled) return
        setPreview(p)
        setCategory(p.source_name)
        // Default: select every trigger (broken-regex ones import disabled
        // but stay editable, so don't silently drop them).
        setSelected(new Set(p.triggers.map((_, i) => i)))
      })
      .catch((err: Error) => {
        if (!cancelled) setLoadError(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [file])

  const toggle = (i: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })
  }

  const selectAll = () => {
    if (!preview) return
    setSelected(new Set(preview.triggers.map((_, i) => i)))
  }
  const selectNone = () => setSelected(new Set())

  const warningCount = useMemo(() => {
    if (!preview) return 0
    return preview.triggers.reduce((n, t) => n + (t.warnings?.length ?? 0), 0)
  }, [preview])

  const handleImport = () => {
    if (!preview) return
    const cat = category.trim()
    if (!cat) {
      setCommitError('Enter a category name')
      return
    }
    if (selected.size === 0) {
      setCommitError('Select at least one trigger')
      return
    }
    setCommitting(true)
    setCommitError(null)
    const chosen = preview.triggers
      .filter((_, i) => selected.has(i))
      .map((it) => it.trigger)
    commitTriggerImport(cat, chosen)
      .then((r) => onImported(r.category, r.imported))
      .catch((err: Error) => {
        setCommitError(err.message)
        setCommitting(false)
      })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={() => !committing && onClose()}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between px-4 py-3"
          style={{ borderBottom: '1px solid var(--color-border)' }}
        >
          <div className="flex items-center gap-2">
            <Upload size={15} style={{ color: 'var(--color-accent)' }} />
            <span
              className="text-sm font-semibold"
              style={{ color: 'var(--color-foreground)' }}
            >
              Import Triggers
            </span>
            {preview && (
              <span
                className="rounded px-1.5 py-0.5 text-[11px] font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {IMPORT_FORMAT_LABELS[preview.format] ?? preview.format}
              </span>
            )}
          </div>
          <button
            onClick={() => !committing && onClose()}
            className="rounded p-1"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            <X size={15} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-4 py-3">
          {loadError ? (
            <div
              className="flex items-start gap-2 rounded p-3 text-xs"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-danger)',
              }}
            >
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <span>{loadError}</span>
            </div>
          ) : !preview ? (
            <div
              className="flex items-center justify-center gap-2 py-10 text-xs"
              style={{ color: 'var(--color-muted)' }}
            >
              <RefreshCw size={14} className="animate-spin" />
              Reading {file.name}…
            </div>
          ) : (
            <>
              {/* Category name */}
              <label
                className="mb-1 block text-[11px] font-semibold"
                style={{ color: 'var(--color-foreground)' }}
              >
                Category
              </label>
              <input
                type="text"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                placeholder="Imported Triggers"
                className="mb-1 w-full rounded px-2 py-1.5 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              />
              <p
                className="mb-3 text-[11px]"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                Imported triggers are filed under this category. Delete the
                category later to remove them all at once.
              </p>

              {/* Selection toolbar */}
              <div className="mb-2 flex items-center justify-between">
                <span
                  className="text-[11px]"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  {selected.size} of {preview.triggers.length} selected
                  {warningCount > 0 && ` · ${warningCount} warning${warningCount === 1 ? '' : 's'}`}
                </span>
                <div className="flex gap-2">
                  <button
                    onClick={selectAll}
                    className="text-[11px] underline"
                    style={{ color: 'var(--color-accent)' }}
                  >
                    Select all
                  </button>
                  <button
                    onClick={selectNone}
                    className="text-[11px] underline"
                    style={{ color: 'var(--color-accent)' }}
                  >
                    Select none
                  </button>
                </div>
              </div>

              {/* Trigger list */}
              <div
                className="rounded"
                style={{ border: '1px solid var(--color-border)' }}
              >
                {preview.triggers.map((it, i) => (
                  <TriggerRow
                    key={i}
                    item={it}
                    checked={selected.has(i)}
                    onToggle={() => toggle(i)}
                    last={i === preview.triggers.length - 1}
                  />
                ))}
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-between gap-3 px-4 py-3"
          style={{ borderTop: '1px solid var(--color-border)' }}
        >
          <span className="text-xs" style={{ color: 'var(--color-danger)' }}>
            {commitError}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => !committing && onClose()}
              className="rounded px-3 py-1.5 text-xs"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              Cancel
            </button>
            <button
              onClick={handleImport}
              disabled={!preview || committing || selected.size === 0}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-accent)',
                color: 'var(--color-accent-foreground, #fff)',
              }}
            >
              {committing ? (
                <RefreshCw size={12} className="animate-spin" />
              ) : (
                <CheckCircle2 size={12} />
              )}
              Import {selected.size > 0 ? selected.size : ''}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

interface TriggerRowProps {
  item: ImportedTrigger
  checked: boolean
  onToggle: () => void
  last: boolean
}

function TriggerRow({
  item,
  checked,
  onToggle,
  last,
}: TriggerRowProps): React.ReactElement {
  const { trigger, warnings, original_group, regex_ok } = item
  return (
    <button
      onClick={onToggle}
      className="flex w-full items-start gap-2 px-2.5 py-2 text-left"
      style={{
        borderBottom: last ? 'none' : '1px solid var(--color-border)',
        backgroundColor: checked ? 'var(--color-surface-2)' : 'transparent',
      }}
    >
      <span className="mt-0.5 shrink-0" style={{ color: 'var(--color-accent)' }}>
        {checked ? <CheckSquare size={14} /> : <Square size={14} style={{ color: 'var(--color-muted)' }} />}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span
            className="truncate text-xs font-medium"
            style={{ color: 'var(--color-foreground)' }}
          >
            {trigger.name}
          </span>
          {!regex_ok && (
            <span
              className="shrink-0 rounded px-1 py-0.5 text-[10px]"
              style={{
                backgroundColor: 'var(--color-surface)',
                color: 'var(--color-danger)',
                border: '1px solid var(--color-border)',
              }}
            >
              regex
            </span>
          )}
        </div>
        <div
          className="truncate font-mono text-[10px]"
          style={{ color: 'var(--color-muted)' }}
        >
          {trigger.pattern}
        </div>
        {original_group && (
          <div className="text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
            {original_group}
          </div>
        )}
        {warnings && warnings.length > 0 && (
          <div className="mt-1 space-y-0.5">
            {warnings.map((wmsg, j) => (
              <div
                key={j}
                className="flex items-start gap-1 text-[10px]"
                style={{ color: 'var(--color-warning, #d9a441)' }}
              >
                <AlertTriangle size={10} className="mt-0.5 shrink-0" />
                <span>{wmsg}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </button>
  )
}

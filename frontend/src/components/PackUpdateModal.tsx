import React, { useEffect, useMemo, useState } from 'react'
import {
  X,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  CheckSquare,
  Square,
  Pencil,
  Plus,
  Trash2,
  Undo2,
} from 'lucide-react'
import { getPackDiff, applyPackUpdate } from '../services/api'
import type {
  PackDiff,
  PackFieldDiff,
  PackUpdateMode,
  PackUpdateResult,
} from '../types/trigger'

interface PackUpdateModalProps {
  packName: string
  onClose: () => void
  // Called after a successful apply so the parent refreshes summaries +
  // trigger list.
  onApplied: (result: PackUpdateResult) => void
}

/**
 * Review-and-apply dialog for a built-in pack whose shipped definitions
 * changed since the user installed it. Groups the diff into color-coded
 * changed / added / removed / deleted-by-you sections with per-trigger
 * checkboxes, expandable field-level old→new comparisons, and two apply
 * modes: keep-my-customizations (per-field merge) or full reset.
 */
export default function PackUpdateModal({
  packName,
  onClose,
  onApplied,
}: PackUpdateModalProps): React.ReactElement {
  const [diff, setDiff] = useState<PackDiff | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mode, setMode] = useState<PackUpdateMode>('preserve')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [applying, setApplying] = useState(false)
  const [applyError, setApplyError] = useState<string | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !applying) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, applying])

  useEffect(() => {
    getPackDiff(packName)
      .then((d) => {
        setDiff(d)
        // Pending updates default checked; triggers the user deleted
        // locally default unchecked (opt-in resurrection only).
        const initial = new Set<string>()
        for (const c of d.changed ?? []) initial.add(c.pack_key)
        for (const a of d.added ?? []) initial.add(a.pack_key)
        for (const r of d.removed ?? []) initial.add(r.pack_key)
        setSelected(initial)
      })
      .catch((err: Error) => setLoadError(err.message))
  }, [packName])

  const changed = diff?.changed ?? []
  const added = diff?.added ?? []
  const removed = diff?.removed ?? []
  const deletedLocally = diff?.deleted_locally ?? []

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const toggleExpanded = (key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const customizedCount = useMemo(
    () =>
      changed.filter((c) => c.fields.some((f) => f.user_customized)).length,
    [changed],
  )

  const handleApply = () => {
    if (selected.size === 0) return
    setApplying(true)
    setApplyError(null)
    applyPackUpdate(packName, mode, Array.from(selected))
      .then((res) => onApplied(res))
      .catch((err: Error) => setApplyError(err.message))
      .finally(() => setApplying(false))
  }

  const checkbox = (key: string) =>
    selected.has(key) ? (
      <CheckSquare size={14} style={{ color: 'var(--color-primary)' }} />
    ) : (
      <Square size={14} style={{ color: 'var(--color-muted)' }} />
    )

  const sectionHeader = (
    icon: React.ReactNode,
    label: string,
    count: number,
    color: string,
  ) => (
    <div className="flex items-center gap-1.5 pt-2">
      {icon}
      <span className="text-[11px] font-semibold uppercase tracking-widest" style={{ color }}>
        {label} ({count})
      </span>
    </div>
  )

  return (
    <div
      onClick={() => !applying && onClose()}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg flex flex-col"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 720,
          maxHeight: '85vh',
        }}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between p-3"
          style={{ borderBottom: '1px solid var(--color-border)' }}
        >
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Update "{packName}" pack
          </p>
          <button onClick={() => !applying && onClose()} aria-label="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-3 space-y-2">
          {loadError && (
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
              {loadError}
            </p>
          )}
          {!diff && !loadError && (
            <div className="flex items-center justify-center py-8">
              <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          )}

          {diff && (
            <>
              <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                This app version ships changes to the "{packName}" pack.
                Review below, uncheck anything you want to skip, then apply.
                {customizedCount > 0 && (
                  <>
                    {' '}You customized {customizedCount} of the changed trigger
                    {customizedCount !== 1 ? 's' : ''} — with "Keep my
                    customizations", the fields you changed (including actions
                    and sounds) stay yours.
                  </>
                )}
              </p>

              {/* Changed */}
              {changed.length > 0 && (
                <div>
                  {sectionHeader(
                    <Pencil size={11} style={{ color: 'var(--color-warning, #f59e0b)' }} />,
                    'Updated',
                    changed.length,
                    'var(--color-warning, #f59e0b)',
                  )}
                  <div className="space-y-1 mt-1">
                    {changed.map((c) => {
                      const isOpen = expanded.has(c.pack_key)
                      return (
                        <div
                          key={c.pack_key}
                          className="rounded"
                          style={{
                            backgroundColor: 'var(--color-surface-2)',
                            border: '1px solid var(--color-border)',
                            borderLeft: '2px solid var(--color-warning, #f59e0b)',
                          }}
                        >
                          <div className="flex items-center gap-2 px-2 py-1.5">
                            <button onClick={() => toggle(c.pack_key)} aria-label="Include">
                              {checkbox(c.pack_key)}
                            </button>
                            <button
                              onClick={() => toggleExpanded(c.pack_key)}
                              className="flex-1 min-w-0 flex items-center gap-1.5 text-left"
                              aria-expanded={isOpen}
                            >
                              {isOpen ? (
                                <ChevronDown size={12} style={{ color: 'var(--color-muted)' }} />
                              ) : (
                                <ChevronRight size={12} style={{ color: 'var(--color-muted)' }} />
                              )}
                              <span className="text-xs truncate" style={{ color: 'var(--color-foreground)' }}>
                                {c.installed_name}
                              </span>
                              <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
                                {c.fields.length} field{c.fields.length !== 1 ? 's' : ''}
                              </span>
                              {c.fields.some((f) => f.user_customized) && (
                                <span
                                  className="text-[10px] px-1 py-0.5 rounded shrink-0"
                                  style={{
                                    backgroundColor: 'var(--color-surface)',
                                    color: 'var(--color-primary)',
                                  }}
                                >
                                  customized by you
                                </span>
                              )}
                            </button>
                          </div>
                          {isOpen && (
                            <div
                              className="px-2 pb-2 space-y-1.5"
                              style={{ borderTop: '1px solid var(--color-border)' }}
                            >
                              {c.fields.map((f) => (
                                <FieldDiffRow key={f.field} f={f} mode={mode} />
                              ))}
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {/* Added */}
              {added.length > 0 && (
                <div>
                  {sectionHeader(
                    <Plus size={11} style={{ color: 'var(--color-success)' }} />,
                    'New in this version',
                    added.length,
                    'var(--color-success)',
                  )}
                  <div className="space-y-1 mt-1">
                    {added.map((a) => (
                      <div
                        key={a.pack_key}
                        className="flex items-center gap-2 px-2 py-1.5 rounded"
                        style={{
                          backgroundColor: 'var(--color-surface-2)',
                          border: '1px solid var(--color-border)',
                          borderLeft: '2px solid var(--color-success)',
                        }}
                      >
                        <button onClick={() => toggle(a.pack_key)} aria-label="Include">
                          {checkbox(a.pack_key)}
                        </button>
                        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                          {a.name}
                        </span>
                        <span className="text-[10px] font-mono truncate" style={{ color: 'var(--color-muted)' }}>
                          {a.pattern}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Removed */}
              {removed.length > 0 && (
                <div>
                  {sectionHeader(
                    <Trash2 size={11} style={{ color: 'var(--color-danger)' }} />,
                    'Removed from the pack',
                    removed.length,
                    'var(--color-danger)',
                  )}
                  <p className="text-[10px] mt-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
                    Checked triggers are deleted from your install.
                  </p>
                  <div className="space-y-1 mt-1">
                    {removed.map((r) => (
                      <div
                        key={r.pack_key}
                        className="flex items-center gap-2 px-2 py-1.5 rounded"
                        style={{
                          backgroundColor: 'var(--color-surface-2)',
                          border: '1px solid var(--color-border)',
                          borderLeft: '2px solid var(--color-danger)',
                        }}
                      >
                        <button onClick={() => toggle(r.pack_key)} aria-label="Include">
                          {checkbox(r.pack_key)}
                        </button>
                        <span
                          className="text-xs"
                          style={{
                            color: 'var(--color-foreground)',
                            textDecoration: selected.has(r.pack_key) ? 'line-through' : 'none',
                          }}
                        >
                          {r.name}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Deleted locally */}
              {deletedLocally.length > 0 && (
                <div>
                  {sectionHeader(
                    <Undo2 size={11} style={{ color: 'var(--color-muted)' }} />,
                    'Deleted by you',
                    deletedLocally.length,
                    'var(--color-muted)',
                  )}
                  <p className="text-[10px] mt-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
                    Triggers you removed that the pack still ships. Left alone
                    unless you check them to restore.
                  </p>
                  <div className="space-y-1 mt-1">
                    {deletedLocally.map((d) => (
                      <div
                        key={d.pack_key}
                        className="flex items-center gap-2 px-2 py-1.5 rounded"
                        style={{
                          backgroundColor: 'var(--color-surface-2)',
                          border: '1px solid var(--color-border)',
                        }}
                      >
                        <button onClick={() => toggle(d.pack_key)} aria-label="Restore">
                          {checkbox(d.pack_key)}
                        </button>
                        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                          {d.name}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer: mode + apply */}
        {diff && (
          <div
            className="p-3 space-y-2"
            style={{ borderTop: '1px solid var(--color-border)' }}
          >
            <div className="space-y-1.5">
              <label className="flex items-start gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="pack-update-mode"
                  checked={mode === 'preserve'}
                  onChange={() => setMode('preserve')}
                  className="mt-0.5"
                />
                <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                  Keep my customizations
                  <span className="block text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                    Recommended. Fields you edited — actions, sounds, patterns,
                    categories, characters — keep your values; everything else
                    updates to the new defaults.
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="pack-update-mode"
                  checked={mode === 'reset'}
                  onChange={() => setMode('reset')}
                  className="mt-0.5"
                />
                <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                  Reset to pack defaults
                  <span className="block text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                    Overwrites the checked triggers entirely, including actions,
                    category, and active characters.
                  </span>
                </span>
              </label>
            </div>
            {applyError && (
              <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
                {applyError}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button
                onClick={() => !applying && onClose()}
                className="text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleApply}
                disabled={applying || selected.size === 0}
                className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-background)',
                  opacity: applying || selected.size === 0 ? 0.6 : 1,
                }}
              >
                {applying && <RefreshCw size={11} className="animate-spin" />}
                Apply {selected.size} update{selected.size !== 1 ? 's' : ''}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

/**
 * One field's old→new comparison. Values are pre-rendered by the backend
 * (plain strings unquoted, list fields as JSON).
 */
function FieldDiffRow({ f, mode }: { f: PackFieldDiff; mode: PackUpdateMode }): React.ReactElement {
  const kept = f.user_customized && mode === 'preserve'
  return (
    <div className="pt-1.5 text-[11px] space-y-0.5">
      <div className="flex items-center gap-1.5">
        <span className="font-medium" style={{ color: 'var(--color-foreground)' }}>
          {f.label}
        </span>
        {f.user_customized && (
          <span
            className="text-[10px] px-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface)',
              color: kept ? 'var(--color-primary)' : 'var(--color-warning, #f59e0b)',
            }}
          >
            {kept ? 'your value kept' : 'your edit will be overwritten'}
          </span>
        )}
      </div>
      <div className="grid grid-cols-2 gap-2">
        <DiffCell label="Was" value={f.old} muted />
        <DiffCell label="Now" value={kept ? f.current : f.new} muted={false} />
      </div>
    </div>
  )
}

function DiffCell({ label, value, muted }: { label: string; value: string; muted: boolean }): React.ReactElement {
  return (
    <div
      className="rounded px-1.5 py-1 min-w-0"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <span className="block text-[9px] uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
        {label}
      </span>
      <span
        className="block font-mono break-all whitespace-pre-wrap"
        style={{ color: muted ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
      >
        {value === '' ? '—' : value}
      </span>
    </div>
  )
}

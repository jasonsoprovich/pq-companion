import React, { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import type { RawRow } from '../services/api'

interface RawDataModalProps {
  open: boolean
  title: string
  /** Fetcher invoked when the modal opens; receives no args. */
  fetcher: () => Promise<RawRow>
  onClose: () => void
}

function formatValue(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return JSON.stringify(v)
}

export default function RawDataModal({
  open,
  title,
  fetcher,
  onClose,
}: RawDataModalProps): React.ReactElement | null {
  const [row, setRow] = useState<RawRow | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    if (!open) return
    setRow(null)
    setError(null)
    setFilter('')
    fetcher()
      .then(setRow)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
  }, [open, fetcher])

  useEffect(() => {
    if (!open) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open) return null

  const fields = row?.fields ?? []
  const needle = filter.trim().toLowerCase()
  const visible = needle
    ? fields.filter(
        (f) =>
          f.name.toLowerCase().includes(needle) ||
          formatValue(f.value).toLowerCase().includes(needle),
      )
    : fields

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="flex flex-col rounded-lg shadow-2xl w-full max-w-5xl max-h-[85vh]"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="shrink-0 border-b px-5 pt-4 pb-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <div className="flex items-start justify-between gap-2 mb-2">
            <div className="min-w-0">
              <h2
                className="text-lg font-bold leading-tight"
                style={{ color: 'var(--color-primary)' }}
              >
                Raw Data
              </h2>
              <div
                className="text-xs mt-0.5 truncate"
                style={{ color: 'var(--color-muted)' }}
              >
                {title}
                {row?.table ? (
                  <span className="ml-2" style={{ opacity: 0.7 }}>
                    · table: {row.table}
                  </span>
                ) : null}
              </div>
            </div>
            <button onClick={onClose} className="shrink-0 mt-0.5" title="Close">
              <X size={16} style={{ color: 'var(--color-muted)' }} />
            </button>
          </div>
          <input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter columns…"
            className="w-full rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-1)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          />
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 text-xs">
          {error && (
            <div style={{ color: 'var(--color-danger, #f87171)' }}>
              Failed to load raw data: {error}
            </div>
          )}
          {!error && !row && (
            <div style={{ color: 'var(--color-muted)' }}>Loading…</div>
          )}
          {!error && row && visible.length === 0 && (
            <div style={{ color: 'var(--color-muted)' }}>No matching columns.</div>
          )}
          {!error && row && visible.length > 0 && (
            <div
              style={{
                columnWidth: '220px',
                columnGap: '1.5rem',
              }}
            >
              {visible.map((f) => (
                <div
                  key={f.name}
                  className="flex gap-1.5 leading-snug py-0.5"
                  style={{ breakInside: 'avoid' }}
                >
                  <span
                    className="font-semibold shrink-0"
                    style={{ color: 'var(--color-text)' }}
                  >
                    {f.name}:
                  </span>
                  <span
                    className="break-all"
                    style={{ color: 'var(--color-muted)' }}
                  >
                    {formatValue(f.value)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div
          className="shrink-0 border-t px-5 py-2 flex justify-end"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <button
            onClick={onClose}
            className="rounded px-3 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-1)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}

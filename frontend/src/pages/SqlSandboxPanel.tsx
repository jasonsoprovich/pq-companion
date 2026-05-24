import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Play, Loader2, Copy, Check, AlertTriangle, ChevronRight, ChevronDown, Database, Table as TableIcon, Eye, BookOpen } from 'lucide-react'
import { getSandboxSchema, runSandboxQuery } from '../services/api'
import type { SandboxResult, SandboxTable } from '../types/sandbox'
import { STARTER_QUERIES } from '../lib/sandboxStarterQueries'

const DEFAULT_QUERY = `-- Try a query. Hard cap: 10,000 rows, 8s deadline.
-- Tip: click a table in the sidebar to insert its name at the cursor.
SELECT id, Name, ac, hp, mana
FROM items
WHERE Name LIKE '%fungus%'
ORDER BY ac DESC
LIMIT 50;`

// Visible-row cap to keep the DOM bounded. 10k rows of HTML cells stalls
// the renderer; 1k is plenty for exploration and the user can refine the
// query if they need to see more rows at once.
const VISIBLE_ROW_CAP = 1000

export default function SqlSandboxPanel(): React.ReactElement {
  const [sql, setSql] = useState<string>(DEFAULT_QUERY)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<SandboxResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [schema, setSchema] = useState<SandboxTable[] | null>(null)
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [copied, setCopied] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    getSandboxSchema()
      .then((r) => setSchema(r.tables))
      .catch((e: Error) => setSchemaError(e.message))
  }, [])

  async function run(): Promise<void> {
    if (running) return
    setRunning(true)
    setError(null)
    setShowAll(false)
    try {
      const r = await runSandboxQuery(sql)
      setResult(r)
    } catch (e) {
      setError((e as Error).message)
      setResult(null)
    } finally {
      setRunning(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>): void {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void run()
    }
  }

  function insertAtCursor(text: string): void {
    const el = textareaRef.current
    if (!el) {
      setSql((s) => s + text)
      return
    }
    const start = el.selectionStart ?? sql.length
    const end = el.selectionEnd ?? sql.length
    const next = sql.slice(0, start) + text + sql.slice(end)
    setSql(next)
    // Restore caret right after the inserted text.
    requestAnimationFrame(() => {
      el.focus()
      el.selectionStart = el.selectionEnd = start + text.length
    })
  }

  async function copyQuery(): Promise<void> {
    try {
      await navigator.clipboard.writeText(sql)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Some restricted contexts (older Electron) reject clipboard writes;
      // fall through silently rather than crash the page.
    }
  }

  const visibleRows = useMemo(() => {
    if (!result) return null
    if (showAll || result.rows.length <= VISIBLE_ROW_CAP) return result.rows
    return result.rows.slice(0, VISIBLE_ROW_CAP)
  }, [result, showAll])

  return (
    <section
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="mb-3 flex items-center gap-2">
        <Database size={14} style={{ color: 'var(--color-primary)' }} />
        <h3
          className="text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          SQL Sandbox
        </h3>
      </div>

      <div className="flex gap-3" style={{ minHeight: 320 }}>
        {/* Schema sidebar */}
        <aside
          className="shrink-0 overflow-y-auto rounded text-xs"
          style={{
            width: 220,
            maxHeight: 520,
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <div
            className="sticky top-0 px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wide"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted)',
              borderBottom: '1px solid var(--color-border)',
            }}
          >
            Tables
          </div>
          {schemaError && (
            <p className="px-3 py-2" style={{ color: '#f87171' }}>
              {schemaError}
            </p>
          )}
          {!schema && !schemaError && (
            <p className="px-3 py-2" style={{ color: 'var(--color-muted-foreground)' }}>
              Loading schema…
            </p>
          )}
          {schema && schema.map((t) => {
            const isOpen = !!expanded[t.name]
            return (
              <div key={t.name}>
                <div
                  className="flex items-center gap-1 px-2 py-1"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  <button
                    type="button"
                    onClick={() => setExpanded((e) => ({ ...e, [t.name]: !isOpen }))}
                    title={isOpen ? 'Hide columns' : 'Show columns'}
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      cursor: 'pointer',
                      color: 'var(--color-muted)',
                    }}
                  >
                    {isOpen ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
                  </button>
                  {t.kind === 'view' ? (
                    <Eye size={11} style={{ color: 'var(--color-muted)' }} />
                  ) : (
                    <TableIcon size={11} style={{ color: 'var(--color-muted)' }} />
                  )}
                  <button
                    type="button"
                    onClick={() => insertAtCursor(t.name)}
                    title="Insert table name at cursor"
                    className="truncate text-left"
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      cursor: 'pointer',
                      color: 'var(--color-foreground)',
                      flex: 1,
                    }}
                  >
                    {t.name}
                  </button>
                </div>
                {isOpen && (
                  <div className="pb-1" style={{ paddingLeft: 28 }}>
                    {t.columns.map((c) => (
                      <button
                        key={c.name}
                        type="button"
                        onClick={() => insertAtCursor(c.name)}
                        title={`Insert "${c.name}" at cursor`}
                        className="block w-full truncate text-left text-[11px]"
                        style={{
                          background: 'transparent',
                          border: 'none',
                          padding: '1px 0',
                          cursor: 'pointer',
                          color: c.pk ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                        }}
                      >
                        {c.name}
                        <span className="ml-1" style={{ color: 'var(--color-muted)' }}>
                          {c.type || '?'}
                          {c.pk ? ' · pk' : ''}
                        </span>
                      </button>
                    ))}
                    {t.columns.length === 0 && (
                      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                        (no columns)
                      </span>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </aside>

        {/* Query + results */}
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <textarea
            ref={textareaRef}
            value={sql}
            onChange={(e) => setSql(e.target.value)}
            onKeyDown={handleKeyDown}
            spellCheck={false}
            rows={10}
            className="w-full rounded px-3 py-2 font-mono text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              outline: 'none',
              resize: 'vertical',
              minHeight: 160,
            }}
          />

          <div className="flex items-center gap-2">
            <ExamplesPicker
              onPick={(sql) => {
                setSql(sql)
                setResult(null)
                setError(null)
              }}
            />
            <button
              type="button"
              onClick={() => void run()}
              disabled={running}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: '#fff',
                border: 'none',
                cursor: running ? 'not-allowed' : 'pointer',
                opacity: running ? 0.7 : 1,
              }}
            >
              {running ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              {running ? 'Running…' : 'Run'}
            </button>
            <button
              type="button"
              onClick={() => void copyQuery()}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
            >
              {copied ? <Check size={12} /> : <Copy size={12} />}
              {copied ? 'Copied' : 'Copy'}
            </button>
            <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              <kbd>Ctrl/Cmd+Enter</kbd> runs
            </span>
            <div className="ml-auto text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {result && (
                <span>
                  {result.row_count.toLocaleString()} row{result.row_count === 1 ? '' : 's'} in {result.duration_ms} ms
                  {result.truncated && ' · capped at 10,000'}
                </span>
              )}
            </div>
          </div>

          {error && (
            <div
              className="flex items-start gap-2 rounded px-3 py-2 text-xs"
              style={{
                backgroundColor: 'color-mix(in srgb, #f87171 12%, transparent)',
                border: '1px solid #f87171',
                color: '#f87171',
              }}
            >
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <pre className="whitespace-pre-wrap break-all font-mono">{error}</pre>
            </div>
          )}

          {result && visibleRows && (
            <div
              className="overflow-auto rounded"
              style={{
                border: '1px solid var(--color-border)',
                backgroundColor: 'var(--color-surface-2)',
                maxHeight: 520,
              }}
            >
              {result.rows.length === 0 ? (
                <p className="px-3 py-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  (no rows)
                </p>
              ) : (
                <table className="w-full text-xs">
                  <thead
                    style={{
                      position: 'sticky',
                      top: 0,
                      backgroundColor: 'var(--color-surface-2)',
                      borderBottom: '1px solid var(--color-border)',
                    }}
                  >
                    <tr>
                      {result.columns.map((c) => (
                        <th
                          key={c}
                          className="px-2 py-1 text-left font-semibold"
                          style={{ color: 'var(--color-muted)', whiteSpace: 'nowrap' }}
                        >
                          {c}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {visibleRows.map((row, i) => (
                      <tr
                        key={i}
                        style={{
                          borderTop: i === 0 ? undefined : '1px solid var(--color-border)',
                        }}
                      >
                        {row.map((v, j) => (
                          <td
                            key={j}
                            className="px-2 py-1 font-mono"
                            style={{
                              color: v === null
                                ? 'var(--color-muted)'
                                : 'var(--color-foreground)',
                              whiteSpace: 'nowrap',
                              fontStyle: v === null ? 'italic' : 'normal',
                            }}
                          >
                            {formatCell(v)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {result.rows.length > VISIBLE_ROW_CAP && !showAll && (
                <div
                  className="border-t px-3 py-2 text-xs"
                  style={{
                    borderColor: 'var(--color-border)',
                    color: 'var(--color-muted-foreground)',
                  }}
                >
                  Showing first {VISIBLE_ROW_CAP.toLocaleString()} of{' '}
                  {result.rows.length.toLocaleString()} rows.{' '}
                  <button
                    type="button"
                    onClick={() => setShowAll(true)}
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      cursor: 'pointer',
                      color: 'var(--color-primary)',
                      textDecoration: 'underline',
                    }}
                  >
                    Show all
                  </button>{' '}
                  (may be slow).
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

// ExamplesPicker shows a small dropdown of curated SELECT statements the
// user can drop into the query box as a starting point. Click outside or
// pick an entry to dismiss.
function ExamplesPicker({ onPick }: { onPick: (sql: string) => void }): React.ReactElement {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent): void => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [open])
  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
          cursor: 'pointer',
        }}
      >
        <BookOpen size={12} />
        Examples
      </button>
      {open && (
        <div
          className="absolute z-10 mt-1 overflow-hidden rounded shadow-lg"
          style={{
            top: '100%',
            left: 0,
            width: 320,
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
          }}
        >
          {STARTER_QUERIES.map((q) => (
            <button
              key={q.id}
              type="button"
              onClick={() => {
                onPick(q.sql)
                setOpen(false)
              }}
              className="block w-full px-3 py-2 text-left text-xs"
              style={{
                background: 'transparent',
                border: 'none',
                borderBottom: '1px solid var(--color-border)',
                cursor: 'pointer',
                color: 'var(--color-foreground)',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = 'var(--color-surface-2)')}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              <div style={{ fontWeight: 600 }}>{q.label}</div>
              <div className="mt-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
                {q.description}
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// formatCell renders a SQLite scalar to a string for display. null becomes
// the literal "NULL" (styled italic by the caller), numbers and strings
// pass through verbatim, and anything exotic gets JSON-stringified.
function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') {
    return String(v)
  }
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

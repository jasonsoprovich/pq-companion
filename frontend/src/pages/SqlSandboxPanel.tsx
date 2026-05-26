import React, { useEffect, useMemo, useRef, useState } from 'react'
import {
  Play,
  Loader2,
  Copy,
  Check,
  AlertTriangle,
  ChevronRight,
  ChevronDown,
  Table as TableIcon,
  Eye,
  BookOpen,
  History as HistoryIcon,
  Bookmark,
  BookmarkPlus,
  Trash2,
  Download,
  Upload,
} from 'lucide-react'
import {
  getSandboxSchema,
  runSandboxQuery,
  listSavedQueries,
  createSavedQuery,
  updateSavedQuery,
  deleteSavedQuery,
  exportSavedQueryPack,
  importSavedQueryPack,
} from '../services/api'
import type {
  SandboxResult,
  SandboxTable,
  SavedQuery,
  SavedQueryPack,
} from '../types/sandbox'
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

// sessionStorage keys — scoped per-window so they survive tab switches
// inside the app but clear when the user quits.
const STORAGE_KEY_QUERY = 'sql-sandbox.query'
const STORAGE_KEY_HISTORY = 'sql-sandbox.history'
const HISTORY_LIMIT = 15

function loadInitialQuery(): string {
  try {
    const saved = sessionStorage.getItem(STORAGE_KEY_QUERY)
    if (saved !== null) return saved
  } catch {
    // sessionStorage can throw in restricted contexts; fall through.
  }
  return DEFAULT_QUERY
}

function loadInitialHistory(): string[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY_HISTORY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (Array.isArray(parsed)) return parsed.filter((v): v is string => typeof v === 'string')
  } catch {
    // Ignore corrupt history — better to start fresh than crash the panel.
  }
  return []
}

export default function SqlSandboxPanel(): React.ReactElement {
  const [sql, setSql] = useState<string>(loadInitialQuery)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<SandboxResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [schema, setSchema] = useState<SandboxTable[] | null>(null)
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [copied, setCopied] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const [history, setHistory] = useState<string[]>(loadInitialHistory)
  const [saved, setSaved] = useState<SavedQuery[]>([])
  const [savedError, setSavedError] = useState<string | null>(null)
  // Tracks the id of the currently-loaded saved query (if any) so the Save
  // button can offer "Save changes" vs "Save as new". Cleared whenever the
  // user picks Examples / History or starts a new query from scratch.
  const [loadedSavedId, setLoadedSavedId] = useState<string | null>(null)
  const [saveDialogOpen, setSaveDialogOpen] = useState(false)
  const [saveDialogName, setSaveDialogName] = useState('')
  const [saveDialogDesc, setSaveDialogDesc] = useState('')
  const [saving, setSaving] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const importInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    getSandboxSchema()
      .then((r) => setSchema(r.tables))
      .catch((e: Error) => setSchemaError(e.message))
  }, [])

  // Load the user's saved queries on mount. Failures aren't fatal — the
  // sandbox still works without them, so we just surface a small inline
  // error inside the Saved dropdown.
  useEffect(() => {
    listSavedQueries()
      .then((r) => setSaved(r.queries))
      .catch((e: Error) => setSavedError(e.message))
  }, [])

  const loadedSaved = useMemo(
    () => (loadedSavedId ? saved.find((q) => q.id === loadedSavedId) ?? null : null),
    [saved, loadedSavedId],
  )
  const hasUnsavedChanges = loadedSaved !== null && loadedSaved.sql !== sql

  // Persist the in-progress query so a tab switch (component unmount)
  // doesn't wipe the user's work. Cleared automatically on app quit
  // because sessionStorage is per-window.
  useEffect(() => {
    try {
      sessionStorage.setItem(STORAGE_KEY_QUERY, sql)
    } catch {
      // Storage quota / private-mode restrictions — ignore.
    }
  }, [sql])

  useEffect(() => {
    try {
      sessionStorage.setItem(STORAGE_KEY_HISTORY, JSON.stringify(history))
    } catch {
      // Same fallback as the query persist.
    }
  }, [history])

  function pushHistory(entry: string): void {
    const trimmed = entry.trim()
    if (!trimmed) return
    setHistory((prev) => {
      // Dedup: drop any earlier copy so the latest run is always at the top.
      const filtered = prev.filter((q) => q !== entry)
      return [entry, ...filtered].slice(0, HISTORY_LIMIT)
    })
  }

  async function run(): Promise<void> {
    if (running) return
    setRunning(true)
    setError(null)
    setShowAll(false)
    try {
      const r = await runSandboxQuery(sql)
      setResult(r)
      pushHistory(sql)
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

  function openSaveDialog(): void {
    // Prefill the form with the loaded saved query's metadata when there is
    // one, so the common "edit & save changes" path doesn't force the user
    // to retype the name every time.
    if (loadedSaved) {
      setSaveDialogName(loadedSaved.name)
      setSaveDialogDesc(loadedSaved.description)
    } else {
      setSaveDialogName('')
      setSaveDialogDesc('')
    }
    setSaveDialogOpen(true)
  }

  async function performSave(mode: 'update' | 'create'): Promise<void> {
    const name = saveDialogName.trim()
    if (!name) {
      setSavedError('Name is required.')
      return
    }
    setSaving(true)
    setSavedError(null)
    try {
      if (mode === 'update' && loadedSavedId) {
        const updated = await updateSavedQuery(loadedSavedId, {
          name,
          description: saveDialogDesc,
          sql,
        })
        setSaved((prev) => sortByName(prev.map((q) => (q.id === updated.id ? updated : q))))
      } else {
        const created = await createSavedQuery({ name, description: saveDialogDesc, sql })
        setSaved((prev) => sortByName([...prev, created]))
        setLoadedSavedId(created.id)
      }
      setSaveDialogOpen(false)
    } catch (e) {
      setSavedError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  function loadSavedQuery(q: SavedQuery): void {
    setSql(q.sql)
    setLoadedSavedId(q.id)
    setResult(null)
    setError(null)
  }

  async function removeSavedQuery(id: string): Promise<void> {
    try {
      await deleteSavedQuery(id)
      setSaved((prev) => prev.filter((q) => q.id !== id))
      if (loadedSavedId === id) setLoadedSavedId(null)
    } catch (e) {
      setSavedError((e as Error).message)
    }
  }

  async function exportPack(): Promise<void> {
    try {
      const pack = await exportSavedQueryPack()
      // Build a Blob on the renderer side and trigger a download via a
      // synthetic anchor. Keeps the file picker (and "where to save")
      // entirely client-side, where the user expects it.
      const blob = new Blob([JSON.stringify(pack, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'pq-companion-queries.json'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (e) {
      setSavedError((e as Error).message)
    }
  }

  function triggerImport(): void {
    importInputRef.current?.click()
  }

  async function handleImportFile(file: File): Promise<void> {
    try {
      const text = await file.text()
      let pack: SavedQueryPack
      try {
        pack = JSON.parse(text) as SavedQueryPack
      } catch {
        setSavedError('Selected file is not valid JSON.')
        return
      }
      if (pack.kind !== 'pq-companion.query-pack') {
        setSavedError(
          'Selected file is not a PQ Companion query pack (missing or wrong "kind" field).',
        )
        return
      }
      await importSavedQueryPack(pack)
      const refreshed = await listSavedQueries()
      setSaved(refreshed.queries)
      setSavedError(null)
    } catch (e) {
      setSavedError((e as Error).message)
    }
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
      {/* Top row: schema sidebar (left) + query editor (right). Results
          render in a separate block below so they get the full panel
          width — clicking Run shouldn't squeeze the data into a narrow
          column next to the sidebar. */}
      <div className="flex gap-3">
        {/* Schema sidebar */}
        <aside
          className="shrink-0 overflow-y-auto rounded text-xs"
          style={{
            width: 260,
            maxHeight: 360,
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

        {/* Query editor + action row */}
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          {loadedSaved && !saveDialogOpen && (
            <div
              className="flex items-center gap-2 rounded px-3 py-1.5 text-[11px]"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
            >
              <Bookmark size={12} style={{ color: 'var(--color-primary)' }} />
              <span>
                Editing <strong style={{ color: 'var(--color-foreground)' }}>{loadedSaved.name}</strong>
              </span>
              {hasUnsavedChanges && (
                <span style={{ color: '#fbbf24' }}>· unsaved changes</span>
              )}
              <button
                type="button"
                onClick={() => setLoadedSavedId(null)}
                className="ml-auto"
                style={{
                  background: 'transparent',
                  border: 'none',
                  padding: 0,
                  cursor: 'pointer',
                  color: 'var(--color-muted)',
                  textDecoration: 'underline',
                }}
                title="Detach this query from the saved entry (won't delete it)"
              >
                detach
              </button>
            </div>
          )}
          {saveDialogOpen && (
            <SaveDialog
              name={saveDialogName}
              description={saveDialogDesc}
              onName={setSaveDialogName}
              onDescription={setSaveDialogDesc}
              saving={saving}
              canUpdate={loadedSavedId !== null}
              onCancel={() => {
                setSaveDialogOpen(false)
                setSavedError(null)
              }}
              onUpdate={() => void performSave('update')}
              onCreate={() => void performSave('create')}
            />
          )}
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
              minHeight: 200,
            }}
          />

          <div className="flex flex-wrap items-center gap-2">
            <ExamplesPicker
              onPick={(sql) => {
                setSql(sql)
                setLoadedSavedId(null)
                setResult(null)
                setError(null)
              }}
            />
            <HistoryPicker
              history={history}
              onPick={(sql) => {
                setSql(sql)
                setLoadedSavedId(null)
                setResult(null)
                setError(null)
              }}
              onClear={() => setHistory([])}
            />
            <SavedPicker
              saved={saved}
              error={savedError}
              loadedSavedId={loadedSavedId}
              onPick={loadSavedQuery}
              onDelete={(id) => void removeSavedQuery(id)}
              onExport={() => void exportPack()}
              onImport={triggerImport}
            />
            <button
              type="button"
              onClick={openSaveDialog}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
              title={
                loadedSaved
                  ? hasUnsavedChanges
                    ? `Update "${loadedSaved.name}" or save a new copy`
                    : `Edit "${loadedSaved.name}" metadata`
                  : 'Save this query for later'
              }
            >
              <BookmarkPlus size={12} />
              {loadedSaved && hasUnsavedChanges ? 'Save changes' : 'Save'}
            </button>
            <input
              ref={importInputRef}
              type="file"
              accept="application/json,.json"
              style={{ display: 'none' }}
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) void handleImportFile(file)
                // Reset so picking the same file twice still fires onChange.
                e.target.value = ''
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
        </div>
      </div>

      {/* Results: full-width below the query so wide row sets (items,
          spells, NPCs) get the horizontal room they need. Stays hidden
          until the first run. */}
      {(error || result) && (
        <div className="mt-4">
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
                maxHeight: 560,
              }}
            >
              {result.rows.length === 0 ? (
                <p className="px-3 py-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  (no rows)
                </p>
              ) : (
                <table className="min-w-full text-xs" style={{ width: 'max-content' }}>
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
      )}
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

// sortByName keeps the in-memory list aligned with the backend's
// ORDER BY name COLLATE NOCASE so insertions don't briefly shuffle the
// dropdown out of order.
function sortByName(list: SavedQuery[]): SavedQuery[] {
  return [...list].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
}

// SavedPicker is the dropdown next to Examples/History that lists the
// user's saved queries from user.db. Each row is clickable to load,
// plus a small Delete affordance and pack import/export at the bottom.
function SavedPicker({
  saved,
  error,
  loadedSavedId,
  onPick,
  onDelete,
  onExport,
  onImport,
}: {
  saved: SavedQuery[]
  error: string | null
  loadedSavedId: string | null
  onPick: (q: SavedQuery) => void
  onDelete: (id: string) => void
  onExport: () => void
  onImport: () => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent): void => {
      if (!ref.current?.contains(e.target as Node)) {
        setOpen(false)
        setConfirmId(null)
      }
    }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [open])
  const count = saved.length
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
        <Bookmark size={12} />
        Saved
        {count > 0 && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
            ({count})
          </span>
        )}
      </button>
      {open && (
        <div
          className="absolute z-10 mt-1 overflow-hidden rounded shadow-lg"
          style={{
            top: '100%',
            left: 0,
            width: 440,
            maxHeight: 420,
            display: 'flex',
            flexDirection: 'column',
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
          }}
        >
          <div
            style={{
              overflowY: 'auto',
              flex: 1,
            }}
          >
            {error && (
              <p className="px-3 py-2 text-xs" style={{ color: '#f87171' }}>
                {error}
              </p>
            )}
            {!error && saved.length === 0 && (
              <p className="px-3 py-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                No saved queries yet. Use <strong>Save</strong> after a run to keep one for later.
              </p>
            )}
            {saved.map((q) => {
              const isLoaded = q.id === loadedSavedId
              const confirming = q.id === confirmId
              return (
                <div
                  key={q.id}
                  className="flex items-start gap-2 px-3 py-2 text-xs"
                  style={{
                    borderBottom: '1px solid var(--color-border)',
                    backgroundColor: isLoaded
                      ? 'color-mix(in srgb, var(--color-primary) 8%, transparent)'
                      : 'transparent',
                  }}
                  onMouseEnter={(e) => {
                    if (!isLoaded) e.currentTarget.style.backgroundColor = 'var(--color-surface-2)'
                  }}
                  onMouseLeave={(e) => {
                    if (!isLoaded) e.currentTarget.style.backgroundColor = 'transparent'
                  }}
                >
                  <button
                    type="button"
                    onClick={() => {
                      onPick(q)
                      setOpen(false)
                    }}
                    className="min-w-0 flex-1 text-left"
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      cursor: 'pointer',
                      color: 'var(--color-foreground)',
                    }}
                    title={q.sql}
                  >
                    <div style={{ fontWeight: 600 }}>{q.name}</div>
                    {q.description && (
                      <div
                        className="mt-0.5"
                        style={{
                          color: 'var(--color-muted-foreground)',
                          whiteSpace: 'nowrap',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                        }}
                      >
                        {q.description}
                      </div>
                    )}
                  </button>
                  {confirming ? (
                    <div className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        onClick={() => {
                          onDelete(q.id)
                          setConfirmId(null)
                        }}
                        className="rounded px-2 py-0.5 text-[10px] font-semibold"
                        style={{
                          backgroundColor: '#dc2626',
                          color: '#fff',
                          border: 'none',
                          cursor: 'pointer',
                        }}
                      >
                        Delete
                      </button>
                      <button
                        type="button"
                        onClick={() => setConfirmId(null)}
                        className="rounded px-2 py-0.5 text-[10px]"
                        style={{
                          backgroundColor: 'transparent',
                          color: 'var(--color-muted)',
                          border: '1px solid var(--color-border)',
                          cursor: 'pointer',
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <button
                      type="button"
                      onClick={() => setConfirmId(q.id)}
                      className="shrink-0"
                      title="Delete this saved query"
                      style={{
                        background: 'transparent',
                        border: 'none',
                        padding: 2,
                        cursor: 'pointer',
                        color: 'var(--color-muted)',
                      }}
                    >
                      <Trash2 size={12} />
                    </button>
                  )}
                </div>
              )
            })}
          </div>
          <div
            className="flex items-center gap-1 px-2 py-1.5"
            style={{
              borderTop: '1px solid var(--color-border)',
              backgroundColor: 'var(--color-surface-2)',
            }}
          >
            <button
              type="button"
              onClick={() => {
                onImport()
                setOpen(false)
              }}
              className="flex items-center gap-1 rounded px-2 py-1 text-[11px]"
              style={{
                background: 'transparent',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
              title="Import a JSON query pack from a file"
            >
              <Upload size={11} />
              Import pack
            </button>
            <button
              type="button"
              onClick={() => {
                onExport()
                setOpen(false)
              }}
              disabled={saved.length === 0}
              className="flex items-center gap-1 rounded px-2 py-1 text-[11px]"
              style={{
                background: 'transparent',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: saved.length === 0 ? 'not-allowed' : 'pointer',
                opacity: saved.length === 0 ? 0.5 : 1,
              }}
              title="Download all saved queries as a JSON pack to share"
            >
              <Download size={11} />
              Export pack
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// SaveDialog is the inline form rendered above the textarea when the user
// clicks Save. Two paths: "Save changes" updates the currently-loaded
// saved entry; "Save as new" always creates a fresh row.
function SaveDialog({
  name,
  description,
  onName,
  onDescription,
  saving,
  canUpdate,
  onCancel,
  onUpdate,
  onCreate,
}: {
  name: string
  description: string
  onName: (v: string) => void
  onDescription: (v: string) => void
  saving: boolean
  canUpdate: boolean
  onCancel: () => void
  onUpdate: () => void
  onCreate: () => void
}): React.ReactElement {
  return (
    <div
      className="flex flex-col gap-2 rounded px-3 py-2 text-xs"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={name}
          onChange={(e) => onName(e.target.value)}
          placeholder="Query name"
          className="rounded px-2 py-1 text-xs"
          style={{
            flex: 1,
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            outline: 'none',
          }}
          autoFocus
        />
        <input
          type="text"
          value={description}
          onChange={(e) => onDescription(e.target.value)}
          placeholder="Description (optional)"
          className="rounded px-2 py-1 text-xs"
          style={{
            flex: 2,
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            outline: 'none',
          }}
        />
      </div>
      <div className="flex items-center gap-2">
        {canUpdate && (
          <button
            type="button"
            onClick={onUpdate}
            disabled={saving}
            className="rounded px-3 py-1 text-xs font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: saving ? 'not-allowed' : 'pointer',
              opacity: saving ? 0.7 : 1,
            }}
          >
            Save changes
          </button>
        )}
        <button
          type="button"
          onClick={onCreate}
          disabled={saving}
          className="rounded px-3 py-1 text-xs font-medium"
          style={{
            backgroundColor: canUpdate ? 'var(--color-surface)' : 'var(--color-primary)',
            color: canUpdate ? 'var(--color-foreground)' : '#fff',
            border: canUpdate ? '1px solid var(--color-border)' : 'none',
            cursor: saving ? 'not-allowed' : 'pointer',
            opacity: saving ? 0.7 : 1,
          }}
        >
          {canUpdate ? 'Save as new' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          disabled={saving}
          className="ml-auto rounded px-3 py-1 text-xs"
          style={{
            backgroundColor: 'transparent',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            cursor: saving ? 'not-allowed' : 'pointer',
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// HistoryPicker surfaces the recent run history (most recent first) so the
// user can re-load a prior query without retyping it. History is kept in
// sessionStorage, so it persists across tab switches inside the app but
// clears on quit. Capped at HISTORY_LIMIT entries.
function HistoryPicker({
  history,
  onPick,
  onClear,
}: {
  history: string[]
  onPick: (sql: string) => void
  onClear: () => void
}): React.ReactElement {
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
  const empty = history.length === 0
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
          opacity: empty ? 0.6 : 1,
        }}
        title={empty ? 'No history yet — run a query first' : 'Recent queries'}
      >
        <HistoryIcon size={12} />
        History
        {!empty && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
            ({history.length})
          </span>
        )}
      </button>
      {open && (
        <div
          className="absolute z-10 mt-1 overflow-hidden rounded shadow-lg"
          style={{
            top: '100%',
            left: 0,
            width: 420,
            maxHeight: 360,
            overflowY: 'auto',
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
          }}
        >
          {empty ? (
            <p className="px-3 py-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No queries yet. Run one and it'll show up here.
            </p>
          ) : (
            <>
              {history.map((q, i) => (
                <button
                  key={`${i}-${q.slice(0, 16)}`}
                  type="button"
                  onClick={() => {
                    onPick(q)
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
                  title={q}
                >
                  <div
                    className="font-mono"
                    style={{
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {summarizeQuery(q)}
                  </div>
                </button>
              ))}
              <button
                type="button"
                onClick={() => {
                  onClear()
                  setOpen(false)
                }}
                className="block w-full px-3 py-2 text-left text-xs"
                style={{
                  background: 'transparent',
                  border: 'none',
                  cursor: 'pointer',
                  color: 'var(--color-muted-foreground)',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = 'var(--color-surface-2)')}
                onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
              >
                Clear history
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

// summarizeQuery collapses a multi-line SQL statement into a single line of
// significant text so it fits nicely in a dropdown row. Strips leading
// comments + whitespace so a header comment doesn't drown out the actual
// statement.
function summarizeQuery(sql: string): string {
  const lines = sql.split('\n')
  for (const raw of lines) {
    const line = raw.trim()
    if (!line) continue
    if (line.startsWith('--')) continue
    return line.length > 120 ? line.slice(0, 117) + '…' : line
  }
  // Fallback: collapse whitespace and truncate.
  const collapsed = sql.replace(/\s+/g, ' ').trim()
  return collapsed.length > 120 ? collapsed.slice(0, 117) + '…' : collapsed
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

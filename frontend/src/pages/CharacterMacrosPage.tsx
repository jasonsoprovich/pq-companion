import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import {
  AlertTriangle,
  Keyboard,
  RefreshCw,
  Save,
  Trash2,
  Undo2,
  X,
} from 'lucide-react'
import {
  getAllMacros,
  getTextColors,
  listCharacters,
  updateMacros,
  type Character,
} from '../services/api'
import type { MacroButton, MacroColor, MacroFile } from '../types/zeal'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { ConfirmModal } from '../components/ConfirmModal'

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

const PAGE_COUNT = 10
const BUTTONS_PER_PAGE = 12
const LINE_COUNT = 5

function keyOf(page: number, button: number): number {
  return page * 100 + button
}

function emptyButton(page: number, button: number): MacroButton {
  return { page, button, name: '', color: 0, lines: ['', '', '', '', ''] }
}

function buttonIsEmpty(b: MacroButton): boolean {
  return b.name.trim() === '' && b.lines.every((l) => l.trim() === '')
}

function deepCloneFiles(files: MacroFile[]): MacroFile[] {
  return files.map((f) => ({ ...f, buttons: f.buttons.map((b) => ({ ...b, lines: b.lines.slice() })) }))
}

function fileIsDirty(a: MacroFile | null, b: MacroFile | null): boolean {
  if (!a || !b) return false
  const norm = (f: MacroFile): string =>
    JSON.stringify(
      [...f.buttons]
        .sort((x, y) => keyOf(x.page, x.button) - keyOf(y.page, y.button))
        .map((bt) => [bt.page, bt.button, bt.name, bt.color, bt.lines]),
    )
  return norm(a) !== norm(b)
}

function cssColor(c: MacroColor | undefined): string | null {
  if (!c) return null
  return `rgb(${c.r}, ${c.g}, ${c.b})`
}

// ── Button editor modal ──────────────────────────────────────────────────────

interface ButtonEditorProps {
  initial: MacroButton
  palette: Map<number, MacroColor>
  onSave: (b: MacroButton) => void
  onClear: () => void
  onClose: () => void
}

function ButtonEditor({ initial, palette, onSave, onClear, onClose }: ButtonEditorProps): React.ReactElement {
  useEscapeToClose(onClose)
  const [name, setName] = useState(initial.name)
  const [color, setColor] = useState(initial.color)
  const [lines, setLines] = useState<string[]>(() => {
    const l = initial.lines.slice()
    while (l.length < LINE_COUNT) l.push('')
    return l.slice(0, LINE_COUNT)
  })

  const swatch = cssColor(palette.get(color))
  // Show the palette indices we actually resolved, ordered, as clickable chips.
  const paletteList = useMemo(() => Array.from(palette.values()).sort((a, b) => a.index - b.index), [palette])

  function setLine(i: number, v: string): void {
    setLines((prev) => prev.map((l, idx) => (idx === i ? v : l)))
  }

  function save(): void {
    onSave({ ...initial, name, color, lines: lines.slice() })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}
      onClick={onClose}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-lg flex-col rounded-lg border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex-1 text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            Page {initial.page} · Button {initial.button}
          </div>
          <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }} title="Close">
            <X size={16} />
          </button>
        </div>

        <div className="flex flex-col gap-3 overflow-y-auto p-3">
          {/* Name */}
          <label className="flex flex-col gap-1">
            <span className="text-[11px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Button label
            </span>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="(unnamed)"
              maxLength={64}
              className="rounded px-2 py-1.5 text-sm outline-none"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            />
          </label>

          {/* Color */}
          <div className="flex flex-col gap-1">
            <span className="text-[11px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Label color (EQ palette index — swatch is approximate)
            </span>
            <div className="flex items-center gap-2">
              <span
                className="inline-block rounded"
                style={{
                  width: 22,
                  height: 22,
                  backgroundColor: swatch ?? 'transparent',
                  border: '1px solid var(--color-border)',
                }}
                title={swatch ? `Color #${color}` : `Color #${color} (no swatch)`}
              />
              <input
                type="number"
                value={color}
                min={0}
                max={255}
                onChange={(e) => setColor(Math.max(0, Math.min(255, Math.round(Number(e.target.value) || 0))))}
                className="w-16 rounded px-2 py-1 text-sm outline-none"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              />
            </div>
            {paletteList.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {paletteList.map((c) => (
                  <button
                    key={c.index}
                    onClick={() => setColor(c.index)}
                    title={`Color #${c.index}`}
                    className="rounded"
                    style={{
                      width: 18,
                      height: 18,
                      backgroundColor: `rgb(${c.r}, ${c.g}, ${c.b})`,
                      border: c.index === color ? '2px solid var(--color-primary)' : '1px solid var(--color-border)',
                    }}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Lines */}
          <div className="flex flex-col gap-1">
            <span className="text-[11px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Command lines (run top to bottom)
            </span>
            {lines.map((l, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="w-4 text-right text-[10px] font-mono" style={{ color: 'var(--color-muted)' }}>
                  {i + 1}
                </span>
                <input
                  type="text"
                  value={l}
                  onChange={(e) => setLine(i, e.target.value)}
                  placeholder={i === 0 ? '/say Hello' : ''}
                  className="flex-1 rounded px-2 py-1 font-mono text-xs outline-none"
                  style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                />
              </div>
            ))}
          </div>
        </div>

        <div className="flex items-center gap-2 border-t px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onClear}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ color: 'var(--color-danger, #ef4444)', border: '1px solid var(--color-border)' }}
            title="Clear this button"
          >
            <Trash2 size={12} /> Clear
          </button>
          <div className="flex-1" />
          <button
            onClick={onClose}
            className="rounded px-2 py-1 text-xs"
            style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={save}
            className="rounded px-3 py-1 text-xs font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }}
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Character sub-tabs ───────────────────────────────────────────────────────

function CharacterTabs({
  value,
  onChange,
  characters,
  active,
}: {
  value: string
  onChange: (next: string) => void
  characters: string[]
  active: string
}): React.ReactElement {
  return (
    <div
      className="flex shrink-0 items-center gap-1 overflow-x-auto border-b px-4"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      {characters.map((name) => {
        const isActive = name === value
        const isLogged = name === active
        return (
          <button
            key={name}
            onClick={() => onChange(name)}
            className="whitespace-nowrap px-3 py-2 text-xs font-medium transition-colors"
            style={{
              color: isActive ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              borderBottom: isActive ? '2px solid var(--color-primary)' : '2px solid transparent',
            }}
            title={isLogged ? `${name} (active character)` : name}
          >
            {name}
            {isLogged && (
              <span
                className="ml-1 text-[9px] uppercase tracking-wider"
                style={{ color: isActive ? 'var(--color-primary)' : 'var(--color-muted)' }}
              >
                ●
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────

export default function CharacterMacrosPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [files, setFiles] = useState<MacroFile[]>([])
  const [originalFiles, setOriginalFiles] = useState<MacroFile[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [colors, setColors] = useState<MacroColor[]>([])
  const [viewed, setViewed] = useState('')
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [editing, setEditing] = useState<{ page: number; button: number } | null>(null)
  const [confirmAction, setConfirmAction] = useState<{ type: 'save' } | { type: 'cancel' } | null>(null)

  const palette = useMemo(() => {
    const m = new Map<number, MacroColor>()
    for (const c of colors) m.set(c.index, c)
    return m
  }, [colors])

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    Promise.all([getAllMacros(), listCharacters(), getTextColors()])
      .then(([macros, chars, tc]) => {
        const fresh = macros.characters ?? []
        setFiles(fresh)
        setOriginalFiles(deepCloneFiles(fresh))
        setCharacters(chars.characters ?? [])
        setColors(tc.colors ?? [])
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (files.length === 0) {
      if (viewed !== '') setViewed('')
      return
    }
    const names = files.map((f) => f.character)
    if (viewed && names.includes(viewed)) return
    setViewed(names.find((n) => n === active) ?? names[0])
  }, [files, active, viewed])

  const viewedFile = files.find((f) => f.character === viewed) ?? null
  const viewedChar = characters.find((c) => c.name === viewed) ?? null
  const classIndex = viewedChar?.class ?? -1
  const filenames = useMemo(() => files.map((f) => f.character), [files])
  const originalViewedFile = originalFiles.find((f) => f.character === viewed) ?? null
  const dirty = fileIsDirty(viewedFile, originalViewedFile)

  const buttonsByKey = useMemo(() => {
    const m = new Map<number, MacroButton>()
    if (viewedFile) for (const b of viewedFile.buttons) m.set(keyOf(b.page, b.button), b)
    return m
  }, [viewedFile])

  const mutateViewed = useCallback(
    (fn: (buttons: MacroButton[]) => MacroButton[]) => {
      setFiles((prev) =>
        prev.map((f) =>
          f.character === viewed ? { ...f, buttons: fn(f.buttons.map((b) => ({ ...b, lines: b.lines.slice() }))) } : f,
        ),
      )
    },
    [viewed],
  )

  function handleApply(edited: MacroButton): void {
    mutateViewed((buttons) => {
      const without = buttons.filter((b) => !(b.page === edited.page && b.button === edited.button))
      if (buttonIsEmpty(edited)) return without // emptied → remove
      return [...without, edited]
    })
    setEditing(null)
  }

  function handleClear(): void {
    if (!editing) return
    const { page: p, button } = editing
    mutateViewed((buttons) => buttons.filter((b) => !(b.page === p && b.button === button)))
    setEditing(null)
  }

  function handleSave(): void {
    if (!viewedFile) return
    setSaving(true)
    setError(null)
    updateMacros(viewedFile.character, viewedFile.buttons)
      .then((res) => {
        if (!res.macros) return
        const saved = res.macros
        setFiles((prev) => prev.map((f) => (f.character === saved.character ? saved : f)))
        setOriginalFiles((prev) => {
          const without = prev.filter((f) => f.character !== saved.character)
          return deepCloneFiles([...without, saved])
        })
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => {
        setSaving(false)
        setConfirmAction(null)
      })
  }

  function handleCancel(): void {
    if (!originalViewedFile) return
    const reverted = deepCloneFiles([originalViewedFile])[0]
    setFiles((prev) => prev.map((f) => (f.character === viewed ? reverted : f)))
    setConfirmAction(null)
  }

  const editingButton = editing
    ? buttonsByKey.get(keyOf(editing.page, editing.button)) ?? emptyButton(editing.page, editing.button)
    : null
  const isLoggedIn = viewed !== '' && viewed === active

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      {filenames.length > 0 && (
        <CharacterTabs value={viewed} onChange={setViewed} characters={filenames} active={active} />
      )}

      <div className="flex shrink-0 items-center gap-3 border-b px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
        <Keyboard size={18} style={{ color: 'var(--color-primary)' }} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Macros
          </div>
          {viewedChar && (
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {viewedChar.name} · Level {viewedChar.level} {classIndex >= 0 ? CLASS_NAMES[classIndex] : ''}
            </div>
          )}
        </div>
        {dirty && (
          <span
            className="rounded px-2 py-0.5 text-[11px]"
            style={{ backgroundColor: 'var(--color-warning, #f59e0b)', color: '#000' }}
            title="Unsaved changes to this character's macros"
          >
            Unsaved
          </span>
        )}
        <button
          onClick={() => setConfirmAction({ type: 'cancel' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Discard unsaved changes"
        >
          <Undo2 size={12} /> Cancel
        </button>
        <button
          onClick={() => setConfirmAction({ type: 'save' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium disabled:opacity-40"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }}
          title="Write changes to the .ini file"
        >
          <Save size={12} className={saving ? 'animate-pulse' : ''} /> Save
        </button>
        <button
          onClick={load}
          disabled={saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Reload macro files from disk"
        >
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Refresh
        </button>
      </div>

      {viewedFile && isLoggedIn && (
        <div
          className="flex shrink-0 items-center gap-2 border-b px-4 py-1.5 text-[11px]"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-warning, #f59e0b)' }}
        >
          <AlertTriangle size={12} />
          {viewed} appears to be logged in. EverQuest rewrites this file on logout, so
          saved macro edits will be overwritten — camp out before saving.
        </div>
      )}

      {/* Page selector */}
      {viewedFile && (
        <div
          className="flex shrink-0 items-center gap-1 overflow-x-auto border-b px-4 py-1.5"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {Array.from({ length: PAGE_COUNT }, (_, i) => i + 1).map((p) => {
            const count = viewedFile.buttons.filter((b) => b.page === p).length
            return (
              <button
                key={p}
                onClick={() => setPage(p)}
                className="rounded px-2.5 py-1 text-xs font-medium transition-colors"
                style={{
                  backgroundColor: p === page ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: p === page ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
                title={`Page ${p}${count ? ` (${count} macro${count === 1 ? '' : 's'})` : ''}`}
              >
                {p}
                {count > 0 && p !== page && (
                  <span className="ml-1 text-[9px]" style={{ color: 'var(--color-primary)' }}>
                    ●
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}

      <div className="flex-1 overflow-y-auto px-4 py-4">
        {loading && (
          <div className="flex items-center justify-center py-12">
            <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {!loading && error && (
          <p className="py-12 text-center text-sm" style={{ color: 'var(--color-danger, #ef4444)' }}>
            {error}
          </p>
        )}
        {!loading && !error && filenames.length === 0 && (
          <div className="py-12 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
            No character config files found. <code>&lt;CharName&gt;_pq.proj.ini</code> is created
            by the EverQuest client — log in at least once to generate it.
          </div>
        )}
        {!loading && !error && viewedFile && (
          <div
            style={{
              display: 'grid',
              gap: 10,
              gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))',
            }}
          >
            {Array.from({ length: BUTTONS_PER_PAGE }, (_, i) => i + 1).map((btn) => {
              const b = buttonsByKey.get(keyOf(page, btn))
              const swatch = b ? cssColor(palette.get(b.color)) : null
              const isEmpty = !b || buttonIsEmpty(b)
              return (
                <button
                  key={btn}
                  onClick={() => setEditing({ page, button: btn })}
                  className="flex flex-col gap-1 rounded-lg border p-2 text-left transition-colors hover:bg-(--color-surface-2)"
                  style={{
                    backgroundColor: 'var(--color-surface)',
                    borderColor: 'var(--color-border)',
                    minHeight: 64,
                  }}
                >
                  <div className="flex items-center gap-1.5">
                    <span className="text-[10px] font-mono" style={{ color: 'var(--color-muted)' }}>
                      {btn}
                    </span>
                    {!isEmpty && (
                      <span
                        className="inline-block rounded"
                        style={{ width: 10, height: 10, backgroundColor: swatch ?? 'transparent', border: '1px solid var(--color-border)' }}
                      />
                    )}
                    <span
                      className="truncate text-xs font-medium"
                      style={{ color: isEmpty ? 'var(--color-muted)' : 'var(--color-foreground)' }}
                    >
                      {isEmpty ? 'Empty' : b!.name || '(unnamed)'}
                    </span>
                  </div>
                  {!isEmpty && b!.lines.find((l) => l.trim() !== '') && (
                    <span className="truncate font-mono text-[10px]" style={{ color: 'var(--color-muted)' }}>
                      {b!.lines.find((l) => l.trim() !== '')}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </div>

      {editing && editingButton && (
        <ButtonEditor
          initial={editingButton}
          palette={palette}
          onSave={handleApply}
          onClear={handleClear}
          onClose={() => setEditing(null)}
        />
      )}

      {confirmAction?.type === 'save' && viewedFile && (
        <ConfirmModal
          title="Save macros to disk?"
          confirmLabel="Save"
          onConfirm={handleSave}
          message={
            <p>
              Overwrite the <code>[Socials]</code> section of{' '}
              <code>{viewedFile.character}_pq.proj.ini</code>? Only your macros are
              touched — the rest of the file is left exactly as-is. {viewedFile.character}{' '}
              should be camped out of the game so the client doesn&rsquo;t overwrite the save on logout.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {confirmAction?.type === 'cancel' && viewedFile && (
        <ConfirmModal
          title="Discard changes?"
          confirmLabel="Discard"
          tone="danger"
          onConfirm={handleCancel}
          message={
            <p>
              Revert unsaved changes to <code>{viewedFile.character}</code>'s macros? The .ini
              file on disk is unaffected.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}

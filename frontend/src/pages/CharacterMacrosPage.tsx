import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import {
  AlertTriangle,
  Check,
  Copy,
  Download,
  GripVertical,
  Keyboard,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Trash2,
  Undo2,
  X,
} from 'lucide-react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import {
  getAllMacros,
  getTextColors,
  listCharacters,
  parseMacrosFile,
  updateMacros,
  type Character,
} from '../services/api'
import type { MacroButton, MacroColor, MacroFile } from '../types/zeal'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { ConfirmModal } from '../components/ConfirmModal'
import { BuffPicker } from '../components/BuffPicker'
import { SpellIcon } from '../components/Icon'
import { fetchSpellCached, getCachedSpell } from '../lib/spellCache'

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

const PAGE_COUNT = 10
const BUTTONS_PER_PAGE = 12
const LINE_COUNT = 5

// EverQuest's Actions/Socials window is two columns of six. Buttons number
// down the left column (1–6) then down the right (7–12). To render that in a
// row-major CSS grid we feed the cells column-interleaved: 1,7, 2,8, … 6,12 —
// so each visual row pairs a left-column button with its right-column mate.
const GRID_ROWS = BUTTONS_PER_PAGE / 2
const SLOT_ORDER: number[] = Array.from({ length: GRID_ROWS }, (_, r) => [
  r + 1,
  r + 1 + GRID_ROWS,
]).flat()

// The 12 built-in socials EQ pre-fills page 1 with. They live only in the
// client (never written to the .ini until edited). Socials number down the
// left column then down the right in the in-game Actions window, so 1–6 is
// the left column and 7–12 the right. Names and commands are ground-truthed
// from a client that wrote all 12 defaults to disk after each was touched.
interface Page1Default {
  name: string
  command: string
}
const PAGE1_DEFAULTS: Record<number, Page1Default> = {
  1: { name: 'Afk', command: '/afk' },
  2: { name: 'Anon', command: '/anon' },
  3: { name: 'Split', command: '/split' },
  4: { name: 'Bug', command: '/bug' },
  5: { name: 'Consider', command: '/con' },
  6: { name: 'Duel', command: '/duel' },
  7: { name: 'Feedback', command: '/feedback' },
  8: { name: 'Hail', command: '/hail' },
  9: { name: 'Played', command: '/played' },
  10: { name: 'Time', command: '/time' },
  11: { name: 'GM List', command: '/who all GM' },
  12: { name: 'Wave', command: '/wave' },
}

function keyOf(page: number, button: number): number {
  return page * 100 + button
}

function emptyButton(page: number, button: number): MacroButton {
  return { page, button, name: '', color: 0, lines: ['', '', '', '', ''] }
}

function buttonIsEmpty(b: MacroButton): boolean {
  return b.name.trim() === '' && b.lines.every((l) => l.trim() === '')
}

// EverQuest pre-seeds page 1 with twelve built-in socials and only writes them
// to the .ini once changed. We surface them as ordinary (pre-filled) slots so
// page 1 looks and drags exactly like every other page, then strip any the user
// left untouched (isPage1Default) back out at save time — so an unedited default
// is never actually written to the file, preserving EQ's own behavior.
function page1DefaultButton(button: number): MacroButton {
  const d = PAGE1_DEFAULTS[button]
  return { page: 1, button, name: d.name, color: 0, lines: [d.command, '', '', '', ''] }
}

function withPage1Defaults(file: MacroFile): MacroFile {
  const present = new Set(file.buttons.filter((b) => b.page === 1).map((b) => b.button))
  const buttons = file.buttons.slice()
  for (let n = 1; n <= BUTTONS_PER_PAGE; n++) {
    if (!present.has(n)) buttons.push(page1DefaultButton(n))
  }
  return { ...file, buttons }
}

// True when a page-1 slot still holds its pristine built-in default (matching
// name, color 0, and only the default command). These are dropped at save so
// the file only ever gains defaults the user actually customized or moved.
function isPage1Default(b: MacroButton): boolean {
  if (b.page !== 1) return false
  const d = PAGE1_DEFAULTS[b.button]
  if (!d) return false
  const cmds = b.lines.filter((l) => l.trim() !== '')
  return b.name === d.name && b.color === 0 && cmds.length === 1 && cmds[0] === d.command
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

// ── /cancelbuff builder ──────────────────────────────────────────────────────

// Zeal's /cancelbuff (as of the up-to-5-ids patch) removes beneficial buffs by
// spell id, up to 5 in one line: `/cancelbuff 278 39 145`. The builder below
// lets the user pick those buffs by name instead of hunting down raw ids.
const CANCELBUFF_MAX = 5

// A line is a /cancelbuff command when its first token is /cancelbuff.
function isCancelbuffLine(line: string): boolean {
  return /^\s*\/cancelbuff(\s|$)/i.test(line)
}

// Pull the positive-integer spell ids off a /cancelbuff line — de-duped and
// capped at the 5 Zeal accepts. Non-numeric tokens (a half-typed id) are simply
// ignored rather than fought with, so manual editing never breaks the builder.
function parseCancelbuffIds(line: string): number[] {
  const m = line.match(/^\s*\/cancelbuff\b(.*)$/i)
  if (!m) return []
  const ids: number[] = []
  for (const tok of m[1].trim().split(/\s+/)) {
    if (!tok) continue
    const n = Number(tok)
    if (Number.isInteger(n) && n > 0 && !ids.includes(n)) ids.push(n)
    if (ids.length >= CANCELBUFF_MAX) break
  }
  return ids
}

function serializeCancelbuff(ids: number[]): string {
  return ids.length ? `/cancelbuff ${ids.join(' ')}` : '/cancelbuff'
}

// CancelbuffBuilder renders under any command line that starts with /cancelbuff.
// It shows the referenced spell ids as named chips (icon + name resolved from
// the shared spell cache) and adds new ones via the beneficial-spell BuffPicker,
// rewriting the underlying line as the selection changes.
function CancelbuffBuilder({
  line,
  onChange,
}: {
  line: string
  onChange: (next: string) => void
}): React.ReactElement {
  const [picking, setPicking] = useState(false)
  const [copied, setCopied] = useState(false)
  const [, bump] = useState(0)

  const ids = parseCancelbuffIds(line)
  const idsKey = ids.join(',')
  const full = ids.length >= CANCELBUFF_MAX

  // Resolve ids -> spell (name + icon) from the shared session cache; async
  // misses re-render via bump once they land.
  useEffect(() => {
    const missing = ids.filter((id) => !getCachedSpell(id))
    if (missing.length === 0) return
    let cancelled = false
    Promise.all(missing.map((id) => fetchSpellCached(id).catch(() => null))).then(() => {
      if (!cancelled) bump((v) => v + 1)
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey])

  useEffect(() => {
    if (!copied) return
    const t = window.setTimeout(() => setCopied(false), 1500)
    return () => window.clearTimeout(t)
  }, [copied])

  function addId(id: number): void {
    if (ids.includes(id) || full) return
    onChange(serializeCancelbuff([...ids, id]))
  }
  function removeId(id: number): void {
    onChange(serializeCancelbuff(ids.filter((x) => x !== id)))
  }
  function copy(): void {
    navigator.clipboard
      ?.writeText(serializeCancelbuff(ids))
      .then(() => setCopied(true))
      .catch(() => {})
  }

  return (
    <div
      className="flex flex-col gap-1.5 rounded-md border px-2.5 py-2"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-2)' }}
    >
      <div className="flex items-center gap-2">
        <span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Cancel-buff builder
        </span>
        <span
          className="text-[10px]"
          style={{ color: full ? 'var(--color-warning, #f59e0b)' : 'var(--color-muted)' }}
        >
          {ids.length}/{CANCELBUFF_MAX}
        </span>
        <div className="flex-1" />
        {ids.length > 0 && (
          <button
            onClick={copy}
            className="flex items-center gap-1 text-[10px]"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Copy the /cancelbuff command to the clipboard"
          >
            {copied ? <Check size={11} /> : <Copy size={11} />} {copied ? 'Copied' : 'Copy'}
          </button>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        {ids.map((id) => {
          const s = getCachedSpell(id)
          return (
            <span
              key={id}
              className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px]"
              style={{
                backgroundColor: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            >
              {s && <SpellIcon id={s.new_icon} name={s.name} size={16} />}
              <span className="max-w-[150px] truncate" title={s ? `${s.name} (#${id})` : `Spell #${id}`}>
                {s ? s.name : `Spell #${id}`}
              </span>
              <button onClick={() => removeId(id)} title="Remove" style={{ color: 'var(--color-muted-foreground)' }}>
                <X size={11} />
              </button>
            </span>
          )
        })}
        <button
          onClick={() => setPicking(true)}
          disabled={full}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] disabled:opacity-40"
          style={{ border: '1px dashed var(--color-primary)', color: 'var(--color-primary)' }}
          title={full ? 'Zeal allows at most 5 buffs per /cancelbuff' : 'Search a buff by name to add its spell id'}
        >
          <Plus size={11} /> Add buff
        </button>
      </div>
      {picking && (
        <BuffPicker
          currentSpellId={0}
          existingSpellIDs={ids}
          heading="Add a buff to strip (search by name)"
          alreadyPickedLabel="Already in this command"
          onPick={(s) => {
            addId(s.id)
            setPicking(false)
          }}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  )
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
  // Clickable chips for the choices the in-game color list offers (0–19).
  // Higher indices from eqclient.ini still resolve a swatch, they just aren't
  // offered as new picks.
  const paletteList = useMemo(
    () => Array.from(palette.values()).filter((c) => c.index < 20).sort((a, b) => a.index - b.index),
    [palette],
  )

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
              Label color (EQ social palette index)
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
                max={19}
                onChange={(e) => setColor(Math.max(0, Math.min(19, Math.round(Number(e.target.value) || 0))))}
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
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
              Tip: start a line with <code style={{ color: 'var(--color-foreground)' }}>/cancelbuff</code> to
              build a buff-strip hotkey by name (no spell ids to look up).
            </span>
            {lines.map((l, i) => (
              <div key={i} className="flex flex-col gap-1">
                <div className="flex items-center gap-2">
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
                {isCancelbuffLine(l) && (
                  <div className="ml-6">
                    <CancelbuffBuilder line={l} onChange={(v) => setLine(i, v)} />
                  </div>
                )}
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

// ── Import review modal ──────────────────────────────────────────────────────

interface ImportRow {
  source: MacroButton
  conflict: boolean // target already has a macro at this page/button
  include: boolean
}

interface MacroImportModalProps {
  sourceCharacter: string
  targetCharacter: string
  rows: ImportRow[]
  palette: Map<number, MacroColor>
  onChange: (rows: ImportRow[]) => void
  onConfirm: () => void
  onCancel: () => void
}

function MacroImportModal({
  sourceCharacter,
  targetCharacter,
  rows,
  palette,
  onChange,
  onConfirm,
  onCancel,
}: MacroImportModalProps): React.ReactElement {
  useEscapeToClose(onCancel)
  const includedCount = rows.filter((r) => r.include).length
  const conflictCount = rows.filter((r) => r.conflict).length
  const allIncluded = rows.every((r) => r.include)

  function toggle(i: number): void {
    onChange(rows.map((r, idx) => (idx === i ? { ...r, include: !r.include } : r)))
  }
  function setAll(include: boolean): void {
    onChange(rows.map((r) => ({ ...r, include })))
  }
  function setNewOnly(): void {
    onChange(rows.map((r) => ({ ...r, include: !r.conflict })))
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}
      onClick={onCancel}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-lg border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex items-center gap-2">
            <div className="flex-1 text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
              Import macros{sourceCharacter ? ` from ${sourceCharacter}` : ''} → {targetCharacter}
            </div>
            <button onClick={onCancel} style={{ color: 'var(--color-muted-foreground)' }} title="Close">
              <X size={16} />
            </button>
          </div>
          <p className="mt-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Each macro imports into the same page/button it occupies in the source.
            {conflictCount > 0 && ` ${conflictCount} would overwrite an existing macro (flagged below).`}
          </p>
          <div className="mt-2 flex items-center gap-2">
            <button
              onClick={() => setAll(!allIncluded)}
              className="rounded px-2 py-0.5 text-[11px]"
              style={{ border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)' }}
            >
              {allIncluded ? 'Deselect all' : 'Select all'}
            </button>
            {conflictCount > 0 && (
              <button
                onClick={setNewOnly}
                className="rounded px-2 py-0.5 text-[11px]"
                style={{ border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)' }}
                title="Skip macros that would overwrite an existing one"
              >
                Only non-conflicting
              </button>
            )}
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-2">
          {rows.map((r, i) => {
            const swatch = cssColor(palette.get(r.source.color))
            const firstLine = r.source.lines.find((l) => l.trim() !== '')
            return (
              <label
                key={`${r.source.page}-${r.source.button}`}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface-2)"
              >
                <input type="checkbox" checked={r.include} onChange={() => toggle(i)} />
                <span className="w-12 shrink-0 text-[10px] font-mono" style={{ color: 'var(--color-muted)' }}>
                  P{r.source.page} B{r.source.button}
                </span>
                <span
                  className="inline-block shrink-0 rounded"
                  style={{ width: 10, height: 10, backgroundColor: swatch ?? 'transparent', border: '1px solid var(--color-border)' }}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-xs" style={{ color: 'var(--color-foreground)' }}>
                    {r.source.name || '(unnamed)'}
                  </span>
                  {firstLine && (
                    <span className="block truncate font-mono text-[10px]" style={{ color: 'var(--color-muted)' }}>
                      {firstLine}
                    </span>
                  )}
                </span>
                {r.conflict && (
                  <span
                    className="shrink-0 rounded px-1.5 py-0.5 text-[10px]"
                    style={{ backgroundColor: 'var(--color-warning, #f59e0b)', color: '#000' }}
                    title="Overwrites an existing macro at this position"
                  >
                    overwrites
                  </span>
                )}
              </label>
            )
          })}
        </div>

        <div className="flex items-center gap-2 border-t px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <span className="flex-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            {includedCount} of {rows.length} selected
          </span>
          <button
            onClick={onCancel}
            className="rounded px-2 py-1 text-xs"
            style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={includedCount === 0}
            className="rounded px-3 py-1 text-xs font-medium disabled:opacity-40"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }}
          >
            Import {includedCount > 0 ? includedCount : ''}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Macro grid slot (drag to rearrange) ──────────────────────────────────────

interface MacroSlotProps {
  btn: number
  button: MacroButton | undefined
  palette: Map<number, MacroColor>
  onEdit: () => void
}

// One cell of the 2×6 macro grid. The whole slot is both draggable (when it
// holds a macro) and droppable, so a macro can be dragged onto any slot on the
// page: drop it on another macro to swap the two, or on an empty slot to move
// it there. Drag uses dnd-kit (pointer-based) — native HTML5 drag is unreliable
// in Electron on Windows. A short activation distance lets a plain click still
// open the editor without starting a drag. Page 1's built-in socials arrive as
// ordinary pre-filled buttons, so they render and drag just like any other.
function MacroSlot({ btn, button, palette, onEdit }: MacroSlotProps): React.ReactElement {
  const isEmpty = !button || buttonIsEmpty(button)
  const id = String(btn)
  const {
    setNodeRef: setDragRef,
    attributes,
    listeners,
    isDragging,
  } = useDraggable({ id, disabled: isEmpty })
  const { setNodeRef: setDropRef, isOver, active } = useDroppable({ id })
  const setRef = useCallback(
    (el: HTMLElement | null) => {
      setDragRef(el)
      setDropRef(el)
    },
    [setDragRef, setDropRef],
  )

  const isSource = active != null && String(active.id) === id
  const swapHint = isOver && !isEmpty && !isSource // drop onto a macro → swap
  const moveHint = isOver && isEmpty // drop onto empty → move here

  const swatch = button && !isEmpty ? cssColor(palette.get(button.color)) : null
  const firstLine = button?.lines.find((l) => l.trim() !== '')

  return (
    <div
      ref={setRef}
      onClick={onEdit}
      {...attributes}
      {...listeners}
      className="flex touch-none flex-col gap-1 rounded-lg border p-2 text-left transition-colors hover:bg-(--color-surface-2)"
      style={{
        backgroundColor: 'var(--color-surface)',
        // Border is neutral except while a drag hovers: amber = swap with the
        // macro under the cursor, primary = move into this empty slot.
        borderColor: swapHint
          ? 'var(--color-warning, #f59e0b)'
          : moveHint
            ? 'var(--color-primary)'
            : 'var(--color-border)',
        minHeight: 64,
        cursor: isEmpty ? 'pointer' : 'grab',
        opacity: isDragging ? 0.4 : 1,
      }}
      title={
        isEmpty
          ? undefined
          : 'Click to edit · drag onto another slot to move or swap'
      }
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
          style={{
            color: isEmpty ? 'var(--color-muted)' : 'var(--color-foreground)',
            fontStyle: isEmpty ? 'italic' : 'normal',
          }}
        >
          {isEmpty ? 'Empty' : button!.name || '(unnamed)'}
        </span>
        {!isEmpty && (
          <GripVertical
            size={12}
            className="ml-auto shrink-0"
            style={{ color: 'var(--color-muted)' }}
          />
        )}
      </div>
      {!isEmpty && firstLine && (
        <span className="truncate font-mono text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {firstLine}
        </span>
      )}
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
  const [confirmAction, setConfirmAction] = useState<
    { type: 'save' } | { type: 'cancel' } | { type: 'reset-page1' } | null
  >(null)
  const [importing, setImporting] = useState(false)
  const [importState, setImportState] = useState<{ sourceCharacter: string; rows: ImportRow[] } | null>(null)
  const [draggingBtn, setDraggingBtn] = useState<number | null>(null)

  // Pointer-based drag with a 6px activation distance: a plain click still opens
  // the editor, only a real drag rearranges. dnd-kit (not native HTML5 drag)
  // because native drag is unreliable in Electron on Windows.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
  )

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
        // Pre-seed page 1's built-in socials so it looks and drags like every
        // other page. Untouched defaults are stripped again at save time.
        const fresh = (macros.characters ?? []).map(withPage1Defaults)
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

  // Rearrange within the current page: drop a macro onto an empty slot to move
  // it there, or onto another macro to swap the two. Only the two buttons'
  // slot numbers change — names, colors, and command lines ride along.
  function handleReorder(srcBtn: number, destBtn: number): void {
    if (srcBtn === destBtn) return
    mutateViewed((buttons) => {
      const src = buttons.find((b) => b.page === page && b.button === srcBtn)
      if (!src) return buttons
      const dest = buttons.find((b) => b.page === page && b.button === destBtn)
      return buttons.map((b) => {
        if (b.page === page && b.button === srcBtn) return { ...b, button: destBtn }
        if (dest && b.page === page && b.button === destBtn) return { ...b, button: srcBtn }
        return b
      })
    })
  }

  function handleDragStart(e: DragStartEvent): void {
    setDraggingBtn(Number(e.active.id))
  }

  function handleDragEnd(e: DragEndEvent): void {
    setDraggingBtn(null)
    const { active, over } = e
    if (!over) return
    handleReorder(Number(active.id), Number(over.id))
  }

  // Restore page 1 to EverQuest's twelve built-in socials, replacing whatever
  // is there. Untouched defaults are stripped again at save, so this simply
  // drops any page-1 customizations back to the game's originals.
  function handleResetPage1(): void {
    mutateViewed((buttons) => {
      const others = buttons.filter((b) => b.page !== 1)
      const defaults = Array.from({ length: BUTTONS_PER_PAGE }, (_, i) => page1DefaultButton(i + 1))
      return [...others, ...defaults]
    })
    setConfirmAction(null)
  }

  function handleSave(): void {
    if (!viewedFile) return
    setSaving(true)
    setError(null)
    // Drop page-1 slots still holding their pristine built-in default — EQ
    // supplies those itself, so writing them would only bloat the file. Ones
    // the user edited or moved no longer match and are kept.
    const toSave = viewedFile.buttons.filter((b) => !isPage1Default(b))
    // exported_at is the file's mtime when loaded; the backend refuses the
    // write (409) if the file changed on disk since, so a stale editor never
    // clobbers edits the game client (or anything else) made meanwhile.
    updateMacros(viewedFile.character, toSave, viewedFile.exported_at)
      .then((res) => {
        if (!res.macros) return
        // Re-seed page-1 defaults into the round-tripped file so the editor
        // keeps showing a full page 1 (and stays not-dirty).
        const saved = withPage1Defaults(res.macros)
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

  async function handleImportClick(): Promise<void> {
    if (!viewedFile) return
    setError(null)
    if (!window.electron?.dialog?.openMacrosFile) {
      setError('Import requires the desktop app — not available in browser preview.')
      return
    }
    let path: string | null
    try {
      path = await window.electron.dialog.openMacrosFile()
    } catch (err) {
      setError(`File picker failed: ${(err as Error).message}`)
      return
    }
    if (!path) return

    setImporting(true)
    try {
      const res = await parseMacrosFile(path)
      const source = res.macros
      if (!source || source.buttons.length === 0) {
        setError('Selected file contained no macros.')
        return
      }
      if (source.character && source.character.toLowerCase() === viewedFile.character.toLowerCase()) {
        setError(`That file is ${viewedFile.character}'s own macros — use Refresh to reload from disk.`)
        return
      }
      const existing = new Set(viewedFile.buttons.map((b) => keyOf(b.page, b.button)))
      const rows: ImportRow[] = source.buttons.map((b) => ({
        source: b,
        conflict: existing.has(keyOf(b.page, b.button)),
        include: true,
      }))
      setImportState({ sourceCharacter: source.character || '', rows })
    } catch (err) {
      setError(`Failed to parse macros file: ${(err as Error).message}`)
    } finally {
      setImporting(false)
    }
  }

  function handleConfirmImport(): void {
    if (!importState) return
    const { rows } = importState
    mutateViewed((buttons) => {
      let next = buttons
      for (const r of rows) {
        if (!r.include) continue
        next = next.filter((b) => !(b.page === r.source.page && b.button === r.source.button))
        if (!buttonIsEmpty(r.source)) next.push({ ...r.source, lines: r.source.lines.slice() })
      }
      return next
    })
    setImportState(null)
  }

  const editingButton = editing
    ? buttonsByKey.get(keyOf(editing.page, editing.button)) ?? emptyButton(editing.page, editing.button)
    : null

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
          onClick={handleImportClick}
          disabled={!viewedFile || saving || importing}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Import macros from another character's _pq.proj.ini"
        >
          <Download size={12} className={importing ? 'animate-pulse' : ''} /> Import
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

      {viewedFile && (
        <div
          className="flex shrink-0 items-center gap-2 border-b px-4 py-1.5 text-[11px]"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-warning, #f59e0b)' }}
        >
          <AlertTriangle size={12} className="shrink-0" />
          <span>
            EverQuest loads these socials when {viewed} logs in or zones, and rewrites this file
            when the client saves them (on camp/logout, or when you edit a social in-game). After
            saving here while {viewed} is in-game, <b>zone</b> (or relog) to load the changes.
            Saving while camped out is safest — a save the client makes on camp can overwrite
            edits you haven&rsquo;t loaded yet.
          </span>
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
                {/* Always render the dot so its width is reserved on every
                    tab — colored when the page has macros, transparent
                    otherwise — so tabs don't grow/shrink on click. */}
                <span
                  className="ml-1 text-[9px]"
                  style={{
                    color:
                      count > 0
                        ? p === page
                          ? 'var(--color-background)'
                          : 'var(--color-primary)'
                        : 'transparent',
                  }}
                >
                  ●
                </span>
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
          <>
            <div className="mb-2 flex items-start gap-3">
              <p className="flex-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Laid out like the in-game Actions window (two columns of six). Click a slot to
                edit it, or drag a macro onto another slot to <b>swap</b> the two — or onto an
                empty slot to <b>move</b> it there.
                {page === 1 && (
                  <>
                    {' '}Page 1 starts with EverQuest&rsquo;s built-in socials; a default you never
                    change isn&rsquo;t written to the file.
                  </>
                )}
              </p>
              {page === 1 && (
                <button
                  onClick={() => setConfirmAction({ type: 'reset-page1' })}
                  disabled={saving}
                  className="flex shrink-0 items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
                  style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
                  title="Restore page 1's twelve built-in default socials"
                >
                  <RotateCcw size={12} /> Reset page 1
                </button>
              )}
            </div>
            <DndContext
              sensors={sensors}
              onDragStart={handleDragStart}
              onDragEnd={handleDragEnd}
              onDragCancel={() => setDraggingBtn(null)}
            >
              <div
                style={{
                  display: 'grid',
                  gap: 10,
                  gridTemplateColumns: '1fr 1fr',
                  maxWidth: 560,
                }}
              >
                {SLOT_ORDER.map((btn) => (
                  <MacroSlot
                    key={btn}
                    btn={btn}
                    button={buttonsByKey.get(keyOf(page, btn))}
                    palette={palette}
                    onEdit={() => setEditing({ page, button: btn })}
                  />
                ))}
              </div>
              <DragOverlay>
                {draggingBtn != null && (() => {
                  const b = buttonsByKey.get(keyOf(page, draggingBtn))
                  if (!b) return null
                  const swatch = cssColor(palette.get(b.color))
                  return (
                    <div
                      className="flex items-center gap-1.5 rounded-lg border px-2 py-2 text-xs font-medium shadow-lg"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        borderColor: 'var(--color-primary)',
                        color: 'var(--color-foreground)',
                        cursor: 'grabbing',
                      }}
                    >
                      <span
                        className="inline-block rounded"
                        style={{ width: 10, height: 10, backgroundColor: swatch ?? 'transparent', border: '1px solid var(--color-border)' }}
                      />
                      <span className="truncate">{b.name || '(unnamed)'}</span>
                    </div>
                  )
                })()}
              </DragOverlay>
            </DndContext>
          </>
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
              touched — the rest of the file is left exactly as-is. If {viewedFile.character} is
              in-game, zone or relog afterward to load the changes; saving while camped out is
              safest, so the client doesn&rsquo;t overwrite the save on logout.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {importState && viewedFile && (
        <MacroImportModal
          sourceCharacter={importState.sourceCharacter}
          targetCharacter={viewedFile.character}
          rows={importState.rows}
          palette={palette}
          onChange={(rows) => setImportState((prev) => (prev ? { ...prev, rows } : prev))}
          onConfirm={handleConfirmImport}
          onCancel={() => setImportState(null)}
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

      {confirmAction?.type === 'reset-page1' && viewedFile && (
        <ConfirmModal
          title="Reset page 1 to defaults?"
          confirmLabel="Reset page 1"
          tone="danger"
          onConfirm={handleResetPage1}
          message={
            <p>
              Replace page 1 with EverQuest&rsquo;s twelve built-in socials (Afk, Anon, Hail, …),
              discarding any macros you&rsquo;ve put there? Other pages are untouched, and nothing is
              written to disk until you Save.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}

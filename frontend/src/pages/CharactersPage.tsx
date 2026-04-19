import React, { useCallback, useEffect, useState } from 'react'
import { Users, Plus, Pencil, Trash2, Check, X } from 'lucide-react'
import {
  listCharacters,
  createCharacter,
  updateCharacter,
  deleteCharacter,
  getConfig,
  updateConfig,
  type Character,
} from '../services/api'

const CLASS_LABELS: Record<number, string> = {
  [-1]: 'Not set',
  0: 'WAR — Warrior',
  1: 'CLR — Cleric',
  2: 'PAL — Paladin',
  3: 'RNG — Ranger',
  4: 'SHD — Shadow Knight',
  5: 'DRU — Druid',
  6: 'MNK — Monk',
  7: 'BRD — Bard',
  8: 'ROG — Rogue',
  9: 'SHM — Shaman',
  10: 'NEC — Necromancer',
  11: 'WIZ — Wizard',
  12: 'MAG — Magician',
  13: 'ENC — Enchanter',
  14: 'BST — Beastlord',
}

const CLASS_OPTIONS = Object.entries(CLASS_LABELS).map(([v, label]) => ({
  value: Number(v),
  label,
}))

interface FormState {
  name: string
  class: number
  level: number
}

const EMPTY_FORM: FormState = { name: '', class: -1, level: 1 }

interface CharacterRowProps {
  char: Character
  active: boolean
  onSelect: (c: Character) => void
  onEdit: (c: Character) => void
  onDelete: (c: Character) => void
}

function CharacterRow({ char, active, onSelect, onEdit, onDelete }: CharacterRowProps): React.ReactElement {
  return (
    <div
      className="flex items-center gap-3 rounded-lg px-4 py-3"
      style={{
        backgroundColor: active ? 'color-mix(in srgb, var(--color-primary) 8%, var(--color-surface))' : 'var(--color-surface)',
        border: `1px solid ${active ? 'color-mix(in srgb, var(--color-primary) 40%, transparent)' : 'var(--color-border)'}`,
      }}
    >
      {/* Active indicator */}
      <div
        className="h-2 w-2 shrink-0 rounded-full"
        style={{ backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-3)' }}
        title={active ? 'Active character' : ''}
      />

      {/* Name + details */}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          {char.name}
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {CLASS_LABELS[char.class] ?? 'Unknown'} · Level {char.level}
        </p>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2">
        {!active && (
          <button
            onClick={() => onSelect(char)}
            className="rounded px-2 py-1 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: 'pointer',
            }}
          >
            Select
          </button>
        )}
        <button
          onClick={() => onEdit(char)}
          className="rounded p-1 transition-colors hover:bg-(--color-surface-2)"
          style={{ color: 'var(--color-muted)' }}
          title="Edit"
        >
          <Pencil size={14} />
        </button>
        <button
          onClick={() => onDelete(char)}
          className="rounded p-1 transition-colors hover:bg-(--color-surface-2)"
          style={{ color: 'var(--color-muted)' }}
          title="Delete"
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  )
}

interface CharacterFormProps {
  initial: FormState
  onSave: (f: FormState) => void
  onCancel: () => void
  saving: boolean
  error: string | null
}

function CharacterForm({ initial, onSave, onCancel, saving, error }: CharacterFormProps): React.ReactElement {
  const [form, setForm] = useState<FormState>(initial)

  return (
    <div
      className="rounded-lg p-4 space-y-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center gap-2">
        <input
          type="text"
          placeholder="Character name"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          className="flex-1 rounded px-3 py-2 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            outline: 'none',
          }}
        />
        <input
          type="number"
          min={1}
          max={60}
          value={form.level}
          onChange={(e) => setForm({ ...form, level: Math.max(1, Math.min(60, Number(e.target.value))) })}
          className="w-20 rounded px-3 py-2 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            outline: 'none',
          }}
          placeholder="Lvl"
        />
      </div>
      <select
        value={form.class}
        onChange={(e) => setForm({ ...form, class: Number(e.target.value) })}
        className="w-full rounded px-3 py-2 text-sm"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
          outline: 'none',
        }}
      >
        {CLASS_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
      {error && (
        <p className="text-xs" style={{ color: '#f87171' }}>{error}</p>
      )}
      <div className="flex items-center justify-end gap-2">
        <button
          onClick={onCancel}
          disabled={saving}
          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted-foreground)',
            cursor: saving ? 'not-allowed' : 'pointer',
          }}
        >
          <X size={13} />
          Cancel
        </button>
        <button
          onClick={() => onSave(form)}
          disabled={saving || !form.name.trim()}
          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium"
          style={{
            backgroundColor: 'var(--color-primary)',
            color: '#fff',
            border: 'none',
            cursor: saving || !form.name.trim() ? 'not-allowed' : 'pointer',
            opacity: saving || !form.name.trim() ? 0.6 : 1,
          }}
        >
          <Check size={13} />
          Save
        </button>
      </div>
    </div>
  )
}

type Mode = 'idle' | 'creating' | { editing: Character }

export default function CharactersPage(): React.ReactElement {
  const [characters, setCharacters] = useState<Character[]>([])
  const [activeCharacter, setActiveCharacter] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>('idle')
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)

  const load = useCallback(() => {
    return listCharacters()
      .then((resp) => {
        setCharacters(resp.characters)
        setActiveCharacter(resp.active)
      })
      .catch((err: Error) => setLoadError(err.message))
  }, [])

  useEffect(() => {
    setLoading(true)
    load().finally(() => setLoading(false))
  }, [load])

  async function handleSelect(char: Character) {
    try {
      const cfg = await getConfig()
      await updateConfig({ ...cfg, character: char.name, character_class: char.class })
      setActiveCharacter(char.name)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to select character'
      setLoadError(msg)
    }
  }

  async function handleSave(form: FormState) {
    if (!form.name.trim()) return
    setSaving(true)
    setFormError(null)
    try {
      if (mode === 'creating') {
        await createCharacter({ name: form.name.trim(), class: form.class, level: form.level })
      } else if (typeof mode === 'object') {
        await updateCharacter(mode.editing.id, { name: form.name.trim(), class: form.class, level: form.level })
      }
      setMode('idle')
      await load()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to save character'
      setFormError(msg)
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(char: Character) {
    if (deleteConfirm !== char.id) {
      setDeleteConfirm(char.id)
      return
    }
    setDeleteConfirm(null)
    try {
      await deleteCharacter(char.id)
      if (activeCharacter === char.name) {
        const cfg = await getConfig()
        await updateConfig({ ...cfg, character: '', character_class: -1 })
        setActiveCharacter('')
      }
      await load()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to delete character'
      setLoadError(msg)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-auto p-6">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Users size={20} style={{ color: 'var(--color-primary)' }} />
          <div>
            <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
              Characters
            </h1>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Manage your EverQuest characters. Select one to activate its log file.
            </p>
          </div>
        </div>
        {mode === 'idle' && (
          <button
            onClick={() => { setMode('creating'); setFormError(null) }}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: 'pointer',
            }}
          >
            <Plus size={14} />
            Add Character
          </button>
        )}
      </div>

      {loadError && (
        <div
          className="mb-4 rounded px-4 py-3 text-sm"
          style={{ backgroundColor: 'color-mix(in srgb, #f87171 12%, transparent)', border: '1px solid color-mix(in srgb, #f87171 30%, transparent)', color: '#f87171' }}
        >
          {loadError}
        </div>
      )}

      {mode === 'creating' && (
        <div className="mb-4">
          <CharacterForm
            initial={EMPTY_FORM}
            onSave={handleSave}
            onCancel={() => { setMode('idle'); setFormError(null) }}
            saving={saving}
            error={formError}
          />
        </div>
      )}

      {loading ? (
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
      ) : characters.length === 0 && mode !== 'creating' ? (
        <div
          className="flex flex-col items-center justify-center rounded-lg py-12 text-center"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <Users size={32} style={{ color: 'var(--color-muted)', marginBottom: '12px' }} />
          <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>No characters yet</p>
          <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Add a character to get started.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {characters.map((char) => (
            <React.Fragment key={char.id}>
              {typeof mode === 'object' && mode.editing.id === char.id ? (
                <CharacterForm
                  initial={{ name: char.name, class: char.class, level: char.level }}
                  onSave={handleSave}
                  onCancel={() => { setMode('idle'); setFormError(null) }}
                  saving={saving}
                  error={formError}
                />
              ) : (
                <div>
                  <CharacterRow
                    char={char}
                    active={activeCharacter === char.name}
                    onSelect={handleSelect}
                    onEdit={(c) => { setMode({ editing: c }); setFormError(null) }}
                    onDelete={handleDelete}
                  />
                  {deleteConfirm === char.id && (
                    <div
                      className="mt-1 flex items-center justify-between rounded px-4 py-2 text-xs"
                      style={{
                        backgroundColor: 'color-mix(in srgb, #f87171 10%, var(--color-surface))',
                        border: '1px solid color-mix(in srgb, #f87171 30%, transparent)',
                      }}
                    >
                      <span style={{ color: 'var(--color-foreground)' }}>Delete {char.name}?</span>
                      <div className="flex gap-2">
                        <button
                          onClick={() => setDeleteConfirm(null)}
                          className="rounded px-2 py-0.5"
                          style={{
                            backgroundColor: 'var(--color-surface-2)',
                            border: '1px solid var(--color-border)',
                            color: 'var(--color-muted-foreground)',
                            cursor: 'pointer',
                          }}
                        >
                          Cancel
                        </button>
                        <button
                          onClick={() => handleDelete(char)}
                          className="rounded px-2 py-0.5 font-medium"
                          style={{ backgroundColor: '#ef4444', color: '#fff', border: 'none', cursor: 'pointer' }}
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </React.Fragment>
          ))}
        </div>
      )}
    </div>
  )
}

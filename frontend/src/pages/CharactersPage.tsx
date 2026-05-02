import React, { useCallback, useEffect, useState } from 'react'
import { Users, Plus, Trash2, Check, X, Radar } from 'lucide-react'
import {
  listCharacters,
  createCharacter,
  deleteCharacter,
  discoverCharacters,
  getConfig,
  updateConfig,
  type Character,
} from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'

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

const RACE_LABELS: Record<number, string> = {
  [-1]: 'Not set',
  1: 'Human',
  2: 'Barbarian',
  3: 'Erudite',
  4: 'Wood Elf',
  5: 'High Elf',
  6: 'Dark Elf',
  7: 'Half Elf',
  8: 'Dwarf',
  9: 'Troll',
  10: 'Ogre',
  11: 'Halfling',
  12: 'Gnome',
  13: 'Iksar',
  14: 'Vah Shir',
}

const CLASS_OPTIONS = Object.entries(CLASS_LABELS).map(([v, label]) => ({ value: Number(v), label }))
const RACE_OPTIONS = Object.entries(RACE_LABELS).map(([v, label]) => ({ value: Number(v), label }))

interface FormState {
  name: string
  class: number
  race: number
  level: number
}

const EMPTY_FORM: FormState = { name: '', class: -1, race: -1, level: 1 }

interface CharacterRowProps {
  char: Character
  active: boolean
  onSelect: (c: Character) => void
  onDelete: (c: Character) => void
}

function CharacterRow({ char, active, onSelect, onDelete }: CharacterRowProps): React.ReactElement {
  const raceLabel = RACE_LABELS[char.race] ?? 'Unknown'
  const classLabel = CLASS_LABELS[char.class] ?? 'Unknown'
  const details = [
    char.race >= 0 ? raceLabel : null,
    char.class >= 0 ? classLabel.split(' — ')[0] : null,
    `Level ${char.level}`,
  ].filter(Boolean).join(' · ')

  return (
    <div
      className="flex items-center gap-3 rounded-lg px-4 py-3"
      style={{
        backgroundColor: active
          ? 'color-mix(in srgb, var(--color-primary) 8%, var(--color-surface))'
          : 'var(--color-surface)',
        border: `1px solid ${active ? 'color-mix(in srgb, var(--color-primary) 40%, transparent)' : 'var(--color-border)'}`,
      }}
    >
      <div
        className="h-2 w-2 shrink-0 rounded-full"
        style={{ backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-3)' }}
        title={active ? 'Active character' : ''}
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          {char.name}
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {details}
        </p>
      </div>
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
          onChange={(e) =>
            setForm({ ...form, level: Math.max(1, Math.min(60, Number(e.target.value))) })
          }
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
      <div className="flex gap-2">
        <select
          value={form.class}
          onChange={(e) => setForm({ ...form, class: Number(e.target.value) })}
          className="flex-1 rounded px-3 py-2 text-sm"
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
        <select
          value={form.race}
          onChange={(e) => setForm({ ...form, race: Number(e.target.value) })}
          className="flex-1 rounded px-3 py-2 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            outline: 'none',
          }}
        >
          {RACE_OPTIONS.map(({ value, label }) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>
      </div>
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

type Mode = 'idle' | 'creating'

export default function CharactersPage(): React.ReactElement {
  const { active: activeCharacter, setActive } = useActiveCharacter()
  const [characters, setCharacters] = useState<Character[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>('idle')
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)
  const [discovered, setDiscovered] = useState<string[] | null>(null)
  const [discovering, setDiscovering] = useState(false)

  const load = useCallback(() => {
    return listCharacters()
      .then((resp) => {
        setCharacters(resp.characters)
        setActive(resp.active, resp.manual)
      })
      .catch((err: Error) => setLoadError(err.message))
  }, [setActive])

  useEffect(() => {
    setLoading(true)
    load().finally(() => setLoading(false))
  }, [load])

  async function handleSelect(char: Character) {
    try {
      const cfg = await getConfig()
      await updateConfig({ ...cfg, character: char.name, character_class: char.class })
      setActive(char.name, true)
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : 'Failed to select character')
    }
  }

  async function handleSave(form: FormState) {
    if (!form.name.trim()) return
    setSaving(true)
    setFormError(null)
    try {
      await createCharacter({ name: form.name.trim(), class: form.class, race: form.race, level: form.level })
      setMode('idle')
      setDiscovered(null)
      await load()
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : 'Failed to save character')
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
        setActive('', false)
      }
      await load()
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : 'Failed to delete character')
    }
  }

  async function handleDiscover() {
    setDiscovering(true)
    try {
      const res = await discoverCharacters()
      setDiscovered(res.names)
    } catch {
      setDiscovered([])
    } finally {
      setDiscovering(false)
    }
  }

  async function handleImport(name: string) {
    try {
      await createCharacter({ name, class: -1, race: -1, level: 1 })
      setDiscovered((prev) => prev?.filter((n) => n !== name) ?? null)
      await load()
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : 'Failed to import character')
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
        <div className="flex items-center gap-2">
          <button
            onClick={handleDiscover}
            disabled={discovering}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: discovering ? 'not-allowed' : 'pointer',
              opacity: discovering ? 0.6 : 1,
            }}
            title="Scan EQ directory for characters not yet tracked"
          >
            <Radar size={14} />
            {discovering ? 'Scanning…' : 'Discover'}
          </button>
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
      </div>

      {loadError && (
        <div
          className="mb-4 rounded px-4 py-3 text-sm"
          style={{
            backgroundColor: 'color-mix(in srgb, #f87171 12%, transparent)',
            border: '1px solid color-mix(in srgb, #f87171 30%, transparent)',
            color: '#f87171',
          }}
        >
          {loadError}
        </div>
      )}

      {/* Discover results */}
      {discovered !== null && (
        <div
          className="mb-4 rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Found in EQ log directory
            </p>
            <button
              onClick={() => setDiscovered(null)}
              className="rounded p-0.5 hover:bg-(--color-surface-2)"
              style={{ color: 'var(--color-muted)' }}
            >
              <X size={13} />
            </button>
          </div>
          {discovered.length === 0 ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No untracked characters found. Make sure the EQ path is set in Settings.
            </p>
          ) : (
            <div className="space-y-1">
              {discovered.map((name) => (
                <div
                  key={name}
                  className="flex items-center justify-between rounded px-3 py-1.5"
                  style={{ backgroundColor: 'var(--color-surface-2)' }}
                >
                  <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>{name}</span>
                  <button
                    onClick={() => handleImport(name)}
                    className="flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium"
                    style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: 'none', cursor: 'pointer' }}
                  >
                    <Plus size={11} />
                    Import
                  </button>
                </div>
              ))}
            </div>
          )}
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
            Add a character or click Discover to import from your EQ log directory.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {characters.map((char) => (
            <div key={char.id}>
              <CharacterRow
                char={char}
                active={activeCharacter === char.name}
                onSelect={handleSelect}
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
          ))}
        </div>
      )}
    </div>
  )
}

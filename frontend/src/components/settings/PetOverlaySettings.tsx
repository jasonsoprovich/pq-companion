/**
 * Settings → Pet: the Pet overlay's general options, its three alerts
 * (charm expiring, low HP, buff missing) and the active character's buff
 * watch lists, beside a live preview of the panel. The alerts are evaluated
 * by the backend pet tracker and fire through the normal trigger-alert
 * plumbing; AlertActionFields supplies their overlay/TTS/sound sections.
 */
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, Plus, Search, X } from 'lucide-react'
import AlertActionFields from '../AlertActionFields'
import PetView from '../overlays/PetView'
import { SpellIcon } from '../Icon'
import { useVoices } from '../../hooks/useVoices'
import { useTTSVoices } from '../../hooks/usePiperStatus'
import { usePetState, usePetTimers } from '../../hooks/usePetState'
import { listCharacters, searchSpells } from '../../services/api'
import type { PetAlertSettings, PetOverlaySettings, PetWatchList } from '../../types/config'
import type { Spell } from '../../types/spell'

interface Props {
  settings: PetOverlaySettings
  onChange: (next: PetOverlaySettings) => void
}

const muted = { color: 'var(--color-muted-foreground)' }
const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

// Defaults mirror the backend's config.DefaultPet* constants.
const DEFAULT_SWAP_DELAY = 10
const DEFAULT_FINDER_ROWS = 8

function Card({ title, hint, children }: { title: string; hint: React.ReactNode; children: React.ReactNode }): React.ReactElement {
  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2 className="mb-1 text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        {title}
      </h2>
      <p className="mb-4 text-xs" style={muted}>{hint}</p>
      {children}
    </section>
  )
}

function NumberRow({
  label,
  hint,
  value,
  placeholder,
  min,
  max,
  unit,
  onChange,
}: {
  label: string
  hint?: string
  value: number | undefined
  placeholder: number
  min: number
  max: number
  unit?: string
  onChange: (n: number) => void
}): React.ReactElement {
  return (
    <label className="flex items-center justify-between gap-4 py-1.5">
      <span className="flex flex-col">
        <span className="text-sm">{label}</span>
        {hint && <span className="text-[11px]" style={muted}>{hint}</span>}
      </span>
      <span className="flex items-center gap-1.5">
        <input
          type="text"
          inputMode="numeric"
          value={value ? String(value) : ''}
          placeholder={String(placeholder)}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10)
            // 0 = use the default (the backend's omitempty convention).
            onChange(Number.isFinite(n) ? Math.max(min, Math.min(max, n)) : 0)
          }}
          className="w-16 rounded px-2 py-1 text-right text-sm"
          style={inputStyle}
        />
        {unit && <span className="text-xs" style={muted}>{unit}</span>}
      </span>
    </label>
  )
}

export function PetGeneralCard({ settings, onChange }: Props): React.ReactElement {
  return (
    <Card
      title="Pet Overlay"
      hint="How the Pet overlay behaves when a charm breaks, and how much the charm finder lists."
    >
      <NumberRow
        label="Finder swap delay"
        hint="How long the broken-charm card stays up before the finder returns."
        value={settings.swap_delay_secs}
        placeholder={DEFAULT_SWAP_DELAY}
        min={1}
        max={120}
        unit="s"
        onChange={(n) => onChange({ ...settings, swap_delay_secs: n })}
      />
      <NumberRow
        label="Finder rows"
        hint="Pets the compact finder lists before linking to the full Charm Pet Finder."
        value={settings.finder_rows}
        placeholder={DEFAULT_FINDER_ROWS}
        min={3}
        max={30}
        onChange={(n) => onChange({ ...settings, finder_rows: n })}
      />
    </Card>
  )
}

interface AlertDef {
  key: 'charm_expiring' | 'low_hp' | 'buff_missing'
  label: string
  hint: string
  tokens: string
  defaultTemplate: string
  threshold?: { label: string; unit: string; placeholder: number; min: number; max: number }
}

const ALERTS: AlertDef[] = [
  {
    key: 'charm_expiring',
    label: 'Charm expiring',
    hint: 'Once per charm, when its timer runs low. Recasting re-arms it.',
    tokens: '{pet} {spell} {secs}',
    defaultTemplate: 'Charm on {pet} breaks in {secs}s',
    threshold: { label: 'Warn at', unit: 's left', placeholder: 30, min: 5, max: 600 },
  },
  {
    key: 'low_hp',
    label: 'Pet low HP',
    hint: 'When your pet drops to this HP. Re-arms once it heals 10 points above.',
    tokens: '{pet} {hp}',
    defaultTemplate: '{pet} at {hp}% HP',
    threshold: { label: 'Warn at', unit: '% HP', placeholder: 30, min: 1, max: 95 },
  },
  {
    key: 'buff_missing',
    label: 'Buff missing',
    hint:
      'When a spell on your watch list (below) is neither timed on your pet nor in a /pet health ' +
      'reply from the last minute. Waits 20s after a new pet so you have time to buff it.',
    tokens: '{pet} {spells}',
    defaultTemplate: '{pet} is missing {spells}',
  },
]

export function PetAlertsCard({ settings, onChange }: Props): React.ReactElement {
  const voices = useTTSVoices(useVoices())
  const [open, setOpen] = useState<string | null>(null)

  return (
    <Card
      title="Pet Alerts"
      hint="Fire like trigger alerts — overlay text, text-to-speech and/or a sound — through your normal alert output."
    >
      <div className="flex flex-col gap-2">
        {ALERTS.map((def) => {
          const prefs: PetAlertSettings = settings[def.key] ?? {}
          const patch = (p: Partial<PetAlertSettings>): void =>
            onChange({ ...settings, [def.key]: { ...prefs, ...p } })
          const isOpen = open === def.key
          return (
            <div key={def.key} className="rounded" style={{ border: '1px solid var(--color-border)' }}>
              <div className="flex items-center gap-2 px-3 py-2">
                <button
                  type="button"
                  onClick={() => setOpen(isOpen ? null : def.key)}
                  className="flex flex-1 items-center gap-1.5 text-left"
                >
                  {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                  <span className="text-sm font-medium">{def.label}</span>
                  {prefs.enabled && (
                    <span className="text-[10px] font-semibold uppercase" style={{ color: 'var(--color-primary)' }}>
                      On
                    </span>
                  )}
                </button>
                <label className="flex items-center gap-1.5 text-xs" style={muted}>
                  <input
                    type="checkbox"
                    checked={prefs.enabled ?? false}
                    onChange={(e) => patch({ enabled: e.target.checked })}
                  />
                  Enabled
                </label>
              </div>
              {isOpen && (
                <div className="space-y-3 px-3 pb-3">
                  <p className="text-[11px]" style={muted}>{def.hint}</p>
                  {def.threshold && (
                    <label className="flex items-center gap-2 text-xs">
                      <span style={muted}>{def.threshold.label}</span>
                      <input
                        type="text"
                        inputMode="numeric"
                        value={prefs.threshold ? String(prefs.threshold) : ''}
                        placeholder={String(def.threshold.placeholder)}
                        onChange={(e) => {
                          const n = parseInt(e.target.value, 10)
                          const t = def.threshold!
                          patch({ threshold: Number.isFinite(n) ? Math.max(t.min, Math.min(t.max, n)) : 0 })
                        }}
                        className="w-16 rounded px-2 py-1 text-right"
                        style={inputStyle}
                      />
                      <span style={muted}>{def.threshold.unit}</span>
                    </label>
                  )}
                  <label className="flex flex-col gap-1 text-xs">
                    <span style={muted}>
                      Alert text <span className="font-mono">{def.tokens}</span>
                    </span>
                    <input
                      type="text"
                      value={prefs.template ?? ''}
                      placeholder={def.defaultTemplate}
                      onChange={(e) => patch({ template: e.target.value })}
                      className="rounded px-2 py-1 text-sm"
                      style={inputStyle}
                    />
                  </label>
                  <AlertActionFields prefs={prefs} patch={patch} voices={voices} />
                </div>
              )}
            </div>
          )
        })}
      </div>
    </Card>
  )
}

// SpellPicker adds a spell by name: typing searches quarm.db; Enter adds the
// highlighted match, or the typed text as-is when nothing matches.
function SpellPicker({ onAdd }: { onAdd: (name: string) => void }): React.ReactElement {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<Spell[]>([])
  const seq = useRef(0)

  useEffect(() => {
    const term = q.trim()
    if (term.length < 2) {
      setResults([])
      return
    }
    const mine = ++seq.current
    const t = setTimeout(() => {
      searchSpells(term, 6, 0, -1, 0, 0, false, { beneficial: true })
        .then((r) => { if (mine === seq.current) setResults(r.items) })
        .catch(() => {})
    }, 150)
    return () => clearTimeout(t)
  }, [q])

  const add = (name: string): void => {
    if (!name.trim()) return
    onAdd(name.trim())
    setQ('')
    setResults([])
  }

  return (
    <div className="relative">
      <div className="relative">
        <Search size={12} className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2" style={muted} />
        <input
          type="text"
          value={q}
          placeholder="Add a spell…"
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') add(results[0]?.name ?? q)
            if (e.key === 'Escape') setResults([])
          }}
          className="w-full rounded py-1 pl-7 pr-2 text-xs"
          style={inputStyle}
        />
      </div>
      {results.length > 0 && (
        <div
          className="absolute left-0 right-0 top-full z-10 mt-1 overflow-hidden rounded shadow-lg"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        >
          {results.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => add(s.name)}
              className="flex w-full items-center gap-2 px-2 py-1 text-left text-xs hover:bg-(--color-surface-3)"
            >
              <SpellIcon id={s.new_icon} name={s.name} size={14} />
              {s.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

const fold = (n: string): string => n.replace(/`/g, "'").toLowerCase().trim()

function WatchColumn({
  title,
  spells,
  suggestions,
  onChange,
}: {
  title: string
  spells: string[]
  suggestions: string[]
  onChange: (next: string[]) => void
}): React.ReactElement {
  const have = new Set(spells.map(fold))
  const add = (name: string): void => {
    if (!have.has(fold(name))) onChange([...spells, name])
  }
  const offer = suggestions.filter((s) => !have.has(fold(s)))
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2">
      <span className="text-xs font-semibold" style={{ color: 'var(--color-muted)' }}>{title}</span>
      {spells.length === 0 ? (
        <span className="text-[11px]" style={muted}>Nothing watched.</span>
      ) : (
        <div className="flex flex-wrap gap-1">
          {spells.map((s) => (
            <span
              key={s}
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
            >
              {s}
              <button
                type="button"
                aria-label={`Remove ${s}`}
                onClick={() => onChange(spells.filter((x) => x !== s))}
                style={muted}
              >
                <X size={11} />
              </button>
            </span>
          ))}
        </div>
      )}
      <SpellPicker onAdd={add} />
      {offer.length > 0 && (
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-[11px]" style={muted}>On your pet now:</span>
          {offer.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => add(s)}
              className="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[11px]"
              style={{ border: '1px dashed var(--color-border)', color: 'var(--color-foreground)' }}
            >
              <Plus size={10} />
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export function PetWatchListCard({ settings, onChange }: Props): React.ReactElement {
  const [character, setCharacter] = useState('')
  useEffect(() => {
    listCharacters().then((r) => setCharacter(r.active)).catch(() => {})
  }, [])

  // Quick-add: the beneficial effects on the current pet (timed or listed by
  // its latest /pet health), offered for the list matching its kind.
  const pet = usePetState()
  const { timers } = usePetTimers(pet.name)
  const current = useMemo(() => {
    const names = new Map<string, string>()
    for (const t of timers) if (t.category === 'buff') names.set(fold(t.spell_name), t.spell_name)
    for (const s of pet.health?.spells ?? []) {
      if (s.beneficial && !s.charm) names.set(fold(s.name), s.name)
    }
    return [...names.values()].sort()
  }, [timers, pet.health])

  if (!character) {
    return (
      <Card title="Buff Watch Lists" hint="Watch lists are per character — no active character yet.">
        <span />
      </Card>
    )
  }

  const key = character.toLowerCase()
  const list: PetWatchList = settings.watch_lists?.[key] ?? {}
  const setList = (next: PetWatchList): void =>
    onChange({ ...settings, watch_lists: { ...(settings.watch_lists ?? {}), [key]: next } })

  return (
    <Card
      title="Buff Watch Lists"
      hint={
        <>
          Spells <strong>{character}</strong> keeps on its pets, for the Buff missing alert.
          Charmed and summoned pets have separate lists.
        </>
      }
    >
      <div className="flex flex-col gap-4 sm:flex-row">
        <WatchColumn
          title="Charmed pets"
          spells={list.charm ?? []}
          suggestions={pet.mode === 'pet' && pet.kind !== 'summoned' ? current : []}
          onChange={(charm) => setList({ ...list, charm })}
        />
        <WatchColumn
          title="Summoned pets"
          spells={list.summoned ?? []}
          suggestions={pet.mode === 'pet' && pet.kind === 'summoned' ? current : []}
          onChange={(summoned) => setList({ ...list, summoned })}
        />
      </div>
    </Card>
  )
}

// PetPreview renders the live Pet panel, so changes show against whatever
// the current pet (or finder) is.
export function PetPreview(): React.ReactElement {
  const state = usePetState()
  return (
    <div className="flex flex-col gap-2">
      <span className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        Live preview
      </span>
      <div
        className="overflow-hidden rounded-lg"
        style={{ height: 460, backgroundColor: 'rgba(10,10,12,0.92)', border: '1px solid var(--color-border)' }}
      >
        <PetView state={state} />
      </div>
    </div>
  )
}

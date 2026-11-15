import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RotateCcw, Search, Shirt, X } from 'lucide-react'
import {
  getCharacterAppearance,
  listCharacters,
  searchWearables,
  setCharacterAppearance,
  type Character,
  type ItemAppearance,
} from '../services/api'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useCachedState } from '../hooks/useCachedState'
import { charRaceLabel } from '../lib/enumsCache'
import type { DressInfo } from '../components/models/ModelViewer'
import { DRESS_SLOTS, MONK_CLASS, dressFor, loadEquippedItems } from '../lib/characterDress'
import type { DressSlot, ModelDress } from '../lib/characterDress'

const ModelViewer = lazy(() => import('../components/models/ModelViewer'))

// The Dressing Room: your character's model from your own EverQuest
// client, in the gear from their last Zeal inventory export, with any other
// item tried on in its place. The game logs neither gender nor face, so
// gender is picked here and saved per character.

type SlotDef = DressSlot
const SLOTS = DRESS_SLOTS

// Try-ons per slot: an item, or null for nothing; a slot not in the map
// shows what's equipped.
type TryOns = Record<string, ItemAppearance | null>

export default function DressingRoomPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewed, setViewed] = useCachedState('dressingRoom.character', active)
  const [characters, setCharacters] = useState<Character[]>([])
  const [gender, setGender] = useState(0)
  const [luclin, setLuclin] = useState(false)
  const [hair, setHair] = useState(0)
  const [equipped, setEquipped] = useState<Record<string, ItemAppearance>>({})
  const [exported, setExported] = useState<string | null>(null)
  const [tryOns, setTryOns] = useState<TryOns>({})
  const [picking, setPicking] = useState<string | null>(null)
  const [info, setInfo] = useState<DressInfo | null>(null)

  useEffect(() => {
    listCharacters().then((r) => setCharacters(r.characters)).catch(() => setCharacters([]))
  }, [])
  const character = characters.find((c) => c.name === viewed)

  // Saved gender and model era, and the equipped gear, per character.
  useEffect(() => {
    if (!viewed) return
    let live = true
    setTryOns({})
    setPicking(null)
    setEquipped({})
    setExported(null)
    getCharacterAppearance(viewed)
      .then((a) => {
        if (!live) return
        setGender(a.gender)
        setLuclin(a.luclin)
        setHair(a.hair ?? 0)
      })
      .catch(() => {})
    loadEquippedItems(viewed)
      .then((eq) => {
        if (!live) return
        setExported(eq.exportedAt)
        setEquipped(eq.items)
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [viewed])

  const saveAppearance = useCallback((next: { gender?: number; luclin?: boolean; hair?: number }) => {
    if (!viewed) return
    const g = next.gender ?? gender
    const l = next.luclin ?? luclin
    const h = next.hair ?? hair
    setGender(g)
    setLuclin(l)
    setHair(h)
    setCharacterAppearance({ name: viewed, gender: g, luclin: l, hair: h }).catch(() => {})
  }, [viewed, gender, luclin, hair])

  const worn = useCallback(
    (key: string): ItemAppearance | null => (key in tryOns ? tryOns[key] : equipped[key] ?? null),
    [tryOns, equipped],
  )

  const dress = useMemo(
    (): ModelDress => ({ ...dressFor(worn, character?.class === MONK_CLASS), hair }),
    [worn, character?.class, hair],
  )

  const race = character?.race ?? -1
  const tried = Object.keys(tryOns).length

  return (
    <div className="flex h-full flex-col">
      <CharacterSubTabs value={viewed} onChange={setViewed} />
      <div className="flex min-h-0 flex-1 gap-4 p-4">
        <div className="flex min-w-0 flex-1 flex-col rounded-lg"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
          <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2" style={{ borderColor: 'var(--color-border)' }}>
            <Shirt size={18} style={{ color: 'var(--color-primary)' }} />
            <div className="mr-auto">
              <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Dressing Room</h1>
              <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                {character && race > 0
                  ? `${character.name} · ${charRaceLabel(race)}`
                  : 'Your character, drawn from your own EverQuest client.'}
              </p>
            </div>
            <Toggle
              options={[{ value: 0, label: 'Male' }, { value: 1, label: 'Female' }]}
              value={gender}
              onChange={(g) => saveAppearance({ gender: g })}
            />
            <Toggle
              options={[{ value: 0, label: 'Original' }, { value: 1, label: 'Luclin' }]}
              value={luclin ? 1 : 0}
              onChange={(v) => saveAppearance({ luclin: v === 1 })}
            />
            {luclin && (
              <Toggle
                options={[
                  { value: -1, label: 'Bald' }, { value: 0, label: 'Hair 1' },
                  { value: 1, label: 'Hair 2' }, { value: 2, label: 'Hair 3' },
                ]}
                value={hair}
                onChange={(h) => saveAppearance({ hair: h })}
              />
            )}
          </div>
          <div className="relative min-h-0 flex-1">
            {!viewed || race <= 0 ? (
              <div className="flex h-full items-center justify-center p-6 text-center text-xs" style={{ color: 'var(--color-muted)' }}>
                {viewed
                  ? 'Set this character’s race on the Active Character page to see them here.'
                  : 'Pick a character.'}
              </div>
            ) : (
              <Suspense fallback={null}>
                <ModelViewer
                  className="h-full w-full"
                  model={{ race, gender, luclin }}
                  dress={dress}
                  onDressed={setInfo}
                />
              </Suspense>
            )}
          </div>
          <DressNotes info={info} luclin={luclin} hair={hair} />
        </div>

        <div className="flex w-80 shrink-0 flex-col gap-3 overflow-y-auto">
          <div className="rounded-lg"
            style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
            <div className="flex items-center justify-between border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
              <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Gear</span>
              <button
                type="button"
                disabled={tried === 0}
                onClick={() => { setTryOns({}); setPicking(null) }}
                className="flex items-center gap-1 text-[11px] disabled:opacity-40"
                style={{ color: 'var(--color-muted-foreground)' }}
                title="Back to what's equipped"
              >
                <RotateCcw size={12} /> Equipped
              </button>
            </div>
            <ul>
              {SLOTS.map((s) => {
                const item = worn(s.key)
                const changed = s.key in tryOns
                return (
                  <li key={s.key}>
                    <div
                      className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs hover:bg-white/5"
                      style={picking === s.key ? { backgroundColor: 'var(--color-surface-2)' } : undefined}
                      onClick={() => setPicking(picking === s.key ? null : s.key)}
                    >
                      <span className="w-16 shrink-0" style={{ color: 'var(--color-muted)' }}>{s.label}</span>
                      {item ? <ItemIcon id={item.icon} name={item.name} size={18} /> : <span className="inline-block w-[18px]" />}
                      <span
                        className="min-w-0 flex-1 truncate"
                        style={{ color: changed ? 'var(--color-warning, #f59e0b)' : item ? 'var(--color-foreground)' : 'var(--color-muted)' }}
                        title={item?.name}
                      >
                        {item?.name ?? 'Nothing'}
                      </span>
                      {changed && (
                        <button
                          type="button"
                          title="Back to what's equipped"
                          onClick={(e) => {
                            e.stopPropagation()
                            setTryOns((t) => {
                              const next = { ...t }
                              delete next[s.key]
                              return next
                            })
                          }}
                          style={{ color: 'var(--color-muted)' }}
                        >
                          <X size={12} />
                        </button>
                      )}
                    </div>
                    {picking === s.key && (
                      <ItemPicker
                        slot={s}
                        onPick={(it) => setTryOns((t) => ({ ...t, [s.key]: it }))}
                      />
                    )}
                  </li>
                )
              })}
            </ul>
          </div>
          <p className="px-1 text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            {exported
              ? `Equipped gear is from your Zeal inventory export of ${new Date(exported).toLocaleString()}. `
              : 'No Zeal inventory export found for this character, so nothing is equipped. '}
            Click a slot to try something else on.
          </p>
        </div>
      </div>
    </div>
  )
}

function Toggle({ options, value, onChange }: {
  options: { value: number; label: string }[]
  value: number
  onChange: (v: number) => void
}): React.ReactElement {
  return (
    <div className="flex overflow-hidden rounded text-[11px]" style={{ border: '1px solid var(--color-border)' }}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className="px-2.5 py-1"
          style={o.value === value
            ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }
            : { color: 'var(--color-muted-foreground)' }}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

const MATERIAL_SLOT_NAMES = ['head', 'chest', 'arms', 'wrists', 'hands', 'legs', 'feet']

function DressNotes({ info, luclin, hair }: {
  info: DressInfo | null
  luclin: boolean
  hair: number
}): React.ReactElement | null {
  const notes: string[] = []
  const names = (slots: number[]): string => slots.map((s) => MATERIAL_SLOT_NAMES[s]).join(', ')
  if (info?.missing.length) {
    notes.push(`Your client has no texture for this model's ${names(info.missing)}, so they're shown bare.`)
  }
  if (info?.fallback.length) {
    notes.push(`Your client has no texture for this model's ${names(info.fallback)}; a near one stands in.`)
  }
  if (info?.missingItems.length) {
    notes.push(`No model in your client for the ${info.missingItems.join(' and ')} item.`)
  }
  const onHead = info?.wear.some((w) => w.point === 'HAIR_POINT' || w.point === 'HEAD_POINT')
  if (luclin && info && hair >= 0 && !onHead) {
    notes.push('Your client has no hairstyle for this model, so it’s drawn bald (as the game does).')
  }
  if (!notes.length) return null
  return (
    <div className="space-y-0.5 border-t px-4 py-2 text-[11px]" style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}>
      {notes.map((n) => <p key={n}>{n}</p>)}
    </div>
  )
}

function ItemPicker({ slot, onPick }: {
  slot: SlotDef
  onPick: (item: ItemAppearance | null) => void
}): React.ReactElement {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<ItemAppearance[]>([])
  const [loading, setLoading] = useState(false)
  const abort = useRef<AbortController | null>(null)

  useEffect(() => {
    const t = setTimeout(() => {
      abort.current?.abort()
      const ac = new AbortController()
      abort.current = ac
      setLoading(true)
      searchWearables(slot.mask, q, ac.signal)
        .then((r) => !ac.signal.aborted && setResults(r))
        .catch(() => {})
        .finally(() => !ac.signal.aborted && setLoading(false))
    }, 200)
    return () => clearTimeout(t)
  }, [q, slot.mask])

  return (
    <div className="px-3 pb-2" style={{ backgroundColor: 'var(--color-surface-2)' }}>
      <div className="flex items-center gap-1.5 rounded px-2 py-1"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
        <Search size={12} style={{ color: 'var(--color-muted)' }} />
        <input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={`Find ${slot.label.toLowerCase()} items`}
          className="min-w-0 flex-1 bg-transparent text-xs outline-none"
          style={{ color: 'var(--color-foreground)' }}
        />
      </div>
      <ul className="mt-1 max-h-56 overflow-y-auto">
        <li>
          <button type="button" onClick={() => onPick(null)}
            className="w-full px-1 py-1 text-left text-xs hover:bg-white/5" style={{ color: 'var(--color-muted)' }}>
            Nothing
          </button>
        </li>
        {results.map((it) => (
          <li key={it.id}>
            <button type="button" onClick={() => onPick(it)}
              className="flex w-full items-center gap-2 px-1 py-1 text-left text-xs hover:bg-white/5"
              style={{ color: 'var(--color-foreground)' }}>
              <ItemIcon id={it.icon} name={it.name} size={16} />
              <span className="truncate">{it.name}</span>
            </button>
          </li>
        ))}
        {!loading && results.length === 0 && (
          <li className="px-1 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>No items found.</li>
        )}
      </ul>
    </div>
  )
}

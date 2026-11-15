import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RotateCcw, Search, Shirt, X } from 'lucide-react'
import {
  getCharacterAppearance,
  getItemAppearances,
  getZealInventory,
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
import type { DressInfo, ModelDress } from '../components/models/ModelViewer'

const ModelViewer = lazy(() => import('../components/models/ModelViewer'))

// The Dressing Room: your character's model from your own EverQuest
// client, in the gear from their last Zeal inventory export, with any other
// item tried on in its place. The game logs neither gender nor face, so
// gender is picked here and saved per character.

interface SlotDef {
  key: string
  label: string
  // items.slots bits for the slot.
  mask: number
  // Material slot (head, chest, arms, wrist, hands, legs, feet), or -1 for
  // a held item.
  material: number
}

const SLOTS: SlotDef[] = [
  { key: 'Head', label: 'Head', mask: 4, material: 0 },
  { key: 'Chest', label: 'Chest', mask: 131072, material: 1 },
  { key: 'Arms', label: 'Arms', mask: 128, material: 2 },
  { key: 'Wrist', label: 'Wrist', mask: 1536, material: 3 },
  { key: 'Hands', label: 'Hands', mask: 4096, material: 4 },
  { key: 'Legs', label: 'Legs', mask: 262144, material: 5 },
  { key: 'Feet', label: 'Feet', mask: 524288, material: 6 },
  { key: 'Primary', label: 'Primary', mask: 8192, material: -1 },
  { key: 'Secondary', label: 'Secondary', mask: 16384, material: -1 },
]

const SHIELD_ITEM_TYPE = 8

// Try-ons per slot: an item, or null for nothing; a slot not in the map
// shows what's equipped.
type TryOns = Record<string, ItemAppearance | null>

function heldModel(item: ItemAppearance | null | undefined): string | undefined {
  const f = item?.id_file?.toUpperCase() ?? ''
  return /^IT[1-9][0-9]*$/.test(f) ? f : undefined
}

export default function DressingRoomPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewed, setViewed] = useCachedState('dressingRoom.character', active)
  const [characters, setCharacters] = useState<Character[]>([])
  const [gender, setGender] = useState(0)
  const [luclin, setLuclin] = useState(false)
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
      })
      .catch(() => {})
    getZealInventory(viewed)
      .then(async (res) => {
        const inv = res.inventory
        if (!inv || !live) return
        setExported(inv.exported_at)
        // The first of a doubled slot (Wrist) is the one the client draws.
        const bySlot = new Map<string, number>()
        for (const e of inv.entries) {
          if (SLOTS.some((s) => s.key === e.location) && !bySlot.has(e.location) && e.id > 0) {
            bySlot.set(e.location, e.id)
          }
        }
        if (bySlot.size === 0) return
        const items = await getItemAppearances([...bySlot.values()])
        if (!live) return
        const byID = new Map(items.map((i) => [i.id, i]))
        const out: Record<string, ItemAppearance> = {}
        for (const [slot, id] of bySlot) {
          const it = byID.get(id)
          if (it) out[slot] = it
        }
        setEquipped(out)
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [viewed])

  const saveAppearance = useCallback((next: { gender?: number; luclin?: boolean }) => {
    if (!viewed) return
    const g = next.gender ?? gender
    const l = next.luclin ?? luclin
    setGender(g)
    setLuclin(l)
    setCharacterAppearance({ name: viewed, gender: g, luclin: l }).catch(() => {})
  }, [viewed, gender, luclin])

  const worn = useCallback(
    (key: string): ItemAppearance | null => (key in tryOns ? tryOns[key] : equipped[key] ?? null),
    [tryOns, equipped],
  )

  const dress = useMemo((): ModelDress => {
    const mats = [0, 0, 0, 0, 0, 0, 0]
    const tints = [0, 0, 0, 0, 0, 0, 0]
    for (const s of SLOTS) {
      if (s.material < 0) continue
      const it = worn(s.key)
      mats[s.material] = it?.material ?? 0
      tints[s.material] = it?.material ? it.color : 0
    }
    const secondary = worn('Secondary')
    return {
      mats,
      tints,
      primary: heldModel(worn('Primary')),
      secondary: heldModel(secondary),
      shield: secondary?.item_type === SHIELD_ITEM_TYPE,
    }
  }, [worn])

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
          <DressNotes info={info} luclin={luclin} headWorn={dress.mats[0] > 0} />
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

function DressNotes({ info, luclin, headWorn }: {
  info: DressInfo | null
  luclin: boolean
  headWorn: boolean
}): React.ReactElement | null {
  const notes: string[] = []
  const names = (slots: number[]): string => slots.map((s) => MATERIAL_SLOT_NAMES[s]).join(', ')
  if (info?.missing.length) {
    notes.push(`Your client has no texture for this model's ${names(info.missing)}, so they're shown bare.`)
  }
  if (info?.fallback.length) {
    notes.push(`Luclin models have no plate chest in this client; chain stands in.`)
  }
  if (info?.missingItems.length) {
    notes.push(`No model in your client for the ${info.missingItems.join(' and ')} item.`)
  }
  if (luclin && headWorn) notes.push('Helms aren’t drawn on Luclin models yet.')
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

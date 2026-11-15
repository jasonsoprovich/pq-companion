import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RotateCcw, Search, Shirt, X } from 'lucide-react'
import {
  DEFAULT_LOOKS,
  getCharacterAppearance,
  getIllusions,
  getCharacterTextureUrl,
  getModelLooks,
  listCharacters,
  searchWearables,
  setCharacterAppearance,
  type Character,
  type CharacterModelRef,
  type Illusion,
  type ItemAppearance,
  type ModelLookOptions,
  type ModelLooks,
} from '../services/api'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useCachedState } from '../hooks/useCachedState'
import { charRaceLabel } from '../lib/enumsCache'
import type { DressInfo } from '../components/models/ModelViewer'
import { DRESS_SLOTS, MONK_CLASS, PLAYER_RACES, dressFor, isPlayerRace, loadEquippedItems } from '../lib/characterDress'
import type { DressSlot, ModelDress } from '../lib/characterDress'

const ModelViewer = lazy(() => import('../components/models/ModelViewer'))

// The Dressing Room: your character's model from your own EverQuest
// client, in the gear from their last Zeal inventory export, with any other
// item tried on in its place. The game logs neither gender nor face, so
// gender and looks (face, hair, eyes) are picked here and saved per
// character. Another race can be previewed too (a race change, or an
// illusion), with its own gender and looks that aren't saved.

type SlotDef = DressSlot
const SLOTS = DRESS_SLOTS

// Try-ons per slot: an item, or null for nothing; a slot not in the map
// shows what's equipped.
type TryOns = Record<string, ItemAppearance | null>

// Preview is another race shown in the character's gear, with its own
// gender and looks, kept only while it's shown. texture is the race's
// texture set (an elemental's earth, fire, water or air); name is the
// illusion it shows, if it's one.
interface Preview {
  race: number
  texture: number
  gender: number
  looks: ModelLooks
  name?: string
}

// The race picker's values: a race ("r6"), or an illusion by its place
// in the list ("i3").
function pickValue(p: Preview | null, ownRace: number, illusions: Illusion[]): string {
  if (!p) return `r${ownRace}`
  const i = p.name ? illusions.findIndex((il) => il.name === p.name) : -1
  return i >= 0 ? `i${i}` : `r${p.race}`
}

export default function DressingRoomPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewed, setViewed] = useCachedState('dressingRoom.character', active)
  const [characters, setCharacters] = useState<Character[]>([])
  const [gender, setGender] = useState(0)
  const [luclin, setLuclin] = useState(false)
  const [looks, setLooks] = useState<ModelLooks>(DEFAULT_LOOKS)
  const [equipped, setEquipped] = useState<Record<string, ItemAppearance>>({})
  const [exported, setExported] = useState<string | null>(null)
  const [tryOns, setTryOns] = useState<TryOns>({})
  const [picking, setPicking] = useState<string | null>(null)
  const [info, setInfo] = useState<DressInfo | null>(null)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [illusions, setIllusions] = useState<Illusion[]>([])

  useEffect(() => {
    getIllusions().then(setIllusions).catch(() => setIllusions([]))
  }, [])

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
    setPreview(null)
    setEquipped({})
    setExported(null)
    getCharacterAppearance(viewed)
      .then((a) => {
        if (!live) return
        setGender(a.gender)
        setLuclin(a.luclin)
        setLooks({
          face: a.face ?? 0, hair: a.hair ?? 0, hair_color: a.hair_color ?? -1,
          beard: a.beard ?? -1, beard_color: a.beard_color ?? -1, eyes: a.eyes ?? 0,
        })
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

  const saveAppearance = useCallback((next: { gender?: number; luclin?: boolean } & Partial<ModelLooks>) => {
    if (!viewed) return
    const { gender: ng, luclin: nl, ...nextLooks } = next
    const g = ng ?? gender
    const l = nl ?? luclin
    const lk = { ...looks, ...nextLooks }
    setGender(g)
    setLuclin(l)
    setLooks(lk)
    setCharacterAppearance({ name: viewed, gender: g, luclin: l, ...lk }).catch(() => {})
  }, [viewed, gender, luclin, looks])

  // change applies a pick: to the preview while one is shown (the era
  // is still the character's), else saved to the character.
  const change = useCallback((next: { gender?: number; luclin?: boolean } & Partial<ModelLooks>) => {
    if (!preview) {
      saveAppearance(next)
      return
    }
    const { gender: ng, luclin: nl, ...nextLooks } = next
    setPreview({ ...preview, gender: ng ?? preview.gender, looks: { ...preview.looks, ...nextLooks } })
    if (nl !== undefined) saveAppearance({ luclin: nl })
  }, [preview, saveAppearance])

  const worn = useCallback(
    (key: string): ItemAppearance | null => (key in tryOns ? tryOns[key] : equipped[key] ?? null),
    [tryOns, equipped],
  )

  const ownRace = character?.race ?? -1
  const race = preview?.race ?? ownRace
  const shownGender = preview?.gender ?? gender
  const shownLooks = preview?.looks ?? looks
  const player = isPlayerRace(race)

  // Other races than the player ones keep their own skin, in the
  // illusion's texture set: only what's in their hands carries over, as
  // with an illusion in game.
  const texture = preview?.texture ?? 0
  const dress = useMemo((): ModelDress => {
    const d = dressFor(worn, character?.class === MONK_CLASS)
    if (!player) {
      return {
        // The head too: the server gives an illusion's head its texture.
        mats: Array(7).fill(texture), tints: [0, 0, 0, 0, 0, 0, 0],
        primary: d.primary, secondary: d.secondary, shield: d.shield,
      }
    }
    return { ...d, looks: shownLooks }
  }, [worn, character?.class, shownLooks, player, texture])

  const pick = (value: string): void => {
    const n = Number(value.slice(1))
    if (value[0] === 'i') {
      const il = illusions[n]
      if (!il) return
      setPreview({
        race: il.race, texture: il.texture, gender: il.gender >= 0 ? il.gender : shownGender,
        looks: DEFAULT_LOOKS, name: il.name,
      })
    } else if (n === ownRace) {
      setPreview(null)
    } else {
      setPreview({ race: n, texture: 0, gender: shownGender, looks: DEFAULT_LOOKS })
    }
  }
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
                {character && ownRace > 0
                  ? `${character.name} · ${charRaceLabel(ownRace)}`
                  : 'Your character, drawn from your own EverQuest client.'}
                {preview && (
                  <>
                    {' · previewing '}
                    <span style={{ color: 'var(--color-warning, #f59e0b)' }}>{preview.name ?? charRaceLabel(preview.race)}</span>
                    {' '}
                    <button
                      type="button"
                      onClick={() => setPreview(null)}
                      className="underline"
                      style={{ color: 'var(--color-muted-foreground)' }}
                    >
                      back to {charRaceLabel(ownRace)}
                    </button>
                  </>
                )}
              </p>
            </div>
            {ownRace > 0 && (
              <select
                value={pickValue(preview, ownRace, illusions)}
                onChange={(e) => pick(e.target.value)}
                className="rounded px-2 py-1 text-[11px]"
                style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                title="Preview another race in this gear: a race change, or an illusion"
              >
                <option value={`r${ownRace}`}>{charRaceLabel(ownRace)} (yours)</option>
                <optgroup label="Races">
                  {PLAYER_RACES.filter((r) => r !== ownRace).map((r) => (
                    <option key={r} value={`r${r}`}>{charRaceLabel(r)}</option>
                  ))}
                </optgroup>
                {illusions.some((il) => !isPlayerRace(il.race)) && (
                  <optgroup label="Illusions">
                    {illusions.map((il, i) => !isPlayerRace(il.race) && (
                      <option key={il.name} value={`i${i}`} title={il.spells.join(', ')}>{il.name}</option>
                    ))}
                  </optgroup>
                )}
              </select>
            )}
            {/* An illusion into another race takes the gender the game
                gives it (neutral, or the spell's), not a pick. */}
            {player && (
              <Toggle
                options={[{ value: 0, label: 'Male' }, { value: 1, label: 'Female' }]}
                value={shownGender}
                onChange={(g) => change({ gender: g })}
              />
            )}
            <Toggle
              options={[{ value: 0, label: 'Original' }, { value: 1, label: 'Luclin' }]}
              value={luclin ? 1 : 0}
              onChange={(v) => change({ luclin: v === 1 })}
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
                  model={{ race, gender: shownGender, luclin }}
                  dress={dress}
                  onDressed={setInfo}
                />
              </Suspense>
            )}
          </div>
          <DressNotes info={info} luclin={luclin} player={player} hair={shownLooks.hair} />
        </div>

        <div className="flex w-80 shrink-0 flex-col gap-3 overflow-y-auto">
          {race > 0 && player && info && (
            <LooksPanel
              model={info.ref}
              luclin={luclin}
              looks={shownLooks}
              onChange={(l) => change(l)}
            />
          )}
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

// LooksPanel picks a character's face, hair and eyes from what their
// model has in the client. The game keeps these on the character but
// never logs them, so they're set by hand to match what's seen in game.
function LooksPanel({ model, luclin, looks, onChange }: {
  model: CharacterModelRef
  luclin: boolean
  looks: ModelLooks
  onChange: (l: Partial<ModelLooks>) => void
}): React.ReactElement | null {
  const [opts, setOpts] = useState<ModelLookOptions | null>(null)
  const [eyes, setEyes] = useState<Record<number, ImageBitmap>>({})
  useEffect(() => {
    let live = true
    const made: ImageBitmap[] = []
    setOpts(null)
    setEyes({})
    getModelLooks(model)
      .then(async (o) => {
        if (!live) return
        setOpts(o)
        // Fetched and drawn, as the 3D view does (the page's CSP keeps
        // images to its own origin).
        for (const e of o.eyes) {
          const res = await fetch(await getCharacterTextureUrl('lgequip.s3d', e.texture, false))
          if (!live || !res.ok) continue
          const bitmap = await createImageBitmap(await res.blob())
          made.push(bitmap)
          if (live) setEyes((u) => ({ ...u, [e.index]: bitmap }))
        }
      })
      .catch(() => live && setOpts(null))
    return () => {
      live = false
      made.forEach((b) => b.close())
    }
  }, [model.archive, model.code])
  if (!opts) return null

  const hairColor = looks.hair_color >= 0 ? looks.hair_color : opts.default_hair_color
  const showHair = luclin && opts.hair.length > 0
  return (
    <div className="rounded-lg"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
      <div className="flex items-center justify-between border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Looks</span>
        <button
          type="button"
          onClick={() => onChange(DEFAULT_LOOKS)}
          className="flex items-center gap-1 text-[11px]"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Back to the default face, hair and eyes"
        >
          <RotateCcw size={12} /> Default
        </button>
      </div>
      <div className="space-y-2.5 px-3 py-2.5">
        {opts.faces > 1 && (
          <LookRow label="Face">
            <Toggle
              options={Array.from({ length: opts.faces }, (_, i) => ({ value: i, label: String(i + 1) }))}
              value={Math.min(looks.face, opts.faces - 1)}
              onChange={(f) => onChange({ face: f })}
            />
          </LookRow>
        )}
        {showHair && (
          <LookRow label="Hair">
            <Toggle
              options={[
                { value: -1, label: 'Bald' },
                ...opts.hair.map((h) => ({ value: h, label: String(h + 1) })),
              ]}
              value={looks.hair}
              onChange={(h) => onChange({ hair: h })}
            />
          </LookRow>
        )}
        {luclin && opts.hair_colors.length > 0 && (
          <LookRow label="Hair colour">
            <Swatches
              colors={opts.hair_colors}
              value={hairColor}
              onChange={(c) => onChange({ hair_color: c })}
            />
          </LookRow>
        )}
        {luclin && opts.beards.length > 0 && (
          <LookRow label="Beard">
            <Toggle
              options={[
                { value: -1, label: 'None' },
                ...opts.beards.map((b) => ({ value: b, label: String(b + 1) })),
              ]}
              value={opts.beards.includes(looks.beard) ? looks.beard : -1}
              onChange={(b) => onChange({ beard: b })}
            />
          </LookRow>
        )}
        {luclin && opts.beards.length > 0 && looks.beard >= 0 && opts.hair_colors.length > 0 && (
          <LookRow label="Beard colour">
            <div className="flex flex-wrap items-center gap-1">
              <button
                type="button"
                onClick={() => onChange({ beard_color: -1 })}
                className="rounded px-1.5 py-0.5 text-[10px]"
                style={looks.beard_color < 0
                  ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }
                  : { color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
              >
                As hair
              </button>
              <Swatches
                colors={opts.hair_colors}
                value={looks.beard_color}
                onChange={(c) => onChange({ beard_color: c })}
              />
            </div>
          </LookRow>
        )}
        {luclin && opts.eyes.length > 0 && (
          <LookRow label="Eyes">
            <div className="flex flex-wrap gap-1">
              {opts.eyes.map((e) => {
                const on = (looks.eyes || 1) === e.index
                return (
                  <button
                    key={e.index}
                    type="button"
                    title={`Eye colour ${e.index}`}
                    onClick={() => onChange({ eyes: e.index })}
                    className="h-6 w-6 overflow-hidden rounded-full"
                    style={{
                      outline: on ? '2px solid var(--color-primary)' : '1px solid var(--color-border)',
                      outlineOffset: on ? 1 : 0,
                    }}
                  >
                    {eyes[e.index] && <BitmapSwatch bitmap={eyes[e.index]} />}
                  </button>
                )
              })}
            </div>
          </LookRow>
        )}
        <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
          {luclin
            ? 'The game never logs your face, hair or eyes, so pick what you see in game. Saved for this character.'
            : 'The game never logs your face, so pick what you see in game. Hair, beards and eye colour are on the Luclin models.'}
        </p>
      </div>
    </div>
  )
}

// BitmapSwatch draws an image, centred and cropped to fill.
function BitmapSwatch({ bitmap }: { bitmap: ImageBitmap }): React.ReactElement {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const ctx = ref.current?.getContext('2d')
    if (!ctx) return
    // The middle of the texture, where the iris is.
    const side = Math.min(bitmap.width, bitmap.height) / 2
    ctx.clearRect(0, 0, 48, 48)
    ctx.drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, 48, 48)
  }, [bitmap])
  return <canvas ref={ref} width={48} height={48} className="h-full w-full" />
}

function LookRow({ label, children }: { label: string; children: React.ReactNode }): React.ReactElement {
  return (
    <div>
      <div className="mb-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>{label}</div>
      {children}
    </div>
  )
}

function Swatches({ colors, value, onChange }: {
  colors: { index: number; rgb: number }[]
  value: number
  onChange: (index: number) => void
}): React.ReactElement {
  return (
    <div className="flex flex-wrap gap-1">
      {colors.map((c) => {
        const on = c.index === value
        return (
          <button
            key={c.index}
            type="button"
            title={`Colour ${c.index + 1}`}
            onClick={() => onChange(c.index)}
            className="h-5 w-5 rounded-full"
            style={{
              backgroundColor: `#${c.rgb.toString(16).padStart(6, '0')}`,
              outline: on ? '2px solid var(--color-primary)' : '1px solid var(--color-border)',
              outlineOffset: on ? 1 : 0,
            }}
          />
        )
      })}
    </div>
  )
}

const MATERIAL_SLOT_NAMES = ['head', 'chest', 'arms', 'wrists', 'hands', 'legs', 'feet']

function DressNotes({ info, luclin, player, hair }: {
  info: DressInfo | null
  luclin: boolean
  // Only the player races wear armor and hair, or hold items for sure;
  // other races' notes about them would mislead.
  player: boolean
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
  if (player && info?.missingItems.length) {
    notes.push(`No model in your client for the ${info.missingItems.join(' and ')} item.`)
  }
  const onHead = info?.wear.some((w) => w.point === 'HAIR_POINT' || w.point === 'HEAD_POINT')
  if (player && luclin && info && hair >= 0 && !onHead) {
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

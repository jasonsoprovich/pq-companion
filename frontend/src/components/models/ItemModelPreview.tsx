import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { getItemAppearances, listCharacters, type ItemAppearance } from '../../services/api'
import { useActiveCharacter } from '../../contexts/ActiveCharacterContext'
import { useModelPreviewPrefs } from '../../hooks/useModelPreviewPrefs'
import { armorSlots, isHeldModel, racesForMask, type ModelDress } from '../../lib/characterDress'
import { charRaceLabel } from '../../lib/enumsCache'
import ModelPreviewCard, { PreviewToggle } from './ModelPreviewCard'

const ModelViewer = lazy(() => import('./ModelViewer'))

// ItemModelPreview shows an item's look on its page: a weapon, shield or
// other held item as its own model, to turn around; a piece of armor worn
// by a race that can wear it. Items with no look of their own (jewelry,
// scrolls, everything that drops as the bag model) show nothing.

interface PreviewItem {
  id: number
  slots: number
  races: number
  id_file: string
}

export default function ItemModelPreview({ item }: { item: PreviewItem }): React.ReactElement | null {
  const [prefs, setPrefs] = useModelPreviewPrefs()
  const { active } = useActiveCharacter()
  const [look, setLook] = useState<ItemAppearance | null>(null)
  const [ownRace, setOwnRace] = useState(0)
  const slots = armorSlots(item.slots)

  // The material and tint only matter for armor.
  useEffect(() => {
    setLook(null)
    if (slots.length === 0) return
    let live = true
    getItemAppearances([item.id])
      .then((r) => live && setLook(r[0] ?? null))
      .catch(() => {})
    return () => {
      live = false
    }
    // slots follows item.slots.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, item.slots])

  useEffect(() => {
    if (!active) return
    let live = true
    listCharacters()
      .then((r) => live && setOwnRace(r.characters.find((c) => c.name === active)?.race ?? 0))
      .catch(() => {})
    return () => {
      live = false
    }
  }, [active])

  const races = useMemo(() => racesForMask(item.races), [item.races])
  const race = races.includes(prefs.race) ? prefs.race : races.includes(ownRace) ? ownRace : races[0]
  const worn = !!look && look.material > 0 && slots.length > 0
  const dress = useMemo((): ModelDress | undefined => {
    if (!worn || !look) return undefined
    const mats = [0, 0, 0, 0, 0, 0, 0]
    const tints = [0, 0, 0, 0, 0, 0, 0]
    for (const s of slots) {
      mats[s] = look.material
      tints[s] = look.color
    }
    return { mats, tints }
    // slots follows item.slots.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [worn, look, item.slots])

  if (worn) {
    return (
      <ModelPreviewCard
        controls={
          <>
            <select
              value={race}
              onChange={(e) => setPrefs({ race: Number(e.target.value) })}
              className="rounded px-1 py-0.5 text-[10px]"
              style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              title="Who to show it on"
            >
              {races.map((r) => <option key={r} value={r}>{charRaceLabel(r)}</option>)}
            </select>
            <PreviewToggle
              options={[{ value: 0, label: 'M' }, { value: 1, label: 'F' }]}
              value={prefs.gender}
              onChange={(g) => setPrefs({ gender: g })}
            />
            <PreviewToggle
              options={[{ value: 0, label: 'Old' }, { value: 1, label: 'Luclin' }]}
              value={prefs.luclin ? 1 : 0}
              onChange={(v) => setPrefs({ luclin: v === 1 })}
            />
          </>
        }
      >
        <Suspense fallback={null}>
          <ModelViewer
            className="h-full w-full"
            model={{ race, gender: prefs.gender, luclin: prefs.luclin }}
            dress={dress}
          />
        </Suspense>
      </ModelPreviewCard>
    )
  }
  if (slots.length === 0 && isHeldModel(item.id_file)) {
    return (
      <ModelPreviewCard>
        <Suspense fallback={null}>
          <ModelViewer className="h-full w-full" item={item.id_file.toUpperCase()} />
        </Suspense>
      </ModelPreviewCard>
    )
  }
  return null
}

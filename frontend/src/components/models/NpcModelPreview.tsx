import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { getNPCModelLook, type NPCModelLook } from '../../services/api'
import { useModelPreviewPrefs } from '../../hooks/useModelPreviewPrefs'
import { npcDress, type ModelDress } from '../../lib/characterDress'
import ModelPreviewCard, { PreviewToggle } from './ModelPreviewCard'

const ModelViewer = lazy(() => import('./ModelViewer'))

// NpcModelPreview shows an NPC's model on its page as the game draws it:
// a player race in what the server dresses it in, any other race in its
// texture set, preferring its own zone's version of the model.

// Invisible Man: drawn as nothing in game, so there's nothing to show.
const INVISIBLE_MAN = 127

export default function NpcModelPreview({ npcID }: { npcID: number }): React.ReactElement | null {
  const [prefs, setPrefs] = useModelPreviewPrefs()
  const [look, setLook] = useState<NPCModelLook | null>(null)

  useEffect(() => {
    setLook(null)
    let live = true
    getNPCModelLook(npcID)
      .then((l) => live && setLook(l))
      .catch(() => {})
    return () => {
      live = false
    }
  }, [npcID])

  const shown = useMemo(() => {
    if (!look) return null
    const { dress, looks, player } = npcDress(look)
    // A player race takes its face and hair through the dress.
    const d: ModelDress | undefined = player
      ? { ...(dress ?? { mats: [0, 0, 0, 0, 0, 0, 0], tints: [0, 0, 0, 0, 0, 0, 0] }), looks }
      : dress ?? undefined
    return { dress: d, player }
  }, [look])

  if (!look || !shown || look.race <= 0 || look.race === INVISIBLE_MAN) return null
  return (
    <ModelPreviewCard
      controls={shown.player && (
        <PreviewToggle
          options={[{ value: 0, label: 'Old' }, { value: 1, label: 'Luclin' }]}
          value={prefs.luclin ? 1 : 0}
          onChange={(v) => setPrefs({ luclin: v === 1 })}
        />
      )}
    >
      <Suspense fallback={null}>
        <ModelViewer
          className="h-full w-full"
          model={{ race: look.race, gender: look.gender, zone: look.zone || undefined, luclin: prefs.luclin }}
          dress={shown.dress}
        />
      </Suspense>
    </ModelPreviewCard>
  )
}

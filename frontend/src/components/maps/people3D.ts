// People drawn as character models in the 3D map: each marker that says
// who it is (RaiderMarker.model) gets its race's model from the user's own
// client, at its race's height, standing on its arrow and facing its
// heading. Models load in the background; the arrow shows until they do,
// and stays under them so people are found from far off.

import * as THREE from 'three'
import { buildCharacter, raceHeight } from '../../lib/characterModels'
import type { DressedCharacter } from '../../lib/characterModels'
import type { Map3DPeople } from '../../hooks/useMap3DPrefs'
import type { MarkerModel, RaiderMarker, SpawnDot } from './ZoneMap'

interface Person {
  // spec is what the model was asked for; a change reloads it.
  spec: string
  char: DressedCharacter | null
  // scale is the model's draw scale for its race's height.
  scale: number
}

export class PeopleLayer {
  readonly group = new THREE.Group()
  private people = new Map<string, Person>()
  private disposed = false

  constructor(private render: () => void) {}

  /** update places everyone in markers, loading models as needed. */
  update(markers: RaiderMarker[], mode: Map3DPeople): void {
    const seen = new Set<string>()
    if (mode !== 'off') {
      for (const m of markers) {
        if (!m.model || m.dead) continue
        const key = m.label
        seen.add(key)
        const spec = JSON.stringify({ ...m.model, luclin: mode === 'luclin' })
        let p = this.people.get(key)
        if (!p || p.spec !== spec) {
          if (p) this.drop(key, p)
          p = { spec, char: null, scale: 1 }
          this.people.set(key, p)
          this.load(key, p, m, mode === 'luclin')
        }
        const o = p.char?.object
        if (o) {
          // Facing the heading, as the arrow does: models face +y.
          o.position.set(m.x, -m.y, m.z)
          o.rotation.z = ((m.heading ?? 0) / 512) * Math.PI * 2
          o.scale.setScalar(p.scale)
        }
      }
    }
    for (const [key, p] of this.people) {
      if (!seen.has(key)) this.drop(key, p)
    }
  }

  private load(key: string, p: Person, m: RaiderMarker, luclin: boolean): void {
    const model = m.model!
    buildCharacter(
      { race: model.race, gender: model.gender, luclin, looks: model.looks, zone: model.zone },
      model.dress ?? null,
    )
      .then((char) => {
        if (this.disposed || this.people.get(key) !== p) {
          char?.dispose()
          return
        }
        if (!char) return
        p.char = char
        const h = model.height ?? raceHeight(model.race)
        p.scale = model.scale ?? (h && char.base.height > 0 ? h / char.base.height : 1)
        char.object.position.set(m.x, -m.y, m.z)
        char.object.rotation.z = ((m.heading ?? 0) / 512) * Math.PI * 2
        char.object.scale.setScalar(p.scale)
        this.group.add(char.object)
        this.render()
      })
      .catch(() => {
        // No model (or no client): the arrow alone stands in.
      })
  }

  private drop(key: string, p: Person): void {
    if (p.char) {
      this.group.remove(p.char.object)
      p.char.dispose()
    }
    this.people.delete(key)
  }

  dispose(): void {
    this.disposed = true
    for (const [key, p] of this.people) this.drop(key, p)
  }
}

/**
 * npcMarkers stands the mob most likely to spawn at each spawn point on
 * it, for a PeopleLayer: player races dressed in what the server gives
 * them, other races in their texture set, each at its size. Spawns known
 * to be dead are left empty.
 */
export function npcMarkers(dots: SpawnDot[], zone: string): RaiderMarker[] {
  const out: RaiderMarker[] = []
  for (const d of dots) {
    const l = d.look
    if (!l || d.down || d.id === undefined || l.race <= 0) continue
    const player = raceHeight(l.race) !== null
    const model: MarkerModel = { race: l.race, gender: l.gender, zone }
    if (l.size > 0) {
      if (player) model.height = l.size
      else model.scale = l.base_size > 0 ? l.size / l.base_size : 1
    } else if (!player) {
      model.scale = 1
    }
    const mats = player ? l.mats : [0, ...Array(6).fill(l.texture)]
    const dressed = mats.some((m) => m > 0) || l.tints.some((t) => t > 0) || l.primary || l.secondary
    if (dressed) {
      model.dress = {
        mats, tints: l.tints, primary: l.primary, secondary: l.secondary,
      }
    }
    if (player) {
      model.looks = {
        face: l.face, hair: l.hair, hair_color: l.hair_color,
        beard: l.beard, beard_color: l.beard_color, eyes: l.eyes,
      }
    }
    out.push({
      x: d.x, y: d.y, z: d.z, heading: d.heading ?? 0, color: d.color,
      label: `spawn:${d.id}`, model,
    })
  }
  return out
}

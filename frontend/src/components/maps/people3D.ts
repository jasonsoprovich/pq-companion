// People drawn as character models in the 3D map: each marker that says
// who it is (RaiderMarker.model) gets its race's model from the user's own
// client, at its race's height, standing on its arrow and facing its
// heading. Models load in the background; the arrow shows until they do,
// and stays under them so people are found from far off.

import * as THREE from 'three'
import { buildCharacter, raceHeight } from '../../lib/characterModels'
import type { DressedCharacter } from '../../lib/characterModels'
import type { Map3DPeople } from '../../hooks/useMap3DPrefs'
import type { RaiderMarker } from './ZoneMap'

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
    buildCharacter({ race: model.race, gender: model.gender, luclin, looks: model.looks }, model.dress ?? null)
      .then((char) => {
        if (this.disposed || this.people.get(key) !== p) {
          char?.dispose()
          return
        }
        if (!char) return
        p.char = char
        const h = raceHeight(model.race)
        p.scale = h && char.base.height > 0 ? h / char.base.height : 1
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

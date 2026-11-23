// People drawn as character models in the 3D map: each marker that says
// who it is (RaiderMarker.model) gets its race's model from the user's own
// client, at its race's height, standing on its arrow and facing its
// heading. Models load in the background; the arrow shows until they do,
// then gives way to a ring at the model's feet (see footprint).

import * as THREE from 'three'
import { buildCharacter, raceHeight } from '../../lib/characterModels'
import type { DressedCharacter } from '../../lib/characterModels'
import { npcDress } from '../../lib/characterDress'
import type { Map3DPeople } from '../../hooks/useMap3DPrefs'
import type { MarkerModel, RaiderMarker, SpawnDot } from './ZoneMap'

// MAP_TEXTURES is the texture size models are drawn with on the map. A
// busy zone shows a couple of thousand distinct textures; at map distance
// a model is a few dozen pixels tall, and full-size textures would take
// hundreds of MB of GPU memory.
const MAP_TEXTURES = 64

interface Person {
  // spec is what the model was asked for; a change reloads it.
  spec: string
  char: DressedCharacter | null
  // scale is the model's draw scale for its race's height.
  scale: number
  // feet is the height the model stands at, set when it's placed.
  feet: number
}

export class PeopleLayer {
  readonly group = new THREE.Group()
  private people = new Map<string, Person>()
  private disposed = false
  // ground finds the floor under a world point near a height, so models
  // stand on it: the game's height for a spawn is about its middle, not
  // its feet. Null until the zone loads, and where there's no floor near,
  // the marker's own height stands.
  ground: ((x: number, y: number, z: number) => number | null) | null = null

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
          p = { spec, char: null, scale: 1, feet: m.z }
          this.people.set(key, p)
          this.load(key, p, m, mode === 'luclin')
        }
        if (p.char) this.place(p, m)
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
      MAP_TEXTURES,
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
        this.place(p, m)
        this.group.add(char.object)
        this.render()
      })
      .catch(() => {
        // No model (or no client): the arrow alone stands in.
      })
  }

  // place stands p's model on the floor under m, facing its heading as the
  // arrow does (models face +y).
  private place(p: Person, m: RaiderMarker): void {
    const o = p.char!.object
    const z = this.ground?.(m.x, -m.y, m.z) ?? m.z
    p.feet = z
    o.position.set(m.x, -m.y, z)
    o.rotation.z = ((m.heading ?? 0) / 512) * Math.PI * 2
    o.scale.setScalar(p.scale)
  }

  /**
   * footprint is where the model for a marker label stands and how tall it
   * is drawn, once it has loaded; null while there is no model, so the
   * marker keeps its arrow.
   */
  footprint(label: string): { z: number; height: number } | null {
    const p = this.people.get(label)
    if (!p?.char) return null
    return { z: p.feet, height: p.char.base.height * p.scale }
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
    const { dress, looks, player } = npcDress(l)
    const model: MarkerModel = { race: l.race, gender: l.gender, zone }
    if (l.size > 0) {
      if (player) model.height = l.size
      else model.scale = l.base_size > 0 ? l.size / l.base_size : 1
    } else if (!player) {
      model.scale = 1
    }
    if (dress) model.dress = dress
    if (looks) model.looks = looks
    out.push({
      x: d.x, y: d.y, z: d.z, heading: d.heading ?? 0, color: d.color,
      label: `spawn:${d.id}`, model,
    })
  }
  return out
}

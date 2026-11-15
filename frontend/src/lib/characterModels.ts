// Character models (players and NPCs) as three.js objects, read from the
// user's own EverQuest client through the backend (api/models.go): the
// model the client draws for a race and gender, posed standing, with its
// textures. Shared by every 3D view that shows people: the dressing room,
// and the raid replay and live map once they draw models.
//
// Each model is loaded once: callers get their own THREE.Mesh, but the
// geometry, materials and textures are shared, so a raid of 72 copies
// costs one download. Nothing is disposed; a session uses a few dozen
// models at most.

import * as THREE from 'three'
import {
  getCharacterMesh3D,
  getCharacterTextureUrl,
  resolveCharacterModel,
} from '../services/api'
import type { CharacterModelRef, ZoneMesh3D } from '../services/api'

export interface CharacterModelRequest {
  /** npc_types race id. */
  race: number
  /** 0 male, 1 female, 2 neutral. */
  gender: number
  /** Prefer this zone's version of an NPC model. */
  zone?: string
  /** The Luclin-era model, for player races. */
  luclin?: boolean
}

/**
 * A loaded model, shared by everyone who asked for it. Geometry is in
 * world space as the 3D views use it (map y negated, z up), with the feet
 * at z = 0 and the body centred on the origin; `height` is its z extent in
 * game units.
 */
export interface CharacterModelAsset {
  ref: CharacterModelRef
  geometry: THREE.BufferGeometry
  materials: THREE.Material[]
  height: number
}

const assets = new Map<string, Promise<CharacterModelAsset | null>>()
const textures = new Map<string, THREE.Texture>()
// Called when a texture arrives, so open views can redraw.
const listeners = new Set<() => void>()

/** onCharacterTexture registers a redraw for when model textures load. */
export function onCharacterTexture(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

function requestKey(r: CharacterModelRequest): string {
  return `${r.race}|${r.gender}|${r.zone ?? ''}|${r.luclin ? 1 : 0}`
}

/**
 * loadCharacterModel resolves and loads the model for a race and gender.
 * Resolves null when the client has no model for it. Loads are shared.
 */
export function loadCharacterModel(r: CharacterModelRequest): Promise<CharacterModelAsset | null> {
  const key = requestKey(r)
  let p = assets.get(key)
  if (!p) {
    p = (async () => {
      const ref = await resolveCharacterModel(r.race, r.gender, { zone: r.zone, luclin: r.luclin })
      if (!ref) return null
      // Two requests can resolve to one model (a zone without its own
      // copy); share the loaded asset between them.
      const refKey = `ref|${ref.archive}|${ref.code}`
      let byRef = assets.get(refKey)
      if (!byRef) {
        byRef = getCharacterMesh3D(ref).then((data) => (data ? buildAsset(ref, data) : null))
        byRef.catch(() => assets.delete(refKey))
        assets.set(refKey, byRef)
      }
      return byRef
    })()
    // A failed load isn't kept, so it's retried next time.
    p.catch(() => assets.delete(key))
    assets.set(key, p)
  }
  return p
}

/** createCharacterMesh makes a mesh of a loaded model, sharing its data. */
export function createCharacterMesh(asset: CharacterModelAsset): THREE.Mesh {
  const mesh = new THREE.Mesh(asset.geometry, asset.materials)
  mesh.userData.characterModel = asset.ref
  return mesh
}

function buildAsset(ref: CharacterModelRef, data: ZoneMesh3D): CharacterModelAsset {
  const pos = new Float32Array(data.positions)
  let minX = Infinity, minY = Infinity, minZ = Infinity
  let maxX = -Infinity, maxY = -Infinity, maxZ = -Infinity
  for (let i = 0; i < pos.length; i += 3) {
    // World y is map y negated, as in the zone view.
    pos[i + 1] = -pos[i + 1]
    minX = Math.min(minX, pos[i]); maxX = Math.max(maxX, pos[i])
    minY = Math.min(minY, pos[i + 1]); maxY = Math.max(maxY, pos[i + 1])
    minZ = Math.min(minZ, pos[i + 2]); maxZ = Math.max(maxZ, pos[i + 2])
  }
  // Stand the model on its feet at the origin.
  const cx = (minX + maxX) / 2
  const cy = (minY + maxY) / 2
  for (let i = 0; i < pos.length; i += 3) {
    pos[i] -= cx
    pos[i + 1] -= cy
    pos[i + 2] -= minZ
  }
  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(pos, 3))
  geometry.setAttribute('uv', new THREE.BufferAttribute(data.uvs, 2))
  geometry.setIndex(new THREE.BufferAttribute(data.indices, 1))
  for (const g of data.groups) geometry.addGroup(g.start, g.count, g.material)
  geometry.computeVertexNormals()
  geometry.computeBoundingSphere()

  const materials = data.materials.map((m): THREE.Material => {
    const map = m.texture ? texture(ref.archive, m.texture, m.kind === 1) : null
    if (m.kind === 3) {
      return new THREE.MeshBasicMaterial({
        map, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false,
      })
    }
    return new THREE.MeshLambertMaterial({
      map,
      color: map ? 0xffffff : 0x8a8f98,
      // Client textures are lit for the game's own lighting; a little
      // self-light keeps them from going murky (as in the zone view).
      emissive: map ? 0xffffff : 0x000000,
      emissiveMap: map,
      emissiveIntensity: 0.3,
      side: m.kind === 1 ? THREE.DoubleSide : THREE.FrontSide,
      alphaTest: m.kind === 1 ? 0.5 : 0,
      transparent: m.kind === 2,
      opacity: m.kind === 2 ? m.opacity : 1,
      depthWrite: m.kind !== 2,
    })
  })
  return { ref, geometry, materials, height: maxZ - minZ }
}

// texture returns a character texture, starting its load the first time.
function texture(archive: string, name: string, masked: boolean): THREE.Texture {
  const key = `${archive}|${name}|${masked ? 1 : 0}`
  let t = textures.get(key)
  if (t) return t
  const tex = new THREE.Texture()
  // Client UVs run top-down (DirectX); ImageBitmaps aren't flipped.
  tex.flipY = false
  tex.wrapS = THREE.RepeatWrapping
  tex.wrapT = THREE.RepeatWrapping
  tex.colorSpace = THREE.SRGBColorSpace
  textures.set(key, tex)
  // Fetched rather than loaded as an <img>: the page's CSP allows the
  // backend for connections but not for images.
  getCharacterTextureUrl(archive, name, masked)
    .then((url) => fetch(url))
    .then((res) => (res.ok ? res.blob() : Promise.reject(new Error(res.statusText))))
    .then((blob) => createImageBitmap(blob, { premultiplyAlpha: 'none', colorSpaceConversion: 'none' }))
    .then((bitmap) => {
      tex.image = bitmap
      tex.needsUpdate = true
    })
    .catch(() => {
      tex.image = new ImageData(new Uint8ClampedArray([138, 143, 152, 255]), 1, 1)
      tex.needsUpdate = true
    })
    .finally(() => listeners.forEach((fn) => fn()))
  t = tex
  return t
}

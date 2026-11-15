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
  getAttachedMesh3D,
  getCharacterMesh3D,
  getCharacterTextureUrl,
  getModelOutfit,
  locateItemModel,
  resolveCharacterModel,
} from '../services/api'
import type { ModelDress } from './characterDress'
import type {
  CharacterMeshOptions,
  CharacterModelRef,
  ModelOutfit,
  ZoneMesh3D,
} from '../services/api'

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
  /** Each material's client texture name ("" for none). */
  textures: string[]
  height: number
  /** What was taken off map-space positions to centre the model (after
   * the y flip); items attached to it are moved by the same. */
  offset: [number, number, number]
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

/**
 * loadModelVariant loads a resolved player model with mesh swaps (helm,
 * robe), for the dressing room. Loads are shared like loadCharacterModel's.
 */
export function loadModelVariant(
  ref: CharacterModelRef,
  opts: CharacterMeshOptions,
): Promise<CharacterModelAsset | null> {
  if (!opts.helm && !opts.robe) {
    const key = `ref|${ref.archive}|${ref.code}`
    let p = assets.get(key)
    if (!p) {
      p = getCharacterMesh3D(ref).then((data) => (data ? buildAsset(ref, data) : null))
      p.catch(() => assets.delete(key))
      assets.set(key, p)
    }
    return p
  }
  const key = `variant|${ref.archive}|${ref.code}|${opts.helm ?? 0}|${opts.robe ? 1 : 0}`
  let p = assets.get(key)
  if (!p) {
    p = getCharacterMesh3D(ref, opts).then((data) => (data ? buildAsset(ref, data) : null))
    p.catch(() => assets.delete(key))
    assets.set(key, p)
  }
  return p
}

/**
 * loadAttachedItem loads an item model (idfile "IT63") placed at one of a
 * loaded model's attachment points. Resolves null when the client has no
 * such item model or the model no such point.
 */
export async function loadAttachedItem(
  body: CharacterModelAsset,
  point: 'R_POINT' | 'L_POINT' | 'SHIELD_POINT',
  idfile: string,
): Promise<CharacterModelAsset | null> {
  const item = await locateItemModel(idfile)
  if (!item) return null
  const key = `attach|${body.ref.archive}|${body.ref.code}|${point}|${item.archive}|${item.code}`
  let p = assets.get(key)
  if (!p) {
    p = getAttachedMesh3D(body.ref, point, item)
      .then((data) => (data ? buildAsset(item, data, body.offset) : null))
    p.catch(() => assets.delete(key))
    assets.set(key, p)
  }
  return p
}

// Body part textures: model code, part, material (or "sk"), variant.
const partTextureRe = /^[a-z]{3}(ch|fa|ft|hn|lg|ua|he)(?:[0-9]{2}|sk)[0-9]{2}\.(?:bmp|dds)$/
const partSlots: Record<string, number> = { he: 0, ch: 1, ua: 2, fa: 3, hn: 4, lg: 5, ft: 6 }

/** textureSlot is the material slot a body texture belongs to, or -1. */
function textureSlot(name: string): number {
  const lower = name.toLowerCase()
  if (/^clk[0-9]{4}\.(?:bmp|dds)$/.test(lower)) return 1 // the robed body
  const m = partTextureRe.exec(lower)
  return m ? partSlots[m[1]] : -1
}

/**
 * dressMaterials makes a model's materials for an outfit: the outfit's
 * texture swaps, and each slot's tint (0xRRGGBB, 0 for none). The result
 * is the caller's to dispose of; textures stay shared.
 */
export function dressMaterials(
  asset: CharacterModelAsset,
  outfit: ModelOutfit | null,
  tints: number[],
): THREE.Material[] {
  const swaps = new Map((outfit?.swaps ?? []).map((s) => [s.from.toLowerCase(), s]))
  return asset.materials.map((base, i) => {
    const name = asset.textures[i] ?? ''
    const swap = swaps.get(name.toLowerCase())
    const slot = swap ? swap.slot : textureSlot(name)
    const tint = slot >= 0 ? tints[slot] ?? 0 : 0
    if (!swap && !tint) return base.clone()
    const m = base.clone() as THREE.MeshLambertMaterial
    if (swap) {
      const map = texture(swap.archive, swap.name, false)
      m.map = map
      if ('emissiveMap' in m) m.emissiveMap = map
    }
    if (tint && m.color) {
      // The client's tints are dark (Cobalt is 0x323264), so they're doubled.
      const c = new THREE.Color().setRGB(
        ((tint >> 16) & 0xff) / 127.5, ((tint >> 8) & 0xff) / 127.5, (tint & 0xff) / 127.5,
        THREE.SRGBColorSpace,
      )
      m.color = c
      if (m.emissive) m.emissive = c.clone()
    }
    m.needsUpdate = true
    return m
  })
}

/** createCharacterMesh makes a mesh of a loaded model, sharing its data. */
export function createCharacterMesh(asset: CharacterModelAsset): THREE.Mesh {
  const mesh = new THREE.Mesh(asset.geometry, asset.materials)
  mesh.userData.characterModel = asset.ref
  return mesh
}

function buildAsset(
  ref: CharacterModelRef,
  data: ZoneMesh3D,
  fixedOffset?: [number, number, number],
): CharacterModelAsset {
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
  // Stand the model on its feet at the origin (an attached item moves
  // with the model it's attached to).
  const offset: [number, number, number] = fixedOffset ?? [(minX + maxX) / 2, (minY + maxY) / 2, minZ]
  const [cx, cy, cz] = offset
  for (let i = 0; i < pos.length; i += 3) {
    pos[i] -= cx
    pos[i + 1] -= cy
    pos[i + 2] -= cz
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
  const textures = data.materials.map((m) => m.texture)
  return { ref, geometry, materials, textures, height: maxZ - minZ, offset }
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

/** DressedCharacter is a ready-to-draw character: the model and what it
 * holds, as one object. */
export interface DressedCharacter {
  object: THREE.Group
  /** The model as loaded (with its helm and robe swaps). */
  asset: CharacterModelAsset
  /** The model the request resolved to, before swaps. */
  base: CharacterModelAsset
  /** Slots with no texture for this model, left as they were. */
  missing: number[]
  /** Slots shown with a stand-in texture. */
  fallback: number[]
  /** Held items with no model in the client. */
  missingItems: ('primary' | 'secondary')[]
  /** Frees the materials made for this outfit (geometry and textures are
   * shared and stay). */
  dispose: () => void
}

/**
 * buildCharacter loads a character model, dressed when dress is given:
 * armor textures and tints, helm and robe meshes, and the items in its
 * hands. Resolves null when the client has no model for the race.
 */
export async function buildCharacter(
  req: CharacterModelRequest,
  dress?: ModelDress | null,
): Promise<DressedCharacter | null> {
  const base = await loadCharacterModel(req)
  if (!base) return null
  let asset = base
  let outfit: ModelOutfit | null = null
  if (dress) {
    outfit = await getModelOutfit(base.ref, dress.mats, dress.monk)
    asset = (await loadModelVariant(base.ref, { helm: outfit.helm, robe: outfit.robe })) ?? base
  }
  const held: { which: 'primary' | 'secondary'; item: CharacterModelAsset | null }[] = []
  if (dress?.primary) {
    held.push({ which: 'primary', item: await loadAttachedItem(asset, 'R_POINT', dress.primary).catch(() => null) })
  }
  if (dress?.secondary) {
    const point = dress.shield ? 'SHIELD_POINT' : 'L_POINT'
    held.push({ which: 'secondary', item: await loadAttachedItem(asset, point, dress.secondary).catch(() => null) })
  }
  const object = new THREE.Group()
  const owned: THREE.Material[] = []
  if (dress) {
    const mats = dressMaterials(asset, outfit, dress.tints)
    owned.push(...mats)
    object.add(new THREE.Mesh(asset.geometry, mats))
  } else {
    object.add(createCharacterMesh(asset))
  }
  for (const h of held) if (h.item) object.add(createCharacterMesh(h.item))
  return {
    object,
    asset,
    base,
    missing: outfit?.missing ?? [],
    fallback: [...new Set((outfit?.swaps ?? []).filter((w) => w.fallback).map((w) => w.slot))],
    missingItems: held.filter((h) => !h.item).map((h) => h.which),
    dispose: () => owned.forEach((m) => m.dispose()),
  }
}

// Standing heights of the player races in game units (the client's
// defaults), since models aren't drawn at their own size: every Luclin
// model is about as tall as a human.
const RACE_HEIGHTS: Record<number, number> = {
  1: 6, 2: 7, 3: 6, 4: 5, 5: 6, 6: 5, 7: 5.5, 8: 4, 9: 8, 10: 9,
  11: 3.5, 12: 3, 128: 6, 130: 7, 330: 5,
}

/** raceHeight is how tall a player race stands, or null for other races. */
export function raceHeight(race: number): number | null {
  return RACE_HEIGHTS[race] ?? null
}

// Character models (players and NPCs) as three.js objects, read from the
// user's own EverQuest client through the backend (api/models.go): the
// model the client draws for a race and gender, posed standing, with its
// textures. Shared by every 3D view that shows people: the dressing room,
// and the raid replay and live map once they draw models.
//
// Each model is loaded once: callers get their own THREE.Mesh, but the
// geometry, materials and textures are shared, so a raid of 72 copies
// costs one download. Models and textures are counted out to the
// characters using them, and freed (from the GPU too) once nothing uses
// them and a few dozen others have been let go since, so going back to
// one just seen is instant but browsing hundreds doesn't keep them all.

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
  AttachPoint,
  CharacterMeshOptions,
  CharacterModelRef,
  ModelLooks,
  ModelOutfit,
  WornModel,
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
  /** Face, hair and eyes when the dress doesn't say. */
  looks?: ModelLooks
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
  /** The pooled textures its materials hold. */
  textureKeys: string[]
}

/**
 * Pool shares values by key and counts their users. A value nobody uses
 * is kept until keepIdle others have gone unused since, then freed.
 */
class Pool<T> {
  private items = new Map<string, { value: T; refs: number; free: () => void }>()
  // Unused keys, longest unused first.
  private idle = new Set<string>()

  constructor(private keepIdle: number) {}

  /** acquire returns key's value, making it the first time, and counts a
   * user, who must release it. */
  acquire(key: string, make: () => { value: T; free: () => void }): T {
    let it = this.items.get(key)
    if (!it) {
      it = { ...make(), refs: 0 }
      this.items.set(key, it)
    }
    it.refs++
    this.idle.delete(key)
    return it.value
  }

  release(key: string): void {
    const it = this.items.get(key)
    if (!it || it.refs <= 0) return
    if (--it.refs > 0) return
    this.idle.add(key)
    for (const k of this.idle) {
      if (this.idle.size <= this.keepIdle) break
      this.idle.delete(k)
      this.items.get(k)?.free()
      this.items.delete(k)
    }
  }

  /** forget drops a key whose value failed, so it's made again next time
   * (its users' releases then do nothing). */
  forget(key: string, value: T): void {
    if (this.items.get(key)?.value !== value) return
    this.items.delete(key)
    this.idle.delete(key)
  }
}

// FULL_TEXTURES is the texture size close-up views draw models with (the
// backend's largest).
export const FULL_TEXTURES = 256

// How many unused models and textures are kept for reuse. Character
// textures are small (most 256×256), so this holds tens of MB at most.
const assetPool = new Pool<Promise<CharacterModelAsset | null>>(24)
const texturePool = new Pool<THREE.Texture>(64)
// Called when a texture arrives, so open views can redraw.
const listeners = new Set<() => void>()

/** onCharacterTexture registers a redraw for when model textures load. */
export function onCharacterTexture(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

// Holds is what one character or item uses from the pools, released
// together when it's disposed.
class Holds {
  assets: string[] = []
  textures: string[] = []

  /** size is the largest texture side these models are drawn with. */
  constructor(readonly size: number = FULL_TEXTURES) {}

  asset(name: string, load: () => Promise<CharacterModelAsset | null>): Promise<CharacterModelAsset | null> {
    // An asset's materials hold its textures, so each size is its own.
    const key = `${this.size}|${name}`
    this.assets.push(key)
    return assetPool.acquire(key, () => {
      const value = load()
      value.catch(() => assetPool.forget(key, value))
      return { value, free: () => value.then((a) => a && freeAsset(a)).catch(() => {}) }
    })
  }

  texture(archive: string, name: string, masked: boolean): THREE.Texture {
    const key = `${archive}|${name}|${masked ? 1 : 0}|${this.size}`
    this.textures.push(key)
    return acquireTexture(key, archive, name, masked, this.size)
  }

  release(): void {
    this.assets.forEach((k) => assetPool.release(k))
    this.textures.forEach((k) => texturePool.release(k))
    this.assets = []
    this.textures = []
  }
}

// Which model a request resolves to, kept for the session (a few strings
// each).
const resolved = new Map<string, Promise<CharacterModelRef | null>>()

function requestKey(r: CharacterModelRequest): string {
  return `${r.race}|${r.gender}|${r.zone ?? ''}|${r.luclin ? 1 : 0}`
}

function resolveModel(r: CharacterModelRequest): Promise<CharacterModelRef | null> {
  const key = requestKey(r)
  let p = resolved.get(key)
  if (!p) {
    p = resolveCharacterModel(r.race, r.gender, { zone: r.zone, luclin: r.luclin })
    // A failed ask isn't kept, so it's retried next time.
    p.catch(() => resolved.delete(key))
    resolved.set(key, p)
  }
  return p
}

// loadCharacterModel resolves and loads the model for a race and gender,
// or null when the client has none. Two requests can resolve to one model
// (a zone without its own copy) and share it.
async function loadCharacterModel(holds: Holds, r: CharacterModelRequest): Promise<CharacterModelAsset | null> {
  const ref = await resolveModel(r)
  return ref ? loadModelVariant(holds, ref, {}) : null
}

// loadModelVariant loads a resolved model with mesh swaps (helm, robe).
function loadModelVariant(
  holds: Holds,
  ref: CharacterModelRef,
  opts: CharacterMeshOptions,
): Promise<CharacterModelAsset | null> {
  const key = !opts.helm && !opts.robe
    ? `ref|${ref.archive}|${ref.code}`
    : `variant|${ref.archive}|${ref.code}|${opts.helm ?? 0}|${opts.robe ? 1 : 0}`
  return holds.asset(key, () =>
    getCharacterMesh3D(ref, opts.helm || opts.robe ? opts : undefined)
      .then((data) => (data ? buildAsset(ref, data, holds.size) : null)))
}

// Where item models are, by idfile ("IT63"), kept for the session.
const located = new Map<string, Promise<CharacterModelRef | null>>()

function locateItem(idfile: string): Promise<CharacterModelRef | null> {
  let p = located.get(idfile)
  if (!p) {
    p = locateItemModel(idfile)
    p.catch(() => located.delete(idfile))
    located.set(idfile, p)
  }
  return p
}

// loadAttachedItem loads an item model (idfile "IT63") placed at one of a
// loaded model's attachment points, or null when the client has no such
// item model or the model no such point.
async function loadAttachedItem(
  holds: Holds,
  body: CharacterModelAsset,
  point: AttachPoint,
  idfile: string,
): Promise<CharacterModelAsset | null> {
  const item = await locateItem(idfile)
  if (!item) return null
  return loadWornModel(holds, body, point, item)
}

// loadWornModel loads a resolved item model (a held item, or a Luclin
// model's hair, robe or plate) hung from one of a loaded model's bones, or
// null when the model has no such bone.
function loadWornModel(
  holds: Holds,
  body: CharacterModelAsset,
  point: AttachPoint,
  item: CharacterModelRef,
): Promise<CharacterModelAsset | null> {
  const key = `attach|${body.ref.archive}|${body.ref.code}|${point}|${item.archive}|${item.code}`
  return holds.asset(key, () =>
    getAttachedMesh3D(body.ref, point, item)
      .then((data) => (data ? buildAsset(item, data, holds.size, body.offset) : null)))
}

// freeAsset frees a model's geometry and materials, and lets go of its
// textures.
function freeAsset(a: CharacterModelAsset): void {
  a.geometry.dispose()
  a.materials.forEach((m) => m.dispose())
  a.textureKeys.forEach((k) => texturePool.release(k))
}

/** tinted makes materials with one tint (0xRRGGBB, 0 for none); the
 * result is the caller's to dispose of. */
function tinted(materials: THREE.Material[], tint: number): THREE.Material[] {
  return materials.map((base) => {
    const m = base.clone() as THREE.MeshLambertMaterial
    if (tint && m.color) {
      const c = tintColor(tint)
      m.color = c
      if (m.emissive) m.emissive = c.clone()
    }
    return m
  })
}

// tintColor is a client tint as a colour. The client's tints are dark
// (Cobalt is 0x323264), so they're doubled.
function tintColor(tint: number): THREE.Color {
  return new THREE.Color().setRGB(
    ((tint >> 16) & 0xff) / 127.5, ((tint >> 8) & 0xff) / 127.5, (tint & 0xff) / 127.5,
    THREE.SRGBColorSpace,
  )
}

// Body part textures: model code, part, material (or "sk"), variant.
const partTextureRe = /^[a-z]{3}(ch|fa|ft|hn|lg|ua|he)(?:[0-9]{2}|sk)[0-9]{2}\.(?:bmp|dds)$/
const partSlots: Record<string, number> = { he: 0, ch: 1, ua: 2, fa: 3, hn: 4, lg: 5, ft: 6 }

/** textureSlot is the material slot a body texture belongs to, or -1.
 * Heads take no tint: their textures are faces (helms are models). */
function textureSlot(name: string): number {
  const lower = name.toLowerCase()
  if (/^clk[0-9]{4}\.(?:bmp|dds)$/.test(lower)) return 1 // the robed body
  const m = partTextureRe.exec(lower)
  return m && m[1] !== 'he' ? partSlots[m[1]] : -1
}

/**
 * dressMaterials makes a model's materials for an outfit: the outfit's
 * texture swaps, and each slot's tint (0xRRGGBB, 0 for none). The result
 * is the caller's to dispose of; textures stay shared.
 */
function dressMaterials(
  holds: Holds,
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
      const map = holds.texture(swap.archive, swap.name, false)
      m.map = map
      if ('emissiveMap' in m) m.emissiveMap = map
    }
    if (tint && m.color) {
      const c = tintColor(tint)
      m.color = c
      if (m.emissive) m.emissive = c.clone()
    }
    m.needsUpdate = true
    return m
  })
}

// createCharacterMesh makes a mesh of a loaded model, sharing its data.
function createCharacterMesh(asset: CharacterModelAsset): THREE.Mesh {
  const mesh = drawnLoads(new THREE.Mesh(asset.geometry, asset.materials))
  mesh.userData.characterModel = asset.ref
  return mesh
}

function buildAsset(
  ref: CharacterModelRef,
  data: ZoneMesh3D,
  size: number,
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

  const textureKeys: string[] = []
  const materials = data.materials.map((m): THREE.Material => {
    let map: THREE.Texture | null = null
    if (m.texture) {
      const key = `${ref.archive}|${m.texture}|${m.kind === 1 ? 1 : 0}|${size}`
      textureKeys.push(key)
      map = acquireTexture(key, ref.archive, m.texture, m.kind === 1, size)
    }
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
  return { ref, geometry, materials, textures, height: maxZ - minZ, offset, textureKeys }
}

// HTTPError is a texture the backend answered for but couldn't give.
class HTTPError extends Error {}

// TEXTURE_FETCHES is how many character textures load at once. A zone's
// NPCs drawn as models can want a couple of thousand; fetched all at once,
// the browser refuses most of them (ERR_INSUFFICIENT_RESOURCES).
const TEXTURE_FETCHES = 8
let fetching = 0
const fetchQueue: (() => void)[] = []

// queued runs job once fewer than TEXTURE_FETCHES others are running.
function queued<T>(job: () => Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const run = (): void => {
      fetching++
      job().then(resolve, reject).finally(() => {
        fetching--
        fetchQueue.shift()?.()
      })
    }
    if (fetching < TEXTURE_FETCHES) run()
    else fetchQueue.push(run)
  })
}

// Textures whose load hasn't started, and how to start it. A texture
// loads the first time a mesh using it is drawn (see drawnLoads), so a
// model's textures that an outfit replaces, and models off screen, are
// never fetched.
const unstarted = new WeakMap<THREE.Texture, () => void>()

// startMaps is a mesh's onBeforeRender: three.js calls it per material
// group drawn, and not for meshes culled or hidden.
function startMaps(
  _r: THREE.WebGLRenderer, _s: THREE.Scene, _c: THREE.Camera, _g: THREE.BufferGeometry, material: THREE.Material,
): void {
  const map = (material as THREE.MeshLambertMaterial).map
  if (map) unstarted.get(map)?.()
}

// drawnLoads makes mesh start its textures' loads when it's drawn.
function drawnLoads(mesh: THREE.Mesh): THREE.Mesh {
  mesh.onBeforeRender = startMaps
  return mesh
}

// acquireTexture returns a character texture and counts a user of it (see
// texturePool). Its load starts when a mesh using it is first drawn.
function acquireTexture(
  key: string, archive: string, name: string, masked: boolean, size: number,
): THREE.Texture {
  return texturePool.acquire(key, () => {
    const tex = new THREE.Texture()
    let freed = false
    // Client UVs run top-down (DirectX); ImageBitmaps aren't flipped.
    tex.flipY = false
    tex.wrapS = THREE.RepeatWrapping
    tex.wrapT = THREE.RepeatWrapping
    tex.colorSpace = THREE.SRGBColorSpace
    const grey = (): void => {
      tex.image = new ImageData(new Uint8ClampedArray([138, 143, 152, 255]), 1, 1)
      tex.needsUpdate = true
    }
    unstarted.set(tex, () => {
      unstarted.delete(tex)
      // Fetched rather than loaded as an <img>: the page's CSP allows the
      // backend for connections but not for images.
      queued(async () => {
        // Let go of while waiting its turn: nothing to fetch.
        if (freed) return null
        const res = await fetch(await getCharacterTextureUrl(archive, name, masked, size))
        if (!res.ok) throw new HTTPError(res.statusText)
        return res.blob()
      })
        .then((blob) => (blob ? createImageBitmap(blob, { premultiplyAlpha: 'none', colorSpaceConversion: 'none' }) : null))
        .then((bitmap) => {
          if (!bitmap) return
          if (freed) {
            bitmap.close()
            return
          }
          tex.image = bitmap
          tex.needsUpdate = true
        })
        .catch((err) => {
          if (freed) return
          grey()
          // A texture the client lacks stays grey; one that failed to
          // arrive is fetched again by whoever asks for it next.
          if (!(err instanceof HTTPError)) texturePool.forget(key, tex)
        })
        .finally(() => listeners.forEach((fn) => fn()))
    })
    return {
      value: tex,
      free: () => {
        freed = true
        tex.dispose()
        if (tex.image instanceof ImageBitmap) tex.image.close()
      },
    }
  })
}

// Outfits asked for, by model and dress: a zone's NPCs repeat a few
// looks many times over. A failed ask is forgotten so it's tried again.
const outfits = new Map<string, Promise<ModelOutfit>>()
const OUTFITS_KEPT = 512

function cachedOutfit(ref: CharacterModelRef, dress: ModelDress): Promise<ModelOutfit> {
  const key = JSON.stringify([ref.archive, ref.code, dress.mats, !!dress.monk, dress.looks ?? null])
  let p = outfits.get(key)
  if (!p) {
    p = getModelOutfit(ref, dress.mats, dress.monk, dress.looks)
    p.catch(() => outfits.delete(key))
    outfits.set(key, p)
    // Answers are small, but a long session browsing NPCs asks many.
    if (outfits.size > OUTFITS_KEPT) outfits.delete(outfits.keys().next().value!)
  }
  return p
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
  /** What a Luclin model wears as models (hair, robe, plate). */
  wear: WornModel[]
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
  given?: ModelDress | null,
  textureSize: number = FULL_TEXTURES,
): Promise<DressedCharacter | null> {
  const holds = new Holds(textureSize)
  try {
    const c = await dressCharacter(holds, req, given ?? null)
    if (!c) holds.release()
    return c
  } catch (err) {
    holds.release()
    throw err
  }
}

async function dressCharacter(
  holds: Holds,
  req: CharacterModelRequest,
  given: ModelDress | null,
): Promise<DressedCharacter | null> {
  const base = await loadCharacterModel(holds, req)
  if (!base) return null
  // A Luclin model is always dressed: its hair and the bare body's
  // loincloth are worn models.
  let dress = given
  if (base.ref.archive === `global${base.ref.code.toLowerCase()}_chr.s3d`) {
    dress = dress ?? { mats: [0, 0, 0, 0, 0, 0, 0], tints: [0, 0, 0, 0, 0, 0, 0] }
  }
  if (dress && !dress.looks && req.looks) dress = { ...dress, looks: req.looks }
  else if (!dress && req.looks && req.looks.face > 0) {
    // An original model takes a face too.
    dress = { mats: [0, 0, 0, 0, 0, 0, 0], tints: [0, 0, 0, 0, 0, 0, 0], looks: req.looks }
  }
  let asset = base
  let outfit: ModelOutfit | null = null
  if (dress) {
    outfit = await cachedOutfit(base.ref, dress)
    asset = (await loadModelVariant(holds, base.ref, { helm: outfit.helm, robe: outfit.robe })) ?? base
  }
  const held: { which: 'primary' | 'secondary'; item: CharacterModelAsset | null }[] = []
  if (dress?.primary) {
    held.push({ which: 'primary', item: await loadAttachedItem(holds, asset, 'R_POINT', dress.primary).catch(() => null) })
  }
  if (dress?.secondary) {
    const point = dress.shield ? 'SHIELD_POINT' : 'L_POINT'
    held.push({ which: 'secondary', item: await loadAttachedItem(holds, asset, point, dress.secondary).catch(() => null) })
  }
  // A Luclin model's hair, robe and plate, each tinted as its slot (hair
  // and beards in their own colour).
  const wear = outfit?.wear ?? []
  const worn = await Promise.all(wear.map((w) =>
    loadWornModel(holds, asset, w.point, { code: w.item, archive: w.archive }).catch(() => null)))
  const object = new THREE.Group()
  const owned: THREE.Material[] = []
  worn.forEach((item, i) => {
    if (!item) return
    const w = wear[i]
    const tint = w.tint ?? (w.slot >= 0 ? dress?.tints[w.slot] ?? 0 : 0)
    const mats = tinted(item.materials, tint)
    owned.push(...mats)
    object.add(drawnLoads(new THREE.Mesh(item.geometry, mats)))
  })
  if (dress) {
    const mats = dressMaterials(holds, asset, outfit, dress.tints)
    owned.push(...mats)
    object.add(drawnLoads(new THREE.Mesh(asset.geometry, mats)))
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
    wear,
    dispose: () => {
      owned.forEach((m) => m.dispose())
      holds.release()
    },
  }
}

/** ShownItem is an item model on its own, ready to draw. */
export interface ShownItem {
  object: THREE.Group
  asset: CharacterModelAsset
  /** Lets go of the model (it's shared, and freed once nothing uses it). */
  dispose: () => void
}

/**
 * buildItem loads an item's model (idfile "IT10") on its own, as the
 * game draws it held or on the ground. Resolves null when the client has
 * no such model.
 */
export async function buildItem(idfile: string): Promise<ShownItem | null> {
  const holds = new Holds()
  try {
    const ref = await locateItem(idfile)
    const asset = ref ? await loadModelVariant(holds, ref, {}) : null
    if (!asset) {
      holds.release()
      return null
    }
    const object = new THREE.Group()
    object.add(createCharacterMesh(asset))
    return { object, asset, dispose: () => holds.release() }
  } catch (err) {
    holds.release()
    throw err
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

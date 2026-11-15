// What a character wears, as the character models draw it: the material
// and tint per armor slot and the models in their hands. Built from items
// (the dressing room's try-ons, or what a Zeal inventory export says is
// equipped). Shared by the dressing room and the 3D map's people.

import { getItemAppearances, getZealInventory } from '../services/api'
import type { ItemAppearance, ModelLooks } from '../services/api'
import type { NPCLook } from '../types/map'

/** ModelDress is what a model wears. */
export interface ModelDress {
  /** Material per slot: head, chest, arms, wrist, hands, legs, feet. */
  mats: number[]
  /** Tint per slot, 0xRRGGBB, 0 for none. */
  tints: number[]
  /** Model (idfile, "IT1") of the primary hand's item. */
  primary?: string
  /** Model of the secondary hand's item; shield goes on the arm. */
  secondary?: string
  shield?: boolean
  /** Only monks show the monk armor material (4). */
  monk?: boolean
  /** Face, hair and eyes; the defaults when left out. */
  looks?: ModelLooks
}

export interface DressSlot {
  /** Zeal inventory location. */
  key: string
  label: string
  /** items.slots bits for the slot. */
  mask: number
  /** Material slot (0 head … 6 feet), or -1 for a held item. */
  material: number
}

export const DRESS_SLOTS: DressSlot[] = [
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
/** The app's 0-based class index for Monk. */
export const MONK_CLASS = 6

function heldModel(item: ItemAppearance | null | undefined): string | undefined {
  const f = item?.id_file?.toUpperCase() ?? ''
  return /^IT[1-9][0-9]*$/.test(f) ? f : undefined
}

/** dressFor builds what a character wears from the item in each slot. */
export function dressFor(worn: (slot: string) => ItemAppearance | null, monk: boolean): ModelDress {
  const mats = [0, 0, 0, 0, 0, 0, 0]
  const tints = [0, 0, 0, 0, 0, 0, 0]
  for (const s of DRESS_SLOTS) {
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
    monk,
  }
}

/** EquippedItems is a character's gear from their Zeal inventory export. */
export interface EquippedItems {
  items: Record<string, ItemAppearance>
  /** When the export was written; null when there's none. */
  exportedAt: string | null
}

/**
 * loadEquippedItems reads what a character has equipped from their latest
 * Zeal inventory export. The first of a doubled slot (Wrist) is the one the
 * client draws.
 */
export async function loadEquippedItems(character: string): Promise<EquippedItems> {
  const res = await getZealInventory(character)
  const inv = res.inventory
  if (!inv) return { items: {}, exportedAt: null }
  const bySlot = new Map<string, number>()
  for (const e of inv.entries) {
    if (DRESS_SLOTS.some((s) => s.key === e.location) && !bySlot.has(e.location) && e.id > 0) {
      bySlot.set(e.location, e.id)
    }
  }
  if (bySlot.size === 0) return { items: {}, exportedAt: inv.exported_at }
  const found = await getItemAppearances([...bySlot.values()])
  const byID = new Map(found.map((i) => [i.id, i]))
  const items: Record<string, ItemAppearance> = {}
  for (const [slot, id] of bySlot) {
    const it = byID.get(id)
    if (it) items[slot] = it
  }
  return { items, exportedAt: inv.exported_at }
}

/** The player races, in the order of items' race bits (items.races bit 0
 * is Human … bit 13 Vah Shir). */
export const PLAYER_RACES = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 128, 130]

/** isPlayerRace is whether race has the player models (and so wears
 * armor piece by piece). */
export function isPlayerRace(race: number): boolean {
  return PLAYER_RACES.includes(race) || race === 330
}

/** racesForMask is the player races an item's race bits allow (all of
 * them for 0 or the all-races sentinels). */
export function racesForMask(mask: number): number[] {
  if (mask <= 0 || mask >= 16383) return PLAYER_RACES
  return PLAYER_RACES.filter((_, i) => mask & (1 << i))
}

/** armorSlots is the material slots (0 head … 6 feet) an item's slot bits
 * cover. */
export function armorSlots(slotMask: number): number[] {
  return DRESS_SLOTS.filter((s) => s.material >= 0 && slotMask & s.mask).map((s) => s.material)
}

/** isHeldModel is whether an item's idfile is a model drawn in the hand
 * (IT63, the bag every other item drops as, isn't). */
export function isHeldModel(idfile: string | undefined): boolean {
  const f = heldModel(idfile ? { id_file: idfile } as ItemAppearance : null)
  return !!f && f !== 'IT63'
}

/**
 * npcDress is what an NPC wears as the character models draw it: player
 * races dressed piece by piece as the server sends it, other races in
 * their texture set. dress is null when it wears nothing.
 */
export function npcDress(l: NPCLook): { dress: ModelDress | null; looks?: ModelLooks; player: boolean } {
  const player = isPlayerRace(l.race)
  // Another race wears its texture set, with its own head texture
  // (helmtexture, which the server keeps apart).
  const mats = player ? l.mats : [l.mats[0] ?? 0, ...Array(6).fill(l.texture)]
  const dressed = mats.some((m) => m > 0) || l.tints.some((t) => t > 0) || !!l.primary || !!l.secondary
  const looks = player
    ? {
        face: l.face, hair: l.hair, hair_color: l.hair_color,
        beard: l.beard, beard_color: l.beard_color, eyes: l.eyes,
      }
    : undefined
  return {
    dress: dressed ? { mats, tints: l.tints, primary: l.primary, secondary: l.secondary } : null,
    looks,
    player,
  }
}

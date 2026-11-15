// What a character wears, as the character models draw it: the material
// and tint per armor slot and the models in their hands. Built from items
// (the dressing room's try-ons, or what a Zeal inventory export says is
// equipped). Shared by the dressing room and the 3D map's people.

import { getItemAppearances, getZealInventory } from '../services/api'
import type { ItemAppearance } from '../services/api'

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
  /** Luclin hairstyle, 0-2, or -1 for none; 0 when left out. */
  hair?: number
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

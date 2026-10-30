// Section, field and list-limit definitions for the Item Info overlay's
// summary card. Both overlay surfaces and the Settings editor resolve layout
// here. Visibility reuses the NPC overlay's hidden-key scheme (see
// npcItemFilters): "section.<id>" hides a section, "<id>.<field>" one row.

import type { ItemInfoOverlaySettings } from '../types/config'

export type ItemInfoSectionId =
  | 'facts'
  | 'effects'
  | 'ownership'
  | 'tradeskills'
  | 'quests'
  | 'vendors'
  | 'drops'
  | 'sources'

export type EmptyMode = 'show' | 'hide'

export interface ItemInfoSectionDef {
  id: ItemInfoSectionId
  label: string
  hint: string
  fields: Array<{ key: string; label: string; hint?: string }>
  // Sections with a list that collapses to a count past the list limit.
  listed: boolean
  // Default when the section has nothing; undefined = always hidden when
  // empty (no "no" line makes sense, e.g. an item without effects).
  emptyDefault?: EmptyMode
}

export const ITEM_INFO_SECTIONS: ItemInfoSectionDef[] = [
  {
    id: 'facts',
    label: 'Key Facts',
    hint: 'The few stats that matter for this kind of item',
    listed: false,
    fields: [
      { key: 'facts.type', label: 'Item type' },
      { key: 'facts.flags', label: 'Flags', hint: 'MAGIC, LORE, NO DROP, NO RENT' },
      { key: 'facts.slot', label: 'Slots' },
      { key: 'facts.ac', label: 'AC' },
      { key: 'facts.weapon', label: 'Damage / delay' },
      { key: 'facts.hpmana', label: 'HP / Mana' },
      { key: 'facts.stats', label: 'Attributes', hint: 'STR, STA, AGI, DEX, WIS, INT, CHA' },
      { key: 'facts.resists', label: 'Resists' },
      { key: 'facts.level', label: 'Required / recommended level' },
      { key: 'facts.classes', label: 'Classes' },
      { key: 'facts.races', label: 'Races' },
      { key: 'facts.stack', label: 'Stack size' },
      { key: 'facts.bag', label: 'Bag slots / size' },
      { key: 'facts.weight', label: 'Weight' },
      { key: 'facts.size', label: 'Size' },
      { key: 'facts.value', label: 'Value' },
    ],
  },
  {
    id: 'effects',
    label: 'Effects',
    hint: 'Click, worn, focus, proc and scroll spells',
    listed: false,
    fields: [
      { key: 'effects.scroll', label: 'Teaches (scrolls)' },
      { key: 'effects.click', label: 'Click' },
      { key: 'effects.worn', label: 'Worn' },
      { key: 'effects.focus', label: 'Focus' },
      { key: 'effects.proc', label: 'Proc' },
    ],
  },
  {
    id: 'ownership',
    label: 'Characters',
    hint: 'Which of your characters hold it, and where',
    listed: true,
    // The header badge already says "Not on any character".
    emptyDefault: 'hide',
    fields: [],
  },
  {
    id: 'tradeskills',
    label: 'Tradeskills',
    hint: 'Recipes that make or use it, by discipline and trivial',
    listed: true,
    emptyDefault: 'hide',
    fields: [
      { key: 'tradeskills.produced', label: 'Produced by' },
      { key: 'tradeskills.used', label: 'Used in' },
    ],
  },
  {
    id: 'quests',
    label: 'Quests',
    hint: 'Quests that reward it or take it as a turn-in',
    listed: true,
    emptyDefault: 'hide',
    fields: [
      { key: 'quests.reward', label: 'Rewarded by' },
      { key: 'quests.used', label: 'Turned in to' },
    ],
  },
  {
    id: 'vendors',
    label: 'Vendors',
    hint: 'Whether you can buy it, and from whom',
    listed: true,
    emptyDefault: 'show',
    fields: [{ key: 'vendors.price', label: 'Base price' }],
  },
  {
    id: 'drops',
    label: 'Drops From',
    hint: 'NPCs that drop it, best rate first',
    listed: true,
    emptyDefault: 'hide',
    fields: [{ key: 'drops.rate', label: 'Drop rate' }],
  },
  {
    id: 'sources',
    label: 'Other Sources',
    hint: 'Forage zones and ground spawns',
    listed: true,
    emptyDefault: 'hide',
    fields: [
      { key: 'sources.forage', label: 'Forage' },
      { key: 'sources.ground', label: 'Ground spawns' },
    ],
  },
]

// Header controls aren't a reorderable section but toggle the same way.
export const ITEM_INFO_HEADER_FIELDS: Array<{ key: string; label: string; hint?: string }> = [
  { key: 'header.badge', label: 'Ownership badge', hint: '"You have this" under the name' },
  { key: 'header.wishlist', label: 'Wishlist star' },
  { key: 'header.open', label: 'Clickable item name', hint: 'Opens the item in PQ Companion' },
]

export const DEFAULT_ITEM_INFO_LIST_LIMIT = 5

const DEFAULT_ORDER = ITEM_INFO_SECTIONS.map((s) => s.id)

export function sectionDef(id: ItemInfoSectionId): ItemInfoSectionDef {
  return ITEM_INFO_SECTIONS.find((s) => s.id === id) as ItemInfoSectionDef
}

// Saved orders are partial-tolerant, like the NPC overlay's: unknown ids are
// dropped and missing ones keep their default slot at the end.
export function resolveItemInfoOrder(saved: string[] | null | undefined): ItemInfoSectionId[] {
  const known = new Set<string>(DEFAULT_ORDER)
  const out = (saved ?? []).filter(
    (id, i, arr): id is ItemInfoSectionId => known.has(id) && arr.indexOf(id) === i,
  )
  for (const id of DEFAULT_ORDER) if (!out.includes(id)) out.push(id)
  return out
}

export function listLimitFor(
  s: ItemInfoOverlaySettings | undefined,
  id: ItemInfoSectionId,
): number {
  const own = s?.section_limits?.[id]
  if (own && own > 0) return own
  return s?.list_limit && s.list_limit > 0 ? s.list_limit : DEFAULT_ITEM_INFO_LIST_LIMIT
}

export function emptyModeFor(
  s: ItemInfoOverlaySettings | undefined,
  id: ItemInfoSectionId,
): EmptyMode {
  const def = sectionDef(id).emptyDefault
  if (!def) return 'hide'
  const own = s?.empty_modes?.[id]
  return own === 'show' || own === 'hide' ? own : def
}

import {
  baneBodyLabel as enumBaneBodyLabel,
  baneRaceLabel as enumBaneRaceLabel,
  decodeItemClasses,
  decodeItemRaces,
  decodeItemSlots,
  itemTypeLabel as enumItemTypeLabel,
} from './enumsCache'

// ── In-game item link ─────────────────────────────────────────────────────────

// Project Quarm runs on EQMacEmu, which uses the classic Mac-era link format
// rather than the modern live-EQ `\aITEM 0 0 0…:Name\a/` form. The link is
// wrapped in DC2 (CTRL-R, 0x12) control characters with the item ID padded to
// 6 decimal digits ahead of the name, e.g. PQDI produces:
//   \x12025208 War Bow of Rallos Zek\x12
// EQ strips the DC2 chars and renders the name as a clickable link in chat.
const DC2 = '\x12'

export function inGameItemLink(itemId: number, itemName: string): string {
  const paddedId = String(itemId).padStart(6, '0')
  return `${DC2}${paddedId} ${itemName}${DC2}`
}

// ── Slot / Class / Race bitmasks ───────────────────────────────────────────────
//
// Bit → label maps live in the canonical Go catalog
// (backend/internal/db/enums/item_bitmasks.go). Decomposition lives on
// the client via the cache helpers so the API stays a thin static
// payload.

export function decodeSlots(mask: number): string[] {
  return decodeItemSlots(mask)
}

export function slotsLabel(mask: number): string {
  if (mask === 0) return 'None'
  return decodeSlots(mask).join(', ')
}

export function decodeClasses(mask: number): string[] {
  return decodeItemClasses(mask)
}

export function classesLabel(mask: number): string {
  return decodeClasses(mask).join(', ')
}

export function decodeRaces(mask: number): string[] {
  return decodeItemRaces(mask)
}

export function racesLabel(mask: number): string {
  return decodeRaces(mask).join(', ')
}

// ── Item type ──────────────────────────────────────────────────────────────────

// Item type labels live in the canonical Go catalog
// (backend/internal/db/enums/item_type.go). itemTypeLabel re-exports the
// cache-backed helper so existing call sites still import from this
// module.

export function itemTypeLabel(itemType: number): string {
  return enumItemTypeLabel(itemType)
}

/** Returns the display label accounting for item_class (container/book) overrides. */
export function effectiveItemTypeLabel(itemClass: number, itemType: number): string {
  if (itemClass === 1) return 'Container'
  if (itemClass === 2) return 'Book'
  return itemTypeLabel(itemType)
}

/** Returns true if the item is a lore (unique) item — EQ stores this as lore string starting with '*'. */
export function isLoreItem(lore: string): boolean {
  return lore.startsWith('*')
}

// ── Size ───────────────────────────────────────────────────────────────────────

const SIZE_NAMES = ['Tiny', 'Small', 'Medium', 'Large', 'Giant', 'Gigantic']

export function sizeLabel(size: number): string {
  return SIZE_NAMES[size] ?? `Size ${size}`
}

// ── Price (copper → pp/gp/sp/cp) ──────────────────────────────────────────────

export function priceLabel(copper: number): string {
  if (copper === 0) return '0 cp'
  const pp = Math.floor(copper / 1000)
  const gp = Math.floor((copper % 1000) / 100)
  const sp = Math.floor((copper % 100) / 10)
  const cp = copper % 10
  const parts: string[] = []
  if (pp) parts.push(`${pp}pp`)
  if (gp) parts.push(`${gp}gp`)
  if (sp) parts.push(`${sp}sp`)
  if (cp) parts.push(`${cp}cp`)
  return parts.join(' ')
}

// ── Bane damage body type ──────────────────────────────────────────────────────
//
// Bane body labels live in the canonical Go catalog
// (backend/internal/db/enums/bane_body.go).

export function baneBodyLabel(bodyType: number): string {
  return enumBaneBodyLabel(bodyType)
}

// ── Bane damage race ──────────────────────────────────────────────────────────

// Bane race labels live in the canonical Go catalog
// (backend/internal/db/enums/bane_race.go).
export function baneRaceLabel(raceId: number): string {
  return enumBaneRaceLabel(raceId)
}

// ── Weight (tenths of a pound) ─────────────────────────────────────────────────

export function weightLabel(w: number): string {
  return `${(w / 10).toFixed(1)}`
}

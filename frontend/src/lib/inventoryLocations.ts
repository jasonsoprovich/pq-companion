// Helpers for interpreting the `Location` column of Zeal inventory exports,
// shared by the Inventory Tracker and the item detail "Characters" tab.

// Bag containers and slots use either ":" or "-" as separator depending on the
// Zeal version. Treat both consistently.
const BAG_SLOT_RE = /^(General|Bank|SharedBank)(\d+)[:\-]Slot(\d+)$/
const BAG_CONTAINER_RE = /^(General|Bank|SharedBank)(\d+)$/

export function bagSlotInfo(location: string): { kind: string; bag: number; slot: number } | null {
  const m = location.match(BAG_SLOT_RE)
  if (!m) return null
  return { kind: m[1], bag: parseInt(m[2], 10), slot: parseInt(m[3], 10) }
}

export function bagContainerInfo(location: string): { kind: string; bag: number } | null {
  const m = location.match(BAG_CONTAINER_RE)
  if (!m) return null
  return { kind: m[1], bag: parseInt(m[2], 10) }
}

export function isEquipmentSlot(location: string): boolean {
  if (bagSlotInfo(location) || bagContainerInfo(location)) return false
  if (location === 'Cursor' || location === 'Held') return false
  if (location === 'General-Coin' || location.endsWith('-Coin')) return false
  return true
}

function bagLabel(kind: string, bag: number): string {
  if (kind === 'General') return `Bag ${bag}`
  if (kind === 'SharedBank') return `Shared Bank ${bag}`
  return `${kind} ${bag}`
}

// describeLocation renders a Location string as a human-friendly label:
// "General3:Slot5" → "Bag 3, Slot 5"; "Bank2" → "Bank 2 (bag)";
// "Head" → "Equipped — Head"; "Cursor" stays as-is.
export function describeLocation(location: string): string {
  const slot = bagSlotInfo(location)
  if (slot) return `${bagLabel(slot.kind, slot.bag)}, Slot ${slot.slot}`
  const cont = bagContainerInfo(location)
  if (cont) return `${bagLabel(cont.kind, cont.bag)} (bag)`
  if (isEquipmentSlot(location)) return `Equipped — ${location}`
  return location
}

// Bitmask-type equipment slot options (paired slots like Ear/Wrist/Fingers
// collapsed to one bit each, matching items.slots) — canonical source for the
// Items page slot filter and the item Compare tool's slot picker. Mirrors
// backend/internal/db/enums/item_bitmasks.go; keep in sync if that changes.
export interface ItemSlotOption {
  value: number
  label: string
}

export const ITEM_SLOTS: ItemSlotOption[] = [
  { value: 0x000001, label: 'Charm' },
  { value: 0x000012, label: 'Ear' },
  { value: 0x000004, label: 'Head' },
  { value: 0x000008, label: 'Face' },
  { value: 0x000020, label: 'Neck' },
  { value: 0x000040, label: 'Shoulder' },
  { value: 0x000080, label: 'Arms' },
  { value: 0x000100, label: 'Back' },
  { value: 0x000600, label: 'Wrist' },
  { value: 0x000800, label: 'Range' },
  { value: 0x001000, label: 'Hands' },
  { value: 0x002000, label: 'Primary' },
  { value: 0x004000, label: 'Secondary' },
  { value: 0x018000, label: 'Finger' },
  { value: 0x020000, label: 'Chest' },
  { value: 0x040000, label: 'Legs' },
  { value: 0x080000, label: 'Feet' },
  { value: 0x100000, label: 'Waist' },
  // Quarm-specific correction: 0x200000, not the modern-EQEmu 0x800000 — see
  // backend/internal/db/enums/item_bitmasks.go.
  { value: 0x200000, label: 'Ammo' },
]

/** The first slot option an item's bitmask fits, or null if none match. */
export function primarySlotFor(itemSlots: number): ItemSlotOption | null {
  return ITEM_SLOTS.find((o) => (itemSlots & o.value) !== 0) ?? null
}

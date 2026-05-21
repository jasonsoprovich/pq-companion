// Wishlist slot buckets. Kept in EQ canonical worn-slot order so each section
// renders top-to-bottom the way a character sheet does. The "General" bucket
// at the end holds non-equippable items.
//
// Bucket names match backend/internal/api/wishlist.go validSlotBuckets — keep
// in sync.

export const WISHLIST_SLOT_ORDER: readonly string[] = [
  'Charm',
  'Ear',
  'Head',
  'Face',
  'Neck',
  'Shoulder',
  'Arms',
  'Back',
  'Wrist',
  'Range',
  'Hands',
  'Primary',
  'Secondary',
  'Finger',
  'Chest',
  'Legs',
  'Feet',
  'Waist',
  'Ammo',
  'General',
] as const

export const GENERAL_BUCKET = 'General'

// Mirror of backend/internal/db/enums/item_bitmasks.go itemSlotBits. Two bits
// (Ear left/right, Finger left/right, Wrist left/right) share the same label
// because the wishlist groups by label, not raw bit.
const SLOT_BIT_TO_LABEL: { bit: number; label: string }[] = [
  { bit: 0x000001, label: 'Charm' },
  { bit: 0x000002, label: 'Ear' },
  { bit: 0x000004, label: 'Head' },
  { bit: 0x000008, label: 'Face' },
  { bit: 0x000010, label: 'Ear' },
  { bit: 0x000020, label: 'Neck' },
  { bit: 0x000040, label: 'Shoulder' },
  { bit: 0x000080, label: 'Arms' },
  { bit: 0x000100, label: 'Back' },
  { bit: 0x000200, label: 'Wrist' },
  { bit: 0x000400, label: 'Wrist' },
  { bit: 0x000800, label: 'Range' },
  { bit: 0x001000, label: 'Hands' },
  { bit: 0x002000, label: 'Primary' },
  { bit: 0x004000, label: 'Secondary' },
  { bit: 0x008000, label: 'Finger' },
  { bit: 0x010000, label: 'Finger' },
  { bit: 0x020000, label: 'Chest' },
  { bit: 0x040000, label: 'Legs' },
  { bit: 0x080000, label: 'Feet' },
  { bit: 0x100000, label: 'Waist' },
  { bit: 0x200000, label: 'Ammo' },
]

// validSlotsForItem returns the deduplicated list of wishlist buckets an item
// could be added to, in canonical display order. Non-equippable items (slots
// mask == 0) return ["General"].
export function validSlotsForItem(slots: number): string[] {
  if (!slots) return [GENERAL_BUCKET]
  const seen = new Set<string>()
  const out: string[] = []
  for (const { bit, label } of SLOT_BIT_TO_LABEL) {
    if ((slots & bit) !== 0 && !seen.has(label)) {
      seen.add(label)
      out.push(label)
    }
  }
  return out.length > 0 ? out : [GENERAL_BUCKET]
}

// isMultiSlotItem returns true when the user would need to choose between
// more than one slot when wishlisting this item.
export function isMultiSlotItem(slots: number): boolean {
  return validSlotsForItem(slots).length > 1
}

// compareSlotBuckets orders buckets by canonical worn-slot order; unknown
// names sort to the end so legacy data isn't lost.
export function compareSlotBuckets(a: string, b: string): number {
  const ai = WISHLIST_SLOT_ORDER.indexOf(a)
  const bi = WISHLIST_SLOT_ORDER.indexOf(b)
  if (ai === -1 && bi === -1) return a.localeCompare(b)
  if (ai === -1) return 1
  if (bi === -1) return -1
  return ai - bi
}

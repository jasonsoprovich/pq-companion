// ── Slot bitmask ───────────────────────────────────────────────────────────────

const SLOT_MAP: [number, string][] = [
  [0x000001, 'Charm'],
  [0x000002, 'Ear'],
  [0x000004, 'Head'],
  [0x000008, 'Face'],
  [0x000010, 'Ear'],
  [0x000020, 'Neck'],
  [0x000040, 'Shoulder'],
  [0x000080, 'Arms'],
  [0x000100, 'Back'],
  [0x000200, 'Wrist'],
  [0x000400, 'Wrist'],
  [0x000800, 'Range'],
  [0x001000, 'Hands'],
  [0x002000, 'Primary'],
  [0x004000, 'Secondary'],
  [0x008000, 'Finger'],
  [0x010000, 'Finger'],
  [0x020000, 'Chest'],
  [0x040000, 'Legs'],
  [0x080000, 'Feet'],
  [0x100000, 'Waist'],
  [0x800000, 'Ammo'],
]

export function decodeSlots(mask: number): string[] {
  return SLOT_MAP.filter(([bit]) => (mask & bit) !== 0).map(([, name]) => name)
}

export function slotsLabel(mask: number): string {
  if (mask === 0) return 'None'
  const names = decodeSlots(mask)
  return names.join(', ')
}

// ── Class bitmask ──────────────────────────────────────────────────────────────

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight',
  'Druid', 'Monk', 'Bard', 'Rogue', 'Shaman',
  'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

const ALL_CLASSES_MASK = (1 << 15) - 1 // 0x7FFF

export function decodeClasses(mask: number): string[] {
  if (mask === 0 || mask >= ALL_CLASSES_MASK) return ['All']
  return CLASS_NAMES.filter((_, i) => (mask & (1 << i)) !== 0)
}

export function classesLabel(mask: number): string {
  return decodeClasses(mask).join(', ')
}

// ── Race bitmask ───────────────────────────────────────────────────────────────

const RACE_NAMES = [
  'Human', 'Barbarian', 'Erudite', 'Wood Elf', 'High Elf',
  'Dark Elf', 'Half Elf', 'Dwarf', 'Troll', 'Ogre',
  'Halfling', 'Gnome', 'Iksar', 'Vah Shir',
]

const ALL_RACES_MASK = 65535

export function decodeRaces(mask: number): string[] {
  if (mask === 0 || mask >= ALL_RACES_MASK) return ['All']
  return RACE_NAMES.filter((_, i) => (mask & (1 << i)) !== 0)
}

export function racesLabel(mask: number): string {
  return decodeRaces(mask).join(', ')
}

// ── Item type ──────────────────────────────────────────────────────────────────

const ITEM_TYPES: Record<number, string> = {
  0: '1H Slashing',
  1: '2H Slashing',
  2: 'Piercing',
  3: '1H Blunt',
  4: '2H Blunt',
  5: 'Archery',
  7: 'Throwing',
  8: 'Shield',
  10: 'Armor',
  11: 'Gem',
  12: 'Lockpick',
  13: 'Food',
  14: 'Drink',
  15: 'Light',
  16: 'Combinable',
  17: 'Bandage',
  19: 'Spell Scroll',
  20: 'Potion',
  22: 'Wind Instrument',
  23: 'String Instrument',
  24: 'Brass Instrument',
  25: 'Percussion Instrument',
  26: 'Arrow',
  27: 'Jewelry',
  28: 'Note',
  29: 'Key',
  30: 'Coin',
  31: '2H Piercing',
  33: 'Fishing Pole',
  34: 'Fishing Bait',
  35: 'Alcohol',
  39: 'Poison',
}

export function itemTypeLabel(itemType: number): string {
  return ITEM_TYPES[itemType] ?? `Type ${itemType}`
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

const BANE_BODY_TYPES: Record<number, string> = {
  1: 'Humanoid',
  2: 'Lycanthrope',
  3: 'Undead',
  4: 'Giant',
  5: 'Construct',
  6: 'Extraplanar',
  7: 'Magical',
  8: 'Summoned Undead',
  10: 'Vampire',
  12: 'Dragon',
  13: 'Summoned',
  14: 'Humanoid (alt)',
  16: 'Plant',
  17: 'Animal',
  18: 'Insect',
  19: 'Muramite',
  25: 'Chest',
  26: 'Amphibian',
  28: 'Summoned (alt)',
}

export function baneBodyLabel(bodyType: number): string {
  return BANE_BODY_TYPES[bodyType] ?? `Body Type ${bodyType}`
}

export const BANE_BODY_OPTIONS: { value: number; label: string }[] = [
  { value: 0, label: 'Any' },
  ...Object.entries(BANE_BODY_TYPES)
    .map(([k, v]) => ({ value: Number(k), label: v }))
    .sort((a, b) => a.label.localeCompare(b.label)),
]

// ── Weight (tenths of a pound) ─────────────────────────────────────────────────

export function weightLabel(w: number): string {
  return `${(w / 10).toFixed(1)}`
}

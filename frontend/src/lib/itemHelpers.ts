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
  2: '1H Piercing',
  3: '1H Blunt',
  4: '2H Blunt',
  5: 'Archery',
  7: 'Throwing',
  8: 'Shield',
  10: 'Armor',
  11: 'Miscellaneous',
  12: 'Lock Picks',
  14: 'Food',
  15: 'Drink',
  16: 'Light',
  17: 'Combinable',
  18: 'Bandages',
  19: 'Throwable',
  20: 'Spell Scroll',
  21: 'Potion',
  22: 'Tradeskill',
  23: 'Wind Instrument',
  24: 'Stringed Instrument',
  25: 'Brass Instrument',
  26: 'Percussion Instrument',
  27: 'Arrow Making',
  28: 'Jewelry',
  29: 'Skull',
  30: 'Book',
  31: 'Note',
  32: 'Key',
  33: 'Coin',
  34: '2H Piercing',
  35: 'Fishing Pole',
  36: 'Fishing Bait',
  37: 'Alcohol',
  38: 'Key (Alt)',
  39: 'Compass',
  40: 'Poison',
  52: 'Martial',
  54: 'Container',
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

// ── Bane damage race ──────────────────────────────────────────────────────────

const BANE_RACE_NAMES: Record<number, string> = {
  1: 'Human', 2: 'Barbarian', 3: 'Erudite', 4: 'Wood Elf', 5: 'High Elf',
  6: 'Dark Elf', 7: 'Half-Elf', 8: 'Dwarf', 9: 'Troll', 10: 'Ogre',
  11: 'Halfling', 12: 'Gnome', 13: 'Aviak', 14: 'Were Wolf', 15: 'Brownie',
  16: 'Centaur', 17: 'Golem', 18: 'Giant/Cyclops', 19: 'Trakanon',
  20: 'Doppleganger', 21: 'Evil Eye', 22: 'Beetle', 23: 'Kerra', 24: 'Fish',
  25: 'Fairy', 26: 'Froglok', 27: 'Froglok Ghoul', 28: 'Fungusman',
  29: 'Gargoyle', 31: 'Gelatinous Cube', 32: 'Ghost', 33: 'Ghoul',
  34: 'Giant Bat', 35: 'Giant Eel', 36: 'Giant Rat', 37: 'Giant Snake',
  38: 'Giant Spider', 39: 'Gnoll', 40: 'Goblin', 41: 'Gorilla', 42: 'Wolf',
  43: 'Bear', 45: 'Demi Lich', 46: 'Imp', 47: 'Griffin', 48: 'Kobold',
  49: 'Lava Dragon', 50: 'Lion', 51: 'Lizard Man', 52: 'Mimic',
  53: 'Minotaur', 54: 'Orc', 56: 'Pixie', 57: 'Dracnid', 60: 'Skeleton',
  61: 'Shark', 63: 'Tiger', 64: 'Treant', 65: 'Vampire', 70: 'Zombie',
  75: 'Elemental', 76: 'Puma', 79: 'Bixie', 82: 'Scarecrow', 83: 'Skunk',
  84: 'Snake Elemental', 85: 'Spectre', 86: 'Sphinx', 87: 'Armadillo',
  88: 'Clockwork Gnome', 89: 'Drake', 91: 'Alligator', 96: 'Cockatrice',
  101: 'Efreeti', 103: 'Kedge', 107: 'Mammoth', 109: 'Wasp', 110: 'Mermaid',
  111: 'Harpie', 119: 'Sabertooth Cat', 121: 'Gorgon', 128: 'Iksar',
  129: 'Scorpion', 130: 'Vah Shir', 131: 'Sarnak', 133: 'Lycanthrope',
  135: 'Rhino', 138: 'Yeti', 140: 'Forest Giant', 144: 'Burynai',
  157: 'Wyvern', 158: 'Wurm', 159: 'Devourer', 162: 'Man Eating Plant',
  163: 'Raptor', 169: 'Brontotherium', 171: 'Dire Wolf', 172: 'Manticore',
  174: 'Cold Spectre', 183: 'Coldain', 184: 'Velious Dragon', 185: 'Hag',
  186: 'Hippogriff', 187: 'Siren', 188: 'Frost Giant', 189: 'Storm Giant',
  190: 'Ottermen', 192: 'Clockwork Dragon', 193: 'Abhorent',
  199: 'ShikNar', 200: 'Rockhopper', 201: 'Underbulk', 202: 'Grimling',
  203: 'Vacuum Worm', 206: 'Owlbear', 207: 'Rhino Beetle', 208: 'Vampyre',
  214: 'Thought Horror', 215: 'Tegi', 217: 'Shissar', 218: 'Fungal Fiend',
  220: 'StoneGrabber', 221: 'Scarlet Cheetah', 222: 'Zelniak',
  224: 'Shade', 227: 'Shrieker', 229: 'Netherbian', 230: 'Akheva',
  232: 'Sonic Wolf', 233: 'Ground Shaker', 236: 'Seru',
  257: 'Undead', 330: 'Froglok', 353: 'Veksar',
}

export function baneRaceLabel(raceId: number): string {
  return BANE_RACE_NAMES[raceId] ?? `Race ${raceId}`
}

// ── Weight (tenths of a pound) ─────────────────────────────────────────────────

export function weightLabel(w: number): string {
  return `${(w / 10).toFixed(1)}`
}

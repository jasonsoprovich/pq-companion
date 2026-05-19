import {
  baneBodyLabel as enumBaneBodyLabel,
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

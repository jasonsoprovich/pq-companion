// Helpers for Planes of Power armor molds/patterns (Ornate, Elemental,
// Timeless): each combines with a class emblem + ethereal material into a
// class armor piece that never drops itself. See backend db/molds.go.

import type { MoldProduct } from '../types/item'

// isClassProduct reports whether a mold product is for the given EQ class
// index (0-14, the items.classes bit position).
export function isClassProduct(p: MoldProduct, classIndex: number | null | undefined): boolean {
  return classIndex != null && classIndex >= 0 && (p.classes & (1 << classIndex)) !== 0
}

// moldRecipeSummary describes the combine shared by every product of one
// mold: "<Class> Emblem + 2× Ethereal Metal Rings in Mystical Furnace of Ro
// (Tanaan Smithing Hammer returned)". The emblem differs per class, so it's
// shown generically.
export function moldRecipeSummary(products: MoldProduct[]): string {
  const first = products[0]
  if (!first) return ''
  const parts: string[] = []
  let emblem = false
  for (const inp of first.inputs) {
    if (inp.name.endsWith(' Emblem')) {
      emblem = true
      continue
    }
    parts.push(`${inp.count && inp.count > 1 ? `${inp.count}× ` : ''}${inp.name}`)
  }
  if (emblem) parts.unshift('Class Emblem')
  let out = parts.join(' + ')
  if (first.container.name) out += ` in ${first.container.name}`
  if (first.tools.length > 0) {
    out += ` (${first.tools.map((t) => t.name).join(', ')} returned)`
  }
  return out
}

import { getEnums } from '../services/api'
import type { EnumsCatalog, SpecialAbilityMeta } from '../types/enums'

// Single source of truth for raw-code → display-label mappings, served
// by the Go backend at GET /api/enums. The catalog is static for the
// lifetime of the backend process, so we fetch it once on app boot and
// cache it module-level. Synchronous label helpers fall back to a
// minimal stub (e.g. "Tradeskill 75") if they're called before the
// catalog has loaded — in practice loadEnums() resolves before any
// label-rendering component mounts.
//
// See backend/internal/db/enums/ for the canonical Go source.

let catalog: EnumsCatalog | null = null
let loading: Promise<EnumsCatalog> | null = null

export function loadEnums(): Promise<EnumsCatalog> {
  if (catalog) return Promise.resolve(catalog)
  if (!loading) {
    loading = getEnums().then((c) => {
      catalog = c
      return c
    })
  }
  return loading
}

export function tradeskillLabel(id: number): string {
  return catalog?.tradeskills[String(id)] ?? `Tradeskill ${id}`
}

export function specialAbilityMeta(code: number): SpecialAbilityMeta {
  return catalog?.special_abilities[String(code)] ?? { name: `Ability ${code}`, description: '' }
}

export function specialAbilityName(code: number): string {
  return specialAbilityMeta(code).name
}

export function itemTypeLabel(id: number): string {
  return catalog?.item_types[String(id)] ?? `Type ${id}`
}

export function npcClassName(id: number): string {
  return catalog?.npc_classes[String(id)] ?? `Class ${id}`
}

export function npcRaceName(id: number): string {
  return catalog?.npc_races[String(id)] ?? `Race ${id}`
}

// Decompose a bitmask using one of the *_bits maps in the catalog. The
// catalog stores integer bit values as the (stringified) map key — so a
// slot/class/race bit map looks like { "1": "Charm", "2": "Ear", ... }.
// Returns the labels for every bit set in the mask, in ascending bit
// order. Duplicate labels (e.g. left/right Wrist) collapse to a single
// entry.
function decomposeBits(map: Record<string, string> | undefined, mask: number): string[] {
  if (!map) return []
  const labels: string[] = []
  const seen = new Set<string>()
  for (let i = 0; i < 24; i++) {
    const bit = 1 << i
    if ((mask & bit) === 0) continue
    const label = map[String(bit)]
    if (!label || seen.has(label)) continue
    seen.add(label)
    labels.push(label)
  }
  return labels
}

export function decodeItemSlots(mask: number): string[] {
  return decomposeBits(catalog?.item_slot_bits, mask)
}

export function decodeItemClasses(mask: number): string[] {
  // All-15-classes mask renders as "All". The exact "all" value depends
  // on whether Beastlord is the highest set bit (0x7FFF = 32767); we
  // also accept anything ≥ that to match legacy frontend behavior.
  const ALL = (1 << 15) - 1
  if (mask === 0 || mask >= ALL) return ['All']
  return decomposeBits(catalog?.item_class_bits, mask)
}

export function decodeItemRaces(mask: number): string[] {
  // The "all races" sentinel has appeared as both 16383 and 65535 in
  // Quarm data. Mirror the legacy behavior.
  const ALL = 65535
  if (mask === 0 || mask >= ALL) return ['All']
  return decomposeBits(catalog?.item_race_bits, mask)
}

export function baneBodyLabel(id: number): string {
  return catalog?.bane_bodies[String(id)] ?? `Body Type ${id}`
}

export function baneRaceLabel(id: number): string {
  return catalog?.bane_races[String(id)] ?? `Race ${id}`
}

export function zoneExpansionName(id: number): string {
  return catalog?.zone_expansions[String(id)] ?? `Expansion ${id}`
}

export function zoneTypeLabel(id: number): string {
  return catalog?.zone_types[String(id)] ?? ''
}

// charClassLabel returns the "ABBR — Full Name" label for the 0-based
// PC class index used by character-creation dropdowns. id < 0 is the
// "Not set" sentinel used by CharactersPage and OnboardingWizard.
export function charClassLabel(id: number): string {
  if (id < 0) return 'Not set'
  return catalog?.char_classes[String(id)] ?? `Class ${id}`
}

// charClassOptions returns dropdown options including the "Not set"
// sentinel, in canonical class order.
export function charClassOptions(includeNotSet: boolean = true): { value: number; label: string }[] {
  return optionsFromMap(catalog?.char_classes, includeNotSet)
}

// charRaceLabel returns the display name for a 1-based PC race index.
// id < 0 is the "Not set" sentinel used by character creation UIs.
export function charRaceLabel(id: number): string {
  if (id < 0) return 'Not set'
  return catalog?.char_races[String(id)] ?? `Race ${id}`
}

export function charRaceOptions(includeNotSet: boolean = true): { value: number; label: string }[] {
  return optionsFromMap(catalog?.char_races, includeNotSet)
}

function optionsFromMap(
  map: Record<string, string> | undefined,
  includeNotSet: boolean,
): { value: number; label: string }[] {
  const m = map ?? {}
  const ids = Object.keys(m)
    .map(Number)
    .sort((a, b) => a - b)
  const opts = ids.map((id) => ({ value: id, label: m[String(id)] ?? `${id}` }))
  if (includeNotSet) opts.unshift({ value: -1, label: 'Not set' })
  return opts
}

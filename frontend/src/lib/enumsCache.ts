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

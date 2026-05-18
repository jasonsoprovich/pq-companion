import { useEffect, useState } from 'react'
import type { EntityStats } from '../types/combat'

const STORAGE_KEY = 'pq-companion.combine_pet_with_owner'

export function getCombinePetWithOwner(): boolean {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw === null) return true
    return raw === 'true'
  } catch {
    return true
  }
}

export function setCombinePetWithOwner(value: boolean): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, value ? 'true' : 'false')
    window.dispatchEvent(new CustomEvent('pq:combine-pet-with-owner', { detail: value }))
  } catch {
    // ignore: best-effort persistence
  }
}

export function useCombinePetWithOwner(): [boolean, (v: boolean) => void] {
  const [value, setValue] = useState<boolean>(() => getCombinePetWithOwner())
  useEffect(() => {
    function onChange(e: Event): void {
      const detail = (e as CustomEvent<boolean>).detail
      if (typeof detail === 'boolean') setValue(detail)
    }
    window.addEventListener('pq:combine-pet-with-owner', onChange)
    return () => window.removeEventListener('pq:combine-pet-with-owner', onChange)
  }, [])
  return [
    value,
    (v: boolean) => {
      setValue(v)
      setCombinePetWithOwner(v)
    },
  ]
}

export interface RolledUpEntity extends EntityStats {
  // Pets folded into this row. Empty when this row is not an owner.
  pets: EntityStats[]
}

// rollupCombatants groups pet rows under their owners when combine=true.
// When combine=false, returns the original list with empty pets[] on each row.
//
// Combined row math: total_damage / hit_count / dps are summed; max_hit takes
// the per-entity max across owner+pets. When the owner row is absent (e.g. a
// pet did all the damage), an owner row is synthesised with the combined
// totals so the player still gets credit.
export function rollupCombatants(
  combatants: EntityStats[],
  combine: boolean,
  fightDuration: number,
): RolledUpEntity[] {
  if (!combine) {
    return combatants.map((c) => ({ ...c, pets: [] }))
  }

  const ownerToPets = new Map<string, EntityStats[]>()
  const standalone: EntityStats[] = []

  for (const c of combatants) {
    if (c.owner_name && c.owner_name !== c.name) {
      const pets = ownerToPets.get(c.owner_name) ?? []
      pets.push(c)
      ownerToPets.set(c.owner_name, pets)
    } else {
      standalone.push(c)
    }
  }

  const handledOwners = new Set<string>()
  const rolled: RolledUpEntity[] = []

  for (const owner of standalone) {
    const pets = ownerToPets.get(owner.name) ?? []
    if (pets.length === 0) {
      rolled.push({ ...owner, pets: [] })
      continue
    }
    handledOwners.add(owner.name)
    const totalDamage = owner.total_damage + sumDamage(pets)
    const hitCount = owner.hit_count + sumHits(pets)
    const maxHit = Math.max(owner.max_hit, ...pets.map((p) => p.max_hit))
    // Owner and pet are typically engaged in the same fight window, so the
    // union of their active intervals is closer to max(individual active
    // times) than to a sum. Approximating the union here keeps the combined
    // active DPS comparable to a non-pet class's active DPS instead of
    // double-crediting overlapping engagement.
    const activeSecs = Math.max(owner.active_seconds, ...pets.map((p) => p.active_seconds))
    // raid_seconds is constant across the fight; the owner row carries the
    // canonical value (every pet shares it). Inherit via the spread.
    const raidSecs = owner.raid_seconds
    rolled.push({
      ...owner,
      total_damage: totalDamage,
      hit_count: hitCount,
      max_hit: maxHit,
      dps: fightDuration > 0 ? totalDamage / fightDuration : 0,
      active_dps: activeSecs > 0 ? totalDamage / activeSecs : 0,
      active_seconds: activeSecs,
      raid_dps: raidSecs > 0 ? totalDamage / raidSecs : 0,
      raid_seconds: raidSecs,
      crit_count: owner.crit_count + sumCrits(pets),
      crit_damage: owner.crit_damage + sumCritDamage(pets),
      pets,
    })
  }

  // Synthesize owner rows for pets whose owner never appeared in the table
  // (e.g. magician summons doing all the damage while the player is OOM).
  for (const [ownerName, pets] of ownerToPets) {
    if (handledOwners.has(ownerName)) continue
    const totalDamage = sumDamage(pets)
    const hitCount = sumHits(pets)
    const maxHit = Math.max(...pets.map((p) => p.max_hit))
    const activeSecs = Math.max(...pets.map((p) => p.active_seconds))
    // Pull raid_seconds from any pet — it's constant across the fight.
    const raidSecs = pets[0]?.raid_seconds ?? 0
    rolled.push({
      name: ownerName,
      total_damage: totalDamage,
      hit_count: hitCount,
      max_hit: maxHit,
      dps: fightDuration > 0 ? totalDamage / fightDuration : 0,
      active_dps: activeSecs > 0 ? totalDamage / activeSecs : 0,
      active_seconds: activeSecs,
      raid_dps: raidSecs > 0 ? totalDamage / raidSecs : 0,
      raid_seconds: raidSecs,
      crit_count: sumCrits(pets),
      crit_damage: sumCritDamage(pets),
      // Inherit class from any pet (the backend stamps each pet with its
      // owner's class) so the synthesised owner row still colours correctly
      // when the owner never appeared as a damage dealer themselves.
      class: pets[0]?.class,
      pets,
    })
  }

  rolled.sort((a, b) => b.total_damage - a.total_damage)
  return rolled
}

function sumDamage(pets: EntityStats[]): number {
  let s = 0
  for (const p of pets) s += p.total_damage
  return s
}

function sumHits(pets: EntityStats[]): number {
  let s = 0
  for (const p of pets) s += p.hit_count
  return s
}

function sumCrits(pets: EntityStats[]): number {
  let s = 0
  for (const p of pets) s += p.crit_count
  return s
}

function sumCritDamage(pets: EntityStats[]): number {
  let s = 0
  for (const p of pets) s += p.crit_damage
  return s
}

// petBadge returns "(+pet)" for one pet, "(+N pets)" for many; "" otherwise.
export function petBadge(pets: EntityStats[]): string {
  if (pets.length === 0) return ''
  if (pets.length === 1) return ' (+pet)'
  return ` (+${pets.length} pets)`
}

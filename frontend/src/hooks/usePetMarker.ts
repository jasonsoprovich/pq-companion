import { useMemo } from 'react'
import type { PlayerPosition } from './usePlayerPosition'
import { usePetState } from './usePetState'

// PetMarker is your pet as the maps draw it.
export interface PetMarker {
  x: number
  y: number
  z: number
  heading: number
  label: string
}

// PET_COLOR is the pet's marker colour on every map: amber, apart from you
// (near-white), your group (sky) and raid colours.
export const PET_COLOR = '#fbbf24'

// usePetMarker is your pet's map marker from the live position, named from
// the Pet overlay's state when it's the same pet, or null with no pet
// position.
export function usePetMarker(pos: PlayerPosition | null): PetMarker | null {
  const pet = usePetState()
  return useMemo(() => {
    const p = pos?.pet
    if (!p) return null
    const named = pet.name && (pet.pet_id == null || pet.pet_id === p.id)
    return { x: p.x, y: p.y, z: p.z, heading: p.heading, label: named ? (pet.name as string) : 'Pet' }
  }, [pos?.pet, pet.name, pet.pet_id]) // eslint-disable-line react-hooks/exhaustive-deps
}

// petDistance is the flat distance from you to your pet, in game units, or
// null with no pet position.
export function petDistance(pos: PlayerPosition | null): number | null {
  if (!pos?.pet) return null
  return Math.hypot(pos.pet.x - pos.x, pos.pet.y - pos.y)
}

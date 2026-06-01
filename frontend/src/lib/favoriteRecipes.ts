import { useEffect, useSyncExternalStore } from 'react'
import {
  listFavoriteRecipes,
  addFavoriteRecipe,
  removeFavoriteRecipe,
} from '../services/api'

// Module-level cache of the global favorite-recipe id set, shared by every
// FavoriteRecipeStar across the app so toggling a star in one place updates it
// everywhere. Favorites are account-wide (not per-character), so a single set
// suffices for the process lifetime. Mutations are optimistic with rollback.

let favorites = new Set<number>()
let loaded = false
let loading: Promise<void> | null = null
let version = 0
const listeners = new Set<() => void>()

function emit() {
  version++
  for (const l of listeners) l()
}

export function ensureFavoritesLoaded(): Promise<void> {
  if (loaded) return Promise.resolve()
  if (!loading) {
    loading = listFavoriteRecipes()
      .then((rows) => {
        favorites = new Set(rows.map((r) => r.id))
        loaded = true
        emit()
      })
      .catch(() => {
        loaded = true
      })
  }
  return loading
}

export function isFavoriteRecipe(id: number): boolean {
  return favorites.has(id)
}

export async function toggleFavoriteRecipe(id: number): Promise<void> {
  if (favorites.has(id)) {
    favorites.delete(id)
    emit()
    try {
      await removeFavoriteRecipe(id)
    } catch {
      favorites.add(id)
      emit()
    }
  } else {
    favorites.add(id)
    emit()
    try {
      await addFavoriteRecipe(id)
    } catch {
      favorites.delete(id)
      emit()
    }
  }
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

// useFavoriteRecipes wires a component to the shared favorites store. It kicks
// off the initial load once and re-renders the caller whenever the set changes.
export function useFavoriteRecipes(): {
  isFavorite: (id: number) => boolean
  toggle: (id: number) => Promise<void>
} {
  useEffect(() => {
    ensureFavoritesLoaded()
  }, [])
  useSyncExternalStore(subscribe, () => version)
  return { isFavorite: isFavoriteRecipe, toggle: toggleFavoriteRecipe }
}

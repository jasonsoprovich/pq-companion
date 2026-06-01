import React from 'react'
import { Star } from 'lucide-react'
import { useFavoriteRecipes } from '../lib/favoriteRecipes'

interface FavoriteRecipeStarProps {
  recipeId: number
  /** Pixel size of the star icon. Defaults to 18. */
  size?: number
}

/**
 * Toggle for starring a tradeskill recipe into the global favorites list.
 * Outline = not favorited; filled = favorited. Unlike the item wishlist this
 * is account-wide, so it needs no active character.
 */
export default function FavoriteRecipeStar({
  recipeId,
  size = 18,
}: FavoriteRecipeStarProps): React.ReactElement {
  const { isFavorite, toggle } = useFavoriteRecipes()
  const fav = isFavorite(recipeId)
  return (
    <button
      onClick={(e) => {
        e.stopPropagation()
        toggle(recipeId)
      }}
      title={fav ? 'Remove from favorite recipes' : 'Add to favorite recipes'}
      className="rounded p-0.5"
    >
      <Star
        size={size}
        style={{ color: fav ? 'var(--color-primary)' : 'var(--color-muted)' }}
        fill={fav ? 'currentColor' : 'none'}
      />
    </button>
  )
}

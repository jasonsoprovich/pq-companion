import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search, Star, X } from 'lucide-react'
import {
  searchRecipes,
  getRecipe,
  getRecipeTradeskills,
  getRecipeRaw,
  listFavoriteRecipes,
} from '../services/api'
import type { RecipeSearchFilter } from '../services/api'
import type { RecipeSummary, RecipeDetail, RecipeTradeskillCount } from '../types/recipe'
import { tradeskillLabel } from '../lib/enumsCache'
import { ItemIcon } from '../components/Icon'
import RawDataModal from '../components/RawDataModal'
import FavoriteRecipeStar from '../components/FavoriteRecipeStar'
import { RecipeBody } from '../components/RecipeView'
import { useFavoriteRecipes } from '../lib/favoriteRecipes'

const RECIPE_PAGE_SIZE = 50

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (recipe: RecipeSummary) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [tradeskill, setTradeskill] = useState(-1)
  const [trivialMin, setTrivialMin] = useState('')
  const [trivialMax, setTrivialMax] = useState('')
  const [favoritesOnly, setFavoritesOnly] = useState(false)
  const [skills, setSkills] = useState<RecipeTradeskillCount[]>([])
  const [items, setItems] = useState<RecipeSummary[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Re-render this pane when favorites change so the favorites-only list stays
  // current after a star toggle elsewhere.
  useFavoriteRecipes()

  useEffect(() => {
    getRecipeTradeskills()
      .then((s) => setSkills(s.filter((x) => x.count > 0).sort((a, b) => b.count - a.count)))
      .catch(() => setSkills([]))
  }, [])

  const apiFilter = useCallback(
    (): RecipeSearchFilter => ({
      tradeskill: tradeskill >= 0 ? tradeskill : undefined,
      trivialMin: parseInt(trivialMin) > 0 ? parseInt(trivialMin) : undefined,
      trivialMax: parseInt(trivialMax) > 0 ? parseInt(trivialMax) : undefined,
    }),
    [tradeskill, trivialMin, trivialMax],
  )

  // Favorites mode applies the same filters client-side over the starred set.
  const matchesFilters = useCallback(
    (r: RecipeSummary): boolean => {
      if (query && !r.name.toLowerCase().includes(query.toLowerCase())) return false
      if (tradeskill >= 0 && r.tradeskill !== tradeskill) return false
      if (parseInt(trivialMin) > 0 && r.trivial < parseInt(trivialMin)) return false
      if (parseInt(trivialMax) > 0 && r.trivial > parseInt(trivialMax)) return false
      return true
    },
    [query, tradeskill, trivialMin, trivialMax],
  )

  const runSearch = useCallback(() => {
    setLoading(true)
    setError(null)
    if (favoritesOnly) {
      listFavoriteRecipes()
        .then((rows) => {
          const filtered = rows.filter(matchesFilters)
          setItems(filtered)
          setTotal(filtered.length)
        })
        .catch((err: Error) => setError(err.message))
        .finally(() => setLoading(false))
      return
    }
    searchRecipes(query, RECIPE_PAGE_SIZE, 0, apiFilter())
      .then((res) => {
        setItems(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [favoritesOnly, query, apiFilter, matchesFilters])

  const loadMore = useCallback(() => {
    setLoadingMore(true)
    searchRecipes(query, RECIPE_PAGE_SIZE, items.length, apiFilter())
      .then((res) => {
        setItems((prev) => [...prev, ...(res.items ?? [])])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoadingMore(false))
  }, [query, apiFilter, items.length])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(runSearch, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [runSearch])

  const hasMore = !favoritesOnly && !loading && items.length < total

  const selectStyle = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }
  const inputStyle = selectStyle

  return (
    <div className="flex w-72 shrink-0 flex-col border-r" style={{ borderColor: 'var(--color-border)' }}>
      {/* Search row */}
      <div className="flex items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search recipes…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          spellCheck={false}
        />
        {query && (
          <button onClick={() => setQuery('')} className="shrink-0">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        )}
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <select
          value={tradeskill}
          onChange={(e) => setTradeskill(Number(e.target.value))}
          className="rounded px-2 py-1 text-xs outline-none"
          style={selectStyle}
        >
          <option value={-1}>Any tradeskill</option>
          {skills.map((s) => (
            <option key={s.tradeskill} value={s.tradeskill}>
              {tradeskillLabel(s.tradeskill)} ({s.count})
            </option>
          ))}
        </select>
        <div className="flex items-center gap-2">
          <input
            type="number"
            min={0}
            max={300}
            placeholder="Min trivial"
            value={trivialMin}
            onChange={(e) => setTrivialMin(e.target.value)}
            className="w-1/2 rounded px-2 py-1 text-xs outline-none"
            style={inputStyle}
          />
          <input
            type="number"
            min={0}
            max={300}
            placeholder="Max trivial"
            value={trivialMax}
            onChange={(e) => setTrivialMax(e.target.value)}
            className="w-1/2 rounded px-2 py-1 text-xs outline-none"
            style={inputStyle}
          />
        </div>
        <button
          onClick={() => setFavoritesOnly((v) => !v)}
          className="flex items-center gap-1.5 self-start rounded px-2 py-1 text-xs transition-colors"
          style={{
            color: favoritesOnly ? 'var(--color-primary)' : 'var(--color-muted)',
            backgroundColor: favoritesOnly
              ? 'color-mix(in srgb, var(--color-primary) 15%, transparent)'
              : 'transparent',
          }}
        >
          <Star size={13} fill={favoritesOnly ? 'currentColor' : 'none'} />
          Favorites only
        </button>
      </div>

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading
          ? 'Searching…'
          : error
            ? 'Error'
            : items.length < total
              ? `${items.length.toLocaleString()} of ${total.toLocaleString()} recipes`
              : `${total.toLocaleString()} recipes`}
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>{error}</p>
        )}
        {!error && !loading && items.length === 0 && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-muted)' }}>
            {favoritesOnly ? 'No favorite recipes match.' : 'No recipes found.'}
          </p>
        )}
        {!error &&
          items.map((r) => (
            <button
              key={r.id}
              onClick={() => onSelect(r)}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor: selectedId === r.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft: selectedId === r.id ? '2px solid var(--color-primary)' : '2px solid transparent',
              }}
            >
              <ItemIcon id={r.product_icon} name={r.name} size={28} />
              <div className="min-w-0 flex-1">
                <div
                  className="truncate text-sm font-medium"
                  style={{ color: selectedId === r.id ? 'var(--color-primary)' : 'var(--color-foreground)' }}
                >
                  {r.name}
                </div>
                <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {tradeskillLabel(r.tradeskill)} · Trivial {r.trivial}
                </div>
              </div>
            </button>
          ))}
        {hasMore && (
          <div className="px-3 py-2">
            <button
              onClick={loadMore}
              disabled={loadingMore}
              className="w-full rounded border py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: 'var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
            >
              {loadingMore ? 'Loading…' : `Show more (${(total - items.length).toLocaleString()} remaining)`}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

function DetailPanel({ recipe }: { recipe: RecipeSummary | null }): React.ReactElement {
  const [detail, setDetail] = useState<RecipeDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [rawOpen, setRawOpen] = useState(false)
  const rawFetcher = useCallback(() => getRecipeRaw(recipe!.id), [recipe?.id])

  useEffect(() => {
    if (!recipe) {
      setDetail(null)
      return
    }
    setLoading(true)
    setError(false)
    getRecipe(recipe.id)
      .then(setDetail)
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }, [recipe?.id])

  if (!recipe) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select a recipe to view its ingredients
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="shrink-0 border-b px-5 pt-4 pb-3" style={{ borderColor: 'var(--color-border)' }}>
        <div className="flex items-center gap-3">
          <ItemIcon id={recipe.product_icon} name={recipe.name} size={40} />
          <h2 className="text-xl font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
            {recipe.name}
          </h2>
          <FavoriteRecipeStar recipeId={recipe.id} size={20} />
          <button
            onClick={() => setRawOpen(true)}
            className="ml-auto rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-1)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-muted)',
            }}
            title="View raw database row"
          >
            Raw Data
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-5 py-4">
        {loading && <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Loading…</p>}
        {error && <p className="text-sm" style={{ color: 'var(--color-destructive)' }}>Couldn’t load recipe.</p>}
        {detail && <RecipeBody recipe={detail} />}
      </div>

      <RawDataModal open={rawOpen} title={recipe.name} fetcher={rawFetcher} onClose={() => setRawOpen(false)} />
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function RecipesPage(): React.ReactElement {
  const [selected, setSelected] = useState<RecipeSummary | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getRecipe(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel recipe={selected} />
    </div>
  )
}

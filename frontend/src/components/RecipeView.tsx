import React, { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, ChevronRight, Hammer, Check } from 'lucide-react'
import { getRecipe, getItemSources } from '../services/api'
import { tradeskillLabel } from '../lib/enumsCache'
import { priceLabel } from '../lib/itemHelpers'
import { describeLocation } from '../lib/inventoryLocations'
import { findHoldingsForItems, type ItemHolding } from '../services/itemHoldings'
import { ItemIcon } from './Icon'
import FavoriteRecipeStar from './FavoriteRecipeStar'
import type { RecipeDetail, RecipeEntry } from '../types/recipe'
import type { ItemTradeskillEntry } from '../types/item'

// How deep ingredient drill-down may recurse before it stops offering the
// expand affordance. Bounds runaway expansion on recipes whose components are
// themselves crafted (which can chain several levels on Quarm).
const MAX_DRILL_DEPTH = 4

// A recipe entry's item links to its item-detail page only when it's a real
// inventory item. Combine-station rows (Forge, Lexicon, …) and unresolved rows
// are rendered as plain text — navigating to them would 404.
function isLinkableEntry(e: RecipeEntry): boolean {
  return !e.station && e.item_id > 0 && !e.item_name.startsWith('Item #')
}

// Have/missing indicator for a component row, sourced from the player's Zeal
// inventory exports. Held in a single spot shows the character, quantity and
// location inline. Held in several spots shows the total owned and lets the
// user click through to a per-character/location breakdown, since a hover
// tooltip alone doesn't tell a heavy user which of several alts to go grab it
// from.
function HaveBadge({ entry, holdings }: { entry: RecipeEntry; holdings: ItemHolding[] }): React.ReactElement {
  const containerRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    window.addEventListener('mousedown', onClick)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onClick)
      window.removeEventListener('keydown', onKey)
    }
  }, [open])

  const needed = entry.count > 0 ? entry.count : 1
  const totalHeld = holdings.reduce((sum, h) => sum + h.count, 0)
  const have = totalHeld >= needed
  const single = holdings.length === 1 ? holdings[0] : null

  const title = have
    ? `You have ${totalHeld}${totalHeld !== needed ? ` (need ${needed})` : ''}`
    : totalHeld > 0
      ? `Only have ${totalHeld} of ${needed} needed`
      : 'Not found in any character inventory'

  return (
    <div ref={containerRef} className="relative flex shrink-0 items-center gap-1.5">
      {single && (
        <span className="whitespace-nowrap" style={{ color: 'var(--color-muted-foreground)' }} title={title}>
          {single.character || 'Shared Bank'} — {describeLocation(single.location)}
          {single.count > 1 && <span className="ml-1">×{single.count}</span>}
        </span>
      )}
      {holdings.length > 1 && (
        <button
          onClick={() => setOpen((o) => !o)}
          className="flex items-center gap-0.5 whitespace-nowrap underline decoration-dotted"
          style={{ color: 'var(--color-muted-foreground)' }}
          title={title}
        >
          ×{totalHeld} across {holdings.length} spots
          {open ? <ChevronDown size={10} /> : <ChevronRight size={10} />}
        </button>
      )}
      <span className="flex items-center gap-0.5" style={{ color: have ? '#4ade80' : '#f87171' }} title={title}>
        {have ? <Check size={12} /> : <span className="text-[10px] font-semibold uppercase">missing</span>}
      </span>
      {open && holdings.length > 1 && (
        <div
          className="absolute right-0 top-full z-40 mt-1 min-w-[180px] rounded border py-1 shadow-lg"
          style={{ backgroundColor: 'var(--color-surface-2)', borderColor: 'var(--color-border)' }}
        >
          {holdings.map((h, i) => (
            <div key={i} className="flex items-center justify-between gap-3 px-2 py-0.5 text-xs">
              <span className="min-w-0 truncate" style={{ color: 'var(--color-foreground)' }}>
                {h.character || 'Shared Bank'}
                {h.count > 1 && (
                  <span className="ml-1" style={{ color: 'var(--color-muted-foreground)' }}>
                    ×{h.count}
                  </span>
                )}
              </span>
              <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                {describeLocation(h.location)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

interface EntryRowProps {
  entry: RecipeEntry
  depth: number
  onNavigate?: () => void
  // Where the player holds this item, and whether that lookup has resolved —
  // undefined holdingsMap means "not shown" (loading, unsupported role, or
  // Zeal inventory export not configured).
  holdingsMap?: Map<number, ItemHolding[]>
}

// One container/component/product line. Components that are themselves craftable
// can be expanded in place to reveal the sub-recipe that produces them.
function EntryRow({ entry, depth, onNavigate, holdingsMap }: EntryRowProps): React.ReactElement {
  const navigate = useNavigate()
  const [expanded, setExpanded] = useState(false)
  const [sub, setSub] = useState<RecipeDetail | null>(null)
  const [choices, setChoices] = useState<ItemTradeskillEntry[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  const canExpand = entry.role === 'component' && !!entry.craftable && depth < MAX_DRILL_DEPTH
  const linkable = isLinkableEntry(entry)

  function loadRecipe(recipeId: number) {
    setLoading(true)
    setError(false)
    getRecipe(recipeId)
      .then((r) => {
        setSub(r)
        setChoices(null)
      })
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }

  function toggle() {
    if (expanded) {
      setExpanded(false)
      return
    }
    setExpanded(true)
    if (sub || choices) return
    // Find which recipe(s) produce this item. One producer expands directly;
    // several let the user pick which to drill into.
    setLoading(true)
    setError(false)
    getItemSources(entry.item_id)
      .then((s) => {
        const producers = s.tradeskills.filter((t) => t.role === 'product')
        if (producers.length === 1) {
          loadRecipe(producers[0].recipe_id)
        } else {
          setChoices(producers)
          setLoading(false)
        }
      })
      .catch(() => {
        setError(true)
        setLoading(false)
      })
  }

  function goToItem() {
    if (!linkable) return
    navigate(`/items?select=${entry.item_id}`)
    onNavigate?.()
  }

  return (
    <div>
      <div className="flex items-center gap-2 py-0.5 text-sm">
        {canExpand ? (
          <button onClick={toggle} className="shrink-0" title="Show sub-recipe">
            {expanded ? (
              <ChevronDown size={13} style={{ color: 'var(--color-muted)' }} />
            ) : (
              <ChevronRight size={13} style={{ color: 'var(--color-muted)' }} />
            )}
          </button>
        ) : (
          <span className="w-[13px] shrink-0" />
        )}
        {entry.station ? (
          <span
            className="flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
            title="Combine station"
          >
            <Hammer size={13} />
          </span>
        ) : (
          <ItemIcon id={entry.icon} name={entry.item_name} size={22} />
        )}
        {linkable ? (
          <button
            onClick={goToItem}
            className="min-w-0 flex-1 truncate text-left underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
            title="View item details"
          >
            {entry.item_name}
          </button>
        ) : (
          <span
            className="min-w-0 flex-1 truncate"
            style={{ color: 'var(--color-muted-foreground)' }}
            title={entry.station ? 'Combine in any container of this type' : undefined}
          >
            {entry.item_name}
          </span>
        )}
        <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
          {entry.role === 'component' && linkable && holdingsMap && (
            <HaveBadge entry={entry} holdings={holdingsMap.get(entry.item_id) ?? []} />
          )}
          {entry.count > 1 && <span>×{entry.count}</span>}
          {entry.vendor_price != null && entry.vendor_price > 0 && (
            <span title="Base vendor price">{priceLabel(entry.vendor_price)}</span>
          )}
        </div>
      </div>

      {expanded && (
        <div
          className="ml-[13px] border-l pl-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {loading && (
            <p className="py-1 text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>
          )}
          {error && (
            <p className="py-1 text-xs" style={{ color: 'var(--color-destructive)' }}>
              Couldn’t load sub-recipe.
            </p>
          )}
          {!loading && !error && choices && (
            <div className="py-1">
              <div className="mb-1 text-[10px] uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
                Made by {choices.length} recipes — pick one
              </div>
              {choices.map((c) => (
                <button
                  key={c.recipe_id}
                  onClick={() => loadRecipe(c.recipe_id)}
                  className="block w-full truncate py-0.5 text-left text-sm underline decoration-dotted"
                  style={{ color: 'var(--color-primary)' }}
                >
                  {c.recipe_name}
                  <span className="ml-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                    {tradeskillLabel(c.tradeskill)} · Trivial {c.trivial}
                  </span>
                </button>
              ))}
            </div>
          )}
          {!loading && !error && sub && (
            <RecipeBody recipe={sub} depth={depth + 1} onNavigate={onNavigate} />
          )}
        </div>
      )}
    </div>
  )
}

interface EntrySectionProps {
  title: string
  entries: RecipeEntry[]
  depth: number
  onNavigate?: () => void
  holdingsMap?: Map<number, ItemHolding[]>
}

function EntrySection({ title, entries, depth, onNavigate, holdingsMap }: EntrySectionProps): React.ReactElement | null {
  if (entries.length === 0) return null
  return (
    <div>
      <div className="mb-0.5 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        {title}
        {title === 'Containers' && entries.length > 1 && (
          <span className="ml-1 normal-case tracking-normal" style={{ color: 'var(--color-muted)' }}>
            (any one)
          </span>
        )}
      </div>
      {entries.map((e, i) => (
        <EntryRow key={`${e.item_id}-${i}`} entry={e} depth={depth} onNavigate={onNavigate} holdingsMap={holdingsMap} />
      ))}
    </div>
  )
}

interface RecipeBodyProps {
  recipe: RecipeDetail
  depth?: number
  onNavigate?: () => void
}

/**
 * Renders a full recipe's meta line and its container/component/product
 * sections. Components that are themselves craftable can be expanded inline to
 * drill into their sub-recipes. Shared by the recipe browser detail panel and
 * the item-detail Tradeskills tab.
 */
export function RecipeBody({ recipe, depth = 0, onNavigate }: RecipeBodyProps): React.ReactElement {
  const [holdingsInfo, setHoldingsInfo] = useState<{ configured: boolean; map: Map<number, ItemHolding[]> } | null>(null)

  useEffect(() => {
    setHoldingsInfo(null)
    const ids = recipe.components.filter(isLinkableEntry).map((e) => e.item_id)
    if (ids.length === 0) {
      setHoldingsInfo({ configured: true, map: new Map() })
      return
    }
    let cancelled = false
    findHoldingsForItems(ids)
      .then((info) => { if (!cancelled) setHoldingsInfo(info) })
      .catch(() => { if (!cancelled) setHoldingsInfo(null) })
    return () => { cancelled = true }
  }, [recipe.id])

  const missingComponents = holdingsInfo?.configured
    ? recipe.components.filter(isLinkableEntry).filter((e) => {
        const held = holdingsInfo.map.get(e.item_id) ?? []
        const totalHeld = held.reduce((sum, h) => sum + h.count, 0)
        return totalHeld < (e.count > 0 ? e.count : 1)
      })
    : []

  return (
    <div className="flex flex-col gap-2 py-1">
      <div className="flex flex-wrap items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        <span style={{ color: 'var(--color-muted-foreground)' }}>{tradeskillLabel(recipe.tradeskill)}</span>
        <span
          className="cursor-help decoration-dotted underline-offset-2 hover:underline"
          title="Trivial: the skill level at which this recipe stops raising your skill. It does NOT mean 100% success — you can still fail combines above trivial."
        >
          · Trivial {recipe.trivial}
        </span>
        {recipe.skill_needed > 0 && <span>· Min skill {recipe.skill_needed}</span>}
        {recipe.no_fail && (
          <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)' }}>
            No Fail
          </span>
        )}
        {recipe.quest && (
          <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)' }}>
            Quest
          </span>
        )}
      </div>
      <EntrySection title="Containers" entries={recipe.containers} depth={depth} onNavigate={onNavigate} />
      <EntrySection
        title="Components"
        entries={recipe.components}
        depth={depth}
        onNavigate={onNavigate}
        holdingsMap={holdingsInfo?.configured ? holdingsInfo.map : undefined}
      />
      {holdingsInfo && !holdingsInfo.configured && recipe.components.length > 0 && (
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Set up Zeal inventory export to see which components you already have.
        </p>
      )}
      {missingComponents.length > 0 && (
        <div
          className="rounded border px-3 py-2"
          style={{ backgroundColor: 'rgba(248,113,113,0.06)', borderColor: 'rgba(248,113,113,0.3)' }}
        >
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: '#f87171' }}>
            Missing ({missingComponents.length})
          </div>
          <div className="flex flex-wrap gap-1">
            {missingComponents.map((e) => (
              <span
                key={e.item_id}
                className="rounded px-1.5 py-0.5 text-[11px]"
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
              >
                {e.item_name}
                {e.count > 1 && ` ×${e.count}`}
              </span>
            ))}
          </div>
        </div>
      )}
      <EntrySection title="Yields" entries={recipe.products} depth={depth} onNavigate={onNavigate} />
    </div>
  )
}

// ── Item-detail Tradeskills tab ──────────────────────────────────────────────

interface RecipeRowProps {
  entry: ItemTradeskillEntry
  onNavigate?: () => void
}

// An expandable recipe line in the item Tradeskills tab: shows the recipe name,
// discipline, trivial, a favorite star and a link into the recipe browser. On
// expand it lazily fetches the full recipe and renders its ingredients.
function RecipeRow({ entry, onNavigate }: RecipeRowProps): React.ReactElement {
  const navigate = useNavigate()
  const [expanded, setExpanded] = useState(false)
  const [recipe, setRecipe] = useState<RecipeDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  function toggle() {
    if (expanded) {
      setExpanded(false)
      return
    }
    setExpanded(true)
    if (recipe) return
    setLoading(true)
    setError(false)
    getRecipe(entry.recipe_id)
      .then(setRecipe)
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }

  return (
    <div className="border-b last:border-b-0" style={{ borderColor: 'var(--color-border)' }}>
      <div className="flex items-center gap-2 py-1 text-sm">
        <button onClick={toggle} className="shrink-0" title="Show recipe">
          {expanded ? (
            <ChevronDown size={14} style={{ color: 'var(--color-muted)' }} />
          ) : (
            <ChevronRight size={14} style={{ color: 'var(--color-muted)' }} />
          )}
        </button>
        <button
          onClick={toggle}
          className="min-w-0 flex-1 truncate text-left"
          style={{ color: 'var(--color-foreground)' }}
        >
          {entry.recipe_name}
        </button>
        <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
          <span>{tradeskillLabel(entry.tradeskill)}</span>
          {entry.role === 'ingredient' && entry.count > 1 && <span>×{entry.count}</span>}
          <span>Trivial {entry.trivial}</span>
          <FavoriteRecipeStar recipeId={entry.recipe_id} size={15} />
        </div>
      </div>
      {expanded && (
        <div className="pb-2 pl-6">
          {loading && <p className="py-1 text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>}
          {error && (
            <p className="py-1 text-xs" style={{ color: 'var(--color-destructive)' }}>
              Couldn’t load recipe.
            </p>
          )}
          {recipe && (
            <>
              <RecipeBody recipe={recipe} onNavigate={onNavigate} />
              <button
                onClick={() => {
                  navigate(`/recipes?select=${entry.recipe_id}`)
                  onNavigate?.()
                }}
                className="mt-1 text-xs underline decoration-dotted"
                style={{ color: 'var(--color-primary)' }}
              >
                Open in Recipes →
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

interface ItemTradeskillsTabProps {
  entries: ItemTradeskillEntry[]
  /** Called after an in-tab navigation — lets a host modal close itself. */
  onNavigate?: () => void
}

/**
 * Shared Tradeskills tab for item-detail views (the items page detail panel and
 * the item-detail modal). Lists recipes that produce or consume the item; each
 * row expands to the full recipe with clickable ingredients and a favorite star.
 */
export function ItemTradeskillsTab({ entries, onNavigate }: ItemTradeskillsTabProps): React.ReactElement {
  if (entries.length === 0) {
    return <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>Not used in any tradeskill recipe.</p>
  }
  const products = entries.filter((e) => e.role === 'product')
  const ingredients = entries.filter((e) => e.role === 'ingredient')
  return (
    <div className="flex flex-col gap-3">
      {products.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Produced by
          </div>
          {products.map((ts) => (
            <RecipeRow key={ts.recipe_id} entry={ts} onNavigate={onNavigate} />
          ))}
        </div>
      )}
      {ingredients.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Used as ingredient in
          </div>
          {ingredients.map((ts) => (
            <RecipeRow key={ts.recipe_id} entry={ts} onNavigate={onNavigate} />
          ))}
        </div>
      )}
    </div>
  )
}

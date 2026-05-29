import React from 'react'
import { useNavigate } from 'react-router-dom'

// VariantLinks surfaces the same-name duplicate rows that the item/spell lists
// collapse away. The game data ships several rows per name with different ids;
// lists show only the canonical one. On the canonical row's detail we list the
// hidden variant ids; on a variant's detail we link back to the main entry.
//
// `base` is the route that handles a ?select=<id> param ('/items' or
// '/spells'). Renders nothing when the row has no duplicates.
interface VariantLinksProps {
  base: '/items' | '/spells'
  variantIds?: number[]
  canonicalId?: number
}

export default function VariantLinks({
  base,
  variantIds,
  canonicalId,
}: VariantLinksProps): React.ReactElement | null {
  const navigate = useNavigate()
  const variants = variantIds ?? []
  const isVariant = !!canonicalId && canonicalId > 0
  // On a variant's detail, the sibling list includes the canonical id; pull it
  // out so it isn't also listed as a plain "other version" chip.
  const others = isVariant ? variants.filter((id) => id !== canonicalId) : variants

  if (others.length === 0 && !isVariant) return null

  const go = (id: number): void => {
    navigate(`${base}?select=${id}`)
  }

  const chipStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface-2)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-primary)',
  }

  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        Other Versions
      </div>
      <div
        className="rounded border px-3 py-2"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
      >
        <p className="mb-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {isVariant
            ? 'This is a duplicate entry. The main version used by the game is:'
            : 'The game data ships duplicate copies of this name under other database ids:'}
        </p>
        <div className="flex flex-wrap gap-1.5">
          {isVariant && (
            <button
              onClick={() => go(canonicalId)}
              className="rounded px-2 py-0.5 text-xs font-semibold underline decoration-dotted"
              style={chipStyle}
              title="View the main entry"
            >
              ★ #{canonicalId}
            </button>
          )}
          {others.map((id) => (
            <button
              key={id}
              onClick={() => go(id)}
              className="rounded px-2 py-0.5 text-xs underline decoration-dotted"
              style={chipStyle}
              title="View this version"
            >
              #{id}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}

import React from 'react'
import { describeLocation } from '../lib/inventoryLocations'
import type { ItemHolding } from '../services/itemHoldings'

interface ItemCharactersTabProps {
  holdings: ItemHolding[]
}

/**
 * "Characters" tab on the item detail views: lists which of the user's
 * characters hold the item and where, sourced from Zeal inventory exports.
 */
export default function ItemCharactersTab({ holdings }: ItemCharactersTabProps): React.ReactElement {
  if (holdings.length === 0) {
    return <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>No characters hold this item.</p>
  }
  return (
    <div>
      <div className="mb-1 flex justify-between text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        <span>Character</span>
        <span>Location</span>
      </div>
      {holdings.map((h, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <span className="min-w-0 truncate" style={{ color: 'var(--color-foreground)' }}>
            {h.character || 'Shared Bank'}
            {h.count > 1 && (
              <span className="ml-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                ×{h.count}
              </span>
            )}
          </span>
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {describeLocation(h.location)}
          </span>
        </div>
      ))}
      <p className="mt-3 text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Based on each character's most recent Zeal inventory export (written on camp).
      </p>
    </div>
  )
}

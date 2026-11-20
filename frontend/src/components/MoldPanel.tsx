/**
 * MoldPanel — Planes of Power armor mold/pattern cross-reference for the item
 * views (ItemsPage overview and the shared ItemDetailView modal/panel).
 */
import React from 'react'
import { useAppNavigate } from '../hooks/useAppNavigate'
import { useActiveCharacterSummary } from '../hooks/useActiveCharacterLevel'
import { isClassProduct, moldRecipeSummary } from '../lib/molds'
import type { ItemSources } from '../types/item'
import { ItemIcon } from './Icon'
import ItemHoverCard from './ItemHoverCard'

// Matches the item views' own Section styling.
function Section({ title, children }: { title: string; children: React.ReactNode }): React.ReactElement {
  return (
    <div>
      <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        {title}
      </div>
      <div className="rounded border px-3 py-1" style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}>
        {children}
      </div>
    </div>
  )
}

// MoldPanel cross-references Planes of Power armor molds/patterns: on a mold,
// the class armor it combines into (hover for the item, your class marked)
// and the combine; on that armor, the mold it comes from — the mold is what
// drops and gets bid or rolled on.
export default function MoldPanel({ sources, onNavigate }: {
  sources: ItemSources | null
  onNavigate?: () => void
}): React.ReactElement | null {
  const navigate = useAppNavigate()
  const activeChar = useActiveCharacterSummary()
  const products = sources?.mold_products ?? []
  const from = sources?.made_from_mold
  if (products.length === 0 && !from) return null
  const open = (id: number) => {
    onNavigate?.()
    navigate(`/items?select=${id}`)
  }
  const link = (id: number, icon: number, name: string) => (
    <ItemHoverCard itemId={id} clickHint="Click to open">
      <button onClick={() => open(id)}
        className="flex min-w-0 items-center gap-1.5 text-left underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}>
        <ItemIcon id={icon} name={name} size={16} />
        <span className="truncate">{name}</span>
      </button>
    </ItemHoverCard>
  )

  if (products.length === 0 && from) {
    return (
      <Section title="Made From">
        <div className="flex items-center justify-between gap-2 py-1 text-xs">
          {link(from.id, from.icon, from.name)}
          <span style={{ color: 'var(--color-muted-foreground)' }}>
            {from.nodrop === 0 ? 'NO DROP mold' : 'Tradeable mold'}
          </span>
        </div>
        <p className="pb-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          This armor doesn't drop: the mold does. Combine it with your class emblem to make it.
        </p>
      </Section>
    )
  }
  return (
    <Section title="Creates">
      {products.map((m) => {
        const mine = isClassProduct(m, activeChar?.classIndex)
        return (
          <div key={m.recipe_id} className="flex items-center justify-between gap-2 py-0.5 text-xs">
            {link(m.item_id, m.icon, m.name)}
            <span className="shrink-0"
              style={{ color: mine ? '#22c55e' : 'var(--color-muted-foreground)', fontWeight: mine ? 600 : undefined }}>
              {m.class_label}{mine ? ' · you' : ''}
            </span>
          </div>
        )
      })}
      <p className="py-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Combine with {moldRecipeSummary(products)}.
      </p>
    </Section>
  )
}

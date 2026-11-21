/**
 * MoldLootProducts — the sub-line under a PoP armor mold in an NPC loot
 * table: the class items the mold combines into, each with the item hover
 * card. The active character's piece leads and is highlighted. Shared by both
 * NPC overlay renderers (NPCPanel and NPCOverlayWindowPage), so it uses inline
 * styles and plain spans: NPCPanel's loot row is itself a <button>.
 */
import React from 'react'
import ItemHoverCard from '../ItemHoverCard'
import { CLASS_NAMES } from '../../lib/spellHelpers'
import type { MoldProductRef } from '../../types/npc'

function classAbbr(classes: number, label: string): string {
  const i = CLASS_NAMES.findIndex((_, k) => classes === 1 << k)
  return i >= 0 ? CLASS_NAMES[i] : label
}

// yourMoldProduct is the active character's piece among a mold's products.
export function yourMoldProduct(
  products: MoldProductRef[] | undefined,
  classIndex: number | null,
): MoldProductRef | undefined {
  if (!products || classIndex == null || classIndex < 0) return undefined
  return products.find((p) => (p.classes & (1 << classIndex)) !== 0)
}

export default function MoldLootProducts({
  products,
  classIndex,
  onItemClick,
  mutedColor,
  linkColor,
  yourColor = '#22c55e',
}: {
  products: MoldProductRef[]
  classIndex: number | null
  onItemClick?: (itemId: number) => void
  mutedColor: string
  linkColor: string
  yourColor?: string
}): React.ReactElement {
  const mine = yourMoldProduct(products, classIndex)
  const ordered = mine ? [mine, ...products.filter((p) => p !== mine)] : products
  return (
    <div
      style={{
        fontSize: 10, lineHeight: 1.4, color: mutedColor,
        padding: '0 4px 2px 28px', display: 'flex', flexWrap: 'wrap', gap: '0 6px',
      }}
    >
      <span title="PoP armor mold: combine with a class emblem to make one of these">makes</span>
      {ordered.map((p) => {
        const you = p === mine
        return (
          <span key={p.item_id} style={{ whiteSpace: 'nowrap' }}>
            <span style={{ color: you ? yourColor : mutedColor, fontWeight: you ? 600 : 400 }}>
              {classAbbr(p.classes, p.class_label)}
            </span>{' '}
            <ItemHoverCard
              itemId={p.item_id}
              clickHint={onItemClick ? 'Click to open in the item database' : undefined}
            >
              <span
                role={onItemClick ? 'link' : undefined}
                onClick={onItemClick ? (e) => { e.stopPropagation(); onItemClick(p.item_id) } : undefined}
                style={{
                  color: you ? yourColor : linkColor,
                  cursor: onItemClick ? 'pointer' : 'default',
                  textDecoration: onItemClick ? 'underline dotted' : undefined,
                }}
              >
                {p.name}
              </span>
            </ItemHoverCard>
          </span>
        )
      })}
    </div>
  )
}

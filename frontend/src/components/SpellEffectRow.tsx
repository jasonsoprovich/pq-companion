/**
 * SpellEffectRow — one "Click / Proc / Worn / Focus" line in an item's
 * Effects section, shared by the item detail modal and the Items page detail
 * panel. The spell name is a link into the Spells explorer; hovering it shows
 * the SpellHoverCard quick view.
 */
import React from 'react'
import { useNavigate } from 'react-router-dom'
import SpellHoverCard from './SpellHoverCard'

export default function SpellEffectRow({
  label, spellId, name, detail, effectsOnly,
}: {
  label: string
  spellId: number
  name: string
  /** Extra right-aligned annotation, e.g. "+41% Haste" on a worn effect. */
  detail?: string
  /** Worn/focus effects: hover card shows only the EFFECTS section. */
  effectsOnly?: boolean
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <div className="flex min-w-0 items-baseline justify-end gap-2">
        {detail && (
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-foreground)' }}>{detail}</span>
        )}
        <SpellHoverCard spellId={spellId} effectsOnly={effectsOnly}>
          <button
            onClick={() => navigate(`/spells?select=${spellId}`)}
            className="min-w-0 truncate text-right underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {name}
          </button>
        </SpellHoverCard>
      </div>
    </div>
  )
}

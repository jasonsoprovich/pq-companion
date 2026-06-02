import React from 'react'
import { useNavigate } from 'react-router-dom'
import type { ItemSourceNPC } from '../types/item'

// formatNPCName turns underscore-delimited DB names into display names.
export function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

// SourceNPCLink renders one NPC source row (vendor or drop): the NPC name links
// to the NPC page, with an optional drop rate and a zone link. Shared by the
// item detail modal and the spell acquisition panel so both look identical.
export function SourceNPCLink({ npc, showRate }: { npc: ItemSourceNPC; showRate?: boolean }): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex w-full items-center justify-between gap-3 py-0.5 text-sm">
      <button
        onClick={() => navigate(`/npcs?select=${npc.id}`)}
        className="min-w-0 truncate text-left underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
      >
        {formatNPCName(npc.name)}
      </button>
      <div className="flex shrink-0 items-center gap-2">
        {showRate && npc.drop_rate != null && npc.drop_rate > 0 && (
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {npc.drop_rate.toFixed(2)}%
          </span>
        )}
        {npc.zone_name && (
          <button
            onClick={() => navigate(`/zones?select=${npc.zone_short_name}`)}
            className="text-xs underline decoration-dotted"
            style={{ color: 'var(--color-muted)' }}
          >
            {npc.zone_name}
          </button>
        )}
      </div>
    </div>
  )
}

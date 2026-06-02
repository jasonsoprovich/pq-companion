import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { getSpellCrossRefs, getItem, getItemSources } from '../services/api'
import type { Item, ItemSources } from '../types/item'
import { priceLabel } from '../lib/itemHelpers'
import { SourceNPCLink } from './SourceNPCLink'
import { ItemTradeskillsTab } from './RecipeView'

// SpellAcquisition shows every in-game-data way to obtain a spell: it resolves
// the scroll that teaches the spell, then lists that scroll's vendors (with
// price), drop NPCs, tradeskill/research recipes, forage zones, and ground
// spawns — reusing the same source renderers as the item detail view so it
// looks consistent. Quests aren't represented in the game database, so when no
// source is found it says as much rather than implying the spell is
// unobtainable.
export default function SpellAcquisition({
  spellId,
  onNavigate,
}: {
  spellId: number
  onNavigate?: () => void
}): React.ReactElement {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [item, setItem] = useState<Item | null>(null)
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setItem(null)
    setSources(null)
    setError(false)

    const sourceCount = (s: ItemSources | null): number =>
      s
        ? (s.merchants?.length ?? 0) +
          (s.drops?.length ?? 0) +
          (s.tradeskills?.filter((t) => t.role === 'product').length ?? 0) +
          (s.forage_zones?.length ?? 0) +
          (s.ground_spawns?.length ?? 0)
        : 0

    void (async () => {
      try {
        const cr = await getSpellCrossRefs(spellId)
        const scrolls = cr.scroll_items
        if (scrolls.length === 0) {
          if (!cancelled) setLoading(false)
          return
        }
        // A spell can have several scroll variants (e.g. a no-trade quest copy
        // with no sources next to the real one). Pick the variant with the most
        // sources.
        const counted = await Promise.all(
          scrolls.map(async (s) => {
            const src = await getItemSources(s.id).catch(() => null)
            return { id: s.id, src, count: sourceCount(src) }
          }),
        )
        counted.sort((a, b) => b.count - a.count)
        const best = counted[0]
        const it = await getItem(best.id)
        if (cancelled) return
        setItem(it)
        setSources(best.src)
      } catch {
        if (!cancelled) setError(true)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [spellId])

  if (loading) {
    return <p className="py-2 text-sm" style={{ color: 'var(--color-muted)' }}>Loading sources…</p>
  }
  if (error) {
    return <p className="py-2 text-sm" style={{ color: 'var(--color-muted)' }}>Could not load acquisition info.</p>
  }

  const merchants = sources?.merchants ?? []
  const drops = sources?.drops ?? []
  const recipes = (sources?.tradeskills ?? []).filter((e) => e.role === 'product')
  const forage = sources?.forage_zones ?? []
  const ground = sources?.ground_spawns ?? []
  const hasAny = merchants.length + drops.length + recipes.length + forage.length + ground.length > 0

  if (!hasAny) {
    return (
      <p className="py-2 text-sm" style={{ color: 'var(--color-muted)' }}>
        No vendor, drop, or tradeskill source in the game data — this spell is likely a quest
        reward or a starting spell.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      {merchants.length > 0 && (
        <Group title={`Purchased from${item && item.price > 0 ? ` — ${priceLabel(item.price)}` : ''}`}>
          {merchants.map((npc) => (
            <SourceNPCLink key={npc.id} npc={npc} />
          ))}
        </Group>
      )}
      {drops.length > 0 && (
        <Group title="Dropped by">
          {drops.map((npc) => (
            <SourceNPCLink key={npc.id} npc={npc} showRate />
          ))}
        </Group>
      )}
      {recipes.length > 0 && (
        <Group title="Crafted / researched">
          <ItemTradeskillsTab entries={recipes} onNavigate={onNavigate} />
        </Group>
      )}
      {forage.length > 0 && (
        <Group title="Foraged in">
          {forage.map((fz, i) => (
            <ZoneRow
              key={i}
              label={fz.zone_name || fz.zone_short_name}
              detail={fz.chance > 0 ? `${fz.chance}%` : undefined}
              onClick={() => {
                navigate(`/zones?select=${fz.zone_short_name}`)
                onNavigate?.()
              }}
            />
          ))}
        </Group>
      )}
      {ground.length > 0 && (
        <Group title="Ground spawn">
          {ground.map((gs, i) => (
            <ZoneRow
              key={i}
              label={gs.zone_name || gs.zone_short_name}
              detail={gs.name}
              onClick={() => {
                navigate(`/zones?select=${gs.zone_short_name}`)
                onNavigate?.()
              }}
            />
          ))}
        </Group>
      )}
    </div>
  )
}

function Group({ title, children }: { title: string; children: React.ReactNode }): React.ReactElement {
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

function ZoneRow({ label, detail, onClick }: { label: string; detail?: string; onClick: () => void }): React.ReactElement {
  return (
    <div className="flex items-center justify-between gap-3 py-0.5 text-sm">
      <button onClick={onClick} className="min-w-0 truncate text-left underline decoration-dotted" style={{ color: 'var(--color-primary)' }}>
        {label}
      </button>
      {detail && <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{detail}</span>}
    </div>
  )
}

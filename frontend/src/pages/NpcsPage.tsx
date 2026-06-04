import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useCachedState } from '../hooks/useCachedState'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Search, X, Bell } from 'lucide-react'
import { searchNPCs, getNPC, getNPCSpawns, getNPCLoot, getNPCFaction, getNPCSpells, getNPCRaw } from '../services/api'
import type { NPC, NPCSpawns, NPCLootTable, NPCFaction, NPCSpells, NPCSpellEntry, NPCSpellProc, LootDrop } from '../types/npc'
import {
  bodyTypeName,
  className,
  npcDisplayName,
  npcRunSpeedPct,
  npcSpecialAbilities,
  specialAbilityAlertPattern,
  type SpecialAbility,
} from '../lib/npcHelpers'
import { inGameItemLink } from '../lib/itemHelpers'
import CreateTriggerModal, { type TriggerPrefill } from '../components/CreateTriggerModal'
import { ItemIcon } from '../components/Icon'
import RawDataModal from './../components/RawDataModal'
import NPCCasterSummarySection from '../components/overlays/NPCCasterSummarySection'
import { DEFAULT_NPC_OVERLAY_SECTIONS } from '../types/config'

function formatRespawnTime(seconds: number): string {
  if (seconds <= 0) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  const parts: string[] = []
  if (h > 0) parts.push(`${h}h`)
  if (m > 0) parts.push(`${m}m`)
  if (s > 0 || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}

function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (npc: NPC) => void
}

const NPC_PAGE_SIZE = 50

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useCachedState('npcs.query', '')
  const [npcs, setNpcs] = useState<NPC[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showPlaceholders, setShowPlaceholders] = useCachedState('npcs.showPlaceholders', true)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string, placeholders: boolean) => {
    setLoading(true)
    setError(null)
    searchNPCs(q, NPC_PAGE_SIZE, 0, placeholders)
      .then((res) => {
        setNpcs(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  const loadMore = useCallback(() => {
    setLoadingMore(true)
    searchNPCs(query, NPC_PAGE_SIZE, npcs.length, showPlaceholders)
      .then((res) => {
        setNpcs((prev) => [...prev, ...(res.items ?? [])])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoadingMore(false))
  }, [query, showPlaceholders, npcs.length])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, showPlaceholders), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, showPlaceholders, runSearch])

  useEffect(() => {
    runSearch(query, showPlaceholders)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runSearch])

  const hasMore = !loading && npcs.length < total

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Search input */}
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search NPCs…"
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

      {/* Result count + placeholder toggle */}
      <div
        className="flex items-center justify-between border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        <span>
          {loading
            ? 'Searching…'
            : error
              ? 'Error'
              : npcs.length < total
                ? `${npcs.length.toLocaleString()} of ${total.toLocaleString()} NPCs`
                : `${total.toLocaleString()} NPCs`}
        </span>
        <button
          onClick={() => setShowPlaceholders((v) => !v)}
          title={showPlaceholders ? 'Hide placeholder NPCs' : 'Show placeholder NPCs'}
          className="rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors"
          style={{
            backgroundColor: showPlaceholders ? 'var(--color-surface-2)' : 'transparent',
            color: showPlaceholders ? 'var(--color-primary)' : 'var(--color-muted)',
            border: `1px solid ${showPlaceholders ? 'var(--color-primary)' : 'var(--color-border)'}`,
          }}
        >
          +placeholders
        </button>
      </div>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          npcs.map((npc) => (
            <button
              key={npc.id}
              onClick={() => onSelect(npc)}
              className="w-full px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === npc.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === npc.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <div
                className="truncate text-sm font-medium"
                style={{
                  color:
                    selectedId === npc.id
                      ? 'var(--color-primary)'
                      : 'var(--color-foreground)',
                }}
              >
                {npcDisplayName(npc)}
              </div>
              <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {`Lv ${npc.level} ${className(npc.class)}`}
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
              {loadingMore ? 'Loading…' : `Show more (${(total - npcs.length).toLocaleString()} remaining)`}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Detail panel helpers ───────────────────────────────────────────────────────

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps): React.ReactElement {
  return (
    <div className="flex justify-between py-0.5 text-sm">
      <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps): React.ReactElement {
  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      <div
        className="rounded border px-3 py-1"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {children}
      </div>
    </div>
  )
}

// ── Special Abilities list with popover ────────────────────────────────────────

interface SpecialAbilitiesListProps {
  abilities: SpecialAbility[]
}

function SpecialAbilitiesList({ abilities }: SpecialAbilitiesListProps): React.ReactElement {
  const [activeCode, setActiveCode] = useState<number | null>(null)
  const [alertPrefill, setAlertPrefill] = useState<TriggerPrefill | null>(null)
  const containerRef = useRef<HTMLDivElement | null>(null)

  function toggle(code: number) {
    setActiveCode((prev) => (prev === code ? null : code))
  }

  // Dismiss the open popover on Escape or any click outside the badge row.
  // The ref wraps both the badges AND the popover, so clicking another
  // badge falls through to that badge's own toggle handler (no double
  // close + reopen race), and only clicks completely outside the abilities
  // section dismiss.
  useEffect(() => {
    if (activeCode === null) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setActiveCode(null)
    }
    const onClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setActiveCode(null)
      }
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onClick)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onClick)
    }
  }, [activeCode])

  function openCreateAlert(sa: SpecialAbility) {
    const known = specialAbilityAlertPattern(sa.code)
    setAlertPrefill({
      name: `${sa.name} Alert`,
      pattern: known?.pattern ?? '',
      displayText: known?.text ?? sa.name.toUpperCase(),
      displayColor: '#ff4444',
      timerType: 'none',
    })
    setActiveCode(null)
  }

  return (
    <div ref={containerRef} className="flex flex-wrap gap-1.5 py-1">
      {abilities.map((sa) => (
        <div key={sa.code} className="relative">
          <button
            onClick={() => toggle(sa.code)}
            className="rounded px-2 py-0.5 text-[11px] font-medium transition-colors"
            style={{
              backgroundColor: activeCode === sa.code ? 'var(--color-surface-3, var(--color-surface-2))' : 'var(--color-surface-2)',
              color: activeCode === sa.code ? 'var(--color-primary)' : 'var(--color-foreground)',
              border: `1px solid ${activeCode === sa.code ? 'var(--color-primary)' : 'var(--color-border)'}`,
              cursor: 'pointer',
            }}
          >
            {sa.name}
          </button>
          {activeCode === sa.code && (
            <div
              className="absolute left-0 top-full z-10 mt-1 w-64 rounded border p-2 text-xs shadow-lg"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: 'var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
            >
              {sa.description && <p className="mb-1.5">{sa.description}</p>}
              <button
                onClick={() => openCreateAlert(sa)}
                className="flex items-center gap-1.5 text-[11px] px-2 py-0.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-background)',
                  border: '1px solid transparent',
                }}
              >
                <Bell size={10} /> Create Alert
              </button>
            </div>
          )}
        </div>
      ))}
      {alertPrefill && (
        <CreateTriggerModal
          prefill={alertPrefill}
          onClose={() => setAlertPrefill(null)}
        />
      )}
    </div>
  )
}

// ── Spells & Procs section ────────────────────────────────────────────────────

interface NPCSpellsSectionProps {
  spells: NPCSpells
  onSpellClick: (id: number) => void
}

function SpellLink({ id, name, onClick }: { id: number; name: string; onClick: (id: number) => void }): React.ReactElement {
  if (id <= 0) return <span style={{ color: 'var(--color-muted-foreground)' }}>—</span>
  return (
    <button
      type="button"
      onClick={() => onClick(id)}
      className="hover:underline"
      style={{ color: 'var(--color-primary)', cursor: 'pointer', background: 'none', border: 'none', padding: 0, font: 'inherit' }}
      title={`Open spell ${id} in the spell explorer`}
    >
      {name || `Spell #${id}`}
    </button>
  )
}

function ProcRow({ label, proc, onClick }: { label: string; proc: NPCSpellProc; onClick: (id: number) => void }): React.ReactElement {
  return (
    <div className="flex items-center justify-between gap-2 py-0.5 text-sm">
      <div className="flex items-center gap-2">
        <span style={{ color: 'var(--color-muted-foreground)', minWidth: 90 }}>{label}</span>
        <SpellLink id={proc.spell_id} name={proc.spell_name} onClick={onClick} />
      </div>
      <span style={{ color: 'var(--color-muted)' }} className="text-xs">
        {proc.chance}%
      </span>
    </div>
  )
}

// SPELL_LIST_COLLAPSED_COUNT is the cap on spells shown per source bucket
// before the "Show N more" expand button kicks in. Geonid shamans and
// other heavy casters can have 30+ entries; truncating keeps the section
// from dominating the detail page.
const SPELL_LIST_COLLAPSED_COUNT = 10

function SpellBucket({
  title,
  entries,
  onSpellClick,
}: {
  title: string
  entries: NPCSpellEntry[]
  onSpellClick: (id: number) => void
}): React.ReactElement {
  const [expanded, setExpanded] = useState(false)
  const overflow = entries.length - SPELL_LIST_COLLAPSED_COUNT
  const visible = expanded || overflow <= 0 ? entries : entries.slice(0, SPELL_LIST_COLLAPSED_COUNT)

  return (
    <div>
      <div className="mb-0.5 text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        {title} <span style={{ color: 'var(--color-muted)' }}>({entries.length})</span>
      </div>
      <div className="rounded border" style={{ backgroundColor: 'var(--color-surface-2)', borderColor: 'var(--color-border)' }}>
        {visible.map((e) => (
          <div
            key={`${e.source_id}-${e.spell_id}`}
            className="flex items-center justify-between gap-2 px-2 py-1 text-sm"
            style={{ borderTop: '1px solid var(--color-border)' }}
          >
            <SpellLink id={e.spell_id} name={e.spell_name} onClick={onSpellClick} />
            <span className="text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
              L{e.min_level}{e.max_level < 255 ? `–${e.max_level}` : '+'}
              {e.priority !== 0 ? ` · pri ${e.priority}` : ''}
              {e.recast_delay > 0 ? ` · ${Math.round(e.recast_delay / 1000)}s` : ''}
            </span>
          </div>
        ))}
        {overflow > 0 && (
          <button
            type="button"
            onClick={() => setExpanded((s) => !s)}
            className="w-full px-2 py-1 text-xs"
            style={{
              color: 'var(--color-primary)',
              background: 'none',
              border: 'none',
              borderTop: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
          >
            {expanded ? '▴ Show less' : `▾ Show ${overflow} more`}
          </button>
        )}
      </div>
    </div>
  )
}

// Groups inherited entries together so a list with a parent_list chain
// reads as "this NPC's own list" + "inherited from <parent>".
function groupEntriesBySource(entries: NPCSpellEntry[], ownListID: number): { source: string; ownSource: boolean; entries: NPCSpellEntry[] }[] {
  const buckets = new Map<number, { source: string; ownSource: boolean; entries: NPCSpellEntry[] }>()
  for (const e of entries) {
    const key = e.source_id
    if (!buckets.has(key)) {
      buckets.set(key, {
        source: e.source_name || `List #${e.source_id}`,
        ownSource: e.source_id === ownListID,
        entries: [],
      })
    }
    buckets.get(key)!.entries.push(e)
  }
  // Own list first; the rest in insertion order (which mirrors parent chain depth).
  return Array.from(buckets.values()).sort((a, b) => Number(b.ownSource) - Number(a.ownSource))
}

function NPCSpellsSection({ spells, onSpellClick }: NPCSpellsSectionProps): React.ReactElement {
  const [showTiming, setShowTiming] = useState(false)
  const [showFullList, setShowFullList] = useState(false)
  const procs: { label: string; proc?: NPCSpellProc }[] = [
    { label: 'Attack proc', proc: spells.attack_proc },
    { label: 'Range proc', proc: spells.range_proc },
    { label: 'Defensive proc', proc: spells.defensive_proc },
  ]
  const procsPresent = procs.some((p) => p.proc)
  const grouped = groupEntriesBySource(spells.entries, spells.npc_spells_id)

  const hasTimingData =
    spells.fail_recast > 0 ||
    spells.engaged_b_self_chance > 0 ||
    spells.engaged_b_other_chance > 0 ||
    spells.engaged_d_chance > 0 ||
    spells.pursue_d_chance > 0 ||
    spells.idle_b_chance > 0

  return (
    <div className="space-y-2 py-1">
      {/* Distilled summary — same readout as the NPC overlays, for consistency
          across the dashboard panel, popout, and this page. */}
      {spells.summary && (
        <NPCCasterSummarySection
          summary={spells.summary}
          sections={DEFAULT_NPC_OVERLAY_SECTIONS}
          showHeading={false}
          onSpellClick={onSpellClick}
          theme={{
            heading: 'var(--color-muted)',
            muted: 'var(--color-muted)',
            chipBg: 'rgba(255,255,255,0.08)',
            chipText: 'var(--color-foreground)',
          }}
        />
      )}

      {/* Full enumerated list (procs + per-source buckets), collapsed by default
          — the summary above covers the at-a-glance need. */}
      {spells.entries.length > 0 && (
        <div>
          <button
            type="button"
            onClick={() => setShowFullList((s) => !s)}
            className="text-[11px]"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer', background: 'none', border: 'none', padding: 0 }}
          >
            {showFullList ? '▾ Hide full spell list' : `▸ Show full spell list (${spells.entries.length})`}
          </button>
          {showFullList && (
            <div className="mt-2 space-y-2">
              {procsPresent && (
                <div>
                  <div className="mb-0.5 text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>Procs</div>
                  <div className="rounded border px-2 py-1" style={{ backgroundColor: 'var(--color-surface-2)', borderColor: 'var(--color-border)' }}>
                    {procs.map(({ label, proc }) => (proc ? <ProcRow key={label} label={label} proc={proc} onClick={onSpellClick} /> : null))}
                  </div>
                </div>
              )}
              {grouped.map((bucket) => (
                <SpellBucket
                  key={bucket.source}
                  title={bucket.ownSource ? 'Cast spells' : `Inherited from ${bucket.source}`}
                  entries={bucket.entries}
                  onSpellClick={onSpellClick}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {hasTimingData && (
        <div>
          <button
            type="button"
            onClick={() => setShowTiming((s) => !s)}
            className="text-[11px]"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer', background: 'none', border: 'none', padding: 0 }}
          >
            {showTiming ? '▾ Hide AI timing' : '▸ Show AI timing'}
          </button>
          {showTiming && (
            <div className="mt-1 rounded border px-2 py-1 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}>
              {spells.fail_recast > 0 && <div>Fail recast: {spells.fail_recast} ms</div>}
              {spells.engaged_b_self_chance > 0 && <div>Engaged · self-buff chance: {spells.engaged_b_self_chance}%</div>}
              {spells.engaged_b_other_chance > 0 && <div>Engaged · ally-buff chance: {spells.engaged_b_other_chance}%</div>}
              {spells.engaged_d_chance > 0 && <div>Engaged · detrimental chance: {spells.engaged_d_chance}%</div>}
              {spells.pursue_d_chance > 0 && <div>Pursuing · detrimental chance: {spells.pursue_d_chance}%</div>}
              {spells.idle_b_chance > 0 && <div>Idle · buff chance: {spells.idle_b_chance}%</div>}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

interface DetailPanelProps {
  npc: NPC | null
}

function DetailPanel({ npc }: DetailPanelProps): React.ReactElement {
  const navigate = useNavigate()
  const [spawns, setSpawns] = useState<NPCSpawns | null>(null)
  const [loot, setLoot] = useState<NPCLootTable | null>(null)
  const [faction, setFaction] = useState<NPCFaction | null>(null)
  const [spells, setSpells] = useState<NPCSpells | null>(null)
  const [bulkCopied, setBulkCopied] = useState<number | null>(null)
  const [rawOpen, setRawOpen] = useState(false)
  const rawFetcher = useCallback(() => getNPCRaw(npc!.id), [npc?.id])

  useEffect(() => {
    if (!npc) { setSpawns(null); setLoot(null); setFaction(null); return }
    let cancelled = false
    getNPCSpawns(npc.id)
      .then((s) => { if (!cancelled) setSpawns(s) })
      .catch(() => { if (!cancelled) setSpawns({ spawn_points: [], spawn_groups: [] }) })
    getNPCLoot(npc.id)
      .then((l) => { if (!cancelled) setLoot(l) })
      .catch(() => { if (!cancelled) setLoot(null) })
    if (npc.npc_faction_id > 0) {
      getNPCFaction(npc.id)
        .then((f) => { if (!cancelled) setFaction(f) })
        .catch(() => { if (!cancelled) setFaction(null) })
    } else {
      setFaction(null)
    }
    if (npc.npc_spells_id > 0) {
      getNPCSpells(npc.id)
        .then((sp) => { if (!cancelled) setSpells(sp) })
        .catch(() => { if (!cancelled) setSpells(null) })
    } else {
      setSpells(null)
    }
    return () => { cancelled = true }
  }, [npc?.id])

  function copyBulkLinks(dropId: number, items: { item_id: number; item_name: string }[]) {
    const links = items.map((it) => inGameItemLink(it.item_id, it.item_name)).join('\n')
    navigator.clipboard.writeText(links).then(() => {
      setBulkCopied(dropId)
      setTimeout(() => setBulkCopied(null), 2000)
    })
  }

  function renderDrops(drops: LootDrop[]): React.ReactElement[] {
    return drops.map((drop) => (
      <div key={drop.id} className="mb-2 last:mb-0">
        <div className="flex items-center justify-between pt-1 pb-0.5">
          <span className="text-[11px] font-medium" style={{ color: 'var(--color-muted)' }}>
            {drop.name ? `${drop.name} · ` : ''}
            {drop.multiplier > 1 ? `×${drop.multiplier} · ` : ''}
            {drop.probability < 100 ? `${drop.probability}% chance` : 'Always drops'}
          </span>
          <button
            onClick={() => copyBulkLinks(drop.id, drop.items)}
            title="Bulk copy in-game links"
            className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-medium transition-colors"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: 'var(--color-border)',
              color: bulkCopied === drop.id ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            }}
          >
            {bulkCopied === drop.id ? <Check size={10} /> : <Copy size={10} />}
            {bulkCopied === drop.id ? 'Copied!' : 'Copy links'}
          </button>
        </div>
        {drop.items.map((item) => (
          <button
            key={item.item_id}
            onClick={() => navigate(`/items?select=${item.item_id}`)}
            className="flex w-full items-center gap-2 border-t py-0.5 text-left text-sm"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <ItemIcon id={item.item_icon} name={item.item_name} size={20} />
            <span
              className="truncate underline decoration-dotted flex-1"
              style={{ color: 'var(--color-primary)' }}
            >
              {item.item_name}
            </span>
            <span className="ml-3 shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
              {item.chance.toFixed(1)}%
              {item.multiplier > 1 && ` ×${item.multiplier}`}
            </span>
          </button>
        ))}
      </div>
    ))
  }

  if (!npc) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select an NPC to view details
        </p>
      </div>
    )
  }

  const specialAbilities = npcSpecialAbilities(npc)

  const flags: string[] = []
  if (npc.raid_target) flags.push('RAID TARGET')
  if (npc.rare_spawn) flags.push('RARE SPAWN')

  const hasStats = npc.str || npc.sta || npc.dex || npc.agi || npc.int || npc.wis || npc.cha
  const hasResists = npc.mr || npc.cr || npc.dr || npc.fr || npc.pr

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {/* Header */}
      <div className="mb-4">
        <div className="flex items-start justify-between gap-3">
          <h2
            className="text-xl font-bold leading-tight"
            style={{ color: 'var(--color-primary)' }}
          >
            {npcDisplayName(npc)}
          </h2>
          <button
            onClick={() => setRawOpen(true)}
            className="shrink-0 rounded px-2 py-1 text-xs"
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
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            {`Level ${npc.level} ${className(npc.class)} · ${npc.race_name}`}
          </span>
          {flags.map((f) => (
            <span
              key={f}
              className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-primary)',
                border: '1px solid var(--color-border)',
              }}
            >
              {f}
            </span>
          ))}
        </div>
        <div className="mt-0.5 text-xs" style={{ color: 'var(--color-muted)' }}>
          {bodyTypeName(npc.body_type)}
        </div>
      </div>

      <div className="flex flex-col gap-3">
        {/* Combat */}
        <Section title="Combat">
          <StatRow label="HP" value={npc.hp.toLocaleString()} />
          {npc.mana > 0 && <StatRow label="Mana" value={npc.mana.toLocaleString()} />}
          <StatRow label="Damage" value={`${npc.min_dmg}–${npc.max_dmg}`} />
          {npc.attack_count > 0 && (
            <StatRow label="Attacks" value={npc.attack_count} />
          )}
          <StatRow label="AC" value={npc.ac} />
        </Section>

        {/* Attributes */}
        {hasStats ? (
          <Section title="Attributes">
            {npc.str > 0 && <StatRow label="STR" value={npc.str} />}
            {npc.sta > 0 && <StatRow label="STA" value={npc.sta} />}
            {npc.dex > 0 && <StatRow label="DEX" value={npc.dex} />}
            {npc.agi > 0 && <StatRow label="AGI" value={npc.agi} />}
            {npc.int > 0 && <StatRow label="INT" value={npc.int} />}
            {npc.wis > 0 && <StatRow label="WIS" value={npc.wis} />}
            {npc.cha > 0 && <StatRow label="CHA" value={npc.cha} />}
          </Section>
        ) : null}

        {/* Resists */}
        {hasResists ? (
          <Section title="Resists">
            {npc.mr > 0 && <StatRow label="Magic" value={npc.mr} />}
            {npc.cr > 0 && <StatRow label="Cold" value={npc.cr} />}
            {npc.dr > 0 && <StatRow label="Disease" value={npc.dr} />}
            {npc.fr > 0 && <StatRow label="Fire" value={npc.fr} />}
            {npc.pr > 0 && <StatRow label="Poison" value={npc.pr} />}
          </Section>
        ) : null}

        {/* Special Abilities */}
        {specialAbilities.length > 0 && (
          <Section title="Special Abilities">
            <SpecialAbilitiesList abilities={specialAbilities} />
          </Section>
        )}

        {/* Spells & Procs */}
        {spells && (spells.entries.length > 0 || spells.attack_proc || spells.range_proc || spells.defensive_proc) && (
          <Section title="Spells & Procs">
            <NPCSpellsSection spells={spells} onSpellClick={(id) => navigate(`/spells?select=${id}`)} />
          </Section>
        )}

        {/* Behavior */}
        <Section title="Behavior">
          {npc.aggro_radius > 0 && (
            <StatRow label="Aggro Radius" value={npc.aggro_radius} />
          )}
          <StatRow label="Run Speed" value={`${npcRunSpeedPct(npc.run_speed)}%`} />
          <StatRow label="Size" value={npc.size.toFixed(1)} />
        </Section>

        {/* Info */}
        <Section title="Info">
          <StatRow label="NPC ID" value={npc.id} />
          {npc.exp_pct !== 100 && <StatRow label="Exp %" value={`${npc.exp_pct}%`} />}
          {npc.merchant_id > 0 && (
            <StatRow label="Merchant ID" value={npc.merchant_id} />
          )}
          {npc.npc_spells_id > 0 && (
            <StatRow label="Spells ID" value={npc.npc_spells_id} />
          )}
          {npc.npc_faction_id > 0 && !faction && (
            <StatRow label="Faction ID" value={npc.npc_faction_id} />
          )}
          {npc.spell_scale !== 100 && (
            <StatRow label="Spell Scale" value={`${npc.spell_scale.toFixed(0)}%`} />
          )}
          {npc.heal_scale !== 100 && (
            <StatRow label="Heal Scale" value={`${npc.heal_scale.toFixed(0)}%`} />
          )}
        </Section>

        {/* Faction */}
        {faction && (
          <Section title="Faction">
            {faction.primary_faction_name && (
              <StatRow label="Primary" value={faction.primary_faction_name} />
            )}
            {faction.hits.length > 0 && (
              <div className="py-1">
                <div
                  className="mb-1 text-[10px] font-medium uppercase tracking-wide"
                  style={{ color: 'var(--color-muted)' }}
                >
                  On Kill
                </div>
                {faction.hits.map((hit) => (
                  <div key={hit.faction_id} className="flex justify-between border-t py-0.5 text-sm" style={{ borderColor: 'var(--color-border)' }}>
                    <span style={{ color: 'var(--color-muted-foreground)' }}>{hit.faction_name}</span>
                    <span
                      style={{
                        color: hit.value > 0 ? 'var(--color-primary)' : hit.value < 0 ? '#f87171' : 'var(--color-muted)',
                        fontVariantNumeric: 'tabular-nums',
                      }}
                    >
                      {hit.value > 0 ? `+${hit.value}` : hit.value}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Section>
        )}

        {/* Loot Table */}
        {loot && loot.drops.length > 0 && (
          <Section title="Loot Table">{renderDrops(loot.drops)}</Section>
        )}

        {/* Zone-wide loot overlay (e.g., Vex Thal shared drops) */}
        {loot && loot.zone_wide_drops && loot.zone_wide_drops.length > 0 && (
          <Section title={loot.zone_wide_label || 'Zone-wide loot'}>
            {renderDrops(loot.zone_wide_drops)}
          </Section>
        )}

        {/* Spawns */}
        {spawns && (spawns.spawn_points?.length ?? 0) > 0 && (
          <Section title="Spawns">
            <div
              className="mb-1 grid gap-x-3 pt-1 text-[11px] font-medium"
              style={{
                gridTemplateColumns: '1fr auto auto auto auto',
                color: 'var(--color-muted)',
              }}
            >
              <span>Zone</span>
              <span className="text-right">Y</span>
              <span className="text-right">X</span>
              <span className="text-right">Z</span>
              <span className="text-right">Respawn</span>
            </div>
            {spawns.spawn_points.map((sp) => (
              <div
                key={sp.id}
                className="grid items-center gap-x-3 border-t py-0.5 text-sm"
                style={{
                  gridTemplateColumns: '1fr auto auto auto auto',
                  borderColor: 'var(--color-border)',
                }}
              >
                <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
                  {sp.zone_name || sp.zone}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.y.toFixed(0)}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.x.toFixed(0)}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.z.toFixed(0)}
                </span>
                <span className="text-xs text-right whitespace-nowrap" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.fast_respawn_time > 0
                    ? `${formatRespawnTime(sp.fast_respawn_time)} / ${formatRespawnTime(sp.respawn_time)}`
                    : formatRespawnTime(sp.respawn_time)}
                </span>
              </div>
            ))}
          </Section>
        )}

        {/* Spawn Groups */}
        {spawns && (spawns.spawn_groups?.length ?? 0) > 0 && (
          <Section title="Spawn Groups">
            {spawns.spawn_groups.map((sg, i) => (
              <div
                key={sg.id}
                className={i > 0 ? 'mt-3' : ''}
              >
                <div className="flex items-center justify-between pt-1 pb-0.5">
                  <span className="text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                    {sg.name}
                  </span>
                  <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {sg.fast_respawn_time > 0
                      ? `${formatRespawnTime(sg.fast_respawn_time)} / ${formatRespawnTime(sg.respawn_time)}`
                      : formatRespawnTime(sg.respawn_time)}
                  </span>
                </div>
                {sg.members.map((m) => (
                  <div
                    key={m.npc_id}
                    className="flex items-center justify-between border-t py-0.5 text-sm"
                    style={{ borderColor: 'var(--color-border)' }}
                  >
                    <span
                      className="truncate"
                      style={{
                        color: m.npc_id === npc.id
                          ? 'var(--color-primary)'
                          : 'var(--color-foreground)',
                      }}
                    >
                      {formatNPCName(m.name)}
                    </span>
                    <span className="ml-3 shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
                      {m.chance}%
                    </span>
                  </div>
                ))}
              </div>
            ))}
          </Section>
        )}
      </div>

      <RawDataModal
        open={rawOpen}
        title={npcDisplayName(npc)}
        fetcher={rawFetcher}
        onClose={() => setRawOpen(false)}
      />
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function NpcsPage(): React.ReactElement {
  const [selected, setSelected] = useState<NPC | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  // Selection is driven by the URL (?select=id) so each pick is its own history
  // entry — Back steps from one selected NPC to the previously selected one
  // rather than jumping straight to the prior page (issue #9).
  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) {
      setSelected(null)
      return
    }
    if (selected?.id === id) return // already showing it (set on click)
    let cancelled = false
    getNPC(id).then((n) => { if (!cancelled) setSelected(n) }).catch(() => {/* ignore */})
    return () => { cancelled = true }
  }, [searchParams, selected])

  const handleSelect = useCallback(
    (npc: NPC | null) => {
      setSelected(npc)
      setSearchParams(npc ? { select: String(npc.id) } : {})
    },
    [setSearchParams],
  )

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={handleSelect} />
      {/* Key by id so the panel remounts on each pick — spawns/loot/faction/
          spells reset synchronously instead of lingering from the previously
          viewed NPC and flickering when the new fetches land. */}
      <DetailPanel key={selected?.id ?? 'none'} npc={selected} />
    </div>
  )
}

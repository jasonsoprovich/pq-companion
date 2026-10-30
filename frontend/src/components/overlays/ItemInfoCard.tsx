/**
 * ItemInfoCard — the Item Info overlay's single-card summary of an item:
 * "what is it for, can I buy it, where, and do I already have it?" Sections
 * (key facts, effects, characters, tradeskills, quests, vendors, drops, other
 * sources) follow the user's order and visibility from Settings, and any list
 * longer than the section's limit collapses to a count that expands in place.
 * Shared by the dashboard panel, the popout window and the Settings preview.
 */
import React, { useEffect, useMemo, useState } from 'react'
import { Check, ChevronRight, MapPin, X } from 'lucide-react'
import { useAppNavigate } from '../../hooks/useAppNavigate'
import { useLiveZone } from '../../hooks/useLiveZone'
import {
  useItemInfoSettings,
  type ItemInfoSettings,
  type ItemInfoSurface,
} from '../../hooks/useItemInfoSettings'
import { getItemQuests, getItemSources } from '../../services/api'
import { findHoldingsForItems, type ItemHolding } from '../../services/itemHoldings'
import { loadEnums, tradeskillLabel } from '../../lib/enumsCache'
import { bagContainerInfo, bagSlotInfo, isEquipmentSlot } from '../../lib/inventoryLocations'
import {
  decodeRaces,
  effectiveItemTypeLabel,
  isLoreItem,
  priceLabel,
  sizeLabel,
  slotsLabel,
  weaponRatioLabel,
  weightLabel,
} from '../../lib/itemHelpers'
import { CLASS_NAMES } from '../../lib/spellHelpers'
import { sectionKey } from '../../lib/npcItemFilters'
import { sectionDef, type ItemInfoSectionId } from '../../lib/itemInfoLayout'
import { ItemIcon } from '../Icon'
import { ResistChip } from '../ResistChip'
import SpellHoverCard from '../SpellHoverCard'
import WishlistStarButton from '../WishlistStarButton'
import type { Item, ItemQuests, ItemSourceNPC, ItemSources, ItemTradeskillEntry } from '../../types/item'

// ── Theme ──────────────────────────────────────────────────────────────────────

// The popout draws on its own translucent black; the dashboard panel sits on
// the app's surface colours. Same layout, two palettes (like the NPC overlay).
export interface ItemInfoTheme {
  text: string
  muted: string
  faint: string
  chipBg: string
  divider: string
  link: string
}

export const POPOUT_THEME: ItemInfoTheme = {
  text: 'rgba(255,255,255,0.9)',
  muted: 'rgba(255,255,255,0.55)',
  faint: 'rgba(255,255,255,0.4)',
  chipBg: 'rgba(255,255,255,0.06)',
  divider: 'rgba(255,255,255,0.08)',
  link: '#c9a84c',
}

export const DASHBOARD_THEME: ItemInfoTheme = {
  text: 'var(--color-foreground)',
  muted: 'var(--color-muted-foreground)',
  faint: 'var(--color-muted)',
  chipBg: 'var(--color-surface-2)',
  divider: 'var(--color-border)',
  link: 'var(--color-primary)',
}

const YES = '#22c55e'

// ── Data ───────────────────────────────────────────────────────────────────────

interface SummaryData {
  sources: ItemSources | null
  quests: ItemQuests | null
  holdings: ItemHolding[]
  // False when no character has a Zeal inventory export: "unknown", not "none".
  holdingsKnown: boolean
}

const EMPTY_SOURCES: ItemSources = { drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] }

function useSummaryData(itemId: number): SummaryData {
  const [data, setData] = useState<SummaryData>({ sources: null, quests: null, holdings: [], holdingsKnown: false })
  useEffect(() => {
    let cancelled = false
    setData({ sources: null, quests: null, holdings: [], holdingsKnown: false })
    const patch = (p: Partial<SummaryData>) => { if (!cancelled) setData((d) => ({ ...d, ...p })) }
    // Tradeskill labels come from the enum catalog; wait for it so the first
    // paint doesn't show "Tradeskill 61".
    Promise.all([getItemSources(itemId), loadEnums().catch(() => null)])
      .then(([s]) => patch({ sources: s }))
      .catch(() => patch({ sources: EMPTY_SOURCES }))
    getItemQuests(itemId)
      .then((q) => patch({ quests: q }))
      .catch(() => patch({ quests: { walkthrough: [], used_in: [] } }))
    findHoldingsForItems([itemId])
      .then((r) => patch({ holdings: r.map.get(itemId) ?? [], holdingsKnown: r.configured }))
      .catch(() => {})
    return () => { cancelled = true }
  }, [itemId])
  return data
}

// ── Helpers ────────────────────────────────────────────────────────────────────

const eqName = (name: string): string => name.replace(/^#/, '').replace(/_/g, ' ').trim()

function npcInZone(npc: ItemSourceNPC, zone: string | null): boolean {
  if (!zone) return false
  return npc.zone_short_name === zone || (npc.zones ?? []).some((z) => z.short_name === zone)
}

// Stable sort that lifts entries in the player's current zone to the top.
function hereFirst<T>(list: T[], isHere: (t: T) => boolean): T[] {
  return list
    .map((t, i) => ({ t, i, h: isHere(t) ? 0 : 1 }))
    .sort((a, b) => a.h - b.h || a.i - b.i)
    .map((x) => x.t)
}

function classAbbrs(mask: number): string {
  const ALL = (1 << 15) - 1
  if (mask === 0 || mask >= ALL) return 'ALL'
  return CLASS_NAMES.filter((_, i) => mask & (1 << i)).join(' ')
}

// Coarse location for the Characters section: "Worn", "Bags", "Bank" …
function locationBucket(location: string): string {
  const info = bagSlotInfo(location) ?? bagContainerInfo(location)
  if (info) {
    if (info.kind === 'General') return 'Bags'
    if (info.kind === 'SharedBank') return 'Shared Bank'
    return 'Bank'
  }
  if (isEquipmentSlot(location)) return 'Worn'
  return location
}

interface OwnershipRow {
  character: string
  bucket: string
  count: number
}

function ownershipRows(holdings: ItemHolding[]): OwnershipRow[] {
  const byKey = new Map<string, OwnershipRow>()
  for (const h of holdings) {
    const character = h.character || 'Shared Bank'
    const bucket = h.character ? locationBucket(h.location) : 'Shared Bank'
    const key = `${character}\u0000${bucket}`
    const row = byKey.get(key) ?? { character, bucket, count: 0 }
    row.count += Math.max(1, h.count)
    byKey.set(key, row)
  }
  return [...byKey.values()]
}

interface TradeskillGroup {
  tradeskill: number
  recipes: ItemTradeskillEntry[]
  min: number
  max: number
}

function uniqueRecipes(entries: ItemTradeskillEntry[]): ItemTradeskillEntry[] {
  const seen = new Set<number>()
  return entries.filter((e) => (seen.has(e.recipe_id) ? false : (seen.add(e.recipe_id), true)))
}

function groupByTradeskill(entries: ItemTradeskillEntry[]): TradeskillGroup[] {
  const groups = new Map<number, TradeskillGroup>()
  for (const e of entries) {
    const g = groups.get(e.tradeskill) ?? { tradeskill: e.tradeskill, recipes: [], min: e.trivial, max: e.trivial }
    g.recipes.push(e)
    g.min = Math.min(g.min, e.trivial)
    g.max = Math.max(g.max, e.trivial)
    groups.set(e.tradeskill, g)
  }
  return [...groups.values()].sort((a, b) => b.recipes.length - a.recipes.length)
}

const trivialRange = (min: number, max: number): string => (min === max ? `T${min}` : `T${min}–${max}`)

// ── Primitives ─────────────────────────────────────────────────────────────────

function Chip({ label, value, color, theme }: { label?: string; value: React.ReactNode; color?: string; theme: ItemInfoTheme }): React.ReactElement {
  return (
    <span
      style={{
        display: 'inline-flex', alignItems: 'baseline', gap: 4,
        backgroundColor: theme.chipBg, borderRadius: 3, padding: '3px 7px',
        fontSize: 12, lineHeight: 1.4,
      }}
    >
      {label && (
        <span style={{ color: theme.faint, fontWeight: 600, fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          {label}
        </span>
      )}
      <span style={{ color: color ?? theme.text, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>{value}</span>
    </span>
  )
}

function Heading({ children, theme }: { children: React.ReactNode; theme: ItemInfoTheme }): React.ReactElement {
  return (
    <p style={{ fontSize: 9, fontWeight: 600, color: theme.faint, textTransform: 'uppercase', letterSpacing: '0.08em', margin: '0 0 4px' }}>
      {children}
    </p>
  )
}

function SubHeading({ children, theme }: { children: React.ReactNode; theme: ItemInfoTheme }): React.ReactElement {
  return <p style={{ fontSize: 10, color: theme.muted, margin: '4px 0 2px' }}>{children}</p>
}

// A yes/no answer line: "✓ Vendor purchased" / "✗ Not sold by any vendor".
function Verdict({ yes, children, theme }: { yes: boolean; children: React.ReactNode; theme: ItemInfoTheme }): React.ReactElement {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: yes ? YES : theme.muted, margin: '0 0 2px' }}>
      {yes ? <Check size={12} strokeWidth={3} /> : <X size={12} strokeWidth={2.5} />}
      <span>{children}</span>
    </div>
  )
}

function LinkText({ onClick, children, theme, title }: { onClick?: () => void; children: React.ReactNode; theme: ItemInfoTheme; title?: string }): React.ReactElement {
  if (!onClick) return <span style={{ color: theme.text }}>{children}</span>
  return (
    <button
      onClick={onClick}
      title={title}
      style={{
        background: 'none', border: 'none', padding: 0, cursor: 'pointer', textAlign: 'left',
        color: theme.link, font: 'inherit', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
      }}
    >
      {children}
    </button>
  )
}

// One list row: name on the left, detail on the right, optional "here" pin.
function Row({ left, right, here, theme }: { left: React.ReactNode; right?: React.ReactNode; here?: boolean; theme: ItemInfoTheme }): React.ReactElement {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8, fontSize: 12, lineHeight: 1.5 }}>
      <span style={{ display: 'flex', alignItems: 'baseline', gap: 4, minWidth: 0, overflow: 'hidden', whiteSpace: 'nowrap' }}>
        {here && (
          <span title="In your current zone" style={{ color: YES, flexShrink: 0, alignSelf: 'center', display: 'flex' }}>
            <MapPin size={10} />
          </span>
        )}
        {left}
      </span>
      {right != null && (
        <span style={{ flexShrink: 0, fontSize: 11, color: theme.muted, fontVariantNumeric: 'tabular-nums' }}>{right}</span>
      )}
    </div>
  )
}

function ToggleLink({ onClick, children, theme }: { onClick: () => void; children: React.ReactNode; theme: ItemInfoTheme }): React.ReactElement {
  return (
    <button
      onClick={onClick}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 2, marginTop: 2,
        background: 'none', border: 'none', padding: 0, cursor: 'pointer',
        color: theme.link, fontSize: 11,
      }}
    >
      {children}
    </button>
  )
}

// Shows every row when there are `limit` or fewer. Past that, it shows the
// first `limit` rows (callers sort current-zone rows first) plus a summary
// line that expands the full list in place.
function CollapsibleList<T>({
  rows,
  limit,
  render,
  summary,
  theme,
}: {
  rows: T[]
  limit: number
  render: (row: T, i: number) => React.ReactNode
  summary: string
  theme: ItemInfoTheme
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  if (rows.length <= limit) return <>{rows.map(render)}</>
  if (open) {
    return (
      <>
        {rows.map(render)}
        <ToggleLink onClick={() => setOpen(false)} theme={theme}>Show less</ToggleLink>
      </>
    )
  }
  const shown = rows.slice(0, limit)
  return (
    <>
      {shown.map(render)}
      <ToggleLink onClick={() => setOpen(true)} theme={theme}>
        {shown.length > 0 ? `${summary} · ${rows.length - shown.length} more` : summary}
        <ChevronRight size={11} />
      </ToggleLink>
    </>
  )
}

// ── Sections ───────────────────────────────────────────────────────────────────

interface SectionProps {
  item: Item
  data: SummaryData
  settings: ItemInfoSettings
  theme: ItemInfoTheme
  zone: string | null
  vis: (key: string) => boolean
  go: (route: string) => void
}

function FactsSection({ item, theme, vis }: SectionProps): React.ReactElement | null {
  const equippable = item.slots > 0
  const scroll = item.scroll_effect > 0
  const container = item.item_class === 1
  const flags: string[] = []
  if (item.magic) flags.push('MAGIC')
  if (isLoreItem(item.lore)) flags.push('LORE')
  if (item.nodrop === 0) flags.push('NO DROP')
  if (item.norent === 0) flags.push('NO RENT')
  const stats = (['str', 'sta', 'agi', 'dex', 'wis', 'int', 'cha'] as const).filter((k) => item[k] !== 0)
  const races = decodeRaces(item.races)

  const chips: React.ReactNode[] = []
  const add = (key: string, node: React.ReactNode) => chips.push(<React.Fragment key={key}>{node}</React.Fragment>)
  if (vis('facts.slot') && equippable) add('slot', <Chip label="Slot" value={slotsLabel(item.slots)} theme={theme} />)
  if (vis('facts.ac') && item.ac > 0) add('ac', <Chip label="AC" value={item.ac} theme={theme} />)
  if (vis('facts.weapon') && item.damage > 0) add('weapon', <Chip label="Dmg/Dly" value={weaponRatioLabel(item.damage, item.delay)} color="#ef4444" theme={theme} />)
  if (vis('facts.hpmana') && item.hp !== 0) add('hp', <Chip label="HP" value={item.hp > 0 ? `+${item.hp}` : item.hp} color={YES} theme={theme} />)
  if (vis('facts.hpmana') && item.mana !== 0) add('mana', <Chip label="Mana" value={item.mana > 0 ? `+${item.mana}` : item.mana} color="#3b82f6" theme={theme} />)
  if (vis('facts.stats')) for (const k of stats) add(k, <Chip label={k.toUpperCase()} value={item[k] > 0 ? `+${item[k]}` : item[k]} theme={theme} />)
  if (vis('facts.resists')) {
    if (item.mr) add('mr', <ResistChip type="magic" value={item.mr} />)
    if (item.cr) add('cr', <ResistChip type="cold" value={item.cr} />)
    if (item.fr) add('fr', <ResistChip type="fire" value={item.fr} />)
    if (item.dr) add('dr', <ResistChip type="disease" value={item.dr} />)
    if (item.pr) add('pr', <ResistChip type="poison" value={item.pr} />)
  }
  if (vis('facts.level') && item.req_level > 0) add('req', <Chip label="Req Lv" value={item.req_level} theme={theme} />)
  if (vis('facts.level') && item.rec_level > 0) add('rec', <Chip label="Rec Lv" value={item.rec_level} theme={theme} />)
  if (vis('facts.stack') && item.stackable > 0 && item.stack_size > 1) add('stack', <Chip label="Stack" value={item.stack_size} theme={theme} />)
  if (vis('facts.bag') && container) {
    add('bag', <Chip label="Bag" value={`${item.bag_slots} slots`} theme={theme} />)
    add('bagsize', <Chip label="Fits" value={sizeLabel(item.bag_size)} theme={theme} />)
  }
  if (vis('facts.weight')) add('wt', <Chip label="Wt" value={weightLabel(item.weight)} theme={theme} />)
  if (vis('facts.size') && !equippable) add('size', <Chip label="Size" value={sizeLabel(item.size)} theme={theme} />)
  if (vis('facts.value') && item.price > 0) add('value', <Chip label="Value" value={priceLabel(item.price)} theme={theme} />)

  const showType = vis('facts.type')
  const showFlags = vis('facts.flags') && flags.length > 0
  const showClasses = vis('facts.classes') && (equippable || scroll)
  const showRaces = vis('facts.races') && equippable && races[0] !== 'All'
  if (!showType && !showFlags && chips.length === 0 && !showClasses && !showRaces) return null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {(showType || showFlags) && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
          {showType && <span style={{ fontSize: 12, color: theme.muted }}>{effectiveItemTypeLabel(item.item_class, item.item_type)}</span>}
          {showFlags && flags.map((f) => (
            <span key={f} style={{ fontSize: 9, fontWeight: 700, letterSpacing: '0.05em', color: theme.link, border: `1px solid ${theme.divider}`, borderRadius: 3, padding: '1px 4px' }}>
              {f}
            </span>
          ))}
        </div>
      )}
      {chips.length > 0 && <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{chips}</div>}
      {showClasses && (
        <p style={{ fontSize: 11, color: theme.muted, margin: 0 }}>
          <span style={{ color: theme.faint }}>Classes </span>{classAbbrs(item.classes)}
        </p>
      )}
      {showRaces && (
        <p style={{ fontSize: 11, color: theme.muted, margin: 0 }}>
          <span style={{ color: theme.faint }}>Races </span>{races.join(', ')}
        </p>
      )}
    </div>
  )
}

function EffectsSection({ item, theme, vis, go }: SectionProps): React.ReactElement | null {
  const rows: Array<{ key: string; label: string; id: number; name: string; detail?: string; effectsOnly?: boolean }> = []
  if (vis('effects.scroll') && item.scroll_effect > 0 && item.scroll_name) rows.push({ key: 'scroll', label: 'Teaches', id: item.scroll_effect, name: item.scroll_name })
  if (vis('effects.click') && item.click_effect > 0 && item.click_name) {
    rows.push({ key: 'click', label: 'Click', id: item.click_effect, name: item.click_name, detail: item.max_charges > 0 ? `${item.max_charges} chg` : undefined })
  }
  if (vis('effects.worn') && item.worn_effect > 0 && item.worn_name) {
    rows.push({ key: 'worn', label: 'Worn', id: item.worn_effect, name: item.worn_name, effectsOnly: true, detail: item.worn_haste_pct ? `+${item.worn_haste_pct}% haste` : undefined })
  }
  if (vis('effects.focus') && item.focus_effect > 0 && item.focus_name) rows.push({ key: 'focus', label: 'Focus', id: item.focus_effect, name: item.focus_name, effectsOnly: true })
  if (vis('effects.proc') && item.proc_effect > 0 && item.proc_name) rows.push({ key: 'proc', label: 'Proc', id: item.proc_effect, name: item.proc_name })
  if (rows.length === 0) return null
  return (
    <div>
      <Heading theme={theme}>Effects</Heading>
      {rows.map((r) => (
        <Row
          key={r.key}
          theme={theme}
          left={
            <>
              <span style={{ color: theme.faint, fontSize: 11, flexShrink: 0, width: 46 }}>{r.label}</span>
              <SpellHoverCard spellId={r.id} effectsOnly={r.effectsOnly}>
                <button
                  onClick={() => go(`/spells?select=${r.id}`)}
                  style={{ background: 'none', border: 'none', padding: 0, cursor: 'pointer', color: theme.link, font: 'inherit', overflow: 'hidden', textOverflow: 'ellipsis' }}
                >
                  {r.name}
                </button>
              </SpellHoverCard>
            </>
          }
          right={r.detail}
        />
      ))}
    </div>
  )
}

// Wraps a listed section: heading, then either its body or — when it has no
// data — an explicit "no" line or nothing, per the section's empty mode.
function ListedSection({
  id,
  empty,
  emptyText,
  settings,
  theme,
  loading,
  children,
}: {
  id: ItemInfoSectionId
  empty: boolean
  emptyText: string
  settings: ItemInfoSettings
  theme: ItemInfoTheme
  loading: boolean
  children: React.ReactNode
}): React.ReactElement | null {
  if (loading) return null
  if (empty && settings.emptyMode(id) === 'hide') return null
  return (
    <div>
      <Heading theme={theme}>{sectionDef(id).label}</Heading>
      {empty ? <Verdict yes={false} theme={theme}>{emptyText}</Verdict> : children}
    </div>
  )
}

function OwnershipSection(p: SectionProps): React.ReactElement | null {
  const { data, settings, theme } = p
  const rows = ownershipRows(data.holdings)
  const chars = new Set(rows.map((r) => r.character)).size
  return (
    <ListedSection
      id="ownership"
      empty={rows.length === 0}
      emptyText={data.holdingsKnown ? 'None of your characters have it' : 'No Zeal inventory exports yet'}
      settings={settings}
      theme={theme}
      loading={false}
    >
      <CollapsibleList
        rows={rows}
        limit={settings.limitFor('ownership')}
        summary={`On ${chars} character${chars === 1 ? '' : 's'}`}
        theme={theme}
        render={(r, i) => (
          <Row key={i} theme={theme} left={<span style={{ color: theme.text }}>{r.character}</span>} right={`${r.count} · ${r.bucket}`} />
        )}
      />
    </ListedSection>
  )
}

function TradeskillsSection(p: SectionProps): React.ReactElement | null {
  const { data, settings, theme, vis, go } = p
  const all = data.sources?.tradeskills ?? []
  const produced = vis('tradeskills.produced') ? uniqueRecipes(all.filter((e) => e.role === 'product')) : []
  const used = vis('tradeskills.used') ? uniqueRecipes(all.filter((e) => e.role === 'ingredient')) : []
  const limit = settings.limitFor('tradeskills')

  const recipeRow = (e: ItemTradeskillEntry) => (
    <Row
      key={e.recipe_id}
      theme={theme}
      left={<LinkText theme={theme} onClick={() => go(`/recipes?select=${e.recipe_id}`)}>{e.recipe_name}</LinkText>}
      right={`${tradeskillLabel(e.tradeskill)} · T${e.trivial}`}
    />
  )
  const block = (label: string, recipes: ItemTradeskillEntry[]) => {
    if (recipes.length === 0) return null
    if (recipes.length <= limit) {
      return (
        <div>
          <SubHeading theme={theme}>{label}</SubHeading>
          {recipes.map(recipeRow)}
        </div>
      )
    }
    return (
      <div>
        <SubHeading theme={theme}>{label}</SubHeading>
        {groupByTradeskill(recipes).map((g) => (
          <Row
            key={g.tradeskill}
            theme={theme}
            left={<span style={{ color: theme.text }}>{tradeskillLabel(g.tradeskill)}</span>}
            right={`${g.recipes.length} recipe${g.recipes.length === 1 ? '' : 's'} · ${trivialRange(g.min, g.max)}`}
          />
        ))}
        <CollapsibleList rows={recipes} limit={0} summary={`Show all ${recipes.length} recipes`} theme={theme} render={recipeRow} />
      </div>
    )
  }
  return (
    <ListedSection
      id="tradeskills"
      empty={produced.length === 0 && used.length === 0}
      emptyText="Not used in any recipe"
      settings={settings}
      theme={theme}
      loading={data.sources === null}
    >
      {block('Produced by', produced)}
      {block('Used in', used)}
    </ListedSection>
  )
}

function QuestsSection(p: SectionProps): React.ReactElement | null {
  const { data, settings, theme, zone, vis, go } = p
  const reward = vis('quests.reward') ? data.quests?.walkthrough ?? [] : []
  const used = vis('quests.used') ? data.quests?.used_in ?? [] : []
  const limit = settings.limitFor('quests')
  const isHere = (q: { zone_short_name: string }) => !!zone && q.zone_short_name === zone
  const row = (q: { npc: string; zone_short_name: string; zone_name: string }, i: number) => (
    <Row
      key={i}
      theme={theme}
      here={isHere(q)}
      left={<span style={{ color: theme.text, overflow: 'hidden', textOverflow: 'ellipsis' }}>{eqName(q.npc)}</span>}
      right={<LinkText theme={theme} onClick={() => go(`/zones?select=${q.zone_short_name}`)}>{q.zone_name || q.zone_short_name}</LinkText>}
    />
  )
  const block = (label: string, rows: Array<{ npc: string; zone_short_name: string; zone_name: string }>, noun: string) => {
    if (rows.length === 0) return null
    const zones = new Set(rows.map((r) => r.zone_short_name)).size
    return (
      <div>
        <SubHeading theme={theme}>{label}</SubHeading>
        <CollapsibleList
          rows={hereFirst(rows, isHere)}
          limit={limit}
          summary={`${rows.length} ${noun} in ${zones} zone${zones === 1 ? '' : 's'}`}
          theme={theme}
          render={row}
        />
      </div>
    )
  }
  return (
    <ListedSection
      id="quests"
      empty={reward.length === 0 && used.length === 0}
      emptyText="No known quest uses"
      settings={settings}
      theme={theme}
      loading={data.quests === null}
    >
      {block('Rewarded by', reward, 'quests')}
      {block('Turned in to', used, 'turn-ins')}
    </ListedSection>
  )
}

function NPCList({
  npcs,
  limit,
  noun,
  summary,
  right,
  p,
}: {
  npcs: ItemSourceNPC[]
  limit: number
  noun: string
  summary?: string
  right: (n: ItemSourceNPC) => React.ReactNode
  p: SectionProps
}): React.ReactElement {
  const { theme, zone, go } = p
  const isHere = (n: ItemSourceNPC) => npcInZone(n, zone)
  return (
    <CollapsibleList
      rows={hereFirst(npcs, isHere)}
      limit={limit}
      summary={summary ?? `${npcs.length} ${noun}`}
      theme={theme}
      render={(n) => (
        <Row
          key={n.id}
          theme={theme}
          here={isHere(n)}
          left={<LinkText theme={theme} onClick={() => go(`/npcs?select=${n.id}`)}>{eqName(n.name)}</LinkText>}
          right={right(n)}
        />
      )}
    />
  )
}

function VendorsSection(p: SectionProps): React.ReactElement | null {
  const { item, data, settings, theme, vis } = p
  const merchants = data.sources?.merchants ?? []
  return (
    <ListedSection
      id="vendors"
      empty={merchants.length === 0}
      emptyText="Not sold by any vendor"
      settings={settings}
      theme={theme}
      loading={data.sources === null}
    >
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
        <Verdict yes theme={theme}>Vendor purchased</Verdict>
        {vis('vendors.price') && item.price > 0 && (
          <span style={{ fontSize: 11, color: theme.muted }} title="Base price; vendors adjust it for charisma and faction">
            ~{priceLabel(item.price)}
          </span>
        )}
      </div>
      <NPCList
        npcs={merchants}
        limit={settings.limitFor('vendors')}
        noun="vendors"
        summary={`Sold by ${merchants.length} vendors`}
        right={(n) => n.zone_name || n.zone_short_name}
        p={p}
      />
    </ListedSection>
  )
}

function DropsSection(p: SectionProps): React.ReactElement | null {
  const { data, settings, theme, vis } = p
  const drops = [...(data.sources?.drops ?? [])].sort((a, b) => (b.drop_rate ?? 0) - (a.drop_rate ?? 0))
  const showRate = vis('drops.rate')
  return (
    <ListedSection
      id="drops"
      empty={drops.length === 0}
      emptyText="Not dropped by any NPC"
      settings={settings}
      theme={theme}
      loading={data.sources === null}
    >
      <NPCList
        npcs={drops}
        limit={settings.limitFor('drops')}
        noun="NPCs drop it"
        right={(n) => {
          const z = n.zone_name || n.zone_short_name
          return showRate && n.drop_rate ? `${z} · ${n.drop_rate.toFixed(1)}%` : z
        }}
        p={p}
      />
    </ListedSection>
  )
}

function SourcesSection(p: SectionProps): React.ReactElement | null {
  const { data, settings, theme, zone, vis, go } = p
  type Src = { key: string; zone: string; name: string; detail: string }
  const rows: Src[] = []
  if (vis('sources.forage')) {
    for (const f of data.sources?.forage_zones ?? []) {
      rows.push({ key: `f${f.zone_short_name}`, zone: f.zone_short_name, name: f.zone_name || f.zone_short_name, detail: f.chance > 0 ? `Forage · ${f.chance}%` : 'Forage' })
    }
  }
  if (vis('sources.ground')) {
    data.sources?.ground_spawns?.forEach((g, i) => {
      rows.push({ key: `g${i}`, zone: g.zone_short_name, name: g.zone_name || g.zone_short_name, detail: 'Ground spawn' })
    })
  }
  const isHere = (r: Src) => !!zone && r.zone === zone
  return (
    <ListedSection
      id="sources"
      empty={rows.length === 0}
      emptyText="Not foraged or found on the ground"
      settings={settings}
      theme={theme}
      loading={data.sources === null}
    >
      <CollapsibleList
        rows={hereFirst(rows, isHere)}
        limit={settings.limitFor('sources')}
        summary={`${rows.length} zones`}
        theme={theme}
        render={(r) => (
          <Row
            key={r.key}
            theme={theme}
            here={isHere(r)}
            left={<LinkText theme={theme} onClick={() => go(`/zones?select=${r.zone}`)}>{r.name}</LinkText>}
            right={r.detail}
          />
        )}
      />
    </ListedSection>
  )
}

const SECTION_COMPONENTS: Record<ItemInfoSectionId, (p: SectionProps) => React.ReactElement | null> = {
  facts: FactsSection,
  effects: EffectsSection,
  ownership: OwnershipSection,
  tradeskills: TradeskillsSection,
  quests: QuestsSection,
  vendors: VendorsSection,
  drops: DropsSection,
  sources: SourcesSection,
}

// ── Card ───────────────────────────────────────────────────────────────────────

export default function ItemInfoCard({
  item,
  surface,
  theme,
}: {
  item: Item
  surface: ItemInfoSurface
  theme: ItemInfoTheme
}): React.ReactElement {
  const settings = useItemInfoSettings(surface)
  const data = useSummaryData(item.id)
  const { zone } = useLiveZone()
  const navigate = useAppNavigate()
  const vis = useMemo(() => (key: string) => !settings.hidden.has(key), [settings.hidden])
  const openItem = () => navigate(`/items?select=${item.id}`)
  const showOpen = vis('header.open')

  const total = data.holdings.reduce((n, h) => n + Math.max(1, h.count), 0)
  const props: SectionProps = { item, data, settings, theme, zone, vis, go: navigate }

  const sections = settings.order
    .filter((id) => vis(sectionKey(id)))
    .map((id) => {
      const Section = SECTION_COMPONENTS[id]
      return <Section key={id} {...props} />
    })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0, color: theme.text }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, padding: '8px 10px', borderBottom: `1px solid ${theme.divider}`, flexShrink: 0 }}>
        <ItemIcon id={item.icon} name={item.name} size={28} />
        <div style={{ minWidth: 0, flex: 1 }}>
          <LinkText theme={theme} onClick={showOpen ? openItem : undefined} title="Open in PQ Companion">
            <span style={{ fontSize: 14, fontWeight: 700, color: theme.link }}>{item.name}</span>
          </LinkText>
          {vis('header.badge') && data.holdingsKnown && (
            total > 0 ? (
              <Verdict yes theme={theme}>You have this{total > 1 ? ` (${total})` : ''}</Verdict>
            ) : (
              <Verdict yes={false} theme={theme}>Not on any character</Verdict>
            )
          )}
        </div>
        {vis('header.wishlist') && (
          <span className="no-drag" style={{ flexShrink: 0, marginTop: 2 }}>
            <WishlistStarButton item={item} size={14} />
          </span>
        )}
      </div>

      {/* Sections */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '8px 10px', display: 'flex', flexDirection: 'column', gap: 10 }}>
        {sections}
      </div>

    </div>
  )
}

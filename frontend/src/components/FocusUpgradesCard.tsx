import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { X, RefreshCw, PackageCheck } from 'lucide-react'
import {
  getAllInventories,
  getCharacterFocusUpgrades,
  type FocusUpgradeItem,
  type FocusUpgradesResponse,
} from '../services/api'
import { ItemIcon } from './Icon'
import { decodeSlots } from '../lib/itemHelpers'
import { usePersistentState } from '../hooks/usePersistentState'

interface FocusUpgradesCardProps {
  characterID: number
  category: string
  onClose: () => void
}

// fmtPct renders a focus magnitude with its sign: cast-time (127) and mana-cost
// (132) foci are reductions (minus), a negative value (aggro reduction) already
// carries its own sign, and everything else is a bonus (plus).
export function fmtPct(spa: number, percent: number): string {
  if (percent < 0) return `−${-percent}%`
  return `${spa === 127 || spa === 132 ? '−' : '+'}${percent}%`
}

// scopeTags turns a focus's limits into short labels for what it applies to.
function scopeTags(l: FocusUpgradeItem['limits']): string[] {
  const tags: string[] = []
  if (l.spell_type === 1) tags.push('beneficial only')
  else if (l.spell_type === 0) tags.push('detrimental only')
  if (l.instant_only) tags.push('direct damage only')
  if (l.min_duration_sec) tags.push(`spells ≥ ${l.min_duration_sec}s`)
  if (l.min_cast_time_ms) tags.push(`≥ ${l.min_cast_time_ms / 1000}s cast`)
  return tags
}

// slotTag names the slots an item equips to, collapsing "go anywhere" items.
function slotTag(mask: number): string | null {
  const names = decodeSlots(mask)
  if (names.length === 0) return null
  if (names.length > 3) return `${names.slice(0, 2).join(', ')} +${names.length - 2} slots`
  return names.join(', ')
}

// Hate foci where a bigger number isn't plainly better — mirrors the backend's
// "comparable" check, so ranks aren't filtered for them.
const UNRANKED_CATEGORIES = new Set(['aggro_reduction', 'bash_hate'])

// ownedByItem maps item id → names of characters carrying it (bags, bank,
// shared bank), so an upgrade the player already has on an alt is flagged.
async function loadOwners(): Promise<Map<number, string[]>> {
  const owners = new Map<number, string[]>()
  try {
    const res = await getAllInventories()
    const add = (id: number, who: string): void => {
      const list = owners.get(id) ?? []
      if (!list.includes(who)) list.push(who)
      owners.set(id, list)
    }
    for (const inv of res.characters ?? []) {
      if (inv) for (const e of inv.entries) add(e.id, inv.character)
    }
    for (const e of res.shared_bank ?? []) add(e.id, 'Shared Bank')
  } catch {
    /* ownership hints are optional */
  }
  return owners
}

/**
 * Everything a character's class and level can wear for one spell-focus
 * category (e.g. Spell Haste), best first, alongside what they wear now. Only
 * the best focus in a category applies in game, so the list highlights items that
 * beat the current one and flags ones already owned on any character.
 */
export default function FocusUpgradesCard({
  characterID,
  category,
  onClose,
}: FocusUpgradesCardProps): React.ReactElement {
  const navigate = useNavigate()
  const [data, setData] = useState<FocusUpgradesResponse | null>(null)
  const [owners, setOwners] = useState<Map<number, string[]>>(new Map())
  const [error, setError] = useState<string | null>(null)
  const [lowerRanks, setLowerRanks] = usePersistentState<'on' | 'off'>(
    'pq-focus:include-lower-ranks', 'off', ['on', 'off'],
  )

  useEffect(() => {
    let cancelled = false
    setData(null)
    setError(null)
    getCharacterFocusUpgrades(characterID, category)
      .then((d) => { if (!cancelled) setData(d) })
      .catch((e: Error) => { if (!cancelled) setError(e.message) })
    loadOwners().then((o) => { if (!cancelled) setOwners(o) })
    return () => { cancelled = true }
  }, [characterID, category])

  const border = '1px solid var(--color-border)'

  // With a focus worn, hide lower ranks by default: keep what's worn plus
  // anything at least as good that still covers the top spells — the
  // same-or-better options to move into another slot.
  const canFilter = !!data && data.current.length > 0 && !UNRANKED_CATEGORIES.has(data.category)
  const shown = !data
    ? []
    : canFilter && lowerRanks === 'off'
      ? data.items.filter((it) =>
          it.equipped || (it.covers_top && it.percent >= data.current_percent))
      : data.items
  const hiddenCount = data ? data.items.length - shown.length : 0

  return (
    <div
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-primary)' }}
    >
      <div className="mb-2 flex items-start justify-between gap-2">
        <div>
          <p className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>
            {data?.label ?? 'Focus'} — {data && data.current.length > 0
              ? 'items with this focus'
              : 'items you can use'}
          </p>
          {data && (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{data.blurb}</p>
          )}
        </div>
        <button onClick={onClose} title="Close" style={{ color: 'var(--color-muted)' }}>
          <X size={14} />
        </button>
      </div>

      {!data && !error && (
        <p className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <RefreshCw size={11} className="animate-spin" /> Loading…
        </p>
      )}
      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}

      {data && (
        <>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-foreground)' }}>
            {data.current.length === 0 ? (
              'You have nothing worn for this focus.'
            ) : (
              <>
                You wear:{' '}
                {data.current.map((m, i) => (
                  <span key={i} className="font-mono" style={{ color: 'var(--color-primary)' }}>
                    {i > 0 ? ', ' : ''}
                    {m.source === 'item' ? m.source_item_name : m.source_aa_name}
                    {m.source === 'item' && m.source_item_slot ? ` [${m.source_item_slot}]` : ''}
                    {' '}({fmtPct(data.spa, m.percent)})
                    {!m.covers_top && m.limits.max_level ? (
                      <span
                        style={{ color: '#f59e0b' }}
                        title={`Only affects spells up to level ${m.limits.max_level} — nothing above that`}
                      >
                        {' '}⚠ ≤ L{m.limits.max_level}
                      </span>
                    ) : null}
                  </span>
                ))}
                . Only the best focus that applies to a spell counts.
              </>
            )}
          </p>

          {data.rolls && (
            <p className="mb-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              Percentages are the maximum: each cast rolls a random 1% to that amount.
            </p>
          )}

          {canFilter && (
            <label
              className="mb-2 flex w-fit cursor-pointer items-center gap-1.5 text-xs"
              style={{ color: 'var(--color-muted-foreground)' }}
              title="Also list weaker versions of this focus (e.g. a rank III when you wear rank IV)"
            >
              <input
                type="checkbox"
                checked={lowerRanks === 'on'}
                onChange={(e) => setLowerRanks(e.target.checked ? 'on' : 'off')}
              />
              Include lower ranks
              {lowerRanks === 'off' && hiddenCount > 0 && (
                <span style={{ color: 'var(--color-muted)' }}>({hiddenCount} hidden)</span>
              )}
            </label>
          )}

          {data.items.length === 0 ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No items found for your class and level.
            </p>
          ) : (
            <div className="space-y-1">
              {shown.map((it) => {
                const who = owners.get(it.item_id)
                const slots = slotTag(it.slots)
                return (
                  <div
                    key={`${it.item_id}-${it.focus_spell_id}`}
                    className="flex items-center gap-2 rounded px-2 py-1.5 text-xs"
                    style={{
                      backgroundColor: it.equipped ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
                      border,
                    }}
                  >
                    <ItemIcon id={it.icon} name={it.name} size={22} />
                    <div className="min-w-0 flex-1">
                      <button
                        onClick={() => navigate(`/items?select=${it.item_id}`)}
                        className="truncate font-medium underline decoration-dotted"
                        style={{ color: 'var(--color-primary)' }}
                      >
                        {it.name}
                      </button>
                      <span style={{ color: 'var(--color-muted-foreground)' }}> · {it.focus_name}</span>
                      <div className="mt-0.5 flex flex-wrap gap-1">
                        {it.equipped && <Tag label="worn" accent />}
                        {it.is_upgrade && <Tag label="upgrade" success />}
                        {slots && <Tag label={slots} slot />}
                        {it.req_level > 0 && <Tag label={`req L${it.req_level}`} />}
                        {it.limits.max_level ? (
                          <Tag
                            label={`spells ≤ L${it.limits.max_level}`}
                            warn={!it.covers_top}
                            title={it.covers_top
                              ? undefined
                              : `Won't affect spells above level ${it.limits.max_level}`}
                          />
                        ) : null}
                        {scopeTags(it.limits).map((t) => <Tag key={t} label={t} />)}
                        {it.no_drop && <Tag label="no drop" />}
                        {who && (
                          <span
                            className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px]"
                            style={{ backgroundColor: 'var(--color-surface-3)', color: '#f59e0b' }}
                            title={`Owned by ${who.join(', ')}`}
                          >
                            <PackageCheck size={10} /> {who.length > 2 ? `${who.slice(0, 2).join(', ')} +${who.length - 2}` : who.join(', ')}
                          </span>
                        )}
                      </div>
                    </div>
                    <span className="shrink-0 font-mono font-semibold" style={{ color: 'var(--color-primary)' }}>
                      {fmtPct(data.spa, it.percent)}
                    </span>
                  </div>
                )
              })}
            </div>
          )}
        </>
      )}
    </div>
  )
}

function Tag({ label, accent, success, warn, slot, title }: {
  label: string
  accent?: boolean
  success?: boolean
  warn?: boolean
  slot?: boolean
  title?: string
}): React.ReactElement {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px]"
      title={title}
      style={{
        backgroundColor: 'var(--color-surface-3)',
        border: slot ? '1px solid var(--color-border)' : undefined,
        color: warn
          ? '#f59e0b'
          : success ? 'var(--color-success)' : accent ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
      }}
    >
      {label}
    </span>
  )
}

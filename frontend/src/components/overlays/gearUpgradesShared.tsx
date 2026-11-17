import React, { useCallback, useEffect, useMemo, useState } from 'react'
import ItemHoverCard from '../ItemHoverCard'
import { useWebSocket, type WsMessage } from '../../hooks/useWebSocket'
import { useLiveZone } from '../../hooks/useLiveZone'
import { useActivePlayerName } from '../../hooks/useActivePlayerName'
import { WSEvent } from '../../lib/wsEvents'
import {
  getCharacterZoneUpgrades,
  getRespawnState,
  getOverlayNPCTarget,
  listCharacters,
  type ZoneUpgradeCandidate,
  type ZoneUpgradesResponse,
} from '../../services/api'
import type { RespawnState } from '../../types/respawn'
import type { TargetState } from '../../types/overlay'
import { normalizeTargetName } from '../../lib/targetName'

// The Gear Upgrades overlay: the Gear Upgrade Finder's "By zone" view cut
// down to a glance. The active character's best upgrades that drop in the
// zone they're in, by slot, each with the NPCs here that drop it. Scored
// with the character's saved finder weights.

// How many upgrades each slot lists, and NPCs each upgrade names. The
// finder page has the full lists.
const PER_SLOT = 3
const SOURCES_SHOWN = 3
// How many upgrades per slot to fetch: the backend's cap, so every upgrade
// in the zone is on hand for the target filter, not just the zone view's
// top few. The backend scores every candidate whatever the limit and only
// trims the reply, so this costs no extra scoring.
const FETCH_PER_SLOT = 200

export interface GearUpgradeSlotRow {
  label: string
  candidates: ZoneUpgradeCandidate[]
}

export interface ZoneGearUpgrades {
  zone: string | null
  zoneName: string
  character: string
  // No character set up with a class and level for the finder.
  noCharacter: boolean
  data: ZoneUpgradesResponse | null
  // The rows to show: only the current target's upgrades while an NPC that
  // drops any is targeted, else the zone's best per slot.
  rows: GearUpgradeSlotRow[]
  // The zone's rows, whatever is targeted.
  zoneRows: GearUpgradeSlotRow[]
  // The targeted NPC's name while rows is narrowed to its drops, else null.
  targetName: string | null
  loading: boolean
  error: string | null
  reload: () => void
}

// The zone from Zeal when it's live, else the last zone the log showed.
function useCurrentZone(): string | null {
  const { zone: liveZone } = useLiveZone()
  const [logZone, setLogZone] = useState<string | null>(null)
  useEffect(() => {
    getRespawnState()
      .then((st) => { if (st.current_zone) setLogZone(st.current_zone) })
      .catch(() => {})
  }, [])
  const onMessage = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.OverlayRespawns) return
    const z = (msg.data as RespawnState | null)?.current_zone
    if (z) setLogZone(z)
  }, [])
  useWebSocket(onMessage)
  return liveZone ?? logZone
}

// The targeted NPC: its npc_types ids (every variant when the name is
// ambiguous) and its name, for matching upgrade sources. null with no NPC
// targeted.
interface TargetNPC {
  ids: Set<number>
  name: string
  key: string
}

function targetFromState(s: TargetState | null | undefined): TargetNPC | null {
  if (!s?.has_target || !s.target_name) return null
  // Players and player corpses have no loot.
  if (s.target_type === 0 || s.target_type === 3) return null
  const ids = new Set<number>()
  if (s.npc_data?.id) ids.add(s.npc_data.id)
  for (const v of s.variants ?? []) if (v.npc?.id) ids.add(v.npc.id)
  const name = s.target_name.replace(/'s corpse$/i, '').replace(/_/g, ' ')
  return { ids, name, key: normalizeTargetName(name) }
}

function useTargetNPC(): TargetNPC | null {
  const [target, setTarget] = useState<TargetNPC | null>(null)
  const apply = useCallback((s: TargetState | null | undefined) => {
    const next = targetFromState(s)
    // The NPC feed broadcasts on every HP tick; keep the same object unless
    // the target changed.
    setTarget((prev) => {
      if (!prev || !next) return prev === next ? prev : next
      const same = prev.key === next.key && prev.ids.size === next.ids.size &&
        [...next.ids].every((id) => prev.ids.has(id))
      return same ? prev : next
    })
  }, [])
  useEffect(() => {
    getOverlayNPCTarget().then(apply).catch(() => {})
  }, [apply])
  useWebSocket(useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.OverlayNPCTarget) apply(msg.data as TargetState | null)
  }, [apply]))
  return target
}

// dropsFrom reports whether the targeted NPC is one of an upgrade's sources,
// by npc id, or by name for a source without one.
function dropsFrom(c: ZoneUpgradeCandidate, t: TargetNPC): boolean {
  return c.sources.some((s) =>
    s.npc_id > 0 && t.ids.size > 0
      ? t.ids.has(s.npc_id)
      : normalizeTargetName(s.npc_name.replace(/_/g, ' ')) === t.key,
  )
}

// slotRows folds the paired slots (Ear 1 and 2, Wrist, Finger) into one row,
// since they offer the same items, and drops ammo and empty slots.
// keep narrows the candidates first (the target filter), so a match ranked
// low in its slot still makes the cut; perSlot caps each row (0 = all).
export function slotRows(
  data: ZoneUpgradesResponse | null,
  keep?: (c: ZoneUpgradeCandidate) => boolean,
  perSlot = PER_SLOT,
): GearUpgradeSlotRow[] {
  if (!data) return []
  const rows: GearUpgradeSlotRow[] = []
  const byLabel = new Map<string, GearUpgradeSlotRow>()
  for (const s of data.slots) {
    if (s.slot === 'ammo' || s.candidates.length === 0) continue
    const label = s.slot_label.replace(/ \d$/, '')
    let row = byLabel.get(label)
    if (!row) {
      row = { label, candidates: [] }
      byLabel.set(label, row)
      rows.push(row)
    }
    for (const c of s.candidates) {
      if (keep && !keep(c)) continue
      const i = row.candidates.findIndex((x) => x.id === c.id)
      if (i < 0) row.candidates.push(c)
      else if (c.score > row.candidates[i].score) row.candidates[i] = c
    }
  }
  for (let i = rows.length - 1; i >= 0; i--) {
    if (rows[i].candidates.length === 0) rows.splice(i, 1)
  }
  for (const row of rows) {
    row.candidates.sort((a, b) => b.score - a.score)
    if (perSlot > 0) row.candidates = row.candidates.slice(0, perSlot)
  }
  return rows
}

export function useZoneGearUpgrades(): ZoneGearUpgrades {
  const zone = useCurrentZone()
  const character = useActivePlayerName()
  const [charId, setCharId] = useState<number | null>(null)
  const [noCharacter, setNoCharacter] = useState(false)
  const [data, setData] = useState<ZoneUpgradesResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    if (!character) return
    let live = true
    listCharacters()
      .then((r) => {
        if (!live) return
        const c = r.characters.find((x) => x.name.toLowerCase() === character.toLowerCase())
        const usable = !!c && c.class >= 0 && c.level > 0
        setCharId(usable ? c.id : null)
        setNoCharacter(!usable)
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [character, tick])

  useEffect(() => {
    if (!zone || charId === null) {
      setData(null)
      return
    }
    let live = true
    setLoading(true)
    setError(null)
    getCharacterZoneUpgrades(charId, zone, { limit: FETCH_PER_SLOT })
      .then((r) => { if (live) setData(r) })
      .catch((e: Error) => {
        if (!live) return
        setData(null)
        setError(e.message)
      })
      .finally(() => { if (live) setLoading(false) })
    return () => {
      live = false
    }
  }, [zone, charId, tick])

  const reload = useCallback(() => setTick((t) => t + 1), [])
  const target = useTargetNPC()
  const zoneRows = useMemo(() => slotRows(data), [data])
  const targetRows = useMemo(
    // Every upgrade the target drops, however many share a slot.
    () => (target ? slotRows(data, (c) => dropsFrom(c, target), 0) : []),
    [data, target],
  )
  const narrowed = targetRows.length > 0
  return {
    zone,
    zoneName: data?.zone_name || zone || '',
    character,
    noCharacter,
    data,
    rows: narrowed ? targetRows : zoneRows,
    zoneRows,
    targetName: narrowed && target ? target.name : null,
    loading,
    error,
    reload,
  }
}

// "4 slots · best: Hammer of Hours (Primary)".
export function gearUpgradesSummary(rows: GearUpgradeSlotRow[]): string | null {
  if (rows.length === 0) return null
  let best: { name: string; slot: string; score: number } | null = null
  for (const r of rows) {
    const c = r.candidates[0]
    if (c && (!best || c.score > best.score)) best = { name: c.name, slot: r.label, score: c.score }
  }
  const n = `${rows.length} slot${rows.length === 1 ? '' : 's'}`
  return best ? `${n} · best: ${best.name} (${best.slot})` : n
}

// The colours for each surface: the in-app panel uses the theme, the popout
// draws light text on its translucent dark background.
const THEME = {
  panel: {
    label: 'var(--color-muted)',
    item: 'var(--color-primary)',
    score: '#22c55e',
    npc: 'var(--color-muted-foreground)',
    npcLink: 'var(--color-foreground)',
    rule: 'var(--color-border)',
  },
  window: {
    label: 'rgba(255,255,255,0.4)',
    item: '#c9a84c',
    score: '#4ade80',
    npc: 'rgba(255,255,255,0.45)',
    npcLink: 'rgba(255,255,255,0.75)',
    rule: 'rgba(255,255,255,0.08)',
  },
}

const linkButton: React.CSSProperties = {
  background: 'none',
  border: 'none',
  padding: 0,
  font: 'inherit',
  cursor: 'pointer',
  textAlign: 'left',
}

export function GearUpgradeList({
  rows,
  variant,
  onItem,
  onNpc,
}: {
  rows: GearUpgradeSlotRow[]
  variant: 'panel' | 'window'
  onItem: (id: number) => void
  onNpc: (id: number) => void
}): React.ReactElement {
  const t = THEME[variant]
  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      {rows.map((row) => (
        <div key={row.label} style={{ padding: '5px 10px 6px', borderBottom: `1px solid ${t.rule}` }}>
          <div
            style={{
              fontSize: 9, fontWeight: 700, letterSpacing: '0.06em', textTransform: 'uppercase',
              color: t.label, marginBottom: 2,
            }}
          >
            {row.label}
          </div>
          {row.candidates.map((c) => (
            <div key={c.id} style={{ display: 'flex', flexDirection: 'column', padding: '2px 0' }}>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, minWidth: 0 }}>
                {/* The hover card fires on the name text only: the button
                    shrinks to fit, the empty row space around it is inert. */}
                <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                  <ItemHoverCard itemId={c.id} clickHint="Click to open in PQ Companion">
                    <button
                      type="button"
                      onClick={() => onItem(c.id)}
                      style={{
                        ...linkButton, display: 'inline-block', maxWidth: '100%', verticalAlign: 'bottom',
                        fontSize: 12, fontWeight: 600, color: t.item,
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}
                    >
                      {c.name}
                    </button>
                  </ItemHoverCard>
                </div>
                <span
                  style={{ fontSize: 10, fontWeight: 700, color: t.score, flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}
                  title="Upgrade score with your Gear Upgrade Finder weights"
                >
                  +{Math.round(c.score)}
                </span>
              </div>
              <div style={{ fontSize: 10, color: t.npc, lineHeight: 1.35 }}>
                {c.sources.slice(0, SOURCES_SHOWN).map((s, i) => (
                  <React.Fragment key={`${s.npc_id}-${s.npc_name}`}>
                    {i > 0 && ', '}
                    {s.npc_id > 0 ? (
                      <button
                        type="button"
                        onClick={() => onNpc(s.npc_id)}
                        style={{ ...linkButton, color: t.npcLink }}
                        title="Open this NPC in PQ Companion"
                      >
                        {s.npc_name.replace(/_/g, ' ')}
                      </button>
                    ) : (
                      <span>{s.npc_name}</span>
                    )}
                    {s.chance > 0 && ` ${fmtChance(s.chance)}`}
                  </React.Fragment>
                ))}
                {c.sources.length > SOURCES_SHOWN && (
                  <span title={c.sources.slice(SOURCES_SHOWN).map((s) => s.npc_name.replace(/_/g, ' ')).join(', ')}>
                    {`, +${c.sources.length - SOURCES_SHOWN} more`}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

function fmtChance(chance: number): string {
  const v = chance < 10 ? chance.toFixed(1).replace(/\.0$/, '') : Math.round(chance).toString()
  return `(${v}%)`
}

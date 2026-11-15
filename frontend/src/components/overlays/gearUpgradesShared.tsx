import React, { useCallback, useEffect, useState } from 'react'
import ItemHoverCard from '../ItemHoverCard'
import { useWebSocket, type WsMessage } from '../../hooks/useWebSocket'
import { useLiveZone } from '../../hooks/useLiveZone'
import { useActivePlayerName } from '../../hooks/useActivePlayerName'
import { WSEvent } from '../../lib/wsEvents'
import {
  getCharacterZoneUpgrades,
  getRespawnState,
  listCharacters,
  type ZoneUpgradeCandidate,
  type ZoneUpgradesResponse,
} from '../../services/api'
import type { RespawnState } from '../../types/respawn'

// The Gear Upgrades overlay: the Gear Upgrade Finder's "By zone" view cut
// down to a glance. The active character's best upgrades that drop in the
// zone they're in, by slot, each with the NPCs here that drop it. Scored
// with the character's saved finder weights.

// How many upgrades each slot lists, and NPCs each upgrade names. The
// finder page has the full lists.
const PER_SLOT = 3
const SOURCES_SHOWN = 3

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
  rows: GearUpgradeSlotRow[]
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

// slotRows folds the paired slots (Ear 1 and 2, Wrist, Finger) into one row,
// since they offer the same items, and drops ammo and empty slots.
export function slotRows(data: ZoneUpgradesResponse | null): GearUpgradeSlotRow[] {
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
      const i = row.candidates.findIndex((x) => x.id === c.id)
      if (i < 0) row.candidates.push(c)
      else if (c.score > row.candidates[i].score) row.candidates[i] = c
    }
  }
  for (const row of rows) {
    row.candidates.sort((a, b) => b.score - a.score)
    row.candidates = row.candidates.slice(0, PER_SLOT)
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
    getCharacterZoneUpgrades(charId, zone, { limit: PER_SLOT * 2 })
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
  return {
    zone,
    zoneName: data?.zone_name || zone || '',
    character,
    noCharacter,
    data,
    rows: slotRows(data),
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
                <ItemHoverCard itemId={c.id} clickHint="Click to open in PQ Companion">
                  <button
                    type="button"
                    onClick={() => onItem(c.id)}
                    style={{
                      ...linkButton, flex: 1, minWidth: 0, fontSize: 12, fontWeight: 600,
                      color: t.item, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                    }}
                  >
                    {c.name}
                  </button>
                </ItemHoverCard>
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

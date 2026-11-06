// Spawn points on the map: aggro rings and respawn dots, shared by the map
// pages and the map overlay.

import React, { useEffect, useMemo, useState } from 'react'
import { getRespawnState, getZoneSpawnPoints } from '../../services/api'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useActiveCharacterLevel } from '../../hooks/useActiveCharacterLevel'
import { WSEvent } from '../../lib/wsEvents'
import { CON_HEX, levelCon } from '../../lib/conColor'
import type { RespawnState } from '../../types/respawn'
import type { ZoneSpawnPoint } from '../../types/map'
import type { AggroRing, SpawnDot } from './ZoneMap'

// Rings drawn when the active character's level is unknown.
const UNKNOWN_CON_HEX = '#94a3b8'

// spawnRing turns a spawn point into its aggro ring, judged by the most
// dangerous NPC it can roll: the widest radius, the highest level, and any
// see-invis.
//
// faint follows the server's aggro check (EQMacEmu Mob::CheckWillAggro): an
// NPC below level 18 that cons green to you won't attack on sight unless it
// is undead or flagged aggro_pc. Sitting overrides that, which the map can't
// know, so these are dimmed rather than hidden.
export function spawnRing(p: ZoneSpawnPoint, playerLevel: number | null): AggroRing {
  let radius = 0
  let top = 0
  let seeInvis = false
  let faint = playerLevel != null
  for (const n of p.npcs) {
    const lvl = Math.max(n.level, n.max_level ?? 0)
    radius = Math.max(radius, n.aggro_radius)
    top = Math.max(top, lvl)
    if (n.see_invis > 0 || n.see_invis_undead > 0) seeInvis = true
    const passive =
      playerLevel != null && lvl < 18 && !n.undead && !n.aggro_pc &&
      levelCon(playerLevel, lvl) === 'green'
    if (!passive) faint = false
  }
  return {
    x: p.x,
    y: p.y,
    z: p.z,
    radius,
    color: playerLevel != null ? CON_HEX[levelCon(playerLevel, top)] : UNKNOWN_CON_HEX,
    faint,
    seeInvis,
  }
}

// Respawn dot colours: on cooldown (dim, it's dead), up within
// RESPAWN_SOON_SECS, popped, and no kill seen this session.
const DOT_COOLDOWN = '#4b5563'
const DOT_SOON = '#f59e0b'
const DOT_UP = '#22c55e'
const DOT_UNKNOWN = '#94a3b8'
const RESPAWN_SOON_SECS = 120

// AggroScope is which aggro rings are drawn: none, those within the chosen
// distance of you (fading out toward it), or every spawn point's.
export type AggroScope = 'off' | 'near' | 'all'
export const AGGRO_SCOPES: [AggroScope, string, string][] = [
  ['off', 'Off', 'No aggro rings'],
  ['near', 'Near', 'Only rings near you, fading out with distance'],
  ['all', 'All', 'Every spawn point’s ring'],
]

// spawnNames is who can spawn at a point, for the 3D map's hover.
function spawnNames(p: ZoneSpawnPoint): string {
  const names = [...new Set(p.npcs.map((n) => n.name.replace(/_/g, ' ')))]
  return names.length > 3 ? `${names.slice(0, 3).join(', ')} +${names.length - 3}` : names.join(', ')
}

function fmtRemaining(secs: number): string {
  const s = Math.ceil(secs)
  if (s >= 3600) return `${Math.floor(s / 3600)}h${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}`
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

export function DotLegend(): React.ReactElement {
  const rows: [string, string][] = [
    [DOT_COOLDOWN, 'Dead, on cooldown'],
    [DOT_SOON, 'Up within 2 min'],
    [DOT_UP, 'Popped'],
    [DOT_UNKNOWN, 'No kill seen'],
  ]
  return (
    <div className="mt-1 space-y-0.5 text-[9px]" style={{ color: 'var(--color-muted)' }}>
      {rows.map(([c, label]) => (
        <div key={label} className="flex items-center gap-1">
          <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: c }} />
          {label}
        </div>
      ))}
    </div>
  )
}


// TARGET_RING_COLOR is your live target's aggro ring, the same red as the
// line drawn to it.
export const TARGET_RING_COLOR = '#f87171'

// npcKey reduces an NPC name to compare a Zeal target against spawn data:
// "a_death_beetle" and "a death beetle" match, as do "#Lord_Vyemm" and
// "Lord Vyemm".
function npcKey(name: string): string {
  return name.replace(/[#_\d]/g, ' ').replace(/\s+/g, ' ').trim().toLowerCase()
}

// SpawnOverlayOptions is what's drawn from a zone's spawn points: aggro rings
// by scope (Near fades them out within aggroNear of here), and respawn dots,
// without the dead ones when hideKilled is set. target, an NPC you have
// targeted, gets its aggro ring where it stands.
export interface SpawnOverlayOptions {
  aggroScope: AggroScope
  aggroNear: number
  dots: boolean
  hideKilled: boolean
  here: { x: number; y: number; z: number } | null
  target?: { name: string; type: number; x: number; y: number; z: number } | null
}

// useSpawnOverlays fetches a zone's spawn points and the live respawn state
// while either layer is on, and turns them into rings and dots for the map.
// A spawn known to be dead has no ring.
export function useSpawnOverlays(
  zoneShortName: string | null | undefined,
  { aggroScope, aggroNear, dots, hideKilled, here, target }: SpawnOverlayOptions,
): { aggroRings: AggroRing[]; spawnDots: SpawnDot[]; targetRing: AggroRing | null } {
  const aggroOn = aggroScope !== 'off'
  const dotsOn = dots
  const playerLevel = useActiveCharacterLevel()
  const [spawnPoints, setSpawnPoints] = useState<ZoneSpawnPoint[]>([])
  const wantSpawns = aggroOn || dotsOn
  // An NPC target needs the spawn data for its aggro radius, but not the
  // respawn state.
  const targetNPC = target && target.type === 1 ? target : null
  const wantPoints = wantSpawns || targetNPC != null
  useEffect(() => {
    if (!wantPoints || !zoneShortName) {
      setSpawnPoints([])
      return
    }
    let cancelled = false
    getZoneSpawnPoints(zoneShortName)
      .then((r) => { if (!cancelled) setSpawnPoints(r) })
      .catch(() => { if (!cancelled) setSpawnPoints([]) })
    return () => { cancelled = true }
  }, [wantPoints, zoneShortName])
  const [respawns, setRespawns] = useState<RespawnState | null>(null)
  const wsReady = useWebSocket((msg) => {
    // Rings want it too: a spawn that's dead has no ring.
    if (wantSpawns && msg.type === WSEvent.OverlayRespawns) setRespawns(msg.data as RespawnState)
  })
  useEffect(() => {
    if (!wantSpawns || wsReady !== 'open') return
    getRespawnState().then(setRespawns).catch(() => {})
  }, [wantSpawns, wsReady])
  // Seconds until each spawn point's mob is back, by spawn id; 0 or less
  // means up.
  const respawnLeft = useMemo(() => {
    const bySpawn = new Map<number, number>()
    for (const t of respawns?.timers ?? []) {
      if (t.zone !== zoneShortName || !t.spawn_id || t.other_copy) continue
      const prev = bySpawn.get(t.spawn_id)
      if (prev === undefined || t.remaining_seconds < prev) bySpawn.set(t.spawn_id, t.remaining_seconds)
    }
    return bySpawn
  }, [respawns, zoneShortName])
  const spawnDots = useMemo((): SpawnDot[] => {
    const out: SpawnDot[] = []
    for (const p of spawnPoints) {
      const left = respawnLeft.get(p.spawn_id)
      const name = spawnNames(p)
      if (left === undefined) {
        out.push({ x: p.x, y: p.y, z: p.z, color: DOT_UNKNOWN, known: false, name })
        continue
      }
      if (hideKilled && left > RESPAWN_SOON_SECS) continue
      const color = left <= 0 ? DOT_UP : left <= RESPAWN_SOON_SECS ? DOT_SOON : DOT_COOLDOWN
      out.push({
        x: p.x, y: p.y, z: p.z, color, known: true, name,
        label: left > 0 ? fmtRemaining(left) : 'up',
      })
    }
    return out
  }, [spawnPoints, respawnLeft, hideKilled])
  const aggroRings = useMemo((): AggroRing[] => {
    const out: AggroRing[] = []
    for (const p of spawnPoints) {
      const left = respawnLeft.get(p.spawn_id)
      if (left !== undefined && left > 0) continue
      const ring = spawnRing(p, playerLevel)
      if (aggroScope === 'near' && here) {
        // Height counts double, so floors above and below drop out sooner.
        const d = Math.hypot(p.x - here.x, p.y - here.y, (p.z - here.z) * 2)
        if (d > aggroNear) continue
        ring.alpha = 1 - 0.75 * (d / aggroNear) ** 2
      }
      out.push(ring)
    }
    return out
  }, [spawnPoints, respawnLeft, playerLevel, aggroScope, aggroNear, here])

  // The target's widest aggro radius among the spawn points that can roll
  // it, centred where it stands now.
  const targetKey = targetNPC ? npcKey(targetNPC.name) : ''
  const targetRadius = useMemo(() => {
    if (!targetKey) return 0
    let r = 0
    for (const p of spawnPoints) {
      for (const n of p.npcs) if (npcKey(n.name) === targetKey) r = Math.max(r, n.aggro_radius)
    }
    return r
  }, [spawnPoints, targetKey])
  const targetRing = useMemo(
    (): AggroRing | null =>
      targetNPC && targetRadius > 0
        ? { x: targetNPC.x, y: targetNPC.y, z: targetNPC.z, radius: targetRadius, color: TARGET_RING_COLOR }
        : null,
    [targetNPC, targetRadius],
  )

  return { aggroRings, spawnDots, targetRing }
}

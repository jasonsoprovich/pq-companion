import React from 'react'
import type { PlayerSighting } from '../../types/player'

function muted(extra?: React.CSSProperties): React.CSSProperties {
  return { fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px', lineHeight: 1.5, ...extra }
}

function Heading({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <p style={{ fontSize: 9, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.08em', margin: '0 0 4px' }}>
      {children}
    </p>
  )
}

function Chip({ label, value, color }: { label?: string; value: string | number; color?: string }): React.ReactElement {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        gap: 4,
        backgroundColor: 'rgba(255,255,255,0.06)',
        borderRadius: 3,
        padding: '3px 7px',
        fontSize: 12,
        lineHeight: 1.4,
      }}
    >
      {label && (
        <span style={{ color: 'rgba(255,255,255,0.4)', fontWeight: 600, fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          {label}
        </span>
      )}
      <span style={{ color: color ?? 'rgba(255,255,255,0.9)', fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
        {value}
      </span>
    </span>
  )
}

/**
 * Renders what the player tracker knows about a targeted character — level,
 * class, race, guild — when the target isn't an NPC in the game database. Data
 * comes from useTargetPlayer (the /who-sighting store). When the name isn't a
 * tracked player it falls back to a short explanation rather than the bare
 * "no record" message, so a player target no longer reads as an error. Shared
 * by the dashboard NPC panel and the popped-out NPC overlay window.
 */
export default function TargetPlayerCard({
  player,
  loading,
}: {
  player: PlayerSighting | null
  loading: boolean
}): React.ReactElement {
  if (loading) {
    return <p style={muted()}>Looking up player…</p>
  }
  if (!player) {
    return (
      <p style={muted()}>
        No NPC database record. If this is a player, their class and level will
        appear here once they&rsquo;ve been seen in a /who.
      </p>
    )
  }

  const level = player.last_seen_level > 0 ? player.last_seen_level : null
  const hasDetails = !!(level || player.class || player.race)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
      <div>
        <Heading>Player</Heading>
        {hasDetails ? (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {level && <Chip label="Lv" value={level} color="#c9a84c" />}
            {player.class && <Chip value={player.class} />}
            {player.race && <Chip value={player.race} />}
          </div>
        ) : (
          <p style={muted({ padding: 0 })}>Anonymous — no class or level seen yet.</p>
        )}
      </div>

      {player.guild && (
        <div>
          <Heading>Guild</Heading>
          <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.9)', margin: 0 }}>{player.guild}</p>
        </div>
      )}

      <p style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', margin: 0 }}>
        {player.last_seen_zone ? `Last seen in ${player.last_seen_zone}` : 'Zone unknown'}
        {player.sightings_count > 0
          ? ` · ${player.sightings_count} sighting${player.sightings_count === 1 ? '' : 's'}`
          : ''}
      </p>
    </div>
  )
}

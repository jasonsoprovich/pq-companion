// Zeal in-game map commands, built for the clipboard.
//
// Zeal (https://github.com/CoastalRedwood/Zeal) embeds map geometry for every
// zone inside zeal.asi, so these commands drop pins on a map the player already
// has — PQ Companion ships no map data to make this work.
//
// Paste one command at a time. EQ collapses a multi-line paste into a single
// line, so batching several commands into one copy produces one broken line
// rather than several working ones.

import { clampChatLine } from './eqClipboard'

// ── Coordinate convention ─────────────────────────────────────────────────────
//
// `/map marker` takes its arguments in the same order and sign as in-game
// `/loc`, which prints Y before X. That matches spawn2.y / spawn2.x from
// quarm.db directly, with no negation — Zeal applies the map-space negation
// internally when it draws the pin.
//
// NOT YET VERIFIED IN GAME. The check is 30 seconds: stand anywhere in North
// Qeynos and run
//
//     /map marker 6 235 Test
//
// The pin should land on the Priest of Discord (quarm.db has him at x=235,
// y=6). If it lands mirrored, negate both values in markerArgs below — that is
// the only place the convention is encoded, so the fix is one line.
function markerArgs(x: number, y: number): string {
  return `${Math.round(y)} ${Math.round(x)}`
}

// Zeal centres the label above the pin. Collapse whitespace so a name with
// spaces can't be mistaken for extra arguments, and keep it short — the label
// is drawn on the map, not in chat.
function markerLabel(name: string): string {
  return name.replace(/_/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 40)
}

// mapMarkerCommand pins a location on the player's own map.
export function mapMarkerCommand(x: number, y: number, name?: string): string {
  const label = name ? ` ${markerLabel(name)}` : ''
  return clampChatLine(`/map marker ${markerArgs(x, y)}${label}`)
}

// mapRaidMarkerCommand pins a location and broadcasts it to the raid.
export function mapRaidMarkerCommand(x: number, y: number, name?: string): string {
  const label = name ? ` ${markerLabel(name)}` : ''
  return clampChatLine(`/map rsay marker ${markerArgs(x, y)}${label}`)
}

// mapShowZoneCommand previews another zone's map without travelling there.
// Takes the zone short name (spawn2.zone / zone.short_name), e.g. "qeynos2".
export function mapShowZoneCommand(shortName: string): string {
  return clampChatLine(`/map show_zone ${shortName}`)
}

// mapClearMarkersCommand clears every marker the player has placed.
export function mapClearMarkersCommand(): string {
  return '/map marker'
}

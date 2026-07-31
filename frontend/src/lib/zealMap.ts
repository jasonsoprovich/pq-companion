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
// VERIFIED IN GAME 2026-07-29: `/map marker 6 235 Test` run in North Qeynos
// dropped the pin directly on the Priest of Discord, who sits at x=235, y=6 in
// quarm.db. Order and sign are both confirmed.
//
// This stays the only place the convention is encoded, so a future Zeal change
// is a one-line fix here.
function markerArgs(x: number, y: number): string {
  return `${Math.round(y)} ${Math.round(x)}`
}

// Zeal centres the label above the pin.
//
// Spaces MUST become underscores, not the other way round. Zeal parses the
// label with `%s`, which stops at the first whitespace, so "Ward Pungill"
// silently became a marker named "Ward" — and quoting does not help, because
// `%s` has no notion of quotes and would keep the opening one. Underscores are
// what the in-game map draws for every built-in label anyway, so this reads
// native rather than escaped.
//
// This is the same rule as the map-file exporter's sanitizeLabel
// (backend/internal/mapexport/format.go); both write labels Zeal must parse.
// Commas go too — anything reading the line as fields would split on them.
function markerLabel(name: string): string {
  return name
    .trim()
    .replace(/\s+/g, '_')
    .replace(/,/g, ';')
    .slice(0, 40)
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

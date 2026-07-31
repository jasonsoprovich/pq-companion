// Zone map types, mirroring backend/internal/maps.

export interface MapZone {
  zone: string
  min_x: number
  min_y: number
  max_x: number
  max_y: number
  // technique is which extractor built this zone's geometry: 'boundary',
  // 'contours' or 'silhouette'. Surfaced because it changes how the map should
  // be read — contour lines are elevation, not walls.
  technique: 'boundary' | 'contours' | 'silhouette'
  z_span: number
  // min_z/max_z are the heights actually present in the drawn segments, which is
  // what the depth control's range must span. z_span is only a width.
  min_z: number
  max_z: number
}

export type MapPOICategory =
  | 'zone_line'
  | 'locked'
  | 'switch'
  | 'teleport'
  | 'wall'
  | 'hazard'
  | 'note'
  | 'ground_spawn'
  | 'trap'
  | 'tradeskill'
  | 'succor'
  | 'vendor'
  | 'raid_target'
  | 'door'

export interface MapPOI {
  id: number
  x: number
  y: number
  z: number
  category: MapPOICategory
  label: string
  // source distinguishes generated rows ('db:*') from hand-researched or
  // community ones, so a quarm.db regeneration can refresh the former without
  // destroying the latter.
  source: string
  ref_id?: number
}

export interface MapZoneDetail {
  zone: MapZone
  pois: MapPOI[]
}

// ZoneNPCLocation is one NPC spawn point, for map search.
//
// Separate from MapPOI rather than folded into it: a POI is a curated category
// the map draws a layer for, and adding every NPC in the zone to that set would
// bury the layers under thousands of pins. These are only ever shown one at a
// time, as the answer to a search.
export interface ZoneNPCLocation {
  npc_id: number
  name: string
  // Map space, same convention as MapPOI.
  x: number
  y: number
  z: number
  level: number
  raid_target: boolean
}

export interface MapStatus {
  available: boolean
  zones: number
}

// MapGeometry is a zone's line segments, decoded from the packed binary the
// geometry endpoint serves. Flat arrays rather than objects: a big zone has
// tens of thousands of segments and this is drawn every frame while panning.
export interface MapGeometry {
  count: number
  // 6 entries per segment: x1, y1, z1, x2, y2, z2
  coords: Int16Array
  // colors is 3 bytes per segment (r, g, b), present only for an external map
  // pack — the pack's own palette is the reason to render it, since the colour
  // is how a hand-drawn map says "this is water" rather than "this is a wall".
  // Absent for our own layers, which the renderer themes itself.
  colors?: Uint8Array
}

// ExternalMapStatus describes a third-party .txt map pack found in the player's
// EQ folder. Never bundled — read in place, and the mode vanishes if the files
// do.
export interface ExternalMapStatus {
  available: boolean
  name?: string
  dir?: string
  zones?: number
}

// MapRenderMode selects which geometry layers to draw.
//
//   outline  — the clean layer: one simplified line drawing, the same visual
//              language in every zone. Best for reading a route.
//   detailed — whatever the classifier chose for this zone, plus the fine
//              boundary layer. Far more information, and in tall zones it
//              reads as a stack of overlapping levels.
//   external — a map pack the user installed in their own EQ folder, drawn in
//              its own colours. Only selectable when one is detected.
export type MapRenderMode = 'outline' | 'detailed' | 'external'

// UserAnnotation is a marker the user placed themselves, from user.db.
//
// Kept separate from MapPOI even though both draw as pins: these are editable
// and deletable, MapPOIs are not, and merging the two would mean every pin
// needing an is-it-mine check at every call site.
export interface UserAnnotation {
  id: number
  zone: string
  x: number
  y: number
  z: number
  category: 'wall' | 'hazard' | 'note'
  label: string
  created_at: number
  updated_at: number
}

// GameMapExportStatus describes whether markers can be written into the EQ
// folder, and what is already there.
export interface GameMapExportStatus {
  eq_path: string
  ready: boolean
  reason?: string
  default_categories?: string[]
  existing_files: number
  foreign_files: number
  exported_files: number
}

export interface GameMapExportResult {
  written: number
  skipped: number
  points: number
  dir: string
}

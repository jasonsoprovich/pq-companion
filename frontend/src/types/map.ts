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
}

// MapRenderMode selects which geometry layers to draw.
//
//   outline  — the clean layer: one simplified line drawing, the same visual
//              language in every zone. Best for reading a route.
//   detailed — whatever the classifier chose for this zone, plus the fine
//              boundary layer. Far more information, and in tall zones it
//              reads as a stack of overlapping levels.
export type MapRenderMode = 'outline' | 'detailed'

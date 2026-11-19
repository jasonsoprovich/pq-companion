// The nine classic EQ faction disposition ranges shown by /con, worst to
// best. Mirrors backend/internal/logparser/models.go FactionBucketOrder —
// keep in sync.
export type FactionBucket =
  | 'scowling'
  | 'threatening'
  | 'dubious'
  | 'apprehensive'
  | 'indifferent'
  | 'amiable'
  | 'kindly'
  | 'warmly'
  | 'ally'

export const BUCKET_ORDER: FactionBucket[] = [
  'scowling',
  'threatening',
  'dubious',
  'apprehensive',
  'indifferent',
  'amiable',
  'kindly',
  'warmly',
  'ally',
]

export const BUCKET_LABEL: Record<FactionBucket, string> = {
  scowling: 'Scowling',
  threatening: 'Threatening',
  dubious: 'Dubious',
  apprehensive: 'Apprehensive',
  indifferent: 'Indifferent',
  amiable: 'Amiable',
  kindly: 'Kindly',
  warmly: 'Warmly',
  ally: 'Ally',
}

// Red (hostile) to green (friendly), evenly spread across the 9 segments.
export const BUCKET_COLOR: Record<FactionBucket, string> = {
  scowling: '#dc2626',
  threatening: '#ea580c',
  dubious: '#f59e0b',
  apprehensive: '#eab308',
  indifferent: '#a3a3a3',
  amiable: '#84cc16',
  kindly: '#4ade80',
  warmly: '#22c55e',
  ally: '#16a34a',
}

export function bucketIndex(bucket: string): number {
  return BUCKET_ORDER.indexOf(bucket as FactionBucket)
}

// Lowest total (modified) faction value /con reports as each bucket, per
// EQMacEmu's CalculateFaction (common/faction.cpp). Scowling has no floor.
export const BUCKET_MIN: Record<FactionBucket, number> = {
  scowling: -Infinity,
  threatening: -750,
  dubious: -500,
  apprehensive: -100,
  indifferent: 0,
  amiable: 100,
  kindly: 500,
  warmly: 750,
  ally: 1100,
}

// bucketForTotal maps a total (personal + modifier) faction value to the
// bucket /con would report.
export function bucketForTotal(total: number): FactionBucket {
  for (let i = BUCKET_ORDER.length - 1; i > 0; i--) {
    if (total >= BUCKET_MIN[BUCKET_ORDER[i]]) return BUCKET_ORDER[i]
  }
  return 'scowling'
}

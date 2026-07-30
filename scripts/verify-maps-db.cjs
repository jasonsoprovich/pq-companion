#!/usr/bin/env node
// verify-maps-db.cjs — check that backend/data/maps.db is present, non-trivial,
// and newer than the extraction pipeline that produces it. Run from dist:win
// before packaging.
//
// Why this exists: maps.db is gitignored and generated offline by
// `go run ./cmd/mapgen -client <EQ dir>`, on a machine that has an EQ client.
// electron-builder copies it via extraResources without caring whether it is
// there, and the app degrades *silently* when maps are absent — the Map tab
// simply does not appear. So a build with no maps.db, or with geometry from
// before a pipeline change, ships looking like a feature that was never
// written rather than like a broken build. That is exactly the failure mode
// that shipped stale backends for several releases before
// verify-backend-fresh.cjs was added; this is the same guard for map data.

const fs = require('fs')
const path = require('path')

const repoRoot = path.resolve(__dirname, '..')
const dbPath = path.join(repoRoot, 'backend', 'data', 'maps.db')
const mapgenDir = path.join(repoRoot, 'backend', 'internal', 'mapgen')

const REGEN = '  Regenerate: cd backend && go run ./cmd/mapgen -client <EQ client dir>'

if (!fs.existsSync(dbPath)) {
  console.error(`[verify-maps-db] FAIL: ${path.relative(repoRoot, dbPath)} does not exist.`)
  console.error('  Packaging without it ships an installer whose Map tab never appears.')
  console.error(REGEN)
  process.exit(1)
}

// A truncated or empty file would pass an existence check and fail at runtime.
// The real artifact is ~13 MB; 1 MB is a floor no plausible full build is under.
const MIN_BYTES = 1 << 20
const size = fs.statSync(dbPath).size
if (size < MIN_BYTES) {
  console.error(`[verify-maps-db] FAIL: ${path.relative(repoRoot, dbPath)} is only ${size} bytes.`)
  console.error(`  Expected at least ${MIN_BYTES} — the file looks truncated or empty.`)
  console.error(REGEN)
  process.exit(1)
}

// Staleness: the extractor changing without a regen means the shipped geometry
// is not what the current code produces. That is invisible in the app.
const dbMtime = fs.statSync(dbPath).mtimeMs
let newestSource = 0
let newestSourcePath = ''
for (const entry of fs.readdirSync(mapgenDir, { withFileTypes: true })) {
  // _test.go files do not affect output.
  if (!entry.isFile() || !entry.name.endsWith('.go') || entry.name.endsWith('_test.go')) continue
  const full = path.join(mapgenDir, entry.name)
  const m = fs.statSync(full).mtimeMs
  if (m > newestSource) {
    newestSource = m
    newestSourcePath = full
  }
}

if (newestSource > dbMtime) {
  console.error(`[verify-maps-db] FAIL: ${path.relative(repoRoot, dbPath)} is older than the extractor.`)
  console.error(`  maps.db mtime: ${new Date(dbMtime).toISOString()}`)
  console.error(`  newest source: ${new Date(newestSource).toISOString()}  (${path.relative(repoRoot, newestSourcePath)})`)
  console.error('  The shipped geometry is not what the current pipeline produces.')
  console.error(REGEN)
  process.exit(1)
}

console.log(
  `[verify-maps-db] OK: maps.db present (${(size / (1 << 20)).toFixed(1)} MB) and newer than the extractor.`,
)

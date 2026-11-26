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
//
// "The extractor" is only the code cmd/mapgen can reach, listed in
// backend/internal/mapgen/extractor_files.txt (kept current by the Go test
// TestExtractorFiles). The same package also holds the 3D view, textures and
// character models, which run only inside the app; editing those must not
// demand a regen.
//
// Each file's last change is its last commit time, not its mtime: a fresh
// checkout, worktree or pull stamps every file "now" without changing a byte.
// Files with uncommitted edits fall back to their mtime.
const { execFileSync } = require('child_process')
const backendDir = path.join(repoRoot, 'backend')
const listPath = path.join(mapgenDir, 'extractor_files.txt')
if (!fs.existsSync(listPath)) {
  console.error(`[verify-maps-db] FAIL: ${path.relative(repoRoot, listPath)} is missing.`)
  console.error('  Regenerate: cd backend && go test ./internal/mapgen -run TestExtractorFiles -update')
  process.exit(1)
}
const sources = fs
  .readFileSync(listPath, 'utf8')
  .split('\n')
  .map((l) => l.trim())
  .filter((l) => l && !l.startsWith('#'))
  .map((rel) => path.join('backend', rel))

function git(args) {
  return execFileSync('git', args, { cwd: repoRoot, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] })
}

const dbMtime = fs.statSync(dbPath).mtimeMs
let newestSource = 0
let newestSourcePath = ''
let newestHow = ''
const consider = (ms, file, how) => {
  if (ms > newestSource) {
    newestSource = ms
    newestSourcePath = file
    newestHow = how
  }
}

const dirty = new Set()
let useGit = true
try {
  for (const line of git(['status', '--porcelain', '--', ...sources]).split('\n')) {
    if (line.length > 3) dirty.add(line.slice(3).trim())
  }
} catch {
  useGit = false
}

for (const rel of sources) {
  const full = path.join(repoRoot, rel)
  if (!fs.existsSync(full)) {
    console.error(`[verify-maps-db] FAIL: ${rel} is listed in extractor_files.txt but missing.`)
    console.error('  Regenerate: cd backend && go test ./internal/mapgen -run TestExtractorFiles -update')
    process.exit(1)
  }
  if (!useGit || dirty.has(rel)) {
    consider(fs.statSync(full).mtimeMs, rel, useGit ? 'uncommitted edit' : 'mtime (no git)')
    continue
  }
  const out = git(['log', '-1', '--format=%ct %h', '--', rel]).trim()
  if (out) {
    const [ct, sha] = out.split(' ')
    consider(Number(ct) * 1000, rel, `commit ${sha}`)
  }
}

if (newestSource > dbMtime) {
  console.error(`[verify-maps-db] FAIL: ${path.relative(repoRoot, dbPath)} is older than the extractor.`)
  console.error(`  maps.db built:     ${new Date(dbMtime).toISOString()}`)
  console.error(`  extractor changed: ${new Date(newestSource).toISOString()}  (${newestSourcePath}, ${newestHow})`)
  console.error('  The shipped geometry may not be what the current pipeline produces.')
  console.error(REGEN)
  process.exit(1)
}

console.log(
  `[verify-maps-db] OK: maps.db present (${(size / (1 << 20)).toFixed(1)} MB) and newer than the extractor (${sources.length} files).`,
)

#!/usr/bin/env node
// verify-backend-fresh.cjs — belt-and-suspenders check that the Windows Go
// sidecar binary at backend/bin/pq-companion-server.exe is newer than every
// Go source file under backend/. Run from dist:win after build:backend:win
// to catch regressions where the build chain stops rebuilding the binary
// (which silently shipped stale backends through several releases until
// it was discovered while debugging a port-conflict bug).
//
// Exits non-zero with a clear message if the binary is missing or older
// than any Go source. Safe to delete the binary and re-run dist:win.

const fs = require('fs')
const path = require('path')

const repoRoot = path.resolve(__dirname, '..')
const binPath = path.join(repoRoot, 'backend', 'bin', 'pq-companion-server.exe')
const backendDir = path.join(repoRoot, 'backend')

if (!fs.existsSync(binPath)) {
  console.error(`[verify-backend-fresh] FAIL: ${binPath} does not exist.`)
  console.error('  Run `npm run build:backend:win` before packaging.')
  process.exit(1)
}

const binMtime = fs.statSync(binPath).mtimeMs

let newestSource = 0
let newestSourcePath = ''

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'bin' || entry.name === 'data' || entry.name === 'testdata' || entry.name.startsWith('.')) continue
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      walk(full)
    } else if (entry.isFile() && (entry.name.endsWith('.go') || entry.name === 'go.mod' || entry.name === 'go.sum')) {
      const m = fs.statSync(full).mtimeMs
      if (m > newestSource) {
        newestSource = m
        newestSourcePath = full
      }
    }
  }
}

walk(backendDir)

if (newestSource > binMtime) {
  console.error(`[verify-backend-fresh] FAIL: ${path.relative(repoRoot, binPath)} is older than backend source.`)
  console.error(`  binary mtime:  ${new Date(binMtime).toISOString()}`)
  console.error(`  newest source: ${new Date(newestSource).toISOString()}  (${path.relative(repoRoot, newestSourcePath)})`)
  console.error('  Run `npm run build:backend:win` to rebuild before packaging.')
  process.exit(1)
}

console.log(`[verify-backend-fresh] OK: backend binary is newer than all Go sources.`)

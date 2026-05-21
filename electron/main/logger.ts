// Mirrors Electron main-process console output and sidecar stdio into
// ~/.pq-companion/logs/electron.log. Packaged Windows builds have no
// attached console so console.log calls and the [backend] stream we
// already pipe from the Go sidecar are otherwise invisible. Keeps the
// last 3 launches' logs so a user can attach the file to a bug report.
import { existsSync, mkdirSync, openSync, renameSync, unlinkSync, writeSync, closeSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'

const KEEP_SESSIONS = 3

let fd: number | null = null
let logPath: string | null = null

// numbered returns "electron.log" for n=0, "electron.1.log" for n=1, ...
function numbered(base: string, n: number): string {
  if (n === 0) return base
  const dot = base.lastIndexOf('.')
  if (dot < 0) return `${base}.${n}`
  return `${base.slice(0, dot)}.${n}${base.slice(dot)}`
}

// rotate renames electron.log → electron.1.log → electron.2.log → ..., dropping
// anything past `keep`. Same scheme as the Go-side applog rotator.
function rotate(base: string, keep: number): void {
  for (let i = keep; i >= 1; i--) {
    const src = numbered(base, i - 1)
    const dst = numbered(base, i)
    if (i === keep && existsSync(dst)) {
      try {
        unlinkSync(dst)
      } catch {
        // ignore — best-effort cleanup
      }
    }
    if (existsSync(src)) {
      try {
        renameSync(src, dst)
      } catch {
        // ignore — rotation is best-effort
      }
    }
  }
}

// initLogger opens the rotated log file and patches console.{log,warn,error,info}
// to also append a timestamped line to it. Safe to call once at app start.
export function initLogger(appVersion: string): { logPath: string | null; error: Error | null } {
  try {
    const dir = join(homedir(), '.pq-companion', 'logs')
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true })
    logPath = join(dir, 'electron.log')
    rotate(logPath, KEEP_SESSIONS)
    fd = openSync(logPath, 'a')
  } catch (err) {
    return { logPath: null, error: err as Error }
  }

  patchConsole()

  appendLine('INFO', `electron starting version=${appVersion} pid=${process.pid} platform=${process.platform} arch=${process.arch}`)
  appendLine('INFO', `node=${process.versions.node} electron=${process.versions.electron} chrome=${process.versions.chrome}`)
  appendLine('INFO', `exec=${process.execPath}`)
  appendLine('INFO', `cwd=${process.cwd()}`)

  // Surface anything that would otherwise vanish into the void.
  process.on('uncaughtException', (err) => {
    appendLine('ERROR', `uncaughtException: ${err.stack || err.message || String(err)}`)
  })
  process.on('unhandledRejection', (reason) => {
    const r = reason instanceof Error ? reason.stack || reason.message : String(reason)
    appendLine('ERROR', `unhandledRejection: ${r}`)
  })

  return { logPath, error: null }
}

// closeLogger flushes and releases the file handle on app quit.
export function closeLogger(): void {
  if (fd !== null) {
    try {
      closeSync(fd)
    } catch {
      // ignore
    }
    fd = null
  }
}

// appendLine writes one line to the log file. Used internally and also
// exported so the sidecar stdout/stderr piping can tee its chunks here
// without going through console.* (which would prepend [main] noise).
export function appendLine(level: string, msg: string): void {
  if (fd === null) return
  const ts = new Date().toISOString()
  const stripped = msg.replace(/\r?\n$/, '')
  const line = `${ts} ${level} ${stripped}\n`
  try {
    writeSync(fd, line)
  } catch {
    // If the file handle has gone bad, give up silently — we don't want a
    // log failure to take down the app. Stderr still has the original output.
  }
}

// patchConsole wraps console.{log,info,warn,error} so each call still hits
// stdio AND appends a line to the log file. Done once at init; safe to call
// before any IPC handlers register.
function patchConsole(): void {
  const orig = {
    log: console.log.bind(console),
    info: console.info.bind(console),
    warn: console.warn.bind(console),
    error: console.error.bind(console),
  }
  const format = (args: unknown[]): string =>
    args
      .map((a) => {
        if (a instanceof Error) return a.stack || a.message
        if (typeof a === 'object') {
          try {
            return JSON.stringify(a)
          } catch {
            return String(a)
          }
        }
        return String(a)
      })
      .join(' ')

  console.log = (...args: unknown[]) => {
    orig.log(...args)
    appendLine('INFO', format(args))
  }
  console.info = (...args: unknown[]) => {
    orig.info(...args)
    appendLine('INFO', format(args))
  }
  console.warn = (...args: unknown[]) => {
    orig.warn(...args)
    appendLine('WARN', format(args))
  }
  console.error = (...args: unknown[]) => {
    orig.error(...args)
    appendLine('ERROR', format(args))
  }
}

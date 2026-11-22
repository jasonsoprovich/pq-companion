// perfRecorder writes a performance timeline to ~/.pq-companion/logs/
// perf-<date>-<time>.log while it's switched on (Settings → Advanced →
// Diagnostics). Every SAMPLE_MS it logs memory and CPU for every Electron
// process, labelled with the overlay(s) it renders, plus the Go sidecar's
// memory; overlay open/close/hide/show and layout switches are logged as
// events in between. Stopping (or quitting) appends a summary: baseline with
// no overlays, cost per overlay window, peak, and memory growth while the
// overlay set held steady (the leak signal).
//
// Lines are appended as they happen, so a crash still leaves a usable file.
//
// The goal metric is "total": every Electron process's private bytes plus
// the sidecar's (Windows). Where the OS has no private bytes (macOS dev
// builds) the working set stands in, and the header says so.
import { appendFileSync, existsSync, mkdirSync, readdirSync, unlinkSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'

const SAMPLE_MS = 5000
// A steady stretch must last this long to be judged for growth.
const STEADY_MIN_MS = 5 * 60 * 1000
const EDGE_MS = 60 * 1000
// Older recordings beyond this many are deleted when a new one starts.
const KEEP_RECORDINGS = 10

function pruneOld(dir: string): void {
  try {
    const old = readdirSync(dir).filter((f) => /^perf-\d{8}-\d{6}\.log$/.test(f)).sort()
    for (const f of old.slice(0, Math.max(0, old.length - (KEEP_RECORDINGS - 1)))) unlinkSync(join(dir, f))
  } catch {
    // Best-effort.
  }
}

export type PerfProcessRow = {
  pid: number
  role: 'overlay' | 'main' | 'trigger' | 'streamkit' | 'helper'
  keys: string[]
  state?: 'open' | 'hidden'
  type: string
  memoryMB: number
  privateMB: number | null
  cpu: number
}

export type SidecarMemory = {
  working_set_mb: number
  private_mb: number | null
  peak_rss: boolean
  heap_inuse_mb: number
  heap_sys_mb: number
  sys_mb: number
  num_goroutine: number
  num_gc: number
}

export type PerfRecorderDeps = {
  collect: () => PerfProcessRow[]
  sidecar: () => Promise<SidecarMemory | null>
  // Display name for an overlay key (named timer windows etc.).
  label: (key: string) => string
  appVersion: string
  processMode: () => string
}

type Sample = {
  t: number
  total: number
  electron: number
  sidecar: number | null
  sidecarHeap: number | null
  gpu: number
  cpu: number
  // Open overlay windows (processes) and their keys.
  windows: number
  keys: string[]
  setId: string
  perKey: Map<string, number>
}

export type PerfStatus = { recording: boolean; file: string | null; startedAt: number | null }

const r1 = (v: number): string => v.toFixed(1)

function clock(t: number): string {
  const d = new Date(t)
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

function stamp(t: number): string {
  const d = new Date(t)
  const p = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
}

const avg = (xs: number[]): number => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : 0)

export class PerfRecorder {
  private deps: PerfRecorderDeps
  private file: string | null = null
  private startedAt: number | null = null
  private timer: ReturnType<typeof setInterval> | null = null
  private samples: Sample[] = []
  private lastStates = new Map<string, 'open' | 'hidden'>()
  private hasPrivate = false
  private sampling = false

  constructor(deps: PerfRecorderDeps) {
    this.deps = deps
  }

  status(): PerfStatus {
    return { recording: this.timer !== null, file: this.file, startedAt: this.startedAt }
  }

  start(): PerfStatus {
    if (this.timer) return this.status()
    const dir = join(homedir(), '.pq-companion', 'logs')
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true })
    pruneOld(dir)
    const now = Date.now()
    this.file = join(dir, `perf-${stamp(now)}.log`)
    this.startedAt = now
    this.samples = []
    this.lastStates = new Map()
    const rows = this.deps.collect()
    this.hasPrivate = rows.some((r) => r.privateMB !== null)
    this.write([
      'PQ Companion performance recording',
      `Started:      ${new Date(now).toString()}`,
      `Build:        ${this.deps.appVersion}`,
      `Platform:     ${process.platform} ${process.arch}, electron ${process.versions.electron}, chrome ${process.versions.chrome}`,
      `Process mode: ${this.deps.processMode()}`,
      `Sample every: ${SAMPLE_MS / 1000} s`,
      `Memory:       ${this.hasPrivate ? 'private bytes' : 'working set (no private bytes on this OS)'} in MB`,
      'CPU:          % of one core since the previous sample (the Developer > Processes table, if open, shortens that window)',
      '',
      'Sample line: total = all Electron processes + Go sidecar. Then each process as name=memory/cpu%.',
      '',
    ])
    this.timer = setInterval(() => void this.sample(), SAMPLE_MS)
    this.logEvent('recording started')
    void this.sample()
    return this.status()
  }

  // stop ends the recording and appends the summary. Synchronous apart from
  // the summary itself, so it's safe from before-quit.
  stop(reason = 'stopped'): PerfStatus {
    if (!this.timer) return this.status()
    clearInterval(this.timer)
    this.timer = null
    this.logEvent(`recording ${reason}`)
    this.write(['', ...this.summary()])
    const st = this.status()
    this.startedAt = null
    return st
  }

  // event logs something that happened between samples (layout switch etc.).
  event(text: string): void {
    if (!this.timer) return
    this.logEvent(text)
  }

  private logEvent(text: string): void {
    this.write([`[${clock(Date.now())}] EVENT ${text}`])
  }

  private write(lines: string[]): void {
    if (!this.file) return
    try {
      appendFileSync(this.file, lines.join('\n') + '\n')
    } catch {
      // Best-effort diagnostics; never break the app over it.
    }
  }

  private mem(r: PerfProcessRow): number {
    return this.hasPrivate ? r.privateMB ?? 0 : r.memoryMB
  }

  private name(r: PerfProcessRow): string {
    switch (r.role) {
      case 'main': return 'main'
      case 'trigger': return 'trigger'
      case 'streamkit': return 'streamkit'
      case 'overlay': return r.keys.map((k) => this.deps.label(k)).join('+') + (r.state === 'hidden' ? '(hidden)' : '')
      default: return r.type.toLowerCase().replace(/\s+/g, '-')
    }
  }

  private async sample(): Promise<void> {
    if (this.sampling) return
    this.sampling = true
    try {
      const rows = this.deps.collect()
      const sc = await this.deps.sidecar().catch(() => null)
      if (!this.timer) return
      const t = Date.now()

      // Overlay open/close/hide/show since the last sample.
      const states = new Map<string, 'open' | 'hidden'>()
      for (const r of rows) if (r.role === 'overlay') for (const k of r.keys) states.set(k, r.state ?? 'open')
      const changes: string[] = []
      for (const [k, s] of states) {
        const prev = this.lastStates.get(k)
        if (prev === undefined) changes.push(`opened ${this.deps.label(k)}${s === 'hidden' ? ' (hidden)' : ''}`)
        else if (prev !== s) changes.push(`${s === 'hidden' ? 'hid' : 'showed'} ${this.deps.label(k)}`)
      }
      for (const k of this.lastStates.keys()) if (!states.has(k)) changes.push(`closed ${this.deps.label(k)}`)
      // The first sample's "opened" list is the starting state, not a change.
      if (changes.length) this.write([`[${clock(t)}] EVENT ${this.samples.length ? '' : 'already open: '}${changes.join(', ')}`])
      this.lastStates = states

      const electron = rows.reduce((a, r) => a + this.mem(r), 0)
      const scMem = sc ? (this.hasPrivate ? sc.private_mb ?? sc.working_set_mb : sc.working_set_mb) : null
      const total = electron + (scMem ?? 0)
      const cpu = rows.reduce((a, r) => a + r.cpu, 0)
      const gpu = rows.filter((r) => r.role === 'helper' && /gpu/i.test(r.type)).reduce((a, r) => a + this.mem(r), 0)
      const open = rows.filter((r) => r.role === 'overlay' && r.state !== 'hidden')
      const keys = open.flatMap((r) => r.keys).sort()
      const perKey = new Map<string, number>()
      for (const r of open) if (r.keys.length === 1) perKey.set(r.keys[0], this.mem(r))
      this.samples.push({
        t, total, electron, sidecar: scMem, sidecarHeap: sc?.heap_inuse_mb ?? null, gpu, cpu,
        windows: open.length, keys, setId: keys.join(','), perKey,
      })

      const order: Record<PerfProcessRow['role'], number> = { main: 0, overlay: 1, trigger: 2, streamkit: 3, helper: 4 }
      const procs = [...rows]
        .sort((a, b) => order[a.role] - order[b.role] || this.mem(b) - this.mem(a))
        .map((r) => `${this.name(r)}=${r1(this.mem(r))}/${r1(r.cpu)}`)
      const scPart = sc
        ? `sidecar=${r1(scMem ?? 0)} (heap ${r1(sc.heap_inuse_mb)}, ${sc.num_goroutine} goroutines)`
        : 'sidecar=unavailable'
      this.write([
        `[${clock(t)}] total=${r1(total)} electron=${r1(electron)} ${scPart} cpu=${r1(cpu)}% overlays=${open.length} | ${procs.join(' ')}`,
      ])
    } finally {
      this.sampling = false
    }
  }

  private summary(): string[] {
    const s = this.samples
    const out = ['==================== SUMMARY ====================']
    if (s.length < 2) {
      out.push('Too few samples for a summary (record for at least a minute).')
      return out
    }
    const dur = s[s.length - 1].t - s[0].t
    out.push(`Duration: ${Math.round(dur / 60000)} min, ${s.length} samples`)

    // By number of open overlay windows.
    const byN = new Map<number, Sample[]>()
    for (const x of s) byN.set(x.windows, [...(byN.get(x.windows) ?? []), x])
    out.push('', 'By open overlay windows (averages):')
    out.push('  windows  samples   total MB  electron MB  sidecar MB   GPU MB   CPU %')
    for (const n of [...byN.keys()].sort((a, b) => a - b)) {
      const g = byN.get(n)!
      const scs = g.map((x) => x.sidecar).filter((v): v is number => v !== null)
      out.push(
        `  ${String(n).padStart(7)}  ${String(g.length).padStart(7)}  ${r1(avg(g.map((x) => x.total))).padStart(9)}` +
        `  ${r1(avg(g.map((x) => x.electron))).padStart(11)}  ${(scs.length ? r1(avg(scs)) : '-').padStart(10)}` +
        `  ${r1(avg(g.map((x) => x.gpu))).padStart(7)}  ${r1(avg(g.map((x) => x.cpu))).padStart(6)}`,
      )
    }
    const base = byN.get(0)
    out.push('')
    out.push(base ? `Baseline (no overlays): ${r1(avg(base.map((x) => x.total)))} MB` : 'Baseline: no samples with every overlay closed')

    // Least-squares fit of total vs open windows → cost per overlay window.
    if (byN.size >= 2) {
      const xs = s.map((x) => x.windows)
      const ys = s.map((x) => x.total)
      const mx = avg(xs)
      const my = avg(ys)
      let num = 0
      let den = 0
      for (let i = 0; i < xs.length; i++) {
        num += (xs[i] - mx) * (ys[i] - my)
        den += (xs[i] - mx) ** 2
      }
      if (den > 0) out.push(`Cost per open overlay window (fit across all samples): ${r1(num / den)} MB`)
    }

    const peak = s.reduce((a, b) => (b.total > a.total ? b : a))
    out.push(`Peak: ${r1(peak.total)} MB at ${clock(peak.t)} with ${peak.windows} overlay window(s)`)

    // Each overlay's own process, averaged while it was open on its own.
    const perKey = new Map<string, number[]>()
    for (const x of s) for (const [k, v] of x.perKey) perKey.set(k, [...(perKey.get(k) ?? []), v])
    if (perKey.size) {
      out.push('', 'Each overlay window\'s own process (average MB while open):')
      const rows = [...perKey].map(([k, v]) => [this.deps.label(k), avg(v)] as const).sort((a, b) => b[1] - a[1])
      for (const [k, v] of rows) out.push(`  ${k.padEnd(32)} ${r1(v).padStart(7)}`)
    }

    // Growth while the overlay set held steady: compare the first and last
    // minute of each stretch of at least STEADY_MIN_MS.
    const runs: Sample[][] = []
    let cur: Sample[] = []
    for (const x of s) {
      if (cur.length && cur[0].setId !== x.setId) {
        runs.push(cur)
        cur = []
      }
      cur.push(x)
    }
    if (cur.length) runs.push(cur)
    const steady = runs.filter((r) => r[r.length - 1].t - r[0].t >= STEADY_MIN_MS)
    out.push('', `Growth while the overlay set held steady (stretches of ${STEADY_MIN_MS / 60000}+ min):`)
    if (!steady.length) out.push('  none long enough; keep a layout open for 5+ minutes to check for leaks')
    for (const r of steady) {
      const t0 = r[0].t
      const t1 = r[r.length - 1].t
      const first = r.filter((x) => x.t - t0 <= EDGE_MS)
      const last = r.filter((x) => t1 - x.t <= EDGE_MS)
      const hours = (t1 - t0) / 3_600_000
      const d = avg(last.map((x) => x.total)) - avg(first.map((x) => x.total))
      const heap = (xs: Sample[]): number[] => xs.map((x) => x.sidecarHeap).filter((v): v is number => v !== null)
      const dh = avg(heap(last)) - avg(heap(first))
      out.push(
        `  ${clock(t0)}-${clock(t1)} (${Math.round((t1 - t0) / 60000)} min, ${r[0].windows} windows): ` +
        `total ${d >= 0 ? '+' : ''}${r1(d)} MB (${r1(d / hours)} MB/h), sidecar heap ${dh >= 0 ? '+' : ''}${r1(dh)} MB`,
      )
    }
    out.push('', 'Growth after a stretch is a signal to investigate, not proof of a leak (caches and GC settle over time).')
    return out
  }
}

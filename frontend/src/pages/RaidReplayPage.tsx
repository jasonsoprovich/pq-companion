import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity, Crosshair, Film, Flame, Gem, HeartPulse, Megaphone, MessageSquare, Pause, Play, Skull,
  Sparkles, Sunrise, Swords, Trash2, Zap,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { ZoneMapPanel } from '../components/maps/ZoneMapPanel'
import type { RaiderMarker, ReplayTarget } from '../components/maps/ZoneMap'
import { useCachedState } from '../hooks/useCachedState'
import { useDPSClassColors } from '../hooks/useDPSClassColors'
import { classColor } from '../lib/raidColors'
import {
  deleteRaidSession,
  getConfig,
  getRaidRecording,
  listRaidSessions,
  saveConfigChanges,
} from '../services/api'
import {
  EVENT_GROUPS, RETENTION_CHOICES, groupsAt, lastSeenAt, lowHPEvents, raidersAt, recentDeaths, recentKills,
  targetAt,
} from '../lib/raidReplay'
import type { Config } from '../types/config'
import type { RaidEvent, RaidEventKind, RaidRecording, RaidSession } from '../types/raidReplay'

// RaidReplayPage plays back raids recorded from Zeal's raid frames (backend
// package raidtrack) on the zone map: a session list, the map with every
// raider at the playhead (and a line to the recorder's target), a scrubber
// with the night's kills, deaths and CH chain calls, and the raid's groups.

// three.js is large; it loads only when the 3D view is opened.
const ZoneMap3D = lazy(() => import('../components/maps/ZoneMap3D'))

const SPEEDS = [1, 4, 16, 60]

function fmtClock(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function fmtDuration(ms: number): string {
  const m = Math.round(ms / 60000)
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
}

function sessionDate(s: RaidSession): string {
  return new Date(s.started_at).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
}

function RecordingSettings(): React.ReactElement {
  const [cfg, setCfg] = useState<Config | null>(null)
  useEffect(() => {
    getConfig().then(setCfg).catch(() => {})
  }, [])
  if (!cfg) return <div className="h-5" />
  const rr = cfg.raid_replay ?? { disabled: false, retention_days: 30 }
  const save = (next: { disabled: boolean; retention_days: number }): void => {
    saveConfigChanges({ ...cfg, raid_replay: next }, cfg).then(setCfg).catch(() => {})
  }
  return (
    <div className="flex flex-col gap-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={!rr.disabled}
          onChange={(e) => save({ ...rr, disabled: !e.target.checked })}
        />
        Record raids
      </label>
      <label className="flex items-center gap-2">
        Keep
        <select
          className="rounded border bg-transparent px-1 py-0.5"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-foreground)' }}
          value={rr.retention_days}
          onChange={(e) => save({ ...rr, retention_days: Number(e.target.value) })}
        >
          {RETENTION_CHOICES.map((d) => <option key={d} value={d}>{d} days</option>)}
          <option value={-1}>forever</option>
        </select>
      </label>
    </div>
  )
}

function RaiderRow({ name, color, focused, onFocus, right, compact }: {
  name: string
  color: string
  focused: boolean
  onFocus: () => void
  right: string
  compact?: boolean
}): React.ReactElement {
  return (
    <button
      className={`flex w-full items-center gap-2 py-0.5 text-left text-xs ${compact ? 'px-2' : 'px-3'}`}
      style={{ backgroundColor: focused ? 'var(--color-surface-2)' : 'transparent' }}
      onClick={onFocus}
      title={focused ? 'Stop highlighting' : 'Highlight on the map'}
    >
      <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: color }} />
      <span className="min-w-0 flex-1 truncate">{name}</span>
      <span className="tabular-nums" style={{ color: 'var(--color-muted)' }}>{right}</span>
    </button>
  )
}

function SessionRow({ s, selected, onSelect, onDelete }: {
  s: RaidSession
  selected: boolean
  onSelect: () => void
  onDelete: () => void
}): React.ReactElement {
  const start = Date.parse(s.started_at)
  const end = Date.parse(s.ended_at)
  return (
    <div
      className="group flex cursor-pointer items-start gap-2 px-3 py-2 text-left"
      style={{
        backgroundColor: selected ? 'var(--color-surface-2)' : 'transparent',
        borderLeft: selected ? '2px solid var(--color-primary)' : '2px solid transparent',
      }}
      onClick={onSelect}
    >
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm" style={{ color: selected ? 'var(--color-primary)' : 'var(--color-foreground)' }}>
          {s.zone_name || s.zone}
        </div>
        <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {sessionDate(s)} · {fmtClock(start).slice(0, 5)} · {fmtDuration(end - start)}
        </div>
        <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {s.peak_members} raiders · {s.kills} kills · {s.deaths} deaths · {s.character}
        </div>
      </div>
      <button
        className="opacity-0 transition-opacity group-hover:opacity-100"
        title="Delete this recording"
        onClick={(e) => {
          e.stopPropagation()
          onDelete()
        }}
      >
        <Trash2 size={13} style={{ color: 'var(--color-muted)' }} />
      </button>
    </div>
  )
}

// How each kind of event looks: its colour and icon in the list, and its
// lane on the timeline: tall for kills, deaths and rezzes, short for
// mechanics, low health and loot, and a faint strip along the top for CH
// calls and raid calls, which come all night and would bury the rest.
const EVENT_STYLE: Record<RaidEventKind, { color: string; icon: LucideIcon; lane: 'tall' | 'short' | 'faint' | 'none' }> = {
  kill: { color: '#c9a84c', icon: Swords, lane: 'tall' },
  death: { color: '#ef4444', icon: Skull, lane: 'tall' },
  ch: { color: '#60a5fa', icon: HeartPulse, lane: 'faint' },
  // Lands aren't drawn: each sits right after its call.
  ch_land: { color: '#60a5fa', icon: HeartPulse, lane: 'none' },
  enrage: { color: '#f97316', icon: Flame, lane: 'short' },
  rampage: { color: '#f59e0b', icon: Zap, lane: 'short' },
  flurry: { color: '#f59e0b', icon: Zap, lane: 'short' },
  cast: { color: '#c084fc', icon: Sparkles, lane: 'short' },
  npc_shout: { color: '#f472b6', icon: Megaphone, lane: 'short' },
  call: { color: '#94a3b8', icon: MessageSquare, lane: 'faint' },
  loot: { color: '#34d399', icon: Gem, lane: 'short' },
  rez: { color: '#a3e635', icon: Sunrise, lane: 'tall' },
  low_hp: { color: '#fb7185', icon: Activity, lane: 'short' },
}

// eventText is an event's line in the list: what happened, then detail.
function eventText(e: RaidEvent): [string, string] {
  const o = e.other ?? ''
  switch (e.kind) {
    case 'kill': return [e.name, o && `by ${o}`]
    case 'death': return [e.name, o && `by ${o}`]
    case 'ch': return [`CH ${e.name}`, o && `from ${o}`]
    case 'ch_land': return [`CH landed on ${e.name}`, '']
    case 'enrage': return [`${e.name} enraged`, '']
    case 'rampage': return [`${e.name} rampage`, o && `on ${o}`]
    case 'flurry': return [`${e.name} flurry`, o && `on ${o}`]
    case 'cast': return [`${e.name} begins casting`, '']
    case 'npc_shout': return [`${e.name}:`, `“${o}”`]
    case 'call': return [`${e.name}:`, o]
    case 'loot': return [`${e.name} looted`, o]
    case 'rez': return [`${e.name} rezzed`, '']
    case 'low_hp': return [`${e.name} low health`, o]
  }
}

function Scrubber({ rec, events, t, onSeek }: {
  rec: RaidRecording
  events: RaidEvent[]
  t: number
  onSeek: (t: number) => void
}): React.ReactElement {
  const start = rec.frames[0]?.t ?? 0
  const end = rec.frames[rec.frames.length - 1]?.t ?? start
  const span = Math.max(1, end - start)
  const lanes = { tall: [3, 7, 1], short: [5, 4, 0.7], faint: [0, 2, 0.35] } as const
  return (
    <div className="relative h-7">
      {events.map((e, i) => {
        const { lane, color } = EVENT_STYLE[e.kind]
        if (lane === 'none') return null
        const [top, height, opacity] = lanes[lane]
        return (
          <div
            key={i}
            className="pointer-events-none absolute w-px"
            style={{ left: `${((e.t - start) / span) * 100}%`, top, height, opacity, backgroundColor: color }}
          />
        )
      })}
      <input
        type="range"
        className="absolute inset-x-0 bottom-0 w-full"
        min={start}
        max={end}
        step={1000}
        value={t}
        onChange={(e) => onSeek(Number(e.target.value))}
      />
    </div>
  )
}

export default function RaidReplayPage(): React.ReactElement {
  const palette = useDPSClassColors()
  const [sessions, setSessions] = useState<RaidSession[] | null>(null)
  const [selectedId, setSelectedId] = useCachedState<number | null>('raidReplay.session', null)
  const [rec, setRec] = useState<RaidRecording | null>(null)
  const [loading, setLoading] = useState(false)
  const [t, setT] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useCachedState('raidReplay.speed', 16)
  const [focus, setFocus] = useState<number | null>(null)
  const [view3D, setView3D] = useCachedState('raidReplay.view3D', false)
  const [side, setSide] = useCachedState<'list' | 'groups'>('raidReplay.side', 'list')
  // Which event groups (EVENT_GROUPS keys) the timeline and list show.
  const [shownGroups, setShownGroups] = useCachedState<string[]>(
    'raidReplay.eventGroups',
    EVENT_GROUPS.map((g) => g.key),
  )
  // Why 3D fell back to 2D for this session, if it did.
  const [no3D, setNo3D] = useState<string | null>(null)
  useEffect(() => setNo3D(null), [selectedId])

  const refresh = useCallback(() => {
    listRaidSessions().then(setSessions).catch(() => setSessions([]))
  }, [])
  useEffect(refresh, [refresh])

  useEffect(() => {
    setPlaying(false)
    setFocus(null)
    if (selectedId == null) {
      setRec(null)
      return
    }
    setLoading(true)
    let live = true
    getRaidRecording(selectedId)
      .then((r) => {
        if (!live) return
        setRec(r)
        setT(r.frames[0]?.t ?? 0)
      })
      .catch(() => live && setRec(null))
      .finally(() => live && setLoading(false))
    return () => {
      live = false
    }
  }, [selectedId])

  const start = rec?.frames[0]?.t ?? 0
  const end = rec?.frames[rec.frames.length - 1]?.t ?? 0

  // Playback ticks at 20 fps; the map redraws on each new playhead.
  const last = useRef(0)
  useEffect(() => {
    if (!playing || !rec) return
    last.current = performance.now()
    const id = setInterval(() => {
      const now = performance.now()
      const dt = now - last.current
      last.current = now
      setT((prev) => {
        const next = prev + dt * speed
        if (next >= end) {
          setPlaying(false)
          return end
        }
        return next
      })
    }, 50)
    return () => clearInterval(id)
  }, [playing, rec, speed, end])

  const byIndex = useMemo(() => {
    const m = new Map<number, RaidRecording['members'][number]>()
    for (const mem of rec?.members ?? []) m.set(mem.index, mem)
    return m
  }, [rec])
  const indexByName = useMemo(() => {
    const m = new Map<string, number>()
    for (const mem of rec?.members ?? []) m.set(mem.name.toLowerCase(), mem.index)
    return m
  }, [rec])

  const present = useMemo(() => (rec ? raidersAt(rec.frames, t) : []), [rec, t])

  const raiders = useMemo((): RaiderMarker[] => {
    if (!rec) return []
    const out: RaiderMarker[] = []
    const shown = new Set<number>()
    // The recorder and their group are "mine", for Labels: Group.
    const meIdx = indexByName.get(rec.session.character.toLowerCase())
    const myGroup = present.find((r) => r.index === meIdx)?.group ?? 0
    const mine = (idx: number, group: number): boolean => idx === meIdx || (myGroup > 0 && group === myGroup)
    // Raiders who died recently are drawn as a cross where they fell, even
    // after they've left the zone's frames.
    const dead = new Set<number>()
    for (const e of recentDeaths(rec.events, t)) {
      const idx = indexByName.get(e.name.toLowerCase())
      if (idx === undefined || dead.has(idx)) continue
      const at = lastSeenAt(rec.frames, e.t, idx)
      if (!at) continue
      dead.add(idx)
      const mem = byIndex.get(idx)
      out.push({
        x: -at.x, y: -at.y, z: at.z, heading: at.heading,
        color: classColor(palette, mem?.class ?? 0), label: mem?.name ?? '?',
        dead: true, focus: focus === idx, mine: idx === meIdx,
      })
      shown.add(idx)
    }
    for (const r of present) {
      if (shown.has(r.index)) continue
      const mem = byIndex.get(r.index)
      out.push({
        // Map space is the negated game position (see playerpos).
        x: -r.x, y: -r.y, z: r.z, heading: r.heading,
        color: classColor(palette, mem?.class ?? 0), label: mem?.name ?? '?',
        focus: focus === r.index, mine: mine(r.index, r.group),
      })
    }
    return out
  }, [rec, t, present, byIndex, indexByName, palette, focus])

  // The recorder's target: a line from them to it, ending on the raider when
  // they targeted one (a heal target), else at the mob's own position when
  // Zeal reported it.
  const target = useMemo(() => (rec ? targetAt(rec.frames, t) : null), [rec, t])
  const replayTarget = useMemo((): ReplayTarget | null => {
    if (!rec || !target) return null
    const meIdx = indexByName.get(rec.session.character.toLowerCase())
    const me = present.find((r) => r.index === meIdx)
    if (!me) return null
    const from = { x: -me.x, y: -me.y, z: me.z }
    const tIdx = indexByName.get(target.n.toLowerCase())
    const onRaider = present.find((r) => r.index === tIdx)
    if (onRaider) {
      if (onRaider.index === me.index) return null
      return { from, to: { x: -onRaider.x, y: -onRaider.y, z: onRaider.z }, label: target.n, onRaider: true }
    }
    if (!target.l) return null
    return { from, to: { x: -target.l[0], y: -target.l[1], z: target.l[2] }, label: target.n, onRaider: false }
  }, [rec, target, present, indexByName])

  const groups = useMemo(() => groupsAt(present), [present])

  // Kills placed where the mob was, for the 3D view.
  const killPins = useMemo(
    () =>
      rec
        ? recentKills(rec.frames, rec.events, t).map((k) => ({
            x: -k.x, y: -k.y, z: k.z, color: '#c9a84c', label: `✕ ${k.name}`,
          }))
        : [],
    [rec, t],
  )

  const roster = useMemo(() => {
    const rows = present.map((r) => ({ ...r, mem: byIndex.get(r.index) }))
    rows.sort((a, b) => (a.group || 99) - (b.group || 99) || (a.mem?.name ?? '').localeCompare(b.mem?.name ?? ''))
    return rows
  }, [present, byIndex])

  // The recording's events plus low-health marks made from its frames, in
  // time order, cut to the groups shown.
  const events = useMemo(() => {
    if (!rec) return []
    const kinds = new Set(EVENT_GROUPS.filter((g) => shownGroups.includes(g.key)).flatMap((g) => g.kinds))
    return [...rec.events, ...lowHPEvents(rec.frames, rec.members)]
      .filter((e) => kinds.has(e.kind))
      .sort((a, b) => a.t - b.t)
  }, [rec, shownGroups])

  const nearEvents = useMemo(
    () => events.filter((e) => e.t <= t).slice(-40).reverse(),
    [events, t],
  )

  const remove = (id: number): void => {
    deleteRaidSession(id)
      .then(() => {
        if (selectedId === id) setSelectedId(null)
        refresh()
      })
      .catch(() => {})
  }

  return (
    <div className="flex h-full">
      <div className="flex w-64 shrink-0 flex-col border-r" style={{ borderColor: 'var(--color-border)' }}>
        <div className="flex flex-col gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex items-center gap-2">
            <Film size={15} style={{ color: 'var(--color-primary)' }} />
            <h1 className="text-sm font-semibold">Raid Replay</h1>
          </div>
          <RecordingSettings />
        </div>
        <div className="flex-1 overflow-y-auto">
          {sessions === null ? null : sessions.length === 0 ? (
            <p className="p-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No raids recorded yet. With Zeal running, raids are recorded
              automatically while you're in a raid.
            </p>
          ) : (
            sessions.map((s) => (
              <SessionRow
                key={s.id}
                s={s}
                selected={s.id === selectedId}
                onSelect={() => setSelectedId(s.id)}
                onDelete={() => remove(s.id)}
              />
            ))
          )}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col gap-2 p-3">
        {!rec ? (
          <div className="flex flex-1 items-center justify-center text-sm" style={{ color: 'var(--color-muted)' }}>
            {loading ? 'Loading…' : 'Pick a recording.'}
          </div>
        ) : (
          <>
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-semibold">{rec.session.zone_name || rec.session.zone}</h2>
              <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                {sessionDate(rec.session)} · {rec.session.character}
              </span>
              {target && (
                <span className="flex items-center gap-1 text-xs" style={{ color: '#f87171' }} title={`${rec.session.character}'s target`}>
                  <Crosshair size={11} /> {target.n}
                </span>
              )}
              <div className="ml-auto flex overflow-hidden rounded border text-[11px]" style={{ borderColor: 'var(--color-border)' }}>
                {([false, true] as const).map((v) => (
                  <button
                    key={String(v)}
                    className="px-2 py-0.5"
                    style={{
                      backgroundColor: view3D === v ? 'var(--color-surface-2)' : 'transparent',
                      color: view3D === v ? 'var(--color-primary)' : 'var(--color-muted)',
                    }}
                    onClick={() => {
                      setNo3D(null)
                      setView3D(v)
                    }}
                  >
                    {v ? '3D' : '2D'}
                  </button>
                ))}
              </div>
            </div>
            {view3D && no3D && (
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{no3D} Showing the 2D map.</p>
            )}
            <div className="min-h-0 flex-1">
              {view3D && !no3D ? (
                <Suspense fallback={<div className="p-4 text-sm" style={{ color: 'var(--color-muted)' }}>Loading 3D…</div>}>
                  <ZoneMap3D
                    key={rec.session.id}
                    zone={rec.session.zone}
                    markers={raiders}
                    target={replayTarget}
                    pins={killPins}
                    onUnavailable={(why) =>
                      setNo3D(
                        why === 'webgl'
                          ? '3D needs WebGL, which isn’t available here.'
                          : why === 'no-mesh'
                            ? `No ${rec.session.zone}.s3d in your EverQuest folder.`
                            : 'The 3D zone couldn’t be loaded.',
                      )
                    }
                  />
                </Suspense>
              ) : (
                <ZoneMapPanel
                  key={rec.session.id}
                  zoneShortName={rec.session.zone}
                  showZoneButton={false}
                  replay
                  raiders={raiders}
                  replayTarget={replayTarget}
                />
              )}
            </div>
            <div className="flex items-center gap-3">
              <button
                className="rounded border p-1.5"
                style={{ borderColor: 'var(--color-border)' }}
                title={playing ? 'Pause' : 'Play'}
                onClick={() => {
                  if (!playing && t >= end) setT(start)
                  setPlaying(!playing)
                }}
              >
                {playing ? <Pause size={14} /> : <Play size={14} />}
              </button>
              <div className="flex gap-1">
                {SPEEDS.map((s) => (
                  <button
                    key={s}
                    className="rounded px-1.5 py-0.5 text-[11px] tabular-nums"
                    style={{
                      backgroundColor: s === speed ? 'var(--color-surface-2)' : 'transparent',
                      color: s === speed ? 'var(--color-primary)' : 'var(--color-muted)',
                    }}
                    onClick={() => setSpeed(s)}
                  >
                    {s}×
                  </button>
                ))}
              </div>
              <span className="w-20 text-xs tabular-nums" style={{ color: 'var(--color-foreground)' }}>
                {fmtClock(t)}
              </span>
              <div className="min-w-0 flex-1">
                <Scrubber rec={rec} events={events} t={t} onSeek={setT} />
              </div>
              <span className="text-[11px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
                {fmtDuration(end - start)}
              </span>
            </div>
          </>
        )}
      </div>

      {rec && (
        <div className="flex w-56 shrink-0 flex-col border-l" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex items-center border-b px-3 py-2 text-xs font-semibold" style={{ borderColor: 'var(--color-border)' }}>
            In zone ({roster.length})
            <div className="ml-auto flex overflow-hidden rounded border text-[10px] font-normal" style={{ borderColor: 'var(--color-border)' }}>
              {(['list', 'groups'] as const).map((v) => (
                <button
                  key={v}
                  className="px-1.5 py-0.5"
                  style={{
                    backgroundColor: side === v ? 'var(--color-surface-2)' : 'transparent',
                    color: side === v ? 'var(--color-primary)' : 'var(--color-muted)',
                  }}
                  onClick={() => setSide(v)}
                >
                  {v === 'list' ? 'List' : 'Groups'}
                </button>
              ))}
            </div>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto py-1">
            {side === 'list'
              ? roster.map((r) => (
                  <RaiderRow
                    key={r.index}
                    name={r.mem?.name ?? '?'}
                    color={classColor(palette, r.mem?.class ?? 0)}
                    focused={focus === r.index}
                    onFocus={() => setFocus(focus === r.index ? null : r.index)}
                    right={r.group > 0 ? `G${r.group}` : ''}
                  />
                ))
              : groups.map(({ group, indexes }) => (
                  <div
                    key={group}
                    className="mx-2 my-1.5 overflow-hidden rounded-md"
                    style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}
                  >
                    <div
                      className="flex items-center justify-between px-2 py-1"
                      style={{ borderBottom: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)' }}
                    >
                      <span className="text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-primary)' }}>
                        {group > 0 ? `Group ${group}` : 'Ungrouped'}
                      </span>
                      <span className="text-[10px] tabular-nums" style={{ color: 'var(--color-muted-foreground)' }}>
                        {group > 0 ? `${indexes.length}/6` : indexes.length}
                      </span>
                    </div>
                    {indexes
                      .map((i) => byIndex.get(i))
                      .sort((a, b) => (a?.name ?? '').localeCompare(b?.name ?? ''))
                      .map((mem) => mem && (
                        <RaiderRow
                          key={mem.index}
                          name={mem.name}
                          color={classColor(palette, mem.class)}
                          focused={focus === mem.index}
                          onFocus={() => setFocus(focus === mem.index ? null : mem.index)}
                          right={mem.level > 0 ? String(mem.level) : ''}
                          compact
                        />
                      ))}
                  </div>
                ))}
          </div>
          <div className="flex items-center border-y px-3 py-2 text-xs font-semibold" style={{ borderColor: 'var(--color-border)' }}>
            Events
          </div>
          <div className="flex flex-wrap gap-1 px-3 pt-1.5">
            {EVENT_GROUPS.map((g) => {
              const on = shownGroups.includes(g.key)
              return (
                <button
                  key={g.key}
                  onClick={() =>
                    setShownGroups(on ? shownGroups.filter((k) => k !== g.key) : [...shownGroups, g.key])
                  }
                  className="rounded border px-1.5 py-0.5 text-[10px]"
                  style={{
                    borderColor: on ? 'var(--color-primary)' : 'var(--color-border)',
                    color: on ? 'var(--color-primary)' : 'var(--color-muted)',
                  }}
                >
                  {g.label}
                </button>
              )
            })}
          </div>
          <div className="max-h-56 overflow-y-auto py-1">
            {nearEvents.length === 0 ? (
              <p className="px-3 text-[11px]" style={{ color: 'var(--color-muted)' }}>Nothing yet.</p>
            ) : (
              nearEvents.map((e, i) => (
                <button
                  key={`${e.t}-${i}`}
                  className="flex w-full items-start gap-1.5 px-3 py-0.5 text-left text-[11px]"
                  onClick={() => setT(e.t)}
                  title="Jump here"
                >
                  <EventIcon kind={e.kind} />
                  <span className="min-w-0 flex-1">
                    {eventText(e)[0]}
                    {eventText(e)[1] && <span style={{ color: 'var(--color-muted)' }}> {eventText(e)[1]}</span>}
                  </span>
                  <span className="tabular-nums" style={{ color: 'var(--color-muted)' }}>{fmtClock(e.t).slice(0, 5)}</span>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function EventIcon({ kind }: { kind: RaidEventKind }): React.ReactElement {
  const { icon: Icon, color } = EVENT_STYLE[kind]
  return <Icon size={11} className="mt-0.5 shrink-0" style={{ color }} />
}

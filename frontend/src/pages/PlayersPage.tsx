import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { UserSearch, RefreshCw, Trash2, AlertCircle, EyeOff, X, ArrowUp, ArrowDown, Swords, StickyNote, Bell, BellOff } from 'lucide-react'
import { listPlayers, deletePlayer, clearPlayers, getPlayerHistory, updatePlayerNote, updatePlayerManual, getConfig, updateConfig } from '../services/api'
import type { PlayerSighting, PlayerLevelHistoryEntry } from '../types/player'
import MissingLogNotice from '../components/MissingLogNotice'
import BackfillLink from '../components/BackfillLink'

// EQ class list — matches what /who emits. Used to drive the class filter chip
// row plus the "no class data yet" guard.
const CLASS_LIST = [
  'Bard',
  'Beastlord',
  'Cleric',
  'Druid',
  'Enchanter',
  'Magician',
  'Monk',
  'Necromancer',
  'Paladin',
  'Ranger',
  'Rogue',
  'Shadow Knight',
  'Shaman',
  'Warrior',
  'Wizard',
]

// EQ playable races (Velious-era Quarm) — drives the manual race dropdown for
// always-anonymous players the user wants to fill in by hand.
const RACE_LIST = [
  'Human',
  'Barbarian',
  'Erudite',
  'Wood Elf',
  'High Elf',
  'Dark Elf',
  'Half Elf',
  'Dwarf',
  'Troll',
  'Ogre',
  'Halfling',
  'Gnome',
  'Iksar',
  'Vah Shir',
]

// withEffective recomputes the effective_* fields locally after a manual edit
// so the UI updates instantly without a refetch. Mirrors the backend rule:
// a /who-discovered value always wins; manual only fills the gaps.
function withEffective(p: PlayerSighting): PlayerSighting {
  return {
    ...p,
    effective_class: p.class || p.manual_class,
    effective_level: p.last_seen_level > 0 ? p.last_seen_level : p.manual_level,
    effective_race: p.race || p.manual_race,
  }
}

function formatRelative(unix: number): string {
  if (!unix) return '—'
  const diffMs = Date.now() - unix * 1000
  if (diffMs < 60_000) return 'just now'
  const mins = Math.floor(diffMs / 60_000)
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(unix * 1000).toLocaleDateString()
}

function PlayerDetail({
  player,
  onClose,
  onDeleted,
  onChanged,
  onManualChanged,
}: {
  player: PlayerSighting
  onClose: () => void
  onDeleted: () => void
  onChanged: (note: string, pvp: boolean) => void
  onManualChanged: (manual: { class: string; level: number; race: string }) => void
}): React.ReactElement {
  const [history, setHistory] = useState<PlayerLevelHistoryEntry[] | null>(null)
  const [historyErr, setHistoryErr] = useState<string | null>(null)
  const [note, setNote] = useState(player.note)
  const [pvp, setPvp] = useState(player.pvp)
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [mClass, setMClass] = useState(player.manual_class)
  const [mLevel, setMLevel] = useState(player.manual_level ? String(player.manual_level) : '')
  const [mRace, setMRace] = useState(player.manual_race)
  const [manualSave, setManualSave] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')

  useEscapeToClose(onClose)

  // Re-seed the editor when the user clicks a different player row.
  useEffect(() => {
    setNote(player.note)
    setPvp(player.pvp)
    setSaveState('idle')
    setMClass(player.manual_class)
    setMLevel(player.manual_level ? String(player.manual_level) : '')
    setMRace(player.manual_race)
    setManualSave('idle')
  }, [player.name]) // eslint-disable-line react-hooks/exhaustive-deps

  async function saveNote(nextNote: string, nextPvp: boolean) {
    setSaveState('saving')
    try {
      await updatePlayerNote(player.name, nextNote, nextPvp)
      setSaveState('saved')
      onChanged(nextNote, nextPvp)
    } catch {
      setSaveState('error')
    }
  }

  function togglePvp() {
    const next = !pvp
    setPvp(next)
    void saveNote(note, next)
  }

  async function saveManual(next: { class: string; level: string; race: string }) {
    const levelNum = next.level === '' ? 0 : Math.max(0, Math.min(65, parseInt(next.level, 10) || 0))
    setManualSave('saving')
    try {
      await updatePlayerManual(player.name, { class: next.class, level: levelNum, race: next.race })
      setManualSave('saved')
      onManualChanged({ class: next.class, level: levelNum, race: next.race })
    } catch {
      setManualSave('error')
    }
  }

  // Whether a manual override is actually driving any displayed value (i.e.
  // /who has never revealed that field). Drives the "in use" hint.
  const manualClassInUse = !player.class && !!player.manual_class
  const manualLevelInUse = player.last_seen_level === 0 && player.manual_level > 0
  const manualRaceInUse = !player.race && !!player.manual_race

  useEffect(() => {
    let cancelled = false
    getPlayerHistory(player.name)
      .then((r) => {
        if (!cancelled) setHistory(r.history)
      })
      .catch((err: Error) => {
        if (!cancelled) setHistoryErr(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [player.name])

  async function handleDelete() {
    try {
      await deletePlayer(player.name)
      onDeleted()
      onClose()
    } catch {
      // Surface via the existing error toast pattern when we have one.
    }
  }

  return (
    <div
      className="rounded-lg border p-4 mb-3"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      <div className="flex items-start justify-between gap-2 mb-3">
        <div>
          <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {player.name}
            {player.last_anonymous && (
              <EyeOff size={12} className="inline-block ml-2" style={{ color: 'var(--color-muted)' }} />
            )}
            {pvp && <PvpBadge className="ml-2 align-middle" />}
          </h2>
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            {player.effective_level > 0 ? `Level ${player.effective_level}` : 'Level unknown'}
            {player.effective_class ? ` ${player.effective_class}` : ''}
            {player.effective_race ? ` · ${player.effective_race}` : ''}
            {player.guild ? ` · <${player.guild}>` : ''}
            {(manualClassInUse || manualLevelInUse || manualRaceInUse) && (
              <span className="ml-1.5 italic">(manual)</span>
            )}
          </p>
          <p className="text-[11px] mt-0.5" style={{ color: 'var(--color-muted)' }}>
            Seen {player.sightings_count} time{player.sightings_count === 1 ? '' : 's'} · last in{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{player.last_seen_zone || 'unknown zone'}</span>{' '}
            {formatRelative(player.last_seen_at)}
          </p>
          {(player.tell_count > 0 || player.group_count > 0) && (
            <p className="text-[11px] mt-0.5" style={{ color: 'var(--color-muted)' }}>
              {player.group_count > 0 && (
                <>
                  Grouped {player.group_count} time{player.group_count === 1 ? '' : 's'} ·{' '}
                  last {formatRelative(player.last_grouped_at)}
                </>
              )}
              {player.group_count > 0 && player.tell_count > 0 && ' · '}
              {player.tell_count > 0 && (
                <>
                  {player.tell_count} tell{player.tell_count === 1 ? '' : 's'} ·{' '}
                  last {formatRelative(player.last_tell_at)}
                </>
              )}
            </p>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={handleDelete}
            title="Remove this player from the database"
            className="rounded p-1.5"
            style={{ color: 'var(--color-danger)' }}
          >
            <Trash2 size={14} />
          </button>
          <button onClick={onClose} className="rounded p-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
            <X size={14} />
          </button>
        </div>
      </div>

      {/* Notes + PVP flag */}
      <div className="mb-3">
        <div className="flex items-center justify-between mb-1.5">
          <p className="text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Notes
            {saveState === 'saving' && <span className="ml-2 normal-case tracking-normal font-normal">saving…</span>}
            {saveState === 'saved' && <span className="ml-2 normal-case tracking-normal font-normal">saved</span>}
            {saveState === 'error' && (
              <span className="ml-2 normal-case tracking-normal font-normal" style={{ color: 'var(--color-danger)' }}>
                save failed
              </span>
            )}
          </p>
          <button
            onClick={togglePvp}
            title="Flag this player as PVP — you'll get a sound and on-screen warning whenever they show up in a /who"
            className="flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded"
            style={{
              backgroundColor: pvp ? 'rgba(239, 68, 68, 0.15)' : 'var(--color-surface-2)',
              color: pvp ? 'var(--color-danger)' : 'var(--color-muted)',
              border: `1px solid ${pvp ? 'var(--color-danger)' : 'var(--color-border)'}`,
            }}
          >
            <Swords size={11} />
            {pvp ? 'PVP flagged' : 'Flag as PVP'}
          </button>
        </div>
        <textarea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          onBlur={() => {
            if (note !== player.note) void saveNote(note, pvp)
          }}
          rows={2}
          placeholder="Cool cat, new to EQ, gave him a dagger at level 5…"
          className="w-full text-xs rounded px-2 py-1.5 outline-none resize-y"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        />
      </div>

      {/* Manual class/level/race override for permanently-anonymous players */}
      <div className="mb-3">
        <div className="flex items-center justify-between mb-1.5">
          <p className="text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Known info
            {manualSave === 'saving' && <span className="ml-2 normal-case tracking-normal font-normal">saving…</span>}
            {manualSave === 'saved' && <span className="ml-2 normal-case tracking-normal font-normal">saved</span>}
            {manualSave === 'error' && (
              <span className="ml-2 normal-case tracking-normal font-normal" style={{ color: 'var(--color-danger)' }}>
                save failed
              </span>
            )}
          </p>
        </div>
        <div className="grid gap-2" style={{ gridTemplateColumns: '1fr 70px 1fr' }}>
          <label className="flex flex-col gap-0.5">
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>Class</span>
            <select
              value={mClass}
              onChange={(e) => { setMClass(e.target.value); void saveManual({ class: e.target.value, level: mLevel, race: mRace }) }}
              className="text-xs rounded px-1.5 py-1 outline-none"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            >
              <option value="">—</option>
              {CLASS_LIST.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
          <label className="flex flex-col gap-0.5">
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>Level</span>
            <input
              type="number"
              min={0}
              max={65}
              value={mLevel}
              onChange={(e) => setMLevel(e.target.value)}
              onBlur={() => { if (mLevel !== (player.manual_level ? String(player.manual_level) : '')) void saveManual({ class: mClass, level: mLevel, race: mRace }) }}
              className="text-xs rounded px-1.5 py-1 outline-none"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            />
          </label>
          <label className="flex flex-col gap-0.5">
            <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>Race</span>
            <select
              value={mRace}
              onChange={(e) => { setMRace(e.target.value); void saveManual({ class: mClass, level: mLevel, race: e.target.value }) }}
              className="text-xs rounded px-1.5 py-1 outline-none"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            >
              <option value="">—</option>
              {RACE_LIST.map((rr) => <option key={rr} value={rr}>{rr}</option>)}
            </select>
          </label>
        </div>
        <p className="text-[10px] mt-1" style={{ color: 'var(--color-muted)' }}>
          For friends/guildmates who stay anonymous. A real /who sighting always
          wins; these only apply while a field is unknown — and they drive DPS
          meter colors.
        </p>
      </div>

      <p className="text-[10px] font-semibold uppercase tracking-widest mb-1.5" style={{ color: 'var(--color-muted)' }}>
        Level history
      </p>
      {historyErr && (
        <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
          {historyErr}
        </p>
      )}
      {history === null && !historyErr && (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Loading…
        </p>
      )}
      {history !== null && history.length === 0 && (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          No level data captured yet. Future non-anonymous /who sightings will add rows here.
        </p>
      )}
      {history !== null && history.length > 0 && (
        <div
          className="grid gap-x-3 text-[11px] mt-1"
          style={{ gridTemplateColumns: 'auto auto 1fr auto', color: 'var(--color-muted-foreground)' }}
        >
          <span className="font-semibold" style={{ color: 'var(--color-muted)' }}>Level</span>
          <span className="font-semibold" style={{ color: 'var(--color-muted)' }}>Class</span>
          <span className="font-semibold" style={{ color: 'var(--color-muted)' }}>Zone</span>
          <span className="font-semibold text-right" style={{ color: 'var(--color-muted)' }}>When</span>
          {history.map((h) => (
            <React.Fragment key={h.id}>
              <span style={{ color: 'var(--color-foreground)' }}>{h.level}</span>
              <span>{h.class || '—'}</span>
              <span className="truncate">{h.zone || '—'}</span>
              <span className="text-right tabular-nums">{formatRelative(h.observed_at)}</span>
            </React.Fragment>
          ))}
        </div>
      )}
    </div>
  )
}

// SortField names a sortable column. 'none' means "use the API's default
// order" (last_seen_at DESC) — i.e. nothing the user has actively chosen.
type SortField = 'none' | 'name' | 'level' | 'class' | 'guild' | 'zone' | 'sightings' | 'seen'
type SortDir = 'asc' | 'desc'

// PLAYER_PAGE_SIZE rows per fetch — the list paginates with a "Show more"
// button (like the items search pane) instead of silently capping. Search
// and all filters run server-side over the whole database regardless of how
// many rows are loaded.
const PLAYER_PAGE_SIZE = 500

export default function PlayersPage(): React.ReactElement {
  const [players, setPlayers] = useState<PlayerSighting[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [classFilter, setClassFilter] = useState<string>('')
  const [zoneFilter, setZoneFilter] = useState<string>('')
  const [guildFilter, setGuildFilter] = useState<string>('')
  const [pvpOnly, setPvpOnly] = useState(false)
  const [selected, setSelected] = useState<PlayerSighting | null>(null)
  const [sortField, setSortField] = useState<SortField>('none')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  // PVP warning toggle — backed by preferences.pvp_warning_disabled in
  // config.yaml; the backend checks it at fire time so flipping it here
  // takes effect immediately. null until the config loads.
  const [pvpWarningOn, setPvpWarningOn] = useState<boolean | null>(null)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((cfg) => {
        if (!cancelled) setPvpWarningOn(!cfg.preferences.pvp_warning_disabled)
      })
      .catch(() => {
        // Leave the toggle hidden; the warning itself still follows config.
      })
    return () => {
      cancelled = true
    }
  }, [])

  async function togglePvpWarning() {
    if (pvpWarningOn === null) return
    const next = !pvpWarningOn
    setPvpWarningOn(next)
    try {
      const cfg = await getConfig()
      await updateConfig({
        ...cfg,
        preferences: { ...cfg.preferences, pvp_warning_disabled: !next },
      })
    } catch {
      setPvpWarningOn(!next) // revert on failure
    }
  }

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listPlayers({ search, class: classFilter, zone: zoneFilter, guild: guildFilter, pvp: pvpOnly, limit: PLAYER_PAGE_SIZE })
      .then((r) => {
        setPlayers(r.players)
        // Fall back to the page length when talking to a backend that
        // predates the total field (stale dev sidecar).
        setTotal(r.total ?? r.players.length)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [search, classFilter, zoneFilter, guildFilter, pvpOnly])

  const loadMore = useCallback(() => {
    setLoadingMore(true)
    listPlayers({
      search,
      class: classFilter,
      zone: zoneFilter,
      guild: guildFilter,
      pvp: pvpOnly,
      limit: PLAYER_PAGE_SIZE,
      offset: players.length,
    })
      .then((r) => {
        setPlayers((prev) => [...prev, ...r.players])
        setTotal((t) => r.total ?? t)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoadingMore(false))
  }, [search, classFilter, zoneFilter, guildFilter, pvpOnly, players.length])

  useEffect(() => {
    load()
  }, [load])

  // Derive unique zone/guild lists from the loaded set so each filter is a
  // real dropdown of values actually present.
  const zoneOptions = useMemo(() => {
    const zones = new Set<string>()
    players.forEach((p) => { if (p.last_seen_zone) zones.add(p.last_seen_zone) })
    return Array.from(zones).sort()
  }, [players])
  const guildOptions = useMemo(() => {
    const guilds = new Set<string>()
    players.forEach((p) => { if (p.guild) guilds.add(p.guild) })
    return Array.from(guilds).sort()
  }, [players])

  // Apply client-side sort on top of the loaded rows (the PVP filter runs
  // server-side with the other filters). When sortField is 'none' the API's
  // default last-seen ordering wins.
  const sortedPlayers = useMemo(() => {
    if (sortField === 'none') return players
    const direction = sortDir === 'asc' ? 1 : -1
    const cmp = (a: PlayerSighting, b: PlayerSighting): number => {
      switch (sortField) {
        case 'name':
          return a.name.localeCompare(b.name) * direction
        case 'level':
          return (a.effective_level - b.effective_level) * direction
        case 'class':
          return (a.effective_class || '').localeCompare(b.effective_class || '') * direction
        case 'guild':
          return (a.guild || '').localeCompare(b.guild || '') * direction
        case 'zone':
          return (a.last_seen_zone || '').localeCompare(b.last_seen_zone || '') * direction
        case 'sightings':
          return (a.sightings_count - b.sightings_count) * direction
        case 'seen':
          return (a.last_seen_at - b.last_seen_at) * direction
        default:
          return 0
      }
    }
    return [...players].sort(cmp)
  }, [players, sortField, sortDir])

  function handleSort(field: SortField): void {
    if (sortField === field) {
      // Same column — flip direction. A third click on the same column
      // resets to the API default order so the user can recover the
      // last-seen-first behaviour without picking a different column.
      if (sortDir === 'desc') {
        setSortDir('asc')
      } else {
        setSortField('none')
      }
    } else {
      setSortField(field)
      setSortDir('desc')
    }
  }

  const [confirmClearOpen, setConfirmClearOpen] = useState(false)

  async function doClearAll() {
    setConfirmClearOpen(false)
    try {
      await clearPlayers()
      load()
      setSelected(null)
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <UserSearch size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Player Tracker
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {total.toLocaleString()} tracked
          {players.length < total && ` · showing ${players.length.toLocaleString()}`}
        </span>
        <div className="ml-auto flex items-center gap-2">
          {pvpWarningOn !== null && (
            <button
              onClick={() => void togglePvpWarning()}
              title={
                pvpWarningOn
                  ? 'PVP warning is ON — flagged players appearing in a /who or joining your group fire a sound + on-screen alert. Click to disable.'
                  : 'PVP warning is OFF — no alert fires for flagged players. Click to enable.'
              }
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: pvpWarningOn ? 'var(--color-danger)' : 'var(--color-muted)',
                border: `1px solid ${pvpWarningOn ? 'var(--color-danger)' : 'var(--color-border)'}`,
              }}
            >
              {pvpWarningOn ? <Bell size={11} /> : <BellOff size={11} />}
              PVP warning {pvpWarningOn ? 'on' : 'off'}
            </button>
          )}
          <BackfillLink />
          <button
            onClick={load}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
          <button
            onClick={() => setConfirmClearOpen(true)}
            disabled={players.length === 0}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded disabled:opacity-50"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-danger)',
              border: '1px solid var(--color-border)',
            }}
          >
            <Trash2 size={11} />
            Clear all
          </button>
        </div>
      </div>

      {/* Filters */}
      <div
        className="border-b px-4 py-2 shrink-0 flex items-center gap-2 flex-wrap"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name…"
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            minWidth: '14rem',
          }}
        />
        <select
          value={classFilter}
          onChange={(e) => setClassFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <option value="">All classes</option>
          {CLASS_LIST.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        <select
          value={zoneFilter}
          onChange={(e) => setZoneFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <option value="">All zones</option>
          {zoneOptions.map((z) => (
            <option key={z} value={z}>{z}</option>
          ))}
        </select>
        <select
          value={guildFilter}
          onChange={(e) => setGuildFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <option value="">All guilds</option>
          {guildOptions.map((g) => (
            <option key={g} value={g}>{g}</option>
          ))}
        </select>
        <button
          onClick={() => setPvpOnly((v) => !v)}
          title="Show only players flagged as PVP"
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: pvpOnly ? 'rgba(239, 68, 68, 0.15)' : 'var(--color-surface-2)',
            color: pvpOnly ? 'var(--color-danger)' : 'var(--color-muted-foreground)',
            border: `1px solid ${pvpOnly ? 'var(--color-danger)' : 'var(--color-border)'}`,
          }}
        >
          <Swords size={11} />
          PVP only
        </button>
        {(search || classFilter || zoneFilter || guildFilter || pvpOnly || sortField !== 'none') && (
          <button
            onClick={() => {
              setSearch('')
              setClassFilter('')
              setZoneFilter('')
              setGuildFilter('')
              setPvpOnly(false)
              setSortField('none')
              setSortDir('desc')
            }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
            title="Reset all filters and sort"
          >
            <X size={11} />
            Clear filters
          </button>
        )}
      </div>

      {/* Detail panel */}
      <div className="flex-1 overflow-y-auto p-4">
        <MissingLogNotice />
        {selected && (
          <div
            style={{
              position: 'sticky',
              top: -16, // counter the parent's p-4 padding so the panel sits flush with the top edge while sticky
              zIndex: 10,
              marginTop: -16,
              marginLeft: -16,
              marginRight: -16,
              padding: 16,
              backgroundColor: 'var(--color-background)',
              borderBottom: '1px solid var(--color-border)',
              marginBottom: 16,
            }}
          >
            <PlayerDetail
              player={selected}
              onClose={() => setSelected(null)}
              onDeleted={() => {
                // Patch locally instead of reloading so any extra "Show
                // more" pages the user loaded stay loaded.
                setPlayers((prev) => prev.filter((pl) => pl.name !== selected.name))
                setTotal((t) => Math.max(0, t - 1))
              }}
              onChanged={(note, pvp) => {
                setPlayers((prev) =>
                  prev.map((pl) => (pl.name === selected.name ? { ...pl, note, pvp } : pl))
                )
                setSelected((prev) => (prev ? { ...prev, note, pvp } : prev))
              }}
              onManualChanged={(m) => {
                const patch = (pl: PlayerSighting) =>
                  withEffective({ ...pl, manual_class: m.class, manual_level: m.level, manual_race: m.race })
                setPlayers((prev) =>
                  prev.map((pl) => (pl.name === selected.name ? patch(pl) : pl))
                )
                setSelected((prev) => (prev ? patch(prev) : prev))
              }}
            />
          </div>
        )}

        {/* Body states */}
        {loading && (
          <div className="flex flex-1 items-center justify-center py-12">
            <RefreshCw size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {error && !loading && (
          <div className="flex items-start gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
          </div>
        )}

        {!loading && !error && players.length === 0 && (search || classFilter || zoneFilter || guildFilter || pvpOnly) && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <UserSearch size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No players match the current filters.
            </p>
          </div>
        )}

        {!loading && !error && players.length === 0 && !search && !classFilter && !zoneFilter && !guildFilter && !pvpOnly && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <UserSearch size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No players tracked yet.
            </p>
            <p className="text-[11px] max-w-md text-center" style={{ color: 'var(--color-muted)' }}>
              Type <code className="font-mono">/who</code> or <code className="font-mono">/who all</code> in EverQuest and the players in your zone will appear here. Players you exchange tells with or group up with are tracked automatically too. The list persists across restarts and merges anonymous sightings into the last-known class/level.
            </p>
          </div>
        )}

        {!loading && !error && players.length > 0 && (
          <div
            className="grid gap-x-3 text-xs"
            style={{ gridTemplateColumns: 'auto auto 1fr 1fr 1fr auto auto', color: 'var(--color-muted-foreground)' }}
          >
            <SortHeader label="Name" field="name" activeField={sortField} dir={sortDir} onClick={handleSort} />
            <SortHeader label="Lv" field="level" activeField={sortField} dir={sortDir} onClick={handleSort} />
            <SortHeader label="Class" field="class" activeField={sortField} dir={sortDir} onClick={handleSort} />
            <SortHeader label="Guild" field="guild" activeField={sortField} dir={sortDir} onClick={handleSort} />
            <SortHeader label="Last zone" field="zone" activeField={sortField} dir={sortDir} onClick={handleSort} />
            <SortHeader label="Sightings" field="sightings" activeField={sortField} dir={sortDir} onClick={handleSort} align="right" />
            <SortHeader label="Seen" field="seen" activeField={sortField} dir={sortDir} onClick={handleSort} align="right" />
            {sortedPlayers.map((p) => (
              <React.Fragment key={p.name}>
                <button
                  onClick={() => setSelected(p)}
                  className="text-left py-1 hover:underline"
                  style={{ color: 'var(--color-primary)' }}
                >
                  {p.name}
                  {p.last_anonymous && (
                    <EyeOff size={10} className="inline-block ml-1.5" style={{ color: 'var(--color-muted)' }} />
                  )}
                  {p.pvp && <PvpBadge className="ml-1.5 align-middle" />}
                  {p.note && (
                    <span title={p.note}>
                      <StickyNote size={10} className="inline-block ml-1.5" style={{ color: 'var(--color-muted)' }} />
                    </span>
                  )}
                </button>
                <span className="py-1 tabular-nums">{p.effective_level > 0 ? p.effective_level : '—'}</span>
                <span className="py-1 truncate">{p.effective_class || '—'}</span>
                <span className="py-1 truncate" title={p.guild || ''}>{p.guild || '—'}</span>
                <span className="py-1 truncate">{p.last_seen_zone || '—'}</span>
                <span className="py-1 text-right tabular-nums">{p.sightings_count}</span>
                <span className="py-1 text-right tabular-nums">{formatRelative(p.last_seen_at)}</span>
              </React.Fragment>
            ))}
          </div>
        )}

        {!loading && !error && players.length < total && (
          <div className="flex justify-center py-3">
            <button
              onClick={loadMore}
              disabled={loadingMore}
              className="text-xs px-3 py-1.5 rounded disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              {loadingMore
                ? 'Loading…'
                : `Show more (${(total - players.length).toLocaleString()} remaining)`}
            </button>
          </div>
        )}
      </div>

      {confirmClearOpen && (
        <ConfirmModal
          title="Clear player tracker?"
          body="Wipe every tracked player and their sighting history. Notes and PVP flags are kept and re-attach when a player is next seen. This cannot be undone."
          confirmLabel="Clear all"
          onCancel={() => setConfirmClearOpen(false)}
          onConfirm={doClearAll}
        />
      )}
    </div>
  )
}

// PvpBadge is the small red "PVP" chip shown wherever a flagged player's
// name appears.
function PvpBadge({ className = '' }: { className?: string }): React.ReactElement {
  return (
    <span
      className={`inline-flex items-center gap-0.5 rounded px-1 text-[9px] font-bold leading-4 ${className}`}
      style={{
        backgroundColor: 'rgba(239, 68, 68, 0.15)',
        color: 'var(--color-danger)',
        border: '1px solid var(--color-danger)',
      }}
      title="Flagged as PVP"
    >
      <Swords size={8} />
      PVP
    </span>
  )
}

// SortHeader is a clickable table-column header. Renders an arrow icon on
// the active column to indicate sort direction; a third click on the same
// column resets to the API default order (handled by the parent's
// onClick logic).
function SortHeader({
  label,
  field,
  activeField,
  dir,
  onClick,
  align = 'left',
}: {
  label: string
  field: SortField
  activeField: SortField
  dir: SortDir
  onClick: (field: SortField) => void
  align?: 'left' | 'right'
}): React.ReactElement {
  const active = activeField === field
  return (
    <button
      onClick={() => onClick(field)}
      className="font-semibold border-b pb-1 flex items-center gap-1"
      style={{
        color: active ? 'var(--color-foreground)' : 'var(--color-muted)',
        borderColor: 'var(--color-border)',
        backgroundColor: 'transparent',
        cursor: 'pointer',
        justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
        textAlign: align,
      }}
      title="Click to sort. Click the active column again to flip direction; once more to clear."
    >
      <span>{label}</span>
      {active && (dir === 'asc' ? <ArrowUp size={10} /> : <ArrowDown size={10} />)}
    </button>
  )
}

// ConfirmModal mirrors the themed confirmation pattern used elsewhere in
// the app (CombatHistoryPage, CharacterSpellsetsPage). Click the backdrop
// to cancel; the destructive button uses the danger color so it reads as
// "are you really sure?" at a glance.
function ConfirmModal({
  title,
  body,
  confirmLabel,
  onCancel,
  onConfirm,
}: {
  title: string
  body: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  useEscapeToClose(onCancel)
  return (
    <div
      onClick={onCancel}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 420,
        }}
      >
        <div className="flex items-center gap-2">
          <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {title}
          </p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          {body}
        </p>
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onCancel}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-danger)',
              color: '#fff',
              border: '1px solid transparent',
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

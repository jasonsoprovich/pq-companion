import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { UserSearch, RefreshCw, Trash2, AlertCircle, EyeOff, X } from 'lucide-react'
import { listPlayers, deletePlayer, clearPlayers, getPlayerHistory } from '../services/api'
import type { PlayerSighting, PlayerLevelHistoryEntry } from '../types/player'

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
}: {
  player: PlayerSighting
  onClose: () => void
  onDeleted: () => void
}): React.ReactElement {
  const [history, setHistory] = useState<PlayerLevelHistoryEntry[] | null>(null)
  const [historyErr, setHistoryErr] = useState<string | null>(null)

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
          </h2>
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            {player.last_seen_level > 0 ? `Level ${player.last_seen_level}` : 'Level unknown'}
            {player.class ? ` ${player.class}` : ''}
            {player.race ? ` · ${player.race}` : ''}
            {player.guild ? ` · <${player.guild}>` : ''}
          </p>
          <p className="text-[11px] mt-0.5" style={{ color: 'var(--color-muted)' }}>
            Seen {player.sightings_count} time{player.sightings_count === 1 ? '' : 's'} · last in{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{player.last_seen_zone || 'unknown zone'}</span>{' '}
            {formatRelative(player.last_seen_at)}
          </p>
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

export default function PlayersPage(): React.ReactElement {
  const [players, setPlayers] = useState<PlayerSighting[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [classFilter, setClassFilter] = useState<string>('')
  const [zoneFilter, setZoneFilter] = useState<string>('')
  const [selected, setSelected] = useState<PlayerSighting | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listPlayers({ search, class: classFilter, zone: zoneFilter, limit: 500 })
      .then((r) => setPlayers(r.players))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [search, classFilter, zoneFilter])

  useEffect(() => {
    load()
  }, [load])

  // Derive a unique zone list from the loaded set so the zone filter is a real
  // dropdown of zones actually present in the data.
  const zoneOptions = useMemo(() => {
    const zones = new Set<string>()
    players.forEach((p) => { if (p.last_seen_zone) zones.add(p.last_seen_zone) })
    return Array.from(zones).sort()
  }, [players])

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
          Players
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {players.length} tracked
        </span>
        <div className="ml-auto flex items-center gap-2">
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
      </div>

      {/* Detail panel */}
      <div className="flex-1 overflow-y-auto p-4">
        {selected && (
          <PlayerDetail
            player={selected}
            onClose={() => setSelected(null)}
            onDeleted={() => load()}
          />
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

        {!loading && !error && players.length === 0 && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <UserSearch size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No players tracked yet.
            </p>
            <p className="text-[11px] max-w-md text-center" style={{ color: 'var(--color-muted)' }}>
              Type <code className="font-mono">/who</code> or <code className="font-mono">/who all</code> in EverQuest and the players in your zone will appear here. The list persists across restarts and merges anonymous sightings into the last-known class/level.
            </p>
          </div>
        )}

        {!loading && !error && players.length > 0 && (
          <div
            className="grid gap-x-3 text-xs"
            style={{ gridTemplateColumns: 'auto auto 1fr 1fr 1fr auto auto', color: 'var(--color-muted-foreground)' }}
          >
            <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Name</span>
            <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Lv</span>
            <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Class</span>
            <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Guild</span>
            <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Last zone</span>
            <span className="font-semibold border-b pb-1 text-right" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Sightings</span>
            <span className="font-semibold border-b pb-1 text-right" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>Seen</span>
            {players.map((p) => (
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
                </button>
                <span className="py-1 tabular-nums">{p.last_seen_level > 0 ? p.last_seen_level : '—'}</span>
                <span className="py-1 truncate">{p.class || '—'}</span>
                <span className="py-1 truncate" title={p.guild || ''}>{p.guild || '—'}</span>
                <span className="py-1 truncate">{p.last_seen_zone || '—'}</span>
                <span className="py-1 text-right tabular-nums">{p.sightings_count}</span>
                <span className="py-1 text-right tabular-nums">{formatRelative(p.last_seen_at)}</span>
              </React.Fragment>
            ))}
          </div>
        )}
      </div>

      {confirmClearOpen && (
        <ConfirmModal
          title="Clear player tracker?"
          body="Wipe every tracked player and their sighting history. This cannot be undone."
          confirmLabel="Clear all"
          onCancel={() => setConfirmClearOpen(false)}
          onConfirm={doClearAll}
        />
      )}
    </div>
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

import React, { useCallback, useEffect, useState } from 'react'
import {
  AlertCircle, ArrowDown, ArrowUp, ExternalLink, Minus, Pencil, Play, Plus,
  RotateCcw, Square, Trash2, Timer, PackagePlus, Info,
} from 'lucide-react'
import {
  createKillCounter, deleteKillCounter, getKillCounterPresets,
  installKillCounterPreset, updateKillCounter,
} from '../../services/api'
import { fmtClock, nextMilestone, useKillCounters, useNowSecs } from '../../hooks/useKillCounters'
import type { KillCounter, KillCounterDef, KillCounterPreset } from '../../types/kills'
import KillCounterEditor from './KillCounterEditor'

const ghostBtn: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  color: 'var(--color-muted-foreground)',
  border: '1px solid var(--color-border)',
}

const iconBtn: React.CSSProperties = {
  ...ghostBtn,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 24,
  height: 24,
  borderRadius: 4,
  cursor: 'pointer',
}

export default function KillCountersTab(): React.ReactElement {
  const k = useKillCounters()
  const [presets, setPresets] = useState<KillCounterPreset[]>([])
  const [editing, setEditing] = useState<KillCounterDef | 'new' | null>(null)
  const [error, setError] = useState('')
  const anyClock = k.counters.some((c) => c.def.clock && c.clock_started_at > 0)
  const now = useNowSecs(anyClock, k.skewSecs)

  const loadPresets = useCallback(() => {
    getKillCounterPresets().then((r) => setPresets(r.presets ?? [])).catch(() => { /* best effort */ })
  }, [])
  // Re-check "installed" whenever the counter set changes (a preset counter
  // may have been deleted).
  useEffect(() => { loadPresets() }, [loadPresets, k.counters.length])

  async function run(fn: () => Promise<unknown>) {
    try {
      const r = await fn()
      if (r && typeof r === 'object' && 'counters' in r) k.apply(r as Parameters<typeof k.apply>[0])
      setError('')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function save(def: KillCounterDef | Omit<KillCounterDef, 'id'>): Promise<string | null> {
    try {
      const s = 'id' in def && def.id ? await updateKillCounter(def as KillCounterDef) : await createKillCounter(def)
      k.apply(s)
      setEditing(null)
      return null
    } catch (e) {
      return (e as Error).message
    }
  }

  const err = error || k.error

  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-4">
      <div className="flex items-start gap-2 rounded p-3 text-[11px] leading-relaxed"
        style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
        <Info size={14} className="shrink-0 mt-0.5" style={{ color: 'var(--color-primary)' }} />
        <span>
          Counters count kills of the mobs you list (or every log line matching a pattern) and call out
          milestones on the Trigger Alerts overlay.{' '}
          {k.killSource === 'zeal' ? (
            <>
              Zeal death events are active, so every kill in the zone counts, not just the ones your log
              shows.
            </>
          ) : (
            <>
              Your log only shows kills near you, so a raid count can fall behind. A Zeal build that sends
              death events fixes this automatically.
            </>
          )}{' '}
          <span className="font-medium">Resync lines</span> also correct the count when the server
          announces a known number, like a boss-spawn emote. Show them in game with the
          <span className="font-medium"> Kill Counters</span> overlay.
        </span>
      </div>

      {err && (
        <div className="flex items-start gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
          <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
          <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{err}</p>
        </div>
      )}

      <div className="flex items-center gap-2 flex-wrap">
        <button onClick={() => setEditing('new')} className="flex items-center gap-1.5 text-xs px-2 py-1 rounded font-medium"
          style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}>
          <Plus size={12} /> New counter
        </button>
        {window.electron?.overlay && (
          <button onClick={() => window.electron.overlay.toggleKillCounter()}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded" style={ghostBtn}>
            <ExternalLink size={11} /> Pop out overlay
          </button>
        )}
      </div>

      {presets.some((p) => !p.installed) && (
        <div className="rounded" style={{ border: '1px dashed var(--color-border)' }}>
          <p className="flex items-center gap-1.5 px-3 pt-2 pb-1 text-[11px] font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}>
            <PackagePlus size={12} style={{ color: 'var(--color-primary)' }} /> Event presets
          </p>
          {presets.filter((p) => !p.installed).map((p) => (
            <div key={p.key} className="flex items-center gap-3 px-3 py-1.5 border-t"
              style={{ borderColor: 'var(--color-border)' }}>
              <div className="flex-1 min-w-0">
                <p className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                  <span className="font-semibold">{p.name}</span>
                  <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>{p.zone}</span>
                </p>
                <p className="text-[11px] truncate" title={p.description} style={{ color: 'var(--color-muted-foreground)' }}>
                  {p.description}
                </p>
              </div>
              <button onClick={() => run(() => installKillCounterPreset(p.key))}
                className="text-xs px-2 py-1 rounded shrink-0" style={ghostBtn}>
                Add
              </button>
            </div>
          ))}
        </div>
      )}

      {k.loaded && k.counters.length === 0 && (
        <p className="text-xs py-6 text-center" style={{ color: 'var(--color-muted)' }}>
          No counters yet. Add a preset above or create your own.
        </p>
      )}

      <div className="space-y-2">
        {k.counters.map((c, i) => (
          <CounterCard
            key={c.def.id}
            counter={c}
            now={now}
            first={i === 0}
            last={i === k.counters.length - 1}
            onAct={(body) => k.act(c.def.id, body)}
            onEdit={() => setEditing(c.def)}
            onDelete={() => {
              if (window.confirm(`Delete counter "${c.def.name}"?`)) run(() => deleteKillCounter(c.def.id))
            }}
          />
        ))}
      </div>

      {editing && (
        <KillCounterEditor
          initial={editing === 'new' ? null : editing}
          onCancel={() => setEditing(null)}
          onSave={save}
        />
      )}
    </div>
  )
}

function CounterCard({
  counter: c, now, first, last, onAct, onEdit, onDelete,
}: {
  counter: KillCounter
  now: number
  first: boolean
  last: boolean
  onAct: (body: Parameters<ReturnType<typeof useKillCounters>['act']>[1]) => void
  onEdit: () => void
  onDelete: () => void
}): React.ReactElement {
  const [setValue, setSetValue] = useState('')
  const next = nextMilestone(c)
  const milestones = c.def.milestones ?? []
  const goal = c.def.goal || (milestones.length ? milestones[milestones.length - 1].at : 0)
  const clockRunning = !!c.def.clock && c.clock_started_at > 0
  const left = clockRunning && c.def.clock ? c.clock_started_at + c.def.clock.duration_secs - now : 0

  const source = [
    (c.def.mobs?.length ?? 0) > 0 ? `${c.def.mobs!.length} mob name${c.def.mobs!.length === 1 ? '' : 's'}` : '',
    c.def.pattern ? 'line pattern' : '',
  ].filter(Boolean).join(' + ')

  function submitSet() {
    const n = parseInt(setValue, 10)
    if (Number.isFinite(n) && n >= 0) onAct({ action: 'set', value: n })
    setSetValue('')
  }

  return (
    <div className="rounded p-3 space-y-2"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', opacity: c.def.enabled ? 1 : 0.6 }}>
      <div className="flex items-center gap-2">
        <input type="checkbox" checked={c.def.enabled} title={c.def.enabled ? 'Counting — click to pause' : 'Paused — click to count'}
          onChange={(e) => onAct({ action: 'enable', enabled: e.target.checked })} />
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold truncate" style={{ color: 'var(--color-foreground)' }}>
            {c.def.name}
            {c.def.group && <span className="ml-2 text-[11px] font-normal" style={{ color: 'var(--color-muted)' }}>{c.def.group}</span>}
          </p>
          <p className="text-[11px] truncate" style={{ color: 'var(--color-muted)' }}>
            {source}{c.def.zone ? ` · ${c.def.zone}` : ''}
            {(c.def.syncs?.length ?? 0) > 0 ? ` · ${c.def.syncs!.length} resync line${c.def.syncs!.length === 1 ? '' : 's'}` : ''}
          </p>
        </div>
        <span className="text-xl font-bold tabular-nums" style={{ color: goal > 0 && c.count >= goal ? '#22c55e' : 'var(--color-foreground)' }}>
          {c.count}
          {goal > 0 && <span className="text-xs font-normal" style={{ color: 'var(--color-muted)' }}> / {goal}</span>}
        </span>
      </div>

      {(next || clockRunning || c.def.clock) && (
        <div className="flex items-center gap-3 text-[11px] flex-wrap" style={{ color: 'var(--color-muted-foreground)' }}>
          {next && (
            <span>Next: <span style={{ color: 'var(--color-foreground)' }}>{next.label || next.at}</span> at {next.at} ({next.at - c.count} to go)</span>
          )}
          {c.def.clock && (
            <span className="flex items-center gap-1">
              <Timer size={11} />
              {clockRunning ? (
                <span className="tabular-nums" style={{ color: left < 300 ? 'var(--color-danger)' : 'var(--color-foreground)' }}>
                  {c.def.clock.label || 'Clock'}: {left > 0 ? fmtClock(left) : 'expired'}
                </span>
              ) : (
                <span>{c.def.clock.label || 'Clock'}: not running</span>
              )}
            </span>
          )}
        </div>
      )}

      <div className="flex items-center gap-1.5 flex-wrap">
        <button style={iconBtn} title="Subtract one" onClick={() => onAct({ action: 'adjust', value: -1 })}><Minus size={12} /></button>
        <button style={iconBtn} title="Add one" onClick={() => onAct({ action: 'adjust', value: 1 })}><Plus size={12} /></button>
        <input
          type="text" inputMode="numeric" value={setValue} placeholder="Set…"
          onChange={(e) => setSetValue(e.target.value.replace(/[^0-9]/g, ''))}
          onKeyDown={(e) => { if (e.key === 'Enter') submitSet() }}
          onBlur={() => { if (setValue) submitSet() }}
          className="text-xs rounded px-2 outline-none"
          style={{ ...ghostBtn, color: 'var(--color-foreground)', width: 56, height: 24 }}
          title="Type a number and press Enter to set the count"
        />
        <button style={iconBtn} title="Reset count and clock"
          onClick={() => { if (window.confirm(`Reset "${c.def.name}" to 0?`)) onAct({ action: 'reset' }) }}>
          <RotateCcw size={12} />
        </button>
        {c.def.clock && (
          clockRunning ? (
            <button style={iconBtn} title="Stop the event clock" onClick={() => onAct({ action: 'stop_clock' })}><Square size={11} /></button>
          ) : (
            <button style={iconBtn} title="Start the event clock now" onClick={() => onAct({ action: 'start_clock' })}><Play size={11} /></button>
          )
        )}
        <div className="ml-auto flex items-center gap-1.5">
          <button style={{ ...iconBtn, opacity: first ? 0.4 : 1 }} disabled={first} title="Move up" onClick={() => onAct({ action: 'move', value: -1 })}><ArrowUp size={12} /></button>
          <button style={{ ...iconBtn, opacity: last ? 0.4 : 1 }} disabled={last} title="Move down" onClick={() => onAct({ action: 'move', value: 1 })}><ArrowDown size={12} /></button>
          <button style={iconBtn} title="Edit" onClick={onEdit}><Pencil size={12} /></button>
          <button style={{ ...iconBtn, color: 'var(--color-danger)' }} title="Delete" onClick={onDelete}><Trash2 size={12} /></button>
        </div>
      </div>
    </div>
  )
}

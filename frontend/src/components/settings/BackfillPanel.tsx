import React, { useEffect, useState } from 'react'
import { DatabaseBackup, RefreshCw, AlertTriangle, CheckCircle2, AlertCircle, Clock } from 'lucide-react'
import { getBackfillInfo, getConfig, updateConfig, type BackfillSection } from '../../services/api'
import { DEV_SKILLS } from '../../lib/devFlags'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import { useBackfill } from '../../contexts/BackfillContext'

export default function BackfillPanel(): React.ReactElement {
  const [sections, setSections] = useState<BackfillSection[]>([])
  const [characters, setCharacters] = useState<string[]>([])
  const [selChars, setSelChars] = useState<Set<string>>(new Set())
  const [selSections, setSelSections] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  // The run itself lives in the app-root BackfillProvider so it keeps going (and
  // shows in the persistent bottom bar) while the user navigates away from
  // Settings. This panel just kicks it off and reads back the results.
  const { running, results, startBackfill } = useBackfill()

  useEffect(() => {
    getBackfillInfo()
      .then((info) => {
        // The Skill Tracker is hidden behind DEV_SKILLS (no full skill-snapshot
        // data source exists — see lib/devFlags). Drop its backfill row too so
        // it isn't an orphaned, dead-end option.
        const sections = DEV_SKILLS
          ? info.sections
          : info.sections.filter((s) => s.key !== 'skills')
        setSections(sections)
        setCharacters(info.characters)
        // Default: the active character selected, all sections selected.
        setSelChars(new Set(info.active && info.characters.includes(info.active) ? [info.active] : []))
        setSelSections(new Set(sections.map((s) => s.key)))
      })
      .catch((e: Error) => setLoadErr(e.message))
      .finally(() => setLoading(false))
  }, [])

  function toggle(set: Set<string>, key: string, setter: (s: Set<string>) => void) {
    const next = new Set(set)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setter(next)
  }

  const canRun = selChars.size > 0 && selSections.size > 0 && !running

  function doRun() {
    setConfirmOpen(false)
    startBackfill(Array.from(selChars), Array.from(selSections))
  }

  const labelFor = (key: string) => sections.find((s) => s.key === key)?.label ?? key

  return (
    <>
      <ChatRetentionCard />

      <section
        id="log-backfill"
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <h2 className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          <DatabaseBackup size={13} /> Log Backfill
        </h2>
        <p className="mb-4 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          Trackers normally fill in going forward as you play. This replays a character's existing log to
          populate them retroactively. Pick which characters (each has its own log file) and which trackers to
          backfill. Backfilling pays attention to timestamps — it won't double-count or overwrite newer data,
          so re-running is always safe.
        </p>

        {loading && (
          <div className="flex items-center gap-2 py-6" style={{ color: 'var(--color-muted)' }}>
            <RefreshCw size={16} className="animate-spin" />
            <span className="text-xs">Loading…</span>
          </div>
        )}

        {loadErr && !loading && (
          <div className="flex items-start gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{loadErr}</p>
          </div>
        )}

        {!loading && !loadErr && characters.length === 0 && (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            No character log files found. Set your EverQuest folder under the General tab first.
          </p>
        )}

        {!loading && !loadErr && characters.length > 0 && (
          <div className="flex flex-col gap-4">
            {/* Characters */}
            <div>
              <p className="mb-2 text-[11px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                Characters
              </p>
              <div className="flex flex-col gap-1.5">
                {characters.map((c) => (
                  <CheckRow
                    key={c}
                    label={c}
                    checked={selChars.has(c)}
                    onChange={() => toggle(selChars, c, setSelChars)}
                  />
                ))}
              </div>
            </div>

            {/* Sections */}
            <div>
              <p className="mb-2 text-[11px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                Trackers
              </p>
              {sections.length === 0 ? (
                <p className="text-xs" style={{ color: 'var(--color-muted)' }}>No backfillable trackers available.</p>
              ) : (
                <div className="flex flex-col gap-1.5">
                  {sections.map((s) => (
                    <CheckRow
                      key={s.key}
                      label={s.label}
                      checked={selSections.has(s.key)}
                      onChange={() => toggle(selSections, s.key, setSelSections)}
                    />
                  ))}
                </div>
              )}
            </div>

            <div className="flex items-center gap-3">
              <button
                onClick={() => setConfirmOpen(true)}
                disabled={!canRun}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium disabled:opacity-50"
                style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}
              >
                {running ? <RefreshCw size={12} className="animate-spin" /> : <DatabaseBackup size={12} />}
                {running ? 'Backfilling…' : 'Run backfill'}
              </button>
              {running && (
                <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Running in the background — progress shows at the bottom.
                </span>
              )}
            </div>

            {/* Results */}
            {results && (
              <div className="rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
                <div className="mb-2 flex items-center gap-2">
                  <CheckCircle2 size={14} style={{ color: 'var(--color-primary)' }} />
                  <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Backfill complete</span>
                </div>
                <div className="flex flex-col gap-2">
                  {results.map((r) => (
                    <div key={r.character} className="text-xs">
                      <span className="font-medium" style={{ color: 'var(--color-foreground)' }}>{r.character}</span>
                      {r.error ? (
                        <span className="ml-2" style={{ color: 'var(--color-danger)' }}>{r.error}</span>
                      ) : (
                        <span className="ml-2" style={{ color: 'var(--color-muted-foreground)' }}>
                          {Object.keys(r.results).length === 0
                            ? 'nothing to add'
                            : Object.entries(r.results)
                                .map(([k, n]) => `${labelFor(k)}: +${n}`)
                                .join(' · ')}
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </section>

      {confirmOpen && (
        <ConfirmModal
          characters={Array.from(selChars)}
          sections={Array.from(selSections).map(labelFor)}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={doRun}
        />
      )}

    </>
  )
}

function CheckRow({
  label, checked, onChange,
}: {
  label: string
  checked: boolean
  onChange: () => void
}): React.ReactElement {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
      <input type="checkbox" checked={checked} onChange={onChange} />
      {label}
    </label>
  )
}

// ChatRetentionCard controls how long Chat History is kept before the daily
// purge. Negative is stored as "keep forever" (-1); otherwise a positive day
// count. Saves immediately on Apply.
function ChatRetentionCard(): React.ReactElement {
  const [retention, setRetention] = useState<number | null>(null) // current saved value
  const [days, setDays] = useState('30')
  const [keepForever, setKeepForever] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    getConfig()
      .then((c) => {
        const v = c.chat_retention_days
        setRetention(v)
        if (v < 0) {
          setKeepForever(true)
        } else {
          setDays(String(v || 30))
        }
      })
      .catch(() => setRetention(30))
  }, [])

  const target = keepForever ? -1 : Math.max(1, parseInt(days, 10) || 30)
  const dirty = retention !== null && target !== retention

  async function apply() {
    setSaving(true)
    setSaved(false)
    try {
      const cfg = await getConfig()
      const next = { ...cfg, chat_retention_days: target }
      const savedCfg = await updateConfig(next)
      setRetention(savedCfg.chat_retention_days)
      setSaved(true)
    } finally {
      setSaving(false)
    }
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2 className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        <Clock size={13} /> Chat History retention
      </h2>
      <p className="mb-3 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
        Chat messages older than this are deleted by a daily cleanup so the history stays fast. Tells and all
        channels share this window.
      </p>
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
          Keep
          <input
            type="number"
            min={1}
            value={days}
            disabled={keepForever}
            onChange={(e) => { setDays(e.target.value); setSaved(false) }}
            className="w-20 rounded px-2 py-1 text-sm outline-none disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
          />
          days
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
          <input type="checkbox" checked={keepForever} onChange={(e) => { setKeepForever(e.target.checked); setSaved(false) }} />
          Keep forever
        </label>
        <button
          onClick={apply}
          disabled={saving || !dirty}
          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium disabled:opacity-50"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
        >
          {saving ? <RefreshCw size={12} className="animate-spin" /> : null}
          Apply
        </button>
        {saved && !dirty && (
          <span className="flex items-center gap-1 text-xs" style={{ color: 'var(--color-primary)' }}>
            <CheckCircle2 size={12} /> Saved
          </span>
        )}
      </div>
    </section>
  )
}

function ConfirmModal({
  characters, sections, onCancel, onConfirm,
}: {
  characters: string[]
  sections: string[]
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  useEscapeToClose(onCancel)
  return (
    <div
      onClick={onCancel}
      style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 460 }}
      >
        <div className="flex items-center gap-2">
          <AlertTriangle size={16} style={{ color: '#f97316' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Run backfill?</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          This reads the full log file for {characters.length} character{characters.length === 1 ? '' : 's'}
          {' '}({characters.join(', ')}) and populates: {sections.join(', ')}. It runs in the background — you
          can keep using the app while a progress bar at the bottom tracks it. Re-running is safe —
          already-recorded entries are skipped.
        </p>
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onCancel}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}
          >
            Run backfill
          </button>
        </div>
      </div>
    </div>
  )
}

import React, { useState } from 'react'
import { AlertCircle, ChevronDown, ChevronRight } from 'lucide-react'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import type { KillClockMark, KillCounterDef, KillMilestone, KillSyncRule } from '../../types/kills'

// Text-area round-trips. Milestones are "44 Darwol Adan spawns"; resync and
// clock lines are "<number> | <regex>" so a regex containing spaces survives.

function milestonesToText(ms?: KillMilestone[]): string {
  return (ms ?? []).map((m) => (m.label ? `${m.at} ${m.label}` : String(m.at))).join('\n')
}

function parseMilestones(text: string): KillMilestone[] | string {
  const out: KillMilestone[] = []
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    const m = /^(\d+)\s*[:\-–]?\s*(.*)$/.exec(line)
    if (!m) return `Milestone line must start with a number: "${line}"`
    out.push({ at: parseInt(m[1], 10), label: m[2].trim() })
  }
  return out
}

function pairsToText(rows: { n: number; pattern: string }[]): string {
  return rows.map((r) => `${r.n} | ${r.pattern}`).join('\n')
}

function parsePairs(text: string, what: string): { n: number; pattern: string }[] | string {
  const out: { n: number; pattern: string }[] = []
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    const m = /^(\d+)\s*\|\s*(.+)$/.exec(line)
    if (!m) return `${what} line must look like "<number> | <pattern>": "${line}"`
    out.push({ n: parseInt(m[1], 10), pattern: m[2].trim() })
  }
  return out
}

// Clock lines may carry flags between the seconds and the bar:
// "0 idle reset | <regex>" — idle = only while the clock isn't running,
// reset = also zero the count.
function marksToText(ms?: KillClockMark[]): string {
  return (ms ?? [])
    .map((m) => [String(m.at_secs), m.if_idle ? 'idle' : '', m.reset ? 'reset' : ''].filter(Boolean).join(' ') + ` | ${m.pattern}`)
    .join('\n')
}

function parseMarks(text: string): KillClockMark[] | string {
  const out: KillClockMark[] = []
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    const m = /^(\d+)((?:\s+(?:idle|reset))*)\s*\|\s*(.+)$/i.exec(line)
    if (!m) return `Clock line must look like "<seconds> [idle] [reset] | <pattern>": "${line}"`
    const flags = m[2].toLowerCase()
    out.push({ at_secs: parseInt(m[1], 10), pattern: m[3].trim(), if_idle: /idle/.test(flags), reset: /reset/.test(flags) })
  }
  return out
}

function lines(text: string): string[] {
  return text.split('\n').map((l) => l.trim()).filter(Boolean)
}

function intOr(text: string, fallback: number): number {
  const n = parseInt(text, 10)
  return Number.isFinite(n) && n >= 0 ? n : fallback
}

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }): React.ReactElement {
  return (
    <label className="block space-y-1">
      <span className="text-[11px] font-semibold" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      {children}
      {hint && <span className="block text-[10px]" style={{ color: 'var(--color-muted)' }}>{hint}</span>}
    </label>
  )
}

export default function KillCounterEditor({
  initial,
  onCancel,
  onSave,
}: {
  initial: KillCounterDef | null
  onCancel: () => void
  // Resolves to an error message, or null on success.
  onSave: (def: KillCounterDef | Omit<KillCounterDef, 'id'>) => Promise<string | null>
}): React.ReactElement {
  useEscapeToClose(onCancel)
  const d = initial
  const [name, setName] = useState(d?.name ?? '')
  const [group, setGroup] = useState(d?.group ?? '')
  const [mobs, setMobs] = useState((d?.mobs ?? []).join('\n'))
  const [pattern, setPattern] = useState(d?.pattern ?? '')
  const [zone, setZone] = useState(d?.zone ?? '')
  const [goal, setGoal] = useState(d?.goal ? String(d.goal) : '')
  const [milestones, setMilestones] = useState(milestonesToText(d?.milestones))
  const [warnBefore, setWarnBefore] = useState(d?.warn_before ? String(d.warn_before) : '')
  const [alertOverlay, setAlertOverlay] = useState(d?.alert_overlay ?? true)
  const [alertTTS, setAlertTTS] = useState(d?.alert_tts ?? false)
  const [syncs, setSyncs] = useState(pairsToText((d?.syncs ?? []).map((s) => ({ n: s.set_to, pattern: s.pattern }))))
  const [resetPattern, setResetPattern] = useState(d?.reset_pattern ?? '')
  const [clockOn, setClockOn] = useState(!!d?.clock)
  const [clockLabel, setClockLabel] = useState(d?.clock?.label ?? 'Event time left')
  const [clockMins, setClockMins] = useState(d?.clock ? String(Math.round(d.clock.duration_secs / 60)) : '')
  const [clockMarks, setClockMarks] = useState(marksToText(d?.clock?.marks))
  const [clockStops, setClockStops] = useState((d?.clock?.stop_patterns ?? []).join('\n'))
  const [advanced, setAdvanced] = useState(
    !!(d?.syncs?.length || d?.reset_pattern || d?.clock),
  )
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  async function submit() {
    const ms = parseMilestones(milestones)
    if (typeof ms === 'string') return setError(ms)
    const sy = parsePairs(syncs, 'Resync')
    if (typeof sy === 'string') return setError(sy)
    const marks = parseMarks(clockMarks)
    if (typeof marks === 'string') return setError(marks)
    const mins = intOr(clockMins, 0)
    if (clockOn && mins <= 0) return setError('Event clock needs a duration in minutes')

    const def: Omit<KillCounterDef, 'id'> & { id?: string } = {
      ...(d ?? {}),
      name: name.trim(),
      group: group.trim(),
      enabled: d?.enabled ?? true,
      mobs: lines(mobs),
      pattern: pattern.trim(),
      zone: zone.trim(),
      goal: intOr(goal, 0),
      milestones: ms,
      warn_before: intOr(warnBefore, 0),
      syncs: sy.map((s): KillSyncRule => ({ pattern: s.pattern, set_to: s.n })),
      reset_pattern: resetPattern.trim(),
      clock: clockOn
        ? {
            label: clockLabel.trim(),
            duration_secs: mins * 60,
            marks,
            stop_patterns: lines(clockStops),
          }
        : null,
      alert_overlay: alertOverlay,
      alert_tts: alertTTS,
    }
    setSaving(true)
    const err = await onSave(def as KillCounterDef)
    setSaving(false)
    if (err) setError(err)
  }

  const ta = 'w-full text-xs rounded px-2 py-1 outline-none font-mono'

  return (
    <div onClick={onCancel} style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}>
      <div onClick={(e) => e.stopPropagation()} className="rounded-lg flex flex-col"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 560, maxHeight: '90vh' }}>
        <div className="px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {d ? 'Edit counter' : 'New counter'}
          </p>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Name">
              <input value={name} onChange={(e) => setName(e.target.value)} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} autoFocus />
            </Field>
            <Field label="Group (optional)" hint="Counters with the same group show together on the overlay.">
              <input value={group} onChange={(e) => setGroup(e.target.value)} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
            </Field>
          </div>

          <Field label="Mobs to count" hint='One name per line, exactly as the kill line shows it ("Knight of Decay"). Not case-sensitive.'>
            <textarea value={mobs} onChange={(e) => setMobs(e.target.value)} rows={4} className={ta} style={inputStyle} />
          </Field>
          <Field label="…or count log lines matching (regex, optional)" hint='Each matching line adds one, e.g. ^You gain (party |raid )?experience'>
            <input value={pattern} onChange={(e) => setPattern(e.target.value)} className={ta} style={inputStyle} />
          </Field>
          <Field label="Only count in zone (optional)" hint='Zone long name, e.g. "The Crypt of Decay". Leave blank to count everywhere. Entering a different copy of the zone starts the count over.'>
            <input value={zone} onChange={(e) => setZone(e.target.value)} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Goal (optional)" hint="Fills the progress bar. Defaults to the last milestone.">
              <input value={goal} inputMode="numeric" onChange={(e) => setGoal(e.target.value.replace(/[^0-9]/g, ''))} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
            </Field>
            <Field label="Heads-up (kills before milestone)" hint="0 or blank = no heads-up alert.">
              <input value={warnBefore} inputMode="numeric" onChange={(e) => setWarnBefore(e.target.value.replace(/[^0-9]/g, ''))} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
            </Field>
          </div>
          <Field label="Milestones" hint='One per line: count then label, e.g. "44 Darwol Adan spawns".'>
            <textarea value={milestones} onChange={(e) => setMilestones(e.target.value)} rows={4} className={ta} style={inputStyle} />
          </Field>
          <div className="flex items-center gap-4 text-xs" style={{ color: 'var(--color-foreground)' }}>
            <label className="flex items-center gap-1.5">
              <input type="checkbox" checked={alertOverlay} onChange={(e) => setAlertOverlay(e.target.checked)} />
              On-screen alert at milestones
            </label>
            <label className="flex items-center gap-1.5">
              <input type="checkbox" checked={alertTTS} onChange={(e) => setAlertTTS(e.target.checked)} />
              Speak it (TTS)
            </label>
          </div>

          <button onClick={() => setAdvanced((a) => !a)} className="flex items-center gap-1 text-xs font-semibold"
            style={{ color: 'var(--color-muted-foreground)', background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}>
            {advanced ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
            Resync, reset and event clock
          </button>
          {advanced && (
            <div className="space-y-3 pl-3" style={{ borderLeft: '2px solid var(--color-border)' }}>
              <Field label="Resync lines" hint='"<count> | <regex>": when a log line matches, the count is set to that number. Use announcements that reveal the true count.'>
                <textarea value={syncs} onChange={(e) => setSyncs(e.target.value)} rows={3} className={ta} style={inputStyle} />
              </Field>
              <Field label="Reset when a line matches (regex)" hint="Sets the count to 0 and stops the clock, e.g. the event's start emote.">
                <input value={resetPattern} onChange={(e) => setResetPattern(e.target.value)} className={ta} style={inputStyle} />
              </Field>
              <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
                <input type="checkbox" checked={clockOn} onChange={(e) => setClockOn(e.target.checked)} />
                Event clock
              </label>
              {clockOn && (
                <>
                  <div className="grid grid-cols-2 gap-3">
                    <Field label="Clock label">
                      <input value={clockLabel} onChange={(e) => setClockLabel(e.target.value)} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
                    </Field>
                    <Field label="Duration (minutes)">
                      <input value={clockMins} inputMode="numeric" onChange={(e) => setClockMins(e.target.value.replace(/[^0-9]/g, ''))} className="w-full text-xs rounded px-2 py-1 outline-none" style={inputStyle} />
                    </Field>
                  </div>
                  <Field label="Clock start lines" hint='"<seconds into event> | <regex>": a match pins the clock as if the event started that many seconds earlier. Use 0 for the real start line. Add "idle" before the bar to ignore the line while the clock is running, and "reset" to also zero the count.'>
                    <textarea value={clockMarks} onChange={(e) => setClockMarks(e.target.value)} rows={3} className={ta} style={inputStyle} />
                  </Field>
                  <Field label="Clock stop lines (regex, one per line)">
                    <textarea value={clockStops} onChange={(e) => setClockStops(e.target.value)} rows={2} className={ta} style={inputStyle} />
                  </Field>
                </>
              )}
            </div>
          )}

          {error && (
            <div className="flex items-start gap-2 rounded p-2" style={{ backgroundColor: 'var(--color-surface-2)' }}>
              <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
              <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <button onClick={onCancel} className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}>
            Cancel
          </button>
          <button onClick={submit} disabled={saving} className="text-xs px-3 py-1.5 rounded font-medium disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}>
            {d ? 'Save' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  )
}

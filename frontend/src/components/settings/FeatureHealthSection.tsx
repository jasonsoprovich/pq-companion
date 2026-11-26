/**
 * FeatureHealthSection — top of Settings › General. Everything PQ Companion
 * reads from the game in one list: the setup checks (folder, character, log
 * file), the switches that make data flow (log parsing, EverQuest logging at
 * login, Zeal output on camp), and Zeal's live pipe and in-game settings.
 *
 * Each row shows a status dot, its state and a one-click fix (a switch, a
 * copyable command or a link). Clicking the row expands what it does, which
 * features need it and how to turn it on. Data comes from useSetupHealth, the
 * same source as the Getting Started checklist.
 */
import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, ChevronDown, ChevronRight, Loader2, RefreshCw } from 'lucide-react'
import {
  SETUP_HEALTH_CHANGED,
  useSetupHealth,
  type HealthCheck,
  type HealthLevel,
} from '../../hooks/useSetupHealth'
import { getConfig, saveConfigChanges, setExportOnCamp, setLogging } from '../../services/api'
import type { ZealCapRow } from '../../types/zeal'
import { CopyCommand, isSlash } from '../ZealChecklist'

const DOT: Record<HealthLevel, string> = {
  ok: '#22c55e',
  warn: '#f59e0b',
  error: '#ef4444',
  off: 'var(--color-border)',
}

interface Explain {
  what: string
  needs?: string
  how?: React.ReactNode
}

// Explanations for the fixed rows, keyed by row id.
const EXPLAIN: Record<string, Explain> = {
  'eq-folder': {
    what: 'The folder that contains eqgame.exe. PQ Companion finds your log file, .ini files and Zeal exports there.',
    needs: 'Everything that reads the game.',
    how: 'Set it under EverQuest installation below.',
  },
  character: {
    what: 'Whose log file is read. Leave it on auto-detect and the app follows whoever logged in most recently.',
    needs: 'Every live feature.',
    how: 'Log in once with logging on, or pick a character under EverQuest installation.',
  },
  'log-file': {
    what: 'eqlog_<Name>_pq.proj.txt in your EverQuest folder. EverQuest creates it the first time you play with logging on.',
    needs: 'Every live feature.',
    how: 'Turn on EverQuest logging below, or type /log on in game.',
  },
  'log-activity': {
    what: 'When the log was last written. Grey just means EverQuest is closed or idle; it turns green within seconds of playing.',
  },
  parser: {
    what: 'PQ Companion reading new lines from your log file as you play.',
    needs: 'DPS meter, spell and buff timers, triggers, NPC info, maps and most overlays.',
    how: 'Use the switch. Only turn it off to pause everything that reads the log.',
  },
  'eq-logging': {
    what: 'Sets Log=TRUE in eqclient.ini so EverQuest writes your log every time you log in, without typing /log on.',
    needs: 'Every live feature, on every login.',
    how: 'Use the switch with EverQuest closed (the client overwrites eqclient.ini when it exits). A backup is saved first. In game, /log on works for the current session.',
  },
  'zeal-camp': {
    what: 'Zeal writes your inventory, spellbook and character stats to files when you /camp.',
    needs: 'Inventory, spellbook, keys, Character Info, Gear Upgrades and the bag value totals.',
    how: 'Use the switch with EverQuest closed. A backup of zeal.ini is saved first.',
  },
  zeal: {
    what: 'Zeal is a community EverQuest add-on. PQ Companion works without it and uses it for extra data when it is there.',
    needs: 'Inventory and spellbook exports, live target and HP, pet owners, live buffs and more.',
    how: 'Install or update it from the Zeal integration card below.',
  },
  'zeal-pipe': {
    what: 'A live connection to Zeal while EverQuest is running: your target, its HP, group, buffs and spawn data in real time.',
    needs: 'Live target in the NPC overlay, target HP trigger conditions, live buffs and the live map position.',
    how: 'Start EverQuest with Zeal loaded. It connects on its own.',
  },
}

// One row in the section.
function Row({
  id,
  level,
  label,
  detail,
  action,
  explain,
  tag,
}: {
  id: string
  level: HealthLevel
  label: string
  detail: string
  action?: React.ReactNode
  explain?: Explain
  tag?: string
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const canOpen = !!explain
  return (
    <li data-health-row={id} className="text-xs">
      <div className="flex items-start gap-2">
        <button
          type="button"
          onClick={() => canOpen && setOpen((v) => !v)}
          className="flex min-w-0 flex-1 items-start gap-2 text-left"
          style={{ cursor: canOpen ? 'pointer' : 'default' }}
          aria-expanded={canOpen ? open : undefined}
        >
          <span className="mt-0.5 w-3 shrink-0" style={{ color: 'var(--color-muted)' }}>
            {canOpen && (open ? <ChevronDown size={12} /> : <ChevronRight size={12} />)}
          </span>
          <span
            className="mt-1 inline-block h-2 w-2 shrink-0 rounded-full"
            style={{ backgroundColor: DOT[level] }}
          />
          <span className="w-36 shrink-0 font-medium" style={{ color: 'var(--color-foreground)' }}>
            {label}
            {tag && (
              <span
                className="ml-1.5 rounded px-1 text-[9px] uppercase tracking-wide"
                style={{ border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)' }}
              >
                {tag}
              </span>
            )}
          </span>
          <span
            className="min-w-0 flex-1 break-words"
            style={{ color: level === 'off' ? 'var(--color-muted)' : 'var(--color-muted-foreground)' }}
          >
            {detail}
          </span>
        </button>
        {action && <div className="shrink-0">{action}</div>}
      </div>
      {open && explain && (
        <div
          className="mb-1 ml-10 mt-1.5 space-y-1 rounded px-3 py-2"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
        >
          <p>{explain.what}</p>
          {explain.needs && (
            <p>
              <span style={{ color: 'var(--color-foreground)' }}>Used by: </span>
              {explain.needs}
            </p>
          )}
          {explain.how && (
            <div>
              <span style={{ color: 'var(--color-foreground)' }}>How to turn it on: </span>
              {explain.how}
            </div>
          )}
        </div>
      )}
    </li>
  )
}

function Group({
  title,
  anchor,
  children,
}: {
  title: string
  anchor?: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <div data-setting={anchor}>
      <h3
        className="mb-1.5 mt-3 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </h3>
      <ul className="space-y-1.5">{children}</ul>
    </div>
  )
}

function SmallButton({
  onClick,
  busy,
  primary,
  children,
}: {
  onClick: () => void
  busy?: boolean
  primary?: boolean
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      disabled={busy}
      className="flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium disabled:opacity-50"
      style={{
        border: '1px solid var(--color-border)',
        backgroundColor: primary ? 'var(--color-primary)' : 'transparent',
        color: primary ? '#fff' : 'var(--color-primary)',
        cursor: busy ? 'default' : 'pointer',
      }}
    >
      {busy && <Loader2 size={10} className="animate-spin" />}
      {children}
    </button>
  )
}

// Zeal checklist state → health level. Features that are off only warn when
// PQ Companion recommends them.
function capLevel(row: ZealCapRow): HealthLevel {
  if (row.state === 'working' || row.state === 'on') return 'ok'
  if (row.state === 'off') return row.recommended ? 'warn' : 'off'
  return 'off'
}

const CAP_STATE: Record<string, string> = {
  working: 'Working',
  on: 'On',
  off: 'Off',
  unknown: "Can't tell",
  not_seen: 'Not seen yet',
}

export default function FeatureHealthSection(): React.ReactElement {
  const { checks, diag, zealSettings, parserOn, loading, refresh, applyDiag } = useSetupHealth()
  const navigate = useNavigate()
  const [busy, setBusy] = useState<'parser' | 'log' | 'camp' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const byId = new Map<string, HealthCheck>(checks.map((c) => [c.id, c]))
  const check = (id: HealthCheck['id']): HealthCheck | undefined => byId.get(id)

  async function run(key: 'parser' | 'log' | 'camp', fn: () => Promise<void>): Promise<void> {
    setBusy(key)
    setError(null)
    try {
      await fn()
      window.dispatchEvent(new Event(SETUP_HEALTH_CHANGED))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(null)
    }
  }

  const toggleParser = (): Promise<void> =>
    run('parser', async () => {
      const c = await getConfig()
      await saveConfigChanges(
        { ...c, preferences: { ...c.preferences, parse_combat_log: !c.preferences?.parse_combat_log } },
        c,
      )
    })
  const toggleLog = (): Promise<void> =>
    run('log', async () => applyDiag(await setLogging(!diag?.log_enabled)))
  const toggleCamp = (): Promise<void> =>
    run('camp', async () => applyDiag(await setExportOnCamp(!diag?.export_on_camp)))

  // Rows beyond the core checks, built here so the summary counts them too.
  const zealInstalled = !!diag?.zeal_installed
  const logRow = {
    level: (!diag ? 'off' : diag.log_enabled ? 'ok' : 'warn') as HealthLevel,
    detail: !diag
      ? 'Set the EverQuest folder first.'
      : diag.log_enabled
        ? 'On at every login (eqclient.ini Log=TRUE)'
        : diag.log_found
          ? 'Off at login. Type /log on each session, or turn it on here.'
          : 'Not set in eqclient.ini. Type /log on each session, or turn it on here.',
  }
  const campRow = {
    level: (!zealInstalled ? 'off' : diag?.export_on_camp ? 'ok' : 'warn') as HealthLevel,
    detail: !zealInstalled
      ? 'Needs Zeal.'
      : diag?.export_on_camp
        ? 'On. Character data refreshes when you /camp.'
        : 'Off. Inventory and spellbook won’t refresh on /camp.',
  }
  // Zeal's checklist also lists output on camp; the Logging group already
  // has it with a switch, so it isn't repeated here.
  const caps = zealSettings?.installed
    ? zealSettings.capabilities.filter((c) => c.id !== 'export_on_camp')
    : []
  const levels: HealthLevel[] = [
    ...checks.filter((c) => c.id !== 'parser').map((c) => c.level),
    parserOn ? 'ok' : 'error',
    logRow.level,
    campRow.level,
    ...caps.map(capLevel),
  ]
  const problems = levels.filter((l) => l === 'error').length
  const warnings = levels.filter((l) => l === 'warn').length

  const fixButton = (c?: HealthCheck): React.ReactNode =>
    c?.fix ? <SmallButton onClick={() => navigate(c.fix!.to)}>{c.fix.label}</SmallButton> : undefined

  const checkRow = (id: HealthCheck['id'], explainId: string = id): React.ReactNode => {
    const c = check(id)
    if (!c) return null
    return (
      <Row key={id} id={id} level={c.level} label={c.label} detail={c.detail} action={fixButton(c)} explain={EXPLAIN[explainId]} />
    )
  }

  return (
    <section
      data-setting="setup-health"
      data-tour="settings-health"
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center gap-2">
        <Activity size={14} style={{ color: 'var(--color-primary)' }} />
        <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Feature health
        </h2>
        <span className="ml-auto text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {loading
            ? ''
            : problems > 0
              ? `${problems} problem${problems === 1 ? '' : 's'}`
              : warnings > 0
                ? `${warnings} to check`
                : 'All good'}
        </span>
        <button
          onClick={refresh}
          className="rounded p-1"
          style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
          title="Check again"
        >
          <RefreshCw size={12} />
        </button>
      </div>
      <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Everything PQ Companion reads from the game, and whether it&apos;s on. Click a row to see what it does and how
        to turn it on.
      </p>

      {error && (
        <p className="mt-2 text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}

      {loading && checks.length === 0 ? (
        <div className="mt-3 flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <Loader2 size={12} className="animate-spin" /> Checking…
        </div>
      ) : (
        <>
          <Group title="Setup">
            {checkRow('eq-folder')}
            {checkRow('character')}
            {checkRow('log-file')}
            {checkRow('log-activity')}
          </Group>

          <Group title="Logging" anchor="eq-logging">
            <Row
              id="parser"
              level={parserOn ? 'ok' : 'error'}
              label="Log parsing"
              detail={parserOn ? 'On' : 'Off. Live features (DPS, timers, triggers, overlays) need it.'}
              explain={EXPLAIN.parser}
              action={
                <SmallButton onClick={() => void toggleParser()} busy={busy === 'parser'} primary={!parserOn}>
                  {parserOn ? 'Turn off' : 'Turn on'}
                </SmallButton>
              }
            />
            <Row
              id="eq-logging"
              level={logRow.level}
              label="EverQuest logging"
              detail={logRow.detail}
              explain={EXPLAIN['eq-logging']}
              action={
                diag && (
                  <SmallButton onClick={() => void toggleLog()} busy={busy === 'log'} primary={!diag.log_enabled}>
                    {diag.log_enabled ? 'Turn off' : 'Turn on'}
                  </SmallButton>
                )
              }
            />
            <Row
              id="zeal-camp"
              level={campRow.level}
              label="Zeal output on camp"
              detail={campRow.detail}
              explain={EXPLAIN['zeal-camp']}
              action={
                zealInstalled && (
                  <SmallButton
                    onClick={() => void toggleCamp()}
                    busy={busy === 'camp'}
                    primary={!diag?.export_on_camp}
                  >
                    {diag?.export_on_camp ? 'Turn off' : 'Turn on'}
                  </SmallButton>
                )
              }
            />
            <li className="ml-5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              The switches rewrite eqclient.ini / zeal.ini. Close EverQuest first, since it overwrites them on exit. A
              backup is saved automatically.
            </li>
          </Group>

          <Group title="Zeal">
            {checkRow('zeal')}
            {checkRow('zeal-pipe')}
            {caps.map((r) => (
              <Row
                key={r.id}
                id={`zeal-cap-${r.id}`}
                level={capLevel(r)}
                label={r.label}
                tag={r.pending ? 'test build' : undefined}
                detail={
                  r.value && (r.state === 'on' || r.state === 'working')
                    ? `${CAP_STATE[r.state]} (${r.value})`
                    : CAP_STATE[r.state] ?? r.state
                }
                action={
                  r.state !== 'working' && r.state !== 'on' && isSlash(r.command) ? (
                    <CopyCommand cmd={r.command} />
                  ) : undefined
                }
                explain={{
                  what: r.benefit,
                  how:
                    r.command || r.hint ? (
                      <>
                        {r.command && (isSlash(r.command) ? <>Type <code>{r.command}</code> in game.</> : r.command)}
                        {r.hint && <> {r.hint}</>}
                      </>
                    ) : undefined,
                }}
              />
            ))}
            {caps.length > 0 && (
              <li className="ml-5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Type commands in the EverQuest chat box; Zeal saves them in zeal.ini and this list updates on its own.
                {zealSettings?.pipe_connected && !zealSettings.pipe_matches &&
                  ' The connected game isn’t running from this EverQuest folder, so live checks don’t apply here.'}
              </li>
            )}
          </Group>
        </>
      )}
    </section>
  )
}

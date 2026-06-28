import React, { useEffect, useState, Suspense } from 'react'
import { Code2, AlertTriangle, FileText, Database, Network, FlaskConical, Trash2 } from 'lucide-react'
import SqlSandboxPanel from './SqlSandboxPanel'
// Lazy-loaded: SchemaGraphPanel pulls in @xyflow/react (~4MB) + dagre. Keeping
// it out of the static import graph removes those heavy deps from the app's
// boot bundle — only Dev-tab users who open the graph sub-tab pay the cost.
const SchemaGraphPanel = React.lazy(() => import('./SchemaGraphPanel'))
import { getConfig, updateConfig } from '../services/api'
import type { Config } from '../types/config'

// DeveloperTab hosts power-user tools that don't belong in the regular UI.
// Only visible when Preferences.DeveloperMode is true; toggled by
// Ctrl+Shift+D while the Settings page is focused. Each sub-tool gets its
// own sub-tab so we don't pile three large panels onto one scroll-strip.

type SubTab = 'notes' | 'sandbox' | 'graph' | 'flags'

interface SubTabDef {
  id: SubTab
  label: string
  icon: React.ReactNode
}

const SUB_TABS: SubTabDef[] = [
  { id: 'notes', label: 'Notes', icon: <FileText size={13} /> },
  { id: 'sandbox', label: 'SQL Sandbox', icon: <Database size={13} /> },
  { id: 'graph', label: 'Schema Graph', icon: <Network size={13} /> },
  { id: 'flags', label: 'Flags', icon: <FlaskConical size={13} /> },
]

export default function DeveloperTab(): React.ReactElement {
  const [sub, setSub] = useState<SubTab>('sandbox')

  return (
    <div className="flex h-full flex-col">
      {/* Page header */}
      <div className="flex shrink-0 items-center gap-2 px-6 pt-6">
        <Code2 size={18} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Developer
        </h1>
      </div>

      {/* Sub-tab bar */}
      <div
        className="mt-3 flex shrink-0 border-b px-6"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {SUB_TABS.map((t) => {
          const active = sub === t.id
          return (
            <button
              key={t.id}
              type="button"
              onClick={() => setSub(t.id)}
              className="flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition-colors"
              style={{
                borderBottomColor: active ? 'var(--color-primary)' : 'transparent',
                color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                backgroundColor: 'transparent',
                cursor: 'pointer',
              }}
            >
              {t.icon}
              {t.label}
            </button>
          )
        })}
      </div>

      {/* Sub-tab body — fills width, scrolls vertically */}
      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
        {sub === 'notes' && <NotesPanel />}
        {sub === 'sandbox' && <SqlSandboxPanel />}
        {sub === 'graph' && (
          <Suspense fallback={<div className="text-sm text-zinc-500">Loading schema graph…</div>}>
            <SchemaGraphPanel />
          </Suspense>
        )}
        {sub === 'flags' && <FlagsPanel />}
      </div>
    </div>
  )
}

// FlagsPanel hosts experimental era/feature switches. Currently just the
// Planes of Power preview: flipping it raises the level cap to 65 and
// reveals PoP spells, AA tabs, and Plane of Knowledge shopping app-wide
// (see backend internal/era). Stored in config.yaml so it survives
// restarts; consumers track changes live via the config:updated broadcast.
// RAID_CLASSES mirrors internal/raidthreat. There is no built-in per-class
// boost any more (tanks are handled by modelling taunt); these knobs are opt-in
// for a known aggro-mod gear/AA setup, so every default is 0.
const RAID_CLASSES = [
  'Warrior', 'Shadow Knight', 'Paladin', 'Cleric', 'Druid', 'Shaman',
  'Ranger', 'Monk', 'Rogue', 'Bard', 'Beastlord',
  'Necromancer', 'Wizard', 'Magician', 'Enchanter',
]

function FlagsPanel(): React.ReactElement {
  const [config, setConfig] = useState<Config | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [newPlayer, setNewPlayer] = useState('')
  const [newPlayerPct, setNewPlayerPct] = useState('')

  useEffect(() => {
    getConfig()
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
  }, [])

  const popEnabled = Boolean(config?.preferences?.pop_enabled)

  const togglePoP = (): void => {
    if (!config || saving) return
    setSaving(true)
    setError(null)
    updateConfig({
      ...config,
      preferences: { ...config.preferences, pop_enabled: !popEnabled },
    })
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  // ── Raid-estimate threat ──────────────────────────────────────────────
  const raidEnabled = Boolean(config?.preferences?.raid_threat_enabled)
  const classMods = config?.preferences?.raid_threat_class_mods ?? {}
  const playerMods = config?.preferences?.raid_threat_player_mods ?? {}

  // savePrefs persists a Preferences patch (merged over the current config).
  const savePrefs = (patch: Record<string, unknown>): void => {
    if (!config || saving) return
    setSaving(true)
    setError(null)
    updateConfig({ ...config, preferences: { ...config.preferences, ...patch } })
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const toggleRaid = (): void => savePrefs({ raid_threat_enabled: !raidEnabled })

  // setClassMod writes (or clears, when blank) a per-class adjustment.
  const setClassMod = (cls: string, raw: string): void => {
    const next = { ...classMods }
    if (raw.trim() === '') delete next[cls]
    else next[cls] = Math.max(-100, Math.min(500, Math.round(Number(raw) || 0)))
    savePrefs({ raid_threat_class_mods: next })
  }

  const addPlayerMod = (name: string, raw: string): void => {
    const n = name.trim()
    if (n === '') return
    const next = { ...playerMods, [n]: Math.max(-100, Math.min(500, Math.round(Number(raw) || 0))) }
    savePrefs({ raid_threat_player_mods: next })
  }

  const removePlayerMod = (name: string): void => {
    const next = { ...playerMods }
    delete next[name]
    savePrefs({ raid_threat_player_mods: next })
  }

  return (
    <div className="flex flex-col gap-4">
      <section
        className="rounded-lg p-4"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
        }}
      >
        <div className="mb-3 flex items-center gap-2">
          <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
          <h2
            className="text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Planes of Power preview
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Switches the app into Planes of Power era before the expansion
            launches on Project Quarm: level cap 65 instead of 60, PoP spells
            in the class spell lists, the PoP AA tabs, and Plane of Knowledge
            as a shopping-route source. The server itself is still pre-PoP, so
            leave this off for normal play — it exists so PoP support can be
            tested ahead of launch.
          </p>
          <button
            type="button"
            onClick={togglePoP}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: popEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: popEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {popEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
        {error && (
          <p className="mt-2 text-xs" style={{ color: '#f87171' }}>
            {error}
          </p>
        )}
      </section>

      {/* ── Raid-estimate threat ─────────────────────────────────────────── */}
      <section
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="mb-3 flex items-center gap-2">
          <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Raid threat (estimated)
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a <strong>Solo / Raid</strong> toggle to the Threat overlay. Raid
            mode estimates every nearby player&rsquo;s hate from the damage they
            do — the same data the DPS meter uses — and pins a tank to the top
            when it sees their taunt land. It is approximate: out-of-range
            players, others&rsquo; DoTs, heals, and utility casts aren&rsquo;t in
            your log, so DoT/healer classes read low. The per-class field below
            is optional (default 0) for dialing in a known aggro-mod setup.
          </p>
          <button
            type="button"
            onClick={toggleRaid}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: raidEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: raidEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {raidEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>

        {raidEnabled && (
          <>
            {/* Per-class hate adjustment */}
            <div className="mt-4">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                Per-class hate adjustment (%)
              </p>
              <div className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
                {RAID_CLASSES.map((cls) => {
                  const set = classMods[cls]
                  return (
                    <label key={cls} className="flex items-center justify-between gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      <span className="truncate">{cls}</span>
                      <input
                        type="number"
                        defaultValue={set ?? ''}
                        key={set ?? 'unset'}
                        placeholder="0"
                        onBlur={(e) => setClassMod(cls, e.target.value)}
                        className="w-14 rounded px-1 py-0.5 text-right text-xs"
                        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                      />
                    </label>
                  )
                })}
              </div>
              <p className="mt-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Blank = 0 (no adjustment). Applied to a player&rsquo;s observed
                damage — opt-in for known aggro-mod gear/AAs.
              </p>
            </div>

            {/* Per-player override */}
            <div className="mt-4">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                Per-player override (%)
              </p>
              {Object.keys(playerMods).length > 0 && (
                <div className="mb-2 flex flex-col gap-1">
                  {Object.entries(playerMods).map(([name, pct]) => (
                    <div key={name} className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      <span className="w-28 truncate">{name}</span>
                      <span className="w-12 text-right">{pct > 0 ? '+' : ''}{pct}%</span>
                      <button type="button" onClick={() => removePlayerMod(name)} title="Remove" style={{ color: '#f87171', cursor: 'pointer' }}>
                        <Trash2 size={12} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              <div className="flex items-center gap-2">
                <input
                  value={newPlayer}
                  onChange={(e) => setNewPlayer(e.target.value)}
                  placeholder="Player name"
                  className="w-28 rounded px-2 py-0.5 text-xs"
                  style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                />
                <input
                  value={newPlayerPct}
                  onChange={(e) => setNewPlayerPct(e.target.value)}
                  type="number"
                  placeholder="%"
                  className="w-14 rounded px-1 py-0.5 text-right text-xs"
                  style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                />
                <button
                  type="button"
                  onClick={() => { addPlayerMod(newPlayer, newPlayerPct); setNewPlayer(''); setNewPlayerPct('') }}
                  className="rounded px-2 py-0.5 text-xs"
                  style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
                >
                  Add
                </button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  )
}

// NotesPanel is a static disclaimer explaining what the Developer tab is
// for, why the schema can shift between releases, and how to hide the
// tab again. Lives here so the user has somewhere to read it without
// the warning cluttering every sub-panel header.
function NotesPanel(): React.ReactElement {
  return (
    <div className="flex flex-col gap-4">
      <section
        className="rounded-lg p-4"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
        }}
      >
        <div className="mb-3 flex items-center gap-2">
          <AlertTriangle size={14} style={{ color: '#f59e0b' }} />
          <h2
            className="text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            About these tools
          </h2>
        </div>
        <div className="space-y-3 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          <p>
            The Developer tab exposes PQ Companion&rsquo;s internal database
            directly. Table and column names are the upstream Project Quarm
            schema and may change between database refreshes &mdash; queries
            you save or share with other players may need updating after a
            release. Treat the schema as an unstable internal surface, not a
            documented API.
          </p>
          <p>
            The <strong style={{ color: 'var(--color-foreground)' }}>SQL Sandbox</strong>{' '}
            runs SELECT-only statements against a read-only connection to{' '}
            <code>quarm.db</code>. Queries are capped at 10,000 rows and 8
            seconds; the connection is separate from the main app pool, so a
            runaway query here won&rsquo;t affect the item, NPC, or spell
            pages.
          </p>
          <p>
            The <strong style={{ color: 'var(--color-foreground)' }}>Schema Graph</strong>{' '}
            tab renders an interactive map of every game-content table in
            quarm.db. Click a table to focus on its neighbours, drag nodes
            around, scroll to zoom, or type into the search box to filter
            down to one part of the schema. quarm.db has no declared foreign
            keys &mdash; relationships are hand-maintained, so if an edge
            looks wrong after a database refresh, please file an issue.
          </p>
          <p>
            To hide this tab again, press <kbd>Ctrl+Shift+D</kbd> anywhere
            on the Settings page. The flag is stored in{' '}
            <code>~/.pq-companion/config.yaml</code> as{' '}
            <code>preferences.developer_mode</code>.
          </p>
        </div>
      </section>
    </div>
  )
}

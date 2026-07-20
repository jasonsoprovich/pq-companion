import React, { useEffect, useState, Suspense } from 'react'
import { Code2, AlertTriangle, FileText, Database, Network, FlaskConical } from 'lucide-react'
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
function FlagsPanel(): React.ReactElement {
  const [config, setConfig] = useState<Config | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

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

  // ── PoP flagging ──────────────────────────────────────────────────────
  const popFlagsEnabled = Boolean(config?.preferences?.pop_flags_enabled)
  const togglePopFlags = (): void => savePrefs({ pop_flags_enabled: !popFlagsEnabled })

  // ── Session Faction Tracker ───────────────────────────────────────────
  const factionTrackerEnabled = Boolean(config?.preferences?.faction_tracker_enabled)
  const toggleFactionTracker = (): void =>
    savePrefs({ faction_tracker_enabled: !factionTrackerEnabled })

  // ── Raid-wide threat meter ────────────────────────────────────────────
  const raidThreatEnabled = Boolean(config?.preferences?.raid_threat_enabled)
  const toggleRaidThreat = (): void => savePrefs({ raid_threat_enabled: !raidThreatEnabled })

  return (
    <div className="flex flex-col gap-4">
      <div
        className="flex items-start gap-2 rounded-lg px-4 py-3"
        style={{
          border: '1px solid var(--color-warning, #f59e0b)',
          backgroundColor: 'color-mix(in srgb, var(--color-warning, #f59e0b) 12%, transparent)',
        }}
      >
        <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: 'var(--color-warning, #f59e0b)' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          <strong style={{ color: 'var(--color-warning, #f59e0b)' }}>Active development.</strong>{' '}
          These features are still being built and tested — expect bugs, incomplete
          behavior, and breaking changes between releases. Enable them at your own risk.
        </p>
      </div>
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

      {/* ── Raid-wide threat meter ────────────────────────────────────────── */}
      <section
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="mb-3 flex items-center gap-2">
          <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Raid-wide threat meter
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a <strong>Solo / Raid</strong> toggle to the Threat overlay. Raid
            mode estimates every nearby player&rsquo;s hate from the damage in your
            log. It is unreliable: out-of-range players, other players&rsquo; DoTs,
            heals, taunts, and utility casts aren&rsquo;t in your log, so DoT and
            healer classes read far too low and callouts can mislead. The Solo
            (personal) meter is always available and unaffected by this toggle —
            enable Raid mode only if you want to experiment with it. Per-class and
            per-player tuning appears in Settings &rsaquo; Overlays once enabled.
          </p>
          <button
            type="button"
            onClick={toggleRaidThreat}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: raidThreatEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: raidThreatEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {raidThreatEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
      </section>

      {/* ── PoP flagging ──────────────────────────────────────────────────── */}
      <section
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="mb-3 flex items-center gap-2">
          <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            PoP flagging tracker
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a <strong>PoP Flags</strong> page: a per-character checklist of
            the Planes of Power planar-progression flags (Tiers 1&ndash;4 +
            Plane of Time), with prerequisite locking. Flags live server-side as
            qglobals that aren&rsquo;t in the game DB or Zeal, so for now every
            step is a manual toggle &mdash; auto-detection from the Seer NPC and
            live log lines comes once PoP is live on Quarm.
          </p>
          <button
            type="button"
            onClick={togglePopFlags}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: popFlagsEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: popFlagsEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {popFlagsEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
      </section>

      {/* ── Session Faction Tracker ─────────────────────────────────────── */}
      <section
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="mb-3 flex items-center gap-2">
          <FlaskConical size={14} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Session Faction Tracker
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a per-character faction wishlist and a <strong>Factions</strong>{' '}
            page that tallies &ldquo;Your faction standing with X got
            better/worse&rdquo; lines for the factions you&rsquo;ve starred, with a
            best-effort point estimate when a change can be tied to a resolved
            kill. EQ never logs a faction&rsquo;s absolute value or point amount, so
            this is always an approximate, session-only view &mdash; it resets
            on every restart and character switch, and can&rsquo;t tell you your
            real standing.
          </p>
          <button
            type="button"
            onClick={toggleFactionTracker}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: factionTrackerEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: factionTrackerEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {factionTrackerEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
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

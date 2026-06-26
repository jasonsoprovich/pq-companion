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
  const resistCalcEnabled = Boolean(config?.preferences?.resist_calc_enabled)
  const traderTrackerEnabled = Boolean(config?.preferences?.trader_tracker_enabled)
  const charmPetFinderEnabled = Boolean(config?.preferences?.charm_pet_finder_enabled)

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

  const toggleResistCalc = (): void => {
    if (!config || saving) return
    setSaving(true)
    setError(null)
    updateConfig({
      ...config,
      preferences: { ...config.preferences, resist_calc_enabled: !resistCalcEnabled },
    })
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const toggleTraderTracker = (): void => {
    if (!config || saving) return
    setSaving(true)
    setError(null)
    updateConfig({
      ...config,
      preferences: { ...config.preferences, trader_tracker_enabled: !traderTrackerEnabled },
    })
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const toggleCharmPetFinder = (): void => {
    if (!config || saving) return
    setSaving(true)
    setError(null)
    updateConfig({
      ...config,
      preferences: { ...config.preferences, charm_pet_finder_enabled: !charmPetFinderEnabled },
    })
      .then(setConfig)
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
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
            Resist calculator
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a Resist Calculator page (under Database in the nav) that
            estimates the odds a spell lands on a targeted NPC, given your level
            and the NPC&rsquo;s resists. The underlying resist math is a
            community-reverse-engineered, worst-case approximation &mdash; this
            is a power-user/testing feature while its accuracy is validated, so
            it&rsquo;s off by default.
          </p>
          <button
            type="button"
            onClick={toggleResistCalc}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: resistCalcEnabled ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: resistCalcEnabled ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {resistCalcEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
        {error && (
          <p className="mt-2 text-xs" style={{ color: '#f87171' }}>
            {error}
          </p>
        )}
      </section>

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
            Bazaar trader tracker
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a Trader Tracker page that infers what a parked bazaar trader
            sold by diffing inventory exports. The bazaar keeps no sales log, so
            this relies on you running <code>/output inventory</code> before and
            after a selling session &mdash; it&rsquo;s a best-guess, power-user
            feature while the inventory-diff approach is validated in a live
            bazaar, so it&rsquo;s off by default.
          </p>
          <button
            type="button"
            onClick={toggleTraderTracker}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: traderTrackerEnabled
                ? 'var(--color-primary)'
                : 'var(--color-surface-2)',
              color: traderTrackerEnabled
                ? 'var(--color-background)'
                : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {traderTrackerEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
        {error && (
          <p className="mt-2 text-xs" style={{ color: '#f87171' }}>
            {error}
          </p>
        )}
      </section>

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
            Charm pet finder
          </h2>
        </div>
        <div className="flex items-start justify-between gap-4">
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Adds a Charm Pet Finder page (under Database in the nav) that lists a
            zone&rsquo;s charmable NPCs for your charm class and spell, ranked by
            melee DPS, with level-cap warnings and per-NPC charm land odds. The
            DPS/HP scaling and land-chance math are best-effort estimates, so
            it&rsquo;s a power-user feature that&rsquo;s off by default.
          </p>
          <button
            type="button"
            onClick={toggleCharmPetFinder}
            disabled={!config || saving}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: charmPetFinderEnabled
                ? 'var(--color-primary)'
                : 'var(--color-surface-2)',
              color: charmPetFinderEnabled
                ? 'var(--color-background)'
                : 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: !config || saving ? 'default' : 'pointer',
              opacity: !config || saving ? 0.6 : 1,
            }}
          >
            {charmPetFinderEnabled ? 'Enabled' : 'Disabled'}
          </button>
        </div>
        {error && (
          <p className="mt-2 text-xs" style={{ color: '#f87171' }}>
            {error}
          </p>
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

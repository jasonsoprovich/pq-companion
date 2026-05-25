import React, { useState } from 'react'
import { Code2, AlertTriangle, FileText, Database, Network } from 'lucide-react'
import SqlSandboxPanel from './SqlSandboxPanel'
import SchemaDiagramsPanel from './SchemaDiagramsPanel'

// DeveloperTab hosts power-user tools that don't belong in the regular UI.
// Only visible when Preferences.DeveloperMode is true; toggled by
// Ctrl+Shift+D while the Settings page is focused. Each sub-tool gets its
// own sub-tab so we don't pile three large panels onto one scroll-strip.

type SubTab = 'notes' | 'sandbox' | 'diagrams'

interface SubTabDef {
  id: SubTab
  label: string
  icon: React.ReactNode
}

const SUB_TABS: SubTabDef[] = [
  { id: 'notes', label: 'Notes', icon: <FileText size={13} /> },
  { id: 'sandbox', label: 'SQL Sandbox', icon: <Database size={13} /> },
  { id: 'diagrams', label: 'Schema Diagrams', icon: <Network size={13} /> },
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
        {sub === 'diagrams' && <SchemaDiagramsPanel />}
      </div>
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
            The <strong style={{ color: 'var(--color-foreground)' }}>Schema Diagrams</strong>{' '}
            tab renders four curated ER diagrams covering Items &amp; Loot,
            NPCs &amp; Spawns, Spells &amp; AAs, and Keys &amp; Zones.
            quarm.db has no declared foreign keys, so these relationships are
            hand-maintained &mdash; if one looks wrong after a database
            refresh, please file an issue.
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

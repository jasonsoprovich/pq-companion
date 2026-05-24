import React from 'react'
import { Code2, AlertTriangle } from 'lucide-react'
import SqlSandboxPanel from './SqlSandboxPanel'

// DeveloperTab hosts power-user tools that don't belong in the regular UI:
// SQL sandbox, schema explorer, and (planned) ER diagram viewer. Only
// visible when Preferences.DeveloperMode is true; toggled by Ctrl+Shift+D
// while the Settings page is focused.
export default function DeveloperTab(): React.ReactElement {
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
          <Code2 size={16} style={{ color: 'var(--color-primary)' }} />
          <h2
            className="text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Developer
          </h2>
        </div>

        <div
          className="flex items-start gap-2 rounded px-3 py-2 text-xs"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted-foreground)',
          }}
        >
          <AlertTriangle size={14} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
          <p>
            These tools expose PQ Companion's internal database directly. Table
            and column names are the upstream Project Quarm schema and may
            change between database refreshes — shared queries may need
            updating after a release. Press <kbd>Ctrl+Shift+D</kbd> on this
            page again to hide this tab.
          </p>
        </div>
      </section>

      <SqlSandboxPanel />
    </div>
  )
}

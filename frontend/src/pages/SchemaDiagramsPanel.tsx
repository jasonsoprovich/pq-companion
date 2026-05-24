import React, { useEffect, useRef, useState } from 'react'
import { Network, AlertTriangle, Loader2 } from 'lucide-react'
import { SCHEMA_DIAGRAMS } from '../lib/schemaDiagrams'

// SchemaDiagramsPanel renders one of the curated Mermaid erDiagrams from
// lib/schemaDiagrams. Mermaid is lazy-loaded (~2MB) so this code only
// pulls into the bundle when the Developer tab is opened.

type MermaidModule = {
  default: {
    initialize: (cfg: Record<string, unknown>) => void
    render: (id: string, src: string) => Promise<{ svg: string }>
  }
}

export default function SchemaDiagramsPanel(): React.ReactElement {
  const [selectedId, setSelectedId] = useState<string>(SCHEMA_DIAGRAMS[0].id)
  const [svg, setSvg] = useState<string | null>(null)
  const [renderError, setRenderError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const containerRef = useRef<HTMLDivElement | null>(null)
  // Cache the loaded mermaid module across renders so flipping between
  // diagrams doesn't re-import the ~2MB bundle every time.
  const mermaidRef = useRef<MermaidModule['default'] | null>(null)

  const selected = SCHEMA_DIAGRAMS.find((d) => d.id === selectedId) ?? SCHEMA_DIAGRAMS[0]

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setRenderError(null)
    ;(async () => {
      try {
        if (!mermaidRef.current) {
          const mod = (await import('mermaid')) as MermaidModule
          mod.default.initialize({
            startOnLoad: false,
            theme: 'dark',
            securityLevel: 'strict',
            // The default ER renderer is fine; just tighten the spacing
            // so 7–8 entities fit without horizontal scrolling at
            // typical Settings widths.
            er: { entityPadding: 8, layoutDirection: 'TB' },
          })
          mermaidRef.current = mod.default
        }
        const m = mermaidRef.current
        // Mermaid requires the diagram id be a valid CSS selector; the
        // diagram ids are already kebab-case so they work as-is.
        const { svg } = await m.render(`mmd-${selected.id}`, selected.mermaid)
        if (!cancelled) setSvg(svg)
      } catch (e) {
        if (!cancelled) {
          setRenderError((e as Error).message ?? String(e))
          setSvg(null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [selected.id, selected.mermaid])

  return (
    <section
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="mb-3 flex items-center gap-2">
        <Network size={14} style={{ color: 'var(--color-primary)' }} />
        <h3
          className="text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          Schema Diagrams
        </h3>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        {SCHEMA_DIAGRAMS.map((d) => {
          const active = d.id === selected.id
          return (
            <button
              key={d.id}
              type="button"
              onClick={() => setSelectedId(d.id)}
              className="rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                color: active ? '#fff' : 'var(--color-foreground)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
              }}
            >
              {d.label}
            </button>
          )
        })}
      </div>

      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        {selected.description}
      </p>

      <div
        className="rounded p-3"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          minHeight: 240,
          overflow: 'auto',
        }}
      >
        {loading && !svg && (
          <p className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <Loader2 size={12} className="animate-spin" />
            Rendering diagram…
          </p>
        )}
        {renderError && (
          <div
            className="flex items-start gap-2 text-xs"
            style={{ color: '#f87171' }}
          >
            <AlertTriangle size={14} className="mt-0.5 shrink-0" />
            <pre className="whitespace-pre-wrap break-all font-mono">{renderError}</pre>
          </div>
        )}
        {svg && (
          <div
            ref={containerRef}
            // Mermaid SVG is sanitized by the strict security level
            // before being returned, so it's safe to inject here.
            dangerouslySetInnerHTML={{ __html: svg }}
            style={{ maxWidth: '100%' }}
          />
        )}
      </div>
    </section>
  )
}

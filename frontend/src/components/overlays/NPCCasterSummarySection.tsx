import React from 'react'
import type { NPCCasterSummary } from '../../types/overlay'
import type { NPCOverlaySections } from '../../types/config'

// Theme tokens the section needs. The dashboard panel passes CSS-variable
// colours; the floating overlay window passes rgba literals (it can't rely on
// the app's theme stylesheet). Keeps one renderer, two visual idioms.
export interface CasterSummaryTheme {
  heading: string
  muted: string
  chipBg: string
  chipText: string
}

// Danger highlights (combat threats) are red regardless of theme; info
// highlights fall back to the neutral chip colours.
const DANGER_BG = 'rgba(220, 38, 38, 0.18)'
const DANGER_TEXT = '#f87171'

function chunkLabel(kind?: string): string {
  switch (kind) {
    case 'attack':
      return 'on hit'
    case 'range':
      return 'on ranged'
    case 'defensive':
      return 'when hit'
    default:
      return ''
  }
}

// NPCCasterSummarySection renders the distilled caster-AI readout: curated
// highlight chips, procs, named signature spells, and inherited class lists
// collapsed to a count. Inherited lists are never enumerated — that's the whole
// point of the feature (a geonid shaman shouldn't dump 66 shaman spells).
//
// Returns null when the master toggle is off or there's nothing to show, so
// callers can render it unconditionally.
export default function NPCCasterSummarySection({
  summary,
  sections,
  theme,
}: {
  summary: NPCCasterSummary
  sections: NPCOverlaySections
  theme: CasterSummaryTheme
}): React.ReactElement | null {
  if (!sections.spells) return null

  const highlights = summary.highlights ?? []
  const procs = sections.spells_procs ? summary.procs ?? [] : []
  const signature = sections.spells_signature ? summary.signature ?? [] : []
  const overflow = sections.spells_signature ? summary.signature_overflow ?? 0 : 0
  const classLists = sections.spells_class ? summary.class_lists ?? [] : []

  if (
    highlights.length === 0 &&
    procs.length === 0 &&
    signature.length === 0 &&
    classLists.length === 0
  ) {
    return null
  }

  const headingStyle: React.CSSProperties = {
    marginBottom: 4,
    fontSize: 9,
    fontWeight: 600,
    textTransform: 'uppercase',
    letterSpacing: '0.08em',
    color: theme.heading,
  }
  const rowLabelStyle: React.CSSProperties = {
    fontSize: 10,
    fontWeight: 600,
    color: theme.muted,
    marginRight: 4,
  }

  return (
    <div>
      <p style={headingStyle}>Spells &amp; Abilities</p>

      {highlights.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: signature.length || procs.length || classLists.length ? 4 : 0 }}>
          {highlights.map((h) => (
            <span
              key={h.tag}
              style={{
                fontSize: 10,
                fontWeight: 600,
                borderRadius: 3,
                padding: '1px 6px',
                backgroundColor: h.severity === 'danger' ? DANGER_BG : theme.chipBg,
                color: h.severity === 'danger' ? DANGER_TEXT : theme.chipText,
              }}
            >
              {h.label}
            </span>
          ))}
        </div>
      )}

      {procs.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 4, marginBottom: 2 }}>
          <span style={rowLabelStyle}>Procs:</span>
          <span style={{ fontSize: 10, color: theme.chipText }}>
            {procs
              .map((p) => {
                const ctx = chunkLabel(p.kind)
                const chance = p.chance ? ` ${p.chance}%` : ''
                return `${p.spell_name}${chance}${ctx ? ` (${ctx})` : ''}`
              })
              .join(', ')}
          </span>
        </div>
      )}

      {signature.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 4, marginBottom: 2 }}>
          <span style={rowLabelStyle}>Signature:</span>
          <span style={{ fontSize: 10, color: theme.chipText }}>
            {signature.map((s) => s.spell_name).join(', ')}
            {overflow > 0 && <span style={{ color: theme.muted }}>{`, +${overflow} more`}</span>}
          </span>
        </div>
      )}

      {classLists.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 4 }}>
          <span style={rowLabelStyle}>Class:</span>
          <span style={{ fontSize: 10, color: theme.muted }}>
            {classLists
              .map((c) => `${c.list_name} (${c.count} spell${c.count === 1 ? '' : 's'})`)
              .join(' · ')}
          </span>
        </div>
      )}
    </div>
  )
}

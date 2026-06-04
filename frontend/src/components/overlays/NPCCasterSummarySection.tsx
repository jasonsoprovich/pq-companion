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

// SpellName renders a spell as a clickable link when onClick is supplied (the
// DB page wires it to the spell explorer), and as plain text otherwise (the
// floating overlay windows have no in-window navigation target). Mirrors the
// SpellLink used by the full enumerated list so the summary feels the same.
function SpellName({
  id,
  name,
  onClick,
}: {
  id: number
  name: string
  onClick?: (id: number) => void
}): React.ReactElement {
  if (!onClick || id <= 0) return <>{name}</>
  return (
    <button
      type="button"
      onClick={() => onClick(id)}
      className="hover:underline"
      style={{ color: 'var(--color-primary)', cursor: 'pointer', background: 'none', border: 'none', padding: 0, font: 'inherit' }}
      title={`Open spell ${id} in the spell explorer`}
    >
      {name}
    </button>
  )
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
  showHeading = true,
  onSpellClick,
}: {
  summary: NPCCasterSummary
  sections: NPCOverlaySections
  theme: CasterSummaryTheme
  // showHeading renders the "Spells & Abilities" label. The DB page already
  // wraps the section in its own titled card, so it passes false.
  showHeading?: boolean
  // onSpellClick makes proc/signature spell names clickable (e.g. open the
  // spell explorer). Omitted in contexts with no navigation target, where the
  // names stay plain text.
  onSpellClick?: (id: number) => void
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
      {showHeading && <p style={headingStyle}>Spells &amp; Abilities</p>}

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
            {procs.map((p, i) => {
              const ctx = chunkLabel(p.kind)
              const chance = p.chance ? ` ${p.chance}%` : ''
              return (
                <React.Fragment key={p.spell_id || i}>
                  {i > 0 && ', '}
                  <SpellName id={p.spell_id} name={p.spell_name} onClick={onSpellClick} />
                  {chance}
                  {ctx ? ` (${ctx})` : ''}
                </React.Fragment>
              )
            })}
          </span>
        </div>
      )}

      {signature.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 4, marginBottom: 2 }}>
          <span style={rowLabelStyle}>Signature:</span>
          <span style={{ fontSize: 10, color: theme.chipText }}>
            {signature.map((s, i) => (
              <React.Fragment key={s.spell_id || i}>
                {i > 0 && ', '}
                <SpellName id={s.spell_id} name={s.spell_name} onClick={onSpellClick} />
              </React.Fragment>
            ))}
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

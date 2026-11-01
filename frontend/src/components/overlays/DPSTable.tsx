/**
 * DPSTable — the per-combatant row list shared by the in-app DPS panel and
 * the popped-out DPS overlay window. The two differ only in colours and
 * spacing (see DPSTableTheme); bar scaling, pet expansion, and the pinned
 * "you" row behave identically in both.
 */
import React, { useEffect, useRef, useState } from 'react'
import type { FightState } from '../../types/combat'
import type { DPSClassColors } from '../../types/config'
import { rollupCombatants, petBadge, isYouRow, type RolledUpEntity } from '../../lib/dpsRollup'
import { combatantBarColor } from '../../lib/combatantColor'
import { dpsForMode, dpsModeAbbrev, type DPSMode } from '../../hooks/useDPSMode'
import type { DPSBarScale } from '../../hooks/useDPSBarScale'

export interface DPSTableTheme {
  gap: string
  headerPadding: string
  headerFontSize: number
  headerFontWeight: number
  headerLetterSpacing: string
  headerColor: string
  headerBorder: string
  rowPadding: string
  rowBorder: string
  barAlpha: number
  nameColor: string
  youColor: string
  youWeight: number
  mutedColor: string
  pctColor: string
  dmgColor: string
  rateColor: string
  petPadding: string
  petColor: string
  petPctColor: string
  petsBorder: string
  pinnedBackground: string
  pinnedBorder: string
  emptyText: string
  emptyStyle: React.CSSProperties
}

export const PANEL_THEME: DPSTableTheme = {
  gap: '0 10px',
  headerPadding: '4px 10px',
  headerFontSize: 10,
  headerFontWeight: 600,
  headerLetterSpacing: '0.05em',
  headerColor: 'var(--color-muted)',
  headerBorder: '1px solid var(--color-border)',
  rowPadding: '5px 10px',
  rowBorder: '1px solid var(--color-border)',
  barAlpha: 0.22,
  nameColor: 'var(--color-foreground)',
  youColor: 'var(--color-primary)',
  youWeight: 600,
  mutedColor: 'var(--color-muted)',
  pctColor: 'var(--color-muted)',
  dmgColor: 'var(--color-foreground)',
  rateColor: '#f97316',
  petPadding: '3px 10px 3px 22px',
  petColor: 'var(--color-muted-foreground)',
  petPctColor: 'var(--color-muted)',
  petsBorder: '1px solid var(--color-border)',
  pinnedBackground: 'var(--color-surface-2)',
  pinnedBorder: '1px solid var(--color-border)',
  emptyText: 'No damage data',
  emptyStyle: { padding: '16px 10px', fontSize: 12, color: 'var(--color-muted)', textAlign: 'center' },
}

export const OVERLAY_THEME: DPSTableTheme = {
  gap: '0 8px',
  headerPadding: '3px 8px',
  headerFontSize: 9,
  headerFontWeight: 700,
  headerLetterSpacing: '0.06em',
  headerColor: 'rgba(255,255,255,0.3)',
  headerBorder: '1px solid rgba(255,255,255,0.08)',
  rowPadding: '4px 8px',
  rowBorder: '1px solid rgba(255,255,255,0.06)',
  barAlpha: 0.28,
  nameColor: 'rgba(255,255,255,0.85)',
  youColor: '#818cf8',
  youWeight: 700,
  mutedColor: 'rgba(255,255,255,0.45)',
  pctColor: 'rgba(255,255,255,0.4)',
  dmgColor: 'rgba(255,255,255,0.7)',
  rateColor: '#fb923c',
  petPadding: '2px 8px 2px 20px',
  petColor: 'rgba(255,255,255,0.55)',
  petPctColor: 'rgba(255,255,255,0.35)',
  petsBorder: 'none',
  pinnedBackground: 'rgba(255,255,255,0.06)',
  pinnedBorder: '1px solid rgba(255,255,255,0.15)',
  emptyText: 'No data',
  emptyStyle: { padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 },
}

function fmt(n: number): string { return n.toLocaleString() }
function fmtRate(n: number): string { return n.toFixed(1) }
function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

function rowTooltip(stat: RolledUpEntity): string {
  const parts = [`${stat.hit_count} hit${stat.hit_count !== 1 ? 's' : ''}`, `max ${fmt(stat.max_hit)}`]
  if (stat.crit_count > 0) {
    parts.push(`${stat.crit_count} crit${stat.crit_count !== 1 ? 's' : ''} (${fmt(stat.crit_damage)} dmg)`)
  }
  return parts.join(' · ')
}

interface DPSRowProps {
  stat: RolledUpEntity
  rank: number
  totalDamage: number
  barMax: number
  isYou: boolean
  showRank: boolean
  expanded: boolean
  onToggle: () => void
  mode: DPSMode
  palette: DPSClassColors
  theme: DPSTableTheme
}

// DPSRow forwards its ref to the outer wrapper so DPSTable can watch the
// user's own row for scroll visibility.
const DPSRow = React.forwardRef<HTMLDivElement, DPSRowProps>(function DPSRow(
  { stat, rank, totalDamage, barMax, isYou, showRank, expanded, onToggle, mode, palette, theme },
  ref,
) {
  const barPct = barMax > 0 ? (stat.total_damage / barMax) * 100 : 0
  const hasPets = stat.pets.length > 0
  // flexShrink: 0 keeps each row at its natural height when the parent flex
  // column overflows — without it the column flex algorithm squeezes rows to
  // fit, making text and bars look tiny in raid-sized fights.
  return (
    <div ref={ref} style={{ flexShrink: 0 }}>
      <div
        onClick={hasPets ? onToggle : undefined}
        title={rowTooltip(stat)}
        style={{
          position: 'relative', padding: theme.rowPadding,
          display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: theme.gap,
          alignItems: 'center', borderBottom: theme.rowBorder, overflow: 'hidden',
          cursor: hasPets ? 'pointer' : 'default',
        }}
      >
        <div
          style={{
            position: 'absolute', left: 0, top: 0, bottom: 0,
            width: `${barPct}%`,
            backgroundColor: combatantBarColor(stat.class, palette, theme.barAlpha),
            pointerEvents: 'none',
          }}
        />
        <span style={{ fontSize: 12, fontWeight: isYou ? theme.youWeight : 400, color: isYou ? theme.youColor : theme.nameColor, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', position: 'relative' }}>
          {showRank && <span style={{ color: theme.mutedColor, fontWeight: 400, marginRight: 4 }}>#{rank}</span>}
          {stat.name}
          {hasPets && <span style={{ color: theme.mutedColor, fontWeight: 400 }}>{petBadge(stat.pets)}</span>}
        </span>
        <span style={{ fontSize: 11, color: theme.pctColor, fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{pct(stat.total_damage, totalDamage)}</span>
        <span style={{ fontSize: 11, color: theme.dmgColor, fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{fmt(stat.total_damage)}</span>
        <span style={{ fontSize: 11, color: theme.rateColor, fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>{fmtRate(dpsForMode(stat, mode))}</span>
      </div>
      {hasPets && expanded && (
        <div style={{ borderBottom: theme.petsBorder }}>
          {stat.pets.map((p) => (
            <div
              key={p.name}
              style={{
                padding: theme.petPadding,
                display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: theme.gap,
                alignItems: 'center', fontSize: 11,
                color: theme.petColor,
              }}
            >
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>↳ {p.name}</span>
              <span style={{ color: theme.petPctColor, fontVariantNumeric: 'tabular-nums' }}>{pct(p.total_damage, totalDamage)}</span>
              <span style={{ fontVariantNumeric: 'tabular-nums' }}>{fmt(p.total_damage)}</span>
              <span style={{ color: theme.rateColor, fontVariantNumeric: 'tabular-nums', minWidth: 44, textAlign: 'right' }}>{fmtRate(dpsForMode(p, mode))}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
})

interface DPSTableProps {
  fight: FightState
  showAll: boolean
  combine: boolean
  mode: DPSMode
  palette: DPSClassColors
  barScale: DPSBarScale
  theme: DPSTableTheme
}

export default function DPSTable({ fight, showAll, combine, mode, palette, barScale, theme }: DPSTableProps): React.ReactElement {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const rolled = rollupCombatants(fight.combatants ?? [], combine, fight.duration_seconds)
  const rows = showAll ? rolled : rolled.filter(isYouRow)
  const totalDmg = showAll ? fight.total_damage : fight.you_damage
  const barMax = barScale === 'leader'
    ? rows.reduce((m, r) => Math.max(m, r.total_damage), 0)
    : totalDmg
  const youIdx = showAll ? rows.findIndex(isYouRow) : -1

  // Pin a copy of the user's row below the list while their real row sits
  // below the visible area, so their own numbers stay readable in a raid
  // without scrolling. Rows above the fold need no pin.
  const scrollRef = useRef<HTMLDivElement>(null)
  const [youEl, setYouEl] = useState<HTMLDivElement | null>(null)
  const [youBelow, setYouBelow] = useState(false)
  useEffect(() => {
    const root = scrollRef.current
    if (!root || !youEl) {
      setYouBelow(false)
      return
    }
    const obs = new IntersectionObserver(([e]) => {
      const rootBottom = e.rootBounds?.bottom ?? Infinity
      setYouBelow(e.intersectionRatio < 1 && e.boundingClientRect.bottom > rootBottom)
    }, { root, threshold: [0, 1] })
    obs.observe(youEl)
    return () => obs.disconnect()
  }, [youEl])

  function toggle(name: string): void {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const pinned = youIdx > 0 && youBelow ? rows[youIdx] : null

  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      {/* min-height: 0 lets the scroller honor its allocated height instead
          of growing to fit all rows (default min-height: auto on flex
          items), so overflow: auto actually scrolls in big raids. */}
      <div ref={scrollRef} style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        <div
          style={{
            display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: theme.gap,
            padding: theme.headerPadding, fontSize: theme.headerFontSize, fontWeight: theme.headerFontWeight,
            textTransform: 'uppercase', letterSpacing: theme.headerLetterSpacing, color: theme.headerColor,
            borderBottom: theme.headerBorder, flexShrink: 0,
          }}
        >
          <span>Name</span>
          <span>%</span>
          <span>Dmg</span>
          <span style={{ textAlign: 'right' }}>{dpsModeAbbrev(mode)}</span>
        </div>
        {rows.length === 0 ? (
          <p style={theme.emptyStyle}>{theme.emptyText}</p>
        ) : (
          rows.map((s, i) => (
            <DPSRow
              key={s.name}
              ref={i === youIdx ? setYouEl : undefined}
              stat={s}
              rank={i + 1}
              totalDamage={totalDmg}
              barMax={barMax}
              isYou={isYouRow(s)}
              showRank={i === youIdx && i > 0}
              mode={mode}
              palette={palette}
              theme={theme}
              expanded={expanded.has(s.name)}
              onToggle={() => toggle(s.name)}
            />
          ))
        )}
      </div>
      {pinned && (
        <div
          style={{ flexShrink: 0, borderTop: theme.pinnedBorder, backgroundColor: theme.pinnedBackground }}
        >
          <DPSRow
            stat={pinned}
            rank={youIdx + 1}
            totalDamage={totalDmg}
            barMax={barMax}
            isYou
            showRank
            mode={mode}
            palette={palette}
            theme={theme}
            expanded={false}
            onToggle={() => {}}
          />
        </div>
      )}
    </div>
  )
}

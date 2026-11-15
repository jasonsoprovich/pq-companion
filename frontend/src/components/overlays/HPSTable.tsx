/**
 * HPSTable — the DPS meter's HPS view: your healing in the fight, one row
 * per person you healed. Shares DPSTable's themes so the popout and the
 * dashboard panel match their DPS look.
 */
import React from 'react'
import type { FightState } from '../../types/combat'
import type { DPSMode } from '../../hooks/useDPSMode'
import type { DPSBarScale } from '../../hooks/useDPSBarScale'
import { healTargetRows, hpsModeAbbrev, youHealer, type HealTargetRow } from '../../lib/hpsMeter'
import type { DPSTableTheme } from './DPSTable'

const BAR = (alpha: number): string => `rgba(74,222,128,${alpha})`
const RATE = '#4ade80'

function fmt(n: number): string { return n.toLocaleString() }
function pct(part: number, total: number): string {
  if (total === 0) return '—'
  return `${Math.round((part / total) * 100)}%`
}

function Row({ row, total, barMax, theme }: { row: HealTargetRow; total: number; barMax: number; theme: DPSTableTheme }): React.ReactElement {
  const barPct = barMax > 0 ? (row.total_heal / barMax) * 100 : 0
  return (
    <div
      title={`${row.heal_count} heal${row.heal_count !== 1 ? 's' : ''} · biggest ${fmt(row.max_heal)}${row.is_you ? ' · self-heals' : ''}`}
      style={{
        position: 'relative', padding: theme.rowPadding, flexShrink: 0,
        display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: theme.gap,
        alignItems: 'center', borderBottom: theme.rowBorder, overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0, width: `${barPct}%`,
          backgroundColor: BAR(theme.barAlpha), pointerEvents: 'none',
        }}
      />
      <span
        style={{
          fontSize: 12, position: 'relative', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          fontWeight: row.is_you ? theme.youWeight : 400, color: row.is_you ? theme.youColor : theme.nameColor,
        }}
      >
        {row.name}
      </span>
      <span style={{ fontSize: 11, color: theme.pctColor, fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{pct(row.total_heal, total)}</span>
      <span style={{ fontSize: 11, color: theme.dmgColor, fontVariantNumeric: 'tabular-nums', position: 'relative' }}>{fmt(row.total_heal)}</span>
      <span style={{ fontSize: 11, color: RATE, fontVariantNumeric: 'tabular-nums', position: 'relative', minWidth: 44, textAlign: 'right' }}>{row.rate.toFixed(1)}</span>
    </div>
  )
}

export default function HPSTable({
  fight,
  mode,
  barScale,
  theme,
  liveSecs,
}: {
  fight: FightState
  mode: DPSMode
  barScale: DPSBarScale
  theme: DPSTableTheme
  // Live wall-clock for Encounter while the fight runs.
  liveSecs?: number
}): React.ReactElement {
  const rows = healTargetRows(fight, mode, liveSecs)
  const total = youHealer(fight)?.total_heal ?? 0
  const barMax = barScale === 'leader' ? rows.reduce((m, r) => Math.max(m, r.total_heal), 0) : total

  return (
    <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
      <div
        style={{
          display: 'grid', gridTemplateColumns: '1fr auto auto auto', gap: theme.gap,
          padding: theme.headerPadding, fontSize: theme.headerFontSize, fontWeight: theme.headerFontWeight,
          textTransform: 'uppercase', letterSpacing: theme.headerLetterSpacing, color: theme.headerColor,
          borderBottom: theme.headerBorder, flexShrink: 0,
        }}
      >
        <span>Healed</span>
        <span>%</span>
        <span>Amount</span>
        <span style={{ textAlign: 'right' }}>{hpsModeAbbrev(mode)}</span>
      </div>
      {rows.length === 0 ? (
        <p style={theme.emptyStyle}>You haven’t healed anyone this fight</p>
      ) : (
        rows.map((r) => <Row key={r.name} row={r} total={total} barMax={barMax} theme={theme} />)
      )}
      <p style={{ ...theme.emptyStyle, padding: '6px 8px', fontSize: 10, marginTop: 'auto', flexShrink: 0 }}>
        Your heals only. Other healers’ heals aren’t in your log.
      </p>
    </div>
  )
}

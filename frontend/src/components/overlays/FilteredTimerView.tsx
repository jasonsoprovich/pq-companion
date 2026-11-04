/**
 * FilteredTimerView — body of the Filtered Timers overlay, shared by the
 * dashboard panel (FilteredTimerPanel) and the popout window
 * (FilteredTimerWindowPage). Renders the filter bar and the grouped timer
 * rows; the caller owns the chrome (title bar, lock, close).
 *
 * The `theme` prop picks the colour set: 'panel' uses the app's CSS tokens,
 * 'overlay' uses the translucent-white palette every popout overlay uses.
 */
import React, { useState } from 'react'
import { Eraser, Star, X, UserX } from 'lucide-react'
import { SpellIcon } from '../Icon'
import TimerRowControls from './TimerRowControls'
import { clearTargetTimers, removeTimer } from '../../services/api'
import { fmtRemaining } from '../../lib/timeFormat'
import {
  TARGET_CURRENT,
  TARGET_DEBUFFS_PRESET,
  TARGET_SELF,
  knownSpellNames,
  normalizeTarget,
  targetOptions,
  type FilteredTimerConfig,
  type TimerGroup,
} from '../../lib/timerFilter'
import type { TimerAppearance } from '../../hooks/useTimerAppearance'
import type { ActiveTimer, TimerCategory } from '../../types/timer'

export type FilteredTimerTheme = 'panel' | 'overlay'

interface Palette {
  fg: string
  muted: string
  border: string
  surface: string
  input: string
}

const PALETTES: Record<FilteredTimerTheme, Palette> = {
  panel: {
    fg: 'var(--color-foreground)',
    muted: 'var(--color-muted)',
    border: 'var(--color-border)',
    surface: 'var(--color-surface-2)',
    input: 'var(--color-surface)',
  },
  overlay: {
    fg: 'rgba(255,255,255,1)',
    muted: 'rgba(255,255,255,0.55)',
    border: 'rgba(255,255,255,0.1)',
    surface: 'rgba(255,255,255,0.04)',
    input: 'rgba(20,20,24,0.9)',
  },
}

const CATEGORY_COLORS: Partial<Record<TimerCategory, string>> = {
  debuff: '#f97316',
  dot: '#ef4444',
  mez: '#a855f7',
  stun: '#eab308',
}

function buffBarColor(remaining: number, total: number): string {
  if (total <= 0) return '#22c55e'
  const pct = remaining / total
  if (pct > 0.5) return '#22c55e'
  if (pct > 0.2) return '#f97316'
  return '#ef4444'
}

// Compact one-line description of the active filter, for the title bar.
export function filterSummary(cfg: FilteredTimerConfig): string {
  const kind = cfg.type === 'beneficial' ? 'Buffs' : 'Debuffs'
  if (cfg.viewBy === 'spell') return cfg.target || (cfg.onlySelected ? `Selected ${kind.toLowerCase()}` : `${kind} by spell`)
  if (cfg.target === TARGET_SELF) return `${kind} on me`
  if (cfg.target === TARGET_CURRENT) return `${kind} on target`
  return cfg.target ? `${kind} on ${cfg.target}` : `${kind} by ${cfg.type === 'beneficial' ? 'player' : 'mob'}`
}

// ── Filter bar ────────────────────────────────────────────────────────────────

function FilterBar({
  config,
  update,
  timers,
  activePlayer,
  pal,
}: {
  config: FilteredTimerConfig
  update: (patch: Partial<FilteredTimerConfig>) => void
  timers: ActiveTimer[]
  activePlayer: string
  pal: Palette
}): React.ReactElement {
  const [draft, setDraft] = useState('')
  const selectStyle: React.CSSProperties = {
    flex: 1,
    minWidth: 0,
    fontSize: 11,
    padding: '2px 4px',
    borderRadius: 3,
    border: `1px solid ${pal.border}`,
    backgroundColor: pal.input,
    color: pal.fg,
    colorScheme: 'dark',
  }
  const labelStyle: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
    fontSize: 11,
    color: pal.muted,
    cursor: 'pointer',
    whiteSpace: 'nowrap',
  }
  const options = targetOptions(timers, config, activePlayer)
  const suggestions = knownSpellNames(timers, config.type)
  const isTargetView = config.viewBy === 'target'

  const addSpell = (name: string): void => {
    const n = name.trim()
    if (!n) return
    if (config.selectedSpells.some((s) => s.toLowerCase() === n.toLowerCase())) return
    update({ selectedSpells: [...config.selectedSpells, n] })
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 5,
        padding: '6px 8px',
        borderBottom: `1px solid ${pal.border}`,
        backgroundColor: pal.surface,
        flexShrink: 0,
      }}
    >
      <div style={{ display: 'flex', gap: 4 }}>
        <select
          value={config.type}
          title="Type"
          style={selectStyle}
          onChange={(e) =>
            update({
              type: e.target.value as FilteredTimerConfig['type'],
              target: '',
              showMissing: e.target.value === 'beneficial' && config.showMissing,
            })
          }
        >
          <option value="beneficial">Beneficial</option>
          <option value="detrimental">Detrimental</option>
        </select>
        <select
          value={config.viewBy}
          title="View by"
          style={selectStyle}
          onChange={(e) =>
            update({ viewBy: e.target.value as FilteredTimerConfig['viewBy'], target: '' })
          }
        >
          <option value="spell">By spell</option>
          <option value="target">
            {config.type === 'beneficial' ? 'By player' : 'By mob'}
          </option>
        </select>
        <select
          value={config.target}
          title={isTargetView ? 'Target' : 'Spell'}
          style={selectStyle}
          onChange={(e) => update({ target: e.target.value })}
        >
          <option value="">{isTargetView ? 'All targets' : 'All spells'}</option>
          {isTargetView && <option value={TARGET_SELF}>Me</option>}
          {isTargetView && <option value={TARGET_CURRENT}>Current target</option>}
          {options.map((o) => (
            <option key={o} value={o}>{o}</option>
          ))}
        </select>
      </div>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
        <label style={labelStyle}>
          <input
            type="checkbox"
            checked={config.onlySelected}
            onChange={(e) => update({ onlySelected: e.target.checked })}
          />
          Only selected spells
        </label>
        <label style={labelStyle}>
          <input
            type="checkbox"
            checked={config.underOneMin}
            onChange={(e) => update({ underOneMin: e.target.checked })}
          />
          Under 1 min
        </label>
        <label
          style={labelStyle}
          title="Hide timers until they cross the global or per-trigger display threshold, like the Buff and Detrimental overlays"
        >
          <input
            type="checkbox"
            checked={config.respectThresholds}
            onChange={(e) => update({ respectThresholds: e.target.checked })}
          />
          Use display thresholds
        </label>
        {config.type === 'beneficial' && config.viewBy === 'spell' && (
          <label style={labelStyle} title="List group/raid members missing each spell (needs the Zeal pipe)">
            <input
              type="checkbox"
              checked={config.showMissing}
              onChange={(e) => update({ showMissing: e.target.checked })}
            />
            Show missing
          </label>
        )}
        <button
          type="button"
          onClick={() => update(TARGET_DEBUFFS_PRESET)}
          title="Show every detrimental on your current target, regardless of time left"
          style={{
            marginLeft: 'auto',
            fontSize: 10,
            padding: '1px 6px',
            borderRadius: 3,
            border: `1px solid ${pal.border}`,
            background: 'none',
            color: pal.fg,
            cursor: 'pointer',
            whiteSpace: 'nowrap',
          }}
        >
          Target debuffs
        </button>
      </div>
      {config.onlySelected && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, alignItems: 'center' }}>
          {config.selectedSpells.map((s) => (
            <span
              key={s}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 3,
                fontSize: 10,
                padding: '1px 5px',
                borderRadius: 3,
                border: `1px solid ${pal.border}`,
                color: pal.fg,
              }}
            >
              {s}
              <button
                onClick={() => update({ selectedSpells: config.selectedSpells.filter((x) => x !== s) })}
                title={`Remove ${s}`}
                style={{ background: 'none', border: 'none', padding: 0, cursor: 'pointer', color: pal.muted, lineHeight: 0 }}
              >
                <X size={10} />
              </button>
            </span>
          ))}
          <input
            value={draft}
            list="filtered-timer-spell-suggestions"
            placeholder="Add spell…"
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                addSpell(draft)
                setDraft('')
              }
            }}
            style={{ ...selectStyle, flex: '1 1 90px', fontSize: 10 }}
          />
          <datalist id="filtered-timer-spell-suggestions">
            {suggestions.map((s) => (
              <option key={s} value={s} />
            ))}
          </datalist>
        </div>
      )}
    </div>
  )
}

// ── Rows ──────────────────────────────────────────────────────────────────────

function TimerRow({
  timer,
  label,
  appearance,
  showRemove,
  unproven,
  pal,
}: {
  timer: ActiveTimer
  label: string
  appearance: TimerAppearance
  showRemove: boolean
  // Matched by name only; may be on another same-named spawn.
  unproven: boolean
  pal: Palette
}): React.ReactElement {
  const expired = timer.expired === true
  const pct = expired
    ? 1
    : timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = expired
    ? '#ef4444'
    : timer.bar_color ||
      CATEGORY_COLORS[timer.category] ||
      buffBarColor(timer.remaining_seconds, timer.duration_seconds)
  const urgent = expired || pct < 0.2
  const showUrgentColor = urgent && !appearance.urgentTextWhite
  const timeColor = showUrgentColor ? '#f87171' : urgent ? pal.fg : color

  return (
    <div
      className="timer-row"
      title={unproven ? 'Not tied to this exact spawn; may be on another mob with the same name' : undefined}
      style={{
        position: 'relative',
        padding: `${appearance.rowPadding}px 8px`,
        borderBottom: `1px solid ${pal.border}`,
        overflow: 'hidden',
        flexShrink: 0,
        opacity: unproven ? 0.6 : 1,
      }}
    >
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`, backgroundColor: color, opacity: appearance.fillOpacity,
          pointerEvents: 'none', transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'relative', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, flex: 1 }}>
          <SpellIcon id={timer.icon} name={timer.spell_name} size={16} loading="eager" />
          <span
            style={{
              fontSize: appearance.nameFontSize,
              color: showUrgentColor ? '#f87171' : pal.fg,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: urgent ? 600 : 500,
              textShadow: '0 1px 2px rgba(0,0,0,0.9)',
            }}
          >
            {label}
            {unproven && ' ?'}
          </span>
        </div>
        <span
          style={{
            fontSize: appearance.timeFontSize,
            color: timeColor,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: urgent ? 700 : 600,
            textShadow: '0 1px 2px rgba(0,0,0,0.9)',
          }}
        >
          {expired ? 'expired' : fmtRemaining(timer.remaining_seconds, appearance.showSeconds)}
        </span>
        {showRemove && (
          <TimerRowControls alwaysShow={appearance.controlsAlwaysShow}>
            <button
              onClick={() => removeTimer(timer.id).catch(() => {})}
              title="Remove this timer"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 0, color: pal.muted, display: 'flex', alignItems: 'center', flexShrink: 0, lineHeight: 0 }}
            >
              <X size={11} />
            </button>
          </TimerRowControls>
        )}
      </div>
    </div>
  )
}

function GroupHeader({
  group,
  config,
  update,
  roster,
  interactive,
  pal,
}: {
  group: TimerGroup
  config: FilteredTimerConfig
  update: (patch: Partial<FilteredTimerConfig>) => void
  roster: string[]
  interactive: boolean
  pal: Palette
}): React.ReactElement {
  const bySpell = config.viewBy === 'spell'
  const noun = bySpell ? 'player' : config.type === 'beneficial' ? 'buff' : 'debuff'
  const n = group.rows.length
  const count =
    bySpell && config.showMissing && roster.length > 0
      ? `${roster.length - group.missing.length} / ${roster.length}`
      : `${n} ${noun}${n === 1 ? '' : 's'}`
  const starred = config.selectedSpells.some((s) => s.toLowerCase() === group.title.toLowerCase())
  const clearable =
    interactive && !bySpell && config.type === 'detrimental' && config.target === TARGET_CURRENT &&
    group.rows.length > 0
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        padding: '4px 8px',
        borderBottom: `1px solid ${pal.border}`,
        backgroundColor: pal.surface,
        flexShrink: 0,
      }}
    >
      {bySpell && <SpellIcon id={group.icon} name={group.title} size={16} loading="eager" />}
      <span style={{ flex: 1, minWidth: 0, fontSize: 11, fontWeight: 700, color: pal.fg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {bySpell ? group.title : `${config.type === 'beneficial' ? 'Buffs' : 'Debuffs'} on ${group.title}`}
      </span>
      <span style={{ fontSize: 10, color: pal.muted, flexShrink: 0 }}>{count}</span>
      {clearable && (
        <button
          onClick={() => clearTargetTimers(group.title, group.targetId).catch(() => {})}
          title={`Clear the debuffs on ${group.title} (after a leash or evade reset, which the log doesn't show)`}
          style={{ background: 'none', border: 'none', padding: 0, cursor: 'pointer', color: pal.muted, lineHeight: 0 }}
        >
          <Eraser size={11} />
        </button>
      )}
      {bySpell && interactive && (
        <button
          onClick={() =>
            update({
              selectedSpells: starred
                ? config.selectedSpells.filter((s) => s.toLowerCase() !== group.title.toLowerCase())
                : [...config.selectedSpells, group.title],
            })
          }
          title={starred ? 'Remove from selected spells' : 'Add to selected spells'}
          style={{ background: 'none', border: 'none', padding: 0, cursor: 'pointer', color: starred ? '#eab308' : pal.muted, lineHeight: 0 }}
        >
          <Star size={11} fill={starred ? 'currentColor' : 'none'} />
        </button>
      )}
    </div>
  )
}

// ── View ──────────────────────────────────────────────────────────────────────

export interface FilteredTimerViewProps {
  theme: FilteredTimerTheme
  config: FilteredTimerConfig
  update: (patch: Partial<FilteredTimerConfig>) => void
  showFilters: boolean
  timers: ActiveTimer[] | null
  groups: TimerGroup[]
  activePlayer: string
  roster: string[]
  zealConnected: boolean
  appearance: TimerAppearance
  rowsInteractive: boolean
  // Hide the idle "nothing to show" placeholder (overlay chrome faded).
  hideEmptyState?: boolean
}

export default function FilteredTimerView({
  theme,
  config,
  update,
  showFilters,
  timers,
  groups,
  activePlayer,
  roster,
  zealConnected,
  appearance,
  rowsInteractive,
  hideEmptyState,
}: FilteredTimerViewProps): React.ReactElement {
  const pal = PALETTES[theme]
  const bySpell = config.viewBy === 'spell'
  const missingWanted = config.showMissing && config.type === 'beneficial' && bySpell
  const emptyText =
    config.viewBy === 'target' && config.target === TARGET_CURRENT
      ? 'No target, or nothing on it'
      : config.onlySelected && config.selectedSpells.length === 0
        ? 'No spells selected yet'
        : 'Nothing matches this filter'

  const rowLabel = (t: ActiveTimer): string => {
    if (!bySpell) return t.spell_name
    const who = normalizeTarget(t.target_name, activePlayer)
    return who || t.spell_name
  }

  return (
    <>
      {showFilters && (
        <FilterBar
          config={config}
          update={update}
          timers={timers ?? []}
          activePlayer={activePlayer}
          pal={pal}
        />
      )}
      {missingWanted && !zealConnected && showFilters && (
        <div style={{ padding: '4px 8px', fontSize: 10, color: '#f97316', borderBottom: `1px solid ${pal.border}`, flexShrink: 0 }}>
          The missing list needs the Zeal pipe connected (group/raid roster).
        </div>
      )}
      <div
        style={{
          flex: 1,
          minHeight: 0,
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {timers === null ? (
          <p style={{ padding: 12, fontSize: 11, color: pal.muted, textAlign: 'center', margin: 0 }}>
            Connecting…
          </p>
        ) : groups.length === 0 ? (
          hideEmptyState ? null : (
            <p style={{ padding: 16, fontSize: 11, color: pal.muted, textAlign: 'center', margin: 0 }}>
              {emptyText}
            </p>
          )
        ) : (
          groups.map((g) => (
            <React.Fragment key={g.key}>
              <GroupHeader
                group={g}
                config={config}
                update={update}
                roster={roster}
                interactive={rowsInteractive}
                pal={pal}
              />
              {g.rows.map((t) => (
                <TimerRow
                  key={t.id}
                  timer={t}
                  label={rowLabel(t)}
                  appearance={appearance}
                  showRemove={rowsInteractive}
                  unproven={g.unproven.has(t.id)}
                  pal={pal}
                />
              ))}
              {g.missing.map((name) => (
                <div
                  key={`missing-${name}`}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: `${appearance.rowPadding}px 8px`,
                    borderBottom: `1px solid ${pal.border}`,
                    opacity: 0.55,
                    flexShrink: 0,
                  }}
                >
                  <UserX size={14} style={{ color: '#f87171', flexShrink: 0 }} />
                  <span style={{ flex: 1, minWidth: 0, fontSize: appearance.nameFontSize, color: pal.fg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {name}
                  </span>
                  <span style={{ fontSize: appearance.timeFontSize, color: '#f87171', flexShrink: 0 }}>
                    missing
                  </span>
                </div>
              ))}
            </React.Fragment>
          ))
        )}
      </div>
    </>
  )
}

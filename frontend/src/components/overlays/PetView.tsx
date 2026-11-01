/**
 * PetView — body of the Pet overlay, shared by the dashboard panel (PetPanel)
 * and the popout window (PetWindowPage).
 *
 * Three views, driven by the backend's pet:state mode:
 *   - pet:    name/level/class, HP bar, pinned charm bar, timers on the pet, a
 *             stats line, and summon/gate/immunity badges.
 *   - broken: the charm just broke (or the pet died) — the old pet's card in a
 *             red BROKE state with its MR, re-charm odds and immunities, until
 *             the backend's swap delay returns it to none.
 *   - none:   the compact charm finder for charm classes (empty state for
 *             everyone else).
 * A manual toggle peeks at the finder while a pet is up and returns to it;
 * any mode change resets the toggle.
 */
import React, { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, ExternalLink, PawPrint, Search } from 'lucide-react'
import TargetTimerList from './TargetTimerList'
import { SpellIcon } from '../Icon'
import { usePetTimers } from '../../hooks/usePetState'
import { useCharmFinder, type CharmFinderHook } from '../../hooks/useCharmFinder'
import { usePersistentState } from '../../hooks/usePersistentState'
import { useAppNavigate } from '../../hooks/useAppNavigate'
import { landColor } from '../../lib/charmFinder'
import { fmtRemaining } from '../../lib/timeFormat'
import type { CharmPet } from '../../services/api'
import type { PetState, PetStats } from '../../types/pet'
import type { ActiveTimer } from '../../types/timer'

const FG = 'rgba(255,255,255,0.92)'
const MUTED = 'rgba(255,255,255,0.5)'
const FAINT = 'rgba(255,255,255,0.35)'
const BORDER = 'rgba(255,255,255,0.1)'
const TRACK = 'rgba(255,255,255,0.07)'
const CHARM = '#a855f7'
const SUMMONED = '#38bdf8'
const BROKE = '#ef4444'
const WARN = '#f59e0b'

// Rows shown in the compact finder before deferring to the full page.
const FINDER_ROWS = 8

interface PetViewProps {
  state: PetState
  // False in a locked, non-hovered popout: hides the toggle/dropdown/filter
  // controls so a locked overlay stays a clean HUD.
  interactive?: boolean
}

export default function PetView({ state, interactive = true }: PetViewProps): React.ReactElement {
  const [peekFinder, setPeekFinder] = useState(false)
  useEffect(() => { setPeekFinder(false) }, [state.mode, state.pet_id, state.name])

  const showFinder = state.mode === 'none' || peekFinder
  // The finder's data also supplies the re-charm odds on the broken card.
  const finder = useCharmFinder(state.zone, showFinder || state.mode === 'broken')

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6, padding: 8, height: '100%', overflowY: 'auto' }}>
      {showFinder ? (
        <>
          {peekFinder && interactive && (
            <ToggleLink onClick={() => setPeekFinder(false)}>
              ← Back to {state.name || 'pet'}
            </ToggleLink>
          )}
          <FinderView finder={finder} state={state} interactive={interactive} />
        </>
      ) : state.mode === 'broken' ? (
        <BrokenCard state={state} finder={finder} interactive={interactive} onFinder={() => setPeekFinder(true)} />
      ) : (
        <PetCard state={state} interactive={interactive} onFinder={() => setPeekFinder(true)} />
      )}
    </div>
  )
}

// ── Pet mode ─────────────────────────────────────────────────────────────────

function PetCard({
  state,
  interactive,
  onFinder,
}: {
  state: PetState
  interactive: boolean
  onFinder: () => void
}): React.ReactElement {
  const { charm, timers } = usePetTimers(state.name)
  const s = state.stats
  const kindColor = state.kind === 'charm' ? CHARM : state.kind === 'summoned' ? SUMMONED : MUTED

  return (
    <>
      <Header state={state} color={kindColor} />
      <HPBar state={state} />
      {(state.kind === 'charm' || charm) && <CharmBar timer={charm} spell={state.charm_spell} />}

      <Section label="On pet">
        <TargetTimerList timers={timers} emptyText="No timers on your pet." />
      </Section>

      {s ? (
        <>
          <StatsLine stats={s} />
          <Badges stats={s} />
        </>
      ) : (
        <p style={{ fontSize: 10, color: FAINT, margin: 0 }}>
          {state.kind === 'summoned'
            ? 'Stats for summoned pets aren’t available yet.'
            : 'Stats unknown — target your pet to look it up.'}
        </p>
      )}

      <Footer state={state} />
      {interactive && <ToggleLink onClick={onFinder}>Charm finder →</ToggleLink>}
    </>
  )
}

function Header({ state, color }: { state: PetState; color: string }): React.ReactElement {
  const s = state.stats
  const ambiguous = (state.variants ?? 0) > 1
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
        <PawPrint size={12} style={{ color, flexShrink: 0 }} />
        <span
          style={{ fontSize: 13, fontWeight: 700, color: FG, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          title={state.name}
        >
          {state.name || 'Your pet'}
        </span>
        {ambiguous && (
          <span
            title={`${state.variants} database rows share this name \u2014 stats are a best guess. Target your pet to pin the exact one.`}
            style={{ fontSize: 10, fontWeight: 700, color: WARN }}
          >
            ?
          </span>
        )}
        {state.kind && (
          <span
            style={{
              marginLeft: 'auto',
              flexShrink: 0,
              fontSize: 9,
              fontWeight: 700,
              letterSpacing: 0.4,
              textTransform: 'uppercase',
              color,
              padding: '1px 5px',
              borderRadius: 3,
              backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`,
            }}
          >
            {state.kind === 'charm' ? 'Charmed' : 'Summoned'}
          </span>
        )}
      </div>
      {s && (
        <div style={{ fontSize: 11, color: MUTED, paddingLeft: 18 }}>
          Level {s.level ?? range(s.level_min, s.level_max)} {s.class_name}
          {s.race_name ? ` \u00b7 ${s.race_name}` : ''}
        </div>
      )}
    </div>
  )
}

function HPBar({ state }: { state: PetState }): React.ReactElement {
  const pct = state.hp_percent
  const s = state.stats
  const known = pct != null
  const color = !known || pct > 50 ? '#22c55e' : pct > 20 ? '#eab308' : '#ef4444'
  // Total HP from the scaled row; "~" when the level (and so the HP) is a range.
  const total = s ? s.hp_max : 0
  const approx = s != null && s.hp_min !== s.hp_max
  const now = useNowTick(state.hp_source === 'pet_health')
  const age = state.hp_source === 'pet_health' && state.hp_at
    ? Math.max(0, Math.round((now - Date.parse(state.hp_at)) / 1000))
    : null

  return (
    <div>
      <div style={{ position: 'relative', height: 14, borderRadius: 3, backgroundColor: TRACK, overflow: 'hidden' }}>
        {known && (
          <div style={{ position: 'absolute', inset: 0, width: `${pct}%`, backgroundColor: color, opacity: 0.55, transition: 'width 0.3s ease' }} />
        )}
        <div
          style={{
            position: 'relative',
            display: 'flex',
            justifyContent: 'space-between',
            padding: '0 6px',
            fontSize: 10,
            lineHeight: '14px',
            fontWeight: 600,
            color: FG,
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          <span>{known ? `${pct}%` : 'HP unknown'}</span>
          {total > 0 && (
            <span>
              {known && `${approx ? '~' : ''}${fmtK(Math.round((total * pct) / 100))} / `}
              {approx ? '~' : ''}
              {fmtK(total)}
            </span>
          )}
        </div>
      </div>
      {age != null && (
        <div style={{ fontSize: 9, color: FAINT, marginTop: 2 }}>via /pet health · {fmtAge(age)}</div>
      )}
      {!known && (
        <div style={{ fontSize: 9, color: FAINT, marginTop: 2 }}>Type /pet health for a reading.</div>
      )}
    </div>
  )
}

function CharmBar({ timer, spell }: { timer: ActiveTimer | null; spell?: string }): React.ReactElement {
  if (!timer) {
    return (
      <div style={{ fontSize: 10, color: FAINT, display: 'flex', alignItems: 'center', gap: 5 }}>
        <span style={{ color: CHARM, fontWeight: 600 }}>Charm</span>
        {spell ? `${spell} — no timer running` : 'no timer running'}
      </div>
    )
  }
  const pct = timer.duration_seconds > 0
    ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
    : 0
  const urgent = timer.remaining_seconds <= 30
  return (
    <div
      style={{
        position: 'relative',
        padding: '3px 6px',
        borderRadius: 3,
        overflow: 'hidden',
        backgroundColor: TRACK,
        border: `1px solid color-mix(in srgb, ${CHARM} 40%, transparent)`,
      }}
    >
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: urgent ? BROKE : CHARM,
          opacity: 0.35,
          transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 5 }}>
        <SpellIcon id={timer.icon} name={timer.spell_name} size={15} loading="eager" />
        <span style={{ fontSize: 11, fontWeight: 600, color: FG, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {timer.spell_name}
        </span>
        <span style={{ fontSize: 11, fontWeight: 700, color: urgent ? '#f87171' : '#d8b4fe', fontVariantNumeric: 'tabular-nums' }}>
          {fmtRemaining(timer.remaining_seconds, true)}
        </span>
      </div>
    </div>
  )
}

function StatsLine({ stats: s }: { stats: PetStats }): React.ReactElement {
  const items: [string, string][] = [
    ['DPS', range1(s.dps_min, s.dps_max)],
    ['Max hit', range(s.max_hit_min, s.max_hit_max)],
    ['Delay', `${(s.attack_delay / 10).toFixed(1)}s`],
    ['MR', String(s.mr)],
  ]
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', columnGap: 10, rowGap: 2, fontSize: 11, fontVariantNumeric: 'tabular-nums' }}>
      {items.map(([k, v]) => (
        <span key={k}>
          <span style={{ color: MUTED }}>{k}</span> <span style={{ color: FG, fontWeight: 600 }}>{v}</span>
        </span>
      ))}
    </div>
  )
}

// petBadges lists the flags a pet owner cares about: what makes a charm break
// dangerous (summon, gate, can't be rooted/mezzed/stunned/feared) and what
// limits buffing or debuffing (can't be hasted, reverse slow), then offense.
function petBadges(s: PetStats): { label: string; color: string; title?: string }[] {
  const im = s.immunities ?? {}
  const out: { label: string; color: string; title?: string }[] = []
  if (s.summons) out.push({ label: 'Summons', color: BROKE, title: 'Summons you to it when it breaks and you run' })
  if (s.gates) out.push({ label: 'Gates', color: WARN })
  if (im.snare_root) out.push({ label: 'No root/snare', color: BROKE })
  if (im.mez) out.push({ label: 'No mez', color: BROKE })
  if (im.stun) out.push({ label: 'No stun', color: WARN })
  if (im.fear) out.push({ label: 'No fear', color: WARN })
  if (im.slow) out.push({ label: 'No slow', color: WARN })
  if (im.haste) out.push({ label: 'Can’t haste', color: WARN, title: 'Immune to haste — your haste buffs won’t land' })
  if (im.reverse_slow) out.push({ label: 'Reverse slow', color: WARN, title: 'Slows haste this NPC instead' })
  if (im.ranged) out.push({ label: 'Melee-range spells only', color: WARN, title: 'Immune to spells cast from outside melee range' })
  for (const o of s.offense ?? []) out.push({ label: o, color: MUTED })
  return out
}

function Badges({ stats }: { stats: PetStats }): React.ReactElement | null {
  const badges = petBadges(stats)
  if (badges.length === 0) return null
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}>
      {badges.map((b) => (
        <span
          key={b.label}
          title={b.title}
          style={{
            fontSize: 9,
            fontWeight: 600,
            padding: '1px 5px',
            borderRadius: 3,
            color: b.color,
            backgroundColor: `color-mix(in srgb, ${b.color} 14%, transparent)`,
            border: `1px solid color-mix(in srgb, ${b.color} 30%, transparent)`,
          }}
        >
          {b.label}
        </span>
      ))}
    </div>
  )
}

function Footer({ state }: { state: PetState }): React.ReactElement | null {
  if (!state.zeal_limited) return null
  return (
    <p style={{ fontSize: 9, color: FAINT, margin: 0, display: 'flex', gap: 4, alignItems: 'flex-start' }}>
      <AlertTriangle size={10} style={{ flexShrink: 0, marginTop: 1, color: WARN }} />
      Limited without Zeal: HP comes only from /pet health, and a pet&rsquo;s death isn&rsquo;t detected.
    </p>
  )
}

// ── Broken mode ──────────────────────────────────────────────────────────────

function BrokenCard({
  state,
  finder,
  interactive,
  onFinder,
}: {
  state: PetState
  finder: CharmFinderHook
  interactive: boolean
  onFinder: () => void
}): React.ReactElement {
  const s = state.stats
  const now = useNowTick(true)
  const left = state.swap_at ? Math.max(0, Math.ceil((Date.parse(state.swap_at) - now) / 1000)) : null
  const recharm = s ? finder.data?.pets.find((p) => p.npc_id === s.npc_id) : undefined
  const spellName = finder.data?.spell_name

  return (
    <>
      <div
        style={{
          padding: '5px 8px',
          borderRadius: 4,
          backgroundColor: `color-mix(in srgb, ${BROKE} 18%, transparent)`,
          border: `1px solid color-mix(in srgb, ${BROKE} 45%, transparent)`,
        }}
      >
        <div style={{ fontSize: 12, fontWeight: 800, color: '#fca5a5', letterSpacing: 0.4 }}>
          {state.kind === 'charm' ? 'CHARM BROKE' : 'PET LOST'}
        </div>
        <div style={{ fontSize: 12, fontWeight: 600, color: FG }}>{state.name || 'Your pet'}</div>
      </div>
      {s && (
        <>
          <div style={{ display: 'flex', gap: 12, fontSize: 11, fontVariantNumeric: 'tabular-nums' }}>
            <span>
              <span style={{ color: MUTED }}>MR</span> <span style={{ color: FG, fontWeight: 700 }}>{s.mr}</span>
            </span>
            {recharm && (
              <span title={spellName ? `Odds ${spellName} lands at the top of its level range` : undefined}>
                <span style={{ color: MUTED }}>Re-charm</span>{' '}
                <span style={{ color: landColor(recharm.land_chance), fontWeight: 700 }}>
                  {(recharm.land_chance * 100).toFixed(0)}%
                </span>
              </span>
            )}
            <span>
              <span style={{ color: MUTED }}>L</span>{' '}
              <span style={{ color: FG }}>{s.level ?? range(s.level_min, s.level_max)}</span>
            </span>
          </div>
          <Badges stats={s} />
        </>
      )}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 10, color: FAINT }}>
        <span>{left != null ? `Finder in ${left}s` : ''}</span>
        {interactive && <ToggleLink onClick={onFinder}>Show finder now →</ToggleLink>}
      </div>
    </>
  )
}

// ── Finder mode ──────────────────────────────────────────────────────────────

function FinderView({
  finder,
  state,
  interactive,
}: {
  finder: CharmFinderHook
  state: PetState
  interactive: boolean
}): React.ReactElement {
  const navigate = useAppNavigate()
  const [hideOverCap, setHideOverCap] = usePersistentState<'on' | 'off'>('pq-pet-finder-hide-overcap', 'on', ['on', 'off'])
  const [hideSummon, setHideSummon] = usePersistentState<'on' | 'off'>('pq-pet-finder-hide-summon', 'off', ['on', 'off'])
  const [hideGate, setHideGate] = usePersistentState<'on' | 'off'>('pq-pet-finder-hide-gate', 'off', ['on', 'off'])

  const pets = useMemo(() => {
    let p = finder.data?.pets ?? []
    if (hideOverCap === 'on') p = p.filter((x) => !x.level_warning)
    if (hideSummon === 'on') p = p.filter((x) => !x.summon)
    if (hideGate === 'on') p = p.filter((x) => !x.gate)
    return p
  }, [finder.data, hideOverCap, hideSummon, hideGate])

  if (!finder.character) {
    return <Empty>No pet.</Empty>
  }
  if (!finder.charmClass) {
    return <Empty>No pet. The charm finder is for Enchanters, Necromancers, Druids, Bards and Magicians.</Empty>
  }

  const auto = finder.options.find((o) => o.spell_id === finder.autoSpellID)
  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <Search size={11} style={{ color: CHARM, flexShrink: 0 }} />
        <span style={{ fontSize: 12, fontWeight: 700, color: FG, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {state.zone_name || 'Unknown zone'}
        </span>
        {finder.data && (
          <span style={{ fontSize: 10, color: MUTED, marginLeft: 'auto', flexShrink: 0 }}>
            {pets.length} pet{pets.length === 1 ? '' : 's'}
          </span>
        )}
      </div>

      {interactive ? (
        <select
          value={finder.spellPick}
          onChange={(e) => finder.setSpellPick(e.target.value)}
          style={{
            fontSize: 11,
            padding: '2px 4px',
            borderRadius: 3,
            backgroundColor: 'rgba(255,255,255,0.06)',
            color: FG,
            border: `1px solid ${BORDER}`,
          }}
        >
          <option value="">Auto{auto ? ` (${auto.name})` : ''}</option>
          {finder.options.map((o) => (
            <option key={o.spell_id} value={String(o.spell_id)}>
              {o.name} — max L{o.max_charm_level}
              {o.req_level > (finder.character?.level ?? 0) ? ` (needs L${o.req_level})` : ''}
            </option>
          ))}
        </select>
      ) : (
        finder.data && <div style={{ fontSize: 10, color: MUTED }}>{finder.data.spell_name} · max L{finder.data.max_charm_level}</div>
      )}

      {interactive && (
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 4, fontSize: 10, color: FAINT }}>
          Hide
          <Chip on={hideOverCap === 'on'} onClick={() => setHideOverCap(hideOverCap === 'on' ? 'off' : 'on')}>Over cap</Chip>
          <Chip on={hideSummon === 'on'} onClick={() => setHideSummon(hideSummon === 'on' ? 'off' : 'on')}>Summoners</Chip>
          <Chip on={hideGate === 'on'} onClick={() => setHideGate(hideGate === 'on' ? 'off' : 'on')}>Gaters</Chip>
        </div>
      )}

      {!state.zone ? (
        <Empty>Zone in (or connect Zeal) to list charmable pets here.</Empty>
      ) : finder.error ? (
        <Empty>{finder.error}</Empty>
      ) : !finder.data ? (
        <Empty>{finder.loading ? 'Loading…' : 'No charm spell selected.'}</Empty>
      ) : pets.length === 0 ? (
        <Empty>No charmable pets here for {finder.data.spell_name}.</Empty>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <FinderHeader />
          {pets.slice(0, FINDER_ROWS).map((p) => <FinderRow key={p.npc_id} pet={p} />)}
        </div>
      )}

      {interactive && (
        <ToggleLink onClick={() => navigate('/charm-pet-finder')}>
          {pets.length > FINDER_ROWS ? `${pets.length - FINDER_ROWS} more in ` : 'Open '}Charm Pet Finder
          <ExternalLink size={10} style={{ marginLeft: 3 }} />
        </ToggleLink>
      )}
    </>
  )
}

const FINDER_COLS = 'minmax(0,1fr) 40px 30px 24px 32px'

function FinderHeader(): React.ReactElement {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: FINDER_COLS,
        gap: 4,
        fontSize: 9,
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: 0.3,
        color: FAINT,
        padding: '0 2px 2px',
        borderBottom: `1px solid ${BORDER}`,
      }}
    >
      <span>Pet</span>
      <span style={{ textAlign: 'right' }}>Lvl</span>
      <span style={{ textAlign: 'right' }}>DPS</span>
      <span style={{ textAlign: 'right' }}>MR</span>
      <span style={{ textAlign: 'right' }}>Land</span>
    </div>
  )
}

function FinderRow({ pet }: { pet: CharmPet }): React.ReactElement {
  const flags = [pet.summon && 'Summons', pet.gate && 'Gates'].filter(Boolean).join(', ')
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: FINDER_COLS,
        gap: 4,
        alignItems: 'center',
        fontSize: 11,
        padding: '2px',
        borderBottom: `1px solid ${BORDER}`,
        fontVariantNumeric: 'tabular-nums',
      }}
      title={`${pet.class_name} · ${pet.body_type_name}${flags ? ` · ${flags}` : ''}`}
    >
      <span style={{ display: 'flex', alignItems: 'center', gap: 3, minWidth: 0 }}>
        <span style={{ color: FG, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {pet.name.replace(/^#/, '').replace(/_/g, ' ')}
        </span>
        {pet.summon && <Flag color={BROKE} title="Summons">S</Flag>}
        {pet.gate && <Flag color={WARN} title="Gates">G</Flag>}
        {pet.level_warning && (
          <span title="Can spawn above the charm's level cap" style={{ display: 'flex', flexShrink: 0 }}>
            <AlertTriangle size={10} style={{ color: WARN }} />
          </span>
        )}
      </span>
      <span style={{ textAlign: 'right', color: MUTED, whiteSpace: 'nowrap' }}>{range(pet.level_min, pet.level_max)}</span>
      <span style={{ textAlign: 'right', color: FG, fontWeight: 600 }}>{Math.round(pet.dps_max)}</span>
      <span style={{ textAlign: 'right', color: MUTED }}>{pet.mr}</span>
      <span style={{ textAlign: 'right', color: landColor(pet.land_chance), fontWeight: 600 }}>
        {(pet.land_chance * 100).toFixed(0)}%
      </span>
    </div>
  )
}

// ── small pieces ─────────────────────────────────────────────────────────────

function Section({ label, children }: { label: string; children: React.ReactNode }): React.ReactElement {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
      <span style={{ fontSize: 9, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.4, color: FAINT }}>
        {label}
      </span>
      {children}
    </div>
  )
}

function Empty({ children }: { children: React.ReactNode }): React.ReactElement {
  return <p style={{ fontSize: 11, color: FAINT, margin: 0, padding: '4px 2px' }}>{children}</p>
}

function ToggleLink({ onClick, children }: { onClick: () => void; children: React.ReactNode }): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        alignSelf: 'flex-start',
        display: 'inline-flex',
        alignItems: 'center',
        background: 'none',
        border: 'none',
        padding: 0,
        cursor: 'pointer',
        fontSize: 10,
        color: '#c4b5fd',
      }}
    >
      {children}
    </button>
  )
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        fontSize: 10,
        padding: '1px 6px',
        borderRadius: 9,
        cursor: 'pointer',
        border: `1px solid ${on ? CHARM : BORDER}`,
        backgroundColor: on ? `color-mix(in srgb, ${CHARM} 22%, transparent)` : 'transparent',
        color: on ? FG : MUTED,
      }}
    >
      {children}
    </button>
  )
}

function Flag({ color, title, children }: { color: string; title: string; children: React.ReactNode }): React.ReactElement {
  return (
    <span title={title} style={{ fontSize: 8, fontWeight: 800, color, flexShrink: 0 }}>
      {children}
    </span>
  )
}

// useNowTick re-renders once a second while active (countdowns, reading age).
function useNowTick(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    setNow(Date.now())
    if (!active) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [active])
  return now
}

function range(lo: number, hi: number): string {
  return lo === hi ? String(lo) : `${lo}–${hi}`
}

function range1(lo: number, hi: number): string {
  return lo === hi ? lo.toFixed(1) : `${lo.toFixed(1)}–${hi.toFixed(1)}`
}

function fmtK(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}

function fmtAge(secs: number): string {
  if (secs < 60) return `${secs}s ago`
  return `${Math.floor(secs / 60)}m ago`
}

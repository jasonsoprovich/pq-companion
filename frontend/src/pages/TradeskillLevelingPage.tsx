import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertTriangle, Ban, Coins, ExternalLink, Gauge, Hammer, Info, Route, X,
} from 'lucide-react'
import {
  listCharacters,
  getRecipeTradeskills,
  getCharacterTradeskills,
  getTradeskillLevelingPlan,
  type Character,
} from '../services/api'
import type { RecipeTradeskillCount } from '../types/recipe'
import type { TradeskillView } from '../types/skill'
import type {
  TradeskillLevelingPlan,
  TradeskillObjective,
  LevelingStage,
  SubCombineInfo,
} from '../types/tradeskill'
import { tradeskillLabel } from '../lib/enumsCache'
import { priceLabel } from '../lib/itemHelpers'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

const CHAR_KEY = 'tsLevelCharId'
const SKILL_KEY = 'tsLevelSkill'

// Maelin's Magical Concoction (spell 3999, SPA 504): +75% skill-up rate, the
// only source on Quarm. Shortens the estimated combines.
const MAELIN_SKILLUP_PCT = 75

// Fallback target when we don't know the class/level cap (no Zeal export yet).
const FALLBACK_TARGET = 250

// Common Combine disciplines (0 and the Quarm-only 75) have no trainable skill,
// so there is nothing to level — keep them out of the picker.
function isCommonCombine(ts: number): boolean {
  return ts === 0 || ts === 75
}

export default function TradeskillLevelingPage(): React.ReactElement {
  const { active } = useActiveCharacter()

  const [chars, setChars] = useState<Character[]>([])
  const [charId, setCharId] = useState<number | null>(null)
  const [disciplines, setDisciplines] = useState<RecipeTradeskillCount[]>([])
  const [tsId, setTsId] = useState<number | null>(null)

  const [charSkills, setCharSkills] = useState<TradeskillView[] | null>(null)
  const [start, setStart] = useState<number | ''>('')
  const [target, setTarget] = useState<number | ''>('')
  const [objective, setObjective] = useState<TradeskillObjective>('fastest')
  const [allowFarming, setAllowFarming] = useState(true)
  const [avoidOther, setAvoidOther] = useState(false)
  const [maelin, setMaelin] = useState(false)
  // "Custom" mode: recipes the player rejected (rare/annoying mats, or a recipe
  // whose components aren't actually live yet despite the DB row existing) —
  // recipe_id -> display name, so the chip list can still show a name after the
  // recipe drops out of the plan. Recomputed from the full pool minus this set
  // (not filtered from an already-built plan), so gaps route around cleanly.
  const [excluded, setExcluded] = useState<Record<number, string>>({})
  // Whether the plan currently being shown applies those exclusions. Excluding
  // a recipe turns this on automatically; Fastest/Cheapest stay the untouched,
  // pure computed paths — customActive is what actually applies `excluded` to
  // the request, so switching back to Fastest/Cheapest never loses the
  // exclusions, it just stops using them until Custom is reselected.
  const [customActive, setCustomActive] = useState(false)

  const [plan, setPlan] = useState<TradeskillLevelingPlan | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  // Characters — default to the active (or last-used) character.
  useEffect(() => {
    listCharacters()
      .then((r) => {
        setChars(r.characters)
        setCharId((prev) => {
          if (prev != null) return prev
          const stored = Number(localStorage.getItem(CHAR_KEY))
          if (stored && r.characters.some((c) => c.id === stored)) return stored
          const act = r.characters.find(
            (c) => c.name.toLowerCase() === active.toLowerCase(),
          )
          return act?.id ?? r.characters[0]?.id ?? null
        })
      })
      .catch(() => setChars([]))
  }, [active])

  // Disciplines that actually have recipes (minus common combines).
  useEffect(() => {
    getRecipeTradeskills()
      .then((list) => {
        const usable = list
          .filter((d) => !isCommonCombine(d.tradeskill))
          .sort((a, b) =>
            tradeskillLabel(a.tradeskill).localeCompare(tradeskillLabel(b.tradeskill)),
          )
        setDisciplines(usable)
        setTsId((prev) => {
          if (prev != null) return prev
          const stored = Number(localStorage.getItem(SKILL_KEY))
          if (usable.some((d) => d.tradeskill === stored)) return stored
          return usable[0]?.tradeskill ?? null
        })
      })
      .catch(() => setDisciplines([]))
  }, [])

  // The selected character's tradeskill values (for current skill + cap).
  const loadSkills = useCallback(() => {
    if (charId == null) return
    getCharacterTradeskills(charId)
      .then((r) => setCharSkills(r.tradeskills ?? []))
      .catch(() => setCharSkills([]))
  }, [charId])

  useEffect(() => {
    setCharSkills(null)
    loadSkills()
  }, [loadSkills])

  // Re-camp / re-export refreshes current skills.
  const onWs = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ZealQuarmy) loadSkills()
  }, [loadSkills])
  useWebSocket(onWs)

  const skillEntry = useMemo(
    () => charSkills?.find((s) => s.skill_id === tsId) ?? null,
    [charSkills, tsId],
  )
  const untrained = !!skillEntry?.untrained
  const exportSkill = skillEntry && !untrained ? skillEntry.value : 0
  const cap = skillEntry?.cap ?? 0

  // Prefill the skill window from the export whenever the character or
  // discipline changes (once the skills have loaded). Manual edits stick until
  // the next change.
  useEffect(() => {
    if (charSkills == null || tsId == null) return
    setStart(exportSkill)
    setTarget(cap > 0 ? cap : FALLBACK_TARGET)
  }, [charSkills, tsId, exportSkill, cap])

  function selectChar(id: number) {
    setCharId(id)
    localStorage.setItem(CHAR_KEY, String(id))
  }
  function selectSkill(id: number) {
    setTsId(id)
    setExcluded({})
    setCustomActive(false)
    localStorage.setItem(SKILL_KEY, String(id))
  }
  function excludeRecipe(id: number, name: string) {
    setExcluded((prev) => ({ ...prev, [id]: name }))
    setCustomActive(true)
  }
  function includeRecipe(id: number) {
    setExcluded((prev) => {
      const next = { ...prev }
      delete next[id]
      if (Object.keys(next).length === 0) setCustomActive(false)
      return next
    })
  }
  function resetExclusions() {
    setExcluded({})
    setCustomActive(false)
  }

  // Fetch the plan (debounced) whenever an input changes and is valid.
  const reqId = useRef(0)
  useEffect(() => {
    if (charId == null || tsId == null) {
      setPlan(null)
      return
    }
    const startNum = start === '' ? 0 : start
    const targetNum = target === '' ? 0 : target
    if (targetNum > 0 && targetNum <= startNum) {
      setPlan(null)
      setError(false)
      return
    }
    const mine = ++reqId.current
    setLoading(true)
    setError(false)
    const t = setTimeout(() => {
      getTradeskillLevelingPlan(charId, {
        tradeskill: tsId,
        startSkill: start === '' ? undefined : start,
        targetSkill: targetNum,
        objective,
        allowFarming,
        avoidOtherTradeskills: avoidOther,
        excludeRecipeIds: customActive ? Object.keys(excluded).map(Number) : [],
        skillupBonus: maelin ? MAELIN_SKILLUP_PCT : 0,
      })
        .then((p) => {
          if (mine !== reqId.current) return
          setPlan(p)
          setLoading(false)
        })
        .catch(() => {
          if (mine !== reqId.current) return
          setError(true)
          setLoading(false)
        })
    }, 300)
    return () => clearTimeout(t)
  }, [charId, tsId, start, target, objective, allowFarming, avoidOther, excluded, customActive, maelin])

  const label = tsId != null ? tradeskillLabel(tsId) : ''
  const targetInvalid =
    start !== '' && target !== '' && target <= start

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-4">
      <div className="flex items-center gap-2">
        <Route size={18} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold">Tradeskill Leveling</h1>
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
          the most efficient path to raise a tradeskill
        </span>
      </div>

      {/* Controls */}
      <div
        className="flex flex-col gap-3 rounded-lg border p-3"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Character">
            <select
              value={charId ?? ''}
              onChange={(e) => selectChar(Number(e.target.value))}
              className="select"
              style={selectStyle}
            >
              {chars.length === 0 && <option value="">No characters</option>}
              {chars.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </Field>

          <Field label="Tradeskill">
            <select
              value={tsId ?? ''}
              onChange={(e) => selectSkill(Number(e.target.value))}
              style={selectStyle}
            >
              {disciplines.length === 0 && <option value="">—</option>}
              {disciplines.map((d) => (
                <option key={d.tradeskill} value={d.tradeskill}>
                  {tradeskillLabel(d.tradeskill)}
                </option>
              ))}
            </select>
          </Field>

          <Field label="From" hint={skillEntry && !untrained ? 'your skill' : undefined}>
            <input
              type="number"
              min={0}
              max={300}
              value={start}
              onChange={(e) => setStart(e.target.value === '' ? '' : Number(e.target.value))}
              style={numStyle}
            />
          </Field>

          <Field label="To" hint={cap > 0 ? `cap ${cap}` : undefined}>
            <input
              type="number"
              min={0}
              max={300}
              value={target}
              onChange={(e) => setTarget(e.target.value === '' ? '' : Number(e.target.value))}
              style={numStyle}
            />
          </Field>
        </div>

        <div className="flex flex-wrap items-center gap-4">
          {/* Objective toggle */}
          <div className="flex overflow-hidden rounded-md border" style={{ borderColor: 'var(--color-border)' }}>
            <ObjButton
              active={!customActive && objective === 'fastest'}
              onClick={() => { setObjective('fastest'); setCustomActive(false) }}
              icon={<Gauge size={13} />}
              label="Fastest"
            />
            <ObjButton
              active={!customActive && objective === 'cheapest'}
              onClick={() => { setObjective('cheapest'); setCustomActive(false) }}
              icon={<Coins size={13} />}
              label="Cheapest"
            />
            {Object.keys(excluded).length > 0 && (
              <ObjButton
                active={customActive}
                onClick={() => setCustomActive(true)}
                icon={<Ban size={13} />}
                label="Custom"
              />
            )}
          </div>

          <label className="flex cursor-pointer items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
            <input type="checkbox" checked={allowFarming} onChange={(e) => setAllowFarming(e.target.checked)} />
            Allow farmed / dropped components
          </label>

          <label
            className="flex cursor-pointer items-center gap-1.5 text-xs"
            style={{ color: 'var(--color-muted)' }}
            title="Only use recipes you can level with this tradeskill alone — no crafting in another discipline"
          >
            <input type="checkbox" checked={avoidOther} onChange={(e) => setAvoidOther(e.target.checked)} />
            Stay in this tradeskill
          </label>

          <label className="flex cursor-pointer items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
            <input type="checkbox" checked={maelin} onChange={(e) => setMaelin(e.target.checked)} />
            Maelin&apos;s (+{MAELIN_SKILLUP_PCT}% skill-ups)
          </label>
        </div>

        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {customActive
            ? `Custom — ${objective === 'fastest' ? 'fewest combines' : 'cheapest plat'} while routing around your excluded recipes below.`
            : objective === 'fastest'
              ? 'Fastest minimizes the number of combines.'
              : 'Cheapest minimizes vendor plat — farmed/dropped components have no price, so cost can be partial.'}
        </p>
      </div>

      {/* Custom exclusions — recipes routed around; click a chip to bring one
          back, or Reset to drop them all and return to the pure Fastest/
          Cheapest path. Stays visible even while viewing Fastest/Cheapest so
          switching to Custom doesn't lose earlier picks. */}
      {Object.keys(excluded).length > 0 && (
        <div
          className="flex flex-wrap items-center gap-1.5 rounded-lg border p-2 text-xs"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <Ban size={12} style={{ color: 'var(--color-muted)' }} />
          <span style={{ color: 'var(--color-muted)' }}>Excluded:</span>
          {Object.entries(excluded).map(([id, name]) => (
            <button
              key={id}
              onClick={() => includeRecipe(Number(id))}
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
              title="Include this recipe again"
            >
              {name}
              <X size={10} />
            </button>
          ))}
          <button
            onClick={resetExclusions}
            className="ml-1 underline"
            style={{ color: 'var(--color-muted)' }}
            title="Clear all exclusions and return to the pure Fastest/Cheapest path"
          >
            Reset
          </button>
        </div>
      )}

      {/* Status / edge notes */}
      {untrained && (
        <Note tone="warn">
          This character’s class or race can’t train {label}. The plan below shows how
          you’d level it from 0 if you could.
        </Note>
      )}
      {charSkills != null && charSkills.length === 0 && (
        <Note tone="muted">
          No tradeskill levels for this character yet — update Zeal (1.4.3+) and{' '}
          <span className="whitespace-nowrap">/camp</span> to auto-fill your current
          skill, or set “From” manually.
        </Note>
      )}
      {targetInvalid && (
        <Note tone="warn">“To” must be above “From”.</Note>
      )}

      {/* Results */}
      {loading && !plan && (
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Planning…</p>
      )}
      {error && (
        <Note tone="destructive">Couldn’t build a plan. Try again.</Note>
      )}
      {plan && !targetInvalid && (
        <PlanView plan={plan} loading={loading} onExclude={excludeRecipe} />
      )}
    </div>
  )
}

// ── Plan view ──────────────────────────────────────────────────────────────────

function PlanView({ plan, loading, onExclude }: {
  plan: TradeskillLevelingPlan
  loading: boolean
  onExclude: (id: number, name: string) => void
}): React.ReactElement {
  const label = plan.skill_name || tradeskillLabel(plan.tradeskill)
  const partial = plan.reached_skill < plan.target_skill
  const costPartial = plan.objective === 'cheapest' && !plan.cost_complete
  // A plan with no steps may arrive with stages null (e.g. every recipe filtered
  // out), so normalize before reading it.
  const stages = plan.stages ?? []
  const warnings = plan.warnings ?? []

  return (
    <div className="flex flex-col gap-3" style={{ opacity: loading ? 0.6 : 1 }}>
      {/* Summary */}
      <div
        className="flex flex-wrap items-center gap-x-5 gap-y-1 rounded-lg border p-3"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        <Stat
          value={`${plan.start_skill} → ${plan.reached_skill}`}
          label={partial ? `of ${plan.target_skill} target` : `${label} skill`}
        />
        <Stat value={plan.total_combines.toLocaleString()} label="combines" />
        <Stat
          value={plan.total_cost > 0 ? priceLabel(Math.round(plan.total_cost)) : '—'}
          label={costPartial ? 'known vendor cost' : 'vendor cost'}
        />
        <div className="ml-auto text-right text-[11px]" style={{ color: 'var(--color-muted)' }}>
          <div>{plan.objective === 'fastest' ? 'Fewest combines' : 'Cheapest plat'}</div>
          {plan.trade_stat > 0 && (
            <div>
              {plan.stat_name} {plan.trade_stat}
              {plan.stat_source === 'base+gear' ? ' (base + gear)' : ' (base)'}
            </div>
          )}
        </div>
      </div>

      {/* Warnings */}
      {warnings.map((w, i) => (
        <Note key={i} tone="warn">{sentenceCase(w)}</Note>
      ))}
      {costPartial && (
        <Note tone="muted">
          Cost is partial — some stages use farmed or dropped components that have no
          vendor price. The total is a lower bound.
        </Note>
      )}
      {plan.aa_reduce_pct > 0 && (
        <p className="text-[11px]" style={{ color: '#22c55e' }}>
          Your {label} mastery AA is applied (−{plan.aa_reduce_pct}% failure).
        </p>
      )}

      {/* Stages */}
      {stages.length === 0 ? (
        // The warnings above already explain an empty plan (at target, or every
        // recipe filtered out); only add a fallback when there are none.
        warnings.length === 0 ? (
          <Note tone="muted">
            No leveling steps for these settings.
          </Note>
        ) : null
      ) : (
        <div
          className="overflow-hidden rounded-lg border"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr
                className="text-left text-[11px] uppercase tracking-wider"
                style={{ color: 'var(--color-muted)', backgroundColor: 'var(--color-surface-2)' }}
              >
                <th className="px-3 py-2 font-semibold">Skill</th>
                <th className="px-3 py-2 font-semibold">Make</th>
                <th className="px-3 py-2 text-right font-semibold">Trivial</th>
                <th className="px-3 py-2 text-right font-semibold">Combines</th>
                <th className="px-3 py-2 text-right font-semibold">Cost</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {stages.map((s, i) => (
                <StageRow
                  key={`${s.recipe_id}-${i}`}
                  stage={s}
                  subCombines={plan.sub_combines}
                  last={i === stages.length - 1}
                  onExclude={onExclude}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function StageRow({ stage, subCombines, last, onExclude }: {
  stage: LevelingStage
  subCombines?: Record<string, SubCombineInfo>
  last: boolean
  onExclude: (id: number, name: string) => void
}): React.ReactElement {
  return (
    <tr
      style={{ borderTop: last ? undefined : undefined, borderBottom: last ? 'none' : '1px solid var(--color-border)' }}
    >
      <td className="px-3 py-2 font-mono text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {stage.from_skill}<span className="mx-0.5">→</span>{stage.to_skill}
      </td>
      <td className="px-3 py-2">
        <Link
          to={`/recipes?select=${stage.recipe_id}`}
          className="inline-flex items-center gap-1 hover:underline"
          style={{ color: 'var(--color-primary)' }}
        >
          {stage.recipe}
          <ExternalLink size={11} className="opacity-60" />
        </Link>
        {(stage.notes?.length ?? 0) > 0 && (
          <div className="mt-0.5 flex flex-wrap gap-1">
            {stage.notes!.map((n, i) => (
              <span
                key={i}
                className="rounded px-1.5 py-0.5 text-[10px]"
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
              >
                {n}
              </span>
            ))}
          </div>
        )}
        <SubCombineChips ids={stage.sub_combine_recipe_ids} lookup={subCombines} />
      </td>
      <td className="px-3 py-2 text-right font-mono text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {stage.trivial}
      </td>
      <td className="px-3 py-2 text-right font-mono text-xs tabular-nums">
        {stage.combines.toLocaleString()}
      </td>
      <td className="px-3 py-2 text-right text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {stage.cost_known ? priceLabel(Math.round(stage.cost)) : 'farmed'}
      </td>
      <td className="px-3 py-2 text-right">
        <button
          onClick={() => onExclude(stage.recipe_id, stage.recipe)}
          className="inline-flex items-center rounded p-1 hover:opacity-100"
          style={{ color: 'var(--color-muted)', opacity: 0.6 }}
          title="Exclude this recipe — re-route the plan around it"
        >
          <Ban size={13} />
        </button>
      </td>
    </tr>
  )
}

// SubCombineChips lists a stage's crafted intermediates ("must make it yourself"),
// each linking to its recipe. Cross-tradeskill ones are highlighted amber since
// they need a different, skill-gated discipline.
function SubCombineChips({ ids, lookup }: {
  ids?: number[]
  lookup?: Record<string, SubCombineInfo>
}): React.ReactElement | null {
  if (!ids?.length || !lookup) return null
  const items = ids.map((id) => lookup[String(id)]).filter(Boolean) as SubCombineInfo[]
  if (items.length === 0) return null
  return (
    <div className="mt-0.5 flex flex-wrap gap-1">
      {items.map((sc) => (
        <Link
          key={sc.recipe_id}
          to={`/recipes?select=${sc.recipe_id}`}
          className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] hover:underline"
          style={
            sc.cross_tradeskill
              ? { backgroundColor: 'rgba(234,179,8,0.15)', color: '#eab308' }
              : { backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }
          }
          title={
            sc.cross_tradeskill
              ? `Also needs ${sc.tradeskill_name} — craft "${sc.name}" (trivial ${sc.trivial}) first`
              : `Craft "${sc.name}" (trivial ${sc.trivial}) first`
          }
        >
          <Hammer size={9} />
          {sc.tradeskill_name} {sc.trivial} · {sc.name}
        </Link>
      ))}
    </div>
  )
}

// ── Small building blocks ──────────────────────────────────────────────────────

const selectStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  color: 'var(--color-foreground)',
  border: '1px solid var(--color-border)',
  borderRadius: 6,
  padding: '5px 8px',
  fontSize: 13,
  outline: 'none',
}
const numStyle: React.CSSProperties = { ...selectStyle, width: 76 }

function Field({ label, hint, children }: {
  label: string
  hint?: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
        {label}
        {hint && <span className="ml-1 font-normal normal-case tracking-normal">· {hint}</span>}
      </span>
      {children}
    </label>
  )
}

function ObjButton({ active, onClick, icon, label }: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium"
      style={{
        backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
        color: active ? 'var(--color-primary-foreground, #fff)' : 'var(--color-muted)',
      }}
    >
      {icon}
      {label}
    </button>
  )
}

function Stat({ value, label }: { value: string; label: string }): React.ReactElement {
  return (
    <div className="flex flex-col">
      <span className="text-lg font-bold tabular-nums" style={{ color: 'var(--color-foreground)' }}>
        {value}
      </span>
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{label}</span>
    </div>
  )
}

function Note({ tone, children }: {
  tone: 'warn' | 'muted' | 'destructive'
  children: React.ReactNode
}): React.ReactElement {
  const color =
    tone === 'warn' ? '#eab308' : tone === 'destructive' ? 'var(--color-destructive)' : 'var(--color-muted)'
  const Icon = tone === 'muted' ? Info : AlertTriangle
  return (
    <div className="flex items-start gap-2 text-xs" style={{ color }}>
      {/* h-4 matches the text-xs line height so the icon centers on the first
          line (and stays there when the text wraps). */}
      <span className="flex h-4 shrink-0 items-center">
        <Icon size={13} />
      </span>
      <span>{children}</span>
    </div>
  )
}

// sentenceCase upper-cases the first letter so warnings read uniformly,
// regardless of whether the solver or the API produced them.
function sentenceCase(s: string): string {
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : s
}

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown, ChevronRight, Plus } from 'lucide-react'
import {
  listCharacters,
  getCharacterTradeskills,
  getCharacterTradeskillAA,
  getTradeskillModifiers,
  getTradeskillChance,
  getSkillUpEstimate,
} from '../services/api'
import type { Character } from '../services/api'
import type {
  RecipeDetail,
  TradeskillModifier,
  TradeskillChance,
  TradeskillAA,
  SkillUpEstimate,
} from '../types/recipe'
import type { TradeskillView } from '../types/skill'
import { tradeskillLabel } from '../lib/enumsCache'
import { decodeSlots, slotsLabel } from '../lib/itemHelpers'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { ItemIcon } from './Icon'

// Common Combine disciplines (0 and the Quarm-only 75) have no trainable skill —
// success is governed entirely by the recipe's trivial/nofail, so the
// per-character skill lookup and modifier picker don't apply.
function isCommonCombine(tradeskill: number): boolean {
  return tradeskill === 0 || tradeskill === 75
}

// Remembers which character the calculator was last pointed at, so browsing
// recipe-to-recipe (which remounts this panel) keeps your selection.
const CHAR_KEY = 'tsCalcCharId'

// Maelin's Magical Concoction (spell 3999) grants SPA 504, a +75% tradeskill
// skill-up rate for an hour. It's the only source of this effect on Quarm.
const MAELIN_SKILLUP_PCT = 75

// Colour the odds by how safe the combine is. Mirrors the app's convention of
// green = good, amber = caution, red = bad.
function successColor(success: number): string {
  if (success >= 95) return '#22c55e'
  if (success >= 80) return '#84cc16'
  if (success >= 50) return '#eab308'
  return '#ef4444'
}

// TextTooltip is a lightweight hover tooltip that shows almost immediately —
// the native `title` attribute has a browser-fixed ~1s delay we can't tune. It
// clones its single child to attach hover handlers (no wrapper DOM, so it fits
// tight flex rows) and portals the card to <body> so the picker's scroll
// container can't clip it.
function TextTooltip({ content, children, delayMs = 120 }: {
  content: React.ReactNode
  children: React.ReactElement
  delayMs?: number
}): React.ReactElement {
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const show = (e: React.MouseEvent) => {
    const r = e.currentTarget.getBoundingClientRect()
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      setPos({
        top: r.bottom + 6,
        left: Math.min(r.left, window.innerWidth - 280),
      })
    }, delayMs)
  }
  const hide = () => {
    if (timer.current) clearTimeout(timer.current)
    setPos(null)
  }
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])

  return (
    <>
      {React.cloneElement(children, { onMouseEnter: show, onMouseLeave: hide })}
      {pos &&
        createPortal(
          <div
            className="pointer-events-none fixed z-[9999] max-w-[260px] rounded px-2 py-1 text-[11px] shadow-lg"
            style={{
              top: pos.top,
              left: pos.left,
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {content}
          </div>,
          document.body,
        )}
    </>
  )
}

interface Props {
  recipe: RecipeDetail
}

/**
 * RecipeSuccessPanel estimates a recipe's combine success/failure odds for a
 * chosen character, using their tradeskill skill (from the Zeal quarmy export)
 * plus optional skill-boosting item modifiers. The formula is Project Quarm's
 * (EQMacEmu) — see backend internal/tradeskill. Because only the single highest
 * worn skill-mod applies in-game, the picker uses the max of the selected
 * modifiers, not a sum.
 */
export default function RecipeSuccessPanel({ recipe }: Props): React.ReactElement | null {
  const { active } = useActiveCharacter()
  const common = isCommonCombine(recipe.tradeskill)

  const [chars, setChars] = useState<Character[]>([])
  const [charId, setCharId] = useState<number | null>(null)
  const [skills, setSkills] = useState<TradeskillView[] | null>(null)
  const [skillsError, setSkillsError] = useState(false)
  const [aaInfo, setAaInfo] = useState<TradeskillAA | null>(null)

  const [mods, setMods] = useState<TradeskillModifier[]>([])
  const [selectedMods, setSelectedMods] = useState<Set<number>>(new Set())
  const [manualMod, setManualMod] = useState('')
  const [showMods, setShowMods] = useState(false)

  const [result, setResult] = useState<TradeskillChance | null>(null)
  const [estimate, setEstimate] = useState<SkillUpEstimate | null>(null)
  const [maelin, setMaelin] = useState(false)

  // Load characters once; default the selection to the active character (or the
  // last-used one), so the panel is useful immediately.
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

  const loadSkills = useCallback(() => {
    if (common || charId == null) {
      setSkills([])
      return
    }
    setSkillsError(false)
    getCharacterTradeskills(charId)
      .then((r) => setSkills(r.tradeskills ?? []))
      .catch(() => setSkillsError(true))
  }, [common, charId])

  // The character's failure-reduction Mastery AA for this discipline (auto-
  // applied). Only Alchemy/Jewelry/Make Poison have one — applies is false
  // otherwise.
  const loadAA = useCallback(() => {
    if (common || charId == null) {
      setAaInfo(null)
      return
    }
    getCharacterTradeskillAA(charId, recipe.tradeskill)
      .then(setAaInfo)
      .catch(() => setAaInfo(null))
  }, [common, charId, recipe.tradeskill])

  useEffect(() => {
    setSkills(null)
    setAaInfo(null)
    loadSkills()
    loadAA()
  }, [loadSkills, loadAA])

  // A re-camp / re-export refreshes the character's skills and AAs.
  const onWs = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ZealQuarmy) {
      loadSkills()
      loadAA()
    }
  }, [loadSkills, loadAA])
  useWebSocket(onWs)

  // Load the modifier catalog for this discipline (skipped for common combines).
  useEffect(() => {
    setSelectedMods(new Set())
    setManualMod('')
    setShowMods(false)
    if (common) {
      setMods([])
      return
    }
    getTradeskillModifiers(recipe.tradeskill)
      .then(setMods)
      .catch(() => setMods([]))
  }, [recipe.tradeskill, common])

  const skillEntry = useMemo(
    () => skills?.find((s) => s.skill_id === recipe.tradeskill) ?? null,
    [skills, recipe.tradeskill],
  )
  const untrained = !!skillEntry?.untrained
  const rawSkill = skillEntry && !untrained ? skillEntry.value : 0
  const haveSkill = common || (skillEntry != null && !untrained)

  // In-game only the strongest worn skill-mod applies, so take the max of the
  // selected items and the manual entry.
  const effMod = useMemo(() => {
    const picks = mods.filter((m) => selectedMods.has(m.item_id)).map((m) => m.value)
    const manual = parseInt(manualMod)
    if (Number.isFinite(manual) && manual > 0) picks.push(manual)
    return picks.length ? Math.max(...picks) : 0
  }, [mods, selectedMods, manualMod])

  const aaReduce = aaInfo?.applies ? aaInfo.reduce_pct : 0

  // Recompute odds whenever the inputs change. The endpoint is a localhost
  // pure-function call, so a short debounce (for the manual field) is plenty.
  useEffect(() => {
    if (!haveSkill) {
      setResult(null)
      return
    }
    const t = setTimeout(() => {
      getTradeskillChance({
        trivial: recipe.trivial,
        skill: rawSkill,
        mod: effMod,
        aa: aaReduce,
        nofail: recipe.no_fail,
      })
        .then(setResult)
        .catch(() => setResult(null))
    }, 150)
    return () => clearTimeout(t)
  }, [haveSkill, recipe.trivial, recipe.no_fail, rawSkill, effMod, aaReduce])

  // Skill-up estimate — how many combines to raise the skill toward trivial.
  useEffect(() => {
    if (common || charId == null || !haveSkill) {
      setEstimate(null)
      return
    }
    const t = setTimeout(() => {
      getSkillUpEstimate(charId, {
        tradeskill: recipe.tradeskill,
        skill: rawSkill,
        trivial: recipe.trivial,
        mod: effMod,
        aa: aaReduce,
        skillupBonus: maelin ? MAELIN_SKILLUP_PCT : 0,
        nofail: recipe.no_fail,
      })
        .then(setEstimate)
        .catch(() => setEstimate(null))
    }, 200)
    return () => clearTimeout(t)
  }, [
    common, charId, haveSkill, recipe.tradeskill, recipe.trivial,
    recipe.no_fail, rawSkill, effMod, aaReduce, maelin,
  ])

  function selectChar(id: number) {
    setCharId(id)
    localStorage.setItem(CHAR_KEY, String(id))
  }

  function toggleMod(id: number) {
    setSelectedMods((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const label = tradeskillLabel(recipe.tradeskill)

  return (
    <div
      className="mt-3 rounded-lg border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div
        className="flex items-center justify-between gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="min-w-0">
          <span
            className="text-[11px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Success Chance
          </span>
          <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            how often you keep the product
          </span>
        </div>
        {!common && chars.length > 0 && (
          <select
            value={charId ?? ''}
            onChange={(e) => selectChar(Number(e.target.value))}
            className="rounded px-2 py-1 text-xs outline-none"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {chars.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        )}
      </div>

      <div className="px-3 py-3">
        {common ? (
          <CommonCombineBody result={result} />
        ) : skillsError ? (
          <p className="text-xs" style={{ color: 'var(--color-destructive)' }}>
            Couldn’t load tradeskill data.
          </p>
        ) : skills === null ? (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>
        ) : untrained ? (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            This character’s class or race can’t train {label}.
          </p>
        ) : skillEntry == null ? (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            No {label} data for this character. Update Zeal (1.4.3+) and{' '}
            <span className="whitespace-nowrap">/camp</span> so the quarmy export
            includes tradeskill levels.
          </p>
        ) : (
          <OddsBody
            result={result}
            label={label}
            rawSkill={rawSkill}
            cap={skillEntry.cap ?? 0}
            effMod={effMod}
            skillNeeded={recipe.skill_needed}
            trivial={recipe.trivial}
            aa={aaInfo}
          />
        )}

        {/* Skill-up estimate */}
        {!common && haveSkill && estimate && (
          <SkillUpSection
            estimate={estimate}
            label={label}
            maelin={maelin}
            onMaelin={setMaelin}
          />
        )}

        {/* Modifier picker */}
        {!common && haveSkill && mods.length > 0 && (
          <div className="mt-3 border-t pt-2" style={{ borderColor: 'var(--color-border)' }}>
            <button
              onClick={() => setShowMods((v) => !v)}
              className="flex items-center gap-1.5 text-xs"
              style={{ color: 'var(--color-primary)' }}
            >
              {showMods ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              <Plus size={12} />
              Skill modifiers
              {effMod > 0 && (
                <span
                  className="ml-1 rounded px-1.5 py-0.5 text-[10px] font-semibold"
                  style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
                >
                  +{effMod}%
                </span>
              )}
            </button>
            {showMods && (
              <div className="mt-2 flex flex-col gap-1.5">
                <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  Pick any skill-boosting {label} items — owned or not. These raise
                  success (not skill-up speed). In-game only the strongest applies, so
                  this uses the highest, not the sum. Each one takes its gear slot (shown
                  on hover), so you’d swap out whatever you normally wear there.
                </p>
                <div className="max-h-44 overflow-y-auto pr-1">
                  {mods.map((m) => {
                    const list = decodeSlots(m.slots)
                    const slotShort = list.length > 1 ? `${list[0]}+` : list[0] ?? ''
                    const slotFull = slotsLabel(m.slots)
                    return (
                      <TextTooltip
                        key={m.item_id}
                        content={
                          <div>
                            <div className="font-semibold">{m.name}</div>
                            <div>+{m.value}% {label} skill</div>
                            {slotFull && (
                              <div style={{ color: 'var(--color-muted)' }}>Slot: {slotFull}</div>
                            )}
                          </div>
                        }
                      >
                        <label
                          className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-xs"
                          style={{ color: 'var(--color-foreground)' }}
                        >
                          <input
                            type="checkbox"
                            checked={selectedMods.has(m.item_id)}
                            onChange={() => toggleMod(m.item_id)}
                          />
                          <ItemIcon id={m.icon} name={m.name} size={18} />
                          <span className="min-w-0 flex-1 truncate">{m.name}</span>
                          {slotShort && (
                            <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                              {slotShort}
                            </span>
                          )}
                          <span
                            className="shrink-0 font-mono tabular-nums"
                            style={{ color: 'var(--color-muted)' }}
                          >
                            +{m.value}%
                          </span>
                        </label>
                      </TextTooltip>
                    )
                  })}
                </div>
                <label className="mt-1 flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                  Other (+%)
                  <input
                    type="number"
                    min={0}
                    max={200}
                    value={manualMod}
                    onChange={(e) => setManualMod(e.target.value)}
                    placeholder="0"
                    className="w-16 rounded px-2 py-1 text-xs outline-none"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      color: 'var(--color-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  />
                </label>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Bodies ───────────────────────────────────────────────────────────────────

function CommonCombineBody({ result }: { result: TradeskillChance | null }): React.ReactElement {
  return (
    <div>
      <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
        Common combine — no tradeskill skill required.
      </p>
      {result && (
        <div className="mt-2">
          <ChanceBar result={result} />
        </div>
      )}
    </div>
  )
}

interface OddsBodyProps {
  result: TradeskillChance | null
  label: string
  rawSkill: number
  cap: number
  effMod: number
  skillNeeded: number
  trivial: number
  aa: TradeskillAA | null
}

function OddsBody({ result, label, rawSkill, cap, effMod, skillNeeded, trivial, aa }: OddsBodyProps): React.ReactElement {
  const belowMin = skillNeeded > 0 && rawSkill < skillNeeded
  // A skill-mod is a % of your skill, so at low skill it can round down to no
  // gain (e.g. +10% of 1 = 1). Flag that instead of showing a no-op "→ 1".
  const modNoGain = effMod > 0 && !!result && result.eff_skill <= rawSkill

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        <span style={{ color: 'var(--color-foreground)' }}>{label}</span>
        <span className="font-mono tabular-nums">
          {rawSkill}
          {cap > 0 ? ` / ${cap}` : ''}
        </span>
        {effMod > 0 && result && result.eff_skill > rawSkill && (
          <span className="font-mono tabular-nums" style={{ color: 'var(--color-primary)' }}>
            → {result.eff_skill} effective skill
          </span>
        )}
      </div>

      {result && <ChanceBar result={result} />}

      {belowMin && (
        <p className="text-[11px]" style={{ color: '#eab308' }}>
          Requires skill {skillNeeded} to attempt this combine.
        </p>
      )}

      {modNoGain && (
        <p className="text-[11px]" style={{ color: '#eab308' }}>
          +{effMod}% skill rounds to no gain at skill {rawSkill} — skill-mods are a
          percentage of your skill, so they only help once it’s higher.
        </p>
      )}

      {result && <FloorNote result={result} trivial={trivial} effMod={effMod} />}

      <AANote aa={aa} />
    </div>
  )
}

// AANote surfaces the auto-applied Mastery AA (or nudges toward training it).
// Only shown for the three disciplines the server honors (applies === true).
function AANote({ aa }: { aa: TradeskillAA | null }): React.ReactElement | null {
  if (!aa?.applies) return null
  if (aa.rank > 0) {
    return (
      <p className="text-[11px]" style={{ color: '#22c55e' }}>
        {aa.name} {aa.rank} applied — −{aa.reduce_pct}% failure.
      </p>
    )
  }
  return (
    <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
      Training {aa.name} would cut failure by up to 50%.
    </p>
  )
}

// SkillUpSection shows how many combines it takes to raise the skill toward the
// recipe's trivial. Skill-up speed tracks the character's stats, so the estimate
// uses their real total (base + equipped gear).
interface SkillUpSectionProps {
  estimate: SkillUpEstimate
  label: string
  maelin: boolean
  onMaelin: (v: boolean) => void
}

function SkillUpSection({ estimate: e, label, maelin, onMaelin }: SkillUpSectionProps): React.ReactElement {
  return (
    <div className="mt-3 border-t pt-2" style={{ borderColor: 'var(--color-border)' }}>
      <div className="mb-1">
        <span className="text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
          Skill-ups
        </span>
        <span className="ml-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          how fast your skill rises
        </span>
      </div>

      {e.maxed ? (
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {e.at_cap
            ? `At your class/level ${label} cap (${e.cap}) — no more skill-ups here.`
            : `Skill ${e.current_skill} has reached this recipe's trivial (${e.trivial}) — it no longer raises ${label}, though you can still make it.`}
        </p>
      ) : e.impractical ? (
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Not enough stat data to estimate skill-ups.
        </p>
      ) : (
        <div className="flex flex-col gap-1">
          <div className="flex items-baseline gap-2">
            <span className="text-base font-bold tabular-nums" style={{ color: 'var(--color-foreground)' }}>
              ≈{e.attempts_to_target.toLocaleString()}
            </span>
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              combines until it goes trivial at skill {e.target_skill} (+{e.points_to_go} to go)
            </span>
          </div>
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            ≈{e.attempts_to_next.toLocaleString()} combines per skill-up at skill {e.current_skill} —
            most combines don’t raise skill.
          </p>
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Based on {e.stat_name} {e.trade_stat}
            {e.stat_source === 'base+gear' ? ' (base + equipped gear)' : ' (base stats)'}
            ; failures skill up at half rate. Assumes you make this recipe the whole way
            {e.stat_source === 'base+gear' ? ' in your current gear' : ''}.
          </p>
          <label className="mt-0.5 flex cursor-pointer items-center gap-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            <input type="checkbox" checked={maelin} onChange={(ev) => onMaelin(ev.target.checked)} />
            Maelin&apos;s Magical Concoction (+{MAELIN_SKILLUP_PCT}% skill-up on failed combines)
          </label>
        </div>
      )}
    </div>
  )
}

function ChanceBar({ result }: { result: TradeskillChance }): React.ReactElement {
  const color = result.no_fail ? '#22c55e' : successColor(result.success)
  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-lg font-bold tabular-nums" style={{ color }}>
          {result.success}% <span className="text-xs font-normal">success</span>
        </span>
        <span className="text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
          {result.failure}% fail
        </span>
      </div>
      <div
        className="h-2 w-full overflow-hidden rounded-full"
        style={{ backgroundColor: 'var(--color-surface-2)' }}
      >
        <div
          className="h-full rounded-full transition-all"
          style={{ width: `${result.success}%`, backgroundColor: color }}
        />
      </div>
    </div>
  )
}

function FloorNote({ result, trivial, effMod }: { result: TradeskillChance; trivial: number; effMod: number }): React.ReactElement | null {
  if (result.no_fail) {
    return (
      <p className="text-[11px]" style={{ color: '#22c55e' }}>
        No-fail combine — always succeeds.
      </p>
    )
  }
  if (result.at_floor) {
    return (
      <p className="text-[11px]" style={{ color: '#22c55e' }}>
        At the 5% failure floor — as safe as this recipe gets.
      </p>
    )
  }
  if (!result.floor_reachable) {
    return (
      <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        This recipe’s trivial is high enough that it never drops below{' '}
        {result.failure}% failure, even at max skill.
      </p>
    )
  }
  const gap = Math.max(0, result.floor_skill - result.eff_skill)
  // For low-trivial recipes the 5% floor sits ABOVE trivial — a common source
  // of confusion, since "trivial" only stops skill-ups, not failures. Say so.
  const floorAboveTrivial = result.floor_skill > trivial
  const failAtTrivial = Math.round(100 - result.success_at_trivial)
  return (
    <>
      {floorAboveTrivial && (
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Trivial {trivial} stops skill-ups, not failures — even at skill {trivial} you’d
          still fail ~{failAtTrivial}%.
        </p>
      )}
      <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        {effMod > 0 ? (
          <>+{gap} more effective skill reaches the 5% failure floor.</>
        ) : floorAboveTrivial ? (
          <>
            Success keeps climbing past trivial — the 5% fail floor is at skill{' '}
            {result.floor_skill}, reached by skilling up on higher recipes.
          </>
        ) : (
          <>Reach skill {result.floor_skill} (+{gap}) for the 5% failure floor.</>
        )}
      </p>
    </>
  )
}

import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, Plus } from 'lucide-react'
import {
  listCharacters,
  getCharacterTradeskills,
  getTradeskillModifiers,
  getTradeskillChance,
} from '../services/api'
import type { Character } from '../services/api'
import type { RecipeDetail, TradeskillModifier, TradeskillChance } from '../types/recipe'
import type { TradeskillView } from '../types/skill'
import { tradeskillLabel } from '../lib/enumsCache'
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

// Colour the odds by how safe the combine is. Mirrors the app's convention of
// green = good, amber = caution, red = bad.
function successColor(success: number): string {
  if (success >= 95) return '#22c55e'
  if (success >= 80) return '#84cc16'
  if (success >= 50) return '#eab308'
  return '#ef4444'
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

  const [mods, setMods] = useState<TradeskillModifier[]>([])
  const [selectedMods, setSelectedMods] = useState<Set<number>>(new Set())
  const [manualMod, setManualMod] = useState('')
  const [showMods, setShowMods] = useState(false)

  const [result, setResult] = useState<TradeskillChance | null>(null)

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

  useEffect(() => {
    setSkills(null)
    loadSkills()
  }, [loadSkills])

  // A re-camp / re-export refreshes the character's skills.
  const onWs = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ZealQuarmy) loadSkills()
  }, [loadSkills])
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
        nofail: recipe.no_fail,
      })
        .then(setResult)
        .catch(() => setResult(null))
    }, 150)
    return () => clearTimeout(t)
  }, [haveSkill, recipe.trivial, recipe.no_fail, rawSkill, effMod])

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
        <span
          className="text-[11px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Success Chance
        </span>
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
                  Pick any skill-boosting {label} items — owned or not. In-game only
                  the strongest one applies, so this uses the highest, not the sum.
                </p>
                <div className="max-h-44 overflow-y-auto pr-1">
                  {mods.map((m) => (
                    <label
                      key={m.item_id}
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
                      <span
                        className="shrink-0 font-mono tabular-nums"
                        style={{ color: 'var(--color-muted)' }}
                      >
                        +{m.value}%
                      </span>
                    </label>
                  ))}
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
}

function OddsBody({ result, label, rawSkill, cap, effMod, skillNeeded }: OddsBodyProps): React.ReactElement {
  const belowMin = skillNeeded > 0 && rawSkill < skillNeeded

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        <span style={{ color: 'var(--color-foreground)' }}>{label}</span>
        <span className="font-mono tabular-nums">
          {rawSkill}
          {cap > 0 ? ` / ${cap}` : ''}
        </span>
        {effMod > 0 && result && (
          <span className="font-mono tabular-nums" style={{ color: 'var(--color-primary)' }}>
            → {result.eff_skill} eff
          </span>
        )}
      </div>

      {result && <ChanceBar result={result} />}

      {belowMin && (
        <p className="text-[11px]" style={{ color: '#eab308' }}>
          Requires skill {skillNeeded} to attempt this combine.
        </p>
      )}

      {result && <FloorNote result={result} effMod={effMod} />}
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

function FloorNote({ result, effMod }: { result: TradeskillChance; effMod: number }): React.ReactElement | null {
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
  return (
    <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
      {effMod > 0 ? (
        <>+{gap} more effective skill reaches the 5% failure floor.</>
      ) : (
        <>
          Reach skill {result.floor_skill} (+{gap}) for the 5% failure floor.
        </>
      )}
    </p>
  )
}

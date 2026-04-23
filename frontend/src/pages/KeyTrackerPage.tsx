import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Key, RefreshCw, AlertCircle, CheckCircle2, Circle, ChevronDown, ChevronRight } from 'lucide-react'
import { getKeys, getKeysProgress } from '../services/api'
import type { KeyDef, KeysProgressResponse, CharacterKeyProgress } from '../types/keys'

// ── Filter tabs ────────────────────────────────────────────────────────────────

type Filter = 'all' | 'in_progress' | 'complete'

// ── Helpers ────────────────────────────────────────────────────────────────────

function hasFinalKey(chars: CharacterKeyProgress[]): boolean {
  return chars.some((c) => c.final_item && (c.final_item.have || c.final_item.shared_bank))
}

function hasIntermediateItem(chars: CharacterKeyProgress[]): boolean {
  return chars.some((c) => c.intermediate_item && (c.intermediate_item.have || c.intermediate_item.shared_bank))
}

/** Returns the number of components obtained (own + shared bank) across all characters for one key. */
function countHave(chars: CharacterKeyProgress[], componentCount: number, intermediateCoverCount = 0): number {
  if (chars.length === 0 || componentCount === 0) return 0
  // Holding the assembled final key short-circuits component tracking.
  if (hasFinalKey(chars)) return componentCount
  let have = 0
  for (let i = 0; i < componentCount; i++) {
    // Components within the intermediate cover range are complete if any character
    // holds the intermediate item (the combine consumed those components).
    if (i < intermediateCoverCount && hasIntermediateItem(chars)) {
      have++
      continue
    }
    const anyHave = chars.some((c) => {
      const cs = c.components[i]
      return cs && (cs.have || cs.shared_bank)
    })
    if (anyHave) have++
  }
  return have
}

function keyIsComplete(chars: CharacterKeyProgress[], componentCount: number, intermediateCoverCount = 0): boolean {
  if (hasFinalKey(chars)) return true
  return componentCount > 0 && countHave(chars, componentCount, intermediateCoverCount) === componentCount
}

function keyIsInProgress(chars: CharacterKeyProgress[], componentCount: number, intermediateCoverCount = 0): boolean {
  if (hasFinalKey(chars)) return false
  const h = countHave(chars, componentCount, intermediateCoverCount)
  return h > 0 && h < componentCount
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface ProgressBarProps {
  have: number
  total: number
}

function ProgressBar({ have, total }: ProgressBarProps): React.ReactElement {
  const pct = total === 0 ? 0 : Math.round((have / total) * 100)
  const complete = have === total && total > 0
  return (
    <div className="flex items-center gap-2">
      <div
        className="h-1.5 flex-1 rounded-full overflow-hidden"
        style={{ backgroundColor: 'var(--color-surface-2)' }}
      >
        <div
          className="h-full rounded-full transition-all"
          style={{
            width: `${pct}%`,
            backgroundColor: complete ? 'var(--color-success)' : 'var(--color-primary)',
          }}
        />
      </div>
      <span
        className="text-[10px] tabular-nums shrink-0"
        style={{ color: complete ? 'var(--color-success)' : 'var(--color-muted-foreground)' }}
      >
        {have} / {total}
      </span>
    </div>
  )
}

interface KeyCardProps {
  keyDef: KeyDef
  chars: CharacterKeyProgress[]
  defaultOpen?: boolean
}

function KeyCard({ keyDef, chars, defaultOpen = false }: KeyCardProps): React.ReactElement {
  const [open, setOpen] = useState(defaultOpen)
  const coverCount = keyDef.intermediate_cover_count ?? 0
  const have = countHave(chars, keyDef.components.length, coverCount)
  const complete = keyIsComplete(chars, keyDef.components.length, coverCount)
  const hasExportChars = chars.filter((c) => c.has_export)

  return (
    <div
      className="rounded-lg overflow-hidden"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${complete ? 'var(--color-success)' : 'var(--color-border)'}`,
      }}
    >
      {/* Card header — click to expand */}
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left"
        onClick={() => setOpen((v) => !v)}
      >
        <span style={{ color: complete ? 'var(--color-success)' : 'var(--color-muted)' }}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
        <span className="flex-1 text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          {keyDef.name}
        </span>
        {chars.length > 0 && (
          <div className="w-32 shrink-0">
            <ProgressBar have={have} total={keyDef.components.length} />
          </div>
        )}
        {chars.length === 0 && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
            No exports
          </span>
        )}
      </button>

      {/* Expanded content */}
      {open && (
        <div
          className="border-t"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {/* Description */}
          <p
            className="px-4 py-2 text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            {keyDef.description}
          </p>

          {/* No characters with exports */}
          {hasExportChars.length === 0 && (
            <p className="px-4 pb-3 text-xs" style={{ color: 'var(--color-muted)' }}>
              No character inventory exports found. Log out of each character to generate Zeal exports.
            </p>
          )}

          {/* Component table */}
          {hasExportChars.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-xs border-collapse">
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--color-border)' }}>
                    <th
                      className="px-4 py-1.5 text-left font-semibold text-[10px] uppercase tracking-wider"
                      style={{ color: 'var(--color-muted)' }}
                    >
                      Component
                    </th>
                    {hasExportChars.map((c) => (
                      <th
                        key={c.character}
                        className="px-3 py-1.5 text-center font-semibold text-[10px] uppercase tracking-wider"
                        style={{ color: 'var(--color-muted)' }}
                      >
                        {c.character}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {/* Assembled key row (rendered above components, highlighted) */}
                  {keyDef.final_item && (
                    <tr
                      style={{
                        borderBottom: '1px solid var(--color-border)',
                        backgroundColor: 'var(--color-surface-2)',
                      }}
                    >
                      <td className="px-4 py-2">
                        <div className="flex flex-col gap-0.5">
                          <span
                            className="font-semibold"
                            style={{ color: 'var(--color-success)' }}
                          >
                            {keyDef.final_item.item_name}{' '}
                            <span
                              className="ml-1 text-[9px] px-1.5 py-0.5 rounded uppercase tracking-wider"
                              style={{
                                backgroundColor: 'var(--color-success)',
                                color: 'var(--color-surface)',
                              }}
                            >
                              Assembled Key
                            </span>
                          </span>
                          {keyDef.final_item.notes && (
                            <span style={{ color: 'var(--color-muted)' }} className="text-[10px]">
                              {keyDef.final_item.notes}
                            </span>
                          )}
                        </div>
                      </td>
                      {hasExportChars.map((charProg) => {
                        const fi = charProg.final_item
                        return (
                          <td key={charProg.character} className="px-3 py-2 text-center">
                            {fi?.have ? (
                              <span title="Keyed">
                                <CheckCircle2
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-success)' }}
                                />
                              </span>
                            ) : fi?.shared_bank ? (
                              <span
                                className="inline-block text-[9px] px-1.5 py-0.5 rounded font-medium"
                                style={{
                                  backgroundColor: 'var(--color-surface-2)',
                                  color: 'var(--color-primary)',
                                  border: '1px solid var(--color-border)',
                                }}
                                title="In Shared Bank"
                              >
                                SB
                              </span>
                            ) : (
                              <span title="Not assembled">
                                <Circle
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-muted)' }}
                                />
                              </span>
                            )}
                          </td>
                        )
                      })}
                    </tr>
                  )}
                  {/* Intermediate item row (e.g. Unadorned Scepter for Vex Thal) */}
                  {keyDef.intermediate_item && (
                    <tr
                      style={{
                        borderBottom: '1px solid var(--color-border)',
                        backgroundColor: 'var(--color-surface-2)',
                      }}
                    >
                      <td className="px-4 py-2">
                        <div className="flex flex-col gap-0.5">
                          <span
                            className="font-semibold"
                            style={{ color: 'var(--color-primary)' }}
                          >
                            {keyDef.intermediate_item.item_name}{' '}
                            <span
                              className="ml-1 text-[9px] px-1.5 py-0.5 rounded uppercase tracking-wider"
                              style={{
                                backgroundColor: 'var(--color-primary)',
                                color: 'var(--color-surface)',
                              }}
                            >
                              Intermediate Combine
                            </span>
                          </span>
                          {keyDef.intermediate_item.notes && (
                            <span style={{ color: 'var(--color-muted)' }} className="text-[10px]">
                              {keyDef.intermediate_item.notes}
                            </span>
                          )}
                        </div>
                      </td>
                      {hasExportChars.map((charProg) => {
                        const ii = charProg.intermediate_item
                        return (
                          <td key={charProg.character} className="px-3 py-2 text-center">
                            {ii?.have ? (
                              <span title="Have it — first-combine complete">
                                <CheckCircle2
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-primary)' }}
                                />
                              </span>
                            ) : ii?.shared_bank ? (
                              <span
                                className="inline-block text-[9px] px-1.5 py-0.5 rounded font-medium"
                                style={{
                                  backgroundColor: 'var(--color-surface-2)',
                                  color: 'var(--color-primary)',
                                  border: '1px solid var(--color-border)',
                                }}
                                title="In Shared Bank"
                              >
                                SB
                              </span>
                            ) : (
                              <span title="Not yet combined">
                                <Circle
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-muted)' }}
                                />
                              </span>
                            )}
                          </td>
                        )
                      })}
                    </tr>
                  )}
                  {keyDef.components.map((comp, ci) => (
                    <tr
                      key={comp.item_id}
                      style={{ borderBottom: '1px solid var(--color-border)' }}
                    >
                      {/* Component name + notes */}
                      <td className="px-4 py-2">
                        <div className="flex flex-col gap-0.5">
                          <span style={{ color: 'var(--color-foreground)' }}>{comp.item_name}</span>
                          {comp.notes && (
                            <span style={{ color: 'var(--color-muted)' }} className="text-[10px]">
                              {comp.notes}
                            </span>
                          )}
                        </div>
                      </td>
                      {/* Per-character status */}
                      {hasExportChars.map((charProg) => {
                        const cs = charProg.components[ci]
                        const keyedViaFinal = !!(charProg.final_item && (charProg.final_item.have || charProg.final_item.shared_bank))
                        const coveredByIntermediate = !!(
                          ci < coverCount &&
                          charProg.intermediate_item &&
                          (charProg.intermediate_item.have || charProg.intermediate_item.shared_bank)
                        )
                        return (
                          <td key={charProg.character} className="px-3 py-2 text-center">
                            {cs.have ? (
                              <span title="Have it">
                                <CheckCircle2
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-success)' }}
                                />
                              </span>
                            ) : cs.shared_bank ? (
                              <span
                                className="inline-block text-[9px] px-1.5 py-0.5 rounded font-medium"
                                style={{
                                  backgroundColor: 'var(--color-surface-2)',
                                  color: 'var(--color-primary)',
                                  border: '1px solid var(--color-border)',
                                }}
                                title="In Shared Bank"
                              >
                                SB
                              </span>
                            ) : keyedViaFinal ? (
                              <span title="Covered by assembled key">
                                <CheckCircle2
                                  size={14}
                                  className="inline-block opacity-40"
                                  style={{ color: 'var(--color-success)' }}
                                />
                              </span>
                            ) : coveredByIntermediate ? (
                              <span title="Covered by intermediate combine">
                                <CheckCircle2
                                  size={14}
                                  className="inline-block opacity-40"
                                  style={{ color: 'var(--color-primary)' }}
                                />
                              </span>
                            ) : (
                              <span title="Missing">
                                <Circle
                                  size={14}
                                  className="inline-block"
                                  style={{ color: 'var(--color-muted)' }}
                                />
                              </span>
                            )}
                          </td>
                        )
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Characters without exports */}
          {chars.filter((c) => !c.has_export).map((c) => (
            <p key={c.character} className="px-4 py-1 text-[10px]" style={{ color: 'var(--color-muted)' }}>
              {c.character}: no export
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function KeyTrackerPage(): React.ReactElement {
  const [keyDefs, setKeyDefs] = useState<KeyDef[]>([])
  const [progress, setProgress] = useState<KeysProgressResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<Filter>('all')
  const navigate = useNavigate()

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    Promise.all([getKeys(), getKeysProgress()])
      .then(([keysResp, progressResp]) => {
        setKeyDefs(keysResp.keys)
        setProgress(progressResp)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  // Build a lookup from key_id → CharacterKeyProgress[]
  const progressByKey = useMemo<Map<string, CharacterKeyProgress[]>>(() => {
    const m = new Map<string, CharacterKeyProgress[]>()
    if (!progress) return m
    for (const kp of progress.keys) {
      m.set(kp.key_id, kp.characters)
    }
    return m
  }, [progress])

  const filteredKeys = useMemo<KeyDef[]>(() => {
    if (filter === 'all') return keyDefs
    return keyDefs.filter((kd) => {
      const chars = progressByKey.get(kd.id) ?? []
      const cc = kd.intermediate_cover_count ?? 0
      if (filter === 'complete') return keyIsComplete(chars, kd.components.length, cc)
      if (filter === 'in_progress') return keyIsInProgress(chars, kd.components.length, cc)
      return true
    })
  }, [keyDefs, progressByKey, filter])

  // ── Loading / error states ───────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
        <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
          {error}
        </p>
        <button
          onClick={load}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  if (progress && !progress.configured) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Key size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          EQ Path Not Configured
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Set your EverQuest install path in{' '}
          <button
            className="underline"
            style={{ color: 'var(--color-primary)' }}
            onClick={() => navigate('/settings')}
          >
            Settings
          </button>
          , then log out of each character to generate Zeal inventory exports.
        </p>
      </div>
    )
  }

  // ── Main render ──────────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col">
      {/* Header bar */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Key size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Key Tracker
        </span>
        <button
          onClick={load}
          className="ml-auto flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <RefreshCw size={11} />
          Refresh
        </button>
      </div>

      {/* Filter tabs */}
      <div
        className="flex items-center gap-1 border-b px-4 shrink-0"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        {(['all', 'in_progress', 'complete'] as Filter[]).map((f) => {
          const label = f === 'all' ? 'All' : f === 'in_progress' ? 'In Progress' : 'Complete'
          const active = filter === f
          return (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className="px-3 py-2 text-xs font-medium transition-colors"
              style={{
                color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                borderBottom: active ? '2px solid var(--color-primary)' : '2px solid transparent',
              }}
            >
              {label}
            </button>
          )
        })}
      </div>

      {/* Key cards */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {filteredKeys.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              {filter === 'complete'
                ? 'No keys completed yet.'
                : filter === 'in_progress'
                  ? 'No keys in progress.'
                  : 'No keys defined.'}
            </p>
          </div>
        ) : (
          filteredKeys.map((kd) => (
            <KeyCard
              key={kd.id}
              keyDef={kd}
              chars={progressByKey.get(kd.id) ?? []}
            />
          ))
        )}
      </div>
    </div>
  )
}

import React from 'react'
import type { NPCOutlineRule } from '../../types/config'
import { CONSIDER_RULE_ID } from '../../lib/considerOutline'

const PIN_COLOR = '#c9a84c'

/**
 * Coloured frame drawn over an NPC overlay window when the target matches an
 * outline rule (the first match). Absolutely positioned, so the parent must
 * be a positioned element. When the target is also pinned, the pin's gold
 * ring moves just inside the rule frame so both stay visible. A label-only
 * rule draws no frame, just the pin ring when pinned.
 */
export function NPCOutlineFrame({
  rule,
  pinned,
}: {
  rule: NPCOutlineRule | undefined
  pinned: boolean
}): React.ReactElement | null {
  if (!rule) return null
  const layer: React.CSSProperties = {
    position: 'absolute',
    pointerEvents: 'none',
    zIndex: 50,
  }
  if (rule.labelOnly) {
    // The parent leaves the pin ring to us once any rule matched.
    return pinned ? (
      <div
        aria-hidden
        style={{ ...layer, inset: 0, border: `2px solid ${PIN_COLOR}`, borderRadius: 8 }}
      />
    ) : null
  }
  return (
    <>
      <div
        aria-hidden
        className={rule.flash ? 'npc-outline-flash' : undefined}
        style={{ ...layer, inset: 0, border: `3px solid ${rule.color}`, borderRadius: 8 }}
      />
      {pinned && (
        <div
          aria-hidden
          style={{ ...layer, inset: 3, border: `2px solid ${PIN_COLOR}`, borderRadius: 6 }}
        />
      )}
    </>
  )
}

/**
 * Names of the matched outline rules, so it's clear *why* the window is
 * outlined. The first pill is the rule drawing the frame; the rest also
 * matched but lost on priority.
 */
export function NPCOutlineLabels({
  rules,
}: {
  rules: NPCOutlineRule[]
}): React.ReactElement | null {
  if (rules.length === 0) return null
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, padding: '4px 10px 0' }}>
      {rules.map((r, i) => (
        <span
          key={r.id}
          title={
            r.id === CONSIDER_RULE_ID
              ? 'Standing from your last /con of this target. Cleared when you retarget, zone, ' +
                'or your invis, illusion or feign state changes.'
              : i === 0
                ? 'Outline rule (drawing the frame)'
                : 'Outline rule (also matched)'
          }
          style={{
            backgroundColor: i === 0 ? r.color : 'transparent',
            border: `1px solid ${r.color}`,
            color: i === 0 ? '#fff' : r.color,
            fontSize: 10,
            fontWeight: 700,
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            borderRadius: 3,
            padding: '1px 6px',
            lineHeight: 1.4,
          }}
        >
          {r.name || 'Rule'}
        </span>
      ))}
    </div>
  )
}

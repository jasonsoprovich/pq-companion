import React from 'react'

/**
 * Two-state segmented control for the Combat Log / History pages: switch
 * between a per-fight list and a single pooled breakdown across the fights
 * currently shown. Both labels stay visible (unlike a single highlighted
 * toggle) so the meaning is self-evident, and the active "All Fights" pill
 * carries the live count of fights being pooled.
 */
export function FightViewToggle({
  combined,
  count,
  onChange,
}: {
  combined: boolean
  count: number
  onChange: (next: boolean) => void
}): React.ReactElement {
  return (
    <div
      role="group"
      aria-label="Fight view"
      style={{
        display: 'inline-flex',
        border: '1px solid var(--color-border)',
        borderRadius: 4,
        overflow: 'hidden',
      }}
    >
      <Pill active={!combined} onClick={() => onChange(false)} title="List each fight separately">
        Per Fight
      </Pill>
      <Pill
        active={combined}
        onClick={() => onChange(true)}
        title="Pool the fights currently shown into one combined breakdown"
      >
        All Fights{count > 0 ? ` · ${count}` : ''}
      </Pill>
    </div>
  )
}

function Pill({
  active,
  onClick,
  title,
  children,
}: {
  active: boolean
  onClick: () => void
  title: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      aria-pressed={active}
      style={{
        padding: '4px 9px',
        fontSize: 11,
        border: 'none',
        background: active ? 'var(--color-primary)' : 'var(--color-background)',
        color: active ? '#000' : 'var(--color-muted-foreground)',
        fontWeight: active ? 600 : 400,
        cursor: 'pointer',
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </button>
  )
}

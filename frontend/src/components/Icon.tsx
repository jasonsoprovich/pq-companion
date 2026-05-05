import React, { useState } from 'react'

type IconKind = 'item' | 'spell'

type EQIconProps = {
  id: number | null | undefined
  kind: IconKind
  size?: number
  name?: string
  className?: string
  /**
   * Image loading strategy. Defaults to 'lazy' for long lists (spell book,
   * search results). Pass 'eager' for short, always-visible lists like
   * overlays where lazy loading can drop icons that were briefly off-screen.
   */
  loading?: 'lazy' | 'eager'
}

function EQIcon({ id, kind, size = 24, name, className, loading = 'lazy' }: EQIconProps): React.ReactElement {
  const [errored, setErrored] = useState(false)
  const hasIcon = typeof id === 'number' && id > 0 && !errored

  const dim = { width: size, height: size }
  const baseClass = 'inline-block shrink-0 rounded-sm'
  const merged = className ? `${baseClass} ${className}` : baseClass

  if (!hasIcon) {
    return (
      <span
        aria-hidden="true"
        className={merged}
        style={{
          ...dim,
          backgroundColor: 'var(--color-border)',
          border: '1px solid var(--color-border-subtle)',
        }}
      />
    )
  }

  const alt = name ?? `${kind} icon ${id}`
  return (
    <img
      src={`icons/${id}.png`}
      alt={alt}
      title={name}
      loading={loading}
      decoding="async"
      onError={() => setErrored(true)}
      className={merged}
      style={dim}
    />
  )
}

export function ItemIcon(props: Omit<EQIconProps, 'kind'>): React.ReactElement {
  return <EQIcon {...props} kind="item" />
}

export function SpellIcon(props: Omit<EQIconProps, 'kind'>): React.ReactElement {
  return <EQIcon {...props} kind="spell" />
}

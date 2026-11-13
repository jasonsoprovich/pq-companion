/**
 * SettingsSection — the card wrapper used for settings in the Overlays hub
 * drawer, matching the Settings page's section style.
 */
import React from 'react'

export function SettingsSection({
  title,
  description,
  children,
}: {
  title: string
  description?: React.ReactNode
  children: React.ReactNode
}): React.ReactElement {
  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className={`${description ? 'mb-1' : 'mb-3'} text-sm font-semibold uppercase tracking-wide`}
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </h2>
      {description && (
        <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{description}</p>
      )}
      {children}
    </section>
  )
}

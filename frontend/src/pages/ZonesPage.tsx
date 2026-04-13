import React from 'react'
import { Map } from 'lucide-react'

export default function ZonesPage(): React.ReactElement {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <Map size={32} style={{ color: 'var(--color-muted)' }} />
      <h2 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
        Zone Explorer
      </h2>
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Coming in Task 2.6
      </p>
    </div>
  )
}

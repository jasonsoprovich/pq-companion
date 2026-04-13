import React from 'react'
import { Skull } from 'lucide-react'

export default function NpcsPage(): React.ReactElement {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <Skull size={32} style={{ color: 'var(--color-muted)' }} />
      <h2 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
        NPC Explorer
      </h2>
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Coming in Task 2.5
      </p>
    </div>
  )
}

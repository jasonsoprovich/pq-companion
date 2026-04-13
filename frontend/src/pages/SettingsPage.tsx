import React from 'react'
import { Settings } from 'lucide-react'

export default function SettingsPage(): React.ReactElement {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <Settings size={32} style={{ color: 'var(--color-muted)' }} />
      <h2 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
        Settings
      </h2>
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Coming soon
      </p>
    </div>
  )
}

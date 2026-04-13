import React from 'react'

export default function App(): React.ReactElement {
  return (
    <div className="flex h-full items-center justify-center bg-(--color-background)">
      <div className="text-center">
        <h1 className="mb-1 text-3xl font-bold text-(--color-primary)">PQ Companion</h1>
        <p className="text-sm text-(--color-muted-foreground)">
          Project Quarm Desktop Companion
        </p>
        <p className="mt-6 text-xs text-(--color-muted)">
          Electron {window.electron?.versions.electron} · Chrome{' '}
          {window.electron?.versions.chrome}
        </p>
      </div>
    </div>
  )
}

import React from 'react'
import { AlertCircle, RefreshCw } from 'lucide-react'

interface ErrorBoundaryProps {
  /** Short label for what failed, e.g. "Spell Modifiers". */
  label?: string
  children: React.ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

// ErrorBoundary catches render-time exceptions in its subtree and shows an
// inline fallback instead of letting the error unmount the whole React tree
// (which presents as a black-screen lockup). Wrap any panel that renders
// backend-derived data where a single bad payload shouldn't take down the app.
//
// Class component because React only supports error boundaries via
// getDerivedStateFromError / componentDidCatch — there is no hook equivalent.
export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    // Surface in DevTools so the underlying bug is still diagnosable.
    console.error(`ErrorBoundary (${this.props.label ?? 'unknown'}) caught:`, error, info)
  }

  reset = (): void => {
    this.setState({ error: null })
  }

  render(): React.ReactNode {
    if (this.state.error) {
      return (
        <div
          className="flex flex-col items-center justify-center rounded-lg py-10 text-center"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <AlertCircle size={26} style={{ color: 'var(--color-danger)', marginBottom: '8px' }} />
          <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            {this.props.label ? `${this.props.label} hit an error` : 'Something went wrong'}
          </p>
          <p className="mt-1 max-w-sm text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            This section failed to render, but the rest of the app is fine. Try again, or
            switch selection.
          </p>
          <button
            onClick={this.reset}
            className="mt-3 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: 'pointer',
            }}
          >
            <RefreshCw size={12} />
            Retry
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

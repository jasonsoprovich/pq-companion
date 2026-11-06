import { useCallback, useState } from 'react'

// useStoredState is useCachedState for preferences: a JSON value under `key`
// in localStorage, so it survives app restarts as well as remounts. Use it
// for a toggle the user expects to stay how they left it (a map's 2D/3D
// switch, its layer chips); keep useCachedState for per-visit state like a
// search box.
//
// `valid` guards against a stale or hand-edited stored value; without it, a
// stored value is kept when it has the same type as `initial` (any non-null
// value when `initial` is null). Storage failures are swallowed and the
// state behaves like plain useState.
export function useStoredState<T>(
  key: string,
  initial: T,
  valid?: (v: unknown) => v is T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const [state, setState] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key)
      if (raw === null) return initial
      const v = JSON.parse(raw) as unknown
      if (valid ? valid(v) : initial === null ? v !== null : typeof v === typeof initial) return v as T
    } catch {
      /* fall through to the default */
    }
    return initial
  })

  const set = useCallback(
    (value: T | ((prev: T) => T)) => {
      setState((prev) => {
        const next = typeof value === 'function' ? (value as (p: T) => T)(prev) : value
        try {
          localStorage.setItem(key, JSON.stringify(next))
        } catch {
          /* noop */
        }
        return next
      })
    },
    [key],
  )

  return [state, set]
}

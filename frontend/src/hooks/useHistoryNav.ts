import { useNavigate, useLocation, useNavigationType } from 'react-router-dom'
import { useCallback, useEffect, useRef, useState } from 'react'

export function useHistoryNav() {
  const navigate = useNavigate()
  const location = useLocation()
  const navType = useNavigationType() // 'POP' | 'PUSH' | 'REPLACE'

  const stackRef = useRef<string[]>([location.pathname + location.search])
  const indexRef = useRef(0)
  // Set to true when we triggered the nav ourselves so the effect skips pushing
  const isNavRef = useRef(false)

  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)

  useEffect(() => {
    if (isNavRef.current) {
      isNavRef.current = false
      return
    }
    const key = location.pathname + location.search
    // POP is the initial mount (the stack initializer already seeded this
    // location) — pushing again produced [X, X], so canGoBack was true at boot
    // and the first Back was a no-op that lit Forward. Skip it.
    if (navType === 'POP') return
    // REPLACE (index-route redirects like /combat -> /combat/log) is not a new
    // history step — update the current entry in place instead of pushing, so
    // the stack stays 1:1 with real browser history and doesn't drift.
    if (navType === 'REPLACE') {
      stackRef.current[indexRef.current] = key
      return
    }
    // PUSH: truncate forward history on new navigation, then add.
    stackRef.current = stackRef.current.slice(0, indexRef.current + 1)
    stackRef.current.push(key)
    indexRef.current = stackRef.current.length - 1
    setCanGoBack(indexRef.current > 0)
    setCanGoForward(false)
  }, [location, navType])

  const goBack = useCallback(() => {
    if (indexRef.current > 0) {
      isNavRef.current = true
      indexRef.current--
      setCanGoBack(indexRef.current > 0)
      setCanGoForward(indexRef.current < stackRef.current.length - 1)
      navigate(-1)
    }
  }, [navigate])

  const goForward = useCallback(() => {
    if (indexRef.current < stackRef.current.length - 1) {
      isNavRef.current = true
      indexRef.current++
      setCanGoBack(indexRef.current > 0)
      setCanGoForward(indexRef.current < stackRef.current.length - 1)
      navigate(1)
    }
  }, [navigate])

  return { canGoBack, canGoForward, goBack, goForward }
}

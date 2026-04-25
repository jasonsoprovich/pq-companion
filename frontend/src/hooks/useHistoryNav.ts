import { useNavigate, useLocation } from 'react-router-dom'
import { useCallback, useEffect, useRef, useState } from 'react'

export function useHistoryNav() {
  const navigate = useNavigate()
  const location = useLocation()

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
    // Truncate forward history on new navigation
    stackRef.current = stackRef.current.slice(0, indexRef.current + 1)
    stackRef.current.push(key)
    indexRef.current = stackRef.current.length - 1
    setCanGoBack(indexRef.current > 0)
    setCanGoForward(false)
  }, [location])

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

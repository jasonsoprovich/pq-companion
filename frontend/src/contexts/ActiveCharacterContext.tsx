import React, { createContext, useContext, useState } from 'react'

interface ActiveCharacterContextValue {
  active: string
  manual: boolean
  setActive: (name: string, manual: boolean) => void
}

const ActiveCharacterContext = createContext<ActiveCharacterContextValue>({
  active: '',
  manual: false,
  setActive: () => {},
})

export function ActiveCharacterProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const [active, setActiveState] = useState('')
  const [manual, setManual] = useState(false)

  function setActive(name: string, isManual: boolean) {
    setActiveState(name)
    setManual(isManual)
  }

  return (
    <ActiveCharacterContext.Provider value={{ active, manual, setActive }}>
      {children}
    </ActiveCharacterContext.Provider>
  )
}

export function useActiveCharacter(): ActiveCharacterContextValue {
  return useContext(ActiveCharacterContext)
}

import { useEffect, useState } from 'react'

export function useVoices(): string[] {
  const [voices, setVoices] = useState<string[]>(() =>
    window.speechSynthesis?.getVoices().map((v) => v.name).sort() ?? [],
  )

  useEffect(() => {
    const synth = window.speechSynthesis
    if (!synth) return
    const load = () => {
      const list = synth.getVoices().map((v) => v.name).sort()
      if (list.length > 0) setVoices(list)
    }
    load()
    synth.addEventListener('voiceschanged', load)
    return () => synth.removeEventListener('voiceschanged', load)
  }, [])

  return voices
}

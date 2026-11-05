// Transport boundary only: shipping useMetronomeAlerts/fire remains intact.
export function playSound(...args: unknown[]): void {
  ;(window as any).__effects.push({ kind: 'sound', at: Date.now(), args })
}
export function speakText(...args: unknown[]): void {
  ;(window as any).__effects.push({ kind: 'tts', at: Date.now(), args })
}

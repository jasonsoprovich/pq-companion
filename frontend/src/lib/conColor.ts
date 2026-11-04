// Con colours, matching the server's /con (EQMacEmu Mob::GetLevelCon).
// Classic has no gray: green is the lowest con.

export type ConColor = 'green' | 'lightblue' | 'blue' | 'white' | 'yellow' | 'red'

// [highest player level in the bracket, green at diff <=, light blue at diff <=]
const BRACKETS: [number, number, number][] = [
  [8, -5, -4],
  [12, -6, -4],
  [16, -7, -5],
  [20, -8, -6],
  [24, -9, -7],
  [28, -10, -8],
  [30, -11, -9],
  [32, -12, -9],
  [36, -13, -10],
  [40, -14, -11],
  [44, -16, -12],
  [48, -17, -13],
  [52, -18, -14],
  [54, -19, -15],
  [56, -20, -15],
  [60, -21, -16],
  [61, -19, -14],
  [62, -17, -12],
]

export function levelCon(playerLevel: number, npcLevel: number): ConColor {
  const diff = npcLevel - playerLevel
  if (diff === 0) return 'white'
  if (diff >= 3) return 'red'
  if (diff >= 1) return 'yellow'
  // Levels 1-7 have no light blue.
  if (playerLevel <= 7) return diff <= -4 ? 'green' : 'blue'
  const b = BRACKETS.find(([max]) => playerLevel <= max)
  const [, green, light] = b ?? [0, -16, -11]
  if (diff <= green) return 'green'
  if (diff <= light) return 'lightblue'
  return 'blue'
}

export const CON_HEX: Record<ConColor, string> = {
  green: '#22c55e',
  lightblue: '#67e8f9',
  blue: '#3b82f6',
  white: '#f1f5f9',
  yellow: '#facc15',
  red: '#ef4444',
}

export const CON_LABEL: Record<ConColor, string> = {
  green: 'Green',
  lightblue: 'Light blue',
  blue: 'Dark blue',
  white: 'White',
  yellow: 'Yellow',
  red: 'Red',
}

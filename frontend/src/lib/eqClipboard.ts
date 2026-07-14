// EQ's chat input silently drops/truncates a pasted line once it grows too
// long. 255 is the conservative ceiling that keeps a line under both known
// failure modes reported by players: a per-line send cap around 255 chars,
// and a paste-into-textbox cap around 400 that fails even earlier than that
// if a line is much longer. Every "paste into EQ chat" clipboard feature in
// the app should build against this single constant.
export const EQ_CHAT_LINE_MAX = 255

export function clampChatLine(s: string): string {
  return s.length > EQ_CHAT_LINE_MAX ? s.slice(0, EQ_CHAT_LINE_MAX) : s
}

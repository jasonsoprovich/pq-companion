// EQ's chat input silently fails to paste once the clipboard text exceeds
// ~409 characters (documented: https://articles.eqresource.com/pasteineq.php,
// "There seems to be a 409 character limit"). Cap at 400 to stay safely
// under that boundary. Every "paste into EQ chat" clipboard feature in the
// app should build against this single constant — a prior, unverified 255
// figure lived in rollHelpers.ts with no citation and has been retired.
export const EQ_CHAT_LINE_MAX = 400

export function clampChatLine(s: string): string {
  return s.length > EQ_CHAT_LINE_MAX ? s.slice(0, EQ_CHAT_LINE_MAX) : s
}

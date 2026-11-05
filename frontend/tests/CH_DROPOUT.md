# Experimental sustained-dropout CH metronome

The existing panel and popout have an **Adapt to sustained dropouts (experimental)** checkbox. It defaults off; only localStorage `chMetronome:sustainedDropout` equal to the string `true` enables it. It is not a backend setting. Existing sound/TTS/overlay alert configuration and the popout-open/mute gates remain authoritative.

## Conservative policy

- Before adaptation, learn exactly the configured number of distinct call numbers over **two consecutive, coherent, ascending/wrapping rotations**. Adjacent calls must advance in order, with positive local time gaps no larger than `max(10 seconds, 2 × configured delay)`.
- Retain this ordinal roster, including absent coded numbers (`111/222/...`); do not rerank the remaining callers into their places.
- A slot is eligible only after **strictly more than three configured full cycles** without a genuine new call. A full cycle is `chainSize × max(configured delay, learned median spacing)`. The learned spacing comes from that coherent warmup; this version does not relearn tempo continuously while the roster remains established.
- Every surviving slot must have called in the last full cycle. Require at least two survivors. Never remove the user's own slot. Thus a two-person chain cannot compress to one.
- Follow the nearest surviving predecessor and project along the shortened ring. Display the skipped ordinal slots and shorter ring size. Any genuine returning call reinstates its slot immediately.
- Repeated snapshots, expired rows and `possible_miss` rows lingering at zero remaining time do not count as new calls. Evidence uses distinct increasing `starts_at` identities and local anchors derived once from remaining cast time, not the legacy map's refreshed snapshot timestamps. This tolerates a constant log/local clock offset, not arbitrary timestamp reordering.
- Target, main/secondary category and relevant config changes retire the evidence. Unknown numbering invalidates the learned roster. Idle gaps longer than `max(60 seconds, three full cycles)` retire learning. Existing high-water timestamps prevent frozen snapshots from reviving an idle chain.
- A queued prediction expires ten seconds after the last genuine call even if no more feed messages arrive. This is intentionally conservative for very slow or stopped feeds.

Learning is transient per mounted consumer and is **not restored from persisted preferences or the old learned-number map**. A newly mounted panel, popout, or alert hook requests bounded qualified evidence from a live sibling through the existing same-document/storage event owners. Replies are bound to a fresh request nonce and the exact target/category/config scope, require genuine evidence less than ten seconds old, and cannot replace a newer locally observed return. The imported genuine-call anchors retain their original local times; delivery does not refresh them. Request/reply storage values are removed immediately and are never read back as saved state. Unmount/config/target retirement invalidates pending replies. Thus a fresh consumer joins a live mature consumer's shortened rotation rather than independently displaying an uncompressed countdown. If every consumer has retired, or no fresh qualified sibling remains, a fresh owner must relearn; an old saved/replayed response cannot establish dropout. This uses no new polling loop, central service, or persisted warmup.

Config synchronization uses existing storage events between windows and a local event for the dashboard panel and app-level alert hook. Adaptive config/chain changes clear the old countdown and await a fresh feed. Cue identities are carried across dropout/rejoin corrections to avoid duplicate sounds; the first cue after an adaptive config/scope retirement is conservatively consumed, not replayed. Subsequent cycles use the effective ring size.

The pre-existing default-off fractional-delay parsing mismatch is not repaired: panel/audio use integer parsing and popout uses float parsing. Only adaptive mode makes panel/audio read fractional delay, a necessary prerequisite for agreement with the adaptive popout. Default-off timing math remains on the original path.

## Offline regression

From a repository with its existing Node dependencies installed:

```text
python scripts/run-ch-dropout-isolation.py --output D:/disposable/new-output --browser "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --node "C:/Program Files/nodejs/node.exe"
```

For baseline behavioral RED, add `--source <untouched-baseline-repo>`. The baseline receives the future optional boolean; no new export is required. The fixture bundles actual `chMetronome.ts`, both actual visual components, `useMetronomeAlerts`, WebSocket owner and API code. Only browser clock/transport boundaries are substituted: WebSocket, fetch, Electron IPC and the sound/TTS transport exports. The real hook's `fire` function executes. No backend, native app, game process or audible output runs. Headless Edge has a disposable profile, DNS denial and a `connect-src 'none'` CSP.

Outputs include actual rendered text, real alert transport calls, case results, source hashes, raw browser DOM/stderr, commands and exit codes. Inputs are rehashed after execution. Use a new output directory on each run; failed attempts are preserved. Tests are designed synthetic timelines, **not original raid logs**. Public issue timestamp pairs do not supply the missing full roster or full-night replay. Full Hoogz dropout/Larzek control replay and native/game field acceptance remain separate release gates.

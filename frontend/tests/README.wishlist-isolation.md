# Wishlist character isolation regression

This fixture mounts the shipping `WishlistPage`, character tabs, dialogs, API module and websocket hook in a headless Chromium-family browser. It replaces only `fetch` and `WebSocket` with deterministic synthetic transport. No backend, real database, game connection or native application is started. CSP denies connections and external resources; the runner uses a new browser profile and disables background networking/DNS resolution.

Prerequisites: the repository's existing `node_modules` (including esbuild), Node, Python standard library, and Chromium/Edge. No new dependencies or package commands are required.

From the repository root (replace browser and output paths for your machine):

```sh
python scripts/run-wishlist-isolation.py --browser /path/to/chromium --output /outside/repo/wishlist-green
```

Use `--node /path/to/node` if Node is not on PATH. Each output directory must be new and outside the repository. The runner emits build logs, rendered DOM, browser stderr, input hashes, structured assertion results and invocation receipts. Exit zero means all cases passed, the browser exited zero and bundled source inputs remained unchanged. Nonzero can mean a behavioral failure or harness failure; inspect `result.json` rather than counting every nonzero exit as an expected RED.

To compare an untouched baseline while retaining this fixture and the current repo's dependencies, export its `frontend/src` directory into a separate baseline directory, then run:

```sh
python scripts/run-wishlist-isolation.py --browser /path/to/chromium --output /outside/repo/wishlist-red --source /path/to/baseline-repo
```

The `--source` directory must contain `frontend/src/pages/WishlistPage.tsx` and its imported source closure. The same desired-behavior assertions run against baseline and candidate; baseline failures are not inverted into passes.

The eight cases cover late cross-character data/layout publication, stale errors/loading completion, A→B→A, latest-request ordering and current errors/recovery, retired mutation continuations, delete/search dialog isolation, ordinary add/remove/keep/view/websocket controls, and unmount/remount. There are 25 assertions. Tests drive rendered controls and assert actual serialized API requests; they never call copied production handlers or inject component state.

Limits: synthetic transport does not prove backend persistence, database behavior, native/Electron rendering, real game integration, pointer-drag geometry, or every modal's individual interaction flow. The character-keyed boundary encloses all page-owned modal state; delete/search dialogs exercise that boundary. Already-issued mutations are not canceled or rolled back by changing characters. This repair prevents retired callbacks from displaying in the next owner or initiating a retired reload; it does not introduce mutation ordering/conflict resolution.

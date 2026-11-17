---
title: "Custom Overlays"
slug: custom-overlays
section: "Live & combat"
order: 135
routes: []
keywords: ["custom overlay", "widget", "dkp", "bid", "regex", "rules", "share", "json", "guild"]
---
# Custom Overlays

Build your own overlay from log lines without writing any code. A custom
overlay is a stack of **widgets** fed by **rules**: each rule watches for a
line (optionally only in a channel such as guild or raid, or from one
speaker) and updates a widget when it matches.

### Widgets

- **Fields** — a status line of named values, with optional colour rules.
- **Table** — ranked rows keyed by a capture (a bidder, a player), sortable
  and clearable. Right-click a row to remove it.
- **Feed** — the most recent matching lines, formatted with a template such
  as `{1}: {2}`.
- **Counter** and **Countdown**.

Values persist across restarts. Rules can reset a widget, and the title bar
has a reset button.

### Making one

In the [Overlays hub](help:overlay-dashboard), open **Your windows → + New
custom overlay** and start blank or from the built-in **DKP bid tracker**
template. **Edit overlay** opens the editor with Widgets, Rules, Test and
JSON tabs and a live preview. Paste a log line into **Test** to see exactly
what each rule does before you save.

Rules only change what the overlay shows. For a sound or spoken alert on
the same line, add a normal [trigger](help:triggers).

### Sharing

**Copy to share** puts the overlay's JSON on your clipboard and **Save as
file** downloads it. Importing (from a file or pasted text) shows a preview
before anything is saved. Character assignments are never included.

Custom overlays behave like any other overlay: they pop out, stack, join
layouts, take a hotkey, and collapse when idle.

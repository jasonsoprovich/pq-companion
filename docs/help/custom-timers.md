---
title: "Custom Timers"
slug: custom-timers
section: "Live & combat"
order: 150
routes: []
keywords: []
---
# Custom Timers

Manual countdown timers for the things your combat log won't announce — a spawn window, a click-item cooldown, a raid phase. They sit alongside the rest of the dashboard and pop out as their own overlay windows.

### Start them by hand or from a trigger

Kick off a timer manually whenever you need one, or let a [trigger](help:triggers) start it for you. A trigger can even pull the duration from a regex capture group, so a single log line spins up a perfectly-timed countdown — useful when the line itself tells you how long something lasts.

### Multiple named windows & Timer Groups

Custom Timers has reached full parity with the rest of the overlay dashboard: instead of one shared overlay, you can run several named Custom Timer windows side by side — split raid phase timers from personal cooldowns, or give each alt its own window. A **Timer Groups** screen lets you organize which triggers feed which window, so a trigger's countdown always lands on the overlay you intended. Each window pops out independently and has its own working close (X) button on its title bar, so closing one window doesn't take the others down with it.

### Pinning a timer on top

Any individual trigger-driven timer can be **pinned** to keep it at the top of its window, above the rest of that overlay's bars — handy for the one countdown you never want to lose track of in a crowded window, like a tank cooldown or a raid-wide mechanic.

### Appearance & overdue reminders

Give a timer its own bar colour from the quick-add form, or set a global timer appearance that every bar inherits. A timer can also **keep showing after it expires** as an overdue reminder, instead of vanishing the moment it hits zero — useful for a spawn window you don't want to forget about once it opens.

### Repeating timers

Turn on Repeat and a trigger's timer restarts each time it runs out, until its worn-off line clears it — useful for raid waves on a fixed cadence.

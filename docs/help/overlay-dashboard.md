---
title: "Overlays hub"
slug: overlay-dashboard
section: "Getting started"
order: 30
routes: ["/overlays"]
keywords: ["overlay", "popout", "pop out", "layout", "stack", "arrange", "dashboard", "hotkey", "click-through", "lock", "opacity", "fade", "collapse"]
---
# Overlays hub

The **Overlays** page is where every overlay lives: buff and detrimental
timers, DPS and threat meters, NPC info, pet, CH chain, trigger alerts and
your own custom windows. Overlays pop out as transparent, always-on-top
windows above the game; the same panels can also sit together on a
**Dashboard** for a second monitor.

New here? Open **⋯ → Set up overlays…** for a short wizard that picks a
starting set for your class, places them on your game monitor, and lets you
fine-tune the arrangement. The same wizard runs automatically after first-
launch setup.

### Library, Arrange and Dashboard

- **Library** lists every overlay as a card, grouped into Timers, Combat,
  Targets & info, Alerts & social, and Your windows. Each card pops its
  overlay out or closes it, adds it to the Dashboard, and shows its hotkey.
  Click a card to open its **settings drawer** (the NPC overlay shows a live
  preview above its settings).
- **Arrange** is a miniature of your monitors. Drag a box to move the real
  window, click one to open, show or raise it, and drop one box onto
  another's title strip to stack them. Closed windows show dimmed at the
  spot they'll open.
- **Dashboard** is a snap-grid of panels inside the main window. Use
  **Panels ▾** to choose what shows and **Reset dashboard** to start over.

### Layouts

A **layout** is a saved set of overlays: which windows are open, where they
sit, their collapse state, and the dashboard arrangement. Switch layouts
from the chips at the top of the hub or from the caret on the sidebar's
overlay button. Changes save to the active layout automatically;
**+ New** copies the active layout under a new name to experiment with.

Link characters to a layout (the person icon on a chip) and PQ Companion
switches to it when that character logs in. Characters without a link stay
on whatever layout is active.

### Showing and hiding

- The **overlay button** in the sidebar (next to the character switcher)
  opens or closes every window in the active layout.
- The **show/hide-all hotkey** hides overlays without closing them, for a
  quick clear view. Trigger alert text is never hidden.
- Each overlay and each layout can have its own global hotkey. All hotkeys
  start unbound; set them under **Settings → Hotkeys** or use the suggested
  keys. Hotkeys never steal focus from EverQuest.

### Collapse, idle collapse and stacks

- The chevron in a popout's title bar folds it to a one-line summary.
- **Collapse when idle** (off by default, per overlay) folds a window after a
  few seconds with nothing to show — no timers running, no target, no fight
  — and opens it again when something happens. Turn it on for every
  overlay at once from the ⋯ menu.
- **Stacks** combine overlays into one tabbed window. Drag a popout's title
  bar onto another window (it highlights) and let go. Right-click a tab and
  choose **Unstack** to split it out again. A dot on a background tab means
  it has new activity. Stacks get their own card under Your windows, with
  lock, opacity, fade and idle settings.

### Lock behaviour per overlay

Each overlay's settings drawer sets how it behaves when locked.
**Interactive on hover** (the default) keeps a locked overlay click-through,
but hovering it temporarily allows clicks so you can scroll a timer list,
dismiss a row, or drag the header. **Click-through** leaves only the
title-bar buttons clickable, and **Display only** hides the title bar for a
pure HUD that never captures the mouse.

### Moving and recovering windows

**Move** in a card's drawer makes that overlay draggable regardless of its
lock mode — the way to move a Display-only overlay. **Reset position**
recenters a window on your primary monitor, the recovery path for one that
drifted off-screen. The ⋯ menu also has **Reset all window positions**,
**Close all popouts** and **Clear all timers**.

PQ Companion is multi-monitor aware: windows remember their monitor, size
and position, and the trigger alert overlay can span every monitor.

### Opacity, fade and size

**Overlays → Overlay settings** holds the defaults for every overlay:
opacity, fade when inactive, clickable links, restore on launch, alert text
style and monitor. Each overlay's drawer can override opacity and fade, so
overlays over the game can go see-through while ones on a second monitor
stay solid.

Hold **Ctrl** and scroll over any popout to resize its contents in 5%
steps; per-overlay zoom sliders (50–200%) are under **Settings →
Accessibility**.

### Clickable links

Items, spells and NPCs shown in overlays open in the database explorer when
clicked, controlled by **Clickable links** under Overlay settings.

### Tips

- Build a layout per role (raid, group, solo) and link characters so the
  right one opens on login.
- Stack rarely-watched overlays (Kills, Rolls, Death Recap) into one tabbed
  window to save screen space.
- If an overlay vanishes off-screen, use **Reset position** on its card.

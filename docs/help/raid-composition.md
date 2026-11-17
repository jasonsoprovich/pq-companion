---
title: "Raid Composition"
slug: raid-composition
section: "Raids"
order: 10
routes: ["/raids","/raids/check","/raids/editor"]
keywords: []
---
# Raid Composition

A user-editable role taxonomy, a normalized encounter knowledge base, and a live MIN/REC composition checker run against your current Zeal raid roster — or a manually-entered one if Zeal isn't connected. Contributed by Kav (Intervention), ported from his eqmon project.

### Composition checker

Pick an encounter and the checker maps your live roster against its minimum and recommended staffing per role, showing a GAP figure and class-eligible candidate names for every short row. Candidate names are class-eligible members, not actual assignments — the same person can be counted toward more than one role's need at once, so treat the report as a staffing guide rather than a final call. The encounter picker orders by proximity to your live zone, and the pick stays in sync across the check page, the dashboard panel, and the popout overlay.

### Role taxonomy & encounter knowledge base

The Raid Editor lets you add, edit, and delete roles and encounters yourself — the knowledge base ships with a single seeded encounter (Avatar of War), so most raids will want to build out their own. Encounters can be exported and imported as JSON packs to share between officers, auto-provisioning any taxonomy roles a pack references that don't exist locally yet.

### Raid Summary & Raid Readiness

A Raid Summary dashboard tab shows encounter-agnostic per-class coverage for the current roster — its Members list has a Groups view with one card per raid group, every column sorts, and a Snapshot button saves the roster for attendance history — and a Raid Readiness overlay (dashboard panel and popout window) surfaces missing MIN-role classes at a glance without leaving the game.

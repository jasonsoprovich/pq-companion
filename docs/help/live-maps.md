---
title: "Live Maps"
slug: live-maps
section: "Live & combat"
order: 90
routes: ["/live-map","/maps"]
keywords: []
---
# Live Maps

Rendered zone maps, built from the same client files the actual game reads — not a scanned-in image. Walls, floors, and depth are extracted directly from EQ's .s3d/WLD zone geometry and shown as depth-tinted contours, so the shape on screen genuinely matches the zone you're standing in.

Every map has a **2D / 3D** switch. 3D is the default, and your map toggles are kept across restarts.

### Where to find it

A standalone **Maps** page and a **Map** tab on every zone in the Database Explorer share the same panel, so you can browse a zone's layout without leaving the zone page, or jump straight to the dedicated Maps page for a bigger view.

### Display modes

**Outline** mode shows a clean floor/ceiling silhouette; **Detailed** mode adds a full height legend with depth-tinted geometry sliced by elevation, so multi-level zones read correctly instead of collapsing into one flat layer.

### Live player & group position

**Prerequisite:** [Zeal](https://github.com/iamclint/Zeal). With Zeal connected, your position renders live on the map as you move, with auto-depth so the visible layer tracks the floor you're actually standing on. Groupmates in your zone show as faint secondary arrows alongside yours; turn them off under Settings → Maps → “Show groupmates on the map” if you'd rather just see your own. Your arrow eases between Zeal's updates instead of stepping.

### Raid & target

With Zeal, the **Group** and **Raid** toggles show every raider in your zone, coloured by raid group or by class, with your group drawn on top. A dashed line runs to your target: to their arrow for a raider, or to a marker with the name for anything else, and an NPC target gets its aggro ring. **Fit group** frames you, your group and the raid at once, and **Heading up** turns the 2D map so the way you face is up, with a compass for north.

### 3D view

The 3D view is built from your own client files, so no game geometry ships with the app. The View menu offers:

- **Style** — Textured (the zone's own textures), Height, or Plain, with trees, buildings, furniture, animated objects like torches, and the zone's doors.
- **See-through** — a wall opacity slider and Hide ceilings, so enclosed zones don't look like a solid box.
- **Cut-away** — Top and Chase cameras clip the zone just above you, so stacked zones like Bastion of Thunder don't hide your floor, and a *Slice from top* slider clips above any height you pick (useful in Akheva Ruins).
- **Cameras** — Top looks straight down, Chase rides behind whoever is followed, and you can drag or free-fly anywhere.

### Aggro rings & respawn dots

**Aggro rings** draw each spawn point's aggro radius, con-coloured against your level using the most dangerous NPC the spawn can roll. An eye marks mobs that see invisible, and greens under level 18 that won't attack on sight get a dashed ring. Rings can be Off, Near (fading out past a distance you set), or All. **Respawn dots** colour every spawn point by its respawn timer: red on cooldown (with a countdown when zoomed in), amber up within two minutes, green popped, and grey when no kill has been seen. Spawns known to be dead lose their ring, and Hide killed removes them.

### Annotations & markers

Drop your own markers and notes directly on the map — they write back to the in-game `/map` file, so they show up on EverQuest's own map display too, not just in the app.

### NPC spawn points

Named and notable spawns render on the map with patrol routes for mobs that walk a path, and trap markers where the zone has them, pulled from the same database that backs the NPC Info Overlay.

### User-installed map packs

If you already have a Brewall-style map pack installed for EverQuest, PQ Companion can render it as a third display mode alongside Outline and Detailed — the app reads your existing pack in place, it isn't bundled with the app.

### Pop-out window

Like the other overlays, Live Maps pops out as its own window you can float over the game — handy for keeping an eye on a patrol route or your position mid-pull without tabbing back to the main window.

The overlay draws everything the Live Map page does: 2D or 3D, your group and the raid, aggro rings, spawn points and labels. A row of toggles shows while it's unlocked, and the same row is under Settings → Maps for when it's locked. Layers you turn on in the Live Map, like doors and zone lines, show on the overlay too.

### Map search

Map search groups each NPC or point of interest into one result. One with several spawn points shows a ×N count and lights up all of them, and "Show all N" highlights every match at once — every trap in a zone, for example.

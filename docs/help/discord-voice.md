---
title: "Discord Voice Overlay"
slug: discord-voice
section: "Live & combat"
order: 140
routes: []
keywords: []
---
# Discord Voice Overlay

**Prerequisite:** a Discord [StreamKit Overlay](https://streamkit.discord.com/overlay) URL for your voice channel. Off by default.

A pop-out overlay showing a live roster of who's in your Discord voice channel, with a speaking highlight, floating on top of the game. PQ Companion doesn't integrate with Discord's API directly — instead it embeds Discord's own hosted StreamKit Overlay page, the same tool people paste into OBS, inside a popout window with the usual overlay chrome (lock/close).

### Setup

Generate a StreamKit Overlay URL for your server's voice channel from Discord's own [StreamKit page](https://streamkit.discord.com/overlay), then paste it into the **Discord Voice** overlay's settings in the **Overlays** hub. The overlay goes live as soon as a URL is saved.

### Multiple links

Save more than one named link if you jump between voice channels or servers — a main raid Discord and a guild-only channel, say — and switch which one the overlay shows without re-pasting a URL each time.

### Transparency

The overlay's content-area transparency is a toggle in Settings, so the roster can sit cleanly over the game without a solid background box behind it.

### Where it lives

Discord Voice shows up in the Manage overlays menu and the dashboard like any other panel, and is included in Pop Out All / Close All Popouts.

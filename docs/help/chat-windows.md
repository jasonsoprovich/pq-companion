---
title: "Chat Windows"
slug: chat-windows
section: "Live & combat"
order: 136
routes: []
keywords: ["chat", "chat window", "tells", "guild chat", "raid chat", "filter", "spam", "tabs", "highlight", "pin", "overlay"]
---
# Chat Windows

Chat windows are overlays that show only the chat you care about, so guild
chat, raid callouts or tells don't get lost in combat spam. Each window reads
your log, keeps the channels you pick, and can highlight, pin or hide lines
with simple rules.

### Making one

In the [Overlays hub](help:overlay-dashboard), open **Your windows → + New
chat window** and pick a starting point:

- **Guild chat**: guild and officer chat, your name highlighted.
- **Raid callouts**: raid and group chat; lines with your name are pinned
  for 2 minutes.
- **Tells**: tells in and out.
- **Blank**: choose channels and rules yourself.

The window is created right away and its editor opens so you can adjust it.
Nothing needs saving until you change something; **Done** closes the editor.
Click the window's card in the hub any time to edit it again.

### Sources

Tick the chat channels the window shows (guild, raid, group, tells, say,
OOC, shout, auction and your named channels). **Every named channel**
follows any channel you join. **Other log lines** adds non-chat lines and is
off by default. **Characters** limits the window to some of your characters,
and **Scrollback** sets how many lines it keeps.

### Keep and hide rules

- **Keep rules** always show a matching line and can give it a highlight
  colour or pin it to a strip at the top for a few minutes. Keep beats hide.
- **Hide rules** drop matching lines (loot spam, a chatty player).

A rule matches **Contains**, **Starts with**, **Is exactly** or a **Regex**,
against the whole line or just the message, optionally only from certain
speakers. `*` is a wildcard in the simple modes. **Mentions me** is a ready
made keep rule for your name.

Right-click a line in the window for **Copy**, **Keep lines like this** or
**Hide lines like this**. The suggested rule swaps numbers and player names
for wildcards; adjust it and click **Save**.

### Layout

- **Combined** shows everything in one list.
- **Tabs** gives each channel its own tab with an unread count. Turn on
  **Tell tabs** to get a tab per player you're talking to (closable, up to a
  limit you set; the rest go in a **+N** menu).

On the **Appearance** tab: font size in pixels (Ctrl+scroll over the
window also changes it, 1 px at a time), opacity, times and channel tags, channel
colours, and the colour of your own lines (white by default, or the
channel's colour). The **Window** tab opens the window, pauses or clears
it, sets its lock hotkey, or deletes it.

### Locking and hotkeys

Chat windows lock and unlock like other overlays. Two hotkeys in
**Settings › Hotkeys** help: **lock all chat windows**, and **position
overlays** to move every window at once. Tabs stay clickable while a window
is locked.

### Sharing

Each chat window's card in the hub can **copy** the window (to paste in
Discord) or **save** it as a file, and the section can share every window at
once. Character filters are left out. Load one with **Import a shared
window** in the New chat window dialog.

### Tips

- A new window fills from your recent chat history, so a Tells window may
  open with conversation tabs already there.
- Log Feed replays also play into chat windows; unread counts ignore replayed
  lines.

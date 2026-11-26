---
title: "Inventory Tracker"
slug: inventory-tracker
section: "Character"
order: 50
routes: ["/inventory","/characters/inventory"]
keywords: []
---
# Inventory Tracker

**Prerequisite:** Zeal inventory export.

All inventory across every character in one searchable view, with item icons everywhere — equipped, bags, bank, shared bank. Paper-doll layout with collapsible bag and bank cards. Click any item to open its full database entry in a modal without leaving the page — including a Characters tab on that entry listing every character who holds the item.

### Cross-character search

Searching returns a flat result list spanning every character, so "who's holding that resist gear?" is a one-box question instead of a tab-by-tab hunt. By default the tracker is scoped to your imported characters; a toggle shows the rest.

### Hide empty bags

Empty bag and bank cards are hidden by default — handy on cluttered characters where you only care about the slots that hold something. The toggle persists, so flip it off once and it stays off.

### Bag value

Each bag card shows what a merchant would pay for everything inside it, so you can tell at a glance whether a bag of cash loot or trader stuff is worth selling. It uses the server's sell formula with your character's Charisma (from gear and AAs, without buffs), assuming a normal merchant you're indifferent with; hover the total to see the Amiable and Apprehensive payouts and the base value. NO DROP items are left out, since merchants won't buy them. Characters not on the Characters page show the base value instead.

### Rechargeable items

A dedicated Rechargeable Items section gathers the limited-charge clickies across your inventory into one place, and every limited-charge item shows its remaining charge count — so you can see at a glance which clickies are running low and need a recharge before the next raid.

---
title: "Trader Tracker"
slug: trader-tracker
section: "Progress & goals"
order: 10
routes: ["/trader-tracker"]
keywords: []
---
# Trader Tracker

**Prerequisite:** Zeal inventory exports while you're set up as a Bazaar trader.

Bazaar selling is a black box: you set up a trader, walk away, and come back to a pile of platinum with no idea what actually moved. Trader Tracker infers your sales by comparing your **Trader's Satchel** between inventory snapshots — anything that left the satchel between two exports is treated as a sale and reconciled against your trader price list, so you can see what sold and for how much.

### How it captures

A background poller picks up new inventory exports automatically, and you can also drop a manual `/output inventory` export in at any time. Each snapshot is diffed against the last, so the sales list builds up over a selling session without any manual bookkeeping.

### Why the diff, not the price file

Your Bazaar price file is used only as a price reference — it records what you're *asking*, not what changed hands. The actual sale signal is the satchel going down by a stack between two snapshots, which is why an inventory export (not the price file) is what drives the tracker.

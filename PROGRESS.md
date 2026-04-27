# PQ Companion — Progress

## Phase 0 — Database Setup & Exploration
- [x] Task 0.1 — Docker MySQL Setup
- [x] Task 0.2 — Database Exploration & Documentation
- [x] Task 0.3 — Go Project Init + MySQL→SQLite Converter
- [x] Task 0.4 — Go Database Layer

## Phase 1 — Go Backend API
- [x] Task 1.1 — REST API
- [x] Task 1.2 — WebSocket Server
- [x] Task 1.3 — Configuration System

## Phase 2 — Electron + React Frontend
- [x] Task 2.1 — Electron + React Project Setup
- [x] Task 2.2 — App Layout & Navigation
- [x] Task 2.3 — Database Explorer: Items
- [x] Task 2.4 — Database Explorer: Spells
- [x] Task 2.5 — Database Explorer: NPCs
- [x] Task 2.6 — Database Explorer: Zones
- [x] Task 2.7 — Global Search

## Phase 3 — Zeal Integration & Backup Manager
- [x] Task 3.1 — Zeal Export Reader
- [x] Task 3.2 — Spell Checklist UI
- [x] Task 3.3 — Inventory Tracker (Multi-Character + Search)
- [x] Task 3.4 — Key Tracker
- [x] Task 3.5 — Config Backup Manager (Backend)
- [x] Task 3.6 — Config Backup Manager (UI)

## Phase 4 — Log Parsing & NPC Overlay
- [x] Task 4.1 — Log File Tailer
- [x] Task 4.2 — Event Broadcasting via WebSocket
- [x] Task 4.3 — NPC Info Overlay (Backend)
- [x] Task 4.4 — NPC Info Overlay (Frontend)

## Phase 5 — Combat Tracking & DPS Meter
- [x] Task 5.1 — Combat Parser
- [x] Task 5.2 — DPS Overlay
- [x] Task 5.3 — Combat Log History

## Phase 6 — Windows Build & Distribution
- [x] Task 6.1 — Windows Build Pipeline
- [x] Task 6.2 — Auto-Updater

## Phase 7 — Spell Timer Engine
- [x] Task 7.1 — Spell Timer Engine (Backend)
- [x] Task 7.2 — Timer Overlay (Frontend)
- [x] Task 7.3 — Buff & Detrimental Windows

## Phase 8 — Custom Trigger System
- [x] Task 8.1 — Trigger System (Backend)
- [x] Task 8.2 — Trigger Manager UI
- [x] Task 8.3 — Trigger Overlay

## Phase 9 — Audio Alerts
- [x] Task 9.1 — Audio Engine
- [x] Task 9.2 — Timer Audio Alerts
- [x] Task 9.3 — Event Notifications

## Phase 10 — Character Tools
- [x] Task 10.1 — Multi-Character Support (Characters tab, CRUD, switcher fix)
- [x] Task 10.2 — Character tracking: race field, Discover from logs auto-import (closes #45)
- [x] Bug fix — Character switcher and Characters tab active selection sync (closes #83)
- [x] Feature — Advanced item filter modal: race, class, level, slot, type, stat/resist minimums (closes #94)
- [x] Bug/Feature — Move Alerts panel into Triggers tab; deduplicate voice alert sounds (closes #68)
- [x] Feature — Persist overlay window size and position across sessions (closes #85)
- [x] Feature — Trigger-driven buff/detrimental timers from Spells tab and overlay "Add Timer" button; Create Alert from NPC special abilities; GINA import and quick-share export; global spell timer on/off toggle (closes #88, #52, #51, #50)
- [x] Feature — Character progression: base stat tracking, equipped gear view, and AA tracking from quarmy.txt; split gear upgrade suggestions into #95 (closes #61)
- [x] Bug — Resolve AA names from altadv_vars in quarm.db; fix gear tab scroll clipping (closes #96)
- [x] Chore — Move Backup Manager from nav bar into Settings as a tab (closes #98)
- [x] Chore — Consolidate character-related nav items under a single Characters section with sub-tabs (closes #99)
- [x] Chore — Consolidate NPC, DPS, and Spell Timer overlays into a single Overlays nav section (closes #100)
- [x] Feature — Paper-doll inventory layout with collapsible bag/bank cards; fix bag separator parsing (closes #73)
- [x] Feature — First-launch onboarding wizard: welcome, EQ folder selection with validation, character pick from log discovery, Zeal info, confirmation; re-runnable from Settings (closes #46)
- [x] Chore — Custom PQ Companion app icon: build/icon.ico (multi-res Windows ICO 16/24/32/48/64/128/256), build/icon.png runtime, NSIS installer/uninstaller, and BrowserWindow taskbar icon (closes #6)
- [x] Feature — Per-character to-do list: Tasks tab with name/description, drag-to-reorder, expandable subtask checkboxes, and delete confirmation
- [x] Feature — Character Progress AA tab shows trained vs available AAs filtered by class; gear-slot and inventory rows open the item detail modal in-place instead of jumping to the Item Explorer
- [x] Bug — Resolve item click/proc/worn/focus effect names from spells_new when items.*name columns are blank (closes #107)

## Phase 11 — Project Website
- [ ] Task 11.1 — Project Website

## Future Plans
- Planes of Power Flag Tracker
- Hosted Web API

---
title: "Resist Calculator"
slug: resist-calc
section: "Database & tools"
order: 50
routes: ["/resist-calc"]
keywords: []
---
# Resist Calculator

Estimate a spell's chance to land on an NPC before you spend the cast and the mana. The calculator ports Project Quarm's actual resist check and enumerates the full roll distribution, so the number you get is a real land percentage from the server's own math — not a rule of thumb.

### What it accounts for

The estimate factors in the target's resist for the spell's type, your level relative to the target, the spell's own resist modifier, and the era's resist rules. Binary spells (mez, charm, fear) are classified from the spell's first effect rather than a single database flag, so lines that resolve all-or-nothing are treated correctly instead of being shown a partial-resist curve. Immunities and the level caps on charm, mez, and fear are respected — if a target simply can't be affected, the calculator says so and why.

### Picking a target

A searchable NPC picker pulls targets straight from the database, so you can check a specific named or trash mob by name. A resist-debuff section lists the tashes, malos, and other resist reducers with level-scaled magnitudes, so you can see how much a debuff shifts the odds before you commit a slot to it.

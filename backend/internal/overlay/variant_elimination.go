package overlay

import (
	"log/slog"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/loot"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// Kill elimination for same-name raid bosses.
//
// Vex Thal's Kaas Thox Xi Aten Ha Ra exists as two npc_types rows that are
// identical except for spawn point and loot table. A boss pulled away from its
// spawn point can't be told apart from its twin by anything Zeal reports (the
// target's own position is dropped past 250 units, and the player's position
// says nothing about where it spawned), so the overlay shows both loot
// tables. But the two are separate encounters that die one at a time: once one
// is dead, the same-named boss still standing must be the other row. A death
// is attributed to a row from the live target's resolved variant, from the
// first looted item after a logged kill that exists in exactly one
// candidate's loot table, or — since most raid deaths never reach the log as
// a "has been slain" line — from any looted item that only that one row can
// drop in the whole zone (a looted corpse means that row is dead).

const (
	// deadVariantTTL is how long a recorded death keeps its row out of the
	// candidate set. Raid bosses respawn in days, not hours, so this only needs
	// to outlast a raid night while still expiring if the app is left running
	// across a respawn.
	deadVariantTTL = 12 * time.Hour
	// pendingKillWindow is how long after an unattributed kill a loot line can
	// still be used to identify the dead variant, and how long zone-unique loot
	// evidence for a row is kept.
	pendingKillWindow = 20 * time.Minute
	// zoneLootEvidenceItems is how many different zone-unique items must be
	// looted for one row, with no kill line seen, before it counts as dead. One
	// isn't enough: re-looting your own corpse logs the same "has looted" line,
	// and a raider can die wearing last week's drop from that very boss. A boss
	// corpse usually yields several items unique to it; one player's corpse
	// rarely holds two from the same boss.
	zoneLootEvidenceItems = 2
)

// pendingKill is a same-name raid-boss kill not yet tied to a row. dbName keeps
// its original case: the variant queries are case-sensitive, unlike the
// lowercased eliminationKey.
type pendingKill struct {
	zoneShort, dbName string
	at                time.Time
}

// eliminationKey namespaces a name per zone, since the same boss name can be
// reused across zones with unrelated rows.
func eliminationKey(zoneShort, dbName string) string {
	return zoneShort + "|" + strings.ToLower(dbName)
}

// raidVariants returns the raid_target candidates for name in the zone, or nil
// when there are fewer than two (nothing to eliminate between). Results are
// memoised per zone+name — kills of ordinary trash mobs are frequent and each
// would otherwise cost a DB query. Caller must NOT hold t.mu.
func (t *NPCTracker) raidVariants(zoneShort, dbName string) []db.NPCVariant {
	key := eliminationKey(zoneShort, dbName)
	t.mu.RLock()
	cached, ok := t.raidVariantMemo[key]
	t.mu.RUnlock()
	if ok {
		return cached
	}
	var raid []db.NPCVariant
	if t.db != nil {
		for _, v := range t.fetchVariants(dbName, zoneShort) {
			if v.NPC.RaidTarget == 1 {
				raid = append(raid, v)
			}
		}
	}
	if len(raid) < 2 {
		raid = nil
	}
	t.mu.Lock()
	if t.raidVariantMemo == nil {
		t.raidVariantMemo = make(map[string][]db.NPCVariant)
	}
	t.raidVariantMemo[key] = raid
	t.mu.Unlock()
	return raid
}

// noteKill is called for every EventKill. It only acts when the slain mob has
// two or more same-name raid-boss rows in the current zone.
func (t *NPCTracker) noteKill(displayName string) {
	t.mu.RLock()
	zoneShort := t.pipeZoneShort
	t.mu.RUnlock()
	if zoneShort == "" || t.db == nil {
		return
	}
	dbName := strings.ReplaceAll(displayName, " ", "_")
	if t.raidVariants(zoneShort, dbName) == nil {
		return
	}

	// Attribute the kill to the live target's resolved row when we can: the
	// target is the dead mob (names match) and its spawn id already resolved to
	// exactly one row.
	t.mu.RLock()
	var resolved *db.NPC
	if t.st.HasTarget && t.lastPipeTargetID != nil {
		tgt, _ := stripCorpseSuffix(t.st.TargetName)
		if c, ok := t.variantCache[*t.lastPipeTargetID]; ok &&
			strings.EqualFold(tgt, displayName) && strings.EqualFold(c.name, displayName) &&
			len(c.variants) == 0 && c.npc != nil {
			resolved = c.npc
		}
	}
	t.mu.RUnlock()

	if resolved != nil {
		slog.Debug("overlay: variant killed (resolved target)",
			"name", displayName, "npc_id", resolved.ID)
		t.markVariantDead(zoneShort, dbName, resolved.ID)
		return
	}

	t.mu.Lock()
	if t.pendingKills == nil {
		t.pendingKills = make(map[string]pendingKill)
	}
	t.pendingKills[eliminationKey(zoneShort, dbName)] = pendingKill{zoneShort, dbName, time.Now()}
	t.mu.Unlock()
	slog.Debug("overlay: variant killed, awaiting loot to identify it", "name", displayName)
}

// HandleLootLine feeds a raw log line to the elimination logic. An item looted
// shortly after an unattributed same-name boss kill identifies which row died
// when only one candidate's loot table can drop it; an item only one same-name
// raid row in the zone can drop identifies a dead row with no kill line at all.
// Cheap for every non-loot line.
func (t *NPCTracker) HandleLootLine(ts time.Time, msg string) {
	if t.db == nil || !strings.HasPrefix(msg, "--") {
		return
	}
	// Ignore old lines (backfill / log replay) — they say nothing about a
	// corpse looted now.
	if time.Since(ts) > pendingKillWindow {
		return
	}
	p, ok := loot.ParseLoot(strings.TrimRight(msg, "\r\n"))
	if !ok {
		return
	}
	t.eliminateByZoneLoot(p.Item)

	t.mu.RLock()
	pending := len(t.pendingKills)
	t.mu.RUnlock()
	if pending == 0 {
		return
	}
	itemID, ok := t.db.GetItemIDByName(p.Item)
	if !ok {
		return
	}

	t.mu.RLock()
	keys := make(map[string]pendingKill, len(t.pendingKills))
	for k, pk := range t.pendingKills {
		if time.Since(pk.at) <= pendingKillWindow {
			keys[k] = pk
		}
	}
	t.mu.RUnlock()

	for key, pk := range keys {
		cands := t.raidVariants(pk.zoneShort, pk.dbName)
		var owners []int
		for _, c := range cands {
			if t.lootTableHasItem(c.NPC.ID, itemID) {
				owners = append(owners, c.NPC.ID)
			}
		}
		if len(owners) != 1 {
			continue // shared item (or not from either table): no evidence
		}
		slog.Debug("overlay: variant identified from loot",
			"key", key, "npc_id", owners[0], "item", p.Item, "item_id", itemID)
		t.mu.Lock()
		delete(t.pendingKills, key)
		t.mu.Unlock()
		t.markVariantDead(pk.zoneShort, pk.dbName, owners[0])
	}
}

// eliminateByZoneLoot marks a same-name raid row dead once enough different
// looted items that only it can drop in the current zone have been seen (see
// zoneLootEvidenceItems), or on the first such item after a logged kill of
// that name.
func (t *NPCTracker) eliminateByZoneLoot(item string) {
	t.mu.RLock()
	zoneShort := t.pipeZoneShort
	t.mu.RUnlock()
	if zoneShort == "" {
		return
	}
	droppers, err := t.db.ZoneNPCsDroppingItem(zoneShort, item)
	if err != nil {
		slog.Debug("overlay: zone loot lookup failed", "item", item, "err", err)
		return
	}
	if len(droppers) != 1 || droppers[0].RaidTarget != 1 {
		return
	}
	d := droppers[0]
	isTwin := false
	for _, v := range t.raidVariants(zoneShort, d.Name) {
		if v.NPC.ID == d.ID {
			isTwin = true
			break
		}
	}
	if !isTwin {
		return
	}
	key := eliminationKey(zoneShort, d.Name)
	now := time.Now()
	t.mu.Lock()
	_, killSeen := t.pendingKills[key]
	if t.lootEvidence == nil {
		t.lootEvidence = make(map[int]map[string]time.Time)
	}
	items := t.lootEvidence[d.ID]
	if items == nil {
		items = make(map[string]time.Time)
		t.lootEvidence[d.ID] = items
	}
	for it, at := range items {
		if now.Sub(at) > pendingKillWindow {
			delete(items, it)
		}
	}
	items[strings.ToLower(item)] = now
	enough := killSeen || len(items) >= zoneLootEvidenceItems
	if enough {
		delete(t.pendingKills, key)
		delete(t.lootEvidence, d.ID)
	}
	t.mu.Unlock()
	if !enough {
		slog.Debug("overlay: zone-unique loot for a same-name raid row, waiting for more",
			"name", d.Name, "npc_id", d.ID, "item", item)
		return
	}
	slog.Debug("overlay: variant identified from zone-unique loot",
		"name", d.Name, "npc_id", d.ID, "item", item, "kill_seen", killSeen)
	t.markVariantDead(zoneShort, d.Name, d.ID)
}

// lootTableHasItem reports whether npcID's resolved loot table can drop
// itemID. The per-NPC item set is cached.
func (t *NPCTracker) lootTableHasItem(npcID, itemID int) bool {
	t.mu.RLock()
	set, ok := t.lootItemMemo[npcID]
	t.mu.RUnlock()
	if !ok {
		set = make(map[int]struct{})
		if tbl, err := t.db.GetNPCLoot(npcID); err == nil && tbl != nil {
			for _, d := range tbl.Drops {
				for _, it := range d.Items {
					set[it.ItemID] = struct{}{}
				}
			}
		}
		t.mu.Lock()
		if t.lootItemMemo == nil {
			t.lootItemMemo = make(map[int]map[int]struct{})
		}
		t.lootItemMemo[npcID] = set
		t.mu.Unlock()
	}
	_, has := set[itemID]
	return has
}

// markVariantDead records npcID as dead for the zone+name and drops cached
// still-ambiguous resolutions of that name so a live target re-resolves
// without it on the next pipe tick.
func (t *NPCTracker) markVariantDead(zoneShort, dbName string, npcID int) {
	key := eliminationKey(zoneShort, dbName)
	displayName := strings.ReplaceAll(dbName, "_", " ")
	t.mu.Lock()
	if t.deadVariants == nil {
		t.deadVariants = make(map[string]map[int]time.Time)
	}
	if t.deadVariants[key] == nil {
		t.deadVariants[key] = make(map[int]time.Time)
	}
	t.deadVariants[key][npcID] = time.Now().Add(deadVariantTTL)
	for id, c := range t.variantCache {
		if len(c.variants) > 0 && strings.EqualFold(c.name, displayName) {
			delete(t.variantCache, id)
		}
	}
	t.mu.Unlock()
}

// dropDeadVariants removes raid-boss candidates recorded as dead. It never
// empties the set: if every candidate is dead (a respawn the app didn't see),
// the full set is returned so the overlay still shows the boss.
func (t *NPCTracker) dropDeadVariants(zoneShort, dbName string, cands []db.NPCVariant, desc *zealpipe.TargetDescriptors) []db.NPCVariant {
	// A corpse is by definition one of the dead rows; filtering would show the
	// survivor's loot on it.
	if len(cands) < 2 || zoneShort == "" || (desc != nil && desc.Type == zealpipe.TargetTypeNPCCorpse) {
		return cands
	}
	key := eliminationKey(zoneShort, dbName)
	t.mu.RLock()
	dead := t.deadVariants[key]
	t.mu.RUnlock()
	if len(dead) == 0 {
		return cands
	}
	now := time.Now()
	out := make([]db.NPCVariant, 0, len(cands))
	for _, c := range cands {
		if until, isDead := dead[c.NPC.ID]; isDead && c.NPC.RaidTarget == 1 && now.Before(until) {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 || len(out) == len(cands) {
		return cands
	}
	slog.Debug("overlay: dropped dead variants from candidates",
		"name", dbName, "before", len(cands), "after", len(out))
	return out
}

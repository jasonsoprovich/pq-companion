package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ─── Items ────────────────────────────────────────────────────────────────────

const itemColumns = `
  i.id, i.Name, i.lore, i.idfile, i.itemclass, i.itemtype,
  i.damage, i.delay, i.range, i.ac,
  i.banedmgamt, i.banedmgbody, i.banedmgrace,
  i.hp, i.mana, i.astr, i.asta, i.aagi, i.adex, i.awis, i.aint, i.acha,
  i.mr, i.cr, i.dr, i.fr, i.pr,
  i.magic, i.nodrop, i.norent,
  i.slots, i.classes, i.races, i.weight, i.size,
  i.reclevel, i.reqlevel,
  i.clickeffect, i.clickname, i.proceffect, i.procname,
  i.worneffect, i.wornname, i.focuseffect, i.focusname,
  i.bagsize, i.bagslots, i.bagtype,
  i.stackable, i.stacksize,
  i.price, i.icon, i.minstatus`

func scanItem(row interface {
	Scan(...any) error
}) (*Item, error) {
	var it Item
	err := row.Scan(
		&it.ID, &it.Name, &it.Lore, &it.IDFile, &it.ItemClass, &it.ItemType,
		&it.Damage, &it.Delay, &it.Range, &it.AC,
		&it.BaneAmt, &it.BaneBody, &it.BaneRace,
		&it.HP, &it.Mana, &it.Strength, &it.Stamina, &it.Agility, &it.Dexterity,
		&it.Wisdom, &it.Intelligence, &it.Charisma,
		&it.MagicResist, &it.ColdResist, &it.DiseaseResist, &it.FireResist, &it.PoisonResist,
		&it.Magic, &it.NoDrop, &it.NoRent,
		&it.Slots, &it.Classes, &it.Races, &it.Weight, &it.Size,
		&it.RecLevel, &it.ReqLevel,
		&it.ClickEffect, &it.ClickName, &it.ProcEffect, &it.ProcName,
		&it.WornEffect, &it.WornName, &it.FocusEffect, &it.FocusName,
		&it.BagSize, &it.BagSlots, &it.BagType,
		&it.Stackable, &it.StackSize,
		&it.Price, &it.Icon, &it.MinStatus,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// GetItem returns the item with the given ID, or sql.ErrNoRows if not found.
func (db *DB) GetItem(id int) (*Item, error) {
	q := fmt.Sprintf("SELECT %s FROM items i WHERE i.id = ?", itemColumns)
	row := db.QueryRow(q, id)
	it, err := scanItem(row)
	if err != nil {
		return nil, fmt.Errorf("get item %d: %w", id, err)
	}
	return it, nil
}

// SearchItems searches items by name (case-insensitive prefix/substring match).
// When baneBody > 0, only items with that bane damage body type are returned.
// Returns a page of results and the total count of matching rows.
func (db *DB) SearchItems(query string, baneBody, limit, offset int) (*SearchResult[Item], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	where := "Name LIKE ? ESCAPE '\\'"
	args := []any{pattern}
	if baneBody > 0 {
		where += " AND banedmgbody = ?"
		args = append(args, baneBody)
	}

	var total int
	if err := db.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM items WHERE %s", where),
		args...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count items: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM items i WHERE i.%s ORDER BY i.Name LIMIT ? OFFSET ?",
		itemColumns, where,
	)
	rows, err := db.Query(q, append(args, limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}
	defer rows.Close()

	items, err := collectItems(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[Item]{Items: items, Total: total}, nil
}

func collectItems(rows *sql.Rows) ([]Item, error) {
	var result []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		result = append(result, *it)
	}
	return result, rows.Err()
}

// GetItemSources returns all the ways to obtain the item with the given ID.
func (db *DB) GetItemSources(itemID int) (*ItemSources, error) {
	const zoneJoin = `
		LEFT JOIN spawnentry se ON se.npcid = n.id
		LEFT JOIN spawngroup sg ON sg.id = se.spawngroupid
		LEFT JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		LEFT JOIN zone z ON z.short_name = s2.zone`

	dropRows, err := db.Query(`
		SELECT n.id, n.name,
		       COALESCE(MIN(z.long_name), '') AS zone_name,
		       COALESCE(MIN(s2.zone), '') AS zone_short_name,
		       ROUND(MAX(CAST(lte.probability AS REAL) * lde.chance / 100.0), 2) AS drop_rate
		FROM npc_types n
		JOIN loottable_entries lte ON lte.loottable_id = n.loottable_id
		JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		`+zoneJoin+`
		WHERE lde.item_id = ? AND n.loottable_id > 0
		GROUP BY n.id
		ORDER BY drop_rate DESC, n.name
		LIMIT 100`, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item drop sources %d: %w", itemID, err)
	}
	defer dropRows.Close()
	drops, err := collectSourceNPCs(dropRows, true)
	if err != nil {
		return nil, err
	}

	merchantRows, err := db.Query(`
		SELECT n.id, n.name,
		       COALESCE(MIN(z.long_name), '') AS zone_name,
		       COALESCE(MIN(s2.zone), '') AS zone_short_name
		FROM npc_types n
		JOIN merchantlist ml ON ml.merchantid = n.merchant_id
		`+zoneJoin+`
		WHERE ml.item = ? AND n.merchant_id > 0
		GROUP BY n.id
		ORDER BY n.name
		LIMIT 100`, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item merchant sources %d: %w", itemID, err)
	}
	defer merchantRows.Close()
	merchants, err := collectSourceNPCs(merchantRows, false)
	if err != nil {
		return nil, err
	}

	forageRows, err := db.Query(`
		SELECT COALESCE(z.short_name, ''), COALESCE(z.long_name, ''), f.chance
		FROM forage f
		LEFT JOIN zone z ON z.zoneidnumber = f.zoneid
		WHERE f.Itemid = ?
		ORDER BY z.long_name`, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item forage zones %d: %w", itemID, err)
	}
	defer forageRows.Close()
	var forageZones []ItemForageZone
	for forageRows.Next() {
		var fz ItemForageZone
		if err := forageRows.Scan(&fz.ZoneShortName, &fz.ZoneName, &fz.Chance); err != nil {
			return nil, fmt.Errorf("scan forage zone: %w", err)
		}
		forageZones = append(forageZones, fz)
	}
	if err := forageRows.Err(); err != nil {
		return nil, err
	}

	gsRows, err := db.Query(`
		SELECT COALESCE(z.short_name, ''), COALESCE(z.long_name, ''), g.name, g.max_allowed, g.respawn_timer
		FROM ground_spawns g
		LEFT JOIN zone z ON z.zoneidnumber = g.zoneid
		WHERE g.item = ?
		ORDER BY z.long_name`, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item ground spawns %d: %w", itemID, err)
	}
	defer gsRows.Close()
	var groundSpawns []ItemGroundSpawnZone
	for gsRows.Next() {
		var gs ItemGroundSpawnZone
		if err := gsRows.Scan(&gs.ZoneShortName, &gs.ZoneName, &gs.Name, &gs.MaxAllowed, &gs.RespawnTimer); err != nil {
			return nil, fmt.Errorf("scan ground spawn zone: %w", err)
		}
		groundSpawns = append(groundSpawns, gs)
	}
	if err := gsRows.Err(); err != nil {
		return nil, err
	}

	tsRows, err := db.Query(`
		SELECT r.id, r.name, r.tradeskill, r.trivial,
		       CASE WHEN tre.successcount > 0 THEN 'product' ELSE 'ingredient' END AS role,
		       CASE WHEN tre.successcount > 0 THEN tre.successcount ELSE tre.componentcount END AS cnt
		FROM tradeskill_recipe_entries tre
		JOIN tradeskill_recipe r ON r.id = tre.recipe_id
		WHERE tre.item_id = ? AND tre.iscontainer = 0 AND (tre.successcount > 0 OR tre.componentcount > 0)
		  AND r.enabled = 1
		ORDER BY role DESC, r.name
		LIMIT 100`, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item tradeskills %d: %w", itemID, err)
	}
	defer tsRows.Close()
	var tradeskills []ItemTradeskillEntry
	for tsRows.Next() {
		var ts ItemTradeskillEntry
		if err := tsRows.Scan(&ts.RecipeID, &ts.RecipeName, &ts.Tradeskill, &ts.Trivial, &ts.Role, &ts.Count); err != nil {
			return nil, fmt.Errorf("scan tradeskill entry: %w", err)
		}
		tradeskills = append(tradeskills, ts)
	}
	if err := tsRows.Err(); err != nil {
		return nil, err
	}

	return &ItemSources{
		Drops:        drops,
		Merchants:    merchants,
		ForageZones:  forageZones,
		GroundSpawns: groundSpawns,
		Tradeskills:  tradeskills,
	}, nil
}

func collectSourceNPCs(rows *sql.Rows, withDropRate bool) ([]ItemSourceNPC, error) {
	var result []ItemSourceNPC
	for rows.Next() {
		var s ItemSourceNPC
		if withDropRate {
			if err := rows.Scan(&s.ID, &s.Name, &s.ZoneName, &s.ZoneShortName, &s.DropRate); err != nil {
				return nil, fmt.Errorf("scan source npc: %w", err)
			}
		} else {
			if err := rows.Scan(&s.ID, &s.Name, &s.ZoneName, &s.ZoneShortName); err != nil {
				return nil, fmt.Errorf("scan source npc: %w", err)
			}
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// ─── NPCs ─────────────────────────────────────────────────────────────────────

const npcColumns = `
  n.id, n.name, COALESCE(n.lastname, ''), n.level, n.race, COALESCE(r.name, 'Race ' || n.race), n.class, n.bodytype,
  n.hp, n.mana, n.mindmg, n.maxdmg, n.attack_count,
  n.MR, n.CR, n.DR, n.FR, n.PR, n.AC,
  n.STR, n.STA, n.DEX, n.AGI, n."_INT", n.WIS, n.CHA,
  n.aggroradius, n.runspeed, n.size,
  n.raid_target, COALESCE(n.rare_spawn, 0),
  n.loottable_id, n.merchant_id, n.npc_spells_id, n.npc_faction_id,
  COALESCE(n.special_abilities, ''),
  n.spellscale, n.healscale, n.exp_pct`

const npcJoin = `LEFT JOIN races r ON r.id = n.race`

func scanNPC(row interface {
	Scan(...any) error
}) (*NPC, error) {
	var n NPC
	err := row.Scan(
		&n.ID, &n.Name, &n.LastName, &n.Level, &n.Race, &n.RaceName, &n.Class, &n.BodyType,
		&n.HP, &n.Mana, &n.MinDmg, &n.MaxDmg, &n.AttackCount,
		&n.MR, &n.CR, &n.DR, &n.FR, &n.PR, &n.AC,
		&n.STR, &n.STA, &n.DEX, &n.AGI, &n.INT, &n.WIS, &n.CHA,
		&n.AggroRadius, &n.RunSpeed, &n.Size,
		&n.RaidTarget, &n.RareSpawn,
		&n.LootTableID, &n.MerchantID, &n.NPCSpellsID, &n.NPCFactionID,
		&n.SpecialAbilities,
		&n.SpellScale, &n.HealScale, &n.ExpPct,
	)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNPC returns the NPC with the given ID, or sql.ErrNoRows if not found.
func (db *DB) GetNPC(id int) (*NPC, error) {
	q := fmt.Sprintf("SELECT %s FROM npc_types n %s WHERE n.id = ?", npcColumns, npcJoin)
	row := db.QueryRow(q, id)
	n, err := scanNPC(row)
	if err != nil {
		return nil, fmt.Errorf("get npc %d: %w", id, err)
	}
	return n, nil
}

// GetNPCByName returns the first NPC whose name exactly matches the given
// string (case-insensitive). EQ log display names use spaces; the database
// stores them with underscores — callers must convert before calling.
// Returns sql.ErrNoRows (wrapped) when no match is found.
func (db *DB) GetNPCByName(name string) (*NPC, error) {
	q := fmt.Sprintf(
		"SELECT %s FROM npc_types n %s WHERE n.name = ? COLLATE NOCASE LIMIT 1",
		npcColumns, npcJoin,
	)
	row := db.QueryRow(q, name)
	n, err := scanNPC(row)
	if err != nil {
		return nil, fmt.Errorf("get npc by name %q: %w", name, err)
	}
	return n, nil
}

// SearchNPCs searches NPCs by name (case-insensitive substring match).
func (db *DB) SearchNPCs(query string, limit, offset int) (*SearchResult[NPC], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM npc_types WHERE name LIKE ? ESCAPE '\\'",
		pattern,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count npcs: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM npc_types n %s WHERE n.name LIKE ? ESCAPE '\\' ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns, npcJoin,
	)
	rows, err := db.Query(q, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search npcs: %w", err)
	}
	defer rows.Close()

	npcs, err := collectNPCs(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[NPC]{Items: npcs, Total: total}, nil
}

func collectNPCs(rows *sql.Rows) ([]NPC, error) {
	var result []NPC
	for rows.Next() {
		n, err := scanNPC(rows)
		if err != nil {
			return nil, fmt.Errorf("scan npc: %w", err)
		}
		result = append(result, *n)
	}
	return result, rows.Err()
}

// GetNPCFaction returns resolved faction info for the NPC with the given ID.
// Returns nil (no error) when the NPC has no faction or the faction record is missing.
func (db *DB) GetNPCFaction(npcID int) (*NPCFaction, error) {
	var npcFactionID int
	err := db.QueryRow("SELECT npc_faction_id FROM npc_types WHERE id = ?", npcID).Scan(&npcFactionID)
	if err != nil {
		return nil, fmt.Errorf("get npc faction id: %w", err)
	}
	if npcFactionID == 0 {
		return nil, nil
	}

	var result NPCFaction
	err = db.QueryRow(`
		SELECT nf.primaryfaction, COALESCE(fl.name, '')
		FROM npc_faction nf
		LEFT JOIN faction_list fl ON fl.id = nf.primaryfaction
		WHERE nf.id = ?`, npcFactionID,
	).Scan(&result.PrimaryFactionID, &result.PrimaryFactionName)
	if err != nil {
		return nil, fmt.Errorf("get npc faction info: %w", err)
	}

	rows, err := db.Query(`
		SELECT nfe.faction_id, fl.name, nfe.value
		FROM npc_faction_entries nfe
		JOIN faction_list fl ON fl.id = nfe.faction_id
		WHERE nfe.npc_faction_id = ?
		ORDER BY nfe.sort_order, nfe.faction_id`, npcFactionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get faction hits: %w", err)
	}
	defer rows.Close()

	result.Hits = []FactionHit{}
	for rows.Next() {
		var h FactionHit
		if err := rows.Scan(&h.FactionID, &h.FactionName, &h.Value); err != nil {
			return nil, fmt.Errorf("scan faction hit: %w", err)
		}
		result.Hits = append(result.Hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetNPCSpawns returns spawn point and spawn group data for the NPC with the given ID.
func (db *DB) GetNPCSpawns(npcID int) (*NPCSpawns, error) {
	// Spawn points: each row is one spawn2 entry where this NPC can appear.
	spawnRows, err := db.Query(`
		SELECT s2.id, s2.zone, COALESCE(z.long_name, s2.zone),
		       s2.x, s2.y, s2.z, s2.respawntime, s2.boot_respawntime
		FROM spawnentry se
		JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID
		LEFT JOIN zone z ON z.short_name = s2.zone
		WHERE se.npcID = ?
		ORDER BY z.long_name, s2.id
		LIMIT 200`, npcID)
	if err != nil {
		return nil, fmt.Errorf("get npc spawn points %d: %w", npcID, err)
	}
	defer spawnRows.Close()

	var points []NPCSpawnPoint
	for spawnRows.Next() {
		var p NPCSpawnPoint
		if err := spawnRows.Scan(&p.ID, &p.Zone, &p.ZoneName, &p.X, &p.Y, &p.Z, &p.RespawnTime, &p.FastRespawnTime); err != nil {
			return nil, fmt.Errorf("scan spawn point: %w", err)
		}
		points = append(points, p)
	}
	if err := spawnRows.Err(); err != nil {
		return nil, err
	}

	// Spawn groups with members: join to get all NPCs in every group this NPC belongs to.
	groupRows, err := db.Query(`
		SELECT sg.id, sg.name,
		       COALESCE((SELECT s2.respawntime FROM spawn2 s2 WHERE s2.spawngroupID = sg.id LIMIT 1), 0),
		       COALESCE((SELECT s2.boot_respawntime FROM spawn2 s2 WHERE s2.spawngroupID = sg.id LIMIT 1), 0),
		       se2.npcID, COALESCE(n2.name, ''), se2.chance
		FROM spawnentry se
		JOIN spawngroup sg ON sg.id = se.spawngroupID
		JOIN spawnentry se2 ON se2.spawngroupID = sg.id
		JOIN npc_types n2 ON n2.id = se2.npcID
		WHERE se.npcID = ?
		ORDER BY sg.id, se2.chance DESC
		LIMIT 500`, npcID)
	if err != nil {
		return nil, fmt.Errorf("get npc spawn groups %d: %w", npcID, err)
	}
	defer groupRows.Close()

	groupMap := make(map[int]*NPCSpawnGroup)
	var groupOrder []int
	for groupRows.Next() {
		var (
			gID, respawn, fastRespawn int
			gName                     string
			memberID, chance          int
			memberName                string
		)
		if err := groupRows.Scan(&gID, &gName, &respawn, &fastRespawn, &memberID, &memberName, &chance); err != nil {
			return nil, fmt.Errorf("scan spawn group row: %w", err)
		}
		g, exists := groupMap[gID]
		if !exists {
			g = &NPCSpawnGroup{
				ID:              gID,
				Name:            gName,
				RespawnTime:     respawn,
				FastRespawnTime: fastRespawn,
			}
			groupMap[gID] = g
			groupOrder = append(groupOrder, gID)
		}
		g.Members = append(g.Members, SpawnGroupMember{NPCID: memberID, Name: memberName, Chance: chance})
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}

	groups := make([]NPCSpawnGroup, 0, len(groupOrder))
	for _, gID := range groupOrder {
		groups = append(groups, *groupMap[gID])
	}

	return &NPCSpawns{
		SpawnPoints: points,
		SpawnGroups: groups,
	}, nil
}

// GetNPCLoot returns the resolved loot table for the NPC with the given ID.
// Returns nil when the NPC has no loottable_id set.
func (db *DB) GetNPCLoot(npcID int) (*NPCLootTable, error) {
	// Resolve the loottable_id for this NPC.
	var ltID int
	var ltName string
	err := db.QueryRow(`
		SELECT lt.id, lt.name
		FROM npc_types n
		JOIN loottable lt ON lt.id = n.loottable_id
		WHERE n.id = ? AND n.loottable_id > 0`, npcID).Scan(&ltID, &ltName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get npc loottable %d: %w", npcID, err)
	}

	rows, err := db.Query(`
		SELECT lte.lootdrop_id, ld.name, lte.multiplier, lte.probability,
		       lde.item_id, i.Name, lde.chance, lde.multiplier
		FROM loottable_entries lte
		JOIN lootdrop ld ON ld.id = lte.lootdrop_id
		JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		JOIN items i ON i.id = lde.item_id
		WHERE lte.loottable_id = ?
		ORDER BY lte.lootdrop_id, lde.chance DESC
		LIMIT 500`, ltID)
	if err != nil {
		return nil, fmt.Errorf("get npc loot entries %d: %w", npcID, err)
	}
	defer rows.Close()

	dropMap := make(map[int]*LootDrop)
	var dropOrder []int
	for rows.Next() {
		var (
			dropID, lteMultiplier, lteProbability int
			dropName                               string
			itemID, ldeMultiplier                 int
			itemName                               string
			chance                                 float64
		)
		if err := rows.Scan(&dropID, &dropName, &lteMultiplier, &lteProbability, &itemID, &itemName, &chance, &ldeMultiplier); err != nil {
			return nil, fmt.Errorf("scan loot row: %w", err)
		}
		d, exists := dropMap[dropID]
		if !exists {
			d = &LootDrop{
				ID:          dropID,
				Name:        dropName,
				Multiplier:  lteMultiplier,
				Probability: lteProbability,
			}
			dropMap[dropID] = d
			dropOrder = append(dropOrder, dropID)
		}
		d.Items = append(d.Items, LootDropItem{
			ItemID:     itemID,
			ItemName:   itemName,
			Chance:     chance,
			Multiplier: ldeMultiplier,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	drops := make([]LootDrop, 0, len(dropOrder))
	for _, id := range dropOrder {
		drops = append(drops, *dropMap[id])
	}
	return &NPCLootTable{ID: ltID, Name: ltName, Drops: drops}, nil
}

// ─── Spells ───────────────────────────────────────────────────────────────────

const spellColumns = `
  s.id, s.name,
  COALESCE(s.you_cast,''), COALESCE(s.other_casts,''),
  COALESCE(s.cast_on_you,''), COALESCE(s.cast_on_other,''),
  COALESCE(s.spell_fades,''),
  s.cast_time, s.recovery_time, s.recast_time,
  s.buffduration, s.buffdurationformula,
  s.mana, s.range, s.aoerange, s.targettype, s.resisttype, s.skill,
  s.effectid1,  s.effectid2,  s.effectid3,  s.effectid4,
  s.effectid5,  s.effectid6,  s.effectid7,  s.effectid8,
  s.effectid9,  s.effectid10, s.effectid11, s.effectid12,
  s.effect_base_value1,  s.effect_base_value2,  s.effect_base_value3,
  s.effect_base_value4,  s.effect_base_value5,  s.effect_base_value6,
  s.effect_base_value7,  s.effect_base_value8,  s.effect_base_value9,
  s.effect_base_value10, s.effect_base_value11, s.effect_base_value12,
  s.effect_limit_value1,  s.effect_limit_value2,  s.effect_limit_value3,
  s.effect_limit_value4,  s.effect_limit_value5,  s.effect_limit_value6,
  s.effect_limit_value7,  s.effect_limit_value8,  s.effect_limit_value9,
  s.effect_limit_value10, s.effect_limit_value11, s.effect_limit_value12,
  s.max1,  s.max2,  s.max3,  s.max4,  s.max5,  s.max6,
  s.max7,  s.max8,  s.max9,  s.max10, s.max11, s.max12,
  s.classes1,  s.classes2,  s.classes3,  s.classes4,  s.classes5,
  s.classes6,  s.classes7,  s.classes8,  s.classes9,  s.classes10,
  s.classes11, s.classes12, s.classes13, s.classes14, s.classes15,
  s.icon, s.new_icon, s.IsDiscipline, s.suspendable, s.nodispell,
  COALESCE(s.zonetype, 0)`

func scanSpell(row interface {
	Scan(...any) error
}) (*Spell, error) {
	var sp Spell
	err := row.Scan(
		&sp.ID, &sp.Name,
		&sp.YouCast, &sp.OtherCasts, &sp.CastOnYou, &sp.CastOnOther, &sp.SpellFades,
		&sp.CastTime, &sp.RecoveryTime, &sp.RecastTime,
		&sp.BuffDuration, &sp.BuffDurationFormula,
		&sp.Mana, &sp.Range, &sp.AoERange, &sp.TargetType, &sp.ResistType, &sp.Skill,
		&sp.EffectIDs[0], &sp.EffectIDs[1], &sp.EffectIDs[2], &sp.EffectIDs[3],
		&sp.EffectIDs[4], &sp.EffectIDs[5], &sp.EffectIDs[6], &sp.EffectIDs[7],
		&sp.EffectIDs[8], &sp.EffectIDs[9], &sp.EffectIDs[10], &sp.EffectIDs[11],
		&sp.EffectBaseValues[0], &sp.EffectBaseValues[1], &sp.EffectBaseValues[2],
		&sp.EffectBaseValues[3], &sp.EffectBaseValues[4], &sp.EffectBaseValues[5],
		&sp.EffectBaseValues[6], &sp.EffectBaseValues[7], &sp.EffectBaseValues[8],
		&sp.EffectBaseValues[9], &sp.EffectBaseValues[10], &sp.EffectBaseValues[11],
		&sp.EffectLimitValues[0], &sp.EffectLimitValues[1], &sp.EffectLimitValues[2],
		&sp.EffectLimitValues[3], &sp.EffectLimitValues[4], &sp.EffectLimitValues[5],
		&sp.EffectLimitValues[6], &sp.EffectLimitValues[7], &sp.EffectLimitValues[8],
		&sp.EffectLimitValues[9], &sp.EffectLimitValues[10], &sp.EffectLimitValues[11],
		&sp.EffectMaxValues[0], &sp.EffectMaxValues[1], &sp.EffectMaxValues[2],
		&sp.EffectMaxValues[3], &sp.EffectMaxValues[4], &sp.EffectMaxValues[5],
		&sp.EffectMaxValues[6], &sp.EffectMaxValues[7], &sp.EffectMaxValues[8],
		&sp.EffectMaxValues[9], &sp.EffectMaxValues[10], &sp.EffectMaxValues[11],
		&sp.ClassLevels[0], &sp.ClassLevels[1], &sp.ClassLevels[2],
		&sp.ClassLevels[3], &sp.ClassLevels[4], &sp.ClassLevels[5],
		&sp.ClassLevels[6], &sp.ClassLevels[7], &sp.ClassLevels[8],
		&sp.ClassLevels[9], &sp.ClassLevels[10], &sp.ClassLevels[11],
		&sp.ClassLevels[12], &sp.ClassLevels[13], &sp.ClassLevels[14],
		&sp.Icon, &sp.NewIcon, &sp.IsDiscipline, &sp.Suspendable, &sp.NoDispell,
		&sp.ZoneType,
	)
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// GetSpell returns the spell with the given ID, or sql.ErrNoRows if not found.
func (db *DB) GetSpell(id int) (*Spell, error) {
	q := fmt.Sprintf("SELECT %s FROM spells_new s WHERE s.id = ?", spellColumns)
	row := db.QueryRow(q, id)
	sp, err := scanSpell(row)
	if err != nil {
		return nil, fmt.Errorf("get spell %d: %w", id, err)
	}
	return sp, nil
}

// GetSpellByExactName returns the first spell whose name matches exactly
// (case-insensitive). Returns nil, nil when no match is found.
func (db *DB) GetSpellByExactName(name string) (*Spell, error) {
	q := fmt.Sprintf("SELECT %s FROM spells_new s WHERE LOWER(s.name) = LOWER(?) AND s.name != '' LIMIT 1", spellColumns)
	row := db.QueryRow(q, name)
	sp, err := scanSpell(row)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("get spell by name %q: %w", name, err)
	}
	return sp, nil
}

// SearchSpells searches spells by name with optional class and level filters.
// classIndex: -1 = all classes, 0–14 = filter to that class.
// minLevel/maxLevel: 0 = no bound; only applied when classIndex >= 0.
func (db *DB) SearchSpells(query string, classIndex, minLevel, maxLevel, limit, offset int) (*SearchResult[Spell], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	conditions := []string{"s.name LIKE ? ESCAPE '\\'", "s.name != ''"}
	args := []any{pattern}

	var classCol string
	if classIndex >= 0 && classIndex <= 14 {
		classCol = fmt.Sprintf("s.classes%d", classIndex+1)
		conditions = append(conditions, classCol+" < 255")
		if minLevel > 0 {
			conditions = append(conditions, classCol+" >= ?")
			args = append(args, minLevel)
		}
		if maxLevel > 0 {
			conditions = append(conditions, classCol+" <= ?")
			args = append(args, maxLevel)
		}
	}

	where := strings.Join(conditions, " AND ")
	orderBy := "s.name"
	if classCol != "" {
		orderBy = classCol + ", s.name"
	}

	var total int
	countArgs := append([]any{}, args...)
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM spells_new s WHERE "+where,
		countArgs...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count spells: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM spells_new s WHERE %s ORDER BY %s LIMIT ? OFFSET ?",
		spellColumns, where, orderBy,
	)
	queryArgs := append(args, limit, offset)
	rows, err := db.Query(q, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("search spells: %w", err)
	}
	defer rows.Close()

	spells, err := collectSpells(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[Spell]{Items: spells, Total: total}, nil
}

func collectSpells(rows *sql.Rows) ([]Spell, error) {
	var result []Spell
	for rows.Next() {
		sp, err := scanSpell(rows)
		if err != nil {
			return nil, fmt.Errorf("scan spell: %w", err)
		}
		result = append(result, *sp)
	}
	return result, rows.Err()
}

// GetSpellsByClass returns all spells castable by the given class index (0-based:
// 0=Warrior, 1=Cleric, ..., 14=Beastlord), ordered by that class's required level
// then by spell ID. Empty-name spells are excluded.
func (db *DB) GetSpellsByClass(classIndex, limit, offset int) (*SearchResult[Spell], error) {
	if classIndex < 0 || classIndex > 14 {
		return nil, fmt.Errorf("class index %d out of range [0,14]", classIndex)
	}
	col := fmt.Sprintf("s.classes%d", classIndex+1)
	whereClause := fmt.Sprintf("%s < 255 AND s.name != ''", col)

	var total int
	if err := db.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM spells_new s WHERE %s", whereClause),
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count spells by class: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM spells_new s WHERE %s ORDER BY %s, s.id LIMIT ? OFFSET ?",
		spellColumns, whereClause, col,
	)
	rows, err := db.Query(q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query spells by class: %w", err)
	}
	defer rows.Close()

	spells, err := collectSpells(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[Spell]{Items: spells, Total: total}, nil
}

// GetSpellCrossRefs returns items that reference the given spell ID, split into
// scroll items (which teach the spell) and effect items (click/worn/proc/focus).
func (db *DB) GetSpellCrossRefs(spellID int) (*SpellCrossRefs, error) {
	scrollRows, err := db.Query(
		"SELECT id, name FROM items WHERE scrolleffect = ? ORDER BY name",
		spellID,
	)
	if err != nil {
		return nil, fmt.Errorf("get spell scroll items: %w", err)
	}
	defer scrollRows.Close()

	result := &SpellCrossRefs{
		ScrollItems: []SpellItemRef{},
		EffectItems: []SpellItemRef{},
	}
	for scrollRows.Next() {
		var ref SpellItemRef
		if err := scrollRows.Scan(&ref.ID, &ref.Name); err != nil {
			return nil, fmt.Errorf("scan scroll item: %w", err)
		}
		result.ScrollItems = append(result.ScrollItems, ref)
	}
	if err := scrollRows.Err(); err != nil {
		return nil, err
	}

	effectRows, err := db.Query(`
		SELECT effect_type, id, name FROM (
			SELECT 'click' AS effect_type, id, name FROM items WHERE clickeffect = ?
			UNION
			SELECT 'worn', id, name FROM items WHERE worneffect = ?
			UNION
			SELECT 'proc', id, name FROM items WHERE proceffect = ?
			UNION
			SELECT 'focus', id, name FROM items WHERE focuseffect = ?
		) ORDER BY effect_type, name`,
		spellID, spellID, spellID, spellID,
	)
	if err != nil {
		return nil, fmt.Errorf("get spell effect items: %w", err)
	}
	defer effectRows.Close()

	for effectRows.Next() {
		var ref SpellItemRef
		if err := effectRows.Scan(&ref.EffectType, &ref.ID, &ref.Name); err != nil {
			return nil, fmt.Errorf("scan effect item: %w", err)
		}
		result.EffectItems = append(result.EffectItems, ref)
	}
	if err := effectRows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ─── Zones ────────────────────────────────────────────────────────────────────

const zoneColumns = `
  z.id, COALESCE(z.short_name,''), z.long_name, COALESCE(z.file_name,''),
  z.zoneidnumber, z.safe_x, z.safe_y, z.safe_z,
  z.min_level, COALESCE(z.note,''),
  z.castoutdoor, z.hotzone, z.canlevitate, z.canbind,
  COALESCE(z.zone_exp_multiplier, 1.0), z.expansion,
  COALESCE((SELECT MIN(n.level) FROM npc_types n JOIN spawnentry se ON se.npcID = n.id JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID WHERE s2.zone = z.short_name), 0),
  COALESCE((SELECT MAX(n.level) FROM npc_types n JOIN spawnentry se ON se.npcID = n.id JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID WHERE s2.zone = z.short_name), 0)`

func scanZone(row interface {
	Scan(...any) error
}) (*Zone, error) {
	var z Zone
	err := row.Scan(
		&z.ID, &z.ShortName, &z.LongName, &z.FileName,
		&z.ZoneIDNumber, &z.SafeX, &z.SafeY, &z.SafeZ,
		&z.MinLevel, &z.Note,
		&z.Outdoor, &z.Hotzone, &z.CanLevitate, &z.CanBind,
		&z.ExpMod, &z.Expansion,
		&z.NPCLevelMin, &z.NPCLevelMax,
	)
	if err != nil {
		return nil, err
	}
	return &z, nil
}

// GetNPCsByZone returns all distinct NPCs that spawn in the given zone short_name.
// It follows both the spawnentry chain (spawn2→spawnentry→npc_types) and direct
// solo-spawn entries (spawn2.spawngroupID == npc_types.id).
func (db *DB) GetNPCsByZone(shortName string, limit, offset int) (*SearchResult[NPC], error) {
	// Subquery returns the set of npc_types.id values present in the zone.
	idSubquery := `
		SELECT DISTINCT se.npcID
		FROM spawnentry se
		JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID
		WHERE s2.zone = ?
		UNION
		SELECT DISTINCT s2.spawngroupID
		FROM spawn2 s2
		WHERE s2.zone = ?
		  AND EXISTS (SELECT 1 FROM npc_types n2 WHERE n2.id = s2.spawngroupID)`

	var total int
	countQ := fmt.Sprintf(
		"SELECT COUNT(*) FROM (SELECT DISTINCT id FROM npc_types WHERE id IN (%s))",
		idSubquery,
	)
	if err := db.QueryRow(countQ, shortName, shortName).Scan(&total); err != nil {
		return nil, fmt.Errorf("count zone npcs: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM npc_types n %s WHERE n.id IN (%s) ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns, npcJoin, idSubquery,
	)
	rows, err := db.Query(q, shortName, shortName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query zone npcs: %w", err)
	}
	defer rows.Close()

	npcs, err := collectNPCs(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[NPC]{Items: npcs, Total: total}, nil
}

// GetZone returns the zone with the given ID, or sql.ErrNoRows if not found.
func (db *DB) GetZone(id int) (*Zone, error) {
	q := fmt.Sprintf("SELECT %s FROM zone z WHERE z.id = ?", zoneColumns)
	row := db.QueryRow(q, id)
	z, err := scanZone(row)
	if err != nil {
		return nil, fmt.Errorf("get zone %d: %w", id, err)
	}
	return z, nil
}

// GetZoneByShortName returns the zone matching the given short_name.
func (db *DB) GetZoneByShortName(shortName string) (*Zone, error) {
	q := fmt.Sprintf("SELECT %s FROM zone z WHERE z.short_name = ?", zoneColumns)
	row := db.QueryRow(q, shortName)
	z, err := scanZone(row)
	if err != nil {
		return nil, fmt.Errorf("get zone %q: %w", shortName, err)
	}
	return z, nil
}

// SearchZones searches zones by long_name (case-insensitive substring match).
func (db *DB) SearchZones(query string, limit, offset int) (*SearchResult[Zone], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM zone WHERE long_name LIKE ? ESCAPE '\\'",
		pattern,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count zones: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM zone z WHERE z.long_name LIKE ? ESCAPE '\\' ORDER BY z.long_name LIMIT ? OFFSET ?",
		zoneColumns,
	)
	rows, err := db.Query(q, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search zones: %w", err)
	}
	defer rows.Close()

	zones, err := collectZones(rows)
	if err != nil {
		return nil, err
	}
	return &SearchResult[Zone]{Items: zones, Total: total}, nil
}

func collectZones(rows *sql.Rows) ([]Zone, error) {
	var result []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		result = append(result, *z)
	}
	return result, rows.Err()
}

// GetZoneConnections returns all zones reachable via zone lines from the given short_name.
func (db *DB) GetZoneConnections(shortName string) ([]ZoneConnection, error) {
	rows, err := db.Query(`
		SELECT DISTINCT z.id, z.short_name, z.long_name, z.expansion
		FROM zone_points zp
		JOIN zone z ON z.zoneidnumber = zp.target_zone_id
		WHERE zp.zone = ?
		ORDER BY z.long_name`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone connections %q: %w", shortName, err)
	}
	defer rows.Close()

	var result []ZoneConnection
	for rows.Next() {
		var c ZoneConnection
		if err := rows.Scan(&c.ZoneID, &c.ShortName, &c.LongName, &c.Expansion); err != nil {
			return nil, fmt.Errorf("scan zone connection: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetZoneGroundSpawns returns items that spawn on the ground in the given zone.
func (db *DB) GetZoneGroundSpawns(shortName string) ([]ZoneGroundSpawn, error) {
	rows, err := db.Query(`
		SELECT g.id, g.item, COALESCE(i.Name, ''), g.name, g.max_allowed, g.respawn_timer
		FROM ground_spawns g
		LEFT JOIN items i ON i.id = g.item
		WHERE g.zoneid = (SELECT zoneidnumber FROM zone WHERE short_name = ? LIMIT 1)
		ORDER BY i.Name`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone ground spawns %q: %w", shortName, err)
	}
	defer rows.Close()

	var result []ZoneGroundSpawn
	for rows.Next() {
		var g ZoneGroundSpawn
		if err := rows.Scan(&g.ID, &g.ItemID, &g.ItemName, &g.Name, &g.MaxAllowed, &g.RespawnTimer); err != nil {
			return nil, fmt.Errorf("scan ground spawn: %w", err)
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

// GetZoneForage returns items obtainable via Forage in the given zone.
func (db *DB) GetZoneForage(shortName string) ([]ZoneForageItem, error) {
	rows, err := db.Query(`
		SELECT f.id, f.Itemid, COALESCE(i.Name, ''), f.chance, f.level
		FROM forage f
		LEFT JOIN items i ON i.id = f.Itemid
		WHERE f.zoneid = (SELECT zoneidnumber FROM zone WHERE short_name = ? LIMIT 1)
		ORDER BY i.Name`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone forage %q: %w", shortName, err)
	}
	defer rows.Close()

	var result []ZoneForageItem
	for rows.Next() {
		var f ZoneForageItem
		if err := rows.Scan(&f.ID, &f.ItemID, &f.ItemName, &f.Chance, &f.Level); err != nil {
			return nil, fmt.Errorf("scan forage item: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// GetZoneDrops returns items dropped by NPCs in the given zone (capped at 500).
func (db *DB) GetZoneDrops(shortName string) ([]ZoneDropItem, error) {
	rows, err := db.Query(`
		SELECT DISTINCT lde.item_id, i.Name, n.id, n.name, lde.chance
		FROM npc_types n
		JOIN loottable_entries lte ON lte.loottable_id = n.loottable_id
		JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		JOIN items i ON i.id = lde.item_id
		WHERE n.id IN (
			SELECT DISTINCT se.npcID
			FROM spawnentry se
			JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID
			WHERE s2.zone = ?
		) AND n.loottable_id > 0
		ORDER BY i.Name, n.name
		LIMIT 500`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone drops %q: %w", shortName, err)
	}
	defer rows.Close()

	var result []ZoneDropItem
	for rows.Next() {
		var d ZoneDropItem
		if err := rows.Scan(&d.ItemID, &d.ItemName, &d.NPCID, &d.NPCName, &d.Chance); err != nil {
			return nil, fmt.Errorf("scan zone drop: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

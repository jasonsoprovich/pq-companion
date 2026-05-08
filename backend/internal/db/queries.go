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
  i.clickeffect,
  COALESCE(NULLIF(i.clickname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.clickeffect), '') AS clickname,
  i.proceffect,
  COALESCE(NULLIF(i.procname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.proceffect), '') AS procname,
  i.worneffect,
  COALESCE(NULLIF(i.wornname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.worneffect), '') AS wornname,
  i.focuseffect,
  COALESCE(NULLIF(i.focusname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.focuseffect), '') AS focusname,
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
// Returns sql.ErrNoRows for any ID on the hidden list.
func (db *DB) GetItem(id int) (*Item, error) {
	if isHiddenItem(id) {
		return nil, fmt.Errorf("get item %d: %w", id, sql.ErrNoRows)
	}
	q := fmt.Sprintf("SELECT %s FROM items i WHERE i.id = ?", itemColumns)
	row := db.QueryRow(q, id)
	it, err := scanItem(row)
	if err != nil {
		return nil, fmt.Errorf("get item %d: %w", id, err)
	}
	return it, nil
}

// ItemIcons returns a map of itemID → icon for the given IDs in a single query.
// IDs not present in the items table (or with icon=0) are omitted from the map.
func (db *DB) ItemIcons(ids []int) (map[int]int, error) {
	out := make(map[int]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	q := fmt.Sprintf("SELECT id, icon FROM items WHERE icon > 0 AND id IN (%s)", placeholders)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query item icons: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, icon int
		if err := rows.Scan(&id, &icon); err != nil {
			return nil, fmt.Errorf("scan item icon: %w", err)
		}
		out[id] = icon
	}
	return out, rows.Err()
}

// SearchItems returns a filtered, paginated list of items.
// Zero-value fields in f mean "no filter" (except ItemType: -1 = any).
func (db *DB) SearchItems(f ItemFilter) (*SearchResult[Item], error) {
	pattern := "%" + strings.ReplaceAll(f.Query, "%", "\\%") + "%"

	where := "Name LIKE ? ESCAPE '\\'"
	args := []any{pattern}

	if f.BaneBody > 0 {
		where += " AND banedmgbody = ?"
		args = append(args, f.BaneBody)
	}
	if f.Race > 0 {
		where += " AND (races = 0 OR races >= 65535 OR (races & ?) != 0)"
		args = append(args, f.Race)
	}
	if f.Class > 0 {
		where += " AND (classes = 0 OR classes >= 32767 OR (classes & ?) != 0)"
		args = append(args, f.Class)
	}
	if f.MinLevel > 0 {
		where += " AND reqlevel >= ?"
		args = append(args, f.MinLevel)
	}
	if f.MaxLevel > 0 {
		where += " AND (reqlevel = 0 OR reqlevel <= ?)"
		args = append(args, f.MaxLevel)
	}
	if f.Slot > 0 {
		where += " AND (slots & ?) != 0"
		args = append(args, f.Slot)
	}
	if f.ItemType >= 0 {
		where += " AND itemtype = ?"
		args = append(args, f.ItemType)
	}
	if f.MinSTR > 0 {
		where += " AND astr >= ?"
		args = append(args, f.MinSTR)
	}
	if f.MinSTA > 0 {
		where += " AND asta >= ?"
		args = append(args, f.MinSTA)
	}
	if f.MinAGI > 0 {
		where += " AND aagi >= ?"
		args = append(args, f.MinAGI)
	}
	if f.MinDEX > 0 {
		where += " AND adex >= ?"
		args = append(args, f.MinDEX)
	}
	if f.MinWIS > 0 {
		where += " AND awis >= ?"
		args = append(args, f.MinWIS)
	}
	if f.MinINT > 0 {
		where += " AND aint >= ?"
		args = append(args, f.MinINT)
	}
	if f.MinCHA > 0 {
		where += " AND acha >= ?"
		args = append(args, f.MinCHA)
	}
	if f.MinHP > 0 {
		where += " AND hp >= ?"
		args = append(args, f.MinHP)
	}
	if f.MinMana > 0 {
		where += " AND mana >= ?"
		args = append(args, f.MinMana)
	}
	if f.MinAC > 0 {
		where += " AND ac >= ?"
		args = append(args, f.MinAC)
	}
	if f.MinMR > 0 {
		where += " AND mr >= ?"
		args = append(args, f.MinMR)
	}
	if f.MinCR > 0 {
		where += " AND cr >= ?"
		args = append(args, f.MinCR)
	}
	if f.MinDR > 0 {
		where += " AND dr >= ?"
		args = append(args, f.MinDR)
	}
	if f.MinFR > 0 {
		where += " AND fr >= ?"
		args = append(args, f.MinFR)
	}
	if f.MinPR > 0 {
		where += " AND pr >= ?"
		args = append(args, f.MinPR)
	}

	if clause, hargs := hiddenItemClause(); clause != "" {
		where += " AND " + clause
		args = append(args, hargs...)
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
	rows, err := db.Query(q, append(args, f.Limit, f.Offset)...)
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

// BaseData is the per-(class, level) "blank-slate" HP/Mana values from the
// base_data table — what a level-up gives a character before any stat-bonus
// or equipment math is applied.
type BaseData struct {
	HP   float64
	Mana float64
}

// GetBaseData returns the base HP/Mana for a given EQEmu class index
// (1=Warrior … 14=Enchanter) at the given level. Returns zeros if the row
// doesn't exist (e.g. caller passed class 0 / unset).
func (db *DB) GetBaseData(level, classIdx int) (BaseData, error) {
	if level <= 0 || classIdx <= 0 {
		return BaseData{}, nil
	}
	var bd BaseData
	err := db.QueryRow(
		`SELECT hp, mana FROM base_data WHERE level = ? AND class = ?`,
		level, classIdx,
	).Scan(&bd.HP, &bd.Mana)
	if err == sql.ErrNoRows {
		return BaseData{}, nil
	}
	if err != nil {
		return BaseData{}, fmt.Errorf("get base_data(%d,%d): %w", level, classIdx, err)
	}
	return bd, nil
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
  n.see_invis, n.see_invis_undead,
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
		&n.SeeInvis, &n.SeeInvisUndead,
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
// When hidePlaceholders is true, entries with level 0, class 0, or names
// starting with "#" are excluded.
// NPCs with empty names or a name of just "#" are always excluded — those
// rows do not reference real in-game NPCs.
func (db *DB) SearchNPCs(query string, limit, offset int, hidePlaceholders bool) (*SearchResult[NPC], error) {
	// EQEmu stores NPC names with underscores; the UI displays spaces. Map
	// query whitespace to underscores so what the user sees matches what
	// the user can type (e.g. "#Lord_Inquisitor_Seru" surfaces for both
	// "lord" and "lord inquisitor").
	normalized := strings.ReplaceAll(query, " ", "_")
	pattern := "%" + strings.ReplaceAll(normalized, "%", "\\%") + "%"

	baseClause := " AND n.name != '' AND n.name != '#'"
	placeholderClause := ""
	if hidePlaceholders {
		placeholderClause = " AND n.name NOT LIKE '#%' AND n.level > 0 AND n.class > 0"
	}
	filterClause := baseClause + placeholderClause

	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM npc_types n WHERE n.name LIKE ? ESCAPE '\\'"+filterClause,
		pattern,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count npcs: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM npc_types n %s WHERE n.name LIKE ? ESCAPE '\\'%s ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns, npcJoin, filterClause,
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

	points := []NPCSpawnPoint{}
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
		       lde.item_id, i.Name, i.icon, lde.chance, lde.multiplier
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
			itemID, itemIcon, ldeMultiplier       int
			itemName                               string
			chance                                 float64
		)
		if err := rows.Scan(&dropID, &dropName, &lteMultiplier, &lteProbability, &itemID, &itemName, &itemIcon, &chance, &ldeMultiplier); err != nil {
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
			ItemIcon:   itemIcon,
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
  COALESCE(s.zonetype, 0),
  COALESCE(s.goodEffect, 0)`

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
		&sp.GoodEffect,
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
// classIndex: -1 = all classes (excludes NPC-only spells), 0–14 = filter to
// that player class, 15 = NPC-only (every classes1–classes15 column is 255).
// minLevel/maxLevel: 0 = no bound; only applied when classIndex is 0–14.
func (db *DB) SearchSpells(query string, classIndex, minLevel, maxLevel, limit, offset int) (*SearchResult[Spell], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	conditions := []string{"s.name LIKE ? ESCAPE '\\'", "s.name != ''"}
	args := []any{pattern}

	npcOnlyExpr := "(s.classes1 = 255 AND s.classes2 = 255 AND s.classes3 = 255 AND " +
		"s.classes4 = 255 AND s.classes5 = 255 AND s.classes6 = 255 AND " +
		"s.classes7 = 255 AND s.classes8 = 255 AND s.classes9 = 255 AND " +
		"s.classes10 = 255 AND s.classes11 = 255 AND s.classes12 = 255 AND " +
		"s.classes13 = 255 AND s.classes14 = 255 AND s.classes15 = 255)"

	var classCol string
	switch {
	case classIndex >= 0 && classIndex <= 14:
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
	case classIndex == 15:
		conditions = append(conditions, npcOnlyExpr)
	default:
		conditions = append(conditions, "NOT "+npcOnlyExpr)
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
		"SELECT id, name, icon FROM items WHERE scrolleffect = ? ORDER BY name",
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
		if err := scrollRows.Scan(&ref.ID, &ref.Name, &ref.Icon); err != nil {
			return nil, fmt.Errorf("scan scroll item: %w", err)
		}
		result.ScrollItems = append(result.ScrollItems, ref)
	}
	if err := scrollRows.Err(); err != nil {
		return nil, err
	}

	effectRows, err := db.Query(`
		SELECT effect_type, id, name, icon FROM (
			SELECT 'click' AS effect_type, id, name, icon FROM items WHERE clickeffect = ?
			UNION
			SELECT 'worn', id, name, icon FROM items WHERE worneffect = ?
			UNION
			SELECT 'proc', id, name, icon FROM items WHERE proceffect = ?
			UNION
			SELECT 'focus', id, name, icon FROM items WHERE focuseffect = ?
		) ORDER BY effect_type, name`,
		spellID, spellID, spellID, spellID,
	)
	if err != nil {
		return nil, fmt.Errorf("get spell effect items: %w", err)
	}
	defer effectRows.Close()

	for effectRows.Next() {
		var ref SpellItemRef
		if err := effectRows.Scan(&ref.EffectType, &ref.ID, &ref.Name, &ref.Icon); err != nil {
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

// zoneVisibilityFilter returns a SQL clause restricting `short_name` to the
// allowlist defined in zone_allowlist.go (sourced from PQDI). Returns the
// clause prefixed with the given string (e.g. " AND " or " WHERE ") and the
// matching arg slice; empty if the allowlist is empty.
func zoneVisibilityFilter(prefix string) (string, []any) {
	if len(visibleZoneShortNames) == 0 {
		return "", nil
	}
	args := make([]any, 0, len(visibleZoneShortNames))
	for name := range visibleZoneShortNames {
		args = append(args, name)
	}
	ph := strings.Repeat("?,", len(args))
	return fmt.Sprintf("%sshort_name IN (%s)", prefix, ph[:len(ph)-1]), args
}

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

// ZoneSearchFilters narrows zone search results. Nil fields mean no filter.
type ZoneSearchFilters struct {
	Expansion *int
}

// SearchZones searches zones by long_name (case-insensitive substring match).
func (db *DB) SearchZones(query string, filters ZoneSearchFilters, limit, offset int) (*SearchResult[Zone], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	hiddenFilter, hiddenArgs := zoneVisibilityFilter(" AND ")

	extraFilter := ""
	extraArgs := []any{}
	if filters.Expansion != nil {
		extraFilter = " AND expansion = ?"
		extraArgs = append(extraArgs, *filters.Expansion)
	}

	var total int
	countArgs := append([]any{pattern}, hiddenArgs...)
	countArgs = append(countArgs, extraArgs...)
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM zone WHERE long_name LIKE ? ESCAPE '\\'"+hiddenFilter+extraFilter,
		countArgs...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count zones: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM zone z WHERE z.long_name LIKE ? ESCAPE '\\'%s%s ORDER BY z.long_name LIMIT ? OFFSET ?",
		zoneColumns, hiddenFilter, extraFilter,
	)
	queryArgs := append([]any{pattern}, hiddenArgs...)
	queryArgs = append(queryArgs, extraArgs...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := db.Query(q, queryArgs...)
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

// ZoneExpansions returns the distinct expansion IDs present in the zone
// browser, after hidden-zone filtering. Sorted ascending.
func (db *DB) ZoneExpansions() ([]int, error) {
	hiddenFilter, hiddenArgs := zoneVisibilityFilter(" WHERE ")
	rows, err := db.Query(
		"SELECT DISTINCT expansion FROM zone"+hiddenFilter+" ORDER BY expansion",
		hiddenArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("query zone expansions: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var e int
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("scan zone expansion: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
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

// ─── AA ───────────────────────────────────────────────────────────────────────

// AAInfo describes a single Alternate Advancement ability from altadv_vars.
// AAID is altadv_vars.eqmacid — the AA index used by the EQ client and by the
// Zeal "AAIndex" export (NOT altadv_vars.skill_id, which is an internal EQEmu
// row id that isn't 1:1 with what the client/server reference).
type AAInfo struct {
	AAID        int    `json:"aa_id"` // altadv_vars.eqmacid
	Name        string `json:"name"`
	Cost        int    `json:"cost"`
	CostInc     int    `json:"cost_inc"`
	MaxLevel    int    `json:"max_level"`
	Type        int    `json:"type"` // 1=General, 2=Archetype, 3=Class, 4=PoP Advance, 5=PoP Ability
	Description string `json:"description,omitempty"`
}

// LookupAANames returns a map of eqmacid → name for the given AA indexes.
// Indexes not found in altadv_vars are omitted from the result.
func (db *DB) LookupAANames(ids []int) (map[int]string, error) {
	if len(ids) == 0 {
		return map[int]string{}, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query(
		`SELECT eqmacid, name FROM altadv_vars
		 WHERE eqmacid IN (`+placeholders+`) AND name != 'NOT USED'`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup aa names: %w", err)
	}
	defer rows.Close()
	result := make(map[int]string, len(ids))
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan aa name: %w", err)
		}
		result[id] = name
	}
	return result, rows.Err()
}

// ListAvailableAAs returns all Alternate Advancement abilities eligible for the
// given EQ class index (1-15). The classes column is a bitmask where bit N
// (1-indexed) is set when class N can purchase the AA.
//
// Filters:
//   - name != 'NOT USED'   — placeholder rows kept for AA index slot reservation.
//   - cost > 0             — disabled / template rows.
//   - eqmacid > 0          — rows the client AA window can't reference.
//   - class_type != 0      — class_type=0 marks rogue rows that lie about their
//     classes mask (e.g. Sonic Call, Chain Combo, Quick Hide, Quick Throw,
//     Advanced Spell Casting Mastery). They claim classes=65534 (all classes)
//     but are really class-specific bonus AAs misfiled with type=1 (General).
//     Real General AAs use class_type=51; every legitimate row uses a non-zero
//     class_type matched to its category.
//
// Some abilities (e.g. "Mental Clarity", "Innate Regeneration") appear twice
// in altadv_vars — once with a legacy eqmacid and again with the current
// client-facing eqmacid. We dedupe by name keeping the row with the highest
// eqmacid, which is what the client/Zeal export references.
func (db *DB) ListAvailableAAs(class int) ([]AAInfo, error) {
	if class < 1 || class > 15 {
		return []AAInfo{}, nil
	}
	mask := 1 << class
	rows, err := db.Query(
		`SELECT eqmacid, name, cost, cost_inc, max_level, type
		 FROM altadv_vars
		 WHERE name != 'NOT USED'
		   AND cost > 0
		   AND eqmacid > 0
		   AND class_type != 0
		   AND (classes & ?) != 0
		 ORDER BY eqmacid`,
		mask,
	)
	if err != nil {
		return nil, fmt.Errorf("list available aas: %w", err)
	}
	defer rows.Close()

	byName := make(map[string]AAInfo)
	for rows.Next() {
		var info AAInfo
		if err := rows.Scan(&info.AAID, &info.Name, &info.Cost, &info.CostInc, &info.MaxLevel, &info.Type); err != nil {
			return nil, fmt.Errorf("scan aa: %w", err)
		}
		// Keep the entry with the highest eqmacid for each duplicated name —
		// it's the one the live client/Zeal export references.
		if existing, ok := byName[info.Name]; !ok || info.AAID > existing.AAID {
			byName[info.Name] = info
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	descs := loadAADescriptions()
	out := make([]AAInfo, 0, len(byName))
	for _, info := range byName {
		info.Description = descs[info.AAID]
		out = append(out, info)
	}
	return out, nil
}

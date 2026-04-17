package db

import (
	"database/sql"
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

// ─── NPCs ─────────────────────────────────────────────────────────────────────

const npcColumns = `
  n.id, n.name, COALESCE(n.lastname, ''), n.level, n.race, n.class, n.bodytype,
  n.hp, n.mana, n.mindmg, n.maxdmg, n.attack_count,
  n.MR, n.CR, n.DR, n.FR, n.PR, n.AC,
  n.STR, n.STA, n.DEX, n.AGI, n."_INT", n.WIS, n.CHA,
  n.aggroradius, n.runspeed, n.size,
  n.raid_target, COALESCE(n.rare_spawn, 0),
  n.loottable_id, n.merchant_id, n.npc_spells_id, n.npc_faction_id,
  COALESCE(n.special_abilities, ''),
  n.spellscale, n.healscale, n.exp_pct`

func scanNPC(row interface {
	Scan(...any) error
}) (*NPC, error) {
	var n NPC
	err := row.Scan(
		&n.ID, &n.Name, &n.LastName, &n.Level, &n.Race, &n.Class, &n.BodyType,
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
	q := fmt.Sprintf("SELECT %s FROM npc_types n WHERE n.id = ?", npcColumns)
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
		"SELECT %s FROM npc_types n WHERE n.name = ? COLLATE NOCASE LIMIT 1",
		npcColumns,
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
		"SELECT %s FROM npc_types n WHERE n.name LIKE ? ESCAPE '\\' ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns,
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
  s.icon, s.new_icon, s.IsDiscipline, s.suspendable, s.nodispell`

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

// SearchSpells searches spells by name (case-insensitive substring match).
func (db *DB) SearchSpells(query string, limit, offset int) (*SearchResult[Spell], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM spells_new WHERE name LIKE ? ESCAPE '\\'",
		pattern,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count spells: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM spells_new s WHERE s.name LIKE ? ESCAPE '\\' ORDER BY s.name LIMIT ? OFFSET ?",
		spellColumns,
	)
	rows, err := db.Query(q, pattern, limit, offset)
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

// ─── Zones ────────────────────────────────────────────────────────────────────

const zoneColumns = `
  z.id, COALESCE(z.short_name,''), z.long_name, COALESCE(z.file_name,''),
  z.zoneidnumber, z.safe_x, z.safe_y, z.safe_z,
  z.min_level, COALESCE(z.note,'')`

func scanZone(row interface {
	Scan(...any) error
}) (*Zone, error) {
	var z Zone
	err := row.Scan(
		&z.ID, &z.ShortName, &z.LongName, &z.FileName,
		&z.ZoneIDNumber, &z.SafeX, &z.SafeY, &z.SafeZ,
		&z.MinLevel, &z.Note,
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
		"SELECT %s FROM npc_types n WHERE n.id IN (%s) ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns, idSubquery,
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

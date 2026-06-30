package db

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
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
  i.wornlevel,
  i.focuseffect,
  COALESCE(NULLIF(i.focusname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.focuseffect), '') AS focusname,
  i.maxcharges,
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
		&it.WornEffect, &it.WornName, &it.WornLevel, &it.FocusEffect, &it.FocusName,
		&it.MaxCharges,
		&it.BagSize, &it.BagSlots, &it.BagType,
		&it.Stackable, &it.StackSize,
		&it.Price, &it.Icon, &it.MinStatus,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// MatchItemNameInText returns the longest game-item name that appears as a
// case-insensitive substring of text, or ok=false when none does. It backs
// the roll tracker's best-effort loot-item auto-suggest: given a raid/chat
// line like "Robe of the Lost Circle 333 pick", it recovers the canonical
// item name regardless of surrounding words or the roll number.
//
// Names shorter than minItemNameMatchLen are ignored so common short item
// names (e.g. "Bone") don't spuriously match ordinary chatter. LIKE is
// case-insensitive for ASCII in SQLite, which covers EQ item names; the
// longest match wins so "Robe of the Lost Circle" beats a bare "Robe".
func (db *DB) MatchItemNameInText(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) < minItemNameMatchLen {
		return "", false
	}
	const q = `SELECT Name FROM items
WHERE LENGTH(Name) >= ? AND ? LIKE '%' || Name || '%'
ORDER BY LENGTH(Name) DESC
LIMIT 1`
	var name string
	if err := db.QueryRow(q, minItemNameMatchLen, text).Scan(&name); err != nil {
		return "", false
	}
	return name, true
}

// minItemNameMatchLen is the shortest item name (and shortest input text)
// the loot-item auto-suggest will consider, to keep false positives down.
const minItemNameMatchLen = 5

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
	// Derive WornHastePct from the worn spell when applicable. Best-effort:
	// a missing or malformed spell just leaves the field at 0.
	if it.WornEffect > 0 {
		if sp, sperr := db.GetSpell(it.WornEffect); sperr == nil && sp != nil {
			it.WornHastePct = ComputeWornHastePct(sp, it.WornLevel)
		}
	}
	// Surface any same-name duplicates collapsed out of list views. Variants
	// stay fetchable here (unlike hidden items above) so the detail view can
	// link to them.
	db.ensureVariants()
	it.VariantIDs, it.CanonicalID = db.itemVariants.variantFields(it.ID)
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

// RechargeableMaxCharges returns id→maxcharges for the given item IDs, limited
// to genuinely rechargeable items: click items (clickeffect > 0) with a
// positive multi-charge cap (maxcharges > 1). Single-charge consumables and
// unlimited clickies (the -1/0 sentinel) are excluded, so a present entry means
// "this is a rechargeable item." Used to flag held inventory items.
func (db *DB) RechargeableMaxCharges(ids []int) (map[int]int, error) {
	out := make(map[int]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	q := fmt.Sprintf(
		"SELECT id, maxcharges FROM items WHERE clickeffect > 0 AND maxcharges > 1 AND id IN (%s)",
		placeholders,
	)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query rechargeable charges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, maxCharges int
		if err := rows.Scan(&id, &maxCharges); err != nil {
			return nil, fmt.Errorf("scan rechargeable charges: %w", err)
		}
		out[id] = maxCharges
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

	// Collapse duplicate-name rows to the canonical one (see variants.go).
	db.ensureVariants()
	if clause := db.itemVariants.excludeNonCanonical("id"); clause != "" {
		where += " AND " + clause
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
	// List rows are canonical; attach their hidden variants so a row clicked
	// straight from the list (without a follow-up GetItem) still shows them.
	for i := range items {
		items[i].VariantIDs, items[i].CanonicalID = db.itemVariants.variantFields(items[i].ID)
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

// Mac-era skill_caps.skill_id values for the Quarm/EQMacEmu data. The Mac-era
// skill enum is offset from modern EQEmu (where Defense is 16): here Defense is
// 15 (id 9 is Bind Wound). quarm.db skill_caps follows the EQMacEmu
// common/skills.h enum verbatim — verified against the DB by class availability:
// Defense (15) gives WAR/PAL/SHD/MNK/BRD/ROG 252, RNG/BST 240, CLR/DRU/SHM 200,
// NEC/WIZ/MAG/ENC 145; Offense (33) has a row for every class (casters ~140), as
// expected for a universal skill. (skill_id 22 is Dual Wield, not Offense — only
// the dual-wield classes have it; it was previously misidentified here.)
const (
	defenseSkillID = 15
	offenseSkillID = 33
)

// meleeWeaponSkillIDs are the player melee weapon skills plus Hand to Hand, in
// the EQMac enum: 1HBlunt(0), 1HSlash(1), 2HBlunt(2), 2HSlash(3), 1HPiercing(36),
// HandToHand(28). BestWeaponSkillCap takes the max cap across these — the
// character's most-trained melee skill, used as the assumed weapon skill for the
// ATK rating (the export carries no equipped-weapon type). (Previously used
// 29/19, which are actually Hide and Dodge.)
var meleeWeaponSkillIDs = []int{0, 1, 2, 3, 36, 28}

// skillCap returns the maximum value of the given skill a class can reach at
// the given level. classIdx is 1-indexed (1=Warrior … 14=Enchanter,
// 15=Beastlord). Returns 0 if no row matches (the class never trains it).
func (db *DB) skillCap(skillID, classIdx, level int) (int, error) {
	if level <= 0 || classIdx <= 0 {
		return 0, nil
	}
	var cap int
	err := db.QueryRow(
		`SELECT cap FROM skill_caps WHERE skill_id = ? AND class_id = ? AND level = ?`,
		skillID, classIdx, level,
	).Scan(&cap)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("skill cap(skill=%d,class=%d,level=%d): %w", skillID, classIdx, level, err)
	}
	return cap, nil
}

// SkillCap returns the maximum value of the given skill_id a class can reach
// at the given level, looked up from skill_caps. classIdx is 1-indexed
// (1=Warrior … 15=Beastlord). Returns 0 when the class never trains the skill.
// Exported wrapper around skillCap for the Skill Tracker.
func (db *DB) SkillCap(skillID, classIdx, level int) (int, error) {
	return db.skillCap(skillID, classIdx, level)
}

// DefenseSkillCap returns the maximum Defense skill a class can reach at the
// given level. Used as the assumed Defense value when computing displayed AC —
// a max-level main is virtually always at cap, and the Quarmy export carries
// no live skill values.
func (db *DB) DefenseSkillCap(classIdx, level int) (int, error) {
	return db.skillCap(defenseSkillID, classIdx, level)
}

// OffenseSkillCap returns the maximum Offense skill a class can reach at the
// given level, used as the assumed Offense value in the ATK rating. Pure
// casters have no Offense skill and correctly return 0.
func (db *DB) OffenseSkillCap(classIdx, level int) (int, error) {
	return db.skillCap(offenseSkillID, classIdx, level)
}

// BestWeaponSkillCap returns the highest melee weapon skill cap (across the
// player melee weapon skills and Hand to Hand) for the class/level — the
// assumed weapon skill when deriving the ATK rating. Returns 0 if no row
// matches.
func (db *DB) BestWeaponSkillCap(classIdx, level int) (int, error) {
	if level <= 0 || classIdx <= 0 {
		return 0, nil
	}
	best := 0
	for _, id := range meleeWeaponSkillIDs {
		c, err := db.skillCap(id, classIdx, level)
		if err != nil {
			return 0, err
		}
		if c > best {
			best = c
		}
	}
	return best, nil
}

func collectItems(rows *sql.Rows) ([]Item, error) {
	// Initialise non-nil so an empty result marshals to [] not null — the
	// SearchResult JSON contract is items: T[], and a null trips consumers
	// that index/spread/length the array (black-screened the resist
	// calculator's NPC search on a no-match query).
	result := []Item{}
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
  n.id, n.name, COALESCE(n.lastname, ''), n.level, n.maxlevel, n.race, COALESCE(r.name, 'Race ' || n.race), n.class, n.bodytype,
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
		&n.ID, &n.Name, &n.LastName, &n.Level, &n.MaxLevel, &n.Race, &n.RaceName, &n.Class, &n.BodyType,
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

// GetNPCIDByName resolves an in-game NPC display name (spaces, any case) to a
// single npc_types id, best-effort. EQ display names use spaces; the database
// stores them with underscores, so spaces are converted before matching.
// Same-name variants (e.g. multi-id raid bosses) all collapse to the lowest id;
// the NPC detail page surfaces the rest. Returns (id, true) on a match,
// (0, false) when nothing matches. Used by the lockout tracker to link
// raid-boss rows.
func (db *DB) GetNPCIDByName(name string) (int, bool) {
	underscored := strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
	if underscored == "" {
		return 0, false
	}
	var id int
	if err := db.QueryRow(
		"SELECT id FROM npc_types WHERE name = ? COLLATE NOCASE ORDER BY id LIMIT 1",
		underscored,
	).Scan(&id); err != nil {
		return 0, false
	}
	return id, true
}

// GetItemIDByName resolves an exact item name (any case) to its canonical item
// id, best-effort. Item names are usually unique; when duplicates exist the
// canonical (most-referenced) variant is returned so the link lands on the
// "main" entry. Returns (id, true) on a match, (0, false) otherwise. Used by
// the lockout tracker to link legacy-item rows.
func (db *DB) GetItemIDByName(name string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return 0, false
	}
	var id int
	if err := db.QueryRow(
		"SELECT id FROM items WHERE Name = ? COLLATE NOCASE ORDER BY id LIMIT 1",
		trimmed,
	).Scan(&id); err != nil {
		return 0, false
	}
	db.ensureVariants()
	if canon := db.itemVariants.canonicalID(id); canon != 0 {
		id = canon
	}
	return id, true
}

// GetNPCVariantsByNameInZone returns every npc_types row matching the given
// name (case-insensitive), each paired with its spawn2 locations. About 24%
// of NPCs in the Quarm DB share a name with at least one other row — the
// overlay uses this to disambiguate against the player's zone and position
// (see backend/internal/overlay/npc.go).
//
// When zoneShortName is non-empty, only variants with at least one spawn
// point in that zone are returned, and each variant's SpawnPoints field is
// populated. When it is empty (no Zeal data → zone unknown), the spawn join
// is skipped and variants come back with SpawnPoints nil — callers can still
// see the alternatives but can't position-match.
//
// Variants are ordered by npc_types.id ascending so callers get a
// deterministic primary pick when no disambiguation signal is available.
// Returns an empty slice (not an error) when nothing matches.
func (db *DB) GetNPCVariantsByNameInZone(name, zoneShortName string) ([]NPCVariant, error) {
	if zoneShortName == "" {
		q := fmt.Sprintf(
			"SELECT %s FROM npc_types n %s WHERE n.name = ? COLLATE NOCASE ORDER BY n.id",
			npcColumns, npcJoin,
		)
		rows, err := db.Query(q, name)
		if err != nil {
			return nil, fmt.Errorf("get npc variants by name %q: %w", name, err)
		}
		defer rows.Close()
		npcs, err := collectNPCs(rows)
		if err != nil {
			return nil, err
		}
		out := make([]NPCVariant, len(npcs))
		for i := range npcs {
			out[i] = NPCVariant{NPC: npcs[i]}
		}
		return out, nil
	}

	// Zone-scoped variant: join npc_types → spawnentry → spawn2 so we only
	// return variants that actually spawn in the requested zone, and gather
	// every spawn point in one pass. Rows are ordered by npc_id, so we can
	// fold them into variants in a single linear scan.
	q := fmt.Sprintf(`
		SELECT %s, s2.spawngroupID, s2.x, s2.y, s2.z
		FROM npc_types n %s
		JOIN spawnentry se ON se.npcID = n.id
		JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID
		WHERE n.name = ? COLLATE NOCASE
		  AND s2.zone = ?
		ORDER BY n.id, s2.spawngroupID, s2.id`,
		npcColumns, npcJoin)
	rows, err := db.Query(q, name, zoneShortName)
	if err != nil {
		return nil, fmt.Errorf("get npc variants by name %q in zone %q: %w", name, zoneShortName, err)
	}
	defer rows.Close()

	var variants []NPCVariant
	curID := -1
	for rows.Next() {
		var n NPC
		var spawngroupID int
		var x, y, z float64
		err := rows.Scan(
			&n.ID, &n.Name, &n.LastName, &n.Level, &n.MaxLevel, &n.Race, &n.RaceName, &n.Class, &n.BodyType,
			&n.HP, &n.Mana, &n.MinDmg, &n.MaxDmg, &n.AttackCount,
			&n.MR, &n.CR, &n.DR, &n.FR, &n.PR, &n.AC,
			&n.STR, &n.STA, &n.DEX, &n.AGI, &n.INT, &n.WIS, &n.CHA,
			&n.AggroRadius, &n.RunSpeed, &n.Size,
			&n.RaidTarget, &n.RareSpawn,
			&n.LootTableID, &n.MerchantID, &n.NPCSpellsID, &n.NPCFactionID,
			&n.SpecialAbilities,
			&n.SeeInvis, &n.SeeInvisUndead,
			&n.SpellScale, &n.HealScale, &n.ExpPct,
			&spawngroupID, &x, &y, &z,
		)
		if err != nil {
			return nil, fmt.Errorf("scan npc variant: %w", err)
		}
		if curID != n.ID {
			variants = append(variants, NPCVariant{NPC: n})
			curID = n.ID
		}
		idx := len(variants) - 1
		variants[idx].SpawnPoints = append(variants[idx].SpawnPoints, SpawnPoint{
			SpawngroupID: spawngroupID,
			X:            x,
			Y:            y,
			Z:            z,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate npc variants: %w", err)
	}
	return variants, nil
}

// nonPlayerNPCClause filters out rows in npc_types that do not reference
// real in-game NPCs: empty/placeholder names ("", "#", "_") and the
// Invisible Man race (127), which EQEmu uses exclusively for invisible
// dev/trigger objects (spawners, animation puppets, environment effects,
// "You_hear_..." emitters, etc.). All NPC list/summary queries should
// chain this clause so a placeholder doesn't surface in the UI as
// "level 99 warrior" with no name.
//
// Use as `WHERE ... ` + nonPlayerNPCClause — the leading " AND " is
// included. The alias `n` must be the npc_types row.
const nonPlayerNPCClause = " AND n.name != '' AND n.name != '#' AND n.name != '_' AND n.race != 127"

// SearchNPCs searches NPCs by name (case-insensitive substring match).
// When hidePlaceholders is true, entries with level 0, class 0, or names
// starting with "#" are also excluded on top of the always-on
// nonPlayerNPCClause filter.
func (db *DB) SearchNPCs(query string, limit, offset int, hidePlaceholders bool) (*SearchResult[NPC], error) {
	// EQEmu stores NPC names with underscores; the UI displays spaces. Map
	// query whitespace to underscores so what the user sees matches what
	// the user can type (e.g. "#Lord_Inquisitor_Seru" surfaces for both
	// "lord" and "lord inquisitor").
	normalized := strings.ReplaceAll(query, " ", "_")
	pattern := "%" + strings.ReplaceAll(normalized, "%", "\\%") + "%"

	placeholderClause := ""
	if hidePlaceholders {
		placeholderClause = " AND n.name NOT LIKE '#%' AND n.level > 0 AND n.class > 0"
	}
	filterClause := nonPlayerNPCClause + placeholderClause

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
	// Non-nil so an empty result marshals to [] not null (see collectItems).
	result := []NPC{}
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

// GetNPCSpells returns the resolved npc_spells row + spell entries for an
// NPC. If the NPC has no npc_spells_id (0) the result is (nil, nil) — no
// caster AI is attached and the UI should hide the section. If parent_list
// is set, this walks the inheritance chain (max depth 4) and appends each
// ancestor's entries, tagging each entry with the source list name.
//
// Procs (attack/range/defensive) come from the NPC's own npc_spells row
// only — they don't inherit. The schema models them as direct columns,
// not list-merged entries.
func (db *DB) GetNPCSpells(npcID int) (*NPCSpells, error) {
	var listID int
	if err := db.QueryRow(`SELECT npc_spells_id FROM npc_types WHERE id = ?`, npcID).Scan(&listID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get npc %d spells id: %w", npcID, err)
	}
	if listID == 0 {
		return nil, nil
	}

	head, err := db.fetchNPCSpellListRow(listID)
	if err != nil {
		return nil, err
	}
	if head == nil {
		return nil, nil
	}

	out := &NPCSpells{
		NPCSpellsID:    head.id,
		ListName:       head.name,
		FailRecast:     head.failRecast,
		EngagedSelf:    head.engagedSelf,
		EngagedOther:   head.engagedOther,
		EngagedDetri:   head.engagedDetri,
		PursueDetri:    head.pursueDetri,
		IdleBeneficial: head.idleBenef,
		Entries:        []NPCSpellEntry{},
	}

	if head.attackProc > 0 {
		out.AttackProc = &NPCSpellProc{
			SpellID:   head.attackProc,
			SpellName: db.lookupSpellName(head.attackProc),
			Chance:    head.attackChance,
		}
	}
	if head.rangeProc > 0 {
		out.RangeProc = &NPCSpellProc{
			SpellID:   head.rangeProc,
			SpellName: db.lookupSpellName(head.rangeProc),
			Chance:    head.rangeChance,
		}
	}
	if head.defensiveProc > 0 {
		out.DefensiveProc = &NPCSpellProc{
			SpellID:   head.defensiveProc,
			SpellName: db.lookupSpellName(head.defensiveProc),
			Chance:    head.defensiveChance,
		}
	}

	// Walk the list + its parent chain (parent_list -> parent_list -> ...),
	// pulling entries from each. Depth-limited so a cyclic / mistyped chain
	// can't hang the request.
	current := head
	visited := map[int]bool{current.id: true}
	for depth := 0; depth < 4 && current != nil; depth++ {
		entries, err := db.fetchNPCSpellEntries(current.id, current.name)
		if err != nil {
			return nil, err
		}
		out.Entries = append(out.Entries, entries...)
		if current.parentList == 0 || visited[current.parentList] {
			break
		}
		visited[current.parentList] = true
		current, err = db.fetchNPCSpellListRow(current.parentList)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// npcSpellListRow is the parsed in-memory shape of a single npc_spells row.
// Internal to GetNPCSpells — not part of the API payload (the columns are
// projected onto NPCSpells fields with renaming).
type npcSpellListRow struct {
	id              int
	name            string
	parentList      int
	attackProc      int
	attackChance    int
	rangeProc       int
	rangeChance     int
	defensiveProc   int
	defensiveChance int
	failRecast      int
	engagedSelf     int
	engagedOther    int
	engagedDetri    int
	pursueDetri     int
	idleBenef       int
}

func (db *DB) fetchNPCSpellListRow(id int) (*npcSpellListRow, error) {
	row := db.QueryRow(`
		SELECT id, COALESCE(name, ''), parent_list,
		       attack_proc, proc_chance,
		       range_proc, rproc_chance,
		       defensive_proc, dproc_chance,
		       fail_recast,
		       engaged_b_self_chance, engaged_b_other_chance, engaged_d_chance,
		       pursue_d_chance, idle_b_chance
		FROM npc_spells WHERE id = ?`, id)
	var r npcSpellListRow
	err := row.Scan(
		&r.id, &r.name, &r.parentList,
		&r.attackProc, &r.attackChance,
		&r.rangeProc, &r.rangeChance,
		&r.defensiveProc, &r.defensiveChance,
		&r.failRecast,
		&r.engagedSelf, &r.engagedOther, &r.engagedDetri,
		&r.pursueDetri, &r.idleBenef,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch npc_spells %d: %w", id, err)
	}
	return &r, nil
}

func (db *DB) fetchNPCSpellEntries(listID int, listName string) ([]NPCSpellEntry, error) {
	rows, err := db.Query(`
		SELECT e.spellid, COALESCE(s.name, ''), e.type,
		       e.minlevel, e.maxlevel, e.manacost, e.recast_delay, e.priority
		FROM npc_spells_entries e
		LEFT JOIN spells_new s ON s.id = e.spellid
		WHERE e.npc_spells_id = ?
		ORDER BY e.priority DESC, e.minlevel ASC, e.spellid ASC`, listID)
	if err != nil {
		return nil, fmt.Errorf("fetch npc_spells_entries %d: %w", listID, err)
	}
	defer rows.Close()

	var out []NPCSpellEntry
	for rows.Next() {
		var e NPCSpellEntry
		if err := rows.Scan(
			&e.SpellID, &e.SpellName, &e.Type,
			&e.MinLevel, &e.MaxLevel, &e.ManaCost, &e.RecastDelay, &e.Priority,
		); err != nil {
			return nil, fmt.Errorf("scan npc_spells_entry: %w", err)
		}
		e.SourceID = listID
		e.SourceName = listName
		out = append(out, e)
	}
	return out, rows.Err()
}

// lookupSpellName resolves a spell ID to its name; empty string if missing.
// Used for proc rendering — none of the call sites need to fail on a stale
// proc id, so SQL errors are swallowed (the caller still has the id).
func (db *DB) lookupSpellName(id int) string {
	if id <= 0 {
		return ""
	}
	var name string
	if err := db.QueryRow(`SELECT COALESCE(name, '') FROM spells_new WHERE id = ?`, id).Scan(&name); err != nil {
		return ""
	}
	return name
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

// GetRespawnTimesInZone returns the respawn timing of every spawn point that
// can host an NPC with the given name (underscore form, e.g. "a_gnoll") in the
// zone identified by its short_name. An empty slice (not an error) means the
// name has no spawn data in that zone — the death-timer overlay treats that as
// "skip, no timer". The same name can resolve to multiple npc_types rows and
// multiple spawn points with different respawn times; the engine summarises the
// set into a single estimate plus an ambiguity flag.
func (db *DB) GetRespawnTimesInZone(name, zoneShort string) ([]RespawnInfo, error) {
	rows, err := db.Query(`
		SELECT n.id, s2.respawntime, s2.variance, n.level
		FROM npc_types n
		JOIN spawnentry se ON se.npcID = n.id
		JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID
		WHERE n.name = ? COLLATE NOCASE AND s2.zone = ?`, name, zoneShort)
	if err != nil {
		return nil, fmt.Errorf("get respawn times for %q in zone %q: %w", name, zoneShort, err)
	}
	defer rows.Close()

	var out []RespawnInfo
	for rows.Next() {
		var ri RespawnInfo
		if err := rows.Scan(&ri.NPCID, &ri.RespawnTime, &ri.Variance, &ri.Level); err != nil {
			return nil, fmt.Errorf("scan respawn info: %w", err)
		}
		out = append(out, ri)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate respawn info: %w", err)
	}
	return out, nil
}

// GetZoneSpawnReduction reports the two zone flags that drive Quarm's
// fast-respawn (RespawnReductionSystem): whether the zone participates in
// reduced spawn timers (zone.reducedspawntimers) and whether it uses the
// dungeon bounds rather than the standard/newbie ones (zone.castdungeon).
// An unknown zone returns (false, false, nil) so the engine falls back to the
// raw spawn2.respawntime. See respawn.reduceRespawnTime.
func (db *DB) GetZoneSpawnReduction(zoneShort string) (reduced, dungeon bool, err error) {
	var r, d int
	err = db.QueryRow(
		`SELECT reducedspawntimers, castdungeon FROM zone WHERE short_name = ? COLLATE NOCASE LIMIT 1`,
		zoneShort,
	).Scan(&r, &d)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("get spawn reduction flags for zone %q: %w", zoneShort, err)
	}
	return r != 0, d != 0, nil
}

// GetZoneShortNameByLongName resolves a zone's long_name (as it appears in the
// "You have entered <long_name>." log line) to its short_name. Returns "" with
// no error when nothing matches, so callers can degrade gracefully. A trailing
// parenthetical (e.g. "Plane of Fear (Instanced)") is stripped before matching.
func (db *DB) GetZoneShortNameByLongName(longName string) (string, error) {
	longName = strings.TrimSpace(longName)
	if i := strings.Index(longName, " ("); i >= 0 {
		longName = strings.TrimSpace(longName[:i])
	}
	if longName == "" {
		return "", nil
	}
	var short string
	err := db.QueryRow(
		`SELECT short_name FROM zone WHERE long_name = ? COLLATE NOCASE LIMIT 1`,
		longName,
	).Scan(&short)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get zone short_name for %q: %w", longName, err)
	}
	return short, nil
}

// Quarm-specific zone-wide loot overlays. The Quarm DB stores zone-wide
// shared drops as standalone lootdrops that aren't actually attached to any
// NPC's loottable — they need to be surfaced manually on every NPC in the
// zone, in addition to whatever loot the NPC's own table provides.
type zoneLootOverlay struct {
	npcIDMin   int
	npcIDMax   int
	lootdropID int
	label      string
}

var zoneLootOverlays = []zoneLootOverlay{
	// Vex Thal (zone id 158). Lootdrop 6150532 ("VT LegacyLoot") holds the
	// 19-item zone-wide drop pool that applies to every Vex Thal mob.
	{npcIDMin: 158000, npcIDMax: 158999, lootdropID: 6150532, label: "Vex Thal zone-wide loot"},
}

func zoneOverlayFor(npcID int) *zoneLootOverlay {
	for i := range zoneLootOverlays {
		o := &zoneLootOverlays[i]
		if npcID >= o.npcIDMin && npcID <= o.npcIDMax {
			return o
		}
	}
	return nil
}

// GetNPCLoot returns the resolved loot table for the NPC with the given ID.
// Returns nil when the NPC has neither its own loot nor a zone-wide overlay.
func (db *DB) GetNPCLoot(npcID int) (*NPCLootTable, error) {
	ltID, ltName, drops, err := db.loadNPCLoot(npcID)
	if err != nil {
		return nil, err
	}

	var zoneDrops []LootDrop
	var zoneLabel string
	if overlay := zoneOverlayFor(npcID); overlay != nil {
		drop, err := db.loadLootdrop(overlay.lootdropID)
		if err != nil {
			return nil, fmt.Errorf("load zone overlay lootdrop %d for npc %d: %w", overlay.lootdropID, npcID, err)
		}
		if drop != nil && len(drop.Items) > 0 {
			zoneDrops = []LootDrop{*drop}
			zoneLabel = overlay.label
		}
	}

	if ltID == 0 && len(zoneDrops) == 0 {
		return nil, nil
	}

	return &NPCLootTable{
		ID:            ltID,
		Name:          ltName,
		Drops:         drops,
		ZoneWideDrops: zoneDrops,
		ZoneWideLabel: zoneLabel,
	}, nil
}

// loadLootdrop loads a single lootdrop pool and its items by lootdrop_id,
// independent of any NPC's loottable. Used to surface zone-wide loot pools
// (e.g. Vex Thal's "VT LegacyLoot") that aren't attached to any NPC's
// loottable in the Quarm DB. Returns nil if the lootdrop doesn't exist.
func (db *DB) loadLootdrop(ldID int) (*LootDrop, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM lootdrop WHERE id = ?`, ldID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lootdrop %d: %w", ldID, err)
	}

	rows, err := db.Query(`
		SELECT lde.item_id, i.Name, i.icon, lde.chance, lde.multiplier
		FROM lootdrop_entries lde
		JOIN items i ON i.id = lde.item_id
		WHERE lde.lootdrop_id = ?
		ORDER BY lde.chance DESC
		LIMIT 500`, ldID)
	if err != nil {
		return nil, fmt.Errorf("get lootdrop entries %d: %w", ldID, err)
	}
	defer rows.Close()

	drop := &LootDrop{
		ID:          ldID,
		Name:        name,
		Multiplier:  1,
		Probability: 100,
	}
	for rows.Next() {
		var (
			itemID, itemIcon, mult int
			itemName               string
			chance                 float64
		)
		if err := rows.Scan(&itemID, &itemName, &itemIcon, &chance, &mult); err != nil {
			return nil, fmt.Errorf("scan lootdrop row: %w", err)
		}
		drop.Items = append(drop.Items, LootDropItem{
			ItemID:     itemID,
			ItemName:   itemName,
			ItemIcon:   itemIcon,
			Chance:     chance,
			Multiplier: mult,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return drop, nil
}

// loadNPCLoot resolves the loottable for a single NPC and returns its
// loottable id, name, and drop groups. ltID is 0 when the NPC has no
// loottable_id set.
func (db *DB) loadNPCLoot(npcID int) (int, string, []LootDrop, error) {
	var ltID int
	var ltName string
	err := db.QueryRow(`
		SELECT lt.id, lt.name
		FROM npc_types n
		JOIN loottable lt ON lt.id = n.loottable_id
		WHERE n.id = ? AND n.loottable_id > 0`, npcID).Scan(&ltID, &ltName)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil, nil
	}
	if err != nil {
		return 0, "", nil, fmt.Errorf("get npc loottable %d: %w", npcID, err)
	}

	// Order pools by specificity, not raw lootdrop_id. A lootdrop referenced
	// by only a handful of loottables is this NPC's own signature loot; pools
	// shared across dozens of tables ("Velious Spells 60", "level_49_research",
	// gem/cash pools) are generic libraries that belong below the NPC-specific
	// drops. A quarm.db data release shifted raw id ordering and pushed those
	// shared spell/research pools above signature item pools; ranking by
	// reference count (then drop probability) restores NPC-specific items to
	// the top regardless of id churn. The ref-count subquery aggregates the
	// (~20k row) table once.
	rows, err := db.Query(`
		SELECT lte.lootdrop_id, ld.name, lte.multiplier, lte.probability,
		       lde.item_id, i.Name, i.icon, lde.chance, lde.multiplier
		FROM loottable_entries lte
		JOIN lootdrop ld ON ld.id = lte.lootdrop_id
		JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		JOIN items i ON i.id = lde.item_id
		JOIN (SELECT lootdrop_id, COUNT(*) AS ref_tables
		      FROM loottable_entries GROUP BY lootdrop_id) rc
		  ON rc.lootdrop_id = lte.lootdrop_id
		WHERE lte.loottable_id = ?
		ORDER BY rc.ref_tables ASC, lte.probability DESC, lte.lootdrop_id, lde.chance DESC
		LIMIT 500`, ltID)
	if err != nil {
		return 0, "", nil, fmt.Errorf("get npc loot entries %d: %w", npcID, err)
	}
	defer rows.Close()

	dropMap := make(map[int]*LootDrop)
	var dropOrder []int
	for rows.Next() {
		var (
			dropID, lteMultiplier, lteProbability int
			dropName                              string
			itemID, itemIcon, ldeMultiplier       int
			itemName                              string
			chance                                float64
		)
		if err := rows.Scan(&dropID, &dropName, &lteMultiplier, &lteProbability, &itemID, &itemName, &itemIcon, &chance, &ldeMultiplier); err != nil {
			return 0, "", nil, fmt.Errorf("scan loot row: %w", err)
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
		return 0, "", nil, err
	}

	drops := make([]LootDrop, 0, len(dropOrder))
	for _, id := range dropOrder {
		drops = append(drops, *dropMap[id])
	}
	return ltID, ltName, drops, nil
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
  s.formula1,  s.formula2,  s.formula3,  s.formula4,  s.formula5,  s.formula6,
  s.formula7,  s.formula8,  s.formula9,  s.formula10, s.formula11, s.formula12,
  s.classes1,  s.classes2,  s.classes3,  s.classes4,  s.classes5,
  s.classes6,  s.classes7,  s.classes8,  s.classes9,  s.classes10,
  s.classes11, s.classes12, s.classes13, s.classes14, s.classes15,
  s.icon, s.new_icon, s.IsDiscipline, s.suspendable, s.nodispell,
  COALESCE(s.zonetype, 0),
  COALESCE(s.goodEffect, 0),
  COALESCE(s.ResistDiff, 0), COALESCE(s.no_partial_resist, 0),
  COALESCE(s.resist_per_level, 0), COALESCE(s.resist_cap, 0),
  COALESCE(s.AEDuration, 0)`

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
		&sp.EffectFormulas[0], &sp.EffectFormulas[1], &sp.EffectFormulas[2],
		&sp.EffectFormulas[3], &sp.EffectFormulas[4], &sp.EffectFormulas[5],
		&sp.EffectFormulas[6], &sp.EffectFormulas[7], &sp.EffectFormulas[8],
		&sp.EffectFormulas[9], &sp.EffectFormulas[10], &sp.EffectFormulas[11],
		&sp.ClassLevels[0], &sp.ClassLevels[1], &sp.ClassLevels[2],
		&sp.ClassLevels[3], &sp.ClassLevels[4], &sp.ClassLevels[5],
		&sp.ClassLevels[6], &sp.ClassLevels[7], &sp.ClassLevels[8],
		&sp.ClassLevels[9], &sp.ClassLevels[10], &sp.ClassLevels[11],
		&sp.ClassLevels[12], &sp.ClassLevels[13], &sp.ClassLevels[14],
		&sp.Icon, &sp.NewIcon, &sp.IsDiscipline, &sp.Suspendable, &sp.NoDispell,
		&sp.ZoneType,
		&sp.GoodEffect,
		&sp.ResistDiff, &sp.NoPartialResist,
		&sp.ResistPerLevel, &sp.ResistCap,
		&sp.AEDuration,
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
	// Surface any same-name duplicates collapsed out of list views.
	db.ensureVariants()
	sp.VariantIDs, sp.CanonicalID = db.spellVariants.variantFields(sp.ID)
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
// goodEffectOnly: when true, only beneficial spells (spells_new.goodEffect=1)
// are returned — used by the raid-buff picker so debuffs/nukes don't appear.
func (db *DB) SearchSpells(query string, classIndex, minLevel, maxLevel, limit, offset int, goodEffectOnly bool) (*SearchResult[Spell], error) {
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

	if goodEffectOnly {
		conditions = append(conditions, "COALESCE(s.goodEffect, 0) = 1")
	}

	where := strings.Join(conditions, " AND ")

	// Collapse duplicate-name rows to the canonical one (see variants.go).
	db.ensureVariants()
	if clause := db.spellVariants.excludeNonCanonical("s.id"); clause != "" {
		where += " AND " + clause
	}

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
	for i := range spells {
		spells[i].VariantIDs, spells[i].CanonicalID = db.spellVariants.variantFields(spells[i].ID)
	}
	return &SearchResult[Spell]{Items: spells, Total: total}, nil
}

func collectSpells(rows *sql.Rows) ([]Spell, error) {
	// Non-nil so an empty result marshals to [] not null (see collectItems).
	result := []Spell{}
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
// then by spell ID. Empty-name spells are excluded. Disciplines (a LoY-era
// concept) and spells gated above maxLevel — the era's level cap, 60 until
// Planes of Power launches (see internal/era) — are also excluded, since
// neither is obtainable on Quarm.
func (db *DB) GetSpellsByClass(classIndex, maxLevel, limit, offset int) (*SearchResult[Spell], error) {
	if classIndex < 0 || classIndex > 14 {
		return nil, fmt.Errorf("class index %d out of range [0,14]", classIndex)
	}
	col := fmt.Sprintf("s.classes%d", classIndex+1)
	whereClause := fmt.Sprintf("%s BETWEEN 1 AND %d AND s.IsDiscipline = 0 AND s.name != ''", col, maxLevel)

	// Collapse duplicate-name rows to the canonical one (see variants.go).
	db.ensureVariants()
	if clause := db.spellVariants.excludeNonCanonical("s.id"); clause != "" {
		whereClause += " AND " + clause
	}

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
	for i := range spells {
		spells[i].VariantIDs, spells[i].CanonicalID = db.spellVariants.variantFields(spells[i].ID)
	}
	return &SearchResult[Spell]{Items: spells, Total: total}, nil
}

// GetSpellCrossRefs returns items that reference the given spell ID, split into
// scroll items (which teach the spell) and effect items (click/worn/proc/focus).
func (db *DB) GetSpellCrossRefs(spellID int) (*SpellCrossRefs, error) {
	// Collapse duplicate-name item rows to their canonical row, matching the
	// items list/search. Without this the same scroll (e.g. "Spell:
	// Mesmerization") shows up once per duplicate id. See variants.go.
	db.ensureVariants()
	exclude := db.itemVariants.excludeNonCanonical("id")
	scrollWhere := "scrolleffect = ?"
	if exclude != "" {
		scrollWhere += " AND " + exclude
	}
	scrollRows, err := db.Query(
		"SELECT id, name, icon FROM items WHERE "+scrollWhere+" ORDER BY name",
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

	effectQuery := `
		SELECT effect_type, id, name, icon FROM (
			SELECT 'click' AS effect_type, id, name, icon FROM items WHERE clickeffect = ?
			UNION
			SELECT 'worn', id, name, icon FROM items WHERE worneffect = ?
			UNION
			SELECT 'proc', id, name, icon FROM items WHERE proceffect = ?
			UNION
			SELECT 'focus', id, name, icon FROM items WHERE focuseffect = ?
		)`
	if exclude != "" {
		effectQuery += " WHERE " + exclude
	}
	effectQuery += " ORDER BY effect_type, name"
	effectRows, err := db.Query(effectQuery, spellID, spellID, spellID, spellID)
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

// GetSpellVendorOptions returns, for each requested spell id, every vendor/zone
// pair where a scroll teaching that spell can be bought. A spell maps to its
// scroll via items.scrolleffect; the scroll maps to vendors via merchantlist,
// and each vendor to its zone(s) via the spawn chain. Rows are collapsed to one
// per (spell, zone, vendor) — taking the cheapest scroll variant and one
// representative spawn point — and vendors with no resolvable spawn (no zone)
// are dropped, since the route can't direct a player there.
//
// This is the batch input for the shopping-route optimizer (internal/shoproute).
func (db *DB) GetSpellVendorOptions(spellIDs []int) ([]SpellVendorOption, error) {
	if len(spellIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(spellIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(spellIDs))
	for i, id := range spellIDs {
		args[i] = id
	}

	rows, err := db.Query(`
		SELECT i.scrolleffect AS spell_id,
		       sn.name        AS spell_name,
		       s2.zone        AS zone_short,
		       COALESCE(z.long_name, s2.zone) AS zone_name,
		       n.id           AS vendor_id,
		       n.name         AS vendor_name,
		       MIN(i.price)   AS price,
		       MIN(s2.x)      AS x,
		       MIN(s2.y)      AS y
		FROM items i
		JOIN spells_new sn ON sn.id = i.scrolleffect
		JOIN merchantlist ml ON ml.item = i.id
		JOIN npc_types n ON n.merchant_id = ml.merchantid AND n.merchant_id > 0
		JOIN spawnentry se ON se.npcid = n.id
		JOIN spawngroup sg ON sg.id = se.spawngroupid
		JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		LEFT JOIN zone z ON z.short_name = s2.zone
		WHERE i.scrolleffect IN (`+placeholders+`)
		  AND s2.zone IS NOT NULL AND s2.zone != ''
		GROUP BY i.scrolleffect, s2.zone, n.id
		ORDER BY i.scrolleffect, zone_name, vendor_name`, args...)
	if err != nil {
		return nil, fmt.Errorf("get spell vendor options: %w", err)
	}
	defer rows.Close()

	var out []SpellVendorOption
	for rows.Next() {
		var o SpellVendorOption
		if err := rows.Scan(&o.SpellID, &o.SpellName, &o.ZoneShort, &o.ZoneName,
			&o.VendorID, &o.VendorName, &o.Price, &o.X, &o.Y); err != nil {
			return nil, fmt.Errorf("scan spell vendor option: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetZoneAdjacency returns an undirected zone-connectivity graph derived from
// zone_points: a map of zone short_name → the short_names of zones reachable by
// a single zone line. Each line yields both directions so the graph can be
// walked back and forth. Self-loops and unresolved targets are skipped. Used by
// the shopping-route optimizer to order stops from a starting zone.
func (db *DB) GetZoneAdjacency() (map[string][]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT zp.zone AS src, z.short_name AS dst
		FROM zone_points zp
		JOIN zone z ON z.zoneidnumber = zp.target_zone_id
		WHERE zp.target_zone_id > 0
		  AND zp.zone IS NOT NULL AND zp.zone != ''
		  AND z.short_name != zp.zone`)
	if err != nil {
		return nil, fmt.Errorf("get zone adjacency: %w", err)
	}
	defer rows.Close()

	// Dedup edges with a set per node so undirected mirrors don't double up.
	seen := map[string]map[string]bool{}
	add := func(a, b string) {
		if seen[a] == nil {
			seen[a] = map[string]bool{}
		}
		seen[a][b] = true
	}
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err != nil {
			return nil, fmt.Errorf("scan zone edge: %w", err)
		}
		add(src, dst)
		add(dst, src)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	adj := make(map[string][]string, len(seen))
	for node, neighbors := range seen {
		list := make([]string, 0, len(neighbors))
		for n := range neighbors {
			list = append(list, n)
		}
		adj[node] = list
	}
	return adj, nil
}

// GetTeleportDestinations returns the zone short_names a Druid or Wizard can
// deliberately port a group to — spells whose first effect is Teleport (SPA 83)
// or Translocate (SPA 104). Succor/Evacuate (SPA 88) are excluded: those are
// one-way escapes to a fixed safe spot, not travel a player would choose for a
// shopping trip. Destinations that don't resolve to a real zone (familiar/pet
// pseudo-zones, instanced planes) are dropped via the zone join.
//
// The shopping-route optimizer adds these as cheap edges from the Nexus, where
// most Quarm players bind and can readily catch a port, so portable zones count
// as easy to reach.
func (db *DB) GetTeleportDestinations() ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT z.short_name
		FROM spells_new sp
		JOIN zone z ON z.short_name = sp.teleport_zone
		WHERE sp.effectid1 IN (83, 104)
		  AND sp.teleport_zone IS NOT NULL AND sp.teleport_zone != ''
		  AND (sp.classes6 < 255 OR sp.classes12 < 255)
		ORDER BY z.short_name`)
	if err != nil {
		return nil, fmt.Errorf("get teleport destinations: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var z string
		if err := rows.Scan(&z); err != nil {
			return nil, fmt.Errorf("scan teleport destination: %w", err)
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// ─── Zones ────────────────────────────────────────────────────────────────────

// zoneVisibilityFilter returns a SQL clause restricting `short_name` to the
// allowlist defined in zone_allowlist.go (sourced from PQDI). Returns the
// clause prefixed with the given string (e.g. " AND " or " WHERE ") and the
// matching arg slice; empty if the allowlist is empty.
func zoneVisibilityFilter(prefix string) (string, []any) {
	if len(zoneCatalog) == 0 {
		return "", nil
	}
	args := make([]any, 0, len(zoneCatalog))
	for name := range zoneCatalog {
		args = append(args, name)
	}
	ph := strings.Repeat("?,", len(args))
	return fmt.Sprintf("%sz.short_name IN (%s)", prefix, ph[:len(ph)-1]), args
}

// applyExpansionOverride replaces the raw zone.expansion column with the
// canonical bucket from zoneCatalog. See the comment on zoneCatalog for why.
func applyExpansionOverride(shortName string, raw int) int {
	if exp, ok := zoneCatalog[shortName]; ok {
		return exp
	}
	return raw
}

const zoneColumns = `
  z.id, COALESCE(z.short_name,''), z.long_name, COALESCE(z.file_name,''),
  z.zoneidnumber, z.safe_x, z.safe_y, z.safe_z,
  z.min_level, COALESCE(z.note,''),
  z.castoutdoor, z.hotzone, z.canlevitate, z.canbind,
  COALESCE(z.zone_exp_multiplier, 1.0), z.expansion,
  COALESCE((SELECT MIN(n.level) FROM npc_types n JOIN spawnentry se ON se.npcID = n.id JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID WHERE s2.zone = z.short_name AND n.name != '' AND n.name != '#' AND n.name != '_' AND n.race != 127), 0),
  COALESCE((SELECT MAX(n.level) FROM npc_types n JOIN spawnentry se ON se.npcID = n.id JOIN spawn2 s2 ON s2.spawngroupID = se.spawngroupID WHERE s2.zone = z.short_name AND n.name != '' AND n.name != '#' AND n.name != '_' AND n.race != 127), 0),
  COALESCE(z.pull_limit, 80),
  COALESCE(z.graveyard_id, 0),
  COALESCE(z.graveyard_time, 0),
  COALESCE(gz.id, 0),
  COALESCE(gz.short_name, ''),
  COALESCE(gz.long_name, ''),
  COALESCE(g.x, 0),
  COALESCE(g.y, 0),
  COALESCE(g.z, 0)`

// zoneFrom is the FROM clause that pairs with zoneColumns. It LEFT JOINs
// the graveyard table and the destination zone so a single SELECT returns
// graveyard pop-out info when configured.
const zoneFrom = `
  FROM zone z
  LEFT JOIN graveyard g ON g.id = z.graveyard_id AND z.graveyard_id > 0
  LEFT JOIN zone gz ON gz.zoneidnumber = g.zone_id`

func scanZone(row interface {
	Scan(...any) error
}) (*Zone, error) {
	var z Zone
	var graveyardID, graveyardTime int
	var gyDestID int
	var gyShort, gyLong string
	var gyX, gyY, gyZ float64
	err := row.Scan(
		&z.ID, &z.ShortName, &z.LongName, &z.FileName,
		&z.ZoneIDNumber, &z.SafeX, &z.SafeY, &z.SafeZ,
		&z.MinLevel, &z.Note,
		&z.Outdoor, &z.Hotzone, &z.CanLevitate, &z.CanBind,
		&z.ExpMod, &z.Expansion,
		&z.NPCLevelMin, &z.NPCLevelMax,
		&z.PullLimit,
		&graveyardID, &graveyardTime,
		&gyDestID, &gyShort, &gyLong,
		&gyX, &gyY, &gyZ,
	)
	if err != nil {
		return nil, err
	}
	z.Expansion = applyExpansionOverride(z.ShortName, z.Expansion)
	if graveyardID > 0 && gyDestID > 0 {
		z.Graveyard = &ZoneGraveyard{
			ZoneID:       gyDestID,
			ShortName:    gyShort,
			LongName:     gyLong,
			X:            gyX,
			Y:            gyY,
			Z:            gyZ,
			TimerMinutes: graveyardTime,
		}
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
		"SELECT COUNT(*) FROM (SELECT DISTINCT n.id FROM npc_types n WHERE n.id IN (%s)%s)",
		idSubquery, nonPlayerNPCClause,
	)
	if err := db.QueryRow(countQ, shortName, shortName).Scan(&total); err != nil {
		return nil, fmt.Errorf("count zone npcs: %w", err)
	}

	q := fmt.Sprintf(
		"SELECT %s FROM npc_types n %s WHERE n.id IN (%s)%s ORDER BY n.name LIMIT ? OFFSET ?",
		npcColumns, npcJoin, idSubquery, nonPlayerNPCClause,
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
	q := fmt.Sprintf("SELECT %s %s WHERE z.id = ?", zoneColumns, zoneFrom)
	row := db.QueryRow(q, id)
	z, err := scanZone(row)
	if err != nil {
		return nil, fmt.Errorf("get zone %d: %w", id, err)
	}
	return z, nil
}

// GetZoneByShortName returns the zone matching the given short_name.
func (db *DB) GetZoneByShortName(shortName string) (*Zone, error) {
	q := fmt.Sprintf("SELECT %s %s WHERE z.short_name = ?", zoneColumns, zoneFrom)
	row := db.QueryRow(q, shortName)
	z, err := scanZone(row)
	if err != nil {
		return nil, fmt.Errorf("get zone %q: %w", shortName, err)
	}
	return z, nil
}

// GetZoneByZoneIDNumber returns the zone matching the given zoneidnumber —
// the in-game runtime zone identifier (e.g. 158 = Vex Thal). Distinct from
// zone.id, which is a database primary key; the two never coincide.
// Used to resolve the Zeal MsgPlayer "zone" field to a short_name.
func (db *DB) GetZoneByZoneIDNumber(zoneIDNumber int) (*Zone, error) {
	q := fmt.Sprintf("SELECT %s %s WHERE z.zoneidnumber = ?", zoneColumns, zoneFrom)
	row := db.QueryRow(q, zoneIDNumber)
	z, err := scanZone(row)
	if err != nil {
		return nil, fmt.Errorf("get zone by zoneidnumber %d: %w", zoneIDNumber, err)
	}
	return z, nil
}

// ZoneSearchFilters narrows zone search results. Nil fields mean no filter.
type ZoneSearchFilters struct {
	Expansion *int
}

// SearchZones searches zones by long_name (case-insensitive substring match).
// Expansion filtering is applied in Go against the curated zoneCatalog
// (see applyExpansionOverride) because the raw zone.expansion column doesn't
// match player-visible buckets for several Classic/Kunark dungeons.
func (db *DB) SearchZones(query string, filters ZoneSearchFilters, limit, offset int) (*SearchResult[Zone], error) {
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	hiddenFilter, hiddenArgs := zoneVisibilityFilter(" AND ")

	q := fmt.Sprintf(
		"SELECT %s %s WHERE z.long_name LIKE ? ESCAPE '\\'%s ORDER BY z.long_name",
		zoneColumns, zoneFrom, hiddenFilter,
	)
	queryArgs := append([]any{pattern}, hiddenArgs...)
	rows, err := db.Query(q, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("search zones: %w", err)
	}
	defer rows.Close()

	zones, err := collectZones(rows)
	if err != nil {
		return nil, err
	}

	if filters.Expansion != nil {
		filtered := zones[:0]
		for _, z := range zones {
			if z.Expansion == *filters.Expansion {
				filtered = append(filtered, z)
			}
		}
		zones = filtered
	}

	total := len(zones)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return &SearchResult[Zone]{Items: zones[offset:end], Total: total}, nil
}

// ZoneExpansions returns the distinct expansion IDs surfaced in the zone
// browser, derived from the curated zoneCatalog. Sorted ascending.
func (db *DB) ZoneExpansions() ([]int, error) {
	seen := make(map[int]struct{}, 5)
	for _, exp := range zoneCatalog {
		seen[exp] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for exp := range seen {
		out = append(out, exp)
	}
	sort.Ints(out)
	return out, nil
}

func collectZones(rows *sql.Rows) ([]Zone, error) {
	// Non-nil so an empty result marshals to [] not null (see collectItems).
	result := []Zone{}
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
		c.Expansion = applyExpansionOverride(c.ShortName, c.Expansion)
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

// aaClassMaskOverrides corrects altadv_vars rows whose `classes` bitmask is
// wrong in the source MySQL dump (quarm.db is regenerated from those dumps and
// is never hand-edited, so the correction lives in code).
//
// Keyed by eqmacid → correct classes bitmask (bit N set for 1-indexed class N).
//
//   - 83 Fletching Mastery → 16 (Ranger only). The dump flags it 65534 (all
//     classes), so druids and everyone else wrongly saw it (issue #134). It's
//     a Ranger archery AA: eqmacid 82/83/84 are Archery Mastery / Fletching
//     Mastery / Endless Quiver, and 82 & 84 both correctly read classes=16.
var aaClassMaskOverrides = map[int]int{
	83: 16, // Fletching Mastery → Ranger
}

// effectiveAAClasses returns the class bitmask to use for an AA, applying any
// known correction from aaClassMaskOverrides.
func effectiveAAClasses(eqmacid, classes int) int {
	if c, ok := aaClassMaskOverrides[eqmacid]; ok {
		return c
	}
	return classes
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
//
// The class bitmask filter is applied in Go (not SQL) so aaClassMaskOverrides
// can correct rows whose `classes` value is wrong in the source dump.
func (db *DB) ListAvailableAAs(class int) ([]AAInfo, error) {
	if class < 1 || class > 15 {
		return []AAInfo{}, nil
	}
	mask := 1 << class
	rows, err := db.Query(
		`SELECT eqmacid, name, cost, cost_inc, max_level, type, classes
		 FROM altadv_vars
		 WHERE name != 'NOT USED'
		   AND cost > 0
		   AND eqmacid > 0
		   AND class_type != 0
		 ORDER BY eqmacid`,
	)
	if err != nil {
		return nil, fmt.Errorf("list available aas: %w", err)
	}
	defer rows.Close()

	byName := make(map[string]AAInfo)
	for rows.Next() {
		var info AAInfo
		var classes int
		if err := rows.Scan(&info.AAID, &info.Name, &info.Cost, &info.CostInc, &info.MaxLevel, &info.Type, &classes); err != nil {
			return nil, fmt.Errorf("scan aa: %w", err)
		}
		if effectiveAAClasses(info.AAID, classes)&mask == 0 {
			continue
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

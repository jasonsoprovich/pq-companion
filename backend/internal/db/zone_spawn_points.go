package db

import (
	"fmt"
	"strings"
)

// defaultAggroRadius is the aggro range the server gives an NPC whose
// aggroradius is 0 (EQMacEmu Mob::AI_Init).
const defaultAggroRadius = 70

// ZoneSpawnPoint is one spawn2 point in a zone with the NPCs it can roll, for
// the map's spawn and aggro-ring layer.
type ZoneSpawnPoint struct {
	SpawnID int `json:"spawn_id"`
	// X, Y and Z are map space (negated from spawn2), like every other
	// coordinate the renderer receives.
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
	// Heading is where a mob faces as it spawns, EQ's 0-512.
	Heading float64         `json:"heading"`
	NPCs    []SpawnPointNPC `json:"npcs"`
}

// NPCLook is how an NPC is drawn: its race model and, on player-race
// models, what it wears (as the server sends it, NPC::GetEquipmentMaterial).
type NPCLook struct {
	Race   int `json:"race"`
	Gender int `json:"gender"`
	// Texture picks the model's texture set; on player races it's the
	// armor material.
	Texture int `json:"texture"`
	// Mats and Tints are the material and tint (0xRRGGBB) per armor slot:
	// head, chest, arms, wrist, hands, legs, feet.
	Mats  [7]int    `json:"mats"`
	Tints [7]uint32 `json:"tints"`
	// Size is its height in game units; BaseSize the size most NPCs of its
	// race have, which its model is taken to be made at. 0 for unknown.
	Size     float64 `json:"size"`
	BaseSize float64 `json:"base_size"`
	// Primary and Secondary are the held items' models ("IT38"), or "".
	Primary   string `json:"primary,omitempty"`
	Secondary string `json:"secondary,omitempty"`
	// Face and the Luclin hair, beard and eyes, as mapgen.Looks has them
	// (-1 for no beard, or the default colour).
	Face       int `json:"face"`
	Hair       int `json:"hair"`
	HairColor  int `json:"hair_color"`
	Beard      int `json:"beard"`
	BeardColor int `json:"beard_color"`
	Eyes       int `json:"eyes"`
}

// SpawnPointNPC is one NPC a spawn point can roll.
type SpawnPointNPC struct {
	NPCID int    `json:"npc_id"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	// MaxLevel is the top of the NPC's spawn level range (0 = fixed level).
	MaxLevel int `json:"max_level,omitempty"`
	// AggroRadius is the effective aggro range: npc_types.aggroradius, or
	// the server's default when that is 0.
	AggroRadius int `json:"aggro_radius"`
	// SeeInvis and SeeInvisUndead mirror npc_types: 0 = no, 1 = always, and
	// a higher value is the percent chance, rolled at spawn, that it does.
	SeeInvis       int  `json:"see_invis"`
	SeeInvisUndead int  `json:"see_invis_undead"`
	Undead         bool `json:"undead"`
	AggroPC        bool `json:"aggro_pc"`
	// Pct is the NPC's share of the spawn group's roll.
	Pct float64 `json:"pct"`
	// Look is how it's drawn.
	Look NPCLook `json:"look"`
}

// bodyTypeUndead is EQMacEmu BodyType::Undead.
const bodyTypeUndead = 3

// GetZoneSpawnPoints returns every spawn point active in the current era,
// each with the NPCs it can roll (chance > 0). Placeholder and Invisible Man
// rows are left out (nonPlayerNPCClause): they are script anchors and
// decoys, not mobs a player meets.
func (db *DB) GetZoneSpawnPoints(shortName string) ([]ZoneSpawnPoint, error) {
	rows, err := db.Query(`
		WITH pts AS (
			SELECT s2.id, s2.spawngroupID, s2.x, s2.y, s2.z, s2.heading
			FROM spawn2 s2
			WHERE s2.zone = ? AND `+activeContentClause("s2")+`
		),
		ents AS (
			SELECT se.spawngroupID, se.npcID, se.chance
			FROM spawnentry se
			WHERE se.spawngroupID IN (SELECT spawngroupID FROM pts)
			  AND se.chance > 0
			  AND `+activeContentClause("se")+`
		),
		tot AS (
			SELECT spawngroupID, SUM(chance) AS t FROM ents GROUP BY spawngroupID
		)
		SELECT p.id, p.x, p.y, p.z, p.heading,
		       n.id, n.name, n.level, n.maxlevel, n.aggroradius,
		       n.see_invis, n.see_invis_undead, n.bodytype, n.aggro_pc,
		       e.chance * 100.0 / tot.t,
		       n.race, n.gender, n.texture, n.helmtexture, n.size,
		       n.chesttexture, n.armtexture, n.bracertexture, n.handtexture,
		       n.legtexture, n.feettexture,
		       n.armortint_red, n.armortint_green, n.armortint_blue,
		       COALESCE(t.red1h, -1), COALESCE(t.grn1h, 0), COALESCE(t.blu1h, 0),
		       COALESCE(t.red2c, 0), COALESCE(t.grn2c, 0), COALESCE(t.blu2c, 0),
		       COALESCE(t.red3a, 0), COALESCE(t.grn3a, 0), COALESCE(t.blu3a, 0),
		       COALESCE(t.red4b, 0), COALESCE(t.grn4b, 0), COALESCE(t.blu4b, 0),
		       COALESCE(t.red5g, 0), COALESCE(t.grn5g, 0), COALESCE(t.blu5g, 0),
		       COALESCE(t.red6l, 0), COALESCE(t.grn6l, 0), COALESCE(t.blu6l, 0),
		       COALESCE(t.red7f, 0), COALESCE(t.grn7f, 0), COALESCE(t.blu7f, 0),
		       n.d_melee_texture1, n.d_melee_texture2,
		       n.face, n.luclin_hairstyle, n.luclin_haircolor, n.luclin_beard,
		       n.luclin_beardcolor, n.luclin_eyecolor
		FROM pts p
		JOIN ents e ON e.spawngroupID = p.spawngroupID
		JOIN tot ON tot.spawngroupID = p.spawngroupID
		JOIN npc_types n ON n.id = e.npcID
		LEFT JOIN npc_types_tint t ON t.id = n.armortint_id AND n.armortint_id > 0
		WHERE 1 = 1`+nonPlayerNPCClause+`
		ORDER BY p.id, e.chance DESC, n.id`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone spawn points %q: %w", shortName, err)
	}
	defer rows.Close()

	out := []ZoneSpawnPoint{}
	races := map[int]bool{}
	for rows.Next() {
		var (
			id                int
			x, y, z, heading  float64
			n                 SpawnPointNPC
			bodyType, aggroPC int
			helm              int
			slotTex           [6]int
			tintRGB           [3]int
			set               [21]int
			melee1, melee2    int
		)
		l := &n.Look
		if err := rows.Scan(&id, &x, &y, &z, &heading,
			&n.NPCID, &n.Name, &n.Level, &n.MaxLevel, &n.AggroRadius,
			&n.SeeInvis, &n.SeeInvisUndead, &bodyType, &aggroPC, &n.Pct,
			&l.Race, &l.Gender, &l.Texture, &helm, &l.Size,
			&slotTex[0], &slotTex[1], &slotTex[2], &slotTex[3], &slotTex[4], &slotTex[5],
			&tintRGB[0], &tintRGB[1], &tintRGB[2],
			&set[0], &set[1], &set[2], &set[3], &set[4], &set[5], &set[6],
			&set[7], &set[8], &set[9], &set[10], &set[11], &set[12], &set[13],
			&set[14], &set[15], &set[16], &set[17], &set[18], &set[19], &set[20],
			&melee1, &melee2,
			&l.Face, &l.Hair, &l.HairColor, &l.Beard, &l.BeardColor, &l.Eyes); err != nil {
			return nil, fmt.Errorf("scan zone spawn point: %w", err)
		}
		dressNPC(l, helm, slotTex, tintRGB, set, melee1, melee2)
		races[l.Race] = true
		if n.AggroRadius <= 0 {
			n.AggroRadius = defaultAggroRadius
		}
		n.Undead = bodyType == bodyTypeUndead
		n.AggroPC = aggroPC != 0
		if len(out) == 0 || out[len(out)-1].SpawnID != id {
			// The same negation the geometry pipeline and every POI applies.
			out = append(out, ZoneSpawnPoint{SpawnID: id, X: int(-x), Y: int(-y), Z: int(z), Heading: heading})
		}
		p := &out[len(out)-1]
		p.NPCs = append(p.NPCs, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	base, err := db.raceBaseSizes(races)
	if err != nil {
		return nil, err
	}
	for i := range out {
		for j := range out[i].NPCs {
			l := &out[i].NPCs[j].Look
			l.BaseSize = base[l.Race]
		}
	}
	return out, nil
}

// noTexture is the texture an NPC's slots take for "none" (the client's
// 0xFF): shown as bare.
const noTexture = 255

// dressNPC fills in what an NPC wears from its npc_types row: the helm
// texture on the head, each other slot's own texture or else the NPC's
// texture, and the tint set (armortint_id) or else one tint on every slot.
func dressNPC(l *NPCLook, helm int, slotTex [6]int, rgb [3]int, set [21]int, melee1, melee2 int) {
	if l.Texture == noTexture {
		l.Texture = 0
	}
	l.Mats[0] = helm
	for i, t := range slotTex {
		if t == 0 {
			t = l.Texture
		}
		l.Mats[i+1] = t
	}
	for i := range l.Mats {
		if l.Mats[i] == noTexture || l.Mats[i] < 0 {
			l.Mats[i] = 0
		}
	}
	pack := func(r, g, b int) uint32 {
		return uint32(r&0xff)<<16 | uint32(g&0xff)<<8 | uint32(b&0xff)
	}
	if set[0] >= 0 {
		for i := range l.Tints {
			l.Tints[i] = pack(set[3*i], set[3*i+1], set[3*i+2])
		}
	} else if c := pack(rgb[0], rgb[1], rgb[2]); c != 0 {
		for i := range l.Tints {
			l.Tints[i] = c
		}
	}
	// 0xFF is "not set": the model's own face and hair, no beard.
	unset := func(v *int, def, hi int) {
		if *v < 0 || *v > hi {
			*v = def
		}
	}
	unset(&l.Face, 0, 7)
	unset(&l.Hair, 0, 9)
	unset(&l.HairColor, -1, 23)
	unset(&l.Beard, -1, 9)
	unset(&l.BeardColor, -1, 23)
	unset(&l.Eyes, 0, 10)
	if melee1 > 0 {
		l.Primary = fmt.Sprintf("IT%d", melee1)
	}
	if melee2 > 0 {
		l.Secondary = fmt.Sprintf("IT%d", melee2)
	}
}

// raceBaseSizes is the size most NPCs of each race have, which their model
// is taken to be made at.
func (db *DB) raceBaseSizes(races map[int]bool) (map[int]float64, error) {
	out := map[int]float64{}
	if len(races) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(races))
	marks := make([]string, 0, len(races))
	for r := range races {
		args = append(args, r)
		marks = append(marks, "?")
	}
	rows, err := db.Query(`
		SELECT race, size, COUNT(*) AS c FROM npc_types
		WHERE size > 0 AND race IN (`+strings.Join(marks, ",")+`)
		GROUP BY race, size
		ORDER BY race, c DESC, size`, args...)
	if err != nil {
		return nil, fmt.Errorf("race base sizes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			race  int
			size  float64
			count int
		)
		if err := rows.Scan(&race, &size, &count); err != nil {
			return nil, fmt.Errorf("scan race base size: %w", err)
		}
		if _, ok := out[race]; !ok {
			out[race] = size
		}
	}
	return out, rows.Err()
}

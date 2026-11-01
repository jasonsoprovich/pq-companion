package db

// PetNPC identifies a summoned pet's npc_types row by the live attributes the
// Zeal pipe reports for a target, so a pet the player targets can be matched
// when its summoning spell wasn't seen. Texture is what separates a
// magician's four elementals (same level, race 75 and class; air 3, earth 0,
// fire 1, water 2).
type PetNPC struct {
	NPCID   int
	Level   int
	Race    int
	Class   int
	Texture int
}

// petSummonSPAs are the spell effects that create the caster's controllable
// pet (the one Zeal reports as pet_id): 33 Summon Pet (magician elementals,
// shaman/beastlord spirits, enchanter animations), 71 Necromancer pet, 106
// Beastlord warder. Familiars (108), Eye of Zomm (67) and swarm pets (152)
// are not the player's pet and are left out.
const petSummonSPAs = "33, 71, 106"

// PetSummons maps every pet-summoning spell name to its pet's npc_types id
// (spells_new.teleport_zone names a pets.type row, whose npcID is the pet),
// and lists those pet rows for attribute matching. Quarm's pets table has
// effectively no pet-power variants (one petpower 0 row against 139 at -1),
// so a spell with several rows takes the lowest npcID. Read-only data,
// loaded once at startup.
func (d *DB) PetSummons() (bySpell map[string]int, rows []PetNPC, err error) {
	const q = `
		SELECT s.name, MIN(p.npcID), n.level, n.race, n.class, n.texture
		FROM spells_new s
		JOIN pets p ON p.type = s.teleport_zone
		JOIN npc_types n ON n.id = p.npcID
		WHERE s.effectid1 IN (` + petSummonSPAs + `)
		  AND s.name IS NOT NULL AND s.name != ''
		GROUP BY s.name
	`
	r, err := d.Query(q)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()

	bySpell = make(map[string]int)
	seen := make(map[int]bool)
	for r.Next() {
		var name string
		var p PetNPC
		if err := r.Scan(&name, &p.NPCID, &p.Level, &p.Race, &p.Class, &p.Texture); err != nil {
			return nil, nil, err
		}
		bySpell[name] = p.NPCID
		if !seen[p.NPCID] {
			seen[p.NPCID] = true
			rows = append(rows, p)
		}
	}
	return bySpell, rows, r.Err()
}

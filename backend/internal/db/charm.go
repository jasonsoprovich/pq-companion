package db

import "fmt"

// CharmCandidate is the slim NPC row the charm pet finder needs: enough to
// classify charmability, scale stats across the spawn level range, and run the
// resist check. It carries attack_delay (absent from the general NPC struct and
// npcColumns) and the raw special_abilities string for immunity/summon parsing.
type CharmCandidate struct {
	ID               int
	Name             string
	Level            int
	MaxLevel         int
	Class            int
	BodyType         int
	HP               int
	MinDmg           int
	MaxDmg           int
	AttackDelay      int
	MR               int
	CR               int
	FR               int
	DR               int
	PR               int
	SpecialAbilities string
	NPCSpellsID      int
}

// CharmCandidatesByZone returns every distinct, non-placeholder, real-level NPC
// that spawns in the given zone short_name, with the columns the charm pet
// finder needs. It follows the same spawn chain as GetNPCsByZone (the spawnentry
// join plus direct solo-spawn entries) but selects a charm-specific column set,
// including attack_delay. Charmability (body type, immunity, level cap) is
// filtered downstream in the api layer against the chosen charm spell.
//
// Bankers and merchants (npc class 40/41) are excluded here: they're non-combat
// service NPCs the game won't let you charm, so they'd only be noise in a
// best-pet list.
func (db *DB) CharmCandidatesByZone(shortName string) ([]CharmCandidate, error) {
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

	q := fmt.Sprintf(`
		SELECT n.id, n.name, n.level, n.maxlevel, n.class, n.bodytype,
		       n.hp, n.mindmg, n.maxdmg, n.attack_delay,
		       n.MR, n.CR, n.FR, n.DR, n.PR,
		       COALESCE(n.special_abilities, ''), n.npc_spells_id
		FROM npc_types n
		WHERE n.id IN (%s)%s AND n.level > 0
		  AND n.class NOT IN (40, 41)
		ORDER BY n.level DESC, n.name`, idSubquery, nonPlayerNPCClause)

	rows, err := db.Query(q, shortName, shortName)
	if err != nil {
		return nil, fmt.Errorf("query charm candidates for zone %q: %w", shortName, err)
	}
	defer rows.Close()

	out := []CharmCandidate{}
	for rows.Next() {
		var c CharmCandidate
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Level, &c.MaxLevel, &c.Class, &c.BodyType,
			&c.HP, &c.MinDmg, &c.MaxDmg, &c.AttackDelay,
			&c.MR, &c.CR, &c.FR, &c.DR, &c.PR,
			&c.SpecialAbilities, &c.NPCSpellsID,
		); err != nil {
			return nil, fmt.Errorf("scan charm candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

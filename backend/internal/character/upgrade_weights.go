package character

import "database/sql"

// migrateUpgradeWeights creates the per-character gear-upgrade weight table.
// One row per character holds the user-tuned scoring weights as JSON; absence
// of a row means "use the class default preset". Stored as opaque JSON so this
// package stays decoupled from the upgrade scoring types.
func (s *Store) migrateUpgradeWeights() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_upgrade_weights (
			character_id INTEGER PRIMARY KEY,
			weights_json TEXT NOT NULL,
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`)
	return err
}

// GetUpgradeWeights returns the stored weight JSON for a character. ok is false
// when the character has no saved weights (caller should fall back to the class
// default preset).
func (s *Store) GetUpgradeWeights(characterID int) (weightsJSON string, ok bool, err error) {
	row := s.db.QueryRow(
		`SELECT weights_json FROM character_upgrade_weights WHERE character_id = ?`,
		characterID,
	)
	switch err := row.Scan(&weightsJSON); err {
	case nil:
		return weightsJSON, true, nil
	case sql.ErrNoRows:
		return "", false, nil
	default:
		return "", false, err
	}
}

// SetUpgradeWeights upserts a character's tuned weight JSON.
func (s *Store) SetUpgradeWeights(characterID int, weightsJSON string) error {
	_, err := s.db.Exec(`
		INSERT INTO character_upgrade_weights (character_id, weights_json)
		VALUES (?, ?)
		ON CONFLICT(character_id) DO UPDATE SET weights_json = excluded.weights_json`,
		characterID, weightsJSON,
	)
	return err
}

// DeleteUpgradeWeights removes a character's tuned weights, resetting them to
// the class default on the next read.
func (s *Store) DeleteUpgradeWeights(characterID int) error {
	_, err := s.db.Exec(
		`DELETE FROM character_upgrade_weights WHERE character_id = ?`,
		characterID,
	)
	return err
}

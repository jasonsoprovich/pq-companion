package trigger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ActionTemplate is a named, reusable Actions list. Users save the actions
// of a trigger they like (e.g. "buff fading sound" or "big red overlay +
// TTS") and apply it to other triggers from the editor's Templates menu or
// the bulk editor. At most one template is the default — its actions
// prefill newly created triggers.
type ActionTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Actions   []Action  `json:"actions"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrTemplateNotFound is returned when a requested template doesn't exist.
var ErrTemplateNotFound = fmt.Errorf("action template not found")

// ListActionTemplates returns all templates, default first, then by name.
func (s *Store) ListActionTemplates() ([]*ActionTemplate, error) {
	rows, err := s.db.Query(
		`SELECT id, name, actions, is_default, created_at
		 FROM action_templates ORDER BY is_default DESC, name COLLATE NOCASE ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list action templates: %w", err)
	}
	defer rows.Close()
	var out []*ActionTemplate
	for rows.Next() {
		t, err := scanActionTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetActionTemplate returns one template by id, or ErrTemplateNotFound.
func (s *Store) GetActionTemplate(id string) (*ActionTemplate, error) {
	row := s.db.QueryRow(
		`SELECT id, name, actions, is_default, created_at
		 FROM action_templates WHERE id = ?`, id,
	)
	t, err := scanActionTemplate(row)
	if err == sql.ErrNoRows {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get action template %s: %w", id, err)
	}
	return t, nil
}

// CreateActionTemplate saves a new template, assigning its ID and timestamp.
// Setting IsDefault clears the flag on every other template.
func (s *Store) CreateActionTemplate(t *ActionTemplate) error {
	id, err := NewID()
	if err != nil {
		return err
	}
	t.ID = id
	t.CreatedAt = time.Now().UTC()
	if t.Actions == nil {
		t.Actions = []Action{}
	}
	blob, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal template actions: %w", err)
	}
	if t.IsDefault {
		if err := s.clearDefaultTemplate(); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO action_templates (id, name, actions, is_default, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.Name, string(blob), boolToInt(t.IsDefault), t.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert action template: %w", err)
	}
	return nil
}

// UpdateActionTemplate saves changes to an existing template. Setting
// IsDefault clears the flag on every other template.
func (s *Store) UpdateActionTemplate(t *ActionTemplate) error {
	if t.Actions == nil {
		t.Actions = []Action{}
	}
	blob, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal template actions: %w", err)
	}
	if t.IsDefault {
		if err := s.clearDefaultTemplate(); err != nil {
			return err
		}
	}
	res, err := s.db.Exec(
		`UPDATE action_templates SET name = ?, actions = ?, is_default = ? WHERE id = ?`,
		t.Name, string(blob), boolToInt(t.IsDefault), t.ID,
	)
	if err != nil {
		return fmt.Errorf("update action template: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// DeleteActionTemplate removes a template.
func (s *Store) DeleteActionTemplate(id string) error {
	res, err := s.db.Exec(`DELETE FROM action_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete action template: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (s *Store) clearDefaultTemplate() error {
	if _, err := s.db.Exec(`UPDATE action_templates SET is_default = 0 WHERE is_default = 1`); err != nil {
		return fmt.Errorf("clear default template: %w", err)
	}
	return nil
}

func scanActionTemplate(row scanner) (*ActionTemplate, error) {
	var t ActionTemplate
	var actJSON string
	var defaultInt int
	var unixSec int64
	if err := row.Scan(&t.ID, &t.Name, &actJSON, &defaultInt, &unixSec); err != nil {
		return nil, err
	}
	t.IsDefault = defaultInt != 0
	t.CreatedAt = time.Unix(unixSec, 0).UTC()
	if err := json.Unmarshal([]byte(actJSON), &t.Actions); err != nil {
		t.Actions = []Action{}
	}
	if t.Actions == nil {
		t.Actions = []Action{}
	}
	return &t, nil
}

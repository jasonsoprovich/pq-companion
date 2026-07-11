package trigger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TimerGroup is a user-created Custom Timers window, letting raid leaders
// split signature-spell/boss timers into their own overlay separate from
// general trigger timers. Referenced by Trigger.CustomGroupID. An empty
// CustomGroupID means the original/default Custom Timers window — every
// install has that implicitly, with no TimerGroup row required for it.
type TimerGroup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Count     int       `json:"count"` // triggers currently assigned to this group
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// TimerGroup sentinel errors, mapped to HTTP status codes by the API layer.
var (
	ErrTimerGroupNameEmpty = errors.New("timer group name is required")
	ErrTimerGroupExists    = errors.New("timer group name already exists")
	ErrTimerGroupNotFound  = errors.New("timer group not found")
)

func normalizeTimerGroupName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", ErrTimerGroupNameEmpty
	}
	return n, nil
}

// ListTimerGroups returns every user-created timer group ordered by
// SortOrder then name, each with its current trigger count. The implicit
// default group (CustomGroupID == "") is not included — the frontend
// renders it separately as the original "Custom Timers" window.
func (s *Store) ListTimerGroups() ([]TimerGroup, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.name, g.sort_order, g.created_at,
		        (SELECT COUNT(*) FROM triggers WHERE custom_group_id = g.id) AS cnt
		 FROM trigger_timer_groups g
		 ORDER BY g.sort_order ASC, g.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list timer groups: %w", err)
	}
	defer rows.Close()
	var out []TimerGroup
	for rows.Next() {
		var g TimerGroup
		var unixSec int64
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &unixSec, &g.Count); err != nil {
			return nil, err
		}
		g.CreatedAt = time.Unix(unixSec, 0).UTC()
		out = append(out, g)
	}
	return out, rows.Err()
}

// timerGroupNameTaken reports whether name is already used by another group.
func (s *Store) timerGroupNameTaken(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM trigger_timer_groups WHERE name = ?`, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check timer group name: %w", err)
	}
	return n > 0, nil
}

func (s *Store) nextTimerGroupSortOrder() (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(sort_order) FROM trigger_timer_groups`).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("next timer group sort order: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// CreateTimerGroup persists a new, empty custom timer window.
func (s *Store) CreateTimerGroup(name string) (TimerGroup, error) {
	norm, err := normalizeTimerGroupName(name)
	if err != nil {
		return TimerGroup{}, err
	}
	taken, err := s.timerGroupNameTaken(norm)
	if err != nil {
		return TimerGroup{}, err
	}
	if taken {
		return TimerGroup{}, ErrTimerGroupExists
	}
	id, err := NewID()
	if err != nil {
		return TimerGroup{}, fmt.Errorf("generate timer group id: %w", err)
	}
	order, err := s.nextTimerGroupSortOrder()
	if err != nil {
		return TimerGroup{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO trigger_timer_groups (id, name, sort_order, created_at) VALUES (?, ?, ?, ?)`,
		id, norm, order, now.Unix(),
	); err != nil {
		return TimerGroup{}, fmt.Errorf("create timer group %s: %w", norm, err)
	}
	return TimerGroup{ID: id, Name: norm, Count: 0, SortOrder: order, CreatedAt: now}, nil
}

// RenameTimerGroup renames a timer group in place. Triggers reference it by
// ID, so no cascade is needed — existing Electron window bounds/lock config
// (also keyed by ID) are unaffected by a rename.
func (s *Store) RenameTimerGroup(id, newName string) error {
	norm, err := normalizeTimerGroupName(newName)
	if err != nil {
		return err
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM trigger_timer_groups WHERE id = ?`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check timer group %s: %w", id, err)
	}
	if exists == 0 {
		return ErrTimerGroupNotFound
	}
	taken, err := s.timerGroupNameTaken(norm)
	if err != nil {
		return err
	}
	if taken {
		var sameID string
		_ = s.db.QueryRow(`SELECT id FROM trigger_timer_groups WHERE name = ?`, norm).Scan(&sameID)
		if sameID != id {
			return ErrTimerGroupExists
		}
	}
	if _, err := s.db.Exec(`UPDATE trigger_timer_groups SET name = ? WHERE id = ?`, norm, id); err != nil {
		return fmt.Errorf("rename timer group %s: %w", id, err)
	}
	return nil
}

// DeleteTimerGroup removes a timer group and reassigns any triggers that
// referenced it back to the default Custom Timers window (CustomGroupID =
// ""). Unlike DeleteCategory, there is no "delete triggers too" option — a
// timer group is just a display window, and removing it should never delete
// the underlying triggers.
func (s *Store) DeleteTimerGroup(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete timer group: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.Exec(`DELETE FROM trigger_timer_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete timer group %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTimerGroupNotFound
	}
	if _, err := tx.Exec(`UPDATE triggers SET custom_group_id = '' WHERE custom_group_id = ?`, id); err != nil {
		return fmt.Errorf("reassign timer group %s triggers: %w", id, err)
	}
	return tx.Commit()
}

// ReorderTimerGroups rewrites display order to match the position of each
// ID in order (0-based). Unknown/blank IDs are skipped.
func (s *Store) ReorderTimerGroups(order []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder timer groups: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for i, id := range order {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := tx.Exec(`UPDATE trigger_timer_groups SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return fmt.Errorf("reorder timer group %s: %w", id, err)
		}
	}
	return tx.Commit()
}

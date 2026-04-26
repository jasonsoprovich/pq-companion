package character

import (
	"database/sql"
	"fmt"
	"time"
)

// Task is a manual to-do entry attached to a character.
type Task struct {
	ID          int       `json:"id"`
	CharacterID int       `json:"character_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Position    int       `json:"position"`
	Completed   bool      `json:"completed"`
	CreatedAt   int64     `json:"created_at"`
	Subtasks    []Subtask `json:"subtasks"`
}

// Subtask is a checkbox item nested under a Task.
type Subtask struct {
	ID        int    `json:"id"`
	TaskID    int    `json:"task_id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
	Position  int    `json:"position"`
}

func (s *Store) migrateTasks() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_tasks (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			name         TEXT    NOT NULL,
			description  TEXT    NOT NULL DEFAULT '',
			position     INTEGER NOT NULL DEFAULT 0,
			completed    INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL,
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_tasks_char_pos ON character_tasks(character_id, position)`,
	); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_task_subtasks (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id   INTEGER NOT NULL,
			name      TEXT    NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0,
			position  INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (task_id) REFERENCES character_tasks(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_task_subtasks_task_pos ON character_task_subtasks(task_id, position)`,
	); err != nil {
		return err
	}
	return nil
}

// ListTasks returns all tasks for the character ordered by position, with subtasks attached.
func (s *Store) ListTasks(characterID int) ([]Task, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, name, description, position, completed, created_at
		 FROM character_tasks
		 WHERE character_id = ?
		 ORDER BY position, id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	idx := make(map[int]int)
	for rows.Next() {
		var t Task
		var completed int
		if err := rows.Scan(&t.ID, &t.CharacterID, &t.Name, &t.Description, &t.Position, &completed, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Completed = completed != 0
		t.Subtasks = []Subtask{}
		idx[t.ID] = len(tasks)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return tasks, nil
	}

	subRows, err := s.db.Query(
		`SELECT s.id, s.task_id, s.name, s.completed, s.position
		 FROM character_task_subtasks s
		 JOIN character_tasks t ON t.id = s.task_id
		 WHERE t.character_id = ?
		 ORDER BY s.position, s.id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer subRows.Close()
	for subRows.Next() {
		var sub Subtask
		var completed int
		if err := subRows.Scan(&sub.ID, &sub.TaskID, &sub.Name, &completed, &sub.Position); err != nil {
			return nil, err
		}
		sub.Completed = completed != 0
		if i, ok := idx[sub.TaskID]; ok {
			tasks[i].Subtasks = append(tasks[i].Subtasks, sub)
		}
	}
	return tasks, subRows.Err()
}

// CreateTask inserts a new task at the end of the character's list.
func (s *Store) CreateTask(characterID int, name, description string) (Task, error) {
	var maxPos sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MAX(position) FROM character_tasks WHERE character_id = ?`,
		characterID,
	).Scan(&maxPos); err != nil {
		return Task{}, err
	}
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO character_tasks (character_id, name, description, position, completed, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		characterID, name, description, pos, now,
	)
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	id, _ := res.LastInsertId()
	return Task{
		ID:          int(id),
		CharacterID: characterID,
		Name:        name,
		Description: description,
		Position:    pos,
		Completed:   false,
		CreatedAt:   now,
		Subtasks:    []Subtask{},
	}, nil
}

// UpdateTask replaces name/description/completed for a task.
func (s *Store) UpdateTask(id int, name, description string, completed bool) error {
	completedInt := 0
	if completed {
		completedInt = 1
	}
	_, err := s.db.Exec(
		`UPDATE character_tasks SET name=?, description=?, completed=? WHERE id=?`,
		name, description, completedInt, id,
	)
	return err
}

// DeleteTask removes a task and all its subtasks.
func (s *Store) DeleteTask(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM character_task_subtasks WHERE task_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM character_tasks WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReorderTasks rewrites the position of every task for a character to match the
// supplied ID order. Tasks not present in the list are left at the end.
func (s *Store) ReorderTasks(characterID int, orderedIDs []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for i, id := range orderedIDs {
		if _, err := tx.Exec(
			`UPDATE character_tasks SET position=? WHERE id=? AND character_id=?`,
			i, id, characterID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateSubtask inserts a new subtask at the end of a task's subtask list.
func (s *Store) CreateSubtask(taskID int, name string) (Subtask, error) {
	var maxPos sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MAX(position) FROM character_task_subtasks WHERE task_id = ?`,
		taskID,
	).Scan(&maxPos); err != nil {
		return Subtask{}, err
	}
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}
	res, err := s.db.Exec(
		`INSERT INTO character_task_subtasks (task_id, name, completed, position) VALUES (?, ?, 0, ?)`,
		taskID, name, pos,
	)
	if err != nil {
		return Subtask{}, fmt.Errorf("create subtask: %w", err)
	}
	id, _ := res.LastInsertId()
	return Subtask{ID: int(id), TaskID: taskID, Name: name, Completed: false, Position: pos}, nil
}

// UpdateSubtask replaces name/completed for a subtask.
func (s *Store) UpdateSubtask(id int, name string, completed bool) error {
	completedInt := 0
	if completed {
		completedInt = 1
	}
	_, err := s.db.Exec(
		`UPDATE character_task_subtasks SET name=?, completed=? WHERE id=?`,
		name, completedInt, id,
	)
	return err
}

// DeleteSubtask removes a subtask.
func (s *Store) DeleteSubtask(id int) error {
	_, err := s.db.Exec(`DELETE FROM character_task_subtasks WHERE id=?`, id)
	return err
}

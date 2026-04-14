// Package backup manages configuration file backups for PQ Companion.
// Backups are zip archives of EQ *.ini files stored in ~/.pq-companion/backups/.
// Metadata is persisted in ~/.pq-companion/user.db.
package backup

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a requested backup does not exist.
var ErrNotFound = errors.New("backup not found")

// Backup describes a single configuration backup.
type Backup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
	FileCount int       `json:"file_count"`
}

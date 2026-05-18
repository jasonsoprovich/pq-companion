// Package appbackup exports and imports the user-owned portion of the
// application's state — user.db plus every backup zip created by the EQ
// config Backup Manager — as a single ".pqcb" bundle file. It is the
// "move my setup to another device" feature, separate from the in-app
// EQ-config Backup Manager which only protects EQ .ini files.
package appbackup

const (
	// BundleExt is the file extension for export bundles.
	BundleExt = ".pqcb"

	// FormatVersion is the on-disk manifest schema version. Bumped only when
	// the bundle layout changes in a way older app versions wouldn't
	// understand. Import refuses bundles with a higher format version than
	// the running app supports.
	FormatVersion = 1

	// manifestName is the manifest file name inside the bundle zip.
	manifestName = "manifest.json"

	// userDBName is the user.db copy's name inside the bundle.
	userDBName = "user.db"

	// backupsDir is the directory inside the bundle that holds copies of
	// every EQ-config backup zip.
	backupsDir = "backups"
)

// Manifest is the bundle's table of contents. It is serialised to
// manifest.json at the root of the bundle zip.
type Manifest struct {
	FormatVersion int          `json:"format_version"`
	AppVersion    string       `json:"app_version"`
	ExportedAt    string       `json:"exported_at"`
	Files         []FileEntry  `json:"files"`
	Stats         ManifestStat `json:"stats"`
}

// FileEntry records a single file's metadata inside the bundle.
type FileEntry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// ManifestStat summarises bundle contents for quick display in import
// preview UIs.
type ManifestStat struct {
	BackupCount    int   `json:"backup_count"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
}

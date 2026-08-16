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
	// the running app supports. v2 added config.yaml, client-state.json and
	// sounds/ — a v1 bundle is still importable, it just predates those and
	// leaves that state untouched.
	FormatVersion = 2

	// manifestName is the manifest file name inside the bundle zip.
	manifestName = "manifest.json"

	// userDBName is the user.db copy's name inside the bundle.
	userDBName = "user.db"

	// backupsDir is the directory inside the bundle that holds copies of
	// every EQ-config backup zip.
	backupsDir = "backups"

	// configName is the config.yaml copy's name inside the bundle.
	configName = "config.yaml"

	// clientStateName is the Electron/renderer state file's name inside the
	// bundle — see Manager.Export's client-state handling.
	clientStateName = "client-state.json"

	// soundsDir is the directory inside the bundle that holds copies of
	// custom sound files referenced by trigger actions/timer alerts.
	soundsDir = "sounds"
)

// Manifest is the bundle's table of contents. It is serialised to
// manifest.json at the root of the bundle zip.
type Manifest struct {
	FormatVersion int          `json:"format_version"`
	AppVersion    string       `json:"app_version"`
	ExportedAt    string       `json:"exported_at"`
	Files         []FileEntry  `json:"files"`
	Stats         ManifestStat `json:"stats"`

	// SoundMap maps each bundled sound file's original absolute path (on the
	// exporting machine) to its name inside the bundle (e.g.
	// "sounds/a1b2c3d4-alarm.wav"). Import uses it to know which files to
	// copy out and which stored sound_path values to rewrite. Absent/empty on
	// bundles with no custom sounds.
	SoundMap map[string]string `json:"sound_map,omitempty"`
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
	// ConfigIncluded reports whether config.yaml (settings) is in the bundle.
	ConfigIncluded bool `json:"config_included"`
	// ClientStateIncluded reports whether client-state.json (overlay layout,
	// window positions, dashboard/localStorage prefs) is in the bundle.
	ClientStateIncluded bool `json:"client_state_included"`
	// SoundCount is how many custom sound files were bundled.
	SoundCount int `json:"sound_count"`
}

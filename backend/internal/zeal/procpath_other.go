//go:build !windows

package zeal

// processExeDir: Zeal only runs on Windows; elsewhere the game's folder is
// unknown, so live proof is never persisted.
func processExeDir(pid uint32) string { return "" }

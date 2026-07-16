package logparser

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// LogSizeWarningThreshold is the file size (bytes) above which a cleanup
	// notification is shown in the UI.
	LogSizeWarningThreshold = 75 * 1024 * 1024 // 75 MB

	// purgeKeepDays is how many days of log entries the cleanup keeps when
	// trimming the live log. 30 days matches the Chat History retention default
	// and the typical backfill window, so trimming won't drop entries the
	// trackers still expect to be able to backfill.
	purgeKeepDays = 30
)

// FileInfo holds metadata about an EQ log file.
type FileInfo struct {
	SizeBytes   int64     `json:"size_bytes"`
	OldestEntry time.Time `json:"oldest_entry"`
	NewestEntry time.Time `json:"newest_entry"`
	LargeFile   bool      `json:"large_file"`
}

// GetFileInfo returns the size and timestamp range for the given log file.
// It reads only the first few lines (for oldest) and the last 32 KB (for newest),
// so it is fast even on very large files.
func GetFileInfo(path string) (FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}

	size := fi.Size()
	info := FileInfo{
		SizeBytes: size,
		LargeFile: size >= LogSizeWarningThreshold,
	}

	f, err := openShared(path)
	if err != nil {
		return info, err
	}
	defer f.Close()

	// Oldest entry: scan from start until we find the first valid timestamp.
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ts, _, ok := ParseRawLine(sc.Text())
		if ok {
			info.OldestEntry = ts
			break
		}
	}

	// Newest entry: seek near the end and find the last valid timestamp.
	const tailBytes = 32 * 1024
	seekPos := size - tailBytes
	if seekPos < 0 {
		seekPos = 0
	}
	if _, err := f.Seek(seekPos, io.SeekStart); err == nil {
		var lastTS time.Time
		sc2 := bufio.NewScanner(f)
		for sc2.Scan() {
			ts, _, ok := ParseRawLine(sc2.Text())
			if ok {
				lastTS = ts
			}
		}
		if !lastTS.IsZero() {
			info.NewestEntry = lastTS
		}
	}

	return info, nil
}

// BackupAndPurge atomically backs up the log file and rewrites it keeping
// only the most recent purgeKeepDays days of entries.
// Returns the path to the backup file.
func BackupAndPurge(logPath string) (string, error) {
	dir := filepath.Dir(logPath)
	base := filepath.Base(logPath)
	name := strings.TrimSuffix(base, ".txt")
	backupName := name + "." + time.Now().Format("2006-01-02") + ".bak.zip"
	backupPath := filepath.Join(dir, backupName)

	origInfo, err := os.Stat(logPath)
	if err != nil {
		return "", fmt.Errorf("stat log file: %w", err)
	}

	// Step 1: Zip original into backup. EQ log text compresses heavily, so
	// this keeps repeated purges from silently filling the user's disk.
	if err := zipFile(logPath, backupPath, base, origInfo); err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}

	// Step 2: Verify backup integrity by comparing the zip entry's
	// uncompressed size against the original file.
	if err := verifyZipEntry(backupPath, base, origInfo.Size()); err != nil {
		return backupPath, fmt.Errorf("verify backup: %w", err)
	}

	// Step 3: Filter lines to the past purgeKeepDays days.
	cutoff := time.Now().AddDate(0, 0, -purgeKeepDays)
	kept, err := filterLines(logPath, cutoff)
	if err != nil {
		return backupPath, fmt.Errorf("filter lines: %w", err)
	}

	// Step 4: Rewrite original with filtered content.
	content := strings.Join(kept, "\n")
	if len(kept) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		return backupPath, fmt.Errorf("rewrite log: %w", err)
	}

	return backupPath, nil
}

// zipFile writes src (opened with sharing flags so it works while the log is
// being tailed) into a single-entry zip at dst.
func zipFile(src, dst, entryName string, srcInfo os.FileInfo) error {
	in, err := openShared(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	w := zip.NewWriter(out)
	hdr, err := zip.FileInfoHeader(srcInfo)
	if err != nil {
		w.Close()
		return err
	}
	hdr.Name = entryName
	hdr.Method = zip.Deflate

	entry, err := w.CreateHeader(hdr)
	if err != nil {
		w.Close()
		return err
	}
	if _, err := io.Copy(entry, in); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return out.Sync()
}

// verifyZipEntry confirms zipPath contains an entry named entryName whose
// uncompressed size matches wantSize.
func verifyZipEntry(zipPath, entryName string, wantSize int64) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == entryName {
			if int64(f.UncompressedSize64) != wantSize {
				return fmt.Errorf("size mismatch: orig=%d backup=%d", wantSize, f.UncompressedSize64)
			}
			return nil
		}
	}
	return fmt.Errorf("entry %q not found in zip", entryName)
}

func filterLines(path string, cutoff time.Time) ([]string, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var kept []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		ts, _, ok := ParseRawLine(line)
		if !ok || ts.After(cutoff) {
			kept = append(kept, line)
		}
	}
	return kept, sc.Err()
}

// Package quarm reads Project Quarm client-file metadata (DLL versions, MD5
// hashes, PE compile timestamps) and compares them against the public Quarm
// patch manifest. It is used purely for read-only display of "is your client
// up to date" status — pq-companion does not patch or modify game files.
package quarm

import (
	"crypto/md5"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf16"
)

// FileInfo is what we can derive from a Windows DLL on disk without running
// it: the MD5 of its byte contents, its PE-header compile timestamp, and (if
// the binary ships a VS_VERSION_INFO resource) the FileVersion string from
// that resource. eqw.dll has no version resource, so FileVersion is "" for
// it — callers must tolerate empty strings.
type FileInfo struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	MD5         string    `json:"md5"`
	CompiledAt  time.Time `json:"compiled_at"`
	FileVersion string    `json:"file_version,omitempty"`
}

// InspectDLL reads path and extracts FileInfo. Returns an error only on I/O
// or PE-parse failure — a missing VS_VERSION_INFO resource is reported as an
// empty FileVersion rather than an error, because eqw.dll legitimately has
// no version resource.
func InspectDLL(path string) (FileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileInfo{}, err
	}
	sum := md5.Sum(data)
	info := FileInfo{
		Path: path,
		Size: int64(len(data)),
		MD5:  hex.EncodeToString(sum[:]),
	}

	f, err := pe.NewFile(memReader(data))
	if err != nil {
		return info, fmt.Errorf("parse PE: %w", err)
	}
	defer f.Close()

	info.CompiledAt = time.Unix(int64(f.FileHeader.TimeDateStamp), 0).UTC()

	if v, err := readFileVersion(f, data); err == nil && v != "" {
		info.FileVersion = v
	}
	return info, nil
}

// readFileVersion walks the .rsrc section for a VS_VERSION_INFO block and
// returns its FileVersion string. Returns ("", nil) when no resource is
// present (e.g. eqw.dll), and a non-nil error only when the resource is
// present but malformed.
func readFileVersion(f *pe.File, raw []byte) (string, error) {
	rsrc := f.Section(".rsrc")
	if rsrc == nil {
		return "", nil
	}
	data, err := rsrc.Data()
	if err != nil {
		return "", fmt.Errorf("read .rsrc: %w", err)
	}
	// Resource section RVAs are relative to the section start when stored
	// inline. The PE loader fixes them up by adding the section's
	// VirtualAddress; to look up an offset within `data`, subtract that VA
	// from the stored RVA.
	rsrcVA := rsrc.VirtualAddress

	// Find the RT_VERSION (type 16) directory entry.
	verDataRVA, verDataSize, err := findVersionResource(data, rsrcVA)
	if err != nil || verDataSize == 0 {
		return "", err
	}
	off := int(verDataRVA - rsrcVA)
	if off < 0 || off+int(verDataSize) > len(data) {
		return "", errors.New("version resource out of bounds")
	}
	return parseVersionInfo(data[off : off+int(verDataSize)])
}

// findVersionResource walks the .rsrc directory tree (3 levels: type, name,
// language) and returns the RVA + size of the first RT_VERSION data entry.
// The .rsrc section in PE is itself a tiny tree of IMAGE_RESOURCE_DIRECTORY
// structures; type 16 (RT_VERSION) is the version-info bucket.
func findVersionResource(rsrc []byte, rsrcVA uint32) (rva, size uint32, err error) {
	const rtVersion = 16
	// Level 1: types
	typeEntries, err := readDirEntries(rsrc, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("read type dir: %w", err)
	}
	var typeOff uint32
	found := false
	for _, e := range typeEntries {
		if !e.isName && e.id == rtVersion {
			typeOff = e.offset
			found = true
			break
		}
	}
	if !found {
		return 0, 0, nil
	}
	// Level 2: names (we just take the first)
	nameEntries, err := readDirEntries(rsrc, typeOff)
	if err != nil {
		return 0, 0, fmt.Errorf("read name dir: %w", err)
	}
	if len(nameEntries) == 0 {
		return 0, 0, nil
	}
	// Level 3: languages (first one is fine — neutral or first localized)
	langEntries, err := readDirEntries(rsrc, nameEntries[0].offset)
	if err != nil {
		return 0, 0, fmt.Errorf("read lang dir: %w", err)
	}
	if len(langEntries) == 0 {
		return 0, 0, nil
	}
	// The leaf is an IMAGE_RESOURCE_DATA_ENTRY: { OffsetToData, Size, CodePage, Reserved }
	leafOff := langEntries[0].offset
	if int(leafOff)+16 > len(rsrc) {
		return 0, 0, errors.New("resource data entry out of bounds")
	}
	rva = binary.LittleEndian.Uint32(rsrc[leafOff : leafOff+4])
	size = binary.LittleEndian.Uint32(rsrc[leafOff+4 : leafOff+8])
	return rva, size, nil
}

type rsrcDirEntry struct {
	isName bool
	id     uint32
	offset uint32 // offset within the .rsrc section
}

// readDirEntries parses an IMAGE_RESOURCE_DIRECTORY at offset off and returns
// its child entries with the high-bit "subdirectory" flag cleared from the
// offset field so callers can index straight into the section.
func readDirEntries(rsrc []byte, off uint32) ([]rsrcDirEntry, error) {
	if int(off)+16 > len(rsrc) {
		return nil, errors.New("dir header out of bounds")
	}
	nNamed := binary.LittleEndian.Uint16(rsrc[off+12 : off+14])
	nID := binary.LittleEndian.Uint16(rsrc[off+14 : off+16])
	total := int(nNamed) + int(nID)
	start := int(off) + 16
	if start+total*8 > len(rsrc) {
		return nil, errors.New("dir entries out of bounds")
	}
	out := make([]rsrcDirEntry, 0, total)
	for i := 0; i < total; i++ {
		e := rsrc[start+i*8 : start+i*8+8]
		nameField := binary.LittleEndian.Uint32(e[0:4])
		dataField := binary.LittleEndian.Uint32(e[4:8])
		entry := rsrcDirEntry{}
		if nameField&0x80000000 != 0 {
			entry.isName = true
			entry.id = nameField & 0x7fffffff
		} else {
			entry.id = nameField
		}
		// The high bit on dataField marks a subdirectory; either way we
		// want the offset within .rsrc, which is the low 31 bits.
		entry.offset = dataField & 0x7fffffff
		out = append(out, entry)
	}
	return out, nil
}

// parseVersionInfo walks the VS_VERSION_INFO structure and extracts the
// FileVersion string from the first StringTable entry. The format is a tree
// of variable-length records each starting with { wLength, wValueLength,
// wType, szKey (UTF-16, padded to DWORD), padding, Value }. We don't need to
// understand every record — we just descend along
// VS_VERSION_INFO → StringFileInfo → <lang-codepage> → FileVersion.
func parseVersionInfo(b []byte) (string, error) {
	root, err := readVerRecord(b, 0)
	if err != nil {
		return "", err
	}
	if root.key != "VS_VERSION_INFO" {
		return "", fmt.Errorf("unexpected root key %q", root.key)
	}
	// Children of the root start after the fixed VS_FIXEDFILEINFO struct
	// (52 bytes when present) and any padding to a DWORD boundary.
	childOff := root.valueEnd
	childOff = align32(childOff)
	for childOff < root.end {
		rec, err := readVerRecord(b, childOff)
		if err != nil {
			return "", err
		}
		if rec.length == 0 { // defensive: avoid infinite loop on malformed input
			break
		}
		if rec.key == "StringFileInfo" {
			if v, ok := findStringValue(b, rec, "FileVersion"); ok {
				return v, nil
			}
		}
		childOff = align32(rec.end)
	}
	return "", nil
}

// findStringValue searches a StringFileInfo record for the given string key
// and returns its value, normalizing surrounding whitespace. A
// StringFileInfo contains one or more language-codepage StringTable
// records, each of which contains String records (the actual key/value
// pairs).
func findStringValue(b []byte, sfi verRecord, wantKey string) (string, bool) {
	off := sfi.valueEnd
	off = align32(off)
	for off < sfi.end {
		table, err := readVerRecord(b, off)
		if err != nil || table.length == 0 {
			return "", false
		}
		entryOff := table.valueEnd
		entryOff = align32(entryOff)
		for entryOff < table.end {
			entry, err := readVerRecord(b, entryOff)
			if err != nil || entry.length == 0 {
				break
			}
			if entry.key == wantKey {
				v := readUTF16Z(b, entry.valueStart, entry.valueEnd)
				v = strings.TrimSpace(v)
				if v != "" {
					return v, true
				}
			}
			entryOff = align32(entry.end)
		}
		off = align32(table.end)
	}
	return "", false
}

// verRecord captures the byte offsets of one variable-length VS_VERSION_INFO
// record so callers can navigate its children without re-parsing.
type verRecord struct {
	length     int // wLength: total record bytes including header + value
	valueLen   int // wValueLength: value bytes (or words, for string types)
	typ        int // wType: 0 = binary, 1 = text
	key        string
	valueStart int // byte offset where the value begins
	valueEnd   int // byte offset where the value ends (valueStart + valueLen bytes/words)
	end        int // byte offset where this record ends (start + length)
}

// readVerRecord parses one variable-length VS_VERSION_INFO record at offset
// off. The on-disk layout is: u16 wLength, u16 wValueLength, u16 wType,
// UTF-16 NUL-terminated szKey, padding to DWORD, then the value. For
// wType=1 (text) records the value is also UTF-16 and wValueLength counts
// 16-bit words, not bytes.
func readVerRecord(b []byte, off int) (verRecord, error) {
	if off+6 > len(b) {
		return verRecord{}, errors.New("record header out of bounds")
	}
	rec := verRecord{
		length:   int(binary.LittleEndian.Uint16(b[off : off+2])),
		valueLen: int(binary.LittleEndian.Uint16(b[off+2 : off+4])),
		typ:      int(binary.LittleEndian.Uint16(b[off+4 : off+6])),
	}
	if rec.length == 0 || off+rec.length > len(b) {
		// Truncated record — caller decides how to handle (we use 0 to halt).
		rec.end = len(b)
		return rec, nil
	}
	rec.end = off + rec.length

	// Read szKey (UTF-16 NUL-terminated)
	keyStart := off + 6
	keyEnd := keyStart
	for keyEnd+1 < rec.end {
		if b[keyEnd] == 0 && b[keyEnd+1] == 0 {
			break
		}
		keyEnd += 2
	}
	rec.key = decodeUTF16(b[keyStart:keyEnd])
	// Skip past the NUL terminator (2 bytes) and align to DWORD.
	rec.valueStart = align32(keyEnd + 2)
	if rec.valueStart > rec.end {
		rec.valueStart = rec.end
	}
	// For text records wValueLength is in characters; for binary, bytes.
	if rec.typ == 1 {
		rec.valueEnd = rec.valueStart + rec.valueLen*2
	} else {
		rec.valueEnd = rec.valueStart + rec.valueLen
	}
	if rec.valueEnd > rec.end {
		rec.valueEnd = rec.end
	}
	return rec, nil
}

func readUTF16Z(b []byte, start, end int) string {
	if start >= end || end > len(b) {
		return ""
	}
	chunk := b[start:end]
	// Trim trailing NUL words.
	for len(chunk) >= 2 && chunk[len(chunk)-1] == 0 && chunk[len(chunk)-2] == 0 {
		chunk = chunk[:len(chunk)-2]
	}
	return decodeUTF16(chunk)
}

func decodeUTF16(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u))
}

func align32(x int) int { return (x + 3) &^ 3 }

// memReader wraps a []byte as an io.ReaderAt + io.Closer so debug/pe can
// parse it without re-reading from disk.
type memReader []byte

func (m memReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(m)) {
		return 0, io.EOF
	}
	n := copy(p, m[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}


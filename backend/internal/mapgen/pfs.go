// Package mapgen reads EverQuest client zone geometry and turns it into the
// 2D vector maps PQ Companion ships in maps.db.
//
// This is a build-time tool, not part of the running app: it needs the EQ
// client files, which users have but the app never reads. See
// docs/maps-feasibility.md for the format research behind it.
package mapgen

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// pfsMagic marks a .s3d archive ("PFS " in the header).
var pfsMagic = [4]byte{'P', 'F', 'S', ' '}

// Archive is a parsed .s3d (PFS) archive: a map of filename to inflated bytes.
type Archive map[string][]byte

type pfsEntry struct {
	crc    uint32
	offset uint32
	size   uint32 // inflated size
}

// OpenArchive reads a .s3d file and inflates every entry.
//
// Zone archives are tens of megabytes and mostly textures we don't want, but
// they're read once per zone by an offline tool, so inflating everything keeps
// the code simple. Callers pick out the .wld entries they need.
func OpenArchive(path string) (Archive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parsePFS(data, nil)
}

// OpenArchiveFiles reads a .s3d file and inflates only the entries keep
// accepts. A zone archive is mostly textures, so the app's 3D view, which
// needs just the zone .wld, skips inflating the rest.
func OpenArchiveFiles(path string, keep func(name string) bool) (Archive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parsePFS(data, keep)
}

// parsePFS parses a PFS archive, inflating the entries keep accepts (all of
// them when keep is nil).
func parsePFS(data []byte, keep func(name string) bool) (Archive, error) {
	return parsePFSInto(data, keep, nil)
}

// parsePFSInto is parsePFS; with lazy set, it fills lazy's directory
// instead of inflating anything.
func parsePFSInto(data []byte, keep func(name string) bool, lazy *LazyArchive) (Archive, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("too short to be a PFS archive (%d bytes)", len(data))
	}
	dirOffset := binary.LittleEndian.Uint32(data[0:4])
	var magic [4]byte
	copy(magic[:], data[4:8])
	if magic != pfsMagic {
		return nil, fmt.Errorf("bad magic %q, want %q", magic, pfsMagic)
	}
	if int(dirOffset)+4 > len(data) {
		return nil, fmt.Errorf("directory offset %d past end of file (%d)", dirOffset, len(data))
	}

	count := binary.LittleEndian.Uint32(data[dirOffset:])
	if count == 0 {
		return nil, fmt.Errorf("archive directory is empty")
	}
	entries := make([]pfsEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		base := int(dirOffset) + 4 + int(i)*12
		if base+12 > len(data) {
			return nil, fmt.Errorf("directory entry %d past end of file", i)
		}
		entries = append(entries, pfsEntry{
			crc:    binary.LittleEndian.Uint32(data[base:]),
			offset: binary.LittleEndian.Uint32(data[base+4:]),
			size:   binary.LittleEndian.Uint32(data[base+8:]),
		})
	}

	// The filename table is NOT identified by a fixed CRC. A TAKPv22 install
	// ships both 0xFFFFFFFF (unrest, qeynos2, gfaydark) and 0x61580AC9 (akheva,
	// the value PC-client tooling assumes) across its own archives. Matching
	// either constant breaks on the other, so find it structurally instead: it
	// is the one entry that inflates into a name list holding exactly
	// len(entries)-1 names.
	// The table is normally the last entry in the file, so candidates are
	// tried from the end: otherwise finding it inflates most textures first.
	var names []string
	nameIdx := -1
	byOffset := make([]int, len(entries))
	for i := range byOffset {
		byOffset[i] = i
	}
	sort.Slice(byOffset, func(a, b int) bool { return entries[byOffset[a]].offset > entries[byOffset[b]].offset })
	for _, i := range byOffset {
		e := entries[i]
		raw, err := inflateEntry(data, e)
		if err != nil {
			continue
		}
		got, ok := decodeNameTable(raw, len(entries)-1)
		if ok {
			names, nameIdx = got, i
			break
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("no filename table found among %d entries", len(entries))
	}

	// Names are listed in ascending data-offset order, which is not the
	// directory order (the directory is sorted by CRC).
	payload := make([]pfsEntry, 0, len(entries)-1)
	for i, e := range entries {
		if i != nameIdx {
			payload = append(payload, e)
		}
	}
	sort.Slice(payload, func(i, j int) bool { return payload[i].offset < payload[j].offset })
	if len(payload) != len(names) {
		return nil, fmt.Errorf("name count %d != payload entry count %d", len(names), len(payload))
	}

	if lazy != nil {
		lazy.data = data
		lazy.entries = make(map[string]pfsEntry, len(payload))
		for i, e := range payload {
			lazy.entries[strings.ToLower(names[i])] = e
		}
		return nil, nil
	}
	out := make(Archive, len(payload))
	for i, e := range payload {
		if keep != nil && !keep(names[i]) {
			continue
		}
		raw, err := inflateEntry(data, e)
		if err != nil {
			return nil, fmt.Errorf("inflate %s: %w", names[i], err)
		}
		out[names[i]] = raw
	}
	return out, nil
}

// decodeNameTable parses the filename block: a uint32 count, then that many
// length-prefixed NUL-terminated names. Returns ok only when the block is
// well-formed AND holds wantCount names, which is what distinguishes it from a
// texture that happens to start with plausible-looking bytes.
func decodeNameTable(raw []byte, wantCount int) ([]string, bool) {
	if len(raw) < 4 {
		return nil, false
	}
	n := binary.LittleEndian.Uint32(raw)
	if n == 0 || int(n) != wantCount {
		return nil, false
	}
	names := make([]string, 0, n)
	p := 4
	for i := uint32(0); i < n; i++ {
		if p+4 > len(raw) {
			return nil, false
		}
		l := int(binary.LittleEndian.Uint32(raw[p:]))
		p += 4
		if l <= 0 || p+l > len(raw) {
			return nil, false
		}
		names = append(names, string(raw[p:p+l-1])) // drop trailing NUL
		p += l
	}
	return names, true
}

// inflateEntry walks an entry's chain of zlib blocks. Each block is a
// (deflatedLen, inflatedLen) header followed by the compressed bytes; blocks
// repeat until the entry's declared inflated size is reached.
func inflateEntry(data []byte, e pfsEntry) ([]byte, error) {
	return inflateEntryAt(bytes.NewReader(data), int64(len(data)), e)
}

// inflateEntryAt is inflateEntry over an archive of fileSize bytes read
// through r, so an open file can be read one entry at a time.
func inflateEntryAt(r io.ReaderAt, fileSize int64, e pfsEntry) ([]byte, error) {
	out := make([]byte, 0, e.size)
	p := int64(e.offset)
	var hdr [8]byte
	var block []byte
	for uint32(len(out)) < e.size {
		if p+8 > fileSize {
			return nil, fmt.Errorf("block header past end of file")
		}
		if _, err := r.ReadAt(hdr[:], p); err != nil {
			return nil, fmt.Errorf("read block header: %w", err)
		}
		deflated := int64(binary.LittleEndian.Uint32(hdr[:]))
		p += 8 // skip the inflated-length field; the zlib stream carries it
		if deflated <= 0 || p+deflated > fileSize {
			return nil, fmt.Errorf("block length %d past end of file", deflated)
		}
		if int64(cap(block)) < deflated {
			block = make([]byte, deflated)
		}
		block = block[:deflated]
		if _, err := r.ReadAt(block, p); err != nil {
			return nil, fmt.Errorf("read block: %w", err)
		}
		zr, err := zlib.NewReader(bytes.NewReader(block))
		if err != nil {
			return nil, fmt.Errorf("open zlib block: %w", err)
		}
		chunk, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return nil, fmt.Errorf("inflate block: %w", err)
		}
		out = append(out, chunk...)
		p += deflated
	}
	return out, nil
}

// LazyArchive is an archive whose entries are inflated one at a time, on
// request. Character archives hold over a thousand textures, of which a
// model uses a handful.
//
// It keeps the file open and only its directory in memory, reading an
// entry's blocks when asked: a zone's or gequip's archive is tens of
// megabytes that would otherwise sit in memory for a few textures. Call
// Close when done with it. Get is safe for concurrent use.
type LazyArchive struct {
	data    []byte // set only while parsing the directory
	entries map[string]pfsEntry
	file    *os.File
	size    int64
}

// OpenArchiveLazy opens a .s3d file and reads its directory without
// inflating any entry. The caller must Close it.
func OpenArchiveLazy(path string) (*LazyArchive, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	a := &LazyArchive{}
	if _, err := parsePFSInto(data, nil, a); err != nil {
		f.Close()
		return nil, err
	}
	// Drop the file's bytes: entries are read back through the open file.
	a.data, a.file, a.size = nil, f, int64(len(data))
	return a, nil
}

// Close closes the archive's file. Get fails afterwards.
func (a *LazyArchive) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}

// Has reports whether the archive holds name (any case).
func (a *LazyArchive) Has(name string) bool {
	_, ok := a.entries[strings.ToLower(name)]
	return ok
}

// Get inflates one entry by name (any case). ok is false when the archive
// has no such entry.
func (a *LazyArchive) Get(name string) (b []byte, ok bool, err error) {
	e, ok := a.entries[strings.ToLower(name)]
	if !ok {
		return nil, false, nil
	}
	if a.file == nil {
		b, err = inflateEntry(a.data, e)
		return b, true, err
	}
	b, err = inflateEntryAt(a.file, a.size, e)
	return b, true, err
}

// Names returns every entry name, lower-cased.
func (a *LazyArchive) Names() []string {
	out := make([]string, 0, len(a.entries))
	for n := range a.entries {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

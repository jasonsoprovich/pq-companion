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
	return parsePFS(data)
}

func parsePFS(data []byte) (Archive, error) {
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
	var names []string
	nameIdx := -1
	for i, e := range entries {
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

	out := make(Archive, len(payload))
	for i, e := range payload {
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
	out := make([]byte, 0, e.size)
	p := int(e.offset)
	for uint32(len(out)) < e.size {
		if p+8 > len(data) {
			return nil, fmt.Errorf("block header past end of file")
		}
		deflated := int(binary.LittleEndian.Uint32(data[p:]))
		p += 8 // skip the inflated-length field; the zlib stream carries it
		if deflated <= 0 || p+deflated > len(data) {
			return nil, fmt.Errorf("block length %d past end of file", deflated)
		}
		zr, err := zlib.NewReader(bytes.NewReader(data[p : p+deflated]))
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

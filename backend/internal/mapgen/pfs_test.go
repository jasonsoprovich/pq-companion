package mapgen

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

// buildPFS packs files into a minimal PFS archive: each entry one zlib block,
// the filename table last, the directory after it.
func buildPFS(t *testing.T, names []string, files [][]byte) []byte {
	t.Helper()
	var body bytes.Buffer
	body.Write(make([]byte, 12)) // header, filled in below
	type ent struct{ crc, off, size uint32 }
	var ents []ent
	add := func(crc uint32, raw []byte) {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(raw) //nolint:errcheck
		zw.Close()
		ents = append(ents, ent{crc, uint32(body.Len()), uint32(len(raw))})
		binary.Write(&body, binary.LittleEndian, uint32(z.Len()))  //nolint:errcheck
		binary.Write(&body, binary.LittleEndian, uint32(len(raw))) //nolint:errcheck
		body.Write(z.Bytes())
	}
	for i, f := range files {
		add(uint32(100+i), f)
	}
	var table bytes.Buffer
	binary.Write(&table, binary.LittleEndian, uint32(len(names))) //nolint:errcheck
	for _, n := range names {
		binary.Write(&table, binary.LittleEndian, uint32(len(n)+1)) //nolint:errcheck
		table.WriteString(n)
		table.WriteByte(0)
	}
	add(0xFFFFFFFF, table.Bytes())
	dir := uint32(body.Len())
	binary.Write(&body, binary.LittleEndian, uint32(len(ents))) //nolint:errcheck
	for _, e := range ents {
		binary.Write(&body, binary.LittleEndian, e) //nolint:errcheck
	}
	out := body.Bytes()
	binary.LittleEndian.PutUint32(out[0:], dir)
	copy(out[4:8], pfsMagic[:])
	return out
}

func TestParsePFSKeep(t *testing.T) {
	data := buildPFS(t,
		[]string{"zone.wld", "wall.bmp", "objects.wld"},
		[][]byte{[]byte("WLDDATA"), bytes.Repeat([]byte{7}, 5000), []byte("OBJ")})

	all, err := parsePFS(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || string(all["zone.wld"]) != "WLDDATA" || len(all["wall.bmp"]) != 5000 {
		t.Fatalf("all = %v entries", len(all))
	}

	wld, err := parsePFS(data, func(n string) bool { return n == "zone.wld" || n == "objects.wld" })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wld["wall.bmp"]; ok || string(wld["zone.wld"]) != "WLDDATA" || string(wld["objects.wld"]) != "OBJ" {
		t.Fatalf("filtered = %v", wld)
	}
}

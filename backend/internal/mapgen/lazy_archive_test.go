package mapgen

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// buildPFSChunked is buildPFS with each file split into blocks of at most
// chunk bytes, as the client's archives split large textures.
func buildPFSChunked(t *testing.T, names []string, files [][]byte, chunk int) []byte {
	t.Helper()
	var body bytes.Buffer
	body.Write(make([]byte, 12))
	type ent struct{ crc, off, size uint32 }
	var ents []ent
	add := func(crc uint32, raw []byte) {
		ents = append(ents, ent{crc, uint32(body.Len()), uint32(len(raw))})
		for p := 0; p < len(raw) || p == 0; p += chunk {
			end := min(p+chunk, len(raw))
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			zw.Write(raw[p:end]) //nolint:errcheck
			zw.Close()
			binary.Write(&body, binary.LittleEndian, uint32(z.Len())) //nolint:errcheck
			binary.Write(&body, binary.LittleEndian, uint32(end-p))   //nolint:errcheck
			body.Write(z.Bytes())
			if len(raw) == 0 {
				break
			}
		}
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

func writeTestArchive(t *testing.T, raw []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.s3d")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func testFiles() ([]string, [][]byte) {
	names := []string{"Big.DDS", "small.bmp", "zone.wld"}
	big := make([]byte, 50000)
	for i := range big {
		big[i] = byte(i * 7)
	}
	return names, [][]byte{big, []byte("tiny"), []byte("world data")}
}

// The file-backed reader returns exactly what the in-memory reader does,
// for single- and multi-block entries.
func TestLazyArchiveMatchesOpenArchiveFiles(t *testing.T) {
	names, files := testFiles()
	path := writeTestArchive(t, buildPFSChunked(t, names, files, 8192))
	full, err := OpenArchiveFiles(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := OpenArchiveLazy(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.data != nil {
		t.Error("the archive's bytes should not stay in memory")
	}
	for name, want := range full {
		got, ok, err := a.Get(name)
		if err != nil || !ok || !bytes.Equal(got, want) {
			t.Errorf("Get(%s): %d bytes ok=%v err=%v, want %d bytes", name, len(got), ok, err, len(want))
		}
	}
	if got, ok, _ := a.Get("BIG.dds"); !ok || len(got) != 50000 {
		t.Error("names should match in any case")
	}
	if _, ok, err := a.Get("missing.bmp"); ok || err != nil {
		t.Errorf("missing entry: ok=%v err=%v", ok, err)
	}
}

func TestLazyArchiveConcurrentGet(t *testing.T) {
	names, files := testFiles()
	a, err := OpenArchiveLazy(writeTestArchive(t, buildPFSChunked(t, names, files, 4096)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := names[i%len(names)]
			got, ok, err := a.Get(n)
			if err != nil || !ok || !bytes.Equal(got, files[i%len(files)]) {
				errs <- fmt.Errorf("%s: ok=%v err=%v", n, ok, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func TestLazyArchiveClose(t *testing.T) {
	names, files := testFiles()
	a, err := OpenArchiveLazy(writeTestArchive(t, buildPFSChunked(t, names, files, 8192)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := a.Get("small.bmp"); err == nil {
		t.Errorf("Get after Close: ok=%v, want an error", ok)
	}
	var nilArc *LazyArchive
	if err := nilArc.Close(); err != nil {
		t.Error("Close on nil should be a no-op")
	}
}

// The file shrinking under an open archive (a client patch mid-session)
// gives an error, not a panic.
func TestLazyArchiveTruncated(t *testing.T) {
	names, files := testFiles()
	raw := buildPFSChunked(t, names, files, 8192)
	path := writeTestArchive(t, raw)
	a, err := OpenArchiveLazy(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := os.Truncate(path, 100); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Get("Big.DDS"); err == nil {
		t.Error("want an error reading a truncated archive")
	}
	if _, err := OpenArchiveLazy(writeTestArchive(t, raw[:20])); err == nil {
		t.Error("want an error opening a truncated archive")
	}
}

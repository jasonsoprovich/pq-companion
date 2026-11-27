package api

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testPFS packs files into a minimal .s3d: one zlib block per file, the
// name table last, then the directory.
func testPFS(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var body bytes.Buffer
	body.Write(make([]byte, 12))
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
	var names []string
	for n, b := range files {
		names = append(names, n)
		add(uint32(100+len(ents)), b)
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
	copy(out[4:8], "PFS ")
	return out
}

// testBMP is a 2x2 8-bit bitmap: palette 0 (the colour key) first in the
// first stored row, which decodes to top-left; the rest palette 1 = white.
func testBMP() []byte {
	b := make([]byte, 54+256*4+8)
	copy(b, "BM")
	le := binary.LittleEndian
	le.PutUint32(b[10:], 54+256*4)
	le.PutUint32(b[14:], 40)
	le.PutUint32(b[18:], 2)
	le.PutUint32(b[22:], 2)
	le.PutUint16(b[28:], 8)
	copy(b[54:], []byte{0, 255, 0, 0, 255, 255, 255, 0})
	px := 54 + 256*4
	copy(b[px:], []byte{0, 1, 0, 0})
	copy(b[px+4:], []byte{1, 1, 0, 0})
	return b
}

// testClient writes a client folder: zone "tz" with zone.bmp, its _obj
// archive with obj.bmp, and, when withShared, gequip.s3d with shared.bmp.
func testClient(t *testing.T, withShared bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, files map[string][]byte) {
		if err := os.WriteFile(filepath.Join(dir, name), testPFS(t, files), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tz.s3d", map[string][]byte{"Zone.BMP": testBMP(), "tz.wld": []byte("w")})
	write("tz_obj.s3d", map[string][]byte{"obj.bmp": testBMP()})
	if withShared {
		write("gequip.s3d", map[string][]byte{"shared.bmp": testBMP()})
	}
	return dir
}

func releaseCache(c *textureCache) {
	c.mu.Lock()
	c.releaseLocked()
	c.mu.Unlock()
}

func TestTextureCacheLookups(t *testing.T) {
	dir := testClient(t, true)
	c := &textureCache{}
	defer releaseCache(c)
	for _, n := range []string{"zone.bmp", "obj.bmp", "shared.bmp"} {
		buf, ok, err := c.get(dir, "tz", n, false)
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", n, ok, err)
		}
		img, err := png.Decode(bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("%s: not a PNG: %v", n, err)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a == 0 {
			t.Errorf("%s: unmasked texture should be opaque", n)
		}
	}
	masked, _, _ := c.get(dir, "tz", "zone.bmp", true)
	img, _ := png.Decode(bytes.NewReader(masked))
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("masked and unmasked PNGs should be cached separately")
	}
	if _, ok, err := c.get(dir, "tz", "nope.bmp", false); ok || err != nil {
		t.Errorf("missing texture: ok=%v err=%v", ok, err)
	}
	if _, _, err := c.get(dir, "nozone", "zone.bmp", false); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing zone archive: err=%v, want ErrNotExist", err)
	}
}

// Switching zone closes the old zone's archives.
func TestTextureCacheZoneSwitchCloses(t *testing.T) {
	dir := testClient(t, false)
	if err := os.WriteFile(filepath.Join(dir, "tz2.s3d"), testPFS(t, map[string][]byte{"b.bmp": testBMP()}), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &textureCache{}
	defer releaseCache(c)
	if _, ok, err := c.get(dir, "tz", "zone.bmp", false); !ok || err != nil {
		t.Fatal(ok, err)
	}
	old := c.zone
	if _, ok, err := c.get(dir, "tz2", "b.bmp", false); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, _, err := old[0].Get("zone.bmp"); err == nil {
		t.Error("old zone's archive still open")
	}
}

// A changed EQ folder reopens the shared archives from the new folder.
func TestTextureCacheEQPathChange(t *testing.T) {
	a, b := testClient(t, true), testClient(t, false)
	c := &textureCache{}
	defer releaseCache(c)
	if _, ok, _ := c.get(a, "tz", "shared.bmp", false); !ok {
		t.Fatal("shared texture not found in the first folder")
	}
	if _, ok, err := c.get(b, "tz", "shared.bmp", false); ok || err != nil {
		t.Errorf("second folder has no gequip: ok=%v err=%v", ok, err)
	}
}

func TestTextureCacheIdleRelease(t *testing.T) {
	dir := testClient(t, true)
	c := &textureCache{idle: 30 * time.Millisecond}
	defer releaseCache(c)
	if _, ok, _ := c.get(dir, "tz", "shared.bmp", false); !ok {
		t.Fatal("not found")
	}
	zone, shared := c.zone, c.shared
	deadline := time.Now().Add(3 * time.Second)
	for {
		c.mu.Lock()
		released := c.key == "" && c.zone == nil && c.shared == nil && c.pngs == nil && c.timer == nil
		c.mu.Unlock()
		if released {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache not released after going idle")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, _, err := zone[0].Get("zone.bmp"); err == nil {
		t.Error("zone archive still open after release")
	}
	if _, _, err := shared[0].Get("shared.bmp"); err == nil {
		t.Error("shared archive still open after release")
	}
	if _, ok, err := c.get(dir, "tz", "shared.bmp", false); !ok || err != nil {
		t.Errorf("request after release: ok=%v err=%v", ok, err)
	}
}

// A timer callback that runs after a newer request finds it recent and
// leaves the cache alone.
func TestTextureCacheStaleExpire(t *testing.T) {
	dir := testClient(t, false)
	c := &textureCache{idle: time.Hour}
	defer releaseCache(c)
	c.get(dir, "tz", "zone.bmp", false) //nolint:errcheck
	c.expire()
	if c.key == "" {
		t.Fatal("stale callback released a busy cache")
	}
	c.mu.Lock()
	c.last = time.Now().Add(-2 * time.Hour)
	c.mu.Unlock()
	c.expire()
	if c.key != "" || c.timer != nil {
		t.Fatal("idle callback didn't release")
	}
	c.expire() // a second, stale callback after release is a no-op
	if _, ok, err := c.get(dir, "tz", "zone.bmp", false); !ok || err != nil || c.timer == nil {
		t.Errorf("request after release: ok=%v err=%v timer=%v", ok, err, c.timer != nil)
	}
}

// Requests racing the idle release all succeed. Run with -race.
func TestTextureCacheRequestsDuringExpiry(t *testing.T) {
	dir := testClient(t, true)
	c := &textureCache{idle: time.Millisecond}
	defer releaseCache(c)
	names := []string{"zone.bmp", "obj.bmp", "shared.bmp"}
	var wg sync.WaitGroup
	errs := make(chan string, 400)
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 40 {
				n := names[(g+i)%len(names)]
				if _, ok, err := c.get(dir, "tz", n, i%2 == 0); !ok || err != nil {
					errs <- n
				}
				if i%5 == 0 {
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for n := range errs {
		t.Errorf("%s failed during expiry", n)
	}
}

// An idle release hands the freed memory back to the OS once.
func TestTextureCacheReleaseFreesOSMemory(t *testing.T) {
	calls := make(chan struct{}, 4)
	old := freeOSMemory
	freeOSMemory = func() { calls <- struct{}{} }
	defer func() { freeOSMemory = old }()
	dir := testClient(t, false)
	c := &textureCache{idle: time.Hour}
	c.get(dir, "tz", "zone.bmp", false) //nolint:errcheck
	releaseCache(c)
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("release didn't free OS memory")
	}
}

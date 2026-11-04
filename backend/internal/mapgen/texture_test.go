package mapgen

import (
	"encoding/binary"
	"testing"
)

// dds builds a DXT1 file of the given size with a full mip chain, every
// block the same.
func dds(size int, block [8]byte) []byte {
	b := make([]byte, 128)
	copy(b, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(b[12:], uint32(size))
	le.PutUint32(b[16:], uint32(size))
	mips := 0
	for s := size; s >= 1; s /= 2 {
		mips++
		for i := 0; i < max((s+3)/4, 1)*max((s+3)/4, 1); i++ {
			b = append(b, block[:]...)
		}
	}
	le.PutUint32(b[28:], uint32(mips))
	copy(b[84:], "DXT1")
	return b
}

func TestDecodeDDSPicksMip(t *testing.T) {
	// c0 = pure red, c1 = pure blue, c0 > c1: four opaque colours, all
	// texels index 0.
	raw := dds(64, [8]byte{0x00, 0xF8, 0x1F, 0x00})
	img, err := DecodeTexture(raw, 16, false)
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dx() != 16 {
		t.Fatalf("width %d, want the 16 px mip", img.Rect.Dx())
	}
	if c := img.NRGBAAt(3, 3); c.R != 255 || c.B != 0 || c.A != 255 {
		t.Fatalf("texel = %v", c)
	}
}

func TestDecodeDXT1Mask(t *testing.T) {
	// c0 <= c1: index 3 is the transparent texel; every texel uses it.
	raw := dds(4, [8]byte{0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF})
	for _, masked := range []bool{false, true} {
		img, err := DecodeTexture(raw, 256, masked)
		if err != nil {
			t.Fatal(err)
		}
		want := uint8(255)
		if masked {
			want = 0
		}
		if a := img.NRGBAAt(0, 0).A; a != want {
			t.Errorf("masked=%v: alpha %d, want %d", masked, a, want)
		}
	}
}

func TestDecodeBMP8(t *testing.T) {
	// 2x2, palette: 0 = green key, 1 = white. Bottom row first.
	const w, h = 2, 2
	b := make([]byte, 54+256*4+4*h)
	copy(b, "BM")
	le := binary.LittleEndian
	le.PutUint32(b[10:], uint32(54+256*4))
	le.PutUint32(b[14:], 40)
	le.PutUint32(b[18:], w)
	le.PutUint32(b[22:], h)
	le.PutUint16(b[28:], 8)
	copy(b[54:], []byte{0, 255, 0, 0, 255, 255, 255, 0})
	px := 54 + 256*4
	copy(b[px:], []byte{1, 1, 0, 0})   // bottom row: white
	copy(b[px+4:], []byte{0, 1, 0, 0}) // top row: key, white
	img, err := DecodeTexture(b, 256, true)
	if err != nil {
		t.Fatal(err)
	}
	if c := img.NRGBAAt(0, 0); c.A != 0 {
		t.Errorf("top-left key texel = %v, want transparent", c)
	}
	if c := img.NRGBAAt(0, 1); c.R != 255 || c.G != 255 || c.A != 255 {
		t.Errorf("bottom-left = %v, want white", c)
	}
	if img, _ = DecodeTexture(b, 1, false); img.Rect.Dx() != 1 || img.NRGBAAt(0, 0).A != 255 {
		t.Errorf("halved unmasked = %v", img.Rect)
	}
}

package mapgen

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
)

// Zone textures arrive under .bmp names, but on a TAKP client nearly all of
// them are DXT1 DDS files with a full mip chain (1024 px on a side); a few
// are real 8-bit paletted bitmaps. The 3D view is a map, not the game, so
// textures are decoded at a small size to keep a zone's worth in GPU memory.

// LoadTextures reads the texture entries of <clientDir>/<shortName>.s3d,
// keyed by lowercased file name.
func LoadTextures(clientDir, shortName string) (map[string][]byte, error) {
	arc, err := OpenArchiveFiles(filepath.Join(clientDir, shortName+".s3d"), func(name string) bool {
		l := strings.ToLower(name)
		return strings.HasSuffix(l, ".bmp") || strings.HasSuffix(l, ".dds")
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(arc))
	for name, b := range arc {
		out[strings.ToLower(name)] = b
	}
	return out, nil
}

// DecodeTexture decodes a zone texture, at most maxSize pixels on a side
// (the largest DDS mip level that fits, or a bitmap halved until it does).
// With masked, the colour key is kept as transparency: palette index 0 in a
// bitmap, or DXT1's transparent texel. Otherwise the result is opaque.
func DecodeTexture(raw []byte, maxSize int, masked bool) (*image.NRGBA, error) {
	switch {
	case len(raw) >= 4 && string(raw[:4]) == "DDS ":
		return decodeDDS(raw, maxSize, masked)
	case len(raw) >= 2 && string(raw[:2]) == "BM":
		img, err := decodeBMP(raw, masked)
		if err != nil {
			return nil, err
		}
		for img.Rect.Dx() > maxSize || img.Rect.Dy() > maxSize {
			img = halve(img)
		}
		return img, nil
	}
	return nil, fmt.Errorf("unknown texture format")
}

func decodeDDS(raw []byte, maxSize int, masked bool) (*image.NRGBA, error) {
	if len(raw) < 128 {
		return nil, fmt.Errorf("dds header too short")
	}
	le := binary.LittleEndian
	h := int(le.Uint32(raw[12:]))
	w := int(le.Uint32(raw[16:]))
	mips := int(le.Uint32(raw[28:]))
	if mips < 1 {
		mips = 1
	}
	var block int
	switch string(raw[84:88]) {
	case "DXT1":
		block = 8
	case "DXT3", "DXT5":
		block = 16
	default:
		return nil, fmt.Errorf("dds format %q not supported", raw[84:88])
	}
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("dds size %dx%d", w, h)
	}
	p := 128
	for level := 0; ; level++ {
		size := ((w + 3) / 4) * ((h + 3) / 4) * block
		if p+size > len(raw) {
			return nil, fmt.Errorf("dds level %d past end of file", level)
		}
		if (w <= maxSize && h <= maxSize) || level == mips-1 || (w == 1 && h == 1) {
			return decodeDXT(raw[p:p+size], w, h, string(raw[84:88]), masked), nil
		}
		p += size
		w, h = max(w/2, 1), max(h/2, 1)
	}
}

// decodeDXT decodes one S3TC image.
func decodeDXT(b []byte, w, h int, format string, masked bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	le := binary.LittleEndian
	block := 8
	if format != "DXT1" {
		block = 16
	}
	var pal [4]color.NRGBA
	var alpha [16]uint8
	p := 0
	for by := 0; by < (h+3)/4; by++ {
		for bx := 0; bx < (w+3)/4; bx++ {
			cb := b[p:]
			if block == 16 {
				decodeDXTAlpha(b[p:p+8], format, &alpha)
				cb = b[p+8:]
			}
			c0, c1 := le.Uint16(cb), le.Uint16(cb[2:])
			pal[0], pal[1] = rgb565(c0), rgb565(c1)
			if c0 > c1 || block == 16 {
				pal[2] = mix(pal[0], pal[1], 2, 1)
				pal[3] = mix(pal[0], pal[1], 1, 2)
			} else {
				pal[2] = mix(pal[0], pal[1], 1, 1)
				pal[3] = color.NRGBA{A: 255}
				if masked {
					pal[3].A = 0
				}
			}
			bits := le.Uint32(cb[4:])
			for i := 0; i < 16; i++ {
				x, y := bx*4+i%4, by*4+i/4
				if x >= w || y >= h {
					continue
				}
				c := pal[(bits>>(2*i))&3]
				if block == 16 {
					c.A = alpha[i]
				}
				img.SetNRGBA(x, y, c)
			}
			p += block
		}
	}
	return img
}

// decodeDXTAlpha decodes a DXT3 or DXT5 block's 16 alpha values.
func decodeDXTAlpha(b []byte, format string, out *[16]uint8) {
	if format == "DXT3" {
		for i := 0; i < 16; i++ {
			out[i] = (b[i/2] >> (4 * (i % 2)) & 0xF) * 17
		}
		return
	}
	a0, a1 := int(b[0]), int(b[1])
	var pal [8]int
	pal[0], pal[1] = a0, a1
	if a0 > a1 {
		for i := 1; i < 7; i++ {
			pal[i+1] = ((7-i)*a0 + i*a1) / 7
		}
	} else {
		for i := 1; i < 5; i++ {
			pal[i+1] = ((5-i)*a0 + i*a1) / 5
		}
		pal[6], pal[7] = 0, 255
	}
	var bits uint64
	for i := 0; i < 6; i++ {
		bits |= uint64(b[2+i]) << (8 * i)
	}
	for i := 0; i < 16; i++ {
		out[i] = uint8(pal[(bits>>(3*i))&7])
	}
}

func rgb565(c uint16) color.NRGBA {
	r, g, b := uint8(c>>11&0x1F), uint8(c>>5&0x3F), uint8(c&0x1F)
	return color.NRGBA{R: r<<3 | r>>2, G: g<<2 | g>>4, B: b<<3 | b>>2, A: 255}
}

// mix returns (wa*a + wb*b) / (wa+wb), opaque.
func mix(a, b color.NRGBA, wa, wb int) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8((wa*int(x) + wb*int(y)) / (wa + wb)) }
	return color.NRGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: 255}
}

// decodeBMP decodes an uncompressed 8, 24 or 32-bit bitmap.
func decodeBMP(raw []byte, masked bool) (*image.NRGBA, error) {
	if len(raw) < 54 {
		return nil, fmt.Errorf("bmp header too short")
	}
	le := binary.LittleEndian
	dataOff := int(le.Uint32(raw[10:]))
	infoSize := int(le.Uint32(raw[14:]))
	w := int(int32(le.Uint32(raw[18:])))
	h := int(int32(le.Uint32(raw[22:])))
	bpp := int(le.Uint16(raw[28:]))
	if comp := le.Uint32(raw[30:]); comp != 0 {
		return nil, fmt.Errorf("compressed bmp (%d) not supported", comp)
	}
	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("bmp size %dx%d", w, h)
	}
	var pal []color.NRGBA
	if bpp == 8 {
		n := int(le.Uint32(raw[46:]))
		if n == 0 || n > 256 {
			n = 256
		}
		p := 14 + infoSize
		for i := 0; i < n && p+4*i+4 <= len(raw); i++ {
			q := raw[p+4*i:]
			pal = append(pal, color.NRGBA{R: q[2], G: q[1], B: q[0], A: 255})
		}
		if masked && len(pal) > 0 {
			pal[0].A = 0
		}
	} else if bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("%d-bit bmp not supported", bpp)
	}
	stride := (w*bpp/8 + 3) &^ 3
	if dataOff < 0 || dataOff+stride*h > len(raw) {
		return nil, fmt.Errorf("bmp pixels past end of file")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := raw[dataOff+stride*y:]
		dy := h - 1 - y
		if topDown {
			dy = y
		}
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch bpp {
			case 8:
				if i := int(row[x]); i < len(pal) {
					c = pal[i]
				}
			case 24:
				c = color.NRGBA{R: row[3*x+2], G: row[3*x+1], B: row[3*x], A: 255}
			case 32:
				c = color.NRGBA{R: row[4*x+2], G: row[4*x+1], B: row[4*x], A: 255}
			}
			img.SetNRGBA(x, dy, c)
		}
	}
	return img, nil
}

// halve box-filters an image to half size.
func halve(src *image.NRGBA) *image.NRGBA {
	w, h := max(src.Rect.Dx()/2, 1), max(src.Rect.Dy()/2, 1)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a int
			for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				c := src.NRGBAAt(min(2*x+d[0], src.Rect.Dx()-1), min(2*y+d[1], src.Rect.Dy()-1))
				r, g, b, a = r+int(c.R), g+int(c.G), b+int(c.B), a+int(c.A)
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / 4), G: uint8(g / 4), B: uint8(b / 4), A: uint8(a / 4)})
		}
	}
	return dst
}

package maps

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// inflate decompresses a segment blob written by cmd/mapgen.
func inflate(blob []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("open segment blob: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompress segment blob: %w", err)
	}
	return raw, nil
}

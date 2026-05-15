package appbackup

import (
	"archive/zip"
	"io"
)

// newTestZip returns a zip.Writer over w. Wrapper exists so the test file
// stays free of archive/zip imports.
func newTestZip(w io.Writer) *zip.Writer {
	return zip.NewWriter(w)
}

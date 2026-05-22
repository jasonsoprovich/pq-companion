package zeal

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// MinSupportedVersion is the lowest Zeal release that exports everything
// pq-companion needs (quarmy export files, full label set, etc.). Older
// installs work for the log-driven features but leave large parts of the UI
// blank, so we surface a warning banner asking the user to update.
//
// Bump this when we adopt a Zeal feature that's only present in a newer
// release. Reducing the value is also OK if a regression workaround lets us
// support an older release again.
const MinSupportedVersion = "1.4.0"

// versionAnchors are stable ASCII literals from CoastalRedwood/Zeal that the
// linker may keep in the same .rdata cluster as the ZEAL_VERSION literal.
// Listed best-first:
//
//   - "Zeal Version: " (capital V) is the crash-handler report label. Zeal's
//     crash handler concatenates it with ZEAL_VERSION, so the compiler emits
//     the version literal into the same translation unit's string pool — it
//     is reliably adjacent.
//   - "Zeal version: " (lowercase v) is the `/version` command output. The
//     version is substituted at runtime via a format string, so the literal
//     is only sometimes clustered nearby.
//   - "Zeal_VersionValue" is the options-panel label key.
var versionAnchors = [][]byte{
	[]byte("Zeal Version: "),
	[]byte("Zeal version: "),
	[]byte("Zeal_VersionValue"),
}

// versionPattern matches a null-terminated MAJOR.MINOR.PATCH string. Bounded
// digit counts avoid hits on long numeric runs that happen to contain dots.
var versionPattern = regexp.MustCompile(`(?:^|\x00)([0-9]{1,2}\.[0-9]{1,3}\.[0-9]{1,4})\x00`)

// ReadInstalledVersion parses the ZEAL_VERSION string out of the PE binary at
// asiPath. Returns "" with no error when the file exists but no version
// string could be located — callers treat that as "unknown" and skip the
// warning rather than false-alarming.
//
// Detection strategy: ZEAL_VERSION is a C string literal compiled into
// .rdata. There's no API to read it. We scan a window around every
// occurrence of every anchor literal (see versionAnchors) for a
// MAJOR.MINOR.PATCH-shaped null-terminated string, and within a window take
// the match closest to the anchor.
//
// We deliberately do NOT fall back to scanning the whole file: Zeal.asi
// statically links libraries (zlib, libpng) that bake their own
// MAJOR.MINOR.PATCH literals into the binary, and an unanchored scan happily
// returns one of those (e.g. libpng's "1.1.3"). A wrong version is worse
// than no version — it would false-alarm the "update Zeal" banner — so an
// unanchored binary is reported as unknown.
func ReadInstalledVersion(asiPath string) (string, error) {
	if strings.TrimSpace(asiPath) == "" {
		return "", nil
	}
	data, err := os.ReadFile(asiPath)
	if err != nil {
		return "", err
	}

	// MSVC packs a translation unit's string literals tightly, so when the
	// version literal is clustered with an anchor it lands within a few KiB.
	const window = 4096
	for _, a := range versionAnchors {
		for off := 0; off < len(data); {
			idx := bytes.Index(data[off:], a)
			if idx < 0 {
				break
			}
			idx += off
			lo := max(idx-window, 0)
			hi := min(len(data), idx+window)
			if v := closestVersionMatch(data[lo:hi], idx-lo); v != "" {
				return v, nil
			}
			off = idx + len(a)
		}
	}
	return "", nil
}

// closestVersionMatch returns the MAJOR.MINOR.PATCH literal nearest to
// anchorPos within b, or "" if none is present. Picking the nearest match
// (rather than the first) keeps the result correct when a window happens to
// span more than one version-shaped string.
func closestVersionMatch(b []byte, anchorPos int) string {
	locs := versionPattern.FindAllSubmatchIndex(b, -1)
	if locs == nil {
		return ""
	}
	best := ""
	bestDist := -1
	for _, loc := range locs {
		// loc[2]:loc[3] is capture group 1 (the version digits).
		dist := loc[2] - anchorPos
		if dist < 0 {
			dist = -dist
		}
		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			best = string(b[loc[2]:loc[3]])
		}
	}
	return best
}

// CompareVersions returns -1 if a < b, 0 if equal, +1 if a > b. Non-numeric
// or missing components are treated as 0, which makes "1.4" compare equal to
// "1.4.0" and "1.4.0.0".
func CompareVersions(a, b string) int {
	ap := splitVersion(a)
	bp := splitVersion(b)
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVersion(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			n = 0
		}
		out[i] = n
	}
	return out
}


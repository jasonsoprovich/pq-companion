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

// versionAnchors are stable ASCII literals that the Zeal source bakes into
// Zeal.asi's .rdata alongside the ZEAL_VERSION literal. Both come from
// CoastalRedwood/Zeal: the first is the format used by the in-game `/version`
// command; the second is the options-panel label key. We scan for either —
// whichever the linker happened to keep adjacent to the version string is
// fine.
var versionAnchors = [][]byte{
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
// .rdata. There's no API to read it. We locate one of the surrounding anchor
// literals from Zeal.cpp, then scan a window around it for a
// MAJOR.MINOR.PATCH-shaped null-terminated string. If no anchor is present
// we widen to the entire file and take the first reasonable match — better
// than nothing, but the anchored hit is preferred because the linker places
// translation-unit strings together.
func ReadInstalledVersion(asiPath string) (string, error) {
	if strings.TrimSpace(asiPath) == "" {
		return "", nil
	}
	data, err := os.ReadFile(asiPath)
	if err != nil {
		return "", err
	}

	anchorOff := -1
	for _, a := range versionAnchors {
		if idx := bytes.Index(data, a); idx >= 0 {
			anchorOff = idx
			break
		}
	}

	// Look in a 4 KiB window on each side of the anchor first — MSVC packs
	// per-translation-unit string literals tightly, so the version literal
	// almost always lands inside this range.
	if anchorOff >= 0 {
		const window = 4096
		lo := max(anchorOff-window, 0)
		hi := min(len(data), anchorOff+window)
		if v := firstVersionMatch(data[lo:hi]); v != "" {
			return v, nil
		}
	}

	// Fallback: scan the whole file. Higher chance of a false positive from
	// some unrelated version-shaped string, but we'd rather find the version
	// than silently skip the check.
	if v := firstVersionMatch(data); v != "" {
		return v, nil
	}
	return "", nil
}

func firstVersionMatch(b []byte) string {
	m := versionPattern.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return string(m[1])
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


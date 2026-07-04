// Package eqw reports the installed version of eqw.dll, the EQW-TAKP
// windowed-mode wrapper (https://github.com/CoastalRedwood/eqw_takp) that
// TAKP/Quarm players drop next to eqgame.exe.
//
// eqw.dll pairs with eqgame.dll in the client folder, but unlike eqgame.dll it
// carries no PE VS_VERSION_INFO resource (so quarm.InspectDLL reports a blank
// FileVersion) and it is not covered by the Quarm patcher manifest. It does,
// however, bake a plain-ASCII build stamp of the form
//
//	1.0.1 (Jan 20 2026 22:09:32)
//
// into .rdata next to an "EQW version: %s" format string. We scan for that
// stamp the same way the zeal package scans Zeal.asi, and compare against the
// newest GitHub release tag — the exact mechanism zeal.LatestFetcher uses, and
// deliberately different from the manifest-based eqgame.dll check.
package eqw

import (
	"bytes"
	"os"
	"regexp"
	"strings"
)

// buildStampPattern matches the MAJOR.MINOR.PATCH prefix of the EQW-TAKP build
// stamp: a version triple immediately followed by " (" (the "(Mon DD YYYY …)"
// compile timestamp). Requiring the " (" suffix keeps us from matching a bare
// version literal baked in by a statically linked library, so — unlike Zeal —
// we can safely scan the whole binary without a separate text anchor. Bounded
// digit counts avoid hits on long numeric runs that happen to contain dots.
var buildStampPattern = regexp.MustCompile(`([0-9]{1,2}\.[0-9]{1,3}\.[0-9]{1,4}) \(`)

// ReadInstalledVersion parses the EQW-TAKP version out of the PE binary at
// dllPath. Returns "" with no error when the file exists but no build stamp
// could be located — callers treat that as "unknown" and skip the update hint
// rather than false-alarming.
func ReadInstalledVersion(dllPath string) (string, error) {
	if strings.TrimSpace(dllPath) == "" {
		return "", nil
	}
	data, err := os.ReadFile(dllPath)
	if err != nil {
		return "", err
	}
	m := buildStampPattern.FindSubmatch(data)
	if m == nil {
		return "", nil
	}
	return string(bytes.TrimSpace(m[1])), nil
}

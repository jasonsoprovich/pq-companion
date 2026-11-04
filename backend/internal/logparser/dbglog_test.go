package logparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dbgZoneIn(ts, host string, port, zone string) string {
	return strings.Join([]string{
		"[" + ts + "]00521:Zone addr [" + host + ":" + port + "] received...",
		"[" + ts + "]00522:Zone info received.",
		"[" + ts + "]00523:         StartWorldDisplay: " + zone,
		"[" + ts + "]00524:         StartWorldDisplay: " + zone + "_obj",
		"[" + ts + "]00525:         StartWorldDisplay: " + zone + "_chr2",
		"[" + ts + "]00526:         StartWorldDisplay: objects",
		"[" + ts + "]00527:         StartWorldDisplay: lights",
	}, "\r\n") + "\r\n"
}

const dbgHost = "pq.projectquarm.pq3.projectquarm.com"

func TestDbgStateFeed(t *testing.T) {
	tests := []struct {
		name      string
		lines     string
		wantZone  string
		wantToken string
	}{
		{"zone-in", dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "7312", "thedeep"), "thedeep", dbgHost + ":7312"},
		{"later zone-in wins",
			dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "11243", "thedeep") +
				dbgZoneIn("Mon Oct 12 16:12:00 2026", dbgHost, "7312", "thedeep"),
			"thedeep", dbgHost + ":7312"},
		{"char select clears",
			dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "7312", "thedeep") +
				"[Mon Oct 12 16:20:00 2026]00090:Starting char select.\r\n",
			"", ""},
		{"camp clears",
			dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "7312", "thedeep") +
				"[Mon Oct 12 16:20:00 2026]00091:*** EXITING: I have completed camping.\r\n",
			"", ""},
		{"no addr line leaves the token empty",
			"[Mon Oct 12 16:00:00 2026]00523:         StartWorldDisplay: thedeep\r\n",
			"thedeep", ""},
		{"addr is used once",
			dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "7312", "thedeep") +
				"[Mon Oct 12 16:30:00 2026]00600:         StartWorldDisplay: fearplane\r\n",
			"fearplane", ""},
		{"other lines ignored",
			"[Mon Oct 12 16:00:00 2026]00001:C:\\EverQuest\\eqgame.exe\r\n" +
				"[Mon Oct 12 16:00:00 2026]00002:Zone addr [bad] received...\r\n" +
				"no timestamp StartWorldDisplay: thedeep\r\n",
			"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s dbgState
			for _, l := range strings.Split(tc.lines, "\n") {
				s.feed(l)
			}
			if s.cur.Zone != tc.wantZone || s.cur.Token != tc.wantToken {
				t.Fatalf("got %q %q, want %q %q", s.cur.Zone, s.cur.Token, tc.wantZone, tc.wantToken)
			}
		})
	}
}

func TestZoneCopyReaderPoll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, dbgLogName)
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	appendTo := func(s string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(s) //nolint:errcheck
		f.Close()        //nolint:errcheck
	}
	r := &ZoneCopyReader{}

	if _, changed := r.poll(path); changed {
		t.Fatal("missing file reported a change")
	}

	write(dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "11243", "thedeep"))
	cur, changed := r.poll(path)
	if !changed || cur.Token != dbgHost+":11243" {
		t.Fatalf("first read: %+v changed=%v", cur, changed)
	}
	if _, changed := r.poll(path); changed {
		t.Fatal("unchanged file reported a change")
	}

	// A line split across two polls is read once it's complete.
	z := dbgZoneIn("Mon Oct 12 16:12:00 2026", dbgHost, "7312", "thedeep")
	cut := strings.Index(z, "StartWorldDisplay") + 5
	appendTo(z[:cut])
	if cur, _ := r.poll(path); cur.Token != dbgHost+":11243" {
		t.Fatalf("half a line moved the copy: %+v", cur)
	}
	appendTo(z[cut:])
	if cur, changed := r.poll(path); !changed || cur.Token != dbgHost+":7312" {
		t.Fatalf("after zone-in: %+v changed=%v", cur, changed)
	}

	// The client starts a new, smaller file: it's read from the top.
	write(dbgZoneIn("Mon Oct 12 17:00:00 2026", dbgHost, "7293", "fearplane"))
	if cur, _ := r.poll(path); cur.Zone != "fearplane" || cur.Token != dbgHost+":7293" {
		t.Fatalf("new file: %+v", cur)
	}

	// A large file is read from its tail; the cut line is dropped.
	big := strings.Repeat("[Mon Oct 12 17:00:00 2026]00001:"+strings.Repeat("x", 90)+"\r\n", dbgSeedBytes/60)
	write(dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "1111", "qeynos") + big +
		dbgZoneIn("Mon Oct 12 18:00:00 2026", dbgHost, "2222", "befallen"))
	r2 := &ZoneCopyReader{}
	if cur, _ := r2.poll(path); cur.Zone != "befallen" || cur.Token != dbgHost+":2222" {
		t.Fatalf("tail seed: %+v", cur)
	}
	if r2.offset != int64(len(dbgZoneIn("Mon Oct 12 16:00:00 2026", dbgHost, "1111", "qeynos")+big+
		dbgZoneIn("Mon Oct 12 18:00:00 2026", dbgHost, "2222", "befallen"))) {
		t.Fatalf("offset %d not at end", r2.offset)
	}

	// The file goes away: the copy is forgotten.
	os.Remove(path) //nolint:errcheck
	if cur, changed := r2.poll(path); !changed || cur.Zone != "" {
		t.Fatalf("removed file: %+v changed=%v", cur, changed)
	}
}

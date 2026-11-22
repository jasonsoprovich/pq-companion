package linematch

import (
	"regexp"
	"strings"
	"testing"
)

func TestSimplePatternWith(t *testing.T) {
	cases := []struct {
		name    string
		m       Simple
		opts    Options
		line    string
		want    bool
		wantErr string
	}{
		{"wildcard off keeps * literal", Simple{Kind: SimpleContains, Text: "a*b"}, Options{}, "a*b", true, ""},
		{"wildcard off rejects", Simple{Kind: SimpleContains, Text: "a*b"}, Options{}, "axxb", false, ""},
		{"wildcard on", Simple{Kind: SimpleExact, Text: "* begins to cast a spell."},
			Options{Wildcards: true}, "a geonid shaman begins to cast a spell.", true, ""},
		{"wildcard is anchored by kind", Simple{Kind: SimpleExact, Text: "* begins to cast"},
			Options{Wildcards: true}, "x begins to cast a spell.", false, ""},
		{"self token matches the name", Simple{Kind: SimpleContains, Text: "{c}"},
			Options{SelfToken: true, Self: "Osui"}, "Osui, mez the add", true, ""},
		{"self token rejects others", Simple{Kind: SimpleContains, Text: "{c}"},
			Options{SelfToken: true, Self: "Osui"}, "Klaital, mez the add", false, ""},
		{"self token with no name matches nothing", Simple{Kind: SimpleContains, Text: "{self}"},
			Options{SelfToken: true}, "anything", false, ""},
		{"self token off captures", Simple{Kind: SimpleContains, Text: "{c} rolls"},
			Options{}, "Klaital rolls", true, ""},
		{"typed self token still captures", Simple{Kind: SimpleStarts, Text: "{c:word} rolls"},
			Options{SelfToken: true, Self: "Osui"}, "Klaital rolls", true, ""},
		{"self token can repeat", Simple{Kind: SimpleContains, Text: "{c} and {c}"},
			Options{SelfToken: true, Self: "Osui"}, "Osui and Osui", true, ""},
		{"duplicate placeholder", Simple{Kind: SimpleStarts, Text: "{a} {a}"}, Options{}, "", false, "used twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := SimplePatternWith(tc.m, tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := regexp.MustCompile(p).MatchString(tc.line); got != tc.want {
				t.Errorf("pattern %q on %q = %v, want %v", p, tc.line, got, tc.want)
			}
		})
	}
}

func TestExpandSelf(t *testing.T) {
	re := regexp.MustCompile(ExpandSelf(`(?i)\b{C}\b`, "Osui"))
	if !re.MatchString("heal osui now") || re.MatchString("heal Osuix") {
		t.Errorf("expanded pattern %q", re)
	}
	if regexp.MustCompile(ExpandSelf(`{self}`, "")).MatchString("{self}") {
		t.Error("with no name the token must match nothing")
	}
}

func TestClassifyChat(t *testing.T) {
	cases := []struct {
		line    string
		ok      bool
		channel string
		speaker string
		message string
	}{
		{"Klaital tells the guild, 'pull in 5'", true, "guild", "Klaital", "pull in 5"},
		{"You tell your raid,  'mez on me'", true, "raid", "Osui", "mez on me"},
		{"Klaital says, 'hail'", true, ChannelSay, "Klaital", "hail"},
		{"You say, 'hail'", true, ChannelSay, "Osui", "hail"},
		{"Klaital tells General:1, 'lfg'", true, "general", "Klaital", "lfg"},
		{"a geonid shaman begins to cast a spell.", false, "", "", ""},
	}
	for _, tc := range cases {
		c, ok := ClassifyChat(tc.line, "Osui")
		if ok != tc.ok || c.Channel != tc.channel || c.Speaker != tc.speaker || c.Message != tc.message {
			t.Errorf("%q = %+v %v", tc.line, c, ok)
		}
	}
}

package lockout

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
		ok   bool
	}{
		{"hours-minutes-seconds", "5 Hours, 50 Minutes, and 55 Seconds", 5*time.Hour + 50*time.Minute + 55*time.Second, true},
		{"singular-hour", "1 Hour, 9 Minutes, and 27 Seconds", time.Hour + 9*time.Minute + 27*time.Second, true},
		{"days-hour-min-sec", "6 Days, 1 Hour, 27 Minutes, and 35 Seconds", 6*24*time.Hour + time.Hour + 27*time.Minute + 35*time.Second, true},
		{"days-no-hours", "2 Days, 57 Minutes, and 13 Seconds", 2*24*time.Hour + 57*time.Minute + 13*time.Second, true},
		{"singular-day-no-hours", "1 Day, 16 Minutes, and 8 Seconds", 24*time.Hour + 16*time.Minute + 8*time.Second, true},
		{"hours-no-days", "23 Hours, 46 Minutes, and 38 Seconds", 23*time.Hour + 46*time.Minute + 38*time.Second, true},
		{"singular-minute", "1 Hour, 1 Minute, and 11 Seconds", time.Hour + time.Minute + 11*time.Second, true},
		{"five-days-big", "5 Days, 13 Hours, 25 Minutes, and 7 Seconds", 5*24*time.Hour + 13*time.Hour + 25*time.Minute + 7*time.Second, true},
		{"no-units", "soon", 0, false},
		{"empty", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseDuration(tt.in)
			if ok != tt.ok {
				t.Fatalf("parseDuration(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsHeader(t *testing.T) {
	tests := []struct {
		in      string
		section Section
		ok      bool
	}{
		{"=== Current Loot Lockouts ===", SectionLoot, true},
		{"=== Current Legacy Item Lockouts ===", SectionLegacy, true},
		{"== King Tranix: Available", "", false},
		{"You have entered The North Karana.", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := IsHeader(tt.in)
		if ok != tt.ok || got != tt.section {
			t.Errorf("IsHeader(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.section, tt.ok)
		}
	}
}

func TestParseIncurred(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantName  string
		remaining time.Duration
		ok        bool
	}{
		{"days-and-hours", "You have incurred a lockout for Diabo Xi Xin Thall that expires in 6 Days and 18 Hours.", "Diabo Xi Xin Thall", 6*24*time.Hour + 18*time.Hour, true},
		{"minutes-seconds", "You have incurred a lockout for Trakanon that expires in 12 Minutes and 3 Seconds.", "Trakanon", 12*time.Minute + 3*time.Second, true},
		{"no-trailing-period", "You have incurred a lockout for Lord Nagafen that expires in 5 Hours", "Lord Nagafen", 5 * time.Hour, true},
		{"sll-row-not-incurred", "== King Tranix: Available", "", 0, false},
		{"chat-not-incurred", "Someone tells the guild, 'hi'", "", 0, false},
		{"empty", "", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, remaining, ok := ParseIncurred(tt.in)
			if ok != tt.ok {
				t.Fatalf("ParseIncurred(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if !ok {
				return
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if remaining != tt.remaining {
				t.Errorf("remaining = %v, want %v", remaining, tt.remaining)
			}
		})
	}
}

func TestParseRow(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantName  string
		available bool
		remaining time.Duration
		ok        bool
	}{
		{"available", "== King Tranix: Available", "King Tranix", true, 0, true},
		{"expires", "== Lord Nagafen: Expires in 5 Hours, 50 Minutes, and 55 Seconds", "Lord Nagafen", false, 5*time.Hour + 50*time.Minute + 55*time.Second, true},
		{"trailing-space-name", "== Shei Vinitras : Available", "Shei Vinitras", true, 0, true},
		{"backtick-name", "== Arch Lich Rhag`Zadune: Expires in 1 Day, 16 Minutes, and 8 Seconds", "Arch Lich Rhag`Zadune", false, 24*time.Hour + 16*time.Minute + 8*time.Second, true},
		{"lowercase-article-name", "== a dracoliche: Expires in 2 Days, 57 Minutes, and 13 Seconds", "a dracoliche", false, 2*24*time.Hour + 57*time.Minute + 13*time.Second, true},
		{"header-not-a-row", "=== Current Loot Lockouts ===", "", false, 0, false},
		{"chat-not-a-row", "Someone tells the guild, 'hi'", "", false, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, ok := ParseRow(tt.in)
			if ok != tt.ok {
				t.Fatalf("ParseRow(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if !ok {
				return
			}
			if row.TargetName != tt.wantName {
				t.Errorf("name = %q, want %q", row.TargetName, tt.wantName)
			}
			if row.Available != tt.available {
				t.Errorf("available = %v, want %v", row.Available, tt.available)
			}
			if row.Remaining != tt.remaining {
				t.Errorf("remaining = %v, want %v", row.Remaining, tt.remaining)
			}
		})
	}
}

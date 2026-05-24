package sandbox

import "testing"

func TestStripLeadingCommentsAndSpace(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"   \n\t  ", ""},
		{"-- comment\nSELECT 1", "SELECT 1"},
		{"/* block */ SELECT 1", "SELECT 1"},
		{"/* one */ /* two */ -- three\nSELECT 1", "SELECT 1"},
		{"SELECT * FROM items", "SELECT * FROM items"},
		{"-- trailing\n", ""},
		{"/* unterminated", ""},
	}
	for _, c := range cases {
		got := stripLeadingCommentsAndSpace(c.in)
		if got != c.want {
			t.Errorf("stripLeadingCommentsAndSpace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHasAllowedPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"SELECT * FROM items", true},
		{"select 1", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"EXPLAIN QUERY PLAN SELECT 1", true},
		{"INSERT INTO items VALUES (1)", false},
		{"UPDATE items SET name = 'x'", false},
		{"DELETE FROM items", false},
		{"DROP TABLE items", false},
		{"PRAGMA writable_schema = 1", false},
		{"ATTACH DATABASE 'x' AS y", false},
		{"VACUUM", false},
		{"SELECT(1)", true},
		{"SELECT;", true},
	}
	for _, c := range cases {
		got := hasAllowedPrefix(c.in)
		if got != c.want {
			t.Errorf("hasAllowedPrefix(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

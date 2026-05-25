package zeal

import "testing"

func TestExtractTagVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/CoastalRedwood/Zeal/releases/tag/v1.4.2", "1.4.2"},
		{"https://github.com/CoastalRedwood/Zeal/releases/tag/1.4.2", "1.4.2"},
		{"/CoastalRedwood/Zeal/releases/tag/v1.5.0", "1.5.0"},
		{"https://github.com/CoastalRedwood/Zeal/releases/tag/v1.4.2/", "1.4.2"},
		{"https://github.com/CoastalRedwood/Zeal/releases/tag/V1.4.2", "1.4.2"},
		{"https://github.com/CoastalRedwood/Zeal/releases/latest", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractTagVersion(c.in); got != c.want {
			t.Errorf("extractTagVersion(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

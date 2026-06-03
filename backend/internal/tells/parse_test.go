package tells

import "testing"

func TestParseTell(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		wantOK  bool
		peer    string
		dir     string
		message string
	}{
		{"incoming", "Maykill tells you, 'Yea i keep clicking the folder'", true, "Maykill", DirectionIn, "Yea i keep clicking the folder"},
		{"outgoing", "You told Maykill, 'if you can send me some screenshots'", true, "Maykill", DirectionOut, "if you can send me some screenshots"},
		{"incoming with apostrophe", "Laoding tells you, 'Ahh thank you so much betta!'", true, "Laoding", DirectionIn, "Ahh thank you so much betta!"},
		// Channel chatter must NOT match.
		{"tells the raid", "Hoder tells the raid,  'Kragg's Mending on < Stonae >'", false, "", "", ""},
		{"tells named channel", "Ruic tells General:2, '4hr Kei // VoG for tips @ nexus east steps'", false, "", "", ""},
		{"tells the guild", "Bob tells the guild, 'hello'", false, "", "", ""},
		// NPC merchant / banker / trainer replies excluded.
		{"merchant sell", "Banker tells you, 'That'll be 3 gold'", false, "", "", ""},
		{"banker welcome", "Soandso tells you, 'Welcome to my bank'", false, "", "", ""},
		{"trainer skill", "Guildmaster tells you, 'You have increased your skill'", false, "", "", ""},
		// Charmed-pet command echo excluded.
		{"pet master", "Gnoll tells you, 'Attacking Soandso Master.'", false, "", "", ""},
		// Non-tell lines.
		{"loot line", "--Yamete has looted a Shield of the Creator.--", false, "", "", ""},
		{"empty", "", false, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTell(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ParseTell(%q) ok = %v, want %v", tt.msg, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Peer != tt.peer || got.Direction != tt.dir || got.Message != tt.message {
				t.Errorf("ParseTell(%q) = %+v, want peer=%q dir=%q msg=%q", tt.msg, got, tt.peer, tt.dir, tt.message)
			}
		})
	}
}

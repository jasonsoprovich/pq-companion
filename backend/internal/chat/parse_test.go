package chat

import "testing"

func TestParseChat(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		wantOK  bool
		channel string
		dir     string
		peer    string
		message string
	}{
		// Tells (both directions); name capitalization normalizes lowercase.
		{"tell in", "Maykill tells you, 'hey'", true, ChannelTell, DirectionIn, "Maykill", "hey"},
		{"tell out", "You told Maykill, 'yo'", true, ChannelTell, DirectionOut, "Maykill", "yo"},
		{"tell out lowercase name", "You told maykill, 'yo'", true, ChannelTell, DirectionOut, "Maykill", "yo"},
		{"tell in mixedcase name", "MAYKILL tells you, 'hi'", true, ChannelTell, DirectionIn, "Maykill", "hi"},
		// Channels in.
		{"guild in", "Maykill tells the guild, 'Hows it going everyone'", true, ChannelGuild, DirectionIn, "Maykill", "Hows it going everyone"},
		{"raid in double-space", "Katrinka tells the raid,  'ready'", true, ChannelRaid, DirectionIn, "Katrinka", "ready"},
		{"group in", "Thought tells the group, '14% mana'", true, ChannelGroup, DirectionIn, "Thought", "14% mana"},
		{"ooc in", "Teemoo says out of character, 'WTS boots'", true, ChannelOOC, DirectionIn, "Teemoo", "WTS boots"},
		{"auction in", "Holdinstuffn auctions, 'wts 12k'", true, ChannelAuction, DirectionIn, "Holdinstuffn", "wts 12k"},
		{"shout in", "Holdinstuffn shouts, 'wts 12k'", true, ChannelShout, DirectionIn, "Holdinstuffn", "wts 12k"},
		{"named channel in", "Eyden tells General:1, 'skills didnt fix it'", true, "general", DirectionIn, "Eyden", "skills didnt fix it"},
		{"named channel lfg", "Theythem tells Lfg:3, 'JP Poachers LF DPS'", true, "lfg", DirectionIn, "Theythem", "JP Poachers LF DPS"},
		// Channels out.
		{"guild out", "You say to your guild, 'where ya at'", true, ChannelGuild, DirectionOut, "", "where ya at"},
		{"group out", "You tell your party, 'ty :)'", true, ChannelGroup, DirectionOut, "", "ty :)"},
		{"raid out", "You tell your raid, 'incoming'", true, ChannelRaid, DirectionOut, "", "incoming"},
		{"ooc out", "You say out of character, 'mgb kei'", true, ChannelOOC, DirectionOut, "", "mgb kei"},
		{"named channel out", "You tell Seechanters:1, 'who you on?'", true, "seechanters", DirectionOut, "", "who you on?"},
		// NPC tell replies excluded.
		{"merchant tell", "Banker tells you, 'That'll be 3 gold'", false, "", "", "", ""},
		{"pet master tell", "Gnoll tells you, 'Attacking Soandso Master.'", false, "", "", "", ""},
		// Non-chat.
		{"loot line", "--Yamete has looted a Shield.--", false, "", "", "", ""},
		{"empty", "", false, "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseChat(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ParseChat(%q) ok=%v want %v", tt.msg, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Channel != tt.channel || got.Direction != tt.dir || got.Peer != tt.peer || got.Message != tt.message {
				t.Errorf("ParseChat(%q) = %+v; want channel=%q dir=%q peer=%q msg=%q",
					tt.msg, got, tt.channel, tt.dir, tt.peer, tt.message)
			}
		})
	}
}

func TestCapitalizeName(t *testing.T) {
	cases := map[string]string{"soandso": "Soandso", "MAYKILL": "Maykill", "Bob": "Bob", "": ""}
	for in, want := range cases {
		if got := CapitalizeName(in); got != want {
			t.Errorf("CapitalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

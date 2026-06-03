package chat

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	base := time.Unix(1_700_000_000, 0)
	ins := func(channel, dir, peer, msg string, off time.Duration) {
		t.Helper()
		if _, err := s.Insert(Input{Character: "Osui", Channel: channel, Direction: dir, Peer: peer, Message: msg, Zone: "Nexus", TS: base.Add(off)}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	ins(ChannelTell, DirectionIn, "Laoding", "could I snag a vog", 0)
	ins(ChannelTell, DirectionOut, "Laoding", "yep!", time.Minute)
	ins(ChannelGuild, DirectionIn, "Maykill", "hi guild", 2*time.Minute)
	ins(ChannelGuild, DirectionOut, "", "hey", 3*time.Minute)
	ins(ChannelRaid, DirectionIn, "Katrinka", "ready", 4*time.Minute)

	// Duplicate tell is ignored.
	if ok, _ := s.Insert(Input{Character: "Osui", Channel: ChannelTell, Direction: DirectionIn, Peer: "Laoding", Message: "could I snag a vog", Zone: "Nexus", TS: base}); ok {
		t.Errorf("duplicate insert should be ignored")
	}

	// Conversations are tells only → just Laoding.
	convos, err := s.Conversations(ConversationFilters{Character: "Osui", SortDesc: true})
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	if len(convos) != 1 || convos[0].Peer != "Laoding" || convos[0].Count != 2 {
		t.Fatalf("conversations=%+v, want 1 (Laoding, count 2)", convos)
	}
	if convos[0].LastMessage != "yep!" || convos[0].LastDirection != DirectionOut {
		t.Errorf("last message=%q dir=%q, want yep!/out", convos[0].LastMessage, convos[0].LastDirection)
	}

	// Guild feed has 2 rows (one in, one out).
	guild, err := s.Feed(FeedFilters{Character: "Osui", Channel: ChannelGuild})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(guild) != 2 {
		t.Errorf("guild feed=%d, want 2", len(guild))
	}

	// Channels present.
	chans, _ := s.Channels("Osui")
	if len(chans) != 3 { // tell, guild, raid
		t.Errorf("channels=%v, want 3", chans)
	}

	// Date filter excludes out-of-range rows.
	only := s.mustFeed(t, FeedFilters{Character: "Osui", Channel: ChannelGuild, From: base.Add(150 * time.Second).Unix()})
	if len(only) != 1 || only[0].Direction != DirectionOut {
		t.Errorf("date-filtered guild feed=%+v, want 1 outgoing", only)
	}

	// Purge removes everything older than cutoff.
	deleted, err := s.Purge(base.Add(3*time.Minute + 30*time.Second))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted != 4 { // all but the raid line at +4m
		t.Errorf("purged %d, want 4", deleted)
	}
}

// TestEmptyStoreReturnsNonNilSlices guards the black-screen regression: an
// empty store must return [] (not nil) for Channels/Characters so the API
// serializes "[]" rather than "null" — a null channels array crashed the
// Chat History page's channel dropdown.
func TestEmptyStoreReturnsNonNilSlices(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	chans, err := s.Channels("Nobody")
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if chans == nil {
		t.Error("Channels returned nil; want non-nil empty slice")
	}
	chars, err := s.Characters()
	if err != nil {
		t.Fatalf("Characters: %v", err)
	}
	if chars == nil {
		t.Error("Characters returned nil; want non-nil empty slice")
	}
}

func (s *Store) mustFeed(t *testing.T, f FeedFilters) []Message {
	t.Helper()
	r, err := s.Feed(f)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	return r
}

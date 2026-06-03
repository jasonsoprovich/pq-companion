package tells

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
	ins := func(peer, dir, msg string, off time.Duration) {
		t.Helper()
		if _, err := s.Insert(Input{Character: "Osui", Peer: peer, Direction: dir, Message: msg, Zone: "The Nexus", TS: base.Add(off)}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	ins("Laoding", DirectionIn, "could I snag a vog", 0)
	ins("Laoding", DirectionOut, "yep!", time.Minute)
	ins("Maykill", DirectionIn, "no log files", 2*time.Minute)

	// Duplicate insert is ignored.
	if ok, _ := s.Insert(Input{Character: "Osui", Peer: "Laoding", Direction: DirectionIn, Message: "could I snag a vog", Zone: "The Nexus", TS: base}); ok {
		t.Errorf("duplicate insert should be ignored")
	}

	convos, err := s.Conversations(ConversationFilters{Character: "Osui", SortDesc: true})
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	if len(convos) != 2 {
		t.Fatalf("got %d conversations, want 2", len(convos))
	}
	// Newest activity first → Maykill before Laoding.
	if convos[0].Peer != "Maykill" {
		t.Errorf("first convo = %q, want Maykill", convos[0].Peer)
	}
	// Laoding has 2 messages; last is the outgoing "yep!".
	var laoding *Conversation
	for i := range convos {
		if convos[i].Peer == "Laoding" {
			laoding = &convos[i]
		}
	}
	if laoding == nil || laoding.Count != 2 || laoding.LastMessage != "yep!" || laoding.LastDirection != DirectionOut {
		t.Errorf("Laoding summary = %+v, want count=2 last='yep!' dir=out", laoding)
	}

	msgs, err := s.Messages("Osui", "Laoding", false)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Message != "could I snag a vog" {
		t.Errorf("Messages = %+v, want 2 oldest-first", msgs)
	}

	if err := s.DeletePeer("Osui", "Maykill"); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	convos, _ = s.Conversations(ConversationFilters{Character: "Osui"})
	if len(convos) != 1 {
		t.Errorf("after delete got %d conversations, want 1", len(convos))
	}
}

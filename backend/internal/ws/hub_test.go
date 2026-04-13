package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dial connects a test WebSocket client to the given httptest.Server URL.
func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", Handler(hub))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	conn := dial(t, srv)
	defer conn.Close()

	// Give the hub time to register the client.
	time.Sleep(50 * time.Millisecond)

	want := Event{Type: "test", Data: map[string]string{"hello": "world"}}
	hub.Broadcast(want)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}

	var got Event
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != want.Type {
		t.Errorf("type: got %q, want %q", got.Type, want.Type)
	}
}

func TestHub_ClientCount(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", Handler(hub))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if n := hub.ClientCount(); n != 0 {
		t.Fatalf("expected 0 clients, got %d", n)
	}

	conn1 := dial(t, srv)
	conn2 := dial(t, srv)
	time.Sleep(50 * time.Millisecond)

	if n := hub.ClientCount(); n != 2 {
		t.Errorf("expected 2 clients, got %d", n)
	}

	conn1.Close()
	conn2.Close()
	time.Sleep(50 * time.Millisecond)

	if n := hub.ClientCount(); n != 0 {
		t.Errorf("expected 0 clients after close, got %d", n)
	}
}

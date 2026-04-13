package ws

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins during development; lock this down before shipping.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler returns an http.HandlerFunc that upgrades the connection and
// registers the resulting client with hub.
func Handler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws upgrade", "err", err)
			return
		}
		c := &Client{
			hub:        hub,
			conn:       conn,
			send:       make(chan []byte, 256),
			remoteAddr: r.RemoteAddr,
		}
		hub.register <- c
		go c.writePump()
		go c.readPump()
	}
}

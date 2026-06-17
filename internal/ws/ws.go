// Package ws bridges a WebSocket connection to the hub. It understands only
// the minimal envelope needed to route ({t, room}); message bodies are
// forwarded verbatim and never inspected.
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"deaddrop/internal/hub"

	"github.com/coder/websocket"
)

type envelope struct {
	T    string `json:"t"`
	Room string `json:"room"`
}

// Serve upgrades the request and pumps messages between the socket and hub.
func Serve(h *hub.Hub, maxFrame int64, w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// TODO(prod): tighten to the deployed origin. Same-origin behind nginx
		// in production; permissive here so the Vite dev proxy works.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer c.CloseNow()

	if maxFrame > 0 {
		c.SetReadLimit(maxFrame)
	}

	ctx := r.Context()
	client := &hub.Client{Send: make(chan []byte, 32)}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for msg := range client.Send {
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}()

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		var env envelope
		if json.Unmarshal(data, &env) != nil || env.Room == "" {
			continue
		}
		switch env.T {
		case "join":
			n := h.Join(env.Room, client)
			broadcastPresence(h, env.Room, n)
		case "msg":
			if client.Room == env.Room {
				h.Broadcast(env.Room, client, data)
			}
		}
	}

	n := h.Leave(client)
	close(client.Send)
	<-writerDone
	if client.Room != "" {
		broadcastPresence(h, client.Room, n)
	}
}

func broadcastPresence(h *hub.Hub, room string, n int) {
	msg, _ := json.Marshal(map[string]any{"t": "presence", "room": room, "n": n})
	h.Broadcast(room, nil, msg)
}

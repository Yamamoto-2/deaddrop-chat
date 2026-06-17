package main

// Reusable client: WS connection + E2EE, shared by the TUI and the headless
// (-send) test path. The crypto lives in crypto.go and matches the web exactly.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// Incoming is a decoded server event handed to the UI.
type Incoming struct {
	Kind string // "msg" | "presence" | "closed"
	Msg  payload
	N    int
}

type Client struct {
	conn   *websocket.Conn
	rid    string
	key    []byte
	ctx    context.Context
	cancel context.CancelFunc
}

// Dial connects, derives the routing id + key, and sends the join frame.
func Dial(server, room, password string) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	conn, _, err := websocket.Dial(ctx, server, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	conn.SetReadLimit(12 << 20)
	c := &Client{
		conn:   conn,
		rid:    routingID(room),
		key:    deriveKey(room, password),
		ctx:    ctx,
		cancel: cancel,
	}
	if err := c.write(envelope{T: "join", Room: c.rid}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) write(e envelope) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	return c.conn.Write(wctx, websocket.MessageText, data)
}

// Send encrypts and sends a chat payload.
func (c *Client) Send(p payload) error {
	ct, err := encryptJSON(c.key, p)
	if err != nil {
		return err
	}
	return c.write(envelope{T: "msg", Room: c.rid, C: ct})
}

// ReadLoop pumps decoded events to out, then closes it on disconnect.
func (c *Client) ReadLoop(out chan<- Incoming) {
	defer close(out)
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			out <- Incoming{Kind: "closed"}
			return
		}
		var e envelope
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		switch e.T {
		case "presence":
			out <- Incoming{Kind: "presence", N: e.N}
		case "msg":
			var p payload
			if decryptJSON(c.key, e.C, &p) == nil {
				out <- Incoming{Kind: "msg", Msg: p}
			}
		}
	}
}

func (c *Client) Close() {
	c.cancel()
	_ = c.conn.CloseNow()
}

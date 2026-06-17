package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deaddrop/internal/hub"
	"deaddrop/internal/ws"

	"github.com/coder/websocket"
)

func testServer(maxFrame int64) *httptest.Server {
	h := hub.New()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.Serve(h, maxFrame, w, r)
	}))
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func writeJSON(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// readUntilMsg reads frames until a {t:"msg"} arrives (returning its `c`), or
// the timeout fires (ok=false). Presence frames are skipped.
func readUntilMsg(c *websocket.Conn, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return "", false
		}
		var env struct {
			T string `json:"t"`
			C string `json:"c"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.T == "msg" {
			return env.C, true
		}
	}
}

func readPresence(c *websocket.Conn, timeout time.Duration) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return 0, false
		}
		var env struct {
			T string `json:"t"`
			N int    `json:"n"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.T == "presence" {
			return env.N, true
		}
	}
}

// Messages must reach other sockets in the same room only, and never echo back
// to the sender. This is the core privacy boundary of the relay.
func TestRoomIsolationAndNoEcho(t *testing.T) {
	srv := testServer(0)
	defer srv.Close()

	a1 := dial(t, srv)
	defer a1.CloseNow()
	a2 := dial(t, srv)
	defer a2.CloseNow()
	b := dial(t, srv)
	defer b.CloseNow()

	writeJSON(t, a1, map[string]any{"t": "join", "room": "rA"})
	writeJSON(t, a2, map[string]any{"t": "join", "room": "rA"})
	writeJSON(t, b, map[string]any{"t": "join", "room": "rB"})
	time.Sleep(100 * time.Millisecond) // let joins settle

	writeJSON(t, a1, map[string]any{"t": "msg", "room": "rA", "c": "CIPHERTEXT"})

	if got, ok := readUntilMsg(a2, time.Second); !ok || got != "CIPHERTEXT" {
		t.Fatalf("a2 (same room) should receive the message; got %q ok=%v", got, ok)
	}
	if _, ok := readUntilMsg(b, 300*time.Millisecond); ok {
		t.Fatal("b (other room) must NOT receive the message")
	}
	if _, ok := readUntilMsg(a1, 300*time.Millisecond); ok {
		t.Fatal("sender must NOT receive its own message (no echo)")
	}
}

// A msg for a room the socket hasn't joined must not be relayed.
func TestMsgRequiresJoin(t *testing.T) {
	srv := testServer(0)
	defer srv.Close()

	sender := dial(t, srv)
	defer sender.CloseNow()
	listener := dial(t, srv)
	defer listener.CloseNow()

	writeJSON(t, listener, map[string]any{"t": "join", "room": "rX"})
	time.Sleep(50 * time.Millisecond)

	// sender never joined rX
	writeJSON(t, sender, map[string]any{"t": "msg", "room": "rX", "c": "NOPE"})

	if _, ok := readUntilMsg(listener, 300*time.Millisecond); ok {
		t.Fatal("message from a non-member must not be relayed")
	}
}

// Presence reports the live socket count as members join.
func TestPresenceCount(t *testing.T) {
	srv := testServer(0)
	defer srv.Close()

	a := dial(t, srv)
	defer a.CloseNow()
	writeJSON(t, a, map[string]any{"t": "join", "room": "rP"})
	if n, ok := readPresence(a, time.Second); !ok || n != 1 {
		t.Fatalf("expected presence n=1, got %d ok=%v", n, ok)
	}

	b := dial(t, srv)
	defer b.CloseNow()
	writeJSON(t, b, map[string]any{"t": "join", "room": "rP"})
	if n, ok := readPresence(a, time.Second); !ok || n != 2 {
		t.Fatalf("expected presence n=2 after b joins, got %d ok=%v", n, ok)
	}
}

// Frames larger than the configured limit are rejected and the connection is
// closed, so a client can't blow past MAX_FRAME_BYTES.
func TestFrameSizeLimit(t *testing.T) {
	srv := testServer(1024) // 1 KiB
	defer srv.Close()

	c := dial(t, srv)
	defer c.CloseNow()
	writeJSON(t, c, map[string]any{"t": "join", "room": "rL"})
	// Drain the join presence by waiting for it to arrive — a Read that times
	// out would itself close the connection (coder/websocket semantics).
	if _, ok := readPresence(c, time.Second); !ok {
		t.Fatal("expected a join presence frame")
	}

	writeJSON(t, c, map[string]any{"t": "msg", "room": "rL", "c": strings.Repeat("A", 4096)})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); err == nil {
		t.Fatal("expected the connection to close after an oversized frame")
	}
}

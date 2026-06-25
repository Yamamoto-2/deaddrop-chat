package hub_test

import (
	"testing"

	"deaddrop/internal/hub"
)

// Switching rooms must remove the client from the old room. Regression test for
// the ghost-reference panic: a client left in a room after its Send is closed
// would crash Broadcast (send on closed channel).
func TestJoinLeavesPreviousRoom(t *testing.T) {
	h := hub.New()
	c := &hub.Client{Send: make(chan []byte, 1)}

	if n, left, _ := h.Join("A", c); n != 1 || left != "" {
		t.Fatalf("first join: got count=%d left=%q, want 1 and no previous room", n, left)
	}
	n, left, leftN := h.Join("B", c)
	if n != 1 || left != "A" || leftN != 0 {
		t.Fatalf("switch join: got count=%d left=%q leftN=%d, want 1, \"A\", 0", n, left, leftN)
	}

	// Simulate the socket's teardown closing Send. If the client were still in
	// room A, the broadcast below would send on a closed channel and panic.
	close(c.Send)
	h.Broadcast("A", nil, []byte("ping")) // must not panic and must not target c
}

// Re-joining the same room is idempotent: no duplicate, count stays at 1.
func TestRejoinSameRoomIsIdempotent(t *testing.T) {
	h := hub.New()
	c := &hub.Client{Send: make(chan []byte, 1)}
	h.Join("A", c)
	if n, left, _ := h.Join("A", c); n != 1 || left != "" {
		t.Fatalf("rejoin: got count=%d left=%q, want 1 and no previous room", n, left)
	}
}

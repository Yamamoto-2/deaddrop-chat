// Package hub keeps the in-memory room registry and fans messages out.
// There is no persistence and no history buffer: a message only reaches
// sockets that are connected at the moment it arrives.
package hub

import "sync"

// Client is one connected socket. Send is drained by the socket's writer.
type Client struct {
	Send chan []byte
	Room string
}

// Hub maps an opaque routing id to the set of clients in that room.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]struct{}
}

// New returns an empty Hub.
func New() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]struct{})}
}

// Join adds c to room and returns the new occupant count.
func (h *Hub) Join(room string, c *Client) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.rooms[room]
	if !ok {
		set = make(map[*Client]struct{})
		h.rooms[room] = set
	}
	set[c] = struct{}{}
	c.Room = room
	return len(set)
}

// Leave removes c from its room and returns the remaining occupant count.
// Empty rooms are deleted so memory tracks only active conversations.
func (h *Hub) Leave(c *Client) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.rooms[c.Room]
	if !ok {
		return 0
	}
	delete(set, c)
	n := len(set)
	if n == 0 {
		delete(h.rooms, c.Room)
	}
	return n
}

// Broadcast delivers msg to every client in room except sender (pass nil to
// include everyone). Slow clients whose buffer is full are skipped, never
// blocking the relay.
func (h *Hub) Broadcast(room string, sender *Client, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[room] {
		if c == sender {
			continue
		}
		select {
		case c.Send <- msg:
		default:
		}
	}
}

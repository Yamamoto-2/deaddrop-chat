// Package ws bridges a WebSocket connection to the hub. It understands only
// the minimal envelope needed to route ({t, room}); message bodies are
// forwarded verbatim and never inspected.
package ws

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"deaddrop/internal/hub"

	"github.com/coder/websocket"
)

type envelope struct {
	T    string `json:"t"`
	Room string `json:"room"`
}

// Config tunes a Server's resource limits. Zero values disable the optional
// limits (idle timeout, per-IP cap, message rate), preserving the old behavior.
type Config struct {
	MaxFrame       int64         // SetReadLimit; <=0 = library default
	IdleTimeout    time.Duration // close a socket idle this long; <=0 = never
	MaxConnsPerIP  int           // reject beyond this many sockets per IP; <=0 = unlimited
	MsgBurst       float64       // per-socket token-bucket capacity for "msg" frames
	MsgRate        float64       // token refill per second; <=0 = unlimited
	TrustProxy     bool          // derive client IP from X-Forwarded-For
	AllowedOrigins []string      // browser Origin allow-list; nil/["*"] = any
}

// Server holds the shared state (hub + per-IP limiter) and serves WebSocket
// upgrades. Construct with NewServer, then use Serve as an http.HandlerFunc.
type Server struct {
	hub   *hub.Hub
	cfg   Config
	ipLim *ipLimiter
}

func NewServer(h *hub.Hub, cfg Config) *Server {
	origins := cfg.AllowedOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	cfg.AllowedOrigins = origins
	return &Server{hub: h, cfg: cfg, ipLim: newIPLimiter(cfg.MaxConnsPerIP)}
}

// Serve upgrades the request and pumps messages between the socket and hub.
func (s *Server) Serve(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, s.cfg.TrustProxy)
	if !s.ipLim.acquire(ip) {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	defer s.ipLim.release(ip)

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Browser Origin allow-list. Non-browser clients (no Origin header) are
		// always allowed, so locking this down does not break native/CLI clients.
		OriginPatterns: s.cfg.AllowedOrigins,
	})
	if err != nil {
		return
	}
	defer c.CloseNow()

	if s.cfg.MaxFrame > 0 {
		c.SetReadLimit(s.cfg.MaxFrame)
	}

	ctx := r.Context()
	client := &hub.Client{Send: make(chan []byte, 32)}
	bucket := newRateBucket(s.cfg.MsgBurst, s.cfg.MsgRate)

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
		// Reap sockets that go silent: every read carries an idle deadline, so a
		// connection that stops sending (a conforming client beacons regularly)
		// is closed instead of pinning a goroutine forever.
		rctx := ctx
		var cancel context.CancelFunc
		if s.cfg.IdleTimeout > 0 {
			rctx, cancel = context.WithTimeout(ctx, s.cfg.IdleTimeout)
		}
		_, data, err := c.Read(rctx)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			break
		}
		var env envelope
		if json.Unmarshal(data, &env) != nil || env.Room == "" {
			continue
		}
		switch env.T {
		case "join":
			n, leftRoom, leftN := s.hub.Join(env.Room, client)
			if leftRoom != "" {
				broadcastPresence(s.hub, leftRoom, leftN)
			}
			broadcastPresence(s.hub, env.Room, n)
		case "msg":
			// Rate-limit the one frame type that fans out to every peer.
			if !bucket.allow() {
				continue
			}
			if client.Room == env.Room {
				s.hub.Broadcast(env.Room, client, data)
			}
		}
	}

	n := s.hub.Leave(client)
	close(client.Send)
	<-writerDone
	if client.Room != "" {
		broadcastPresence(s.hub, client.Room, n)
	}
}

func broadcastPresence(h *hub.Hub, room string, n int) {
	msg, _ := json.Marshal(map[string]any{"t": "presence", "room": room, "n": n})
	h.Broadcast(room, nil, msg)
}

// clientIP returns the remote address used for per-IP limiting. Behind a trusted
// reverse proxy, RemoteAddr is the proxy, so optionally honor the left-most
// X-Forwarded-For hop. X-Forwarded-For is trivially spoofable, so only consult
// it when the operator opts in (TrustProxy).
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

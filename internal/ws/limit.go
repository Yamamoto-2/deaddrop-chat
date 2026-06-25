package ws

import (
	"sync"
	"time"
)

// ipLimiter caps the number of concurrent sockets from a single client IP.
// Best-effort: behind an untrusted proxy every client shares the proxy's IP,
// so this is a coarse abuse brake, not an authorization mechanism.
type ipLimiter struct {
	mu  sync.Mutex
	max int
	cur map[string]int
}

func newIPLimiter(max int) *ipLimiter {
	return &ipLimiter{max: max, cur: make(map[string]int)}
}

func (l *ipLimiter) acquire(ip string) bool {
	if l.max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur[ip] >= l.max {
		return false
	}
	l.cur[ip]++
	return true
}

func (l *ipLimiter) release(ip string) {
	if l.max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cur[ip] <= 1 {
		delete(l.cur, ip)
	} else {
		l.cur[ip]--
	}
}

// rateBucket is a single-goroutine token bucket (no locking: each socket owns
// its own, touched only from its read loop). A refill of <=0 disables limiting.
type rateBucket struct {
	tokens float64
	cap    float64
	refill float64 // tokens per second
	last   time.Time
}

func newRateBucket(burst, rate float64) *rateBucket {
	return &rateBucket{tokens: burst, cap: burst, refill: rate, last: time.Now()}
}

func (b *rateBucket) allow() bool {
	if b.refill <= 0 {
		return true
	}
	now := time.Now()
	b.tokens += now.Sub(b.last).Seconds() * b.refill
	if b.tokens > b.cap {
		b.tokens = b.cap
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

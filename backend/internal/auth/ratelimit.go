package auth

import (
	"sync"
	"time"
)

// rateLimiter throttles login attempts per client IP: after max failures within
// window, that IP is locked out for lockout. A success clears the counter.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt
	max      int
	window   time.Duration
	lockout  time.Duration
}

type attempt struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func newRateLimiter(max int, window, lockout time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string]*attempt),
		max:      max,
		window:   window,
		lockout:  lockout,
	}
}

func (l *rateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[ip]
	if a == nil {
		return true
	}
	return now.After(a.lockedUntil)
}

func (l *rateLimiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)

	a := l.attempts[ip]
	if a == nil || now.Sub(a.windowStart) > l.window {
		a = &attempt{windowStart: now}
		l.attempts[ip] = a
	}
	a.count++
	if a.count >= l.max {
		a.lockedUntil = now.Add(l.lockout)
	}
}

func (l *rateLimiter) success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// maxRateLimiterEntries hard-caps the map so a flood of distinct keys (e.g.
// spoofed addresses) cannot grow memory without bound.
const maxRateLimiterEntries = 4096

// pruneLocked drops stale entries and enforces a hard cap (called under lock).
func (l *rateLimiter) pruneLocked(now time.Time) {
	if len(l.attempts) < 1024 {
		return
	}
	// Drop entries that are both unlocked and past their window.
	for ip, a := range l.attempts {
		if now.After(a.lockedUntil) && now.Sub(a.windowStart) > l.window {
			delete(l.attempts, ip)
		}
	}
	// Hard cap: evict the oldest entries if still over the limit.
	for len(l.attempts) > maxRateLimiterEntries {
		var oldestIP string
		var oldest time.Time
		first := true
		for ip, a := range l.attempts {
			if first || a.windowStart.Before(oldest) {
				oldestIP, oldest, first = ip, a.windowStart, false
			}
		}
		delete(l.attempts, oldestIP)
	}
}

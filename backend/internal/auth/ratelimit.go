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

// pruneLocked drops stale entries to keep the map small (called under lock).
func (l *rateLimiter) pruneLocked(now time.Time) {
	if len(l.attempts) < 1024 {
		return
	}
	for ip, a := range l.attempts {
		if now.After(a.lockedUntil) && now.Sub(a.windowStart) > l.window {
			delete(l.attempts, ip)
		}
	}
}

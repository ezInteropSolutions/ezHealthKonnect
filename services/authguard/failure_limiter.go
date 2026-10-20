// Package authguard provides brute-force protection for credential checks on
// machine-to-machine endpoints (partner API keys / Basic / Bearer), where the
// interactive-login rate limiter in middleware/rateLimiter.js does not apply.
package authguard

import (
	"sync"
	"time"
)

// FailureLimiter tracks failed authentication attempts per key and blocks a key
// once it exceeds a threshold. Callers choose the key; for partner endpoints it
// is client IP + interface, so an attacker can only lock out their own address,
// never the legitimate partner's.
type FailureLimiter interface {
	// Blocked reports whether key is currently locked out, and for how long.
	Blocked(key string) (bool, time.Duration)
	// RecordFailure counts one failed attempt; returns true when this failure
	// triggered a new lockout.
	RecordFailure(key string) bool
	// Reset clears the key's history (call after a successful authentication).
	Reset(key string)
}

// Policy configures an InMemoryFailureLimiter.
type Policy struct {
	MaxFailures int           // failures allowed within Window before locking
	Window      time.Duration // sliding period over which failures are counted
	Lockout     time.Duration // how long a locked key stays blocked
}

// DefaultPolicy: 10 failures within 15 minutes → blocked for 15 minutes.
var DefaultPolicy = Policy{MaxFailures: 10, Window: 15 * time.Minute, Lockout: 15 * time.Minute}

type failureEntry struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

// InMemoryFailureLimiter is a process-local FailureLimiter. State is not shared
// across replicas — acceptable for a single-node deployment; a multi-replica
// deployment would supply a shared implementation of the same interface.
type InMemoryFailureLimiter struct {
	mu      sync.Mutex
	policy  Policy
	entries map[string]*failureEntry
	now     func() time.Time
	ops     int
}

// NewInMemoryFailureLimiter constructs a limiter; now may be nil (time.Now).
func NewInMemoryFailureLimiter(policy Policy, now func() time.Time) *InMemoryFailureLimiter {
	if now == nil {
		now = time.Now
	}
	return &InMemoryFailureLimiter{policy: policy, entries: map[string]*failureEntry{}, now: now}
}

func (l *InMemoryFailureLimiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return false, 0
	}
	if remaining := e.lockedUntil.Sub(l.now()); remaining > 0 {
		return true, remaining
	}
	return false, 0
}

func (l *InMemoryFailureLimiter) RecordFailure(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybePrune(now)

	e, ok := l.entries[key]
	if !ok || now.Sub(e.windowStart) > l.policy.Window {
		e = &failureEntry{windowStart: now}
		l.entries[key] = e
	}
	if now.Before(e.lockedUntil) {
		return false // already locked
	}
	e.count++
	if e.count >= l.policy.MaxFailures {
		e.lockedUntil = now.Add(l.policy.Lockout)
		e.count = 0
		e.windowStart = now
		return true
	}
	return false
}

func (l *InMemoryFailureLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// maybePrune drops expired entries every 256 operations so memory stays bounded
// by the number of distinct keys active within one window. Caller holds mu.
func (l *InMemoryFailureLimiter) maybePrune(now time.Time) {
	l.ops++
	if l.ops%256 != 0 {
		return
	}
	for k, e := range l.entries {
		if now.After(e.lockedUntil) && now.Sub(e.windowStart) > l.policy.Window {
			delete(l.entries, k)
		}
	}
}

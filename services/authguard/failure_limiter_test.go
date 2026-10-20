package authguard

import (
	"fmt"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter() (*InMemoryFailureLimiter, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	return NewInMemoryFailureLimiter(Policy{MaxFailures: 3, Window: time.Minute, Lockout: 5 * time.Minute}, clock.now), clock
}

func TestLocksAfterMaxFailures(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 1; i <= 2; i++ {
		if locked := l.RecordFailure("k"); locked {
			t.Fatalf("locked too early at failure %d", i)
		}
		if b, _ := l.Blocked("k"); b {
			t.Fatalf("blocked too early at failure %d", i)
		}
	}
	if !l.RecordFailure("k") {
		t.Fatal("third failure should trigger the lockout")
	}
	b, remaining := l.Blocked("k")
	if !b || remaining != 5*time.Minute {
		t.Fatalf("expected blocked for 5m, got %v %v", b, remaining)
	}
}

func TestLockoutExpires(t *testing.T) {
	l, clock := newTestLimiter()
	for i := 0; i < 3; i++ {
		l.RecordFailure("k")
	}
	clock.advance(5*time.Minute + time.Second)
	if b, _ := l.Blocked("k"); b {
		t.Fatal("lockout should have expired")
	}
}

func TestFailuresOutsideWindowDoNotAccumulate(t *testing.T) {
	l, clock := newTestLimiter()
	l.RecordFailure("k")
	l.RecordFailure("k")
	clock.advance(2 * time.Minute) // window is 1m
	if l.RecordFailure("k") {
		t.Fatal("old failures outside the window must not count toward the lockout")
	}
}

func TestResetClearsHistory(t *testing.T) {
	l, _ := newTestLimiter()
	l.RecordFailure("k")
	l.RecordFailure("k")
	l.Reset("k")
	if l.RecordFailure("k") {
		t.Fatal("a successful auth (Reset) must clear prior failures")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 3; i++ {
		l.RecordFailure("attacker|iface")
	}
	if b, _ := l.Blocked("partner|iface"); b {
		t.Fatal("an attacker's lockout must not block a different client")
	}
}

func TestPruneKeepsMemoryBounded(t *testing.T) {
	l, clock := newTestLimiter()
	for i := 0; i < 300; i++ {
		l.RecordFailure(fmt.Sprintf("k%d", i))
	}
	clock.advance(10 * time.Minute)
	for i := 0; i < 256; i++ {
		l.RecordFailure("fresh")
	}
	if n := len(l.entries); n > 2 {
		t.Fatalf("expired entries should be pruned, %d remain", n)
	}
}

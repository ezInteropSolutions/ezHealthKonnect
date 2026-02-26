// Package enrichment — unit tests for CircuitBreaker and global registry.
//
// No external services required — the circuit breaker is a pure in-memory
// state machine.  All tests use freshly-constructed breakers so the global
// registry state doesn't bleed between runs.
package enrichment

import (
	"os"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/services/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain initialises the structured logger so the enrichment package
// (which calls logger functions in the executors) doesn't panic with a nil logger.
func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// freshCB creates an isolated circuit breaker that does not touch the global registry.
func freshCB(threshold, openSecs int) *CircuitBreaker {
	return newCircuitBreaker(threshold, openSecs)
}

// ─── Circuit Breaker State Machine ──────────────────────────────────────────

func TestCBInitialStateClosed(t *testing.T) {
	cb := freshCB(3, 60)
	allowed, state := cb.Allow()
	assert.True(t, allowed, "initial state should allow requests")
	assert.Equal(t, "closed", state)
	assert.Equal(t, "closed", cb.StateName())
}

func TestCBTripsAfterThreshold(t *testing.T) {
	cb := freshCB(3, 60)

	// Record threshold-1 failures — breaker should still be closed.
	for i := 0; i < 2; i++ {
		allowed, _ := cb.Allow()
		require.True(t, allowed)
		cb.RecordFailure()
	}
	assert.Equal(t, "closed", cb.StateName(), "breaker should remain closed before threshold")

	// One more failure trips the breaker.
	allowed, _ := cb.Allow()
	require.True(t, allowed)
	cb.RecordFailure()
	assert.Equal(t, "open", cb.StateName(), "breaker should open after threshold failures")
}

func TestCBFastFailsWhenOpen(t *testing.T) {
	cb := freshCB(1, 60)

	// Trip the breaker with one failure.
	allowed, _ := cb.Allow()
	require.True(t, allowed)
	cb.RecordFailure()
	require.Equal(t, "open", cb.StateName())

	// Subsequent Allow() calls should be rejected.
	for i := 0; i < 5; i++ {
		allowed, state := cb.Allow()
		assert.False(t, allowed, "open breaker must reject calls")
		assert.Equal(t, "open", state)
	}
}

func TestCBTransitionsToHalfOpenAfterDuration(t *testing.T) {
	cb := freshCB(1, 0) // openDuration = 0 → immediately eligible for half-open
	// Force trip with duration=0 (applyDefaults makes it 1s minimum — override manually).
	// We manipulate the internal state directly for test speed.
	cb.mu.Lock()
	cb.state = cbOpen
	cb.openedAt = time.Now().Add(-2 * time.Second) // pretend opened 2s ago
	cb.openDuration = 1 * time.Second
	cb.mu.Unlock()

	allowed, state := cb.Allow()
	assert.True(t, allowed, "after openDuration, breaker should allow one probe")
	assert.Equal(t, "half-open", state)
}

func TestCBClosesOnHalfOpenSuccess(t *testing.T) {
	cb := freshCB(1, 1)
	// Put breaker into half-open manually.
	cb.mu.Lock()
	cb.state = cbHalfOpen
	cb.testInFlight = false
	cb.mu.Unlock()

	allowed, state := cb.Allow()
	require.True(t, allowed)
	require.Equal(t, "half-open", state)

	cb.RecordSuccess()
	assert.Equal(t, "closed", cb.StateName(), "success in half-open should close the breaker")
	// Failure count should be reset.
	cb.mu.Lock()
	assert.Equal(t, 0, cb.failureCount)
	cb.mu.Unlock()
}

func TestCBReopensOnHalfOpenFailure(t *testing.T) {
	cb := freshCB(1, 1)
	cb.mu.Lock()
	cb.state = cbHalfOpen
	cb.testInFlight = false
	cb.mu.Unlock()

	allowed, _ := cb.Allow()
	require.True(t, allowed)

	cb.RecordFailure()
	assert.Equal(t, "open", cb.StateName(), "failure in half-open should re-open the breaker")
}

func TestCBHalfOpenRaceGuard(t *testing.T) {
	// Two goroutines arrive simultaneously in half-open; only one should be allowed.
	cb := freshCB(1, 1)
	cb.mu.Lock()
	cb.state = cbHalfOpen
	cb.testInFlight = false
	cb.mu.Unlock()

	results := make([]bool, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := cb.Allow()
			results[i] = allowed
		}()
	}
	wg.Wait()

	allowedCount := 0
	for _, r := range results {
		if r {
			allowedCount++
		}
	}
	assert.Equal(t, 1, allowedCount, "exactly one goroutine should be allowed in half-open")
}

func TestCBSuccessResetsFailureCount(t *testing.T) {
	cb := freshCB(5, 60)

	// Record 4 failures (below threshold).
	for i := 0; i < 4; i++ {
		allowed, _ := cb.Allow()
		require.True(t, allowed)
		cb.RecordFailure()
	}

	// One success should reset the count.
	allowed, _ := cb.Allow()
	require.True(t, allowed)
	cb.RecordSuccess()

	cb.mu.Lock()
	assert.Equal(t, 0, cb.failureCount, "success should reset failure count")
	cb.mu.Unlock()
	assert.Equal(t, "closed", cb.StateName())
}

func TestCBConcurrentAccess(t *testing.T) {
	// 20 goroutines hammer the breaker concurrently — verifies no data races.
	// Run with: go test -race ./services/executors/enrichment/ -run TestCBConcurrentAccess
	cb := freshCB(10, 1)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if allowed, _ := cb.Allow(); allowed {
				cb.RecordFailure()
			}
		}()
	}
	wg.Wait()
	// After 10+ failures the breaker is open; state must be consistent.
	state := cb.StateName()
	assert.True(t, state == "open" || state == "closed" || state == "half-open",
		"state must be one of the three valid states, got %q", state)
}

// ─── Global Registry ────────────────────────────────────────────────────────

func TestGetCircuitBreakerSameKey(t *testing.T) {
	id := "test-step-same-key-" + t.Name()
	cb1 := GetCircuitBreaker(id, 5, 60)
	cb2 := GetCircuitBreaker(id, 5, 60)
	assert.Same(t, cb1, cb2, "same stepID must return the same CircuitBreaker pointer")
}

func TestGetCircuitBreakerDiffKeys(t *testing.T) {
	id1 := "test-step-diff-A-" + t.Name()
	id2 := "test-step-diff-B-" + t.Name()
	cb1 := GetCircuitBreaker(id1, 5, 60)
	cb2 := GetCircuitBreaker(id2, 5, 60)
	assert.NotSame(t, cb1, cb2, "different stepIDs must return different CircuitBreaker instances")
}

func TestGetCircuitBreakerDefaultThreshold(t *testing.T) {
	id := "test-step-defaults-" + t.Name()
	cb := GetCircuitBreaker(id, 0, 0) // zero values → defaults applied
	cb.mu.Lock()
	defer cb.mu.Unlock()
	assert.Equal(t, 5, cb.failureThreshold, "default failureThreshold should be 5")
	assert.Equal(t, 60*time.Second, cb.openDuration, "default openDuration should be 60s")
}

func TestGetCircuitBreakerConcurrentCreation(t *testing.T) {
	// 10 goroutines racing to GetCircuitBreaker for the same ID — only one instance
	// should be created (double-check locking must hold).
	id := "test-step-concurrent-" + t.Name()
	results := make([]*CircuitBreaker, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = GetCircuitBreaker(id, 3, 30)
		}()
	}
	wg.Wait()
	for i := 1; i < 10; i++ {
		assert.Same(t, results[0], results[i],
			"all goroutines must get the same CircuitBreaker instance")
	}
}

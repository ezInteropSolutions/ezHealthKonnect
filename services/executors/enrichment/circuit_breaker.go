// Package enrichment — circuit_breaker.go
//
// Per-step circuit breaker with three states:
//
//	closed   → normal operation; consecutive failures are counted
//	open     → fast-fail without calling the downstream API
//	half-open → one test request is let through; success resets the breaker,
//	            failure re-opens it
//
// A global registry (globalCBRegistry) keeps one CircuitBreaker per step ID so
// that the state is shared across goroutines executing the same step concurrently.
// The registry key is hex(SHA-256(stepID)), following the same pattern as
// services/dbpool.poolKey().
package enrichment

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"ezhealthkonnect/services/metrics"
)

// cbStateMetricValue maps a cbState to the CircuitBreakerState gauge value
// documented in services/metrics/metrics.go: 0=closed, 1=half-open, 2=open.
func cbStateMetricValue(s cbState) float64 {
	switch s {
	case cbOpen:
		return 2
	case cbHalfOpen:
		return 1
	default:
		return 0
	}
}

// ===============================================================
// STATE MACHINE
// ===============================================================

type cbState int

const (
	cbClosed   cbState = iota // normal — calls pass through
	cbOpen                    // tripped — calls fast-fail
	cbHalfOpen                // testing — one call allowed
)

// CircuitBreaker is a thread-safe three-state machine.
// All exported methods are safe for concurrent use.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            cbState
	failureCount     int
	failureThreshold int
	openDuration     time.Duration
	openedAt         time.Time
	testInFlight     bool // prevents double probe in half-open
	stepID           string // labels the CircuitBreakerState gauge (executor_id)
}

func newCircuitBreaker(stepID string, threshold int, openSecs int) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if openSecs <= 0 {
		openSecs = 60
	}
	cb := &CircuitBreaker{
		state:            cbClosed,
		failureThreshold: threshold,
		openDuration:     time.Duration(openSecs) * time.Second,
		stepID:           stepID,
	}
	if metrics.CircuitBreakerState != nil {
		metrics.CircuitBreakerState.WithLabelValues(stepID).Set(cbStateMetricValue(cbClosed))
	}
	return cb
}

// Allow reports whether the caller is permitted to attempt the operation.
// It also returns a human-readable state name for inclusion in execution
// details.  Callers MUST call RecordSuccess or RecordFailure after every
// call for which Allow returned true.
func (cb *CircuitBreaker) Allow() (allowed bool, stateName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case cbClosed:
		return true, "closed"

	case cbOpen:
		if time.Since(cb.openedAt) < cb.openDuration {
			// Still within open window — fast fail.
			return false, "open"
		}
		// Transition to half-open; allow one probe request.
		cb.state = cbHalfOpen
		cb.testInFlight = true
		if metrics.CircuitBreakerState != nil {
			metrics.CircuitBreakerState.WithLabelValues(cb.stepID).Set(cbStateMetricValue(cbHalfOpen))
		}
		return true, "half-open"

	case cbHalfOpen:
		if cb.testInFlight {
			// A probe is already in flight; fast-fail this caller.
			return false, "open"
		}
		// Previous probe completed (shouldn't reach here normally, but be safe).
		cb.testInFlight = true
		return true, "half-open"
	}

	return true, "closed"
}

// RecordSuccess records a successful operation and resets the breaker if it
// was in the half-open state.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.testInFlight = false
	cb.state = cbClosed
	if metrics.CircuitBreakerState != nil {
		metrics.CircuitBreakerState.WithLabelValues(cb.stepID).Set(cbStateMetricValue(cbClosed))
	}
}

// RecordFailure records a failed operation and may trip or re-open the breaker.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.testInFlight = false

	switch cb.state {
	case cbHalfOpen:
		// Probe failed — re-open the breaker.
		cb.state = cbOpen
		cb.openedAt = time.Now()

	case cbClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.state = cbOpen
			cb.openedAt = time.Now()
		}

	// If already open, a failure cannot arrive (Allow returns false),
	// so this case is a no-op guard.
	case cbOpen:
	}

	if metrics.CircuitBreakerState != nil {
		metrics.CircuitBreakerState.WithLabelValues(cb.stepID).Set(cbStateMetricValue(cb.state))
	}
}

// StateName returns the current state as a string without acquiring extra locks.
// This is safe to call after Allow / RecordSuccess / RecordFailure returns.
func (cb *CircuitBreaker) StateName() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return stateName(cb.state)
}

func stateName(s cbState) string {
	switch s {
	case cbClosed:
		return "closed"
	case cbOpen:
		return "open"
	case cbHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ===============================================================
// GLOBAL REGISTRY
// ===============================================================

type cbRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
}

// globalCBRegistry is the process-wide singleton.
var globalCBRegistry = &cbRegistry{breakers: make(map[string]*CircuitBreaker)}

// GetCircuitBreaker returns the CircuitBreaker for the given stepID, creating
// one if it does not yet exist.  threshold and openSecs are only applied on
// creation; subsequent calls with different values do not reconfigure an
// existing breaker.
func GetCircuitBreaker(stepID string, threshold int, openSecs int) *CircuitBreaker {
	key := cbKey(stepID)

	// Fast path — breaker already exists.
	globalCBRegistry.mu.RLock()
	if cb, ok := globalCBRegistry.breakers[key]; ok {
		globalCBRegistry.mu.RUnlock()
		return cb
	}
	globalCBRegistry.mu.RUnlock()

	// Slow path — create under write lock (double-check pattern).
	globalCBRegistry.mu.Lock()
	defer globalCBRegistry.mu.Unlock()

	if cb, ok := globalCBRegistry.breakers[key]; ok {
		return cb
	}

	cb := newCircuitBreaker(stepID, threshold, openSecs)
	globalCBRegistry.breakers[key] = cb
	return cb
}

// cbKey returns an opaque key derived from the step ID.  Keeping credentials
// or user data out of map keys matches the dbpool pattern.
func cbKey(stepID string) string {
	h := sha256.Sum256([]byte(stepID))
	return fmt.Sprintf("%x", h)
}

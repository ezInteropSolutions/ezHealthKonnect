package services

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunEveryCallsUntilCancelledAndSurvivesAPanic(t *testing.T) {
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runEvery(ctx, 10*time.Millisecond, func(context.Context) {
			if atomic.AddInt32(&calls, 1) == 1 {
				panic("first call fails")
			}
		})
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) < 3 {
		select {
		case <-deadline:
			t.Fatalf("only %d calls", atomic.LoadInt32(&calls))
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not stop when cancelled")
	}
}

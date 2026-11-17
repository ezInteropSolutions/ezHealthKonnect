package backpressure

import (
	"sync/atomic"
	"testing"
	"time"
)

// A burst far larger than the queue is accepted in full when callers wait,
// and every job runs; plain Submit refuses the overflow.
func TestSubmitWaitAbsorbsBursts(t *testing.T) {
	p := NewWorkerPool("test-burst", 2, 5)
	defer p.Shutdown()
	var ran atomic.Int64
	job := func() { time.Sleep(2 * time.Millisecond); ran.Add(1) }

	const n = 200
	for i := 0; i < n; i++ {
		if !p.SubmitWait(job, 5*time.Second) {
			t.Fatalf("job %d refused", i)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for ran.Load() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := ran.Load(); got != n {
		t.Fatalf("%d of %d jobs ran", got, n)
	}
}

func TestSubmitWaitGivesUpWhenStuck(t *testing.T) {
	p := NewWorkerPool("test-stuck", 1, 1)
	block := make(chan struct{})
	defer func() { close(block); p.Shutdown() }()
	stuck := func() { <-block }
	// Fill the pool: the worker, the job the dispatcher holds waiting for a
	// worker, and the queue, until plain Submit refuses.
	for i := 0; i < 10 && p.Submit(stuck); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Submit(stuck) {
		t.Fatal("pool never filled")
	}
	start := time.Now()
	if p.SubmitWait(stuck, 50*time.Millisecond) {
		t.Fatal("accepted with no space")
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("didn't wait")
	}
}

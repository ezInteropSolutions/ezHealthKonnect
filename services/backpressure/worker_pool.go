// Package backpressure provides per-interface bounded worker pools to prevent
// unbounded goroutine spawning under load (P9 — Backpressure).
package backpressure

import (
	"sync"
	"sync/atomic"
)

// WorkerPool is a bounded concurrency pool backed by a semaphore channel and
// a buffered job queue. Each interface gets its own pool via the Registry.
type WorkerPool struct {
	semaphore   chan struct{}
	jobs        chan func()
	maxWorkers  int
	maxQueue    int
	queued      atomic.Int64
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewWorkerPool creates a pool that runs up to maxWorkers goroutines concurrently
// and buffers up to maxQueueDepth pending jobs before rejecting new submissions.
func NewWorkerPool(maxWorkers, maxQueueDepth int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if maxQueueDepth <= 0 {
		maxQueueDepth = 100
	}
	p := &WorkerPool{
		semaphore:  make(chan struct{}, maxWorkers),
		jobs:       make(chan func(), maxQueueDepth),
		maxWorkers: maxWorkers,
		maxQueue:   maxQueueDepth,
		done:       make(chan struct{}),
	}
	// Pre-fill semaphore slots
	for i := 0; i < maxWorkers; i++ {
		p.semaphore <- struct{}{}
	}
	go p.dispatch()
	return p
}

// Submit enqueues fn for execution. Returns false if the queue is full (caller
// should NACK or drop the message). Non-blocking.
func (p *WorkerPool) Submit(fn func()) bool {
	select {
	case p.jobs <- fn:
		p.queued.Add(1)
		return true
	default:
		return false // queue full → backpressure signal
	}
}

// QueueDepth returns the number of pending (not yet started) jobs.
func (p *WorkerPool) QueueDepth() int {
	return len(p.jobs)
}

// ActiveWorkers returns the number of goroutines currently running a job.
func (p *WorkerPool) ActiveWorkers() int {
	return p.maxWorkers - len(p.semaphore)
}

// AtCapacity returns true when the queue is at or above pct% of maximum depth.
// e.g. AtCapacity(0.90) → true when ≥90% full.
func (p *WorkerPool) AtCapacity(pct float64) bool {
	return float64(len(p.jobs)) >= float64(p.maxQueue)*pct
}

// MaxWorkers returns the configured worker limit.
func (p *WorkerPool) MaxWorkers() int { return p.maxWorkers }

// MaxQueue returns the configured queue depth limit.
func (p *WorkerPool) MaxQueue() int { return p.maxQueue }

// Shutdown signals the dispatcher to stop after draining all queued jobs.
// Blocks until all running jobs complete.
func (p *WorkerPool) Shutdown() {
	close(p.done)
	p.wg.Wait()
}

// dispatch is the internal goroutine that fans out jobs to worker goroutines.
func (p *WorkerPool) dispatch() {
	for {
		select {
		case fn, ok := <-p.jobs:
			if !ok {
				return
			}
			// Acquire a semaphore slot (blocks if maxWorkers are busy)
			<-p.semaphore
			p.wg.Add(1)
			go func(job func()) {
				defer func() {
					p.semaphore <- struct{}{} // release slot
					p.wg.Done()
				}()
				job()
			}(fn)
		case <-p.done:
			// Drain remaining queued jobs before exiting
			for {
				select {
				case fn := <-p.jobs:
					<-p.semaphore
					p.wg.Add(1)
					go func(job func()) {
						defer func() {
							p.semaphore <- struct{}{}
							p.wg.Done()
						}()
						job()
					}(fn)
				default:
					return
				}
			}
		}
	}
}

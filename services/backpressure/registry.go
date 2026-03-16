package backpressure

import "sync"

// Registry manages one WorkerPool per interface (thread-safe).
type Registry struct {
	mu    sync.RWMutex
	pools map[string]*WorkerPool
	// Default pool sizing — can be overridden per interface
	defaultMaxWorkers int
	defaultMaxQueue   int
}

var globalRegistry = &Registry{
	pools:             make(map[string]*WorkerPool),
	defaultMaxWorkers: 10,
	defaultMaxQueue:   100,
}

// Get returns the package-level singleton registry.
func Get() *Registry { return globalRegistry }

// GetOrCreate returns the pool for interfaceID, creating it on first use with
// the registry defaults. Subsequent calls with the same ID return the same pool.
func (r *Registry) GetOrCreate(interfaceID string) *WorkerPool {
	// Fast path
	r.mu.RLock()
	if p, ok := r.pools[interfaceID]; ok {
		r.mu.RUnlock()
		return p
	}
	r.mu.RUnlock()

	// Slow path — double-check
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pools[interfaceID]; ok {
		return p
	}
	p := NewWorkerPool(r.defaultMaxWorkers, r.defaultMaxQueue)
	r.pools[interfaceID] = p
	return p
}

// GetOrCreateWith returns a pool for interfaceID using the supplied sizing,
// creating it only if it doesn't already exist.
func (r *Registry) GetOrCreateWith(interfaceID string, maxWorkers, maxQueue int) *WorkerPool {
	r.mu.RLock()
	if p, ok := r.pools[interfaceID]; ok {
		r.mu.RUnlock()
		return p
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pools[interfaceID]; ok {
		return p
	}
	p := NewWorkerPool(maxWorkers, maxQueue)
	r.pools[interfaceID] = p
	return p
}

// QueueDepths returns a snapshot of {interfaceID: {queue_depth, active_workers, max_workers, max_queue}}.
func (r *Registry) QueueDepths() map[string]map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]map[string]int, len(r.pools))
	for id, p := range r.pools {
		out[id] = map[string]int{
			"queue_depth":    p.QueueDepth(),
			"active_workers": p.ActiveWorkers(),
			"max_workers":    p.MaxWorkers(),
			"max_queue":      p.MaxQueue(),
		}
	}
	return out
}

// Remove shuts down and removes the pool for interfaceID (called on interface deactivation).
func (r *Registry) Remove(interfaceID string) {
	r.mu.Lock()
	p, ok := r.pools[interfaceID]
	if ok {
		delete(r.pools, interfaceID)
	}
	r.mu.Unlock()
	if ok {
		p.Shutdown()
	}
}

// ShutdownAll shuts down all pools (called on process exit).
func (r *Registry) ShutdownAll() {
	r.mu.Lock()
	pools := make(map[string]*WorkerPool, len(r.pools))
	for id, p := range r.pools {
		pools[id] = p
	}
	r.pools = make(map[string]*WorkerPool)
	r.mu.Unlock()

	for _, p := range pools {
		p.Shutdown()
	}
}

// SetDefaults configures default pool sizes for new pools.
func (r *Registry) SetDefaults(maxWorkers, maxQueue int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultMaxWorkers = maxWorkers
	r.defaultMaxQueue = maxQueue
}

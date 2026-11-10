package ai

import "sync"

// IngestProgress reports background knowledge indexing to the status endpoint, so a slow assistant has a visible
// reason ("Indexing interfaces: 120 of 970") instead of silently queueing questions behind the embedding model.
// Safe for concurrent use; every method is a no-op on a nil receiver.
type IngestProgress struct {
	mu      sync.Mutex
	phase   string
	total   int
	done    int
	changed int
	active  bool
}

// IndexingStatus is the JSON shape of IngestProgress.
type IndexingStatus struct {
	Running bool   `json:"running"`
	Phase   string `json:"phase,omitempty"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	// Changed counts the items that actually had to be embedded (the rest were already up to date).
	Changed int `json:"changed"`
}

func NewIngestProgress() *IngestProgress { return &IngestProgress{} }

// Begin starts a phase of `total` items.
func (p *IngestProgress) Begin(phase string, total int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase, p.total, p.done, p.changed, p.active = phase, total, 0, 0, true
}

// Step records one item finished; skipped means it was already up to date and cost nothing.
func (p *IngestProgress) Step(skipped bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	if !skipped {
		p.changed++
	}
}

// End finishes the current phase.
func (p *IngestProgress) End() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = false
}

// Snapshot returns the current state.
func (p *IngestProgress) Snapshot() IndexingStatus {
	if p == nil {
		return IndexingStatus{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return IndexingStatus{Running: p.active, Phase: p.phase, Done: p.done, Total: p.total, Changed: p.changed}
}

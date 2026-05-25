// services/ai/llm_queue.go
// LLMQueue wraps any LLMProvider with a counting semaphore that limits
// concurrent in-flight requests and prevents overloading single-threaded models.
package ai

import (
	"context"
	"fmt"
	"time"
)

// LLMQueue is a transparent LLMProvider wrapper with back-pressure.
// All calls block until a concurrency slot is available or the timeout elapses.
type LLMQueue struct {
	inner   LLMProvider
	sem     chan struct{} // buffered channel: cap = maxConcurrent
	timeout time.Duration
}

// NewLLMQueue wraps inner with a semaphore of size maxConcurrent.
// queueTimeout is how long a caller will wait before receiving a 503.
func NewLLMQueue(inner LLMProvider, maxConcurrent int, queueTimeout time.Duration) *LLMQueue {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	if queueTimeout <= 0 {
		queueTimeout = 30 * time.Second
	}
	return &LLMQueue{
		inner:   inner,
		sem:     make(chan struct{}, maxConcurrent),
		timeout: queueTimeout,
	}
}

// Depth returns the current number of in-flight requests.
func (q *LLMQueue) Depth() int { return len(q.sem) }

// Cap returns the maximum concurrent requests allowed.
func (q *LLMQueue) Cap() int { return cap(q.sem) }

// acquire blocks until a slot is available or times out.
func (q *LLMQueue) acquire(ctx context.Context) error {
	select {
	case q.sem <- struct{}{}:
		return nil
	case <-time.After(q.timeout):
		return fmt.Errorf("AI queue full (max %d concurrent) — please try again in a moment", cap(q.sem))
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *LLMQueue) release() { <-q.sem }

// ─── LLMProvider interface ────────────────────────────────────────────────────

func (q *LLMQueue) Generate(ctx context.Context, prompt string) (string, error) {
	if err := q.acquire(ctx); err != nil {
		return "", err
	}
	defer q.release()
	return q.inner.Generate(ctx, prompt)
}

func (q *LLMQueue) GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error {
	if err := q.acquire(ctx); err != nil {
		return err
	}
	defer q.release()
	return q.inner.GenerateStream(ctx, prompt, onToken)
}

func (q *LLMQueue) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := q.acquire(ctx); err != nil {
		return nil, err
	}
	defer q.release()
	return q.inner.Embed(ctx, text)
}

func (q *LLMQueue) Ping(ctx context.Context) error      { return q.inner.Ping(ctx) }
func (q *LLMQueue) ProviderName() string                { return q.inner.ProviderName() }
func (q *LLMQueue) ChatModelName() string               { return q.inner.ChatModelName() }

// EmbedModelName forwards to the inner provider if it is an EmbedProvider.
func (q *LLMQueue) EmbedModelName() string {
	if ep, ok := q.inner.(EmbedProvider); ok {
		return ep.EmbedModelName()
	}
	return ""
}

// ListModels forwards to the inner provider if it is an EmbedProvider.
func (q *LLMQueue) ListModels(ctx context.Context) ([]string, error) {
	if ep, ok := q.inner.(EmbedProvider); ok {
		return ep.ListModels(ctx)
	}
	return nil, nil
}

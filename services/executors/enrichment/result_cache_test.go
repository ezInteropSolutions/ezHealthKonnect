// Package enrichment — unit tests for ResultCache and global registry.
//
// No external services required — the result cache is a pure in-memory
// data structure.  All tests use freshly-created caches so the global
// registry state doesn't bleed between runs.
package enrichment

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: TestMain is defined in circuit_breaker_test.go and initialises the
// logger for the whole enrichment_test binary.

// ─── Helpers ────────────────────────────────────────────────────────────────

func freshCache(maxEntries int) *ResultCache {
	return NewResultCache(maxEntries)
}

func makeValue(tag string) map[string]interface{} {
	return map[string]interface{}{"tag": tag, "ts": time.Now().UnixNano()}
}

// ─── Core Get / Set ─────────────────────────────────────────────────────────

func TestCacheHitAndMiss(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-1", "GET", "https://api.example.com/patient/123")

	// Miss before Set.
	val, ok := c.Get(key)
	assert.False(t, ok, "miss expected before Set")
	assert.Nil(t, val)

	// Set then Get.
	c.Set(key, makeValue("first"), 5*time.Second)
	val, ok = c.Get(key)
	require.True(t, ok, "hit expected after Set")
	assert.Equal(t, "first", val["tag"])
}

func TestCacheTTLExpiry(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-ttl")

	c.Set(key, makeValue("expiring"), 50*time.Millisecond)

	// Immediately readable.
	_, ok := c.Get(key)
	require.True(t, ok, "entry should be readable before TTL expires")

	// After TTL expires it should be evicted.
	time.Sleep(70 * time.Millisecond)
	_, ok = c.Get(key)
	assert.False(t, ok, "expired entry should not be returned")
}

func TestCacheZeroTTLIsNoop(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-zero-ttl")

	// Set with ttl=0 should be a no-op.
	c.Set(key, makeValue("zero"), 0)
	_, ok := c.Get(key)
	assert.False(t, ok, "Set with zero TTL should not store the entry")
}

func TestCacheUpdateExistingEntry(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-update")

	c.Set(key, makeValue("v1"), 5*time.Second)
	c.Set(key, makeValue("v2"), 5*time.Second)

	val, ok := c.Get(key)
	require.True(t, ok)
	assert.Equal(t, "v2", val["tag"], "subsequent Set should update the value")
}

// ─── LRU Eviction ───────────────────────────────────────────────────────────

func TestCacheLRUEviction(t *testing.T) {
	maxEntries := 3
	c := freshCache(maxEntries)

	// Fill to capacity.
	for i := 0; i < maxEntries; i++ {
		k := CacheKey(fmt.Sprintf("step-%d", i))
		c.Set(k, makeValue(fmt.Sprintf("v%d", i)), time.Minute)
	}
	assert.Equal(t, maxEntries, c.Len(), "cache should be at capacity")

	// Access key 0 and key 1 to make key 2 the LRU candidate.
	k0 := CacheKey("step-0")
	k1 := CacheKey("step-1")
	k2 := CacheKey("step-2")
	_, ok0 := c.Get(k0)
	_, ok1 := c.Get(k1)
	require.True(t, ok0 && ok1)

	// Insert one more — key 2 (LRU) should be evicted.
	k3 := CacheKey("step-3")
	c.Set(k3, makeValue("v3"), time.Minute)

	assert.Equal(t, maxEntries, c.Len(), "cache must not exceed maxEntries")
	_, stillHere := c.Get(k2)
	assert.False(t, stillHere, "LRU entry (step-2) should have been evicted")
	_, ok3 := c.Get(k3)
	assert.True(t, ok3, "newly inserted entry should be present")
}

func TestCacheLRUDoesNotExceedCapacity(t *testing.T) {
	c := freshCache(5)
	for i := 0; i < 20; i++ {
		k := CacheKey(fmt.Sprintf("step-many-%d", i))
		c.Set(k, makeValue(fmt.Sprintf("v%d", i)), time.Minute)
	}
	assert.LessOrEqual(t, c.Len(), 5, "cache must not grow beyond maxEntries")
}

// ─── Deep Copy Isolation ────────────────────────────────────────────────────

func TestCacheDeepCopyOnSet(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-copy-set")

	original := map[string]interface{}{"nested": map[string]interface{}{"x": 1}}
	c.Set(key, original, time.Minute)

	// Mutate original after Set — cache should be unaffected.
	original["nested"].(map[string]interface{})["x"] = 99

	val, _ := c.Get(key)
	nested := val["nested"].(map[string]interface{})
	assert.Equal(t, 1, nested["x"], "cache must store a deep copy, not the original reference")
}

func TestCacheDeepCopyOnGet(t *testing.T) {
	c := freshCache(10)
	key := CacheKey("step-copy-get")

	c.Set(key, map[string]interface{}{"nested": map[string]interface{}{"y": 42}}, time.Minute)

	// Mutate the returned value — cache should be unaffected.
	val, _ := c.Get(key)
	val["nested"].(map[string]interface{})["y"] = 0

	val2, _ := c.Get(key)
	nested := val2["nested"].(map[string]interface{})
	assert.Equal(t, 42, nested["y"], "Get must return a deep copy, not the cached reference")
}

// ─── Cache Key ──────────────────────────────────────────────────────────────

func TestCacheKeyDeterministic(t *testing.T) {
	k1 := CacheKey("step-1", "GET", "https://api.example.com/patient/123")
	k2 := CacheKey("step-1", "GET", "https://api.example.com/patient/123")
	assert.Equal(t, k1, k2, "same inputs must produce the same cache key")
}

func TestCacheKeyDifferentForDifferentInputs(t *testing.T) {
	k1 := CacheKey("step-A", "GET", "https://api.example.com/1")
	k2 := CacheKey("step-B", "GET", "https://api.example.com/1")
	assert.NotEqual(t, k1, k2, "different step IDs must produce different keys")

	k3 := CacheKey("step-A", "GET", "https://api.example.com/2")
	assert.NotEqual(t, k1, k3, "different URLs must produce different keys")
}

func TestCacheKeyIsHex(t *testing.T) {
	k := CacheKey("step", "param")
	assert.Regexp(t, `^[0-9a-f]+$`, k, "cache key must be lowercase hex")
	assert.Len(t, k, 64, "SHA-256 hex should be 64 chars")
}

// ─── Concurrent Access ──────────────────────────────────────────────────────

func TestCacheConcurrentReadWrite(t *testing.T) {
	// 10 readers and 10 writers hammering the same cache concurrently.
	// Run with: go test -race ./services/executors/enrichment/ -run TestCacheConcurrentReadWrite
	c := freshCache(50)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		i := i
		// Writer goroutine.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				k := CacheKey(fmt.Sprintf("concurrent-key-%d-%d", i, j))
				c.Set(k, makeValue(fmt.Sprintf("v%d", j)), time.Minute)
			}
		}()
		// Reader goroutine.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				k := CacheKey(fmt.Sprintf("concurrent-key-%d-%d", i, j))
				c.Get(k) //nolint:errcheck // result intentionally ignored
			}
		}()
	}
	wg.Wait()
	assert.LessOrEqual(t, c.Len(), 50, "concurrent writes must not exceed maxEntries")
}

// ─── Global Registry ────────────────────────────────────────────────────────

func TestGetResultCacheSingleton(t *testing.T) {
	cacheType := "api_enrichment_test_singleton_" + t.Name()
	rc1 := GetResultCache(cacheType, 100)
	rc2 := GetResultCache(cacheType, 200) // different maxEntries — ignored after creation
	assert.Same(t, rc1, rc2, "same cacheType must return the same ResultCache pointer")
}

func TestGetResultCacheDifferentTypes(t *testing.T) {
	rc1 := GetResultCache("api_enrichment_test_A_"+t.Name(), 100)
	rc2 := GetResultCache("db_enrichment_test_A_"+t.Name(), 100)
	assert.NotSame(t, rc1, rc2, "different cacheTypes must return different ResultCache instances")
}

func TestGetResultCacheConcurrentCreation(t *testing.T) {
	cacheType := "concurrent_cache_" + t.Name()
	results := make([]*ResultCache, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = GetResultCache(cacheType, 500)
		}()
	}
	wg.Wait()
	for i := 1; i < 10; i++ {
		assert.Same(t, results[0], results[i],
			"all goroutines must get the same ResultCache instance")
	}
}

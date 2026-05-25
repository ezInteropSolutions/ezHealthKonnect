// Package enrichment — result_cache.go
//
// In-memory LRU + TTL cache for enrichment executor results.
//
// Design goals:
//   - Zero external dependencies (uses only stdlib container/list)
//   - Thread-safe (single sync.Mutex guards both map and LRU list)
//   - TTL per entry (expired entries are evicted lazily on next Get)
//   - LRU eviction when maxEntries is reached
//   - Deep copy on both Set (protect cache from caller mutations) and
//     Get (protect caller from future cache mutations)
//
// A global registry (globalCacheRegistry) holds one ResultCache per
// logical cache type (e.g. "api_enrichment", "db_enrichment") so the
// cache survives across requests and executor instances.
package enrichment

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// ===============================================================
// CACHE ENTRY & CORE STRUCT
// ===============================================================

type cacheEntry struct {
	key       string
	value     map[string]interface{}
	expiresAt time.Time
	elem      *list.Element // position in LRU doubly-linked list
}

// ResultCache is a thread-safe in-memory LRU+TTL cache.
type ResultCache struct {
	mu         sync.Mutex
	entries    map[string]*cacheEntry
	lruList    *list.List // front = most recently used, back = LRU candidate
	maxEntries int
}

// NewResultCache creates a new empty cache with the given capacity.
// maxEntries must be > 0; negative or zero values are replaced with 1000.
func NewResultCache(maxEntries int) *ResultCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	return &ResultCache{
		entries:    make(map[string]*cacheEntry, maxEntries),
		lruList:    list.New(),
		maxEntries: maxEntries,
	}
}

// Get returns a deep copy of the cached value and true if the key exists
// and has not expired.  An expired entry is removed and (nil, false) is
// returned.
func (c *ResultCache) Get(key string) (map[string]interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Lazy expiry — remove stale entry.
		c.lruList.Remove(entry.elem)
		delete(c.entries, key)
		return nil, false
	}

	// Move to front (most recently used).
	c.lruList.MoveToFront(entry.elem)
	return deepCopyMap(entry.value), true
}

// Set stores a deep copy of value under key with the given TTL.
// If the cache is full, the least-recently-used entry is evicted first.
// A zero or negative TTL is a no-op (caching disabled for this call).
func (c *ResultCache) Set(key string, value map[string]interface{}, ttl time.Duration) {
	if ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry if present.
	if entry, ok := c.entries[key]; ok {
		entry.value = deepCopyMap(value)
		entry.expiresAt = time.Now().Add(ttl)
		c.lruList.MoveToFront(entry.elem)
		return
	}

	// Evict LRU entry if at capacity.
	if len(c.entries) >= c.maxEntries {
		back := c.lruList.Back()
		if back != nil {
			evicted := back.Value.(*cacheEntry)
			c.lruList.Remove(back)
			delete(c.entries, evicted.key)
		}
	}

	// Insert new entry at the front.
	entry := &cacheEntry{
		key:       key,
		value:     deepCopyMap(value),
		expiresAt: time.Now().Add(ttl),
	}
	entry.elem = c.lruList.PushFront(entry)
	c.entries[key] = entry
}

// Len returns the current number of entries in the cache (including expired
// entries that have not yet been evicted).
func (c *ResultCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// ===============================================================
// GLOBAL REGISTRY
// ===============================================================

type cacheRegistry struct {
	mu     sync.RWMutex
	caches map[string]*ResultCache
}

var globalCacheRegistry = &cacheRegistry{caches: make(map[string]*ResultCache)}

// GetResultCache returns the ResultCache for the given cacheType, creating
// one with maxEntries capacity if it does not yet exist.  Subsequent calls
// with the same cacheType but different maxEntries still return the same
// cache (configuration is fixed at creation time).
func GetResultCache(cacheType string, maxEntries int) *ResultCache {
	// Fast path.
	globalCacheRegistry.mu.RLock()
	if rc, ok := globalCacheRegistry.caches[cacheType]; ok {
		globalCacheRegistry.mu.RUnlock()
		return rc
	}
	globalCacheRegistry.mu.RUnlock()

	// Slow path — create under write lock (double-check pattern).
	globalCacheRegistry.mu.Lock()
	defer globalCacheRegistry.mu.Unlock()

	if rc, ok := globalCacheRegistry.caches[cacheType]; ok {
		return rc
	}

	rc := NewResultCache(maxEntries)
	globalCacheRegistry.caches[cacheType] = rc
	return rc
}

// ===============================================================
// CACHE KEY HELPER
// ===============================================================

// CacheKey returns a stable, opaque SHA-256 hex key built from the
// concatenation of all parts joined by "::".  Parts containing sensitive
// data (credentials, tokens) should NOT be included — only the values
// that materially affect the query result.
func CacheKey(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte("::")) //nolint:errcheck
		}
		h.Write([]byte(p)) //nolint:errcheck
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ===============================================================
// HELPER: DEEP COPY
// ===============================================================

// deepCopyMap returns a shallow-to-one-level deep copy of a
// map[string]interface{}.  Nested maps are recursively copied; slices
// and primitive values are copied by value.  This prevents callers from
// mutating cached data and cached data from leaking into caller output.
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch typed := v.(type) {
		case map[string]interface{}:
			dst[k] = deepCopyMap(typed)
		case []interface{}:
			cp := make([]interface{}, len(typed))
			copy(cp, typed)
			dst[k] = cp
		default:
			dst[k] = v
		}
	}
	return dst
}

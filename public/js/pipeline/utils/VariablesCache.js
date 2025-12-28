// ═══════════════════════════════════════════════════════════════
// REFERENCE VARIABLES CACHE - Smart LRU caching for performance
// ═══════════════════════════════════════════════════════════════

class ReferenceVariablesCache {
    constructor(config = {}) {
        this.cache = new Map();
        this.ttl = config.ttl || 5 * 60 * 1000; // 5 minutes default
        this.maxSize = config.maxSize || 50; // 50 pipelines max
        this.hits = 0;
        this.misses = 0;
    }

    /**
     * Generate cache key from pipeline state
     */
    _getCacheKey(pipelineId, pipelineVersion) {
        return `${pipelineId}:${pipelineVersion}`;
    }

    /**
     * Get cached variables if valid
     */
    get(pipelineId, pipelineVersion) {
        const key = this._getCacheKey(pipelineId, pipelineVersion);
        const cached = this.cache.get(key);

        if (!cached) {
            this.misses++;
            console.log('📦 Cache MISS for pipeline:', pipelineId);
            return null;
        }

        // Check TTL
        if (Date.now() - cached.timestamp > this.ttl) {
            this.cache.delete(key);
            this.misses++;
            console.log('⏰ Cache EXPIRED for pipeline:', pipelineId);
            return null;
        }

        this.hits++;
        console.log('✅ Cache HIT for pipeline:', pipelineId, `(${this.getHitRate()}% hit rate)`);
        return cached.data;
    }

    /**
     * Store variables in cache with LRU eviction
     */
    set(pipelineId, pipelineVersion, data) {
        // Implement LRU eviction
        if (this.cache.size >= this.maxSize) {
            const firstKey = this.cache.keys().next().value;
            this.cache.delete(firstKey);
            console.log('🗑️  Cache EVICT (LRU):', firstKey);
        }

        const key = this._getCacheKey(pipelineId, pipelineVersion);
        this.cache.set(key, {
            data,
            timestamp: Date.now()
        });

        console.log('💾 Cache SET for pipeline:', pipelineId, `(${this.cache.size}/${this.maxSize})`);
    }

    /**
     * Invalidate all versions of a pipeline
     */
    invalidate(pipelineId) {
        let removed = 0;
        for (const key of this.cache.keys()) {
            if (key.startsWith(`${pipelineId}:`)) {
                this.cache.delete(key);
                removed++;
            }
        }

        if (removed > 0) {
            console.log(`🗑️  Cache INVALIDATE: ${removed} version(s) of pipeline ${pipelineId}`);
        }
    }

    /**
     * Clear entire cache
     */
    clear() {
        this.cache.clear();
        this.hits = 0;
        this.misses = 0;
        console.log('🗑️  Cache CLEARED');
    }

    /**
     * Get cache statistics
     */
    getStats() {
        return {
            size: this.cache.size,
            maxSize: this.maxSize,
            utilizationPercent: (this.cache.size / this.maxSize * 100).toFixed(1),
            hits: this.hits,
            misses: this.misses,
            hitRate: this.getHitRate(),
            ttlMinutes: this.ttl / 60000
        };
    }

    /**
     * Calculate cache hit rate
     */
    getHitRate() {
        const total = this.hits + this.misses;
        if (total === 0) return 0;
        return ((this.hits / total) * 100).toFixed(1);
    }
}

// Global singleton instance with sensible defaults
window.variablesCache = new ReferenceVariablesCache({
    ttl: 5 * 60 * 1000,  // 5 minutes
    maxSize: 50           // 50 pipelines
});

// Expose cache stats for debugging
window.getCacheStats = () => {
    const stats = window.variablesCache.getStats();
    console.table(stats);
    return stats;
};

/**
 * BuilderMetadata.js
 *
 * Metadata management for step configuration builders.
 * Stores and retrieves builder information, capabilities, and documentation.
 *
 * Part of the modular factory pattern implementation.
 */

class BuilderMetadata {
    constructor() {
        this.metadata = new Map();
    }

    /**
     * Set metadata for a builder
     * @param {string} stepType - Step type identifier
     * @param {Object} metadata - Metadata object
     * @param {string} metadata.displayName - Human-readable display name
     * @param {string} metadata.description - Builder description
     * @param {string} metadata.category - Category (e.g., 'input', 'transform', 'output')
     * @param {string} metadata.icon - Icon identifier or emoji
     * @param {Array<string>} metadata.tags - Searchable tags
     * @param {Object} metadata.capabilities - Supported capabilities
     * @param {string} metadata.version - Builder version
     * @param {string} metadata.author - Builder author
     * @returns {BuilderMetadata} This for chaining
     */
    set(stepType, metadata) {
        if (!stepType || typeof stepType !== 'string') {
            throw new Error('[BuilderMetadata] Step type must be a non-empty string');
        }

        // Validate required fields
        const required = ['displayName', 'description'];
        for (const field of required) {
            if (!metadata[field]) {
                throw new Error(`[BuilderMetadata] Missing required field "${field}" for ${stepType}`);
            }
        }

        // Store with defaults
        this.metadata.set(stepType, {
            displayName: metadata.displayName,
            description: metadata.description,
            category: metadata.category || 'general',
            icon: metadata.icon || '⚙️',
            tags: metadata.tags || [],
            capabilities: metadata.capabilities || {},
            version: metadata.version || '1.0.0',
            author: metadata.author || 'Unknown',
            deprecated: metadata.deprecated || false,
            experimental: metadata.experimental || false,
            updatedAt: new Date()
        });

        console.log(`📋 Set metadata for: ${stepType}`);
        return this;
    }

    /**
     * Get metadata for a builder
     * @param {string} stepType - Step type identifier
     * @returns {Object|null} Metadata object or null if not found
     */
    get(stepType) {
        return this.metadata.get(stepType) || null;
    }

    /**
     * Check if metadata exists for a builder
     * @param {string} stepType - Step type identifier
     * @returns {boolean} True if metadata exists
     */
    has(stepType) {
        return this.metadata.has(stepType);
    }

    /**
     * Remove metadata for a builder
     * @param {string} stepType - Step type identifier
     * @returns {boolean} True if removed, false if not found
     */
    remove(stepType) {
        return this.metadata.delete(stepType);
    }

    /**
     * Get all metadata entries
     * @returns {Array<Object>} Array of {stepType, metadata} objects
     */
    getAll() {
        return Array.from(this.metadata.entries()).map(([stepType, metadata]) => ({
            stepType,
            ...metadata
        }));
    }

    /**
     * Search metadata by query
     * @param {string} query - Search query
     * @param {Object} options - Search options
     * @param {Array<string>} options.fields - Fields to search in
     * @param {boolean} options.caseSensitive - Case-sensitive search
     * @returns {Array<Object>} Matching metadata entries
     */
    search(query, options = {}) {
        const {
            fields = ['displayName', 'description', 'tags'],
            caseSensitive = false
        } = options;

        const normalizedQuery = caseSensitive ? query : query.toLowerCase();

        return this.getAll().filter(entry => {
            for (const field of fields) {
                const value = entry[field];

                if (!value) continue;

                // Array field (tags)
                if (Array.isArray(value)) {
                    const match = value.some(item => {
                        const normalizedItem = caseSensitive ? item : item.toLowerCase();
                        return normalizedItem.includes(normalizedQuery);
                    });
                    if (match) return true;
                }
                // String field
                else if (typeof value === 'string') {
                    const normalizedValue = caseSensitive ? value : value.toLowerCase();
                    if (normalizedValue.includes(normalizedQuery)) return true;
                }
            }

            return false;
        });
    }

    /**
     * Filter metadata by criteria
     * @param {Object} criteria - Filter criteria
     * @param {string} criteria.category - Filter by category
     * @param {boolean} criteria.deprecated - Include deprecated (default: false)
     * @param {boolean} criteria.experimental - Include experimental (default: true)
     * @param {Array<string>} criteria.tags - Filter by tags (OR logic)
     * @returns {Array<Object>} Filtered metadata entries
     */
    filter(criteria = {}) {
        const {
            category = null,
            deprecated = false,
            experimental = true,
            tags = null
        } = criteria;

        return this.getAll().filter(entry => {
            // Filter deprecated
            if (entry.deprecated && !deprecated) return false;

            // Filter experimental
            if (entry.experimental && !experimental) return false;

            // Filter by category
            if (category && entry.category !== category) return false;

            // Filter by tags (OR logic)
            if (tags && tags.length > 0) {
                const hasMatchingTag = tags.some(tag =>
                    entry.tags && entry.tags.includes(tag)
                );
                if (!hasMatchingTag) return false;
            }

            return true;
        });
    }

    /**
     * Get unique categories
     * @returns {Array<string>} Array of unique categories
     */
    getCategories() {
        const categories = new Set();
        for (const metadata of this.metadata.values()) {
            categories.add(metadata.category);
        }
        return Array.from(categories).sort();
    }

    /**
     * Get unique tags
     * @returns {Array<string>} Array of unique tags
     */
    getTags() {
        const tags = new Set();
        for (const metadata of this.metadata.values()) {
            if (metadata.tags) {
                metadata.tags.forEach(tag => tags.add(tag));
            }
        }
        return Array.from(tags).sort();
    }

    /**
     * Get builders by category
     * @param {string} category - Category name
     * @returns {Array<Object>} Metadata entries in category
     */
    getByCategory(category) {
        return this.getAll().filter(entry => entry.category === category);
    }

    /**
     * Get builders by tag
     * @param {string} tag - Tag name
     * @returns {Array<Object>} Metadata entries with tag
     */
    getByTag(tag) {
        return this.getAll().filter(entry =>
            entry.tags && entry.tags.includes(tag)
        );
    }

    /**
     * Get builder display name
     * @param {string} stepType - Step type identifier
     * @returns {string} Display name or stepType if not found
     */
    getDisplayName(stepType) {
        const metadata = this.get(stepType);
        return metadata ? metadata.displayName : stepType;
    }

    /**
     * Get builder icon
     * @param {string} stepType - Step type identifier
     * @returns {string} Icon or default icon
     */
    getIcon(stepType) {
        const metadata = this.get(stepType);
        return metadata ? metadata.icon : '⚙️';
    }

    /**
     * Mark builder as deprecated
     * @param {string} stepType - Step type identifier
     * @param {string} message - Deprecation message
     * @returns {boolean} True if updated, false if not found
     */
    markDeprecated(stepType, message = '') {
        const metadata = this.get(stepType);
        if (!metadata) return false;

        metadata.deprecated = true;
        metadata.deprecationMessage = message;
        metadata.updatedAt = new Date();

        console.warn(`⚠️ Builder "${stepType}" marked as deprecated: ${message}`);
        return true;
    }

    /**
     * Mark builder as experimental
     * @param {string} stepType - Step type identifier
     * @param {boolean} experimental - Experimental flag
     * @returns {boolean} True if updated, false if not found
     */
    markExperimental(stepType, experimental = true) {
        const metadata = this.get(stepType);
        if (!metadata) return false;

        metadata.experimental = experimental;
        metadata.updatedAt = new Date();

        console.log(`🧪 Builder "${stepType}" experimental: ${experimental}`);
        return true;
    }

    /**
     * Clear all metadata
     * Useful for testing
     */
    clear() {
        this.metadata.clear();
        console.log('[BuilderMetadata] Cleared all metadata');
    }

    /**
     * Get metadata statistics
     * @returns {Object} Statistics object
     */
    getStats() {
        const all = this.getAll();

        return {
            total: all.length,
            byCategory: this.getCategories().reduce((acc, category) => {
                acc[category] = this.getByCategory(category).length;
                return acc;
            }, {}),
            deprecated: all.filter(e => e.deprecated).length,
            experimental: all.filter(e => e.experimental).length,
            categories: this.getCategories().length,
            tags: this.getTags().length
        };
    }
}

console.log('[BuilderMetadata] Loaded - metadata management system ready');

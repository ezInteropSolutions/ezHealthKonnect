/**
 * LoopTypeRegistry - Central registry for loop type providers
 *
 * Manages all registered format providers and handles automatic format detection.
 * This is the main entry point for the ForEachLoopBuilder to get loop options.
 *
 * @see docs/LOOP_TYPE_PROVIDERS.md for full architecture documentation
 *
 * Usage:
 *   const provider = LoopTypeRegistry.getProviderForMessage(message);
 *   const options = provider.getAllOptions();
 */
class LoopTypeRegistry {
    constructor() {
        this.providers = new Map();
        this.defaultProviderId = 'hl7'; // Default if detection fails
    }

    /**
     * Register a new loop type provider
     * @param {BaseLoopTypeProvider} provider - The provider to register
     */
    register(provider) {
        if (!provider || !provider.getFormatId) {
            throw new Error('Invalid provider: must extend BaseLoopTypeProvider');
        }
        const formatId = provider.getFormatId();
        console.log(`[LoopTypeRegistry] Registered provider: ${formatId} (${provider.getFormatName()})`);
        this.providers.set(formatId, provider);
    }

    /**
     * Get a provider by format ID
     * @param {string} formatId - The format identifier
     * @returns {BaseLoopTypeProvider|null} The provider or null
     */
    getProvider(formatId) {
        return this.providers.get(formatId) || null;
    }

    /**
     * Get all registered providers
     * @returns {Array<BaseLoopTypeProvider>} All providers
     */
    getAllProviders() {
        return Array.from(this.providers.values());
    }

    /**
     * Get all registered format IDs
     * @returns {Array<string>} Format IDs
     */
    getRegisteredFormats() {
        return Array.from(this.providers.keys());
    }

    /**
     * Detect the message format and return appropriate provider
     * @param {Object|string} message - The message to analyze
     * @returns {BaseLoopTypeProvider} The matching provider (or default)
     */
    getProviderForMessage(message) {
        // Try each provider's detection
        for (const provider of this.providers.values()) {
            try {
                if (provider.detectFormat(message)) {
                    console.log(`[LoopTypeRegistry] Detected format: ${provider.getFormatId()}`);
                    return provider;
                }
            } catch (error) {
                console.warn(`[LoopTypeRegistry] Detection error for ${provider.getFormatId()}:`, error);
            }
        }

        // Return default provider
        console.log(`[LoopTypeRegistry] Using default provider: ${this.defaultProviderId}`);
        return this.providers.get(this.defaultProviderId) || this.providers.values().next().value;
    }

    /**
     * Get loop options for a message (convenience method)
     * @param {Object|string} message - The message
     * @returns {Array<LoopOption>} All applicable loop options
     */
    getOptionsForMessage(message) {
        const provider = this.getProviderForMessage(message);
        return provider ? provider.getAllOptions() : [];
    }

    /**
     * Set the default provider ID (used when detection fails)
     * @param {string} formatId - The default format ID
     */
    setDefaultProvider(formatId) {
        if (!this.providers.has(formatId)) {
            console.warn(`[LoopTypeRegistry] Default provider ${formatId} not registered`);
        }
        this.defaultProviderId = formatId;
    }

    /**
     * Get a combined list of all options from all providers
     * Useful for showing "all possible" options
     * @returns {Object} Options grouped by format
     */
    getAllOptionsGroupedByFormat() {
        const grouped = {};
        for (const [formatId, provider] of this.providers) {
            grouped[formatId] = {
                formatName: provider.getFormatName(),
                options: provider.getAllOptions(),
                categories: provider.getCategoryInfo ? provider.getCategoryInfo() : []
            };
        }
        return grouped;
    }
}

// Create singleton instance
const loopTypeRegistry = new LoopTypeRegistry();

// Auto-register built-in providers when scripts are loaded
// This happens after all provider scripts are loaded
function initializeLoopTypeRegistry() {
    // Check if providers are available (loaded via script tags)
    if (typeof HL7LoopProvider !== 'undefined') {
        loopTypeRegistry.register(new HL7LoopProvider());
    }
    if (typeof FHIRLoopProvider !== 'undefined') {
        loopTypeRegistry.register(new FHIRLoopProvider());
    }

    console.log('[LoopTypeRegistry] Initialized with providers:', loopTypeRegistry.getRegisteredFormats());
}

// Initialize on DOM ready or immediately if already loaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeLoopTypeRegistry);
} else {
    // Small delay to ensure other scripts are loaded
    setTimeout(initializeLoopTypeRegistry, 0);
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { LoopTypeRegistry, loopTypeRegistry };
}

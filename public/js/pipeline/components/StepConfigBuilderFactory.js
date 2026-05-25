/**
 * StepConfigBuilderFactory - Factory Pattern for Step Configuration Builders
 *
 * Purpose:
 * - Centralized builder registration and instantiation
 * - Loose coupling between PropertiesPanel and specific builders
 * - Easy to add new builders without modifying existing code
 *
 * Design Patterns:
 * - Factory Pattern: Creates builders based on step type
 * - Registry Pattern: Maintains builder class registry
 * - Singleton Pattern: Single factory instance
 *
 * Dependencies:
 * - BuilderRegistry: Builder registration and lookup
 * - BuilderMetadata: Metadata management
 *
 * @class StepConfigBuilderFactory
 */
class StepConfigBuilderFactory {
    constructor() {
        // Delegate to modular components
        this.registry = new BuilderRegistry();
        this.metadata = new BuilderMetadata();

        console.log('🏭 [StepConfigBuilderFactory] Initialized (modular version)');
    }

    /**
     * Register a builder class for a step type
     *
     * @param {string} stepType - Step type identifier (e.g., "pre.logic", "core.validation")
     * @param {Class} builderClass - Builder class (must extend BaseStepConfigBuilder)
     * @param {Object} metadata - Optional metadata { category, subcategory, description }
     *
     * @example
     * factory.register('pre.logic', IfThenElseBuilder, {
     *     category: 'Control',
     *     subcategory: 'Conditional Logic',
     *     description: 'If-Then-Else conditional routing'
     * });
     */
    register(stepType, builderClass, metadata = {}) {
        // Validation: stepType must be a string
        if (typeof stepType !== 'string' || !stepType.trim()) {
            throw new Error('[Factory] Step type must be a non-empty string');
        }

        // Validation: builderClass must be a constructor function
        if (typeof builderClass !== 'function') {
            throw new Error(`[Factory] Builder for "${stepType}" must be a class/constructor function`);
        }

        // Validation: Check if builder extends BaseStepConfigBuilder (optional but recommended)
        if (typeof BaseStepConfigBuilder !== 'undefined') {
            const isValidBuilder = builderClass.prototype instanceof BaseStepConfigBuilder;
            if (!isValidBuilder && builderClass !== BaseStepConfigBuilder) {
                console.warn(`[Factory] ⚠️ Builder for "${stepType}" does not extend BaseStepConfigBuilder`);
            }
        }

        // Register builder using BuilderRegistry
        this.registry.register(stepType, builderClass, {
            override: this.registry.has(stepType)
        });

        // Register metadata using BuilderMetadata
        this.metadata.set(stepType, {
            displayName: metadata.displayName || stepType,
            description: metadata.description || '',
            category: metadata.category || 'General',
            icon: metadata.icon || '⚙️',
            tags: metadata.tags || [],
            subcategory: metadata.subcategory || '',
            version: metadata.version || '1.0.0',
            author: metadata.author || 'ezHealthKonnect'
        });

        return this; // Chainable
    }

    /**
     * Register multiple builders at once
     *
     * @param {Array<Object>} builders - Array of { stepType, builderClass, metadata }
     *
     * @example
     * factory.registerBulk([
     *     { stepType: 'pre.logic', builderClass: IfThenElseBuilder },
     *     { stepType: 'pre.validation', builderClass: ValidationRuleBuilder }
     * ]);
     */
    registerBulk(builders) {
        if (!Array.isArray(builders)) {
            throw new Error('[Factory] registerBulk expects an array');
        }

        builders.forEach(({ stepType, builderClass, metadata }) => {
            this.register(stepType, builderClass, metadata);
        });

        console.log(`[Factory] ✅ Bulk registered ${builders.length} builders`);
        return this;
    }

    /**
     * Create a builder instance for a step type
     *
     * @param {string} stepType - Step type identifier
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial configuration (optional)
     * @returns {BaseStepConfigBuilder|null} Builder instance or null if not registered
     *
     * @example
     * const builder = factory.create('pre.logic', containerElement, step.config);
     * if (builder) {
     *     builder.init();
     * }
     */
    create(stepType, container, initialConfig = {}) {
        // Validate inputs
        if (!stepType) {
            console.error('[Factory] ❌ stepType is required');
            return null;
        }

        if (!container || !(container instanceof HTMLElement)) {
            console.error(`[Factory] ❌ Invalid container for step type: ${stepType}`);
            return null;
        }

        // Delegate to BuilderRegistry
        const builder = this.registry.createInstance(stepType, container, initialConfig);

        if (!builder) {
            return null;
        }

        // Verify instance has required methods
        if (typeof builder.init !== 'function') {
            console.error(`[Factory] ❌ Builder for "${stepType}" missing init() method`);
            return null;
        }

        if (typeof builder.getConfig !== 'function') {
            console.error(`[Factory] ❌ Builder for "${stepType}" missing getConfig() method`);
            return null;
        }

        return builder;
    }

    /**
     * Check if a builder is registered for a step type
     *
     * @param {string} stepType - Step type identifier
     * @returns {boolean}
     */
    has(stepType) {
        return this.registry.has(stepType);
    }

    /**
     * Unregister a builder
     *
     * @param {string} stepType - Step type identifier
     * @returns {boolean} True if unregistered, false if not found
     */
    unregister(stepType) {
        const deleted = this.registry.unregister(stepType);
        this.metadata.remove(stepType);
        return deleted;
    }

    /**
     * Get all registered step types
     *
     * @returns {string[]} Array of step type identifiers
     */
    getRegisteredTypes() {
        return this.registry.getRegisteredTypes();
    }

    /**
     * Get builder metadata
     *
     * @param {string} stepType - Step type identifier
     * @returns {Object|null} Metadata object or null if not found
     */
    getMetadata(stepType) {
        return this.metadata.get(stepType);
    }

    /**
     * Get all builders grouped by category
     *
     * @returns {Object} { category: [{ stepType, metadata }] }
     */
    getBuildersByCategory() {
        const allMetadata = this.metadata.getAll();
        const grouped = {};

        allMetadata.forEach(meta => {
            const category = meta.category || 'General';

            if (!grouped[category]) {
                grouped[category] = [];
            }

            grouped[category].push(meta);
        });

        // Sort each category by subcategory and stepType
        Object.keys(grouped).forEach(category => {
            grouped[category].sort((a, b) => {
                const subCatA = a.subcategory || '';
                const subCatB = b.subcategory || '';

                if (subCatA !== subCatB) {
                    return subCatA.localeCompare(subCatB);
                }
                return a.stepType.localeCompare(b.stepType);
            });
        });

        return grouped;
    }

    /**
     * Get diagnostic information
     *
     * @returns {Object} Diagnostic info
     */
    getDiagnostics() {
        return {
            totalRegistered: this.registry.getCount(),
            registeredTypes: this.registry.getRegisteredTypes(),
            categories: this.metadata.getCategories(),
            registryStats: this.registry.getStats(),
            metadataStats: this.metadata.getStats()
        };
    }

    /**
     * Log diagnostic information to console
     */
    logDiagnostics() {
        const diagnostics = this.getDiagnostics();

        console.group('🏭 [StepConfigBuilderFactory] Diagnostics');
        console.log(`Total Registered: ${diagnostics.totalRegistered}`);
        console.log('Categories:', diagnostics.categories);
        console.log('Registry Stats:', diagnostics.registryStats);
        console.log('Metadata Stats:', diagnostics.metadataStats);
        console.groupEnd();
    }

    /**
     * Clear all registrations (for testing)
     */
    clear() {
        this.registry.clear();
        this.metadata.clear();
        console.log('[Factory] 🗑️ Cleared all registrations');
    }
}

// ========================================
// SINGLETON INSTANCE
// ========================================

/**
 * Global singleton instance
 * Use this instance throughout the application
 */
const stepConfigBuilderFactory = new StepConfigBuilderFactory();

// ========================================
// AUTO-REGISTRATION (if builders are available)
// ========================================

/**
 * Auto-register builders if they're already loaded
 * This runs when the factory script is loaded
 */
(function autoRegisterBuilders() {
    console.log('[Factory] 🔍 Auto-registering available builders...');

    const registrations = [];

    // IfThenElseBuilder
    if (typeof IfThenElseBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.logic',
            builderClass: IfThenElseBuilder,
            metadata: {
                displayName: 'If-Then-Else (Pre)',
                category: 'Control',
                subcategory: 'Conditional Logic',
                description: 'If-Then-Else conditional routing with TRUE/FALSE actions',
                icon: '🔀',
                tags: ['conditional', 'logic', 'routing']
            }
        });

        registrations.push({
            stepType: 'core.logic',
            builderClass: IfThenElseBuilder,
            metadata: {
                displayName: 'If-Then-Else (Core)',
                category: 'Control',
                subcategory: 'Conditional Logic',
                description: 'If-Then-Else conditional routing with TRUE/FALSE actions',
                icon: '🔀',
                tags: ['conditional', 'logic', 'routing']
            }
        });

        registrations.push({
            stepType: 'post.logic',
            builderClass: IfThenElseBuilder,
            metadata: {
                displayName: 'If-Then-Else (Post)',
                category: 'Control',
                subcategory: 'Conditional Logic',
                description: 'If-Then-Else conditional routing with TRUE/FALSE actions',
                icon: '🔀',
                tags: ['conditional', 'logic', 'routing']
            }
        });
    }

    // ValidationRuleBuilder
    if (typeof ValidationRuleBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.validation',
            builderClass: ValidationRuleBuilder,
            metadata: {
                displayName: 'Field Validation',
                category: 'Validation',
                subcategory: 'Field Validation',
                description: 'Validate field values (required, format, length, pattern)',
                icon: '✓',
                tags: ['validation', 'rules']
            }
        });
    }

    // MetadataBuilder
    if (typeof MetadataBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.metadata',
            builderClass: MetadataBuilder,
            metadata: {
                displayName: 'Custom Metadata',
                category: 'Enrichment',
                subcategory: 'Metadata',
                description: 'Add custom key-value metadata pairs',
                icon: '🏷️',
                tags: ['metadata', 'enrichment']
            }
        });
    }

    // OAuth2ConfigBuilder
    if (typeof OAuth2ConfigBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.api',
            builderClass: OAuth2ConfigBuilder,
            metadata: {
                displayName: 'OAuth 2.0 Config',
                category: 'Enrichment',
                subcategory: 'API Integration',
                description: 'OAuth 2.0 authentication configuration',
                icon: '🔐',
                tags: ['api', 'oauth', 'authentication']
            }
        });
    }

    // HeaderBuilder
    if (typeof HeaderBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.api.headers',
            builderClass: HeaderBuilder,
            metadata: {
                displayName: 'HTTP Headers',
                category: 'Enrichment',
                subcategory: 'API Integration',
                description: 'HTTP headers for API calls',
                icon: '📋',
                tags: ['api', 'headers', 'http']
            }
        });
    }

    // QueryParamBuilder
    if (typeof QueryParamBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.api.params',
            builderClass: QueryParamBuilder,
            metadata: {
                displayName: 'Query Parameters',
                category: 'Enrichment',
                subcategory: 'API Integration',
                description: 'URL query parameters for API calls',
                icon: '🔗',
                tags: ['api', 'params', 'query']
            }
        });
    }

    // MongoDBFilterBuilder
    if (typeof MongoDBFilterBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.mongodb.filter',
            builderClass: MongoDBFilterBuilder,
            metadata: {
                displayName: 'MongoDB Filters',
                category: 'Enrichment',
                subcategory: 'MongoDB',
                description: 'MongoDB query filters',
                icon: '🍃',
                tags: ['mongodb', 'filter', 'query']
            }
        });
    }

    // MongoDBProjectionBuilder
    if (typeof MongoDBProjectionBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.mongodb.projection',
            builderClass: MongoDBProjectionBuilder,
            metadata: {
                displayName: 'MongoDB Projection',
                category: 'Enrichment',
                subcategory: 'MongoDB',
                description: 'MongoDB field projection',
                icon: '🍃',
                tags: ['mongodb', 'projection', 'fields']
            }
        });
    }

    // RedisQueryBuilder
    if (typeof RedisQueryBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.redis',
            builderClass: RedisQueryBuilder,
            metadata: {
                displayName: 'Redis Query',
                category: 'Enrichment',
                subcategory: 'Redis',
                description: 'Redis cache queries',
                icon: '⚡',
                tags: ['redis', 'cache', 'query']
            }
        });
    }

    // ResultMappingBuilder
    if (typeof ResultMappingBuilder !== 'undefined') {
        registrations.push({
            stepType: 'pre.enrichment.api.mapping',
            builderClass: ResultMappingBuilder,
            metadata: {
                displayName: 'API Result Mapping',
                category: 'Enrichment',
                subcategory: 'API Integration',
                description: 'Map API response fields to message fields',
                icon: '🔄',
                tags: ['api', 'mapping', 'transform']
            }
        });
    }

    // Bulk register
    if (registrations.length > 0) {
        stepConfigBuilderFactory.registerBulk(registrations);
        console.log(`[Factory] ✅ Auto-registered ${registrations.length} builders`);
    } else {
        console.log('[Factory] ℹ️ No builders available for auto-registration (load builders first)');
    }
})();

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.StepConfigBuilderFactory = StepConfigBuilderFactory;
    window.stepConfigBuilderFactory = stepConfigBuilderFactory; // Singleton instance
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = { StepConfigBuilderFactory, stepConfigBuilderFactory };
}

console.log('[StepConfigBuilderFactory] Loaded - factory pattern ready (modular version)');

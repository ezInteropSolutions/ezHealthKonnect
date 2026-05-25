/**
 * BaseLoopTypeProvider - Abstract base class for loop type providers
 *
 * This class defines the interface that all format-specific loop providers must implement.
 * Each provider offers human-friendly loop options for a specific message format (HL7, FHIR, etc.)
 *
 * @see docs/LOOP_TYPE_PROVIDERS.md for full architecture documentation
 *
 * @abstract
 */
class BaseLoopTypeProvider {
    /**
     * Create a new loop type provider
     * @param {string} formatId - Unique identifier for this format (e.g., 'hl7', 'fhir')
     * @param {string} formatName - Human-readable format name (e.g., 'HL7 v2.x', 'FHIR R4')
     */
    constructor(formatId, formatName) {
        if (new.target === BaseLoopTypeProvider) {
            throw new Error('BaseLoopTypeProvider is abstract and cannot be instantiated directly');
        }
        this.formatId = formatId;
        this.formatName = formatName;
    }

    /**
     * Get the unique format identifier
     * @returns {string} Format ID
     */
    getFormatId() {
        return this.formatId;
    }

    /**
     * Get the human-readable format name
     * @returns {string} Format name
     */
    getFormatName() {
        return this.formatName;
    }

    /**
     * Get all available loop options for this format
     * Must be implemented by subclasses
     *
     * @abstract
     * @returns {Array<LoopOption>} Array of loop option objects
     *
     * @typedef {Object} LoopOption
     * @property {string} id - Unique identifier for this option
     * @property {string} icon - FontAwesome icon class
     * @property {string} label - Short label for the card (2-3 words)
     * @property {string} description - Longer description for tooltip
     * @property {string} technicalPath - The actual path/value used by the executor
     * @property {string} category - Category for grouping (records, results, contacts, etc.)
     * @property {Array<LoopVariable>} availableVariables - Variables available in loop body
     *
     * @typedef {Object} LoopVariable
     * @property {string} name - Variable name (e.g., 'item', 'position')
     * @property {string} description - What this variable contains
     */
    getLoopOptions() {
        throw new Error('getLoopOptions() must be implemented by subclass');
    }

    /**
     * Detect if a message matches this format
     * Must be implemented by subclasses
     *
     * @abstract
     * @param {Object|string} message - The message to check
     * @returns {boolean} True if message matches this format
     */
    detectFormat(message) {
        throw new Error('detectFormat() must be implemented by subclass');
    }

    /**
     * Get loop options filtered by category
     * @param {string} category - Category to filter by
     * @returns {Array<LoopOption>} Filtered options
     */
    getOptionsByCategory(category) {
        return this.getLoopOptions().filter(opt => opt.category === category);
    }

    /**
     * Get all unique categories
     * @returns {Array<string>} List of category names
     */
    getCategories() {
        const categories = new Set(this.getLoopOptions().map(opt => opt.category));
        return Array.from(categories);
    }

    /**
     * Find a loop option by ID
     * @param {string} id - Option ID to find
     * @returns {LoopOption|null} The option or null if not found
     */
    getOptionById(id) {
        return this.getLoopOptions().find(opt => opt.id === id) || null;
    }

    /**
     * Get the standard variables available in all loops
     * Subclasses can override to add format-specific variables
     * @returns {Array<LoopVariable>} Standard loop variables
     */
    getStandardVariables() {
        return [
            { name: 'loop.item', description: 'The current item in the iteration' },
            { name: 'loop.position', description: 'Current position (1, 2, 3...)' },
            { name: 'loop.index', description: 'Current index (0, 1, 2...)' },
            { name: 'loop.total', description: 'Total number of items' },
            { name: 'loop.isFirst', description: 'True if this is the first item' },
            { name: 'loop.isLast', description: 'True if this is the last item' }
        ];
    }

    /**
     * Get the utility options (fixed count, custom) - same for all formats
     * @returns {Array<LoopOption>} Utility options
     */
    getUtilityOptions() {
        return [
            {
                id: 'fixed-count',
                icon: 'fas fa-hashtag',
                label: 'Fixed Number',
                description: 'Repeat a specific number of times',
                technicalPath: null, // Special handling
                category: 'utility',
                isFixedCount: true,
                availableVariables: [
                    { name: 'loop.position', description: 'Current iteration (1, 2, 3...)' },
                    { name: 'loop.total', description: 'Total iterations' }
                ]
            },
            {
                id: 'count-of',
                icon: 'fas fa-calculator',
                label: 'Same Count As...',
                description: 'Repeat based on count of another field',
                technicalPath: null, // User selects
                category: 'utility',
                isCountOf: true,
                availableVariables: this.getStandardVariables()
            },
            {
                id: 'custom',
                icon: 'fas fa-cog',
                label: 'Custom Path',
                description: 'Advanced: specify a custom field path',
                technicalPath: null, // User enters
                category: 'utility',
                isAdvanced: true,
                availableVariables: this.getStandardVariables()
            }
        ];
    }

    /**
     * Get all options including utility options
     * @returns {Array<LoopOption>} All options
     */
    getAllOptions() {
        return [...this.getLoopOptions(), ...this.getUtilityOptions()];
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = BaseLoopTypeProvider;
}

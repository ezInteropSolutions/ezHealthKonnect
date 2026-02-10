/**
 * StepVariablesProvider - Mixin for loading step output variables
 *
 * Provides common functionality for:
 * - IfThenElseBuilder
 * - SwitchCaseBuilder
 * - Field Mapping configuration
 * - Any component that needs access to step output variables
 *
 * Usage (mixin pattern):
 *   Object.assign(MyBuilder.prototype, StepVariablesProvider);
 *   // Then in constructor: this.initStepVariables();
 *
 * Or extend directly:
 *   class MyBuilder extends StepVariablesProvider { ... }
 */

const StepVariablesProvider = {
    /**
     * Initialize step variables caching
     * Call this in constructor
     */
    initStepVariables() {
        this.cachedStepVariables = null;
        this.loadStepVariables();
    },

    /**
     * Load available step output variables from previous steps
     * Used for smart search in field pickers
     */
    async loadStepVariables() {
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline) {
                this.cachedStepVariables = [];
                return;
            }

            // Get current step context
            const currentStep = window.pipelineBuilder?.currentStep;
            const currentLayer = currentStep?.layer || 'pre';

            // Collect all steps using flat collection pattern
            let allSteps = [];
            if (pipeline.getAllSteps) {
                allSteps = pipeline.getAllSteps();
            } else {
                (pipeline.executionGroups || []).forEach(group => {
                    if (group.steps) {
                        allSteps.push(...group.steps);
                    }
                });
            }

            // Build backend format with all steps under current layer
            const backendLayers = {};
            backendLayers[currentLayer] = { steps: allSteps };

            // Find current step index
            const stepIndex = allSteps.findIndex(s => s.id === currentStep?.id);

            const response = await fetch('/api/pipeline/reference-variables', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    pipeline: { layers: backendLayers },
                    current_layer: currentLayer,
                    current_step: stepIndex >= 0 ? stepIndex : 0
                })
            });

            if (response.ok) {
                const data = await response.json();
                this.cachedStepVariables = this.transformVariablesToSearchFormat(data.variables || []);
                const className = this.constructor?.name || 'StepVariablesProvider';
                console.log(`[${className}] Loaded`, this.cachedStepVariables.length, 'step variables for smart search');
            } else {
                this.cachedStepVariables = [];
            }
        } catch (err) {
            const className = this.constructor?.name || 'StepVariablesProvider';
            console.warn(`[${className}] Failed to load step variables:`, err);
            this.cachedStepVariables = [];
        }
    },

    /**
     * Transform backend variables format to FieldPathSearchComponent format
     */
    transformVariablesToSearchFormat(variables) {
        const searchFields = [];

        variables.forEach(category => {
            const stepName = category.category || 'Unknown Step';
            const stepType = category.stepType || 'unknown';

            // Determine category based on step type
            let displayCategory = 'Step Outputs';
            if (stepType.includes('database')) displayCategory = 'Database';
            else if (stepType.includes('api')) displayCategory = 'API';
            else if (stepType.includes('script')) displayCategory = 'Script';

            if (category.variables) {
                category.variables.forEach(variable => {
                    searchFields.push({
                        name: `${stepName}: ${variable.name}`,
                        path: variable.path,
                        description: variable.description || `From ${stepName}`,
                        category: displayCategory,
                        examples: variable.examples || []
                    });
                });
            }
        });

        return searchFields;
    },

    /**
     * Get step variables for FieldPathSearchComponent
     * Called as a callback to provide dynamic variables
     */
    getStepVariablesForSearch() {
        return this.cachedStepVariables || [];
    },

    /**
     * Create FieldPathSearchComponent options with step variables
     * @param {Function} onSelect - Callback when field is selected
     * @param {Object} additionalOptions - Additional options to merge
     * @returns {Object} Options for FieldPathSearchComponent
     */
    createFieldSearchOptions(onSelect, additionalOptions = {}) {
        return {
            onSelect,
            placeholder: 'Search fields or variables... (e.g., PID.8, steps.empi.mrn)',
            allowCustom: true,
            showCategories: true,
            getStepVariables: () => this.getStepVariablesForSearch(),
            ...additionalOptions
        };
    }
};

// Export for browser
if (typeof window !== 'undefined') {
    window.StepVariablesProvider = StepVariablesProvider;
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = StepVariablesProvider;
}

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

            // Get current step context.
            // PropertiesPanel stores the edited step in this.currentStep (set in showStepProperties).
            // Builders (IfThenElse, SwitchCase, Loop) store it in this.step.
            // Fall back to window.pipelineBuilder.currentStep as a last resort.
            const currentStep = this.currentStep || this.step || window.pipelineBuilder?.currentStep;
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
     * Walk the last test-run output and enumerate paths under steps.*
     *
     * Strategy:
     *  - Depth 10 (safe because 500-path cap is the real guard)
     *  - Arrays: show field[] as a hint AND walk arr[0] when it's an object
     *    so users see concrete sub-field paths (patients[0].name, etc.)
     *  - Skip internal keys (_metadata, _lastError, _routing, etc.)
     *  - Deduplication against backend-declared variables happens in getStepVariablesForSearch
     */
    _walkTestRunPaths() {
        // Prefer in-memory cache; restore from localStorage if cleared (e.g. after save)
        let testOutput = window.pipelineLastTestOutput;
        if (!testOutput) {
            try {
                const stored = localStorage.getItem('pipeline_last_test_output');
                if (stored) {
                    testOutput = JSON.parse(stored);
                    window.pipelineLastTestOutput = testOutput; // restore to avoid re-parsing
                }
            } catch (_) {}
        }
        if (!testOutput || typeof testOutput.steps !== 'object') return [];

        const MAX_DEPTH = 10;
        const MAX_PATHS = 500;
        const SKIP_KEYS = new Set([
            'step_metadata', '_metadata', '_lastError', '_lastErrorStep',
            '_routing', '_executionDetails', '_stepOutput'
        ]);

        const paths = [];

        const walk = (obj, prefix, depth, stepNs) => {
            if (depth > MAX_DEPTH || paths.length >= MAX_PATHS) return;
            if (!obj || typeof obj !== 'object') return;

            for (const [key, val] of Object.entries(obj)) {
                if (SKIP_KEYS.has(key)) continue;
                const fullPath = prefix ? `${prefix}.${key}` : key;

                // At depth 0 the key IS the step namespace (e.g. "hl7_fhir_transform").
                // Capture it so every descendant can include it in its display name,
                // making the field searchable by step name without knowing the full path.
                const currentStepNs = depth === 0 ? key : stepNs;

                // Build a human-friendly display name:
                //   "hl7_fhir_transform » family"   (so "fhir" or "family" both find it)
                //   "api_enrichment » email"
                const displayName = (currentStepNs && key !== currentStepNs)
                    ? `${currentStepNs} » ${key}`
                    : key;

                if (Array.isArray(val)) {
                    // Show the array itself so users know it exists
                    paths.push({
                        name: `${displayName}[]`,
                        path: `steps.${fullPath}`,
                        description: `Array · ${val.length} item${val.length === 1 ? '' : 's'}`,
                        category: 'Step Outputs (test run)',
                        isArray: true
                    });
                    // Walk first element when it's an object → exposes concrete sub-field paths
                    // e.g. patients[0].name, patients[0].dob
                    if (val.length > 0) {
                        const first = val[0];
                        if (first !== null && typeof first === 'object' && !Array.isArray(first)) {
                            walk(first, `${fullPath}[0]`, depth + 1, currentStepNs);
                        }
                    }
                } else if (val !== null && typeof val === 'object') {
                    walk(val, fullPath, depth + 1, currentStepNs);
                } else {
                    // Leaf — show current value as a description hint
                    paths.push({
                        name: displayName,
                        path: `steps.${fullPath}`,
                        description: String(val ?? ''),
                        category: 'Step Outputs (test run)',
                        examples: val !== null && val !== undefined ? [String(val)] : []
                    });
                }
            }
        };

        walk(testOutput.steps, '', 0, '');
        return paths;
    },

    /**
     * Get step variables for FieldPathSearchComponent.
     * Merges two sources:
     *  1. Backend declared variables (GetOutputVariables) — always available, config-time
     *  2. Test-run walked paths — available after first test run, depth-10 tree walk
     * Test-run paths supplement declared paths (deduped by path string).
     */
    getStepVariablesForSearch() {
        const backendVars = this.cachedStepVariables || [];
        const testRunVars = this._walkTestRunPaths();

        if (testRunVars.length === 0) return backendVars;

        // Deduplicate: only add test-run paths not already declared by the backend
        const knownPaths = new Set(backendVars.map(v => v.path));
        const newPaths = testRunVars.filter(v => !knownPaths.has(v.path));

        return newPaths.length > 0 ? [...backendVars, ...newPaths] : backendVars;
    },

    /**
     * Create FieldPathSearchComponent options with step variables.
     * Automatically reads messageType from the active pipeline so that CDA
     * pipelines (messageType=CCD) get USCDI search mode in the field picker.
     *
     * @param {Function} onSelect - Callback when field is selected
     * @param {Object} additionalOptions - Additional options to merge
     * @returns {Object} Options for FieldPathSearchComponent
     */
    createFieldSearchOptions(onSelect, additionalOptions = {}) {
        const messageType = window.pipelineBuilder?.getPipeline?.()?.messageType || '';
        const isCDA = ['CCD', 'CCDA', 'CDA', 'C32', 'HITSP'].includes((messageType || '').toUpperCase());

        return {
            onSelect,
            placeholder: isCDA
                ? 'Search CDA fields... (e.g., allergy, patient, medication)'
                : 'Search fields or variables... (e.g., PID.8, steps.empi.mrn)',
            allowCustom: true,
            showCategories: true,
            messageType,
            getStepVariables: () => this.getStepVariablesForSearch(),
            ...additionalOptions
        };
    }
};

// Export for browser
if (typeof window !== 'undefined') {
    window.StepVariablesProvider = StepVariablesProvider;

    // Apply mixin to PropertiesPanel — loads after PropertiesPanel.js, so prototype exists here
    if (window.PropertiesPanel) {
        Object.assign(window.PropertiesPanel.prototype, StepVariablesProvider);
    }
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = StepVariablesProvider;
}

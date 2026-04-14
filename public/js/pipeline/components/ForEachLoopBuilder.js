/**
 * ForEachLoopBuilder - No-code UI for Loop Container Steps
 *
 * Extends BaseStepConfigBuilder following OOP Template Method Pattern
 *
 * Design Pattern: OOP with Template Method
 * - Extends BaseStepConfigBuilder (abstract base class)
 * - Implements required abstract methods: getDefaultConfig(), render(), attachEvents()
 * - Uses optional hooks: beforeInit(), afterInit(), addStyles()
 *
 * Features:
 * - Visual loop configuration (For Each, For, While)
 * - Collection/array field selector with smart search
 * - Loop variable naming with auto-suggestions
 * - Nested step container for loop body
 * - Drag & drop steps into loop body
 * - Max iterations safety limit
 * - Break conditions support
 *
 * Config Schema:
 * {
 *   loopType: "foreach",           // 'foreach', 'for', 'while'
 *   collection: "enhancedSegments.OBX",  // For 'foreach': array to iterate
 *   itemVariable: "observation",   // Variable name for current item
 *   indexVariable: "index",        // Variable name for current index
 *   iterations: 10,                // For 'for': number of iterations
 *   condition: {},                 // For 'while': condition object
 *   childStepIds: [],              // IDs of steps in loop body
 *   maxIterations: 1000,           // Safety limit
 *   breakOnError: false,           // Stop loop on first error
 *   continueOnEmpty: true          // Continue pipeline if collection empty
 * }
 *
 * @class ForEachLoopBuilder
 * @extends BaseStepConfigBuilder
 */
class ForEachLoopBuilder extends BaseStepConfigBuilder {
    /**
     * Constructor
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial configuration (optional)
     */
    constructor(container, initialConfig = {}) {
        super(container, initialConfig);
        this.fieldSearchComponent = null;
        this.cachedStepVariables = null;
        this.availableSteps = [];
        this.draggedStepId = null;

        // ✅ NEW: Nested loop support
        this.parentLoopInfo = null;  // Info about parent loop if nested
        this.isNestedLoop = false;   // Flag for nested loop UI

        // Pre-fetch step variables for smart search
        this.loadStepVariables();
        this.loadAvailableSteps();
        this.detectParentLoop();  // ✅ NEW: Detect if inside parent loop
    }

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
                console.log('[ForEachLoopBuilder] Loaded', this.cachedStepVariables.length, 'step variables');
            } else {
                this.cachedStepVariables = [];
            }
        } catch (err) {
            console.warn('[ForEachLoopBuilder] Failed to load step variables:', err);
            this.cachedStepVariables = [];
        }
    }

    /**
     * ✅ NEW: Detect if this loop is nested inside another loop
     * This enables the "relative to parent item" UI option
     */
    detectParentLoop() {
        try {
            const currentStep = window.pipelineBuilder?.currentStep;
            if (!currentStep) {
                this.isNestedLoop = false;
                this.parentLoopInfo = null;
                return;
            }

            // Check if current step has a parent that is a loop
            const parentStepId = currentStep.parentStepId;
            if (!parentStepId) {
                this.isNestedLoop = false;
                this.parentLoopInfo = null;
                return;
            }

            // Find the parent step
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline) {
                this.isNestedLoop = false;
                return;
            }

            // Find parent step using flat collection pattern
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

            const parentStep = allSteps.find(s => s.id === parentStepId) || null;

            // Check if parent is a loop step - use VisualStep utility for OOP-compliant type detection
            if (parentStep && VisualStep.isLoopStep(parentStep)) {
                this.isNestedLoop = true;
                this.parentLoopInfo = {
                    id: parentStep.id,
                    name: parentStep.stepName || 'Parent Loop',
                    itemVariable: parentStep.config?.itemVariable || 'item',
                    indexVariable: parentStep.config?.indexVariable || 'index',
                    collection: parentStep.config?.collection || ''
                };
                console.log('[ForEachLoopBuilder] Detected parent loop:', this.parentLoopInfo);
            } else {
                this.isNestedLoop = false;
                this.parentLoopInfo = null;
            }
        } catch (err) {
            console.warn('[ForEachLoopBuilder] Failed to detect parent loop:', err);
            this.isNestedLoop = false;
            this.parentLoopInfo = null;
        }
    }

    /**
     * Load available steps that can be added to loop body
     */
    loadAvailableSteps() {
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline) {
                this.availableSteps = [];
                return;
            }

            const currentStep = window.pipelineBuilder?.currentStep;
            const allSteps = [];

            // Collect all steps using flat collection pattern
            let pipelineSteps = [];
            if (pipeline.getAllSteps) {
                pipelineSteps = pipeline.getAllSteps();
            } else {
                (pipeline.executionGroups || []).forEach(group => {
                    if (group.steps) {
                        pipelineSteps.push(...group.steps);
                    }
                });
            }

            // Filter to exclude current step and steps already in a container
            pipelineSteps.forEach(step => {
                if (step.id !== currentStep?.id && !step.parentStepId) {
                    allSteps.push({
                        id: step.id,
                        name: step.stepName,
                        type: step.stepType,
                        icon: step.icon || 'fas fa-cog',
                        layer: step.layer || 'pre'
                    });
                }
            });

            this.availableSteps = allSteps;
            console.log('[ForEachLoopBuilder] Loaded', this.availableSteps.length, 'available steps');
        } catch (err) {
            console.warn('[ForEachLoopBuilder] Failed to load available steps:', err);
            this.availableSteps = [];
        }
    }

    /**
     * Transform backend variables format to FieldPathSearchComponent format
     */
    transformVariablesToSearchFormat(variables) {
        const searchFields = [];

        variables.forEach(category => {
            const stepName = category.category || 'Unknown Step';
            const stepType = category.stepType || 'unknown';

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
    }

    // ========================================
    // ABSTRACT METHOD IMPLEMENTATIONS (required)
    // ========================================

    /**
     * Returns default configuration structure
     * @override
     * @returns {Object} Default configuration object
     */
    getDefaultConfig() {
        return {
            loopType: 'foreach',
            collection: '',
            relativeToParent: false,  // ✅ NEW: When true, collection is relative to parent loop's item
            itemVariable: 'item',
            indexVariable: 'index',
            iterations: 10,
            condition: {
                field: '',
                operator: 'not_empty',
                value: ''
            },
            childStepIds: [],
            maxIterations: 1000,
            breakOnError: false,
            continueOnEmpty: true
        };
    }

    /**
     * Renders the UI into the container
     * @override
     */
    render() {
        this.container.innerHTML = this.generateHTML();
    }

    /**
     * Attaches event listeners to rendered elements
     * @override
     */
    attachEvents() {
        // Loop type selector
        const loopTypeSelect = this.container.querySelector('#loopTypeSelect');
        if (loopTypeSelect) {
            loopTypeSelect.addEventListener('change', (e) => {
                this.config.loopType = e.target.value;
                this.render();
                this.attachEvents();
                this.notifyChange();
            });
        }

        // ✅ NEW: Relative to parent toggle (for nested loops)
        const relativeToggle = this.container.querySelector('#relativeToParentToggle');
        if (relativeToggle) {
            relativeToggle.addEventListener('change', (e) => {
                this.config.relativeToParent = e.target.checked;
                // Re-render to update the UI
                this.render();
                this.attachEvents();
                this.notifyChange();
            });
        }

        // ✅ NEW: Collection shortcut buttons
        this.container.querySelectorAll('.shortcut-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const value = e.currentTarget.dataset.value;
                this.config.collection = value;
                const collectionInput = this.container.querySelector('#collectionInput');
                if (collectionInput) {
                    collectionInput.value = value;
                }
                this.notifyChange();
            });
        });

        // NOTE: collectionInput is now a hidden field - value is set by card selection
        // NOTE: itemVariable and indexVariable are auto-set based on card selection
        // NOTE: customPathInput field search is initialized in handleCardSelection() when Custom is selected

        // Iterations input (for 'for' loop)
        const iterationsInput = this.container.querySelector('#iterationsInput');
        if (iterationsInput) {
            iterationsInput.addEventListener('input', (e) => {
                this.config.iterations = parseInt(e.target.value) || 10;
                this.notifyChange();
            });
        }

        // Max iterations input
        const maxIterationsInput = this.container.querySelector('#maxIterationsInput');
        if (maxIterationsInput) {
            maxIterationsInput.addEventListener('input', (e) => {
                this.config.maxIterations = parseInt(e.target.value) || 1000;
                this.notifyChange();
            });
        }

        // Break on error toggle
        const breakOnErrorToggle = this.container.querySelector('#breakOnErrorToggle');
        if (breakOnErrorToggle) {
            breakOnErrorToggle.addEventListener('change', (e) => {
                this.config.breakOnError = e.target.checked;
                this.notifyChange();
            });
        }

        // Continue on empty toggle
        const continueOnEmptyToggle = this.container.querySelector('#continueOnEmptyToggle');
        if (continueOnEmptyToggle) {
            continueOnEmptyToggle.addEventListener('change', (e) => {
                this.config.continueOnEmpty = e.target.checked;
                this.notifyChange();
            });
        }

        // Create new step button
        const createNewStepBtn = this.container.querySelector('#createNewStepBtn');
        if (createNewStepBtn) {
            createNewStepBtn.addEventListener('click', () => this.showStepTypeSelector());
        }

        // Add existing step button
        const addExistingStepBtn = this.container.querySelector('#addExistingStepBtn');
        if (addExistingStepBtn) {
            addExistingStepBtn.addEventListener('click', () => this.showExistingStepSelector());
        }

        // Configure step buttons - opens the step's properties panel
        this.container.querySelectorAll('.configure-child-step-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const stepId = e.currentTarget.dataset.stepId;
                this.openStepProperties(stepId);
            });
        });

        // Also allow clicking on the child step row to configure (except action buttons)
        this.container.querySelectorAll('.loop-child-step').forEach(stepEl => {
            stepEl.addEventListener('click', (e) => {
                // Don't trigger if clicking on action buttons
                if (e.target.closest('.loop-child-step-actions')) return;
                const stepId = stepEl.dataset.stepId;
                this.openStepProperties(stepId);
            });
        });

        // Remove step buttons
        this.container.querySelectorAll('.remove-child-step-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const stepId = e.currentTarget.dataset.stepId;
                this.removeChildStep(stepId);
            });
        });

        // Reorder buttons
        this.container.querySelectorAll('.move-step-up-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const stepId = e.currentTarget.dataset.stepId;
                this.moveChildStep(stepId, -1);
            });
        });

        this.container.querySelectorAll('.move-step-down-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const stepId = e.currentTarget.dataset.stepId;
                this.moveChildStep(stepId, 1);
            });
        });

        // ✅ NEW: Visual card selection event handlers
        // @see docs/LOOP_TYPE_PROVIDERS.md for architecture documentation
        this.container.querySelectorAll('.loop-option-card').forEach(card => {
            card.addEventListener('click', (e) => {
                this.handleCardSelection(e.currentTarget);
            });
        });

        // Custom path input handler
        const customPathInput = this.container.querySelector('#customPathInput');
        if (customPathInput) {
            customPathInput.addEventListener('input', (e) => {
                this.config.collection = e.target.value;
                this.notifyChange();
            });
        }

        // Fixed count input handler
        const fixedCountInput = this.container.querySelector('#fixedCountInput');
        if (fixedCountInput) {
            fixedCountInput.addEventListener('input', (e) => {
                this.config.iterations = parseInt(e.target.value) || 1;
                this.config.loopType = 'for'; // Switch to 'for' loop type for fixed count
                this.notifyChange();
            });
        }

        // Setup drag and drop for loop body
        this.setupDragAndDrop();

        // While condition inputs
        this.attachConditionEvents();
    }

    // ========================================
    // HOOK METHOD OVERRIDES (optional)
    // ========================================

    /**
     * Add custom CSS styles for the builder
     * @override
     */
    addStyles() {
        if (document.getElementById('foreach-loop-builder-styles')) return;

        const styles = document.createElement('style');
        styles.id = 'foreach-loop-builder-styles';
        styles.textContent = `
            .loop-builder-container {
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            }

            /* Header with loop icon - Navy Blue theme */
            .loop-builder-header {
                display: flex;
                align-items: center;
                gap: 12px;
                padding: 16px 20px;
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                border-radius: 12px 12px 0 0;
                color: white;
            }

            .loop-builder-header-icon {
                width: 48px;
                height: 48px;
                background: rgba(248, 187, 217, 0.3);
                border-radius: 12px;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 24px;
            }

            .loop-builder-header-text h3 {
                margin: 0;
                font-size: 18px;
                font-weight: 600;
            }

            .loop-builder-header-text p {
                margin: 4px 0 0 0;
                font-size: 13px;
                opacity: 0.9;
            }

            /* Loop type selector */
            .loop-type-selector {
                display: flex;
                gap: 8px;
                padding: 16px 20px;
                background: #f8fafc;
                border-bottom: 1px solid #e2e8f0;
            }

            .loop-type-btn {
                flex: 1;
                padding: 12px 16px;
                border: 2px solid #e2e8f0;
                border-radius: 8px;
                background: white;
                cursor: pointer;
                transition: all 0.2s ease;
                text-align: center;
            }

            .loop-type-btn:hover {
                border-color: #f8bbd9;
                background: #fdf2f8;
            }

            .loop-type-btn.active {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.08) 0%, rgba(248, 187, 217, 0.15) 100%);
                color: #1e3a8a;
            }

            .loop-type-btn i {
                display: block;
                font-size: 24px;
                margin-bottom: 8px;
            }

            .loop-type-btn span {
                font-size: 13px;
                font-weight: 600;
            }

            /* Configuration section */
            .loop-config-section {
                padding: 20px;
                border-bottom: 1px solid #e2e8f0;
            }

            .loop-config-section h4 {
                margin: 0 0 16px 0;
                font-size: 14px;
                font-weight: 600;
                color: #334155;
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .loop-config-section h4 i {
                color: #1e3a8a;
            }

            .loop-config-row {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 16px;
                margin-bottom: 16px;
            }

            .loop-config-row.single {
                grid-template-columns: 1fr;
            }

            .loop-config-field {
                display: flex;
                flex-direction: column;
                gap: 6px;
            }

            .loop-config-field label {
                font-size: 12px;
                font-weight: 600;
                color: #64748b;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .loop-config-field input,
            .loop-config-field select {
                padding: 10px 12px;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                font-size: 14px;
                transition: border-color 0.2s, box-shadow 0.2s;
            }

            .loop-config-field input:focus,
            .loop-config-field select:focus {
                outline: none;
                border-color: #1e3a8a;
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }

            .loop-config-field .field-hint {
                font-size: 11px;
                color: #94a3b8;
            }

            /* Loop body container - THE VISUAL CONTAINER - Navy/Pink theme */
            .loop-body-container {
                margin: 20px;
                border: 2px dashed #f8bbd9;
                border-radius: 12px;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.03) 0%, rgba(248, 187, 217, 0.08) 100%);
                min-height: 200px;
                position: relative;
            }

            .loop-body-header {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 12px 16px;
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                border-radius: 10px 10px 0 0;
                color: white;
            }

            .loop-body-header h5 {
                margin: 0;
                font-size: 13px;
                font-weight: 600;
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .loop-body-header .loop-indicator {
                background: rgba(248, 187, 217, 0.3);
                padding: 4px 10px;
                border-radius: 12px;
                font-size: 11px;
            }

            .loop-body-content {
                padding: 16px;
                min-height: 150px;
            }

            .loop-body-empty {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 40px 20px;
                color: #94a3b8;
                text-align: center;
            }

            .loop-body-empty i {
                font-size: 48px;
                margin-bottom: 16px;
                opacity: 0.5;
            }

            .loop-body-empty p {
                margin: 0 0 8px 0;
                font-size: 14px;
            }

            .loop-body-empty small {
                font-size: 12px;
                opacity: 0.8;
            }

            /* Child steps in loop body */
            .loop-child-step {
                display: flex;
                align-items: center;
                gap: 12px;
                padding: 12px 16px;
                background: white;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                margin-bottom: 8px;
                cursor: grab;
                transition: all 0.2s ease;
            }

            .loop-child-step:hover {
                border-color: #f8bbd9;
                box-shadow: 0 2px 8px rgba(248, 187, 217, 0.25);
            }

            .loop-child-step.dragging {
                opacity: 0.5;
                border-style: dashed;
            }

            .loop-child-step-number {
                width: 28px;
                height: 28px;
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                color: white;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 12px;
                font-weight: 600;
                flex-shrink: 0;
            }

            .loop-child-step-icon {
                width: 36px;
                height: 36px;
                background: #fdf2f8;
                border-radius: 8px;
                display: flex;
                align-items: center;
                justify-content: center;
                color: #1e3a8a;
                flex-shrink: 0;
            }

            .loop-child-step-info {
                flex: 1;
                min-width: 0;
            }

            .loop-child-step-info h6 {
                margin: 0;
                font-size: 14px;
                font-weight: 600;
                color: #334155;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .loop-child-step-info small {
                color: #94a3b8;
                font-size: 11px;
            }

            .loop-child-step-actions {
                display: flex;
                gap: 4px;
                flex-shrink: 0;
            }

            .loop-child-step-actions button {
                width: 28px;
                height: 28px;
                border: none;
                background: #f1f5f9;
                border-radius: 6px;
                cursor: pointer;
                color: #64748b;
                display: flex;
                align-items: center;
                justify-content: center;
                transition: all 0.2s;
            }

            .loop-child-step-actions button:hover {
                background: #e2e8f0;
                color: #334155;
            }

            .loop-child-step-actions button.configure-child-step-btn {
                background: #dbeafe;
                color: #2563eb;
            }

            .loop-child-step-actions button.configure-child-step-btn:hover {
                background: #bfdbfe;
                color: #1d4ed8;
            }

            .loop-child-step-actions button.remove-child-step-btn:hover {
                background: #fee2e2;
                color: #dc2626;
            }

            /* Add step buttons container */
            .loop-add-step-actions {
                display: flex;
                gap: 8px;
                margin-top: 8px;
            }

            .add-step-to-loop-btn {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 8px;
                flex: 1;
                padding: 12px;
                border: 2px dashed #cbd5e1;
                border-radius: 8px;
                background: transparent;
                color: #64748b;
                font-size: 13px;
                font-weight: 600;
                cursor: pointer;
                transition: all 0.2s;
            }

            .add-step-to-loop-btn:hover {
                border-color: #f8bbd9;
                color: #1e3a8a;
                background: rgba(248, 187, 217, 0.1);
            }

            .add-step-to-loop-btn.primary {
                border-style: solid;
                border-color: #1e3a8a;
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                color: white;
            }

            .add-step-to-loop-btn.primary:hover {
                background: linear-gradient(135deg, #1e40af 0%, #2563eb 100%);
            }

            /* Step selector dropdown */
            .step-selector-dropdown {
                position: absolute;
                top: 100%;
                left: 16px;
                right: 16px;
                background: white;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                box-shadow: 0 4px 20px rgba(0,0,0,0.15);
                max-height: 400px;
                overflow-y: auto;
                z-index: 1000;
            }

            .step-selector-header {
                padding: 12px 16px;
                background: #f8fafc;
                border-bottom: 1px solid #e2e8f0;
                font-size: 12px;
                font-weight: 600;
                color: #64748b;
                text-transform: uppercase;
                letter-spacing: 0.5px;
                position: sticky;
                top: 0;
            }

            .step-selector-section {
                border-bottom: 1px solid #e2e8f0;
            }

            .step-selector-section:last-child {
                border-bottom: none;
            }

            .step-selector-section-title {
                padding: 8px 16px;
                background: #fdf2f8;
                font-size: 11px;
                font-weight: 600;
                color: #1e3a8a;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .step-selector-item {
                display: flex;
                align-items: center;
                gap: 12px;
                padding: 12px 16px;
                cursor: pointer;
                border-bottom: 1px solid #f1f5f9;
                transition: background 0.15s;
            }

            .step-selector-item:hover {
                background: #fdf2f8;
            }

            .step-selector-item:last-child {
                border-bottom: none;
            }

            .step-selector-item.create-new {
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.03) 0%, rgba(248, 187, 217, 0.08) 100%);
            }

            .step-selector-item.create-new:hover {
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.08) 0%, rgba(248, 187, 217, 0.15) 100%);
            }

            /* Loop settings */
            .loop-settings-section {
                padding: 16px 20px;
                background: #f8fafc;
                border-top: 1px solid #e2e8f0;
            }

            .loop-settings-section h4 {
                margin: 0 0 12px 0;
                font-size: 13px;
                font-weight: 600;
                color: #64748b;
            }

            .loop-setting-row {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 8px 0;
            }

            .loop-setting-row label {
                font-size: 13px;
                color: #475569;
            }

            .loop-setting-row input[type="number"] {
                width: 80px;
                padding: 6px 10px;
                border: 1px solid #e2e8f0;
                border-radius: 6px;
                font-size: 13px;
            }

            /* Toggle switch */
            .toggle-switch {
                position: relative;
                width: 44px;
                height: 24px;
            }

            .toggle-switch input {
                opacity: 0;
                width: 0;
                height: 0;
            }

            .toggle-slider {
                position: absolute;
                cursor: pointer;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background-color: #cbd5e1;
                transition: 0.3s;
                border-radius: 24px;
            }

            .toggle-slider:before {
                position: absolute;
                content: "";
                height: 18px;
                width: 18px;
                left: 3px;
                bottom: 3px;
                background-color: white;
                transition: 0.3s;
                border-radius: 50%;
            }

            .toggle-switch input:checked + .toggle-slider {
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
            }

            .toggle-switch input:checked + .toggle-slider:before {
                transform: translateX(20px);
            }

            /* Iteration variables preview - Navy/Pink theme */
            .loop-variables-preview {
                margin-top: 16px;
                padding: 12px;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.03) 0%, rgba(248, 187, 217, 0.1) 100%);
                border: 1px solid #f8bbd9;
                border-radius: 8px;
            }

            .loop-variables-preview h5 {
                margin: 0 0 8px 0;
                font-size: 12px;
                font-weight: 600;
                color: #1e3a8a;
            }

            .loop-variable-item {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 12px;
                color: #334155;
                margin-bottom: 4px;
            }

            .loop-variable-item code {
                background: white;
                padding: 2px 6px;
                border-radius: 4px;
                font-family: monospace;
                color: #1e3a8a;
            }

            /* ============================================
               Nested Loop UI Styles - Navy/Pink theme
               ============================================ */

            /* Nested loop info container */
            .nested-loop-info {
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.05) 0%, rgba(248, 187, 217, 0.15) 100%);
                border: 1px solid #f8bbd9;
                border-radius: 10px;
                padding: 14px 16px;
                margin-bottom: 16px;
                display: flex;
                align-items: center;
                justify-content: space-between;
                flex-wrap: wrap;
                gap: 12px;
            }

            /* Nested loop badge */
            .nested-loop-badge {
                display: inline-flex;
                align-items: center;
                gap: 8px;
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                color: white;
                padding: 6px 14px;
                border-radius: 20px;
                font-size: 12px;
                font-weight: 600;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .nested-loop-badge i {
                font-size: 11px;
            }

            /* Parent loop context display */
            .parent-loop-context {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 13px;
                color: #334155;
            }

            .parent-loop-context .parent-label {
                color: #64748b;
            }

            .parent-loop-context .parent-name {
                font-weight: 600;
                color: #1e3a8a;
            }

            .parent-loop-context .parent-vars {
                font-family: 'Monaco', 'Menlo', monospace;
                background: rgba(255, 255, 255, 0.8);
                padding: 3px 8px;
                border-radius: 4px;
                font-size: 11px;
                color: #1e3a8a;
            }

            /* Relative to parent toggle row */
            .relative-toggle-row {
                background: linear-gradient(135deg, rgba(248, 187, 217, 0.15) 0%, rgba(248, 187, 217, 0.25) 100%);
                border: 1px solid #f8bbd9;
                border-radius: 8px;
                padding: 12px 16px;
                margin-bottom: 16px;
            }

            .relative-toggle-label {
                display: flex;
                align-items: flex-start;
                gap: 12px;
                cursor: pointer;
            }

            .relative-toggle-label input[type="checkbox"] {
                width: 18px;
                height: 18px;
                margin-top: 2px;
                accent-color: #1e3a8a;
                cursor: pointer;
            }

            .relative-toggle-label .toggle-text {
                display: flex;
                flex-direction: column;
                gap: 2px;
            }

            .relative-toggle-label .toggle-text strong {
                font-size: 14px;
                color: #1e3a8a;
            }

            .relative-toggle-label .toggle-text small {
                font-size: 12px;
                color: #64748b;
            }

            /* Collection shortcuts (quick pick) */
            .collection-shortcuts {
                display: flex;
                align-items: center;
                gap: 10px;
                padding: 12px 0;
                flex-wrap: wrap;
            }

            .collection-shortcuts .shortcuts-label {
                font-size: 12px;
                font-weight: 600;
                color: #64748b;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .collection-shortcuts .shortcuts-list {
                display: flex;
                flex-wrap: wrap;
                gap: 6px;
            }

            .shortcut-btn {
                padding: 6px 12px;
                border: 1px solid #e2e8f0;
                border-radius: 16px;
                background: #f8fafc;
                color: #475569;
                font-size: 12px;
                font-weight: 500;
                cursor: pointer;
                transition: all 0.2s ease;
            }

            .shortcut-btn:hover {
                border-color: #f8bbd9;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.05) 0%, rgba(248, 187, 217, 0.15) 100%);
                color: #1e3a8a;
                transform: translateY(-1px);
            }

            .shortcut-btn:active {
                transform: translateY(0);
            }

            /* Collection input wrapper for prefix */
            .collection-input-wrapper {
                display: flex;
                align-items: center;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                overflow: hidden;
                transition: border-color 0.2s, box-shadow 0.2s;
            }

            .collection-input-wrapper:focus-within {
                border-color: #1e3a8a;
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }

            .collection-prefix {
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                color: white;
                padding: 10px 12px;
                font-family: 'Monaco', 'Menlo', monospace;
                font-size: 13px;
                font-weight: 600;
                white-space: nowrap;
                border-right: none;
            }

            .collection-input-wrapper input {
                flex: 1;
                padding: 10px 12px;
                border: none;
                font-size: 14px;
                outline: none;
            }

            .collection-input-wrapper input.with-prefix {
                border-radius: 0 8px 8px 0;
            }

            /* ============================================
               NO-CODE VISUAL CARD SELECTION STYLES
               @see docs/LOOP_TYPE_PROVIDERS.md
               ============================================ */

            /* Subtitle below main heading */
            .loop-config-subtitle {
                margin: -8px 0 16px 0;
                font-size: 13px;
                color: #64748b;
            }

            /* Card container */
            .loop-option-cards {
                display: flex;
                flex-direction: column;
                gap: 12px;
                margin-bottom: 20px;
            }

            /* Category label above card groups */
            .loop-option-category-label {
                font-size: 11px;
                font-weight: 600;
                color: #64748b;
                text-transform: uppercase;
                letter-spacing: 0.5px;
                margin-top: 8px;
                margin-bottom: 4px;
                padding-left: 4px;
            }

            /* Card group container */
            .loop-option-category {
                display: flex;
                flex-wrap: wrap;
                gap: 10px;
            }

            /* Utility options (Custom, Fixed Number) - smaller */
            .loop-option-category.utility-options {
                border-top: 1px dashed #e2e8f0;
                padding-top: 12px;
                margin-top: 8px;
            }

            /* Individual option card */
            .loop-option-card {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                gap: 8px;
                width: 100px;
                min-height: 80px;
                padding: 12px 8px;
                border: 2px solid #e2e8f0;
                border-radius: 12px;
                background: white;
                cursor: pointer;
                transition: all 0.2s ease;
                text-align: center;
            }

            .loop-option-card:hover {
                border-color: #f8bbd9;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.03) 0%, rgba(248, 187, 217, 0.08) 100%);
                transform: translateY(-2px);
                box-shadow: 0 4px 12px rgba(248, 187, 217, 0.25);
            }

            .loop-option-card.selected {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.08) 0%, rgba(248, 187, 217, 0.15) 100%);
                box-shadow: 0 2px 8px rgba(30, 58, 138, 0.15);
            }

            .loop-option-card.selected .loop-option-icon {
                background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                color: white;
            }

            .loop-option-card.advanced {
                border-style: dashed;
            }

            /* Card icon */
            .loop-option-icon {
                width: 40px;
                height: 40px;
                background: #f1f5f9;
                border-radius: 10px;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 18px;
                color: #475569;
                transition: all 0.2s ease;
            }

            /* Card label */
            .loop-option-label {
                font-size: 12px;
                font-weight: 600;
                color: #334155;
                line-height: 1.3;
            }

            /* Custom path section (shown when Custom is selected) */
            .custom-path-section {
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                padding: 16px;
                margin-top: 12px;
            }

            .custom-path-input {
                width: 100%;
                padding: 10px 12px;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                font-size: 14px;
                font-family: 'Monaco', 'Menlo', monospace;
            }

            .custom-path-input:focus {
                outline: none;
                border-color: #1e3a8a;
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }

            /* Available variables info box */
            .loop-variables-info {
                background: linear-gradient(135deg, rgba(30, 58, 138, 0.03) 0%, rgba(248, 187, 217, 0.08) 100%);
                border: 1px solid #e2e8f0;
                border-radius: 10px;
                padding: 16px;
                margin-top: 16px;
            }

            .loop-variables-header {
                display: flex;
                align-items: center;
                gap: 8px;
                margin-bottom: 12px;
                font-size: 13px;
                font-weight: 600;
                color: #1e3a8a;
            }

            .loop-variables-header i {
                font-size: 14px;
            }

            .loop-variables-list {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
                gap: 8px;
            }

            .loop-variable-item {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 12px;
                color: #475569;
            }

            .loop-variable-item code {
                background: white;
                padding: 3px 8px;
                border-radius: 4px;
                font-family: 'Monaco', 'Menlo', monospace;
                font-size: 11px;
                color: #1e3a8a;
                border: 1px solid #e2e8f0;
            }

            .loop-variable-item span {
                color: #64748b;
                font-size: 11px;
            }

            /* Fixed count input (shown when Fixed Number is selected) */
            .fixed-count-section {
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                padding: 16px;
                margin-top: 12px;
            }

            .fixed-count-input {
                width: 120px;
                padding: 10px 12px;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                font-size: 16px;
                text-align: center;
            }

            .fixed-count-input:focus {
                outline: none;
                border-color: #1e3a8a;
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }
        `;
        document.head.appendChild(styles);
    }

    // ========================================
    // PRIVATE METHODS
    // ========================================

    /**
     * Generate the complete HTML for the builder
     * @private
     */
    generateHTML() {
        const loopType = this.config.loopType || 'foreach';

        return `
            <div class="loop-builder-container">
                <!-- Header -->
                <div class="loop-builder-header">
                    <div class="loop-builder-header-icon">
                        <i class="fas fa-redo-alt"></i>
                    </div>
                    <div class="loop-builder-header-text">
                        <h3>Loop Configuration</h3>
                        <p>Configure iteration over collections or conditions</p>
                    </div>
                </div>

                <!-- Loop Type Selector -->
                <div class="loop-type-selector">
                    <button type="button" class="loop-type-btn ${loopType === 'foreach' ? 'active' : ''}" data-type="foreach">
                        <i class="fas fa-list"></i>
                        <span>For Each</span>
                    </button>
                    <button type="button" class="loop-type-btn ${loopType === 'for' ? 'active' : ''}" data-type="for">
                        <i class="fas fa-sort-numeric-down"></i>
                        <span>For (N times)</span>
                    </button>
                    <button type="button" class="loop-type-btn ${loopType === 'while' ? 'active' : ''}" data-type="while">
                        <i class="fas fa-sync"></i>
                        <span>While</span>
                    </button>
                </div>

                <!-- Loop Configuration -->
                ${this.renderLoopConfig(loopType)}

                <!-- Loop Body Container - Visual Nesting -->
                <div class="loop-body-container">
                    <div class="loop-body-header">
                        <h5><i class="fas fa-layer-group"></i> Loop Body</h5>
                        <span class="loop-indicator">
                            ${this.getLoopIndicatorText(loopType)}
                        </span>
                    </div>
                    <div class="loop-body-content" id="loopBodyContent">
                        ${this.renderChildSteps()}
                    </div>
                </div>

                <!-- Loop Variables Preview -->
                ${this.renderVariablesPreview(loopType)}

                <!-- Loop Settings -->
                <div class="loop-settings-section">
                    <h4><i class="fas fa-cog"></i> Advanced Settings</h4>

                    <div class="loop-setting-row">
                        <label>Max Iterations (Safety Limit)</label>
                        <input type="number" id="maxIterationsInput"
                               value="${this.config.maxIterations || 1000}"
                               min="1" max="10000">
                    </div>

                    <div class="loop-setting-row">
                        <label>Stop loop on error</label>
                        <label class="toggle-switch">
                            <input type="checkbox" id="breakOnErrorToggle"
                                   ${this.config.breakOnError ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>

                    <div class="loop-setting-row">
                        <label>Continue pipeline if collection is empty</label>
                        <label class="toggle-switch">
                            <input type="checkbox" id="continueOnEmptyToggle"
                                   ${this.config.continueOnEmpty !== false ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render loop-type-specific configuration
     * @private
     */
    renderLoopConfig(loopType) {
        switch (loopType) {
            case 'foreach':
                return this.renderForEachConfig();
            case 'for':
                return this.renderForConfig();
            case 'while':
                return this.renderWhileConfig();
            default:
                return this.renderForEachConfig();
        }
    }

    /**
     * Render For Each loop configuration
     * NO-CODE FRIENDLY: Visual card-based selection instead of technical inputs
     * @see docs/LOOP_TYPE_PROVIDERS.md for architecture documentation
     * @private
     */
    renderForEachConfig() {
        const selectedOption = this.config.selectedLoopOption || null;
        const collectionValue = this.config.collection || '';

        // Get loop options from provider (HL7 or FHIR based on context)
        const options = this.getLoopOptionsForContext();

        return `
            <div class="loop-config-section">
                <h4><i class="fas fa-redo"></i> What should repeat?</h4>
                <p class="loop-config-subtitle">Select what you want to loop through</p>

                <!-- Visual Card Selection -->
                <div class="loop-option-cards" id="loopOptionCards">
                    ${this.renderLoopOptionCards(options, selectedOption)}
                </div>

                <!-- Hidden technical input (populated by card selection) -->
                <input type="hidden" id="collectionInput" value="${collectionValue}">

                <!-- Custom path input (shown only when "Custom" is selected) -->
                <div class="custom-path-section" id="customPathSection" style="display: ${selectedOption === 'custom' ? 'block' : 'none'};">
                    <div class="loop-config-field">
                        <label>Custom Field Path</label>
                        <input type="text" id="customPathInput"
                               value="${collectionValue}"
                               placeholder="e.g., data.items, results.entries"
                               class="custom-path-input">
                        <span class="field-hint">Enter the path to the array you want to iterate</span>
                    </div>
                </div>

                <!-- Available Variables Info (read-only, not configurable) -->
                ${this.renderAvailableVariablesInfo(selectedOption, options)}
            </div>
        `;
    }

    /**
     * Get loop options based on message context (HL7, FHIR, etc.)
     * @see docs/LOOP_TYPE_PROVIDERS.md
     * @private
     */
    getLoopOptionsForContext() {
        // Try to get provider from registry
        if (typeof loopTypeRegistry !== 'undefined') {
            const message = window.pipelineBuilder?.currentMessage;
            const provider = loopTypeRegistry.getProviderForMessage(message);
            if (provider) {
                return provider.getAllOptions();
            }
        }

        // Fallback to built-in HL7 options if providers not loaded
        return this.getDefaultHL7Options();
    }

    /**
     * Default HL7 options (fallback if provider system not loaded)
     * @private
     */
    getDefaultHL7Options() {
        return [
            { id: 'insurance', icon: 'fas fa-hospital', label: 'Each Insurance', description: 'Insurance plans (IN1)', technicalPath: 'IN1', category: 'records' },
            { id: 'observation', icon: 'fas fa-flask', label: 'Each Test Result', description: 'Lab results (OBX)', technicalPath: 'OBX', category: 'results' },
            { id: 'diagnosis', icon: 'fas fa-diagnoses', label: 'Each Diagnosis', description: 'Diagnoses (DG1)', technicalPath: 'DG1', category: 'clinical' },
            { id: 'next-of-kin', icon: 'fas fa-users', label: 'Each Contact', description: 'Contacts (NK1)', technicalPath: 'NK1', category: 'contacts' },
            { id: 'allergy', icon: 'fas fa-allergies', label: 'Each Allergy', description: 'Allergies (AL1)', technicalPath: 'AL1', category: 'clinical' },
            { id: 'notes', icon: 'fas fa-sticky-note', label: 'Each Note', description: 'Notes (NTE)', technicalPath: 'NTE', category: 'notes' },
            { id: 'fixed-count', icon: 'fas fa-hashtag', label: 'Fixed Number', description: 'Repeat N times', technicalPath: null, category: 'utility', isFixedCount: true },
            { id: 'custom', icon: 'fas fa-cog', label: 'Custom Path', description: 'Advanced option', technicalPath: null, category: 'utility', isAdvanced: true }
        ];
    }

    /**
     * Render visual option cards
     * @private
     */
    renderLoopOptionCards(options, selectedOption) {
        // Group by category
        const grouped = {};
        options.forEach(opt => {
            const cat = opt.category || 'other';
            if (!grouped[cat]) grouped[cat] = [];
            grouped[cat].push(opt);
        });

        // Category order and labels
        const categoryMeta = {
            'records': { label: 'Records', order: 1 },
            'results': { label: 'Results', order: 2 },
            'clinical': { label: 'Clinical', order: 3 },
            'contacts': { label: 'Contacts', order: 4 },
            'financial': { label: 'Financial', order: 5 },
            'utility': { label: 'Other Options', order: 99 }
        };

        // Sort categories
        const sortedCategories = Object.keys(grouped).sort((a, b) => {
            return (categoryMeta[a]?.order || 50) - (categoryMeta[b]?.order || 50);
        });

        let html = '';
        for (const category of sortedCategories) {
            const catOptions = grouped[category];
            const catLabel = categoryMeta[category]?.label || category;

            // Only show category header if more than one category
            if (sortedCategories.length > 1 && category !== 'utility') {
                html += `<div class="loop-option-category-label">${catLabel}</div>`;
            }

            html += `<div class="loop-option-category ${category === 'utility' ? 'utility-options' : ''}">`;
            for (const opt of catOptions) {
                const isSelected = selectedOption === opt.id ||
                                   (this.config.collection === opt.technicalPath && opt.technicalPath);
                html += `
                    <div class="loop-option-card ${isSelected ? 'selected' : ''} ${opt.isAdvanced ? 'advanced' : ''}"
                         data-option-id="${opt.id}"
                         data-technical-path="${opt.technicalPath || ''}"
                         title="${opt.description}">
                        <div class="loop-option-icon">
                            <i class="${opt.icon}"></i>
                        </div>
                        <div class="loop-option-label">${opt.label}</div>
                    </div>
                `;
            }
            html += '</div>';
        }

        return html;
    }

    /**
     * Render available variables info (read-only display)
     * @private
     */
    renderAvailableVariablesInfo(selectedOption, options) {
        const option = options.find(o => o.id === selectedOption);

        return `
            <div class="loop-variables-info">
                <div class="loop-variables-header">
                    <i class="fas fa-info-circle"></i>
                    Inside the loop, you can use:
                </div>
                <div class="loop-variables-list">
                    <div class="loop-variable-item">
                        <code>loop.item</code>
                        <span>The current item</span>
                    </div>
                    <div class="loop-variable-item">
                        <code>loop.position</code>
                        <span>Position number (1, 2, 3...)</span>
                    </div>
                    <div class="loop-variable-item">
                        <code>loop.total</code>
                        <span>Total count</span>
                    </div>
                    <div class="loop-variable-item">
                        <code>loop.isFirst</code>
                        <span>True if first item</span>
                    </div>
                    <div class="loop-variable-item">
                        <code>loop.isLast</code>
                        <span>True if last item</span>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * ✅ NEW: Handle visual card selection for no-code loop configuration
     * @see docs/LOOP_TYPE_PROVIDERS.md for architecture documentation
     * @param {HTMLElement} cardElement - The clicked card element
     * @private
     */
    handleCardSelection(cardElement) {
        const optionId = cardElement.dataset.optionId;
        const technicalPath = cardElement.dataset.technicalPath;

        console.log('[ForEachLoopBuilder] Card selected:', optionId, 'path:', technicalPath);

        // Update visual selection
        this.container.querySelectorAll('.loop-option-card').forEach(card => {
            card.classList.remove('selected');
        });
        cardElement.classList.add('selected');

        // Store the selected option
        this.config.selectedLoopOption = optionId;

        // Handle special options
        if (optionId === 'custom') {
            // Show custom path input
            const customPathSection = this.container.querySelector('#customPathSection');
            if (customPathSection) {
                customPathSection.style.display = 'block';
            }
            // Initialize field search on the custom path input (now that it's visible)
            const customPathInput = this.container.querySelector('#customPathInput');
            if (customPathInput && !customPathInput._fieldSearchInitialized) {
                this.initFieldSearch(customPathInput);
                customPathInput._fieldSearchInitialized = true;
            }
            // Keep existing collection value or clear it
            if (!this.config.collection || this.isStandardPath(this.config.collection)) {
                this.config.collection = '';
            }
        } else if (optionId === 'fixed-count') {
            // Switch to 'for' loop type
            this.config.loopType = 'for';
            this.config.collection = '';
            // Re-render to show the fixed count UI
            this.render();
            this.attachEvents();
        } else if (optionId === 'count-of') {
            // Show count-of selector (similar to custom but with field picker)
            const customPathSection = this.container.querySelector('#customPathSection');
            if (customPathSection) {
                customPathSection.style.display = 'block';
            }
            this.config.collection = '';
        } else {
            // Standard option - use the technical path
            this.config.collection = technicalPath;
            this.config.loopType = 'foreach';

            // Hide custom path input
            const customPathSection = this.container.querySelector('#customPathSection');
            if (customPathSection) {
                customPathSection.style.display = 'none';
            }

            // Auto-set item variable name based on selection
            this.config.itemVariable = this.getRecommendedItemVariable(optionId);
        }

        // Update the hidden collection input
        const collectionInput = this.container.querySelector('#collectionInput');
        if (collectionInput) {
            collectionInput.value = this.config.collection;
        }

        this.notifyChange();
    }

    /**
     * Check if a path is a standard provider path
     * @param {string} path - The path to check
     * @returns {boolean} True if it's a standard path
     * @private
     */
    isStandardPath(path) {
        const standardPaths = [
            'IN1', 'IN2', 'GT1', 'OBX', 'OBR', 'DG1', 'PR1', 'AL1', 'NK1', 'AIS', 'NTE',
            'Bundle.entry', 'contained', 'identifier', 'name', 'address', 'telecom',
            'Condition', 'Observation', 'MedicationRequest', 'AllergyIntolerance',
            'Procedure', 'Coverage', 'Encounter', 'Appointment', 'extension', 'coding'
        ];
        return standardPaths.includes(path);
    }

    /**
     * Get recommended item variable name based on selection
     * @param {string} optionId - The selected option ID
     * @returns {string} Recommended variable name
     * @private
     */
    getRecommendedItemVariable(optionId) {
        const variableNames = {
            // HL7 options
            'insurance': 'insurance',
            'guarantor': 'guarantor',
            'observation': 'result',
            'order': 'order',
            'diagnosis': 'diagnosis',
            'procedure': 'procedure',
            'allergy': 'allergy',
            'next-of-kin': 'contact',
            'appointment': 'appointment',
            'notes': 'note',
            // FHIR options
            'bundle-entry': 'entry',
            'contained': 'resource',
            'identifier': 'identifier',
            'name': 'name',
            'address': 'address',
            'telecom': 'telecom',
            'condition': 'condition',
            'medication': 'medication',
            'coverage': 'coverage',
            'encounter': 'encounter',
            'extension': 'extension',
            'coding': 'coding'
        };
        return variableNames[optionId] || 'item';
    }

    /**
     * ✅ NEW: Render the nested loop section with parent context info
     * @private
     */
    renderNestedLoopSection() {
        if (!this.isNestedLoop || !this.parentLoopInfo) return '';

        return `
            <div class="nested-loop-info">
                <div class="nested-loop-badge">
                    <i class="fas fa-layer-group"></i>
                    Nested Loop
                </div>
                <div class="parent-loop-context">
                    <span class="parent-label">Parent Loop:</span>
                    <span class="parent-name">${this.parentLoopInfo.name}</span>
                    <span class="parent-vars">
                        (${this.parentLoopInfo.itemVariable}, ${this.parentLoopInfo.indexVariable})
                    </span>
                </div>
            </div>

            <div class="relative-toggle-row">
                <label class="relative-toggle-label">
                    <input type="checkbox" id="relativeToParentToggle"
                           ${this.config.relativeToParent ? 'checked' : ''}>
                    <span class="toggle-text">
                        <strong>Collection relative to parent item</strong>
                        <small>Iterate over an array property of the parent loop's current item</small>
                    </span>
                </label>
            </div>
        `;
    }

    /**
     * ✅ NEW: Render quick-pick shortcuts for common collection patterns
     * @private
     */
    renderCollectionShortcuts() {
        // Different shortcuts depending on whether we're in a nested loop
        let shortcuts = [];

        if (this.isNestedLoop && this.config.relativeToParent) {
            // Shortcuts for nested loops (relative to parent item)
            shortcuts = [
                { label: 'obxList', value: 'obxList', desc: 'OBX segments in group' },
                { label: 'nteList', value: 'nteList', desc: 'NTE notes in group' },
                { label: 'children', value: 'children', desc: 'Child elements' },
                { label: 'items', value: 'items', desc: 'Items array' },
                { label: 'fields', value: 'fields', desc: 'Field list' }
            ];
        } else {
            // Shortcuts for top-level loops (HL7 segments)
            shortcuts = [
                { label: 'OBX', value: 'OBX', desc: 'Observation segments' },
                { label: 'OBR', value: 'OBR', desc: 'Order segments' },
                { label: 'IN1', value: 'IN1', desc: 'Insurance segments' },
                { label: 'NK1', value: 'NK1', desc: 'Next of kin' },
                { label: 'DG1', value: 'DG1', desc: 'Diagnosis segments' },
                { label: 'observationGroups', value: 'observationGroups', desc: 'OBR-OBX groups' }
            ];
        }

        return `
            <div class="collection-shortcuts">
                <span class="shortcuts-label">Quick pick:</span>
                <div class="shortcuts-list">
                    ${shortcuts.map(s => `
                        <button type="button" class="shortcut-btn" data-value="${s.value}" title="${s.desc}">
                            ${s.label}
                        </button>
                    `).join('')}
                </div>
            </div>
        `;
    }

    /**
     * Render For loop configuration
     * @private
     */
    renderForConfig() {
        return `
            <div class="loop-config-section">
                <h4><i class="fas fa-sort-numeric-down"></i> Iteration Count</h4>

                <div class="loop-config-row">
                    <div class="loop-config-field">
                        <label>Number of Iterations</label>
                        <input type="number" id="iterationsInput"
                               value="${this.config.iterations || 10}"
                               min="1" max="10000">
                        <span class="field-hint">How many times to execute the loop body</span>
                    </div>
                    <div class="loop-config-field">
                        <label>Index Variable Name</label>
                        <input type="text" id="indexVariableInput"
                               value="${this.config.indexVariable || 'index'}"
                               placeholder="index">
                        <span class="field-hint">Access current index as: loop.{name}</span>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render While loop configuration
     * @private
     */
    renderWhileConfig() {
        const condition = this.config.condition || {};
        const operators = [
            { value: 'equals', label: 'Equals' },
            { value: 'not_equals', label: 'Not Equals' },
            { value: 'contains', label: 'Contains' },
            { value: 'not_empty', label: 'Is Not Empty' },
            { value: 'is_empty', label: 'Is Empty' },
            { value: 'greater_than', label: 'Greater Than' },
            { value: 'less_than', label: 'Less Than' },
            { value: 'true', label: 'Is True' },
            { value: 'false', label: 'Is False' }
        ];

        return `
            <div class="loop-config-section">
                <h4><i class="fas fa-question-circle"></i> Loop Condition</h4>
                <p style="font-size: 12px; color: #64748b; margin-bottom: 16px;">
                    Loop continues while this condition is TRUE
                </p>

                <div class="loop-config-row single">
                    <div class="loop-config-field">
                        <label>Field to Check</label>
                        <input type="text" id="conditionFieldInput"
                               value="${condition.field || ''}"
                               placeholder="e.g., loop.hasMore or data.remaining">
                    </div>
                </div>

                <div class="loop-config-row">
                    <div class="loop-config-field">
                        <label>Operator</label>
                        <select id="conditionOperatorSelect">
                            ${operators.map(op => `
                                <option value="${op.value}" ${condition.operator === op.value ? 'selected' : ''}>
                                    ${op.label}
                                </option>
                            `).join('')}
                        </select>
                    </div>
                    <div class="loop-config-field">
                        <label>Value</label>
                        <input type="text" id="conditionValueInput"
                               value="${condition.value || ''}"
                               placeholder="Comparison value">
                    </div>
                </div>

                <div class="loop-config-row">
                    <div class="loop-config-field">
                        <label>Index Variable Name</label>
                        <input type="text" id="indexVariableInput"
                               value="${this.config.indexVariable || 'index'}"
                               placeholder="index">
                        <span class="field-hint">Access current iteration as: loop.{name}</span>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render child steps in loop body
     * @private
     */
    renderChildSteps() {
        const childStepIds = this.config.childStepIds || [];

        if (childStepIds.length === 0) {
            return `
                <div class="loop-body-empty">
                    <i class="fas fa-inbox"></i>
                    <p>No steps in loop body</p>
                    <small>Add steps that will execute on each iteration</small>
                </div>
                <div class="loop-add-step-actions">
                    <button type="button" class="add-step-to-loop-btn primary" id="createNewStepBtn">
                        <i class="fas fa-plus-circle"></i>
                        Create New Step
                    </button>
                    <button type="button" class="add-step-to-loop-btn" id="addExistingStepBtn">
                        <i class="fas fa-link"></i>
                        Link Existing
                    </button>
                </div>
            `;
        }

        const stepsHTML = childStepIds.map((stepId, index) => {
            const step = this.getStepById(stepId);
            if (!step) return '';

            return `
                <div class="loop-child-step" data-step-id="${stepId}" draggable="true">
                    <div class="loop-child-step-number">${index + 1}</div>
                    <div class="loop-child-step-icon">
                        <i class="${step.icon || 'fas fa-cog'}"></i>
                    </div>
                    <div class="loop-child-step-info">
                        <h6>${step.stepName || step.name || 'Unnamed Step'}</h6>
                        <small>${step.stepType || step.type || ''}</small>
                    </div>
                    <div class="loop-child-step-actions">
                        <button type="button" class="configure-child-step-btn" data-step-id="${stepId}" title="Configure step">
                            <i class="fas fa-cog"></i>
                        </button>
                        <button type="button" class="move-step-up-btn" data-step-id="${stepId}"
                                ${index === 0 ? 'disabled' : ''}>
                            <i class="fas fa-chevron-up"></i>
                        </button>
                        <button type="button" class="move-step-down-btn" data-step-id="${stepId}"
                                ${index === childStepIds.length - 1 ? 'disabled' : ''}>
                            <i class="fas fa-chevron-down"></i>
                        </button>
                        <button type="button" class="remove-child-step-btn" data-step-id="${stepId}">
                            <i class="fas fa-times"></i>
                        </button>
                    </div>
                </div>
            `;
        }).join('');

        return `
            ${stepsHTML}
            <div class="loop-add-step-actions">
                <button type="button" class="add-step-to-loop-btn primary" id="createNewStepBtn">
                    <i class="fas fa-plus-circle"></i>
                    Create New Step
                </button>
                <button type="button" class="add-step-to-loop-btn" id="addExistingStepBtn">
                    <i class="fas fa-link"></i>
                    Link Existing
                </button>
            </div>
        `;
    }

    /**
     * Render loop variables preview
     * @private
     */
    renderVariablesPreview(loopType) {
        const itemVar = this.config.itemVariable || 'item';
        const indexVar = this.config.indexVariable || 'index';

        let variables = [];

        if (loopType === 'foreach') {
            variables = [
                { name: `loop.${itemVar}`, desc: 'Current item in iteration' },
                { name: `loop.${indexVar}`, desc: 'Current index (0-based)' },
                { name: 'loop.isFirst', desc: 'True if first iteration' },
                { name: 'loop.isLast', desc: 'True if last iteration' },
                { name: 'loop.length', desc: 'Total number of items' }
            ];
        } else if (loopType === 'for') {
            variables = [
                { name: `loop.${indexVar}`, desc: 'Current index (0-based)' },
                { name: 'loop.iteration', desc: 'Current iteration (1-based)' },
                { name: 'loop.isFirst', desc: 'True if first iteration' },
                { name: 'loop.isLast', desc: 'True if last iteration' },
                { name: 'loop.total', desc: 'Total number of iterations' }
            ];
        } else if (loopType === 'while') {
            variables = [
                { name: `loop.${indexVar}`, desc: 'Current index (0-based)' },
                { name: 'loop.iteration', desc: 'Current iteration (1-based)' }
            ];
        }

        return `
            <div class="loop-variables-preview" style="margin: 0 20px 20px 20px;">
                <h5><i class="fas fa-code"></i> Available Loop Variables</h5>
                ${variables.map(v => `
                    <div class="loop-variable-item">
                        <code>${v.name}</code>
                        <span>- ${v.desc}</span>
                    </div>
                `).join('')}
            </div>
        `;
    }

    /**
     * Get loop indicator text based on type
     * @private
     */
    getLoopIndicatorText(loopType) {
        switch (loopType) {
            case 'foreach':
                return `Iterates: ${this.config.collection || '(select collection)'}`;
            case 'for':
                return `Repeats: ${this.config.iterations || 10} times`;
            case 'while':
                return `While: condition is true`;
            default:
                return 'Loop';
        }
    }

    /**
     * Get step by ID from available steps or pipeline
     * @private
     */
    getStepById(stepId) {
        // Check pipeline first to get the actual VisualStep object
        // (availableSteps contains simplified copies with different property names)
        const pipeline = window.pipelineBuilder?.getPipeline();
        if (pipeline) {
            // Use flat collection pattern to find step
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
            const found = allSteps.find(s => s.id === stepId);
            if (found) return found;
        }

        // Fallback to available steps (simplified objects with name/type instead of stepName/stepType)
        const availableStep = this.availableSteps.find(s => s.id === stepId);
        if (availableStep) return availableStep;

        return null;
    }

    /**
     * Initialize field search component
     * @private
     */
    initFieldSearch(input) {
        if (typeof FieldPathSearchComponent !== 'undefined' && this.cachedStepVariables) {
            this.fieldSearchComponent = new FieldPathSearchComponent(input, {
                fields: this.cachedStepVariables,
                onSelect: (field) => {
                    this.config.collection = field.path;
                    input.value = field.path;
                    this.notifyChange();
                }
            });
        }
    }

    /**
     * Get available step templates for creating new steps
     * @private
     */
    getStepTemplates() {
        // Common step types that can be created inside a loop
        // Step types must match the existing step types in PropertiesPanel for correct UI rendering
        return [
            {
                category: 'Enrichment',
                templates: [
                    { id: 'api-enrichment', name: 'API Enrichment', icon: 'fas fa-cloud-download-alt', type: 'pre.enrichment.api' },
                    { id: 'database-enrichment', name: 'Database Lookup', icon: 'fas fa-database', type: 'pre.enrichment.database' },
                    { id: 'field-mapping', name: 'Field Mapping', icon: 'fas fa-exchange-alt', type: 'core.transformation' },
                    { id: 'script-enrichment', name: 'Script Enrichment', icon: 'fas fa-code', type: 'pre.enrichment.script' }
                ]
            },
            {
                category: 'Validation',
                templates: [
                    { id: 'field-validation', name: 'Field Validation', icon: 'fas fa-check-circle', type: 'pre.validation' }
                ]
            },
            {
                category: 'Control Flow',
                templates: [
                    { id: 'if-then-else', name: 'If-Then-Else', icon: 'fas fa-code-branch', type: 'pre.logic' },
                    { id: 'switch-case', name: 'Switch/Case', icon: 'fas fa-sitemap', type: 'pre.logic.switch' },
                    { id: 'nested-loop', name: 'Nested Loop', icon: 'fas fa-redo-alt', type: 'control.loop' }
                ]
            }
        ];
    }

    /**
     * Show step type selector for creating new steps
     * @private
     */
    showStepTypeSelector() {
        // Remove existing dropdown
        const existing = this.container.querySelector('.step-selector-dropdown');
        if (existing) {
            existing.remove();
            return;
        }

        const stepTemplates = this.getStepTemplates();

        const dropdown = document.createElement('div');
        dropdown.className = 'step-selector-dropdown';

        let html = '<div class="step-selector-header"><i class="fas fa-plus-circle"></i> Create New Step</div>';

        stepTemplates.forEach(category => {
            html += `<div class="step-selector-section">
                <div class="step-selector-section-title">${category.category}</div>
                ${category.templates.map(template => `
                    <div class="step-selector-item create-new" data-template-id="${template.id}" data-template-type="${template.type}">
                        <div class="loop-child-step-icon">
                            <i class="${template.icon}"></i>
                        </div>
                        <div class="loop-child-step-info">
                            <h6>${template.name}</h6>
                            <small>${template.type}</small>
                        </div>
                    </div>
                `).join('')}
            </div>`;
        });

        dropdown.innerHTML = html;

        const loopBody = this.container.querySelector('.loop-body-container');
        loopBody.style.position = 'relative';
        loopBody.appendChild(dropdown);

        // Attach click events for step creation
        dropdown.querySelectorAll('.step-selector-item').forEach(item => {
            item.addEventListener('click', () => {
                const templateId = item.dataset.templateId;
                const templateType = item.dataset.templateType;
                this.createNewChildStep(templateId, templateType);
                dropdown.remove();
            });
        });

        // Close on outside click
        this.attachDropdownCloseHandler(dropdown);
    }

    /**
     * Show existing step selector for linking existing steps
     * @private
     */
    showExistingStepSelector() {
        // Remove existing dropdown
        const existing = this.container.querySelector('.step-selector-dropdown');
        if (existing) {
            existing.remove();
            return;
        }

        // Reload available steps
        this.loadAvailableSteps();

        // Filter out steps already in loop
        const availableToAdd = this.availableSteps.filter(
            s => !this.config.childStepIds.includes(s.id)
        );

        if (availableToAdd.length === 0) {
            // Show empty state message
            const dropdown = document.createElement('div');
            dropdown.className = 'step-selector-dropdown';
            dropdown.innerHTML = `
                <div class="step-selector-header"><i class="fas fa-link"></i> Link Existing Step</div>
                <div style="padding: 20px; text-align: center; color: #64748b;">
                    <i class="fas fa-info-circle" style="font-size: 24px; margin-bottom: 8px; display: block;"></i>
                    <p style="margin: 0;">No available steps to link.</p>
                    <small>Create steps in the pipeline first, or use "Create New Step".</small>
                </div>
            `;

            const loopBody = this.container.querySelector('.loop-body-container');
            loopBody.style.position = 'relative';
            loopBody.appendChild(dropdown);
            this.attachDropdownCloseHandler(dropdown);
            return;
        }

        const dropdown = document.createElement('div');
        dropdown.className = 'step-selector-dropdown';
        dropdown.innerHTML = `
            <div class="step-selector-header"><i class="fas fa-link"></i> Link Existing Step</div>
            ${availableToAdd.map(step => `
                <div class="step-selector-item" data-step-id="${step.id}">
                    <div class="loop-child-step-icon">
                        <i class="${step.icon}"></i>
                    </div>
                    <div class="loop-child-step-info">
                        <h6>${step.name}</h6>
                        <small>${step.type}</small>
                    </div>
                </div>
            `).join('')}
        `;

        const loopBody = this.container.querySelector('.loop-body-container');
        loopBody.style.position = 'relative';
        loopBody.appendChild(dropdown);

        // Attach click events
        dropdown.querySelectorAll('.step-selector-item').forEach(item => {
            item.addEventListener('click', () => {
                const stepId = item.dataset.stepId;
                this.addChildStep(stepId);
                dropdown.remove();
            });
        });

        // Close on outside click
        this.attachDropdownCloseHandler(dropdown);
    }

    /**
     * Attach close handler to dropdown
     * @private
     */
    attachDropdownCloseHandler(dropdown) {
        setTimeout(() => {
            document.addEventListener('click', function closeDropdown(e) {
                if (!dropdown.contains(e.target)) {
                    dropdown.remove();
                    document.removeEventListener('click', closeDropdown);
                }
            });
        }, 100);
    }

    /**
     * Generate a valid UUID v4
     * @private
     */
    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }

    /**
     * Create a new child step and add it to the loop
     * @private
     */
    createNewChildStep(templateId, templateType) {
        // Generate unique step ID (must be valid UUID for database)
        const stepId = this.generateUUID();

        // Get the current step (the loop step)
        const currentStep = window.pipelineBuilder?.currentStep;
        const currentLayer = currentStep?.layer || 'pre';

        // Build the new step object
        const templates = this.getStepTemplates().flatMap(cat => cat.templates);
        const template = templates.find(t => t.id === templateId);

        if (!template) {
            console.error('[ForEachLoopBuilder] Template not found:', templateId);
            return;
        }

        const newStep = {
            id: stepId,
            stepName: `${template.name} ${this.config.childStepIds.length + 1}`,
            stepType: templateType,
            templateId: templateId,
            icon: template.icon,
            config: {},
            enabled: true,
            // Link to parent loop - use parent_conditional_step_id for database FK constraint
            parent_conditional_step_id: currentStep?.id,
            parentConditionalStepId: currentStep?.id, // Also set the JS property for VisualStep compatibility
            layer: currentLayer
        };

        // Add step to the pipeline via pipelineBuilder if available
        if (window.pipelineBuilder) {
            try {
                // Add the step to the pipeline
                window.pipelineBuilder.addStepToLayer(newStep, currentLayer);

                // Add to child step IDs
                this.addChildStep(stepId);

                console.log('[ForEachLoopBuilder] Created new child step:', newStep);
            } catch (err) {
                console.error('[ForEachLoopBuilder] Failed to create step:', err);
                // Fallback: just add to childStepIds config
                this.addChildStep(stepId);
            }
        } else {
            // Fallback: just add to childStepIds config
            this.addChildStep(stepId);
        }
    }

    /**
     * Add a child step to the loop
     * @private
     */
    addChildStep(stepId) {
        if (!this.config.childStepIds) {
            this.config.childStepIds = [];
        }
        if (!this.config.childStepIds.includes(stepId)) {
            this.config.childStepIds.push(stepId);
            this.render();
            this.attachEvents();
            this.notifyChange();
        }
    }

    /**
     * Remove a child step from the loop
     * @private
     */
    removeChildStep(stepId) {
        this.config.childStepIds = (this.config.childStepIds || []).filter(id => id !== stepId);

        // Also remove the step from the pipeline to prevent orphans
        if (window.pipelineBuilder) {
            window.pipelineBuilder.deleteStep(stepId);
            console.log('[ForEachLoopBuilder] Deleted child step from pipeline:', stepId);
        }

        this.render();
        this.attachEvents();
        this.notifyChange();
    }

    /**
     * Open the properties panel for a child step
     * @private
     */
    openStepProperties(stepId) {
        console.log('[ForEachLoopBuilder] Opening properties for child step:', stepId);

        // Find the step in the pipeline
        const step = this.getStepById(stepId);
        if (!step) {
            console.error('[ForEachLoopBuilder] Step not found:', stepId);
            return;
        }

        // Use the properties panel to open the step's configuration
        if (window.propertiesPanel) {
            window.propertiesPanel.showStepProperties(step);
        } else if (window.pipelineBuilder && window.pipelineBuilder.propertiesPanel) {
            window.pipelineBuilder.propertiesPanel.showStepProperties(step);
        } else {
            console.error('[ForEachLoopBuilder] PropertiesPanel not available');
            AppDialogs.toast('Unable to open step configuration. Please try clicking on the step in the flowchart.', 'warning');
        }
    }

    /**
     * Move a child step up or down
     * @private
     */
    moveChildStep(stepId, direction) {
        const index = this.config.childStepIds.indexOf(stepId);
        if (index === -1) return;

        const newIndex = index + direction;
        if (newIndex < 0 || newIndex >= this.config.childStepIds.length) return;

        // Swap
        const temp = this.config.childStepIds[index];
        this.config.childStepIds[index] = this.config.childStepIds[newIndex];
        this.config.childStepIds[newIndex] = temp;

        this.render();
        this.attachEvents();
        this.notifyChange();
    }

    /**
     * Setup drag and drop for reordering
     * @private
     */
    setupDragAndDrop() {
        const loopBody = this.container.querySelector('#loopBodyContent');
        if (!loopBody) return;

        loopBody.querySelectorAll('.loop-child-step').forEach(step => {
            step.addEventListener('dragstart', (e) => {
                this.draggedStepId = e.currentTarget.dataset.stepId;
                e.currentTarget.classList.add('dragging');
            });

            step.addEventListener('dragend', (e) => {
                e.currentTarget.classList.remove('dragging');
                this.draggedStepId = null;
            });

            step.addEventListener('dragover', (e) => {
                e.preventDefault();
                const draggingOver = e.currentTarget;
                if (draggingOver.dataset.stepId !== this.draggedStepId) {
                    draggingOver.style.borderTop = '2px solid #1e3a8a';
                }
            });

            step.addEventListener('dragleave', (e) => {
                e.currentTarget.style.borderTop = '';
            });

            step.addEventListener('drop', (e) => {
                e.preventDefault();
                e.currentTarget.style.borderTop = '';

                const targetId = e.currentTarget.dataset.stepId;
                if (this.draggedStepId && targetId !== this.draggedStepId) {
                    this.reorderChildSteps(this.draggedStepId, targetId);
                }
            });
        });
    }

    /**
     * Reorder child steps after drag and drop
     * @private
     */
    reorderChildSteps(draggedId, targetId) {
        const ids = [...this.config.childStepIds];
        const draggedIndex = ids.indexOf(draggedId);
        const targetIndex = ids.indexOf(targetId);

        if (draggedIndex === -1 || targetIndex === -1) return;

        // Remove dragged item
        ids.splice(draggedIndex, 1);
        // Insert at target position
        ids.splice(targetIndex, 0, draggedId);

        this.config.childStepIds = ids;
        this.render();
        this.attachEvents();
        this.notifyChange();
    }

    /**
     * Attach while condition events
     * @private
     */
    attachConditionEvents() {
        const fieldInput = this.container.querySelector('#conditionFieldInput');
        const operatorSelect = this.container.querySelector('#conditionOperatorSelect');
        const valueInput = this.container.querySelector('#conditionValueInput');

        if (fieldInput) {
            fieldInput.addEventListener('input', (e) => {
                if (!this.config.condition) this.config.condition = {};
                this.config.condition.field = e.target.value;
                this.notifyChange();
            });
        }

        if (operatorSelect) {
            operatorSelect.addEventListener('change', (e) => {
                if (!this.config.condition) this.config.condition = {};
                this.config.condition.operator = e.target.value;
                this.notifyChange();
            });
        }

        if (valueInput) {
            valueInput.addEventListener('input', (e) => {
                if (!this.config.condition) this.config.condition = {};
                this.config.condition.value = e.target.value;
                this.notifyChange();
            });
        }

        // Loop type buttons
        this.container.querySelectorAll('.loop-type-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.config.loopType = btn.dataset.type;
                this.render();
                this.attachEvents();
                this.notifyChange();
            });
        });
    }

    /**
     * Notify parent of configuration change
     * @private
     */
    notifyChange() {
        const event = new CustomEvent('configChange', {
            detail: { config: this.config },
            bubbles: true
        });
        this.container.dispatchEvent(event);
    }

    /**
     * Validate configuration
     * @override
     */
    validate() {
        const errors = [];

        if (this.config.loopType === 'foreach' && !this.config.collection) {
            errors.push('Collection field path is required for For Each loop');
        }

        if (this.config.loopType === 'for' && (!this.config.iterations || this.config.iterations < 1)) {
            errors.push('Number of iterations must be at least 1');
        }

        if (this.config.loopType === 'while' && !this.config.condition?.field) {
            errors.push('Condition field is required for While loop');
        }

        if (!this.config.childStepIds || this.config.childStepIds.length === 0) {
            errors.push('At least one step is required in the loop body');
        }

        return {
            valid: errors.length === 0,
            errors
        };
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.ForEachLoopBuilder = ForEachLoopBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ForEachLoopBuilder;
}

console.log('[ForEachLoopBuilder] Loaded - Loop container step builder ready');

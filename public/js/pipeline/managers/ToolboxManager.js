/**
 * Toolbox Manager
 * Manages the left panel toolbox with templates and step library
 * Version: 8.8 - Removed context from Script Enrichment (use step chaining instead)
 */

class ToolboxManager {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.templates = [];
        this.searchInput = document.getElementById('toolboxSearch');

        this.init();
    }

    async init() {
        await this.loadTemplates();
        this.renderToolbox();
        this.setupSearch();
        this.setupSectionToggles();
    }

    /**
     * Load templates from API
     */
    async loadTemplates() {
        // Load built-in templates
        this.templates = this.getBuiltInTemplates();
        console.log(`✅ Loaded ${this.templates.length} built-in templates`);

        // Also load user templates from database
        try {
            // Check if API is available
            if (!this.builder.api || typeof this.builder.api.listTemplates !== 'function') {
                console.log('[Toolbox] User templates API not available - using built-in templates only');
                return;
            }

            const response = await this.builder.api.listTemplates();
            const dbTemplates = response.templates || response.data || [];

            // Filter out system templates (already in built-in)
            const userTemplates = dbTemplates.filter(t => !t.is_system);

            // Convert to StepTemplate objects
            userTemplates.forEach(t => {
                this.templates.push(new StepTemplate({
                    id: t.id,
                    name: t.template_name,
                    type: t.template_type,
                    description: t.description || '',
                    layer: t.layer,
                    icon: this.getIconForType(t.template_type),
                    isSystem: false,
                    isPublic: t.is_public,
                    usageCount: t.usage_count,
                    defaultConfig: t.default_config
                }));
            });

            console.log(`✅ Loaded ${userTemplates.length} user templates from database`);
        } catch (error) {
            console.warn('[Toolbox] Failed to load user templates:', error);
        }
    }

    /**
     * Get built-in templates as fallback
     */
    getBuiltInTemplates() {
        return [
            new StepTemplate({
                id: 'validate-fields',
                name: 'Field Validation',
                type: 'field_validation',
                description: 'Validate fields with support for required, format (email, phone, ssn, date, etc.), length, and pattern validation',
                layer: 'core',
                icon: this.getIconForType('field_validation'),
                isSystem: true,
                // Validation defaults: Use standard step controls
                required: false,  // Uncheck to accept messages with warnings (ACK)
                onErrorStrategy: 'continue',  // Continue = ACK with warnings, Fail = NACK
                defaultConfig: {
                    validations: [
                        {
                            field: 'PID.3',
                            validatorType: 'required',
                            errorMessage: 'Patient ID is required'
                        },
                        {
                            field: 'PID.5',
                            validatorType: 'required',
                            errorMessage: 'Patient name is required'
                        },
                        {
                            field: 'PID.7',
                            validatorType: 'format',
                            options: { format: 'date' },
                            errorMessage: 'Date of birth must be in YYYYMMDD format'
                        }
                    ],
                    addFieldNames: true  // Include field names in error messages
                }
            }),
            // REMOVED: "Add Metadata" step - metadata functionality merged into Field Mapping
            new StepTemplate({
                id: 'enrich-api',
                name: 'API Enrichment',
                type: 'enrichment.api',
                description: 'Enrich message data from external REST API (EMPI, EHR, LIMS)',
                layer: 'core',
                icon: this.getIconForType('enrichment.api'),
                isSystem: true,
                defaultConfig: {
                    endpoint: 'https://api.example.com/patients/{patientId}',
                    method: 'GET',
                    authType: 'none',
                    fieldMappings: { patientId: 'PID.3' },
                    targetPath: 'enriched.api',
                    timeoutMs: 5000,
                    retryCount: 0,
                    failOnError: false
                }
            }),
            new StepTemplate({
                id: 'enrich-database',
                name: 'Database Enrichment',
                type: 'enrichment.database',
                description: 'Query database for additional patient or order data',
                layer: 'core',
                icon: this.getIconForType('enrichment.database'),
                isSystem: true,
                defaultConfig: {
                    databaseType: 'postgresql',
                    connectionString: '', // Empty - will be built from individual fields
                    query: 'SELECT * FROM patients WHERE patient_id = $1',
                    queryParams: { patientId: 'PID.3' },
                    targetPath: 'enriched.database',
                    timeoutMs: 3000,
                    failOnError: false
                }
            }),
            // REMOVED: Cache Enrichment - Not implemented yet (cache_enrichment_executor.go returns placeholder)
            // Use "Database Enrichment" with Redis instead for cache lookups
            // Will be re-added when cache-aside pattern automation is fully implemented
            new StepTemplate({
                id: 'enrich-script',
                name: 'Script Enrichment',
                type: 'enrichment.script',
                description: 'Calculate custom fields using JavaScript (age, BMI, etc.)',
                layer: 'core',
                icon: this.getIconForType('enrichment.script'),
                isSystem: true,
                defaultConfig: {
                    script: '',  // Start with empty script - user writes their own code
                    targetPath: 'enriched.script',
                    timeoutMs: 5000,
                    failOnError: false
                }
            }),

            new StepTemplate({
                id: 'hl7-fhir-mapping',
                name: 'HL7→FHIR Transform',
                type: 'hl7_fhir_transform',
                description: 'Transform HL7 v2.x to FHIR R4',
                layer: 'core',
                icon: this.getIconForType('hl7_fhir_transform'),
                isSystem: true,
                defaultConfig: {
                    fhir_version: 'R4',
                    use_template: true
                }
            }),
            new StepTemplate({
                id: 'validate-fhir',
                name: 'FHIR Validation',
                type: 'fhir_validation',
                description: 'Validate FHIR bundle against R4 specification',
                layer: 'core',
                icon: this.getIconForType('fhir_validation'),
                isSystem: true,
                defaultConfig: {
                    validation_level: 'standard',
                    required_resources: [],
                    validate_references: true,
                    validate_required_fields: true
                }
            }),
            new StepTemplate({
                id: 'outbound-connector',
                name: 'Outbound Connector',
                type: 'connector.outbound',
                description: 'Deliver data to external systems (HTTP, TCP/MLLP, DB, MQ, Cloud, File)',
                layer: 'core',
                icon: this.getIconForType('post.delivery'),
                isSystem: true,
                defaultConfig: {
                    connectorType: 'http_outbound',
                    contentField: 'fhirBundle',
                    contentType: 'application/fhir+json',
                    config: {
                        url: 'http://fhir-server:8080/fhir',
                        method: 'POST',
                        headers: { 'Content-Type': 'application/fhir+json' }
                    }
                }
            }),

            // ============================================
            // DATA VALIDATION STEPS (Pre-Processing)
            // ============================================
            // NOTE: The following validation templates have been removed and consolidated
            // into the unified Field Validation step (core.validation):
            //
            // ❌ REMOVED:
            // - Data Type Validation (validate-data-types) - Use Field Validation with format/pattern
            // - Format Validation (validate-format) - Use Field Validation with format option (phone, ssn, email, etc.)
            // - Range Validation (validate-range) - Use Field Validation with custom regex or add RangeValidator if needed
            //
            // ✅ USE INSTEAD: Field Validation step which supports:
            //    - required: Field must exist and not be empty
            //    - format: Preset formats (email, phone, ssn, date, hl7_date, mrn, zip) + custom regex
            //    - length: Min/max/exact string length
            //    - pattern: Custom regex patterns
            //
            // TODO: If numeric range validation is needed, add RangeValidator to built_in_validators.go

            // ❌ REMOVED: Cross-Field Validation (cross-field-validation)
            // USER FEEDBACK: "Whenever you do comparison, there is an action right after that, so if/else should take care of it"
            // USER IS CORRECT: Cross-field validation is just conditional logic with a "reject" action
            // REASON: All comparisons lead to actions:
            //   - If discharge < admit → reject message (validation)
            //   - If age > 65 → route to geriatrics (routing)
            //   - If PID.3 != PV1.5 → log warning (data quality)
            // MIGRATION: Use Conditional Logic step (to be implemented) with these actions:
            //   - continue, reject, log_warning, log_error, set_metadata, set_field, route_to
            // REPLACEMENT: Conditional Logic step (core.conditional) - more flexible, covers all use cases
            // Example:
            //   {
            //     condition: { field1: 'PV1.45', operator: 'greater_than', field2: 'PV1.44' },
            //     onTrue: { action: 'continue' },
            //     onFalse: { action: 'reject', errorMessage: 'Discharge must be after admit' }
            //   }

            // ============================================
            // DATA TRANSFORMATION STEPS (Pre/Core)
            // ============================================
            new StepTemplate({
                id: 'field-mapping',
                name: 'Field Mapping',
                type: 'field_mapping',
                description: 'Map source fields to target fields with powerful transforms (trim, upper, lower, substring, replace, regex). Supports HL7 component paths (PID.5.1, PID.5.2) and chained transforms.',
                layer: 'core',
                icon: this.getIconForType('field_mapping'),
                isSystem: true,
                defaultConfig: {
                    mappings: [
                        { lhs: 'patient.name.family', rhs: 'PID.5.1', transforms: 'trim, upper' },
                        { lhs: 'patient.name.given', rhs: 'PID.5.2', transforms: 'trim' },
                        { lhs: 'patient.birthDate', rhs: 'PID.7', transforms: 'substring:0:4' }
                    ]
                }
            }),

            // ❌ REMOVED: Split/Combine Fields (split-combine-fields)
            // REASON: Field Mapping handles splits via HL7 component paths (PID.5.1, PID.5.2), combines via Script Enrichment
            // MIGRATION FOR SPLIT: Use Field Mapping with component paths:
            //   { lhs: 'lastName', rhs: 'PID.5.1' }, { lhs: 'firstName', rhs: 'PID.5.2' }
            // MIGRATION FOR COMBINE: Use Script Enrichment:
            //   script: "function transform(input) { input.fullName = input.firstName + ' ' + input.lastName; return input; }"

            // ❌ REMOVED: Date/Time Format Conversion (date-time-conversion)
            // REASON: Simple conversions use Field Mapping transforms, complex ones use Script Enrichment
            // MIGRATION (SIMPLE): Use Field Mapping with substring/regex transforms:
            //   { lhs: 'birthDate', rhs: 'PID.7', transforms: 'substring:0:4,substring:4:6,substring:6:8' }
            // MIGRATION (COMPLEX): Use Script Enrichment with JavaScript Date objects for timezone conversions

            // ⚠️ TODO: Unit Conversion - Backend implementation required
            // ❌ REMOVED: Unit Conversion (unit-conversion)
            // REASON: Decision to remove - conversions can be handled via Script Enrichment for complex cases
            // MIGRATION: Use Script Enrichment with custom JavaScript for unit conversions as needed
            // Example: function transform(data) { data.weightKg = data.weightLb * 0.453592; return data; }

            // ❌ REMOVED: String Manipulation (string-manipulation)
            // REASON: 100% redundant - Field Mapping already supports all string operations via transforms
            // MIGRATION: Use Field Mapping with transforms: 'trim', 'upper', 'lower', 'substring:start:end', 'replace:old:new', 'regex:pattern'
            // Example: { lhs: 'lastName', rhs: 'PID.5.1', transforms: 'trim, upper, substring:0:50' }

            // ❌ REMOVED: Value Lookup Table (value-lookup)
            // REASON: 100% redundant - Switch/Case executor does this + much more
            // USER INSIGHT: "Value lookup is just static case statement assignments" - correct!
            // MIGRATION: Use Switch/Case with set_field actions
            // Example migration:
            //   OLD: { field: 'PID.8', table: { 'M': 'male', 'F': 'female' }, default: 'unknown' }
            //   NEW: { field: 'PID.8', cases: [
            //     { when: 'M', actions: [{ action: 'set_field', field: 'PID.8', value: 'male' }] },
            //     { when: 'F', actions: [{ action: 'set_field', field: 'PID.8', value: 'female' }] }
            //   ], default: [{ action: 'set_field', field: 'PID.8', value: 'unknown' }] }

            // ❌ REMOVED: Code System Mapping (code-system-mapping)
            // REASON: No backend executor - use Script Enrichment or Switch/Case for code mapping
            // MIGRATION: Use Script Enrichment with lookup tables, or Switch/Case for static mappings

            // ============================================
            // DATA ENRICHMENT STEPS (Pre-Processing)
            // ============================================
            // REMOVED: Duplicate enrichment steps - use specialized enrichment steps instead:
            //   - For age calculation: Use "Script Enrichment" step
            //   - For UUID generation: Use "Add Metadata" step
            //   - For API calls: Use "API Enrichment" step (enrich-api)
            //   - For database lookups: Use "Database Enrichment" step (enrich-database)

            // ============================================
            // CONDITIONAL LOGIC STEPS (All Layers)
            // ============================================
            new StepTemplate({
                id: 'if-then-else',
                name: 'If-Then-Else',
                type: 'if_then_else',
                description: 'Conditional execution based on rules',
                layer: 'core',
                icon: this.getIconForType('if_then_else'),
                isSystem: true,
                defaultConfig: {
                    // NEW FORMAT: conditions array with onTrue/onFalse actions
                    conditions: [
                        {
                            name: 'Condition 1',
                            condition: {
                                field: '',
                                operator: 'equals',
                                value: '',
                                compareToField: ''
                            },
                            onTrue: {
                                action: 'continue'
                            },
                            onFalse: {
                                action: 'continue'
                            }
                        }
                    ]
                }
            }),

            new StepTemplate({
                id: 'switch-case',
                name: 'Switch/Case',
                type: 'switch_case',
                description: 'Multiple condition branching',
                layer: 'core',
                icon: this.getIconForType('switch_case'),
                isSystem: true,
                defaultConfig: {
                    field: '',  // Field to switch on (matches SwitchCaseBuilder)
                    cases: [
                        { value: '', label: 'Case 1', actions: [{ action: 'continue' }] }
                    ],
                    default: { actions: [{ action: 'continue' }] },
                    options: { caseInsensitive: false, trimWhitespace: true }
                }
            }),

            new StepTemplate({
                id: 'loop-container',
                name: 'Loop',
                type: 'control.loop',
                description: 'Container step - executes nested steps in a loop (For Each, For, While)',
                layer: 'core',
                icon: 'fas fa-redo-alt',
                isSystem: true,
                isContainer: true,  // Mark as container step
                defaultConfig: {
                    loopType: 'foreach',           // 'foreach', 'for', 'while'
                    collection: '',                 // For 'foreach': array field path
                    itemVariable: 'item',           // Variable name for current item
                    indexVariable: 'index',         // Variable name for current index
                    iterations: 10,                 // For 'for': number of iterations
                    condition: {                    // For 'while': condition object
                        field: '',
                        operator: 'not_empty',
                        value: ''
                    },
                    childStepIds: [],               // IDs of steps in loop body
                    maxIterations: 1000,            // Safety limit
                    breakOnError: false,            // Stop loop on first error
                    continueOnEmpty: true           // Continue pipeline if collection empty
                }
            }),

            // ============================================
            // HL7/FHIR SPECIFIC STEPS (Core)
            // ============================================
            // REMOVED: HL7 Segment Extractor - segments already available in parsed data (enhancedSegments, segmentGroups)
            // REMOVED: FHIR Resource Builder - use HL7-FHIR Transform (core.mapping) or Field Mapping instead

            // ============================================
            // ERROR HANDLING STEPS (All Layers)
            // ============================================
            new StepTemplate({
                id: 'try-catch',
                name: 'Try-Catch Block',
                type: 'control.try_catch',
                description: 'Wrap steps in error handling with try/catch/finally blocks',
                layer: 'core',
                icon: this.getIconForType('post.fhir.validation'),
                isSystem: true,
                defaultConfig: {
                    trySteps: [],
                    catchSteps: [],
                    finallySteps: [],
                    onError: 'catch'  // "catch", "suppress", "rethrow"
                }
            }),

            new StepTemplate({
                id: 'retry-logic',
                name: 'Retry Logic',
                type: 'control.retry',
                description: 'Retry failed operations with configurable backoff',
                layer: 'core',
                icon: this.getIconForType('post.error_handling'),
                isSystem: true,
                isContainer: true,
                defaultConfig: {
                    childSteps: [],
                    maxRetries: 3,
                    delayMs: 1000,
                    backoffType: 'exponential',  // "fixed", "exponential", "linear"
                    maxDelayMs: 30000,
                    retryOnErrors: []  // empty = retry on any error
                }
            }),

            // ============================================
            // DATA QUALITY STEPS (Post-Processing)
            // ============================================
            new StepTemplate({
                id: 'remove-duplicates',
                name: 'Remove Duplicates',
                type: 'remove_duplicates',
                description: 'Remove duplicate entries from arrays/collections',
                layer: 'core',
                icon: this.getIconForType('post.quality'),
                isSystem: true,
                defaultConfig: {
                    sourceField: 'bundle.entry',
                    keyFields: ['resource.id'],
                    strategy: 'first',  // "first", "last", "merge"
                    caseSensitive: true
                }
            }),

            new StepTemplate({
                id: 'data-masking',
                name: 'Data Masking/Anonymization',
                type: 'data_masking',
                description: 'Mask or anonymize PHI/PII data for HIPAA compliance',
                layer: 'core',
                icon: this.getIconForType('post.quality'),
                isSystem: true,
                defaultConfig: {
                    rules: [
                        { field: 'PID.5', strategy: 'partial', keepFirst: 1, keepLast: 0 },
                        { field: 'PID.19', strategy: 'hash' },
                        { field: 'PID.13', strategy: 'redact' }
                    ],
                    maskAllPHI: false  // Auto-mask common PHI fields (PID.3, PID.5, PID.7, PID.13, PID.19, PID.18)
                }
            }),

            // ============================================
            // DATA TRANSFORMATION STEPS (Post-Processing)
            // ============================================
            new StepTemplate({
                id: 'normalizer',
                name: 'Normalizer / Pivot / Transpose',
                type: 'normalizer',
                description: 'Normalize, pivot, transpose, flatten or unflatten data structures',
                layer: 'core',
                icon: 'fas fa-exchange-alt',
                isSystem: true,
                defaultConfig: {
                    operation: 'normalize',  // "normalize", "pivot", "transpose", "flatten", "unflatten"
                    sourceField: '',
                    outputField: '',
                    // For normalize (unpivot):
                    keyColumn: 'attribute',
                    valueColumn: 'value',
                    // For pivot:
                    pivotField: '',
                    valueField: '',
                    aggregation: 'first',  // "first", "last", "sum", "count", "list"
                    // For flatten/unflatten:
                    delimiter: '.',
                    maxDepth: 10,
                    // Optional transforms:
                    renameMap: {},
                    caseTransform: ''  // "", "lower", "upper", "camel", "snake"
                }
            }),

            // ============================================
            // CONNECTIVITY STEPS (All Layers)
            // ============================================
            new StepTemplate({
                id: 'inbound-connector',
                name: 'Inbound Connector',
                type: 'connector.inbound',
                description: 'Fetch data from external systems mid-pipeline (DB, API, MQ, Cloud)',
                layer: 'core',
                icon: 'fas fa-download',
                isSystem: true,
                defaultConfig: {
                    connectorType: '',  // e.g., "postgresql_inbound", "mongodb_inbound", "http_rest_inbound"
                    config: {},         // Connector-specific configuration
                    outputField: 'enriched.connector_result',
                    timeoutMs: 30000
                }
            })
        ];
    }

    /**
     * Render toolbox sections
     */
    renderToolbox() {
        this.renderTemplateSection();
        this.renderAllSteps();
        this.renderCustomScripts();
    }

    /**
     * Render template library
     */
    renderTemplateSection() {
        const container = document.getElementById('templates-list');
        if (!container) return;

        container.innerHTML = '';

        // Filter popular/recommended templates
        const popularTemplates = this.templates.filter(t => t.isSystem).slice(0, 5);

        popularTemplates.forEach(template => {
            const card = this.createTemplateCard(template);
            container.appendChild(card);
        });
    }

    /**
     * Render all steps in single list
     */
    renderAllSteps() {
        const container = document.getElementById('all-steps-list');
        if (!container) return;

        container.innerHTML = '';

        this.templates.forEach(template => {
            const card = this.createTemplateCard(template);
            container.appendChild(card);
        });

        if (this.templates.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #94a3b8; font-size: 0.75rem;">No templates available</p>';
        }
    }

    /**
     * Render custom scripts section
     */
    renderCustomScripts() {
        const container = document.getElementById('custom-steps-list');
        if (!container) return;

        container.innerHTML = `
            <div class="step-card" id="addCustomScript" style="cursor: pointer; text-align: center; padding: 1.5rem;">
                <i class="fas fa-plus-circle" style="font-size: 2rem; color: #2563eb; margin-bottom: 0.5rem;"></i>
                <div style="font-size: 0.875rem; font-weight: 500;">Add Custom Script</div>
            </div>
        `;

        const addBtn = container.querySelector('#addCustomScript');
        if (addBtn) {
            addBtn.addEventListener('click', () => this.createCustomScript());
        }
    }

    /**
     * Create template card
     */
    createTemplateCard(template) {
        const card = document.createElement('div');
        card.className = 'step-card';

        card.innerHTML = `
            <div class="step-card-header">
                <div class="step-card-icon">
                    <i class="${template.icon}"></i>
                </div>
                <div class="step-card-title">${template.name}</div>
            </div>
            <div class="step-card-description">${template.description}</div>
            <div class="step-card-hint" style="font-size: 0.75rem; color: #6b7280; margin-top: 0.5rem; font-style: italic;">
                Double-click to preview
            </div>
            ${template.isSystem ? '<span class="step-card-badge">Built-in</span>' : ''}
        `;

        // Simple double-click to preview (avoids drag conflict)
        card.addEventListener('dblclick', (e) => {
            e.preventDefault();
            e.stopPropagation();

            console.log('👁️ === DOUBLE-CLICK EVENT FIRED ===');
            console.log('Template name:', template.name);
            console.log('Template type:', template.type);
            console.log('PropertiesPanel exists:', !!this.builder.propertiesPanel);

            try {
                // Create a temporary step instance for preview
                const previewStep = template.createStep();
                console.log('Preview step created:', previewStep);

                previewStep._isPreview = true;

                // Show properties modal in preview mode
                console.log('Calling showStepProperties with isPreview=true');
                this.builder.propertiesPanel.showStepProperties(previewStep, true);
                console.log('✅ Properties modal should be visible now');
            } catch (error) {
                console.error('❌ Error opening step properties:', error);
                console.error('Stack:', error.stack);
            }
        });

        // Make draggable
        this.builder.dragDropManager.makeDraggable(card, {
            type: 'template',
            template: template
        });

        return card;
    }

    /**
     * Create custom script step
     */
    createCustomScript() {
        const customStep = new VisualStep({
            stepName: 'Custom Script',
            stepType: 'custom',
            scriptType: 'javascript',
            scriptContent: `function transform(input) {
    // Access parsed data
    // var segments = input.enhancedSegments;

    // Your custom logic here

    return input;
}`,
            icon: this.getIconForType('pre.enrichment.script'),
            description: 'Custom JavaScript transformation'
        });

        this.builder.addStep(customStep);
        this.builder.dragDropManager.showNotification('Custom script added', 'success');
    }

    /**
     * Setup search functionality
     */
    setupSearch() {
        if (!this.searchInput) return;

        this.searchInput.addEventListener('input', (e) => {
            const query = e.target.value.toLowerCase().trim();
            this.filterTemplates(query);
        });
    }

    /**
     * Filter templates by search query
     */
    filterTemplates(query) {
        const allCards = document.querySelectorAll('.step-card');

        allCards.forEach(card => {
            const title = card.querySelector('.step-card-title')?.textContent.toLowerCase() || '';
            const description = card.querySelector('.step-card-description')?.textContent.toLowerCase() || '';

            if (query === '' || title.includes(query) || description.includes(query)) {
                card.style.display = '';
            } else {
                card.style.display = 'none';
            }
        });
    }

    /**
     * Setup section toggle buttons
     */
    setupSectionToggles() {
        const toggleButtons = document.querySelectorAll('.section-toggle');

        toggleButtons.forEach(button => {
            button.addEventListener('click', (e) => {
                e.stopPropagation();
                const targetId = button.dataset.target;
                const target = document.getElementById(targetId);

                if (target) {
                    const isCollapsed = target.classList.contains('collapsed');

                    if (isCollapsed) {
                        target.classList.remove('collapsed');
                        button.classList.remove('collapsed');
                    } else {
                        target.classList.add('collapsed');
                        button.classList.add('collapsed');
                    }
                }
            });
        });

        // Also toggle on section title click
        const sectionTitles = document.querySelectorAll('.section-title');
        sectionTitles.forEach(title => {
            title.addEventListener('click', () => {
                const button = title.querySelector('.section-toggle');
                if (button) {
                    button.click();
                }
            });
        });
    }

    /**
     * Get icon for step type (automatic mapping)
     * @param {string} stepType - Step type (e.g., 'pre.validation', 'pre.enrichment.api')
     * @returns {string} Font Awesome icon class
     */
    getIconForType(stepType) {
        // Comprehensive icon mapping based on step type
        const iconMap = {
            // ============================================
            // NEW TYPE NAMES (primary)
            // ============================================
            'field_validation': 'fas fa-check-circle',
            'fhir_validation': 'fas fa-shield-alt',
            'enrichment.api': 'fas fa-cloud',
            'enrichment.database': 'fas fa-database',
            'enrichment.script': 'fas fa-code',
            'field_mapping': 'fas fa-arrows-alt-h',
            'hl7_fhir_transform': 'fas fa-exchange-alt',
            'if_then_else': 'fas fa-sitemap',
            'switch_case': 'fas fa-project-diagram',
            'data_masking': 'fas fa-user-secret',
            'remove_duplicates': 'fas fa-check-double',
            'normalizer': 'fas fa-exchange-alt',

            // ============================================
            // LEGACY TYPE NAMES (backward compat)
            // ============================================
            'pre.validation': 'fas fa-check-circle',
            'pre.validation.field': 'fas fa-check-square',
            'pre.validation.schema': 'fas fa-clipboard-check',
            'pre.validation.cross-field': 'fas fa-code-branch',
            'pre.enrichment': 'fas fa-plus-circle',
            'pre.enrichment.api': 'fas fa-cloud',
            'pre.enrichment.database': 'fas fa-database',
            'pre.enrichment.cache': 'fas fa-bolt',
            'pre.enrichment.script': 'fas fa-code',
            'pre.enrichment.metadata': 'fas fa-tags',
            'pre.extraction': 'fas fa-filter',
            'core.transformation': 'fas fa-arrows-alt-h',
            'core.mapping': 'fas fa-project-diagram',
            'core.mapping.hl7-fhir': 'fas fa-exchange-alt',
            'core.mapping.custom': 'fas fa-wrench',
            'post.validation': 'fas fa-shield-alt',
            'post.fhir.validation': 'fas fa-shield-alt',
            'post.anonymization': 'fas fa-user-secret',
            'post.audit': 'fas fa-clipboard-list',
            'post.delivery': 'fas fa-paper-plane',
            'post.error_handling': 'fas fa-exclamation-triangle',
            'post.quality': 'fas fa-check-double',
            'pre.logic': 'fas fa-sitemap',
            'pre.logic.switch': 'fas fa-project-diagram',
            'core.logic': 'fas fa-sitemap',
            'post.logic': 'fas fa-sitemap',

            // ============================================
            // CONTROL FLOW / CONTAINERS
            // ============================================
            'control.loop': 'fas fa-redo-alt',             // Loop container
            'control.foreach': 'fas fa-list',              // For Each loop
            'control.for': 'fas fa-sort-numeric-down',     // For loop
            'control.while': 'fas fa-sync',                // While loop
            'control.parallel': 'fas fa-columns',          // Parallel execution

            // ============================================
            // ERROR HANDLING
            // ============================================
            'pre.error': 'fas fa-exclamation-triangle',
            'core.error': 'fas fa-exclamation-triangle',
            'post.error': 'fas fa-exclamation-triangle',

            // ============================================
            // CUSTOM/SCRIPT STEPS
            // ============================================
            'custom': 'fas fa-cog',
            'custom.script': 'fas fa-file-code',
            'custom.javascript': 'fas fa-js',

            // ============================================
            // SPECIALIZED ENRICHMENT
            // ============================================
            'pre.enrichment.empi': 'fas fa-id-card',
            'pre.enrichment.terminology': 'fas fa-book-medical',
            'pre.enrichment.provider': 'fas fa-user-md',
            'pre.enrichment.location': 'fas fa-map-marker-alt',

            // ============================================
            // DEFAULT
            // ============================================
            'default': 'fas fa-cog'
        };

        // Try exact match first
        if (iconMap[stepType]) {
            return iconMap[stepType];
        }

        // Try partial match (e.g., 'pre.validation.custom' → 'pre.validation')
        for (const [type, icon] of Object.entries(iconMap)) {
            if (stepType.startsWith(type + '.')) {
                return icon;
            }
        }

        // Try category match (e.g., 'pre.enrichment.xyz' → 'pre.enrichment')
        const parts = stepType.split('.');
        if (parts.length >= 2) {
            const category = parts.slice(0, 2).join('.');
            if (iconMap[category]) {
                return iconMap[category];
            }
        }

        // Default fallback
        return iconMap['default'];
    }

    /**
     * Refresh toolbox
     */
    async refresh() {
        await this.loadTemplates();
        this.renderToolbox();
    }
}

// Export
if (typeof window !== 'undefined') {
    window.ToolboxManager = ToolboxManager;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ToolboxManager;
}

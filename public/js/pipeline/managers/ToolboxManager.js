/**
 * Toolbox Manager
 * Manages the left panel toolbox with templates and step library
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
        try {
            this.templates = await window.pipelineAPI.listTemplates();
        } catch (error) {
            console.error('Failed to load templates:', error);
            // Use fallback built-in templates
            this.templates = this.getBuiltInTemplates();
        }
    }

    /**
     * Get built-in templates as fallback
     */
    getBuiltInTemplates() {
        return [
            new StepTemplate({
                id: 'validate-required-fields',
                name: 'Validate Required Fields',
                type: 'pre.validation',
                description: 'Validates required HL7 fields',
                layer: 'pre',
                icon: 'fas fa-check-circle',
                isSystem: true,
                defaultConfig: {
                    rules: [
                        { field: 'MSH.9', required: true },
                        { field: 'PID.3', required: true }
                    ]
                }
            }),
            new StepTemplate({
                id: 'enrich-patient-data',
                name: 'Enrich Patient Data',
                type: 'pre.enrichment',
                description: 'Enrich from external system',
                layer: 'pre',
                icon: 'fas fa-plus-circle',
                isSystem: true,
                defaultConfig: {
                    source: 'epic',
                    fields: ['demographics', 'insurance']
                }
            }),
            new StepTemplate({
                id: 'hl7-fhir-mapping',
                name: 'HL7 to FHIR Mapping',
                type: 'core.mapping',
                description: 'Transform HL7 to FHIR R4',
                layer: 'core',
                icon: 'fas fa-exchange-alt',
                isSystem: true,
                defaultConfig: {
                    fhir_version: 'R4',
                    use_template: true
                }
            }),
            new StepTemplate({
                id: 'validate-fhir',
                name: 'Validate FHIR Bundle',
                type: 'post.validation',
                description: 'Validate FHIR bundle against R4 specification',
                layer: 'post',
                icon: 'fas fa-shield-alt',
                isSystem: true,
                defaultConfig: {
                    fhir_version: 'R4',
                    validation_level: 'STANDARD', // MINIMAL, STANDARD, STRICT
                    schema_path: 'schemas/fhir/R4',

                    // Bundle-level validation
                    bundle_rules: {
                        require_bundle_type: true,
                        allowed_bundle_types: ['transaction', 'collection', 'document', 'message'],
                        require_timestamp: true,
                        require_total: false
                    },

                    // Resource-level validation
                    resource_rules: {
                        validate_structure: true,      // Structural validation (required fields, data types)
                        validate_cardinality: true,    // Check min/max occurrences (0..1, 0..*, 1..1)
                        validate_data_types: true,     // Data type compliance (string, date, boolean, etc.)
                        validate_references: true,     // Reference integrity checks
                        validate_terminology: false,   // CodeSystem/ValueSet bindings (strict mode only)
                        validate_profiles: false,      // US Core / other profile validation
                        validate_constraints: true     // FHIRPath constraints
                    },

                    // Required resources validation
                    required_resources: {
                        MessageHeader: { min: 0, max: 1 },
                        Patient: { min: 1, max: 1 },      // At least one Patient required
                        Encounter: { min: 0, max: 999 }
                    },

                    // Patient-specific validation rules (from schema)
                    patient_validation: {
                        require_identifier: true,          // Patient.identifier (0..*)
                        require_name: true,               // Patient.name (0..*)
                        validate_gender: true,            // Patient.gender must be from ValueSet
                        validate_birthdate_format: true,  // Must be valid FHIR date (YYYY-MM-DD)
                        validate_address_structure: true, // Address.line, city, state, postalCode
                        validate_telecom_system: true     // telecom.system must be phone|fax|email|pager|url|sms|other
                    },

                    // Encounter-specific validation rules
                    encounter_validation: {
                        require_status: true,             // Encounter.status (required)
                        require_class: true,              // Encounter.class (required)
                        validate_status_values: true,     // Status from: planned|arrived|triaged|in-progress|onleave|finished|cancelled
                        validate_period: true,            // period.start < period.end
                        validate_subject_reference: true  // subject must reference Patient
                    },

                    // MessageHeader-specific validation
                    messageheader_validation: {
                        require_event_coding: true,       // eventCoding (required)
                        require_source: true,             // source (required)
                        validate_source_structure: true,  // source.name + source.endpoint
                        validate_destination: false       // destination (optional)
                    },

                    // Error handling
                    fail_on_error: false,    // Stop processing on validation error
                    fail_on_warning: false,  // Stop processing on warning
                    log_all_issues: true,    // Log all validation issues

                    // Output
                    include_validation_report: true,
                    attach_issues_to_bundle: false
                }
            }),
            new StepTemplate({
                id: 'deliver-fhir',
                name: 'Deliver to FHIR Server',
                type: 'post.delivery',
                description: 'Send FHIR bundle to destination',
                layer: 'post',
                icon: 'fas fa-paper-plane',
                isSystem: true,
                defaultConfig: {
                    endpoint: 'http://fhir-server:8080/fhir',
                    resource: 'Patient'
                }
            })
        ];
    }

    /**
     * Render toolbox sections
     */
    renderToolbox() {
        this.renderTemplateSection();
        this.renderLayerSections();
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
     * Render layer-specific sections
     */
    renderLayerSections() {
        // Pre-processing
        this.renderLayerSection('pre', 'pre-steps-list');

        // Core transformation
        this.renderLayerSection('core', 'core-steps-list');

        // Post-processing
        this.renderLayerSection('post', 'post-steps-list');
    }

    /**
     * Render steps for specific layer
     */
    renderLayerSection(layer, containerId) {
        const container = document.getElementById(containerId);
        if (!container) return;

        container.innerHTML = '';

        const layerTemplates = this.templates.filter(t => t.layer === layer);

        layerTemplates.forEach(template => {
            const card = this.createTemplateCard(template);
            container.appendChild(card);
        });

        // Add empty state
        if (layerTemplates.length === 0) {
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
            ${template.isSystem ? '<span class="step-card-badge">Built-in</span>' : ''}
        `;

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
            icon: 'fas fa-code',
            description: 'Custom JavaScript transformation'
        });

        // Add to core layer by default
        this.builder.addStepToLayer(customStep, 'core');
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

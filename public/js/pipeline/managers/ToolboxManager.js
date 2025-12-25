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
                name: 'field_validation',
                type: 'pre.validation',
                description: 'Validate fields (required, format, length, pattern)',
                layer: 'pre',
                icon: this.getIconForType('pre.validation'),
                isSystem: true,
                // Validation defaults: Use standard step controls
                required: false,  // Uncheck to accept messages with warnings (ACK)
                onErrorStrategy: 'continue',  // Continue = ACK with warnings, Fail = NACK
                defaultConfig: {
                    rules: [
                        { field: 'enhancedSegments.MSH.fields[1].value', type: 'required', errorMessage: 'Message type is required' },
                        { field: 'enhancedSegments.PID.fields[0].value', type: 'required', errorMessage: 'Patient ID is required' },
                        { field: 'enhancedSegments.PID.fields[2].value', type: 'required', errorMessage: 'Date of birth is required' }
                    ]
                }
            }),
            new StepTemplate({
                id: 'enrich-patient-data',
                name: 'Data Enrichment',
                type: 'pre.enrichment',
                description: 'Enrich from external system',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment'),
                isSystem: true,
                defaultConfig: {
                    source: 'epic',
                    fields: ['demographics', 'insurance']
                }
            }),
            new StepTemplate({
                id: 'add-metadata',
                name: 'add_metadata',
                type: 'pre.enrichment.metadata',
                description: 'Add processing metadata (timestamps, IDs, custom fields)',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment.metadata'),
                isSystem: true,
                defaultConfig: {
                    addTimestamp: true,
                    addCorrelationId: true,
                    addInterfaceId: false,
                    addMessageId: false,
                    customMetadata: {}
                }
            }),
            new StepTemplate({
                id: 'enrich-api',
                name: 'api_enrichment',
                type: 'pre.enrichment.api',
                description: 'Enrich message data from external REST API (EMPI, EHR, LIMS)',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment.api'),
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
                name: 'database_enrichment',
                type: 'pre.enrichment.database',
                description: 'Query database for additional patient or order data',
                layer: ['pre', 'core'],  // Allow in both pre-processing and core layers
                icon: this.getIconForType('pre.enrichment.database'),
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
            new StepTemplate({
                id: 'enrich-cache',
                name: 'Cache Enrichment',
                type: 'pre.enrichment.cache',
                description: 'Lookup enrichment data from Redis or Memcached',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment.cache'),
                isSystem: true,
                defaultConfig: {
                    cacheType: 'redis',
                    connectionString: 'redis://localhost:6379',
                    keyTemplate: 'patient:{patientId}',
                    keyMappings: { patientId: 'PID.3' },
                    targetPath: 'enriched.cache',
                    timeoutMs: 1000,
                    writeBack: false,
                    ttlSeconds: 3600,
                    failOnError: false
                }
            }),
            new StepTemplate({
                id: 'enrich-script',
                name: 'Script Enrichment',
                type: 'pre.enrichment.script',
                description: 'Calculate custom fields using JavaScript (age, BMI, etc.)',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment.script'),
                isSystem: true,
                defaultConfig: {
                    script: `// Extract patient date of birth\nvar dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value");\n\n// Calculate age\nvar age = calculateAge(dob);\n\n// Return enrichment data\nreturn {\n    age: age,\n    ageGroup: age < 18 ? "pediatric" : "adult"\n};`,
                    context: {},
                    targetPath: 'enriched.script',
                    timeoutMs: 5000,
                    failOnError: false
                }
            }),

            new StepTemplate({
                id: 'hl7-fhir-mapping',
                name: 'HL7→FHIR Transform',
                type: 'core.mapping',
                description: 'Transform HL7 v2.x to FHIR R4',
                layer: 'core',
                icon: this.getIconForType('core.mapping.hl7-fhir'),
                isSystem: true,
                defaultConfig: {
                    fhir_version: 'R4',
                    use_template: true
                }
            }),
            new StepTemplate({
                id: 'validate-fhir',
                name: 'FHIR Validation',
                type: 'post.validation',
                description: 'Validate FHIR bundle against R4 specification',
                layer: 'post',
                icon: this.getIconForType('post.fhir.validation'),
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
                name: 'FHIR Server Delivery',
                type: 'post.delivery',
                description: 'Send FHIR bundle to destination',
                layer: 'post',
                icon: this.getIconForType('post.delivery'),
                isSystem: true,
                defaultConfig: {
                    endpoint: 'http://fhir-server:8080/fhir',
                    resource: 'Patient'
                }
            }),

            // ============================================
            // DATA VALIDATION STEPS (Pre-Processing)
            // ============================================
            new StepTemplate({
                id: 'validate-data-types',
                name: 'Data Type Validation',
                type: 'pre.validation',
                description: 'Validate field data types (string, number, date)',
                layer: 'pre',
                icon: this.getIconForType('pre.validation'),
                isSystem: true,
                defaultConfig: {
                    rules: [
                        { field: 'PID.7', type: 'date', format: 'YYYYMMDD' },
                        { field: 'PID.3', type: 'string', pattern: '^[0-9]+$' }
                    ],
                    on_error: 'fail' // fail, warn, skip
                }
            }),

            new StepTemplate({
                id: 'validate-format',
                name: 'Format Validation',
                type: 'pre.validation',
                description: 'Validate specific formats (Phone, SSN, Email, etc.)',
                layer: 'pre',
                icon: this.getIconForType('pre.validation'),
                isSystem: true,
                defaultConfig: {
                    validations: [
                        { field: 'PID.13', format: 'phone' },
                        { field: 'PID.19', format: 'ssn' },
                        { field: 'NK1.5', format: 'phone' }
                    ],
                    formats: {
                        phone: '^\\(?([0-9]{3})\\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})$',
                        ssn: '^[0-9]{3}-[0-9]{2}-[0-9]{4}$',
                        email: '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$'
                    }
                }
            }),

            new StepTemplate({
                id: 'validate-range',
                name: 'Range Validation',
                type: 'pre.validation',
                description: 'Validate numeric ranges (min/max values)',
                layer: 'pre',
                icon: this.getIconForType('pre.validation'),
                isSystem: true,
                defaultConfig: {
                    rules: [
                        { field: 'OBX.5', min: 0, max: 300, unit: 'mg/dL', description: 'Blood Glucose' },
                        { field: 'age', min: 0, max: 120, description: 'Patient Age' }
                    ]
                }
            }),

            new StepTemplate({
                id: 'cross-field-validation',
                name: 'Cross-Field Validation',
                type: 'pre.validation',
                description: 'Validate relationships between fields',
                layer: 'pre',
                icon: this.getIconForType('pre.validation.cross-field'),
                isSystem: true,
                defaultConfig: {
                    rules: [
                        {
                            name: 'Discharge after Admit',
                            field1: 'PV1.44', // Discharge date
                            operator: 'greater_than',
                            field2: 'PV1.44', // Admit date
                            message: 'Discharge date must be after admit date'
                        }
                    ]
                }
            }),

            // ============================================
            // DATA TRANSFORMATION STEPS (Pre/Core)
            // ============================================
            new StepTemplate({
                id: 'field-mapping',
                name: 'Field Mapping',
                type: 'core.transformation',
                description: 'Map source fields to target fields',
                layer: 'core',
                icon: this.getIconForType('core.transformation'),
                isSystem: true,
                defaultConfig: {
                    mappings: [
                        { source: 'PID.5[0].1', target: 'patient.name.family' },
                        { source: 'PID.5[0].2', target: 'patient.name.given[0]' },
                        { source: 'PID.7', target: 'patient.birthDate', transform: 'formatDate' }
                    ]
                }
            }),

            new StepTemplate({
                id: 'split-combine-fields',
                name: 'Split/Combine Fields',
                type: 'pre.transformation',
                description: 'Split or combine field values',
                layer: 'pre',
                icon: this.getIconForType('pre.transformation'),
                isSystem: true,
                defaultConfig: {
                    operations: [
                        {
                            type: 'split',
                            source: 'PID.5', // "Smith^John^M"
                            delimiter: '^',
                            targets: ['lastName', 'firstName', 'middleName']
                        },
                        {
                            type: 'combine',
                            sources: ['firstName', 'lastName'],
                            delimiter: ' ',
                            target: 'fullName'
                        }
                    ]
                }
            }),

            new StepTemplate({
                id: 'date-time-conversion',
                name: 'Date/Time Format Conversion',
                type: 'pre.transformation',
                description: 'Convert date/time formats',
                layer: 'pre',
                icon: this.getIconForType('pre.transformation'),
                isSystem: true,
                defaultConfig: {
                    conversions: [
                        {
                            field: 'PID.7',
                            from_format: 'YYYYMMDD',
                            to_format: 'YYYY-MM-DD',
                            timezone: 'UTC'
                        }
                    ]
                }
            }),

            new StepTemplate({
                id: 'unit-conversion',
                name: 'Unit Conversion',
                type: 'pre.transformation',
                description: 'Convert units (lb→kg, F→C, in→cm)',
                layer: 'pre',
                icon: this.getIconForType('pre.transformation'),
                isSystem: true,
                defaultConfig: {
                    conversions: [
                        { field: 'OBX.5', from: 'lb', to: 'kg', factor: 0.453592 },
                        { field: 'temp', from: 'F', to: 'C', formula: '(x - 32) * 5/9' }
                    ]
                }
            }),

            new StepTemplate({
                id: 'string-manipulation',
                name: 'String Manipulation',
                type: 'pre.transformation',
                description: 'Uppercase, lowercase, trim, substring operations',
                layer: 'pre',
                icon: this.getIconForType('pre.transformation'),
                isSystem: true,
                defaultConfig: {
                    operations: [
                        { field: 'PID.5[0].1', operation: 'uppercase' },
                        { field: 'PID.11', operation: 'trim' },
                        { field: 'comments', operation: 'substring', start: 0, length: 100 }
                    ]
                }
            }),

            new StepTemplate({
                id: 'value-lookup',
                name: 'Value Lookup Table',
                type: 'pre.transformation',
                description: 'Map values using lookup tables (M→Male, F→Female)',
                layer: 'pre',
                icon: this.getIconForType('pre.transformation'),
                isSystem: true,
                defaultConfig: {
                    lookups: [
                        {
                            field: 'PID.8',
                            table: {
                                'M': 'male',
                                'F': 'female',
                                'O': 'other',
                                'U': 'unknown'
                            },
                            default: 'unknown'
                        }
                    ]
                }
            }),

            new StepTemplate({
                id: 'code-system-mapping',
                name: 'Code System Mapping',
                type: 'core.transformation',
                description: 'Map between code systems (ICD-9→ICD-10, LOINC)',
                layer: 'core',
                icon: this.getIconForType('pre.enrichment.script'),
                isSystem: true,
                defaultConfig: {
                    mappings: [
                        {
                            source_system: 'ICD-9',
                            target_system: 'ICD-10',
                            field: 'DG1.3',
                            use_api: false,
                            fallback: 'unmapped'
                        }
                    ]
                }
            }),

            // ============================================
            // DATA ENRICHMENT STEPS (Pre-Processing)
            // ============================================
            new StepTemplate({
                id: 'calculate-age',
                name: 'Calculate Age from DOB',
                type: 'pre.enrichment',
                description: 'Calculate age in years from date of birth',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment'),
                isSystem: true,
                defaultConfig: {
                    dob_field: 'PID.7',
                    output_field: 'patient_age',
                    as_of_date: 'today' // or specific date
                }
            }),

            new StepTemplate({
                id: 'generate-uuid',
                name: 'Generate UUID/IDs',
                type: 'pre.enrichment',
                description: 'Generate unique identifiers',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment'),
                isSystem: true,
                defaultConfig: {
                    fields: [
                        { target: 'bundle_id', type: 'uuid' },
                        { target: 'message_id', type: 'timestamp' }
                    ]
                }
            }),

            new StepTemplate({
                id: 'api-call',
                name: 'External API Call',
                type: 'pre.enrichment',
                description: 'Call external REST API for data enrichment',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment'),
                isSystem: true,
                defaultConfig: {
                    url: 'https://api.example.com/patient/{{patientId}}',
                    method: 'GET',
                    headers: {
                        'Authorization': 'Bearer {{token}}',
                        'Content-Type': 'application/json'
                    },
                    timeout_ms: 5000,
                    retry_count: 3,
                    store_response_in: 'api_response',
                    on_error: 'continue'
                }
            }),

            new StepTemplate({
                id: 'database-lookup',
                name: 'Database Lookup',
                type: 'pre.enrichment',
                description: 'Lookup data from database',
                layer: 'pre',
                icon: this.getIconForType('pre.enrichment.database'),
                isSystem: true,
                defaultConfig: {
                    query: 'SELECT * FROM providers WHERE npi = $1',
                    parameters: ['{{parsed.PV1.7.1}}'],
                    store_result_in: 'provider_data',
                    cache_ttl_seconds: 300
                }
            }),

            // ============================================
            // CONDITIONAL LOGIC STEPS (All Layers)
            // ============================================
            new StepTemplate({
                id: 'if-then-else',
                name: 'If-Then-Else',
                type: 'pre.logic',
                description: 'Conditional execution based on rules',
                layer: 'pre',
                icon: this.getIconForType('pre.logic'),
                isSystem: true,
                defaultConfig: {
                    condition: {
                        field: 'parsed.PID.7.value',
                        operator: 'age_greater_than',
                        value: 65
                    },
                    then_actions: [
                        { action: 'set_field', field: 'priority', value: 'high' },
                        { action: 'add_tag', value: 'geriatric' }
                    ],
                    else_actions: [
                        { action: 'set_field', field: 'priority', value: 'normal' }
                    ]
                }
            }),

            new StepTemplate({
                id: 'switch-case',
                name: 'Switch/Case',
                type: 'pre.logic',
                description: 'Multiple condition branching',
                layer: 'pre',
                icon: this.getIconForType('pre.logic'),
                isSystem: true,
                defaultConfig: {
                    switch_field: 'parsed.PV1.2.value',
                    cases: [
                        { value: 'E', actions: [{ action: 'set_field', field: 'visit_type', value: 'Emergency' }] },
                        { value: 'I', actions: [{ action: 'set_field', field: 'visit_type', value: 'Inpatient' }] },
                        { value: 'O', actions: [{ action: 'set_field', field: 'visit_type', value: 'Outpatient' }] }
                    ],
                    default_actions: [{ action: 'set_field', field: 'visit_type', value: 'Unknown' }]
                }
            }),

            new StepTemplate({
                id: 'for-each-loop',
                name: 'For Each Loop',
                type: 'core.logic',
                description: 'Iterate over array elements',
                layer: 'core',
                icon: this.getIconForType('core.logic'),
                isSystem: true,
                defaultConfig: {
                    array_field: 'parsed.OBX',
                    item_variable: 'observation',
                    actions: [
                        { action: 'transform', template: 'OBX_to_Observation' }
                    ]
                }
            }),

            // ============================================
            // HL7/FHIR SPECIFIC STEPS (Core)
            // ============================================
            new StepTemplate({
                id: 'hl7-segment-extractor',
                name: 'HL7 Segment Extractor',
                type: 'pre.extraction',
                description: 'Extract specific HL7 segments',
                layer: 'pre',
                icon: this.getIconForType('pre.extraction'),
                isSystem: true,
                defaultConfig: {
                    segments: ['PID', 'PV1', 'OBX'],
                    include_all_occurrences: true
                }
            }),

            new StepTemplate({
                id: 'fhir-resource-builder',
                name: 'FHIR Resource Builder',
                type: 'core.transformation',
                description: 'Build FHIR resource from data',
                layer: 'core',
                icon: this.getIconForType('core.transformation'),
                isSystem: true,
                defaultConfig: {
                    resource_type: 'Patient',
                    fhir_version: 'R4',
                    template: 'default'
                }
            }),

            // ============================================
            // ERROR HANDLING STEPS (All Layers)
            // ============================================
            new StepTemplate({
                id: 'try-catch',
                name: 'Try-Catch Block',
                type: 'pre.error_handling',
                description: 'Error handling with fallback',
                layer: 'pre',
                icon: this.getIconForType('post.fhir.validation'),
                isSystem: true,
                defaultConfig: {
                    try_steps: ['step_id_1', 'step_id_2'],
                    catch_actions: [
                        { action: 'log_error', level: 'error' },
                        { action: 'set_fallback_value', field: 'status', value: 'failed' }
                    ],
                    finally_actions: []
                }
            }),

            new StepTemplate({
                id: 'retry-logic',
                name: 'Retry Logic',
                type: 'post.error_handling',
                description: 'Retry failed operations with backoff',
                layer: 'post',
                icon: this.getIconForType('post.error_handling'),
                isSystem: true,
                defaultConfig: {
                    max_retries: 3,
                    initial_delay_ms: 1000,
                    backoff_multiplier: 2, // exponential backoff
                    max_delay_ms: 30000
                }
            }),

            // ============================================
            // DATA QUALITY STEPS (Post-Processing)
            // ============================================
            new StepTemplate({
                id: 'remove-duplicates',
                name: 'Remove Duplicates',
                type: 'post.quality',
                description: 'Remove duplicate entries',
                layer: 'post',
                icon: this.getIconForType('post.quality'),
                isSystem: true,
                defaultConfig: {
                    array_field: 'bundle.entry',
                    unique_key: 'resource.id'
                }
            }),

            new StepTemplate({
                id: 'data-masking',
                name: 'Data Masking/Anonymization',
                type: 'post.quality',
                description: 'Mask or anonymize PHI data',
                layer: 'post',
                icon: this.getIconForType('post.quality'),
                isSystem: true,
                defaultConfig: {
                    fields_to_mask: [
                        { field: 'patient.name', method: 'partial', keep_first: 1 },
                        { field: 'patient.ssn', method: 'hash' },
                        { field: 'patient.address', method: 'remove' }
                    ]
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
     * Get icon for step type (automatic mapping)
     * @param {string} stepType - Step type (e.g., 'pre.validation', 'pre.enrichment.api')
     * @returns {string} Font Awesome icon class
     */
    getIconForType(stepType) {
        // Comprehensive icon mapping based on step type
        const iconMap = {
            // ============================================
            // VALIDATION STEPS (Pre-Processing)
            // ============================================
            'pre.validation': 'fas fa-check-circle',
            'pre.validation.field': 'fas fa-check-square',
            'pre.validation.schema': 'fas fa-clipboard-check',
            'pre.validation.cross-field': 'fas fa-code-branch',

            // ============================================
            // ENRICHMENT STEPS (Pre-Processing)
            // ============================================
            'pre.enrichment': 'fas fa-plus-circle',
            'pre.enrichment.api': 'fas fa-cloud',
            'pre.enrichment.database': 'fas fa-database',
            'pre.enrichment.cache': 'fas fa-bolt',
            'pre.enrichment.script': 'fas fa-code',
            'pre.enrichment.metadata': 'fas fa-tags',
            'pre.extraction': 'fas fa-filter',

            // ============================================
            // TRANSFORMATION STEPS (Core Processing)
            // ============================================
            'core.transformation': 'fas fa-arrows-alt-h',
            'core.mapping': 'fas fa-project-diagram',
            'core.mapping.hl7-fhir': 'fas fa-exchange-alt',
            'core.mapping.custom': 'fas fa-wrench',

            // ============================================
            // POST-PROCESSING STEPS
            // ============================================
            'post.validation': 'fas fa-shield-alt',
            'post.fhir.validation': 'fas fa-shield-alt',
            'post.anonymization': 'fas fa-user-secret',
            'post.audit': 'fas fa-clipboard-list',
            'post.delivery': 'fas fa-paper-plane',
            'post.error_handling': 'fas fa-exclamation-triangle',
            'post.quality': 'fas fa-check-double',

            // ============================================
            // CONDITIONAL LOGIC
            // ============================================
            'pre.logic': 'fas fa-sitemap',
            'core.logic': 'fas fa-sitemap',
            'post.logic': 'fas fa-sitemap',

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

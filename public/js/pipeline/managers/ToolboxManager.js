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
                id: 'validate-fhir',
                name: 'FHIR Validation',
                type: 'fhir_validation',
                description: 'Validate FHIR resources and bundles against R4/R5 structure, terminology bindings, and constraints',
                layer: 'core',
                icon: this.getIconForType('fhir_validation'),
                isSystem: true,
                defaultConfig: {
                    validation_level: 'standard',
                    fhir_version: 'R4',
                    profile: 'base',
                    required_resources: [],
                    validate_references: true,
                    fail_on_error: false
                }
            }),
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
            // FILE PARSING STEPS
            // ============================================
            new StepTemplate({
                id: 'parse-file',
                name: 'File Parser',
                type: 'file_parser',
                description: 'Smart file parser with auto-detect — supports CSV, TSV, fixed-width (CCLF, NACHA, X12), Excel .xlsx/.xls and OOB healthcare templates',
                layer: 'core',
                icon: this.getIconForType('file_parser'),
                isSystem: true,
                defaultConfig: {
                    sourceType: 'field',
                    filePath: '',
                    batchMode: false,
                    filePattern: '',
                    sourceField: '',
                    autoDetect: false,
                    fileFormat: 'csv',
                    delimiter: ',',
                    hasHeader: true,
                    columns: [],
                    template: '',
                    sheetName: '',
                    sheetIndex: 0,
                    contentEncoding: '',
                    trimFields: true,
                    skipRows: 0,
                    maxRecords: 0,
                    maxFileSizeMB: 0
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

            // REMOVED: Try-Catch Block & Retry Logic - both are now per-step properties
            // (Enable via "Error Handling & Retry" section in any step's properties panel)

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
                    timeoutMs: 30000
                }
            }),

            // ============================================
            // EDI X12 STEPS (835 phase 1)
            // ============================================
            new StepTemplate({
                id: 'edi-parse',
                name: 'EDI X12 Parse',
                type: 'edi.parse',
                description: 'Parse raw X12 EDI content (835) into structured JSON (interchange/header/loops/trailer)',
                layer: 'core',
                icon: this.getIconForType('edi.parse'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'parsedEDI', transactionSet: '835' }
            }),
            new StepTemplate({
                id: 'edi-validate',
                name: 'EDI X12 Validate',
                type: 'edi.validate',
                description: 'Re-check raw EDI against the base X12 5010 standard plus your own optional rules — malformed values block, business-rule mismatches only warn',
                layer: 'core',
                icon: this.getIconForType('edi.validate'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'ediValidation', customRules: [] }
            }),
            new StepTemplate({
                id: 'edi-map-to-canonical',
                name: 'EDI Map to Canonical',
                type: 'edi.map_to_canonical',
                description: 'No-code field mapping from any source shape (CSV, DB rows, generic JSON) into the canonical JSON edi.build consumes',
                layer: 'core',
                icon: this.getIconForType('edi.map_to_canonical'),
                isSystem: true,
                defaultConfig: { outputField: 'canonicalEDI', transactionSet: '835', header: [], loops: [] }
            }),
            new StepTemplate({
                id: 'edi-build',
                name: 'EDI X12 Build',
                type: 'edi.build',
                description: 'Build a complete ISA...IEA X12 interchange (835) from canonical JSON',
                layer: 'core',
                icon: this.getIconForType('edi.build'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedEDI', transactionSet: '835', outputField: 'ediX12' }
            }),

            // ============================================
            // NCPDP SCRIPT STEPS (pharmacy e-prescribing, Phase 1:
            // NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse)
            // ============================================
            new StepTemplate({
                id: 'ncpdp-parse',
                name: 'NCPDP SCRIPT Parse',
                type: 'ncpdp.parse',
                description: 'Parse raw NCPDP SCRIPT (pharmacy e-prescribing) XML into structured JSON (transactionType/header/body)',
                layer: 'core',
                icon: this.getIconForType('ncpdp.parse'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'parsedNCPDP' }
            }),
            new StepTemplate({
                id: 'ncpdp-validate',
                name: 'NCPDP SCRIPT Validate',
                type: 'ncpdp.validate',
                description: 'Check a parsed NCPDP SCRIPT message against the schema\'s own required field/group flags',
                layer: 'core',
                icon: this.getIconForType('ncpdp.validate'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedNCPDP', outputField: 'ncpdpValidation' }
            }),
            new StepTemplate({
                id: 'ncpdp-map-to-canonical',
                name: 'NCPDP Map to Canonical',
                type: 'ncpdp.map_to_canonical',
                description: 'No-code field mapping from any source shape (CSV, DB rows, generic JSON) into the canonical JSON ncpdp.build consumes',
                layer: 'core',
                icon: this.getIconForType('ncpdp.map_to_canonical'),
                isSystem: true,
                defaultConfig: { outputField: 'parsedNCPDP', transactionType: 'NewRx', headerFields: [], headerGroups: [], bodyFields: [], bodyGroups: [] }
            }),
            new StepTemplate({
                id: 'ncpdp-build',
                name: 'NCPDP SCRIPT Build',
                type: 'ncpdp.build',
                description: 'Build a complete NCPDP SCRIPT XML message (NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse) from canonical JSON',
                layer: 'core',
                icon: this.getIconForType('ncpdp.build'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedNCPDP', transactionType: 'NewRx', outputField: 'ncpdpScript' }
            }),

            // ============================================
            // NCPDP TELECOMMUNICATION D.0 STEPS (real-time pharmacy claims,
            // Phase 1: B1 Claim Billing request/response)
            // ============================================
            new StepTemplate({
                id: 'ncpdp-telecom-parse',
                name: 'NCPDP Telecom D.0 Parse',
                type: 'ncpdptelecom.parse',
                description: 'Parse a raw NCPDP Telecommunication D.0 real-time pharmacy claim transmission into structured JSON (header/transmissionGroup/transactionGroups)',
                layer: 'core',
                icon: this.getIconForType('ncpdptelecom.parse'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'parsedTelecom' }
            }),
            new StepTemplate({
                id: 'ncpdp-telecom-validate',
                name: 'NCPDP Telecom D.0 Validate',
                type: 'ncpdptelecom.validate',
                description: 'Check a parsed D.0 transmission against the schema\'s own required field/segment flags',
                layer: 'core',
                icon: this.getIconForType('ncpdptelecom.validate'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedTelecom', outputField: 'telecomValidation' }
            }),
            new StepTemplate({
                id: 'ncpdp-telecom-map-to-canonical',
                name: 'NCPDP Telecom Map to Canonical',
                type: 'ncpdptelecom.map_to_canonical',
                description: 'No-code field mapping from any source shape (CSV, DB rows, generic JSON) into the canonical JSON ncpdptelecom.build consumes',
                layer: 'core',
                icon: this.getIconForType('ncpdptelecom.map_to_canonical'),
                isSystem: true,
                defaultConfig: { outputField: 'parsedTelecom', transactionCode: 'B1', direction: 'request', headerFields: [], transmissionGroupSegments: [], transactionGroupSegments: [] }
            }),
            new StepTemplate({
                id: 'ncpdp-telecom-build',
                name: 'NCPDP Telecom D.0 Build',
                type: 'ncpdptelecom.build',
                description: 'Build a complete NCPDP Telecommunication D.0 transmission (B1 Claim Billing request/response) from canonical JSON',
                layer: 'core',
                icon: this.getIconForType('ncpdptelecom.build'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedTelecom', transactionCode: 'B1', direction: 'request', outputField: 'ncpdpTelecom' }
            }),

            // ============================================
            // CDA/CCD STEPS
            // ============================================
            new StepTemplate({
                id: 'cda-parse',
                name: 'CDA/CCD Parse',
                type: 'cda.parse',
                description: 'Parse raw CDA/CCD XML into USCDI-keyed JSON',
                layer: 'core',
                icon: this.getIconForType('cda.parse'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'parsedCDA' }
            }),
            new StepTemplate({
                id: 'cda-normalize',
                name: 'CDA Normalizer',
                type: 'cda.normalize',
                description: 'Upgrade C32/HITSP template OIDs to C-CDA 2.1 equivalents before parsing (pass-through if already C-CDA 2.1)',
                layer: 'core',
                icon: this.getIconForType('cda.normalize'),
                isSystem: true,
                defaultConfig: { sourceField: 'raw', outputField: 'raw' }
            }),
            new StepTemplate({
                id: 'cda-to-fhir',
                name: 'CDA → FHIR R4',
                type: 'cda.to_fhir',
                description: 'Convert a parsed CDA/CCD document to a FHIR R4 Bundle (US Core)',
                layer: 'core',
                icon: this.getIconForType('cda.to_fhir'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedCDA', bundleType: 'collection', profileMode: 'us-core', onSectionFailure: 'continue', terminologyValidation: false }
            }),
            new StepTemplate({
                id: 'cda-build',
                name: 'CDA/CCD Build',
                type: 'cda.build',
                description: 'Build a C-CDA 2.1 XML document from canonical USCDI-keyed JSON',
                layer: 'core',
                icon: this.getIconForType('cda.build'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedCDA', inputFormat: 'canonical', outputField: 'cdaXML', documentType: 'CCD' }
            }),
            new StepTemplate({
                id: 'fhir-to-cda',
                name: 'FHIR → CDA',
                type: 'fhir.to_cda',
                description: 'Convert a FHIR R4 Bundle to a C-CDA 2.1 XML document for delivery to legacy systems',
                layer: 'core',
                icon: this.getIconForType('fhir.to_cda'),
                isSystem: true,
                defaultConfig: { sourceField: 'fhirBundle', outputField: 'cdaXML', profile: 'C-CDA 2.1' }
            }),
            new StepTemplate({
                id: 'cda-dedupe',
                name: 'CDA Dedupe',
                type: 'cda.dedupe',
                description: 'Remove duplicate clinical facts within one document, or (optionally) across every document ever seen for a patient on this interface',
                layer: 'core',
                icon: this.getIconForType('cda.dedupe'),
                isSystem: true,
                defaultConfig: { sourceField: 'parsedCDA', sections: [], strategy: 'first', crossMessage: false }
            }),
            new StepTemplate({
                id: 'cda-map-to-canonical',
                name: 'CDA Map to Canonical',
                type: 'cda.map_to_canonical',
                description: 'No-code field mapping from CSV/DB/generic-JSON rows into the canonical USCDI-keyed JSON cda.build consumes',
                layer: 'core',
                icon: this.getIconForType('cda.map_to_canonical'),
                isSystem: true,
                defaultConfig: { outputField: 'parsedCDA', documentType: 'CCD', header: [], sections: [] }
            }),
            new StepTemplate({
                id: 'cda-section-to-csv',
                name: 'CDA Section → CSV',
                type: 'cda.section_to_csv',
                description: 'Convert parsed CDA/CCD clinical sections into flat, one-row-per-entry CSV output',
                layer: 'core',
                icon: this.getIconForType('cda.section_to_csv'),
                isSystem: true,
                defaultConfig: { sourceField: '', sections: [], outputPrefix: 'csv_' }
            }),

            // ============================================
            // FHIR / HL7 BUILD STEPS
            // ============================================
            new StepTemplate({
                id: 'fhir-build',
                name: 'FHIR Build',
                type: 'fhir.build',
                description: 'Build a FHIR R4 resource or Bundle from source data via a no-code field-mapping UI',
                layer: 'core',
                icon: this.getIconForType('fhir.build'),
                isSystem: true,
                defaultConfig: { version: 'R4', profile: 'base', outputField: 'fhirResource' }
            }),
            new StepTemplate({
                id: 'hl7-build',
                name: 'HL7 Build',
                type: 'hl7.build',
                description: 'Build an HL7 v2 message from source data via a no-code segment/field-mapping UI',
                layer: 'core',
                icon: this.getIconForType('hl7.build'),
                isSystem: true,
                defaultConfig: { version: '2.5.1', outputField: 'hl7Message' }
            }),

            // ============================================
            // GENERAL PURPOSE STEPS
            // ============================================
            new StepTemplate({
                id: 'payload-builder',
                name: 'Payload Builder',
                type: 'payload.builder',
                description: 'Build an outbound payload (pass-through, template, field-by-field, or a FHIR Bundle) in any target format',
                layer: 'core',
                icon: this.getIconForType('payload.builder'),
                isSystem: true,
                defaultConfig: { mode: 'pass_through', output_format: 'json' }
            }),
            new StepTemplate({
                id: 'deidentify',
                name: 'De-identify (HIPAA Safe Harbor)',
                type: 'deidentify',
                description: 'Remove or hash the 18 HIPAA Safe Harbor identifiers from HL7v2/FHIR/JSON messages',
                layer: 'core',
                icon: this.getIconForType('deidentify'),
                isSystem: true,
                defaultConfig: { format: 'auto', auditHash: true }
            }),
            new StepTemplate({
                id: 'pas-envelope-mapping',
                name: 'PAS Envelope Mapping',
                type: 'pas_envelope_mapping',
                description: 'Guided field mapping for a Da Vinci PAS (Prior Authorization Support) envelope — Patient, Coverage, Provider, Service Request',
                layer: 'core',
                icon: 'fas fa-file-prescription',
                isSystem: true,
                // The builder self-populates its own default PAS field rows on
                // render (PASEnvelopeBuilder._defaultMappings()) — an empty
                // config is the correct starting point, not an omission.
                defaultConfig: {}
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
     * Filter templates by search query.
     * Fixes three confirmed bugs:
     *  1. Collapsed sections auto-expand when they contain a matching card
     *  2. Empty sections have their header hidden (no orphan section titles)
     *  3. A "no results" empty state is shown when nothing matches the query
     */
    filterTemplates(query) {
        const allCards = document.querySelectorAll('.step-card');
        let totalVisible = 0;

        // Show/hide individual cards
        allCards.forEach(card => {
            const title = card.querySelector('.step-card-title')?.textContent.toLowerCase() || '';
            const desc  = card.querySelector('.step-card-description')?.textContent.toLowerCase() || '';
            const matches = !query || title.includes(query) || desc.includes(query);
            card.style.display = matches ? '' : 'none';
            if (matches) totalVisible++;
        });

        // Per-section: auto-expand if matches inside; hide entirely if empty (no orphan headers)
        document.querySelectorAll('.toolbox-section').forEach(section => {
            const sectionCards = [...section.querySelectorAll('.step-card')];
            const visibleInSection = sectionCards.filter(c => c.style.display !== 'none').length;
            const content = section.querySelector('.section-content');
            const header  = section.querySelector('.section-title');

            if (!query) {
                // No query — restore header visibility; leave user's collapse state intact
                if (header) header.style.display = '';
            } else if (visibleInSection > 0) {
                // Has matches — force expand so cards are actually visible
                if (content) content.classList.remove('collapsed');
                if (header)  header.style.display = '';
            } else {
                // No matches in this section — hide header too, not just the cards
                if (header) header.style.display = 'none';
            }
        });

        // Empty-state feedback
        const toolboxContent = document.querySelector('.toolbox-content');
        let emptyState = document.getElementById('toolboxEmptyState');
        if (!emptyState && toolboxContent) {
            emptyState = document.createElement('div');
            emptyState.id = 'toolboxEmptyState';
            emptyState.className = 'toolbox-empty-state';
            toolboxContent.appendChild(emptyState);
        }
        if (emptyState) {
            if (query && totalVisible === 0) {
                emptyState.innerHTML = `<i class="fas fa-search"></i><p>No steps match "<strong>${query}</strong>"</p>`;
                emptyState.style.display = '';
            } else {
                emptyState.style.display = 'none';
            }
        }
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
            'file_parser': 'fas fa-file-csv',

            // ============================================
            // EDI X12 (835 phase 1)
            // ============================================
            'edi.parse': 'fas fa-file-invoice-dollar',
            'edi.validate': 'fas fa-stamp',
            'edi.map_to_canonical': 'fas fa-random',
            'edi.build': 'fas fa-file-export',

            // NCPDP SCRIPT (pharmacy e-prescribing)
            'ncpdp.parse': 'fas fa-prescription-bottle-alt',
            'ncpdp.validate': 'fas fa-stamp',
            'ncpdp.map_to_canonical': 'fas fa-random',
            'ncpdp.build': 'fas fa-file-export',

            // NCPDP Telecommunication D.0 (real-time pharmacy claims)
            'ncpdptelecom.parse': 'fas fa-receipt',
            'ncpdptelecom.validate': 'fas fa-stamp',
            'ncpdptelecom.map_to_canonical': 'fas fa-random',
            'ncpdptelecom.build': 'fas fa-file-export',

            // ============================================
            // CDA/CCD
            // ============================================
            'cda.parse': 'fas fa-file-medical',
            'cda.normalize': 'fas fa-sync-alt',
            'cda.to_fhir': 'fas fa-exchange-alt',
            'cda.build': 'fas fa-notes-medical',
            'fhir.to_cda': 'fas fa-file-medical-alt',
            'cda.dedupe': 'fas fa-clone',
            'cda.map_to_canonical': 'fas fa-sitemap',
            'cda.section_to_csv': 'fas fa-table',

            // ============================================
            // FHIR / HL7 BUILD
            // ============================================
            'fhir.build': 'fas fa-cubes',
            'hl7.build': 'fas fa-stream',

            // ============================================
            // GENERAL PURPOSE
            // ============================================
            'payload.builder': 'fas fa-box',
            'deidentify': 'fas fa-user-shield',

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

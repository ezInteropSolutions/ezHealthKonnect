/**
 * FormFieldSchema.js - Centralized Form Field Definitions
 *
 * OOB/MVC Pattern: Single source of truth for ALL form fields
 * Used by: WizardView.js, InterfaceConfigComponents.js, Edit Modal
 *
 * This eliminates ID mismatches between wizard and edit forms by defining
 * field mappings once and using them everywhere.
 */

const FormFieldSchema = {
    /**
     * Field type constants
     */
    FIELD_TYPES: {
        TEXT: 'text',
        PASSWORD: 'password',
        NUMBER: 'number',
        SELECT: 'select',
        CHECKBOX: 'checkbox',
        RADIO: 'radio',
        TEXTAREA: 'textarea',
        URL: 'url',
        EMAIL: 'email'
    },

    /**
     * Get field definition by ID
     * @param {string} fieldId - The field ID to look up
     * @returns {Object|null} Field definition or null
     */
    getField(fieldId) {
        for (const category of Object.values(this.fields)) {
            if (Array.isArray(category)) {
                const field = category.find(f => f.id === fieldId);
                if (field) return field;
            } else if (typeof category === 'object') {
                for (const subCategory of Object.values(category)) {
                    if (Array.isArray(subCategory)) {
                        const field = subCategory.find(f => f.id === fieldId);
                        if (field) return field;
                    }
                }
            }
        }
        return null;
    },

    /**
     * Get all fields for a connectivity type
     * @param {string} direction - 'source' or 'target'
     * @param {string} connectivity - e.g., 'tcp', 'http', 'database'
     * @returns {Array} Array of field definitions
     */
    getFieldsByConnectivity(direction, connectivity) {
        const key = `${direction}_${connectivity}`;
        return this.fields.connectivity[key] || [];
    },

    /**
     * Get authentication fields for a specific auth type
     * @param {string} authType - e.g., 'basic', 'bearer', 'api_key'
     * @returns {Array} Array of field definitions
     */
    getAuthFields(authType) {
        return this.fields.authentication[authType] || [];
    },

    /**
     * Extract form data using schema definitions
     * @param {HTMLElement} container - Container element with form fields
     * @param {Array} fieldDefs - Array of field definitions to extract
     * @param {string} idPrefix - Optional prefix for field IDs (for edit modal)
     * @returns {Object} Extracted data with nested paths resolved
     */
    extractFormData(container, fieldDefs, idPrefix = '') {
        const data = {};

        fieldDefs.forEach(field => {
            const elementId = idPrefix ? `${idPrefix}${this.capitalize(field.id)}` : field.id;
            const element = container.querySelector(`#${elementId}`);

            if (!element) return;

            let value;
            switch (field.type) {
                case 'checkbox':
                    value = element.checked;
                    break;
                case 'number':
                    value = element.value ? parseInt(element.value, 10) : field.default;
                    break;
                case 'select':
                case 'text':
                case 'password':
                case 'textarea':
                case 'url':
                case 'email':
                default:
                    value = element.value || field.default || '';
            }

            // Set nested path (e.g., 'sourceConfig.host')
            this.setNestedValue(data, field.modelPath, value);
        });

        return data;
    },

    /**
     * Extract checkbox group values (for HTTP methods, operations, etc.)
     * @param {HTMLElement} container - Container element
     * @param {Array} fieldDefs - Field definitions with checkboxGroup type
     * @param {string} idPrefix - Optional prefix
     * @returns {Array} Array of checked values
     */
    extractCheckboxGroup(container, fieldDefs, idPrefix = '') {
        const values = [];
        fieldDefs.forEach(field => {
            if (field.type === 'checkbox' && field.groupValue) {
                const elementId = idPrefix ? `${idPrefix}${this.capitalize(field.id)}` : field.id;
                const element = container.querySelector(`#${elementId}`);
                if (element?.checked) {
                    values.push(field.groupValue);
                }
            }
        });
        return values;
    },

    /**
     * Populate form fields from data object
     * @param {HTMLElement} container - Container element
     * @param {Object} data - Data object to populate from
     * @param {Array} fieldDefs - Field definitions
     * @param {string} idPrefix - Optional prefix
     */
    populateFormFields(container, data, fieldDefs, idPrefix = '') {
        fieldDefs.forEach(field => {
            const elementId = idPrefix ? `${idPrefix}${this.capitalize(field.id)}` : field.id;
            const element = container.querySelector(`#${elementId}`);

            if (!element) return;

            const value = this.getNestedValue(data, field.modelPath);

            if (value !== undefined && value !== null) {
                switch (field.type) {
                    case 'checkbox':
                        element.checked = Boolean(value);
                        break;
                    default:
                        element.value = value;
                }
            } else if (field.default !== undefined) {
                element.value = field.default;
            }
        });
    },

    /**
     * Helper: Set value at nested path
     */
    setNestedValue(obj, path, value) {
        const keys = path.split('.');
        let current = obj;
        for (let i = 0; i < keys.length - 1; i++) {
            if (!current[keys[i]]) {
                current[keys[i]] = {};
            }
            current = current[keys[i]];
        }
        current[keys[keys.length - 1]] = value;
    },

    /**
     * Helper: Get value from nested path
     */
    getNestedValue(obj, path) {
        const keys = path.split('.');
        let current = obj;
        for (const key of keys) {
            if (current === undefined || current === null) return undefined;
            current = current[key];
        }
        return current;
    },

    /**
     * Helper: Capitalize first letter
     */
    capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    },

    /**
     * =================================================================
     * FIELD DEFINITIONS - Single Source of Truth
     * =================================================================
     */
    fields: {
        /**
         * Basic interface information (Step 1)
         */
        basic: [
            {
                id: 'interfaceName',
                type: 'text',
                modelPath: 'name',
                label: 'Interface Name',
                required: true,
                validation: { minLength: 3, maxLength: 100 }
            },
            {
                id: 'interfaceDescription',
                type: 'textarea',
                modelPath: 'description',
                label: 'Description',
                required: false
            },
            {
                id: 'sourceType',
                type: 'select',
                modelPath: 'sourceType',
                label: 'Source Type',
                required: true,
                options: [
                    { value: 'hl7v2', label: 'HL7 v2.x' },
                    { value: 'fhir', label: 'FHIR' },
                    { value: 'cda', label: 'CDA/CCDA' },
                    { value: 'x12', label: 'X12 EDI' },
                    { value: 'json', label: 'JSON' },
                    { value: 'xml', label: 'XML' },
                    { value: 'csv', label: 'CSV' }
                ]
            },
            {
                id: 'sourceConnectivity',
                type: 'select',
                modelPath: 'sourceConnectivity',
                label: 'Source Connectivity',
                required: true,
                options: [
                    { value: 'tcp', label: 'TCP/MLLP' },
                    { value: 'http', label: 'HTTP/REST' },
                    { value: 'file', label: 'File System' },
                    { value: 'database', label: 'Database' },
                    { value: 'sftp', label: 'SFTP' },
                    { value: 'rabbitmq', label: 'RabbitMQ' },
                    { value: 'kafka', label: 'Kafka' }
                ]
            },
            {
                id: 'transformationFlow',
                type: 'select',
                modelPath: 'transformationFlow',
                label: 'Transformation Flow',
                required: true,
                options: [
                    { value: 'hl7_to_fhir', label: 'HL7 v2 to FHIR R4' },
                    { value: 'ccd_to_fhir', label: 'CCD/CCDA to FHIR R4' },
                    { value: 'hl7_to_fhir_stu3', label: 'HL7 v2 to FHIR STU3' },
                    { value: 'passthrough', label: 'Passthrough (No Transform)' },
                    { value: 'fhir_receiver', label: 'FHIR Receiver' },
                    { value: 'file_processor', label: 'File Processor' }
                ]
            }
        ],

        /**
         * Connectivity-specific fields
         */
        connectivity: {
            // TCP/MLLP Source
            source_tcp: [
                {
                    id: 'sourceHost',
                    type: 'text',
                    modelPath: 'sourceConfig.host',
                    label: 'Host',
                    default: '0.0.0.0',
                    help: 'IP address to listen on (0.0.0.0 = all interfaces)'
                },
                {
                    id: 'sourcePort',
                    type: 'number',
                    modelPath: 'sourceConfig.port',
                    label: 'Port',
                    default: 2575,
                    validation: { min: 1, max: 65535 }
                },
                {
                    id: 'sourceTlsEnabled',
                    type: 'checkbox',
                    modelPath: 'sourceConfig.tlsEnabled',
                    label: 'Enable TLS',
                    default: false
                }
            ],

            // TCP/MLLP Target
            target_tcp: [
                {
                    id: 'targetHost',
                    type: 'text',
                    modelPath: 'targetConfig.host',
                    label: 'Host',
                    required: true
                },
                {
                    id: 'targetPort',
                    type: 'number',
                    modelPath: 'targetConfig.port',
                    label: 'Port',
                    default: 2575,
                    validation: { min: 1, max: 65535 }
                },
                {
                    id: 'targetTimeout',
                    type: 'number',
                    modelPath: 'targetConfig.timeout',
                    label: 'Timeout (seconds)',
                    default: 30
                },
                {
                    id: 'targetMllpEnabled',
                    type: 'checkbox',
                    modelPath: 'targetConfig.mllpEnabled',
                    label: 'Enable MLLP Framing',
                    default: true
                }
            ],

            // HTTP Source (Receiver)
            source_http: [
                {
                    id: 'sourceHost',
                    type: 'text',
                    modelPath: 'sourceConfig.host',
                    label: 'Listen Host',
                    default: '0.0.0.0'
                },
                {
                    id: 'sourcePort',
                    type: 'number',
                    modelPath: 'sourceConfig.port',
                    label: 'Listen Port',
                    default: 8082,
                    validation: { min: 1024, max: 65535 }
                },
                {
                    id: 'httpContextPath',
                    type: 'text',
                    modelPath: 'sourceConfig.contextPath',
                    label: 'Context Path',
                    default: '/api/receive'
                },
                {
                    id: 'httpContentType',
                    type: 'select',
                    modelPath: 'sourceConfig.contentType',
                    label: 'Content Type',
                    default: 'any',
                    options: [
                        { value: 'any', label: 'Any' },
                        { value: 'application/json', label: 'application/json' },
                        { value: 'text/plain', label: 'text/plain' },
                        { value: 'application/xml', label: 'application/xml' }
                    ]
                }
            ],

            // HTTP Target
            target_http: [
                {
                    id: 'targetEndpoint',
                    type: 'url',
                    modelPath: 'targetConfig.endpoint',
                    label: 'Endpoint URL',
                    required: true,
                    help: 'Full URL including protocol (e.g., https://api.example.com/fhir/r4)'
                },
                {
                    id: 'targetVersion',
                    type: 'select',
                    modelPath: 'targetConfig.version',
                    label: 'FHIR Version',
                    default: 'R4',
                    options: [
                        { value: 'R4', label: 'R4' },
                        { value: 'STU3', label: 'STU3' },
                        { value: 'DSTU2', label: 'DSTU2' }
                    ]
                },
                {
                    id: 'targetFormat',
                    type: 'select',
                    modelPath: 'targetConfig.format',
                    label: 'Format',
                    default: 'json',
                    options: [
                        { value: 'json', label: 'JSON' },
                        { value: 'xml', label: 'XML' }
                    ]
                },
                {
                    id: 'defaultOperation',
                    type: 'select',
                    modelPath: 'targetConfig.defaultOperation',
                    label: 'Default Operation',
                    default: 'POST',
                    options: [
                        { value: 'POST', label: 'POST (Create)' },
                        { value: 'PUT', label: 'PUT (Update)' },
                        { value: 'PATCH', label: 'PATCH (Partial Update)' }
                    ]
                }
            ],

            // Database Source
            source_database: [
                {
                    id: 'sourceDbType',
                    type: 'select',
                    modelPath: 'sourceConfig.db_type',
                    label: 'Database Type',
                    required: true,
                    options: [
                        { value: 'postgresql', label: 'PostgreSQL' },
                        { value: 'mysql', label: 'MySQL' },
                        { value: 'sqlserver', label: 'SQL Server' },
                        { value: 'oracle', label: 'Oracle' },
                        { value: 'mongodb', label: 'MongoDB' }
                    ]
                },
                {
                    id: 'sourceHost',
                    type: 'text',
                    modelPath: 'sourceConfig.host',
                    label: 'Host',
                    required: true,
                    default: 'postgres', // Docker service name for containerized deployment
                    help: 'Use "postgres" for Docker, "localhost" for local development'
                },
                {
                    id: 'sourcePort',
                    type: 'number',
                    modelPath: 'sourceConfig.port',
                    label: 'Port',
                    default: 5432
                },
                {
                    id: 'sourceDatabase',
                    type: 'text',
                    modelPath: 'sourceConfig.database',
                    label: 'Database Name',
                    required: true
                },
                {
                    id: 'sourceUsername',
                    type: 'text',
                    modelPath: 'sourceConfig.username',
                    label: 'Username',
                    required: true
                },
                {
                    id: 'sourcePassword',
                    type: 'password',
                    modelPath: 'sourceConfig.password',
                    label: 'Password',
                    required: true
                },
                {
                    id: 'sourceSslMode',
                    type: 'select',
                    modelPath: 'sourceConfig.ssl_mode',
                    label: 'SSL Mode',
                    default: 'disable',
                    options: [
                        { value: 'disable', label: 'Disable' },
                        { value: 'allow', label: 'Allow' },
                        { value: 'prefer', label: 'Prefer' },
                        { value: 'require', label: 'Require' },
                        { value: 'verify-ca', label: 'Verify CA' },
                        { value: 'verify-full', label: 'Verify Full' }
                    ]
                },
                {
                    id: 'sourceTableName',
                    type: 'text',
                    modelPath: 'sourceConfig.table_name',
                    label: 'Table Name'
                },
                {
                    id: 'sourceQuery',
                    type: 'textarea',
                    modelPath: 'sourceConfig.query',
                    label: 'Custom Query',
                    help: 'Leave empty to use table name'
                },
                {
                    id: 'sourceIncrementalColumn',
                    type: 'text',
                    modelPath: 'sourceConfig.incremental_column',
                    label: 'Incremental Column',
                    help: 'Column used for tracking new records (e.g., id, created_at)'
                },
                {
                    id: 'sourceIncrementalType',
                    type: 'select',
                    modelPath: 'sourceConfig.incremental_type',
                    label: 'Incremental Type',
                    default: 'integer',
                    options: [
                        { value: 'integer', label: 'Integer' },
                        { value: 'timestamp', label: 'Timestamp' },
                        { value: 'datetime', label: 'DateTime' }
                    ]
                },
                {
                    id: 'sourcePollingInterval',
                    type: 'number',
                    modelPath: 'sourceConfig.polling_interval',
                    label: 'Polling Interval (seconds)',
                    default: 60
                },
                {
                    id: 'sourceMaxRecords',
                    type: 'number',
                    modelPath: 'sourceConfig.max_records',
                    label: 'Max Records per Poll',
                    default: 100
                },
                {
                    id: 'sourceAfterProcessing',
                    type: 'select',
                    modelPath: 'sourceConfig.after_processing',
                    label: 'After Processing',
                    default: 'nothing',
                    options: [
                        { value: 'nothing', label: 'Do Nothing' },
                        { value: 'update_flag', label: 'Update Flag Column' },
                        { value: 'archive', label: 'Move to Archive Table' },
                        { value: 'delete', label: 'Delete Record' }
                    ]
                }
            ],

            // Database Target
            target_database: [
                {
                    id: 'targetDbType',
                    type: 'select',
                    modelPath: 'targetConfig.db_type',
                    label: 'Database Type',
                    required: true,
                    options: [
                        { value: 'postgresql', label: 'PostgreSQL' },
                        { value: 'mysql', label: 'MySQL' },
                        { value: 'sqlserver', label: 'SQL Server' },
                        { value: 'oracle', label: 'Oracle' },
                        { value: 'mongodb', label: 'MongoDB' }
                    ]
                },
                {
                    id: 'targetHost',
                    type: 'text',
                    modelPath: 'targetConfig.host',
                    label: 'Host',
                    required: true
                },
                {
                    id: 'targetPort',
                    type: 'number',
                    modelPath: 'targetConfig.port',
                    label: 'Port',
                    default: 5432
                },
                {
                    id: 'targetDatabase',
                    type: 'text',
                    modelPath: 'targetConfig.database',
                    label: 'Database Name',
                    required: true
                },
                {
                    id: 'targetUsername',
                    type: 'text',
                    modelPath: 'targetConfig.username',
                    label: 'Username',
                    required: true
                },
                {
                    id: 'targetPassword',
                    type: 'password',
                    modelPath: 'targetConfig.password',
                    label: 'Password',
                    required: true
                },
                {
                    id: 'targetTableName',
                    type: 'text',
                    modelPath: 'targetConfig.table_name',
                    label: 'Target Table',
                    required: true
                }
            ],

            // File Source
            source_file: [
                {
                    id: 'sourceFilePath',
                    type: 'text',
                    modelPath: 'sourceConfig.filePath',
                    label: 'Watch Directory',
                    required: true
                },
                {
                    id: 'sourceFilePattern',
                    type: 'text',
                    modelPath: 'sourceConfig.filePattern',
                    label: 'File Pattern',
                    default: '*.hl7',
                    help: 'Glob pattern (e.g., *.hl7, *.json)'
                },
                {
                    id: 'sourcePollingInterval',
                    type: 'number',
                    modelPath: 'sourceConfig.polling_interval',
                    label: 'Polling Interval (seconds)',
                    default: 30
                },
                {
                    id: 'sourceAfterProcessing',
                    type: 'select',
                    modelPath: 'sourceConfig.after_processing',
                    label: 'After Processing',
                    default: 'archive',
                    options: [
                        { value: 'nothing', label: 'Leave in Place' },
                        { value: 'archive', label: 'Move to Archive' },
                        { value: 'delete', label: 'Delete File' }
                    ]
                }
            ],

            // File Target
            target_file: [
                {
                    id: 'targetFilePath',
                    type: 'text',
                    modelPath: 'targetConfig.filePath',
                    label: 'Output Directory',
                    required: true
                },
                {
                    id: 'targetFilePattern',
                    type: 'text',
                    modelPath: 'targetConfig.filePattern',
                    label: 'Filename Pattern',
                    default: '{messageId}.json',
                    help: 'Available: {messageId}, {timestamp}, {resourceType}'
                },
                {
                    id: 'targetFileFormat',
                    type: 'select',
                    modelPath: 'targetConfig.fileFormat',
                    label: 'Output Format',
                    default: 'json',
                    options: [
                        { value: 'json', label: 'JSON' },
                        { value: 'xml', label: 'XML' },
                        { value: 'hl7', label: 'HL7' }
                    ]
                },
                {
                    id: 'targetCreateDirectory',
                    type: 'checkbox',
                    modelPath: 'targetConfig.createDirectory',
                    label: 'Create Directory if Missing',
                    default: true
                }
            ]
        },

        /**
         * Authentication fields - used by both source and target HTTP
         * Note: IDs match both wizard manual HTML and InterfaceConfigComponents
         * The wizard uses simplified IDs (authType, authUsername) while
         * InterfaceConfigComponents uses prefixed IDs (${prefix}AuthUsername)
         */
        authentication: {
            // Auth type selector - wizard uses 'authType', components use '${prefix}HttpAuthType'
            selector: [
                {
                    id: 'authType',
                    altIds: ['HttpAuthType'], // Alternative ID pattern for shared components
                    type: 'select',
                    modelPath: 'authType',
                    label: 'Authentication Type',
                    default: 'none',
                    options: [
                        { value: 'none', label: 'No Authentication' },
                        { value: 'basic', label: 'Basic Authentication' },
                        { value: 'bearer', label: 'Bearer Token' },
                        { value: 'api_key', label: 'API Key' },
                        { value: 'oauth2', label: 'OAuth 2.0' }
                    ]
                }
            ],

            // Basic Auth - wizard uses authUsername/Password, components use AuthUsername/Password
            basic: [
                {
                    id: 'authUsername',
                    altIds: ['AuthUsername'],
                    type: 'text',
                    modelPath: 'username',
                    label: 'Username',
                    required: true
                },
                {
                    id: 'authPassword',
                    altIds: ['AuthPassword'],
                    type: 'password',
                    modelPath: 'password',
                    label: 'Password',
                    required: true
                }
            ],

            // Bearer Token - wizard uses authToken, components use AuthBearerToken
            bearer: [
                {
                    id: 'authToken',
                    altIds: ['AuthBearerToken'],
                    type: 'password',
                    modelPath: 'bearerToken',
                    label: 'Bearer Token',
                    required: true
                }
            ],

            // API Key
            api_key: [
                {
                    id: 'authApiKey',
                    altIds: ['AuthApiKeyValue'],
                    type: 'password',
                    modelPath: 'apiKey',
                    label: 'API Key',
                    required: true
                },
                {
                    id: 'authApiKeyHeader',
                    altIds: ['AuthApiKeyHeader'],
                    type: 'text',
                    modelPath: 'apiKeyHeader',
                    label: 'Header Name',
                    default: 'X-API-Key'
                }
            ],

            // OAuth 2.0
            oauth2: [
                {
                    id: 'authOAuthIssuer',
                    altIds: ['AuthOAuthIssuer'],
                    type: 'url',
                    modelPath: 'oauthIssuer',
                    label: 'Token Issuer URL',
                    required: true
                },
                {
                    id: 'authOAuthClientId',
                    altIds: ['AuthOAuthClientId'],
                    type: 'text',
                    modelPath: 'clientId',
                    label: 'Client ID',
                    required: true
                },
                {
                    id: 'authOAuthClientSecret',
                    altIds: ['AuthOAuthClientSecret'],
                    type: 'password',
                    modelPath: 'clientSecret',
                    label: 'Client Secret',
                    required: true
                },
                {
                    id: 'authOAuthScopes',
                    altIds: ['AuthOAuthScopes'],
                    type: 'text',
                    modelPath: 'scopes',
                    label: 'Scopes',
                    help: 'Space-separated list of scopes'
                }
            ]
        },

        /**
         * Target configuration (Step 4)
         */
        target: {
            type: [
                {
                    id: 'targetType',
                    type: 'select',
                    modelPath: 'targetType',
                    label: 'Target Type',
                    required: true,
                    options: [
                        { value: 'fhir', label: 'FHIR R4' },
                        { value: 'fhir-stu3', label: 'FHIR STU3' },
                        { value: 'hl7v2', label: 'HL7 v2.x' },
                        { value: 'database', label: 'Database' },
                        { value: 'file', label: 'File System' }
                    ]
                },
                {
                    id: 'targetConnectivity',
                    type: 'select',
                    modelPath: 'targetConnectivity',
                    label: 'Target Connectivity',
                    required: true,
                    options: [
                        { value: 'http', label: 'HTTP/REST' },
                        { value: 'tcp', label: 'TCP/MLLP' },
                        { value: 'file', label: 'File System' },
                        { value: 'database', label: 'Database' },
                        { value: 'sink', label: 'Sink (Store Only)' }
                    ]
                }
            ],

            // Delivery mode for FHIR
            delivery: [
                {
                    id: 'deliveryModeBundle',
                    type: 'radio',
                    modelPath: 'targetConfig.deliveryMode',
                    label: 'Bundle',
                    groupValue: 'bundle',
                    name: 'deliveryMode'
                },
                {
                    id: 'deliveryModeIndividual',
                    type: 'radio',
                    modelPath: 'targetConfig.deliveryMode',
                    label: 'Individual Resources',
                    groupValue: 'individual',
                    name: 'deliveryMode'
                }
            ]
        },

        /**
         * HTTP Methods (checkboxes)
         */
        httpMethods: [
            { id: 'HttpMethodPost', type: 'checkbox', modelPath: 'sourceConfig.allowedMethods', groupValue: 'POST', label: 'POST', default: true },
            { id: 'HttpMethodPut', type: 'checkbox', modelPath: 'sourceConfig.allowedMethods', groupValue: 'PUT', label: 'PUT' },
            { id: 'HttpMethodGet', type: 'checkbox', modelPath: 'sourceConfig.allowedMethods', groupValue: 'GET', label: 'GET' },
            { id: 'HttpMethodPatch', type: 'checkbox', modelPath: 'sourceConfig.allowedMethods', groupValue: 'PATCH', label: 'PATCH' },
            { id: 'HttpMethodDelete', type: 'checkbox', modelPath: 'sourceConfig.allowedMethods', groupValue: 'DELETE', label: 'DELETE' }
        ],

        /**
         * FHIR Operations (checkboxes)
         */
        fhirOperations: [
            { id: 'OpCreate', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'CREATE', label: 'Create', default: true },
            { id: 'OpRead', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'READ', label: 'Read', default: true },
            { id: 'OpUpdate', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'UPDATE', label: 'Update' },
            { id: 'OpDelete', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'DELETE', label: 'Delete' },
            { id: 'OpSearch', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'SEARCH', label: 'Search' },
            { id: 'OpBatch', type: 'checkbox', modelPath: 'sourceConfig.operations', groupValue: 'BATCH', label: 'Batch/Transaction' }
        ],

        /**
         * FHIR Resource Types (checkboxes for individual delivery)
         */
        fhirResources: [
            { id: 'resourcePatient', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'Patient', label: 'Patient' },
            { id: 'resourceEncounter', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'Encounter', label: 'Encounter' },
            { id: 'resourceObservation', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'Observation', label: 'Observation' },
            { id: 'resourceCondition', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'Condition', label: 'Condition' },
            { id: 'resourceProcedure', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'Procedure', label: 'Procedure' },
            { id: 'resourceMedicationRequest', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'MedicationRequest', label: 'MedicationRequest' },
            { id: 'resourceDiagnosticReport', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'DiagnosticReport', label: 'DiagnosticReport' },
            { id: 'resourceAllergyIntolerance', type: 'checkbox', modelPath: 'targetConfig.individualResources', groupValue: 'AllergyIntolerance', label: 'AllergyIntolerance' }
        ]
    }
};

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FormFieldSchema;
}

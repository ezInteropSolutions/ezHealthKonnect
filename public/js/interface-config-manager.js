// interface-config-manager.js
// Enhanced interface configuration management for comprehensive editing

class BasicInterfaceConfigManager {
    constructor() {
        this.availableInterfaces = [];
        this.currentInterface = null;
        this.formBindingEngine = null;
    }

    /**
     * Initialize form binding engine for edit modal
     * OOB Pattern: Uses FormFieldSchema as single source of truth
     */
    initializeFormBinding() {
        const editModal = document.getElementById('editInterfaceModal');
        if (editModal && typeof FormFieldSchema !== 'undefined' && typeof FormBindingEngine !== 'undefined') {
            this.formBindingEngine = new FormBindingEngine(editModal, FormFieldSchema, 'edit');
            console.log('✅ Edit modal FormBindingEngine initialized with schema (prefix: edit)');
        }
    }

    /**
     * Initialize the config manager
     */
    async init() {
        await this.loadAvailableInterfaces();
        this.setupDynamicFieldHandlers();
        this.initializeFormBinding(); // OOB: Initialize schema-based form binding
    }

    /**
     * Load available interfaces for routing configuration
     */
    async loadAvailableInterfaces() {
        try {
            const response = await fetch('/api/interfaces');
            if (response.ok) {
                const data = await response.json();
                this.availableInterfaces = data.interfaces || [];
                this.populateFhirInterfaceOptions();
            }
        } catch (error) {
            console.error('Failed to load available interfaces:', error);
        }
    }

    /**
     * Populate FHIR interface options for routing
     */
    populateFhirInterfaceOptions() {
        const select = document.getElementById('editTargetFhirInterface');
        if (!select) return;

        // Clear existing options
        select.innerHTML = '<option value="">Select FHIR Interface...</option>';

        // Add FHIR interfaces
        this.availableInterfaces
            .filter(i => i.format === 'FHIR' || i.format === 'fhir')
            .forEach(interfaceItem => {
                const option = document.createElement('option');
                option.value = interfaceItem.id;
                option.textContent = `${interfaceItem.name} (${interfaceItem.status})`;
                select.appendChild(option);
            });
    }

    /**
     * Setup dynamic field change handlers
     */
    setupDynamicFieldHandlers() {
        // Source type change handler
        const sourceTypeSelect = document.getElementById('editSourceType');
        if (sourceTypeSelect) {
            sourceTypeSelect.addEventListener('change', () => updateSourceFields());
        }

        // Target type change handler
        const targetTypeSelect = document.getElementById('editTargetType');
        if (targetTypeSelect) {
            targetTypeSelect.addEventListener('change', () => updateTargetFields());
        }

        // Routing mode change handler
        const routingModeSelect = document.getElementById('editRoutingMode');
        if (routingModeSelect) {
            routingModeSelect.addEventListener('change', () => updateRoutingFields());
        }

        // Table strategy auto-recommendation
        const expectedVolumeSelect = document.getElementById('editExpectedVolume');
        if (expectedVolumeSelect) {
            expectedVolumeSelect.addEventListener('change', () => this.recommendTableStrategy());
        }
    }

    /**
     * Auto-recommend table strategy based on expected volume
     */
    recommendTableStrategy() {
        const volume = document.getElementById('editExpectedVolume').value;
        const tableStrategy = document.getElementById('editTableStrategy');

        if (!tableStrategy) return;

        switch (volume) {
            case 'high':
                tableStrategy.value = 'dedicated';
                this.showFieldTooltip(tableStrategy, 'Recommended: Dedicated table for high volume');
                break;
            case 'medium':
                tableStrategy.value = 'dedicated';
                this.showFieldTooltip(tableStrategy, 'Recommended: Dedicated table for better performance');
                break;
            case 'low':
                tableStrategy.value = 'shared';
                this.showFieldTooltip(tableStrategy, 'Shared table is sufficient for low volume');
                break;
        }
    }

    /**
     * Show temporary tooltip for field recommendations
     */
    showFieldTooltip(element, message) {
        const tooltip = document.createElement('div');
        tooltip.className = 'field-recommendation-tooltip';
        tooltip.textContent = message;
        tooltip.style.cssText = `
            position: absolute;
            background: #1e3a8a;
            color: white;
            padding: 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            z-index: 1000;
            top: 100%;
            left: 0;
            margin-top: 0.25rem;
        `;

        element.parentNode.style.position = 'relative';
        element.parentNode.appendChild(tooltip);

        setTimeout(() => {
            if (tooltip.parentNode) {
                tooltip.parentNode.removeChild(tooltip);
            }
        }, 3000);
    }

    /**
     * Populate edit form with interface data
     */
    populateEditForm(interfaceData) {
        console.log('🔧 Populating edit form with interface data:', interfaceData);
        this.currentInterface = interfaceData;

        // Basic information
        const setFieldValue = (fieldId, value, defaultValue = '') => {
            const field = document.getElementById(fieldId);
            if (field) {
                field.value = value || defaultValue;
                console.log(`✅ Set ${fieldId}: ${field.value}`);
            } else {
                console.warn(`⚠️ Field not found: ${fieldId}`);
            }
        };

        setFieldValue('editInterfaceId', interfaceData.id);
        setFieldValue('editInterfaceName', interfaceData.name);
        setFieldValue('editInterfaceDescription', interfaceData.description);
        setFieldValue('editFormat', interfaceData.messageType || interfaceData.format, 'HL7');
        setFieldValue('editStatus', interfaceData.status, 'inactive');

        // Source configuration - MAP sourceType to modal dropdown values
        // Database has: file, tcp, http, database, mllp
        // Interface uses: hl7v2, hl7, fhir, file, database, http
        const sourceTypeMapping = {
            'hl7v2': 'tcp',      // HL7 v2.x uses TCP/MLLP
            'hl7': 'tcp',        // Generic HL7 uses TCP
            'fhir': 'http',      // FHIR uses HTTP
            'file': 'file',      // Direct mapping
            'database': 'database', // Direct mapping
            'http': 'http'       // Direct mapping
        };
        const mappedSourceType = sourceTypeMapping[interfaceData.sourceType] || interfaceData.sourceType || 'tcp';

        setFieldValue('editSourceType', mappedSourceType, 'tcp');
        setFieldValue('editSourceConnectivity', interfaceData.sourceConnectivity, 'inbound');

        // Target configuration
        setFieldValue('editTargetType', interfaceData.targetType, 'fhir');
        setFieldValue('editTargetConnectivity', interfaceData.targetConnectivity, 'outbound');

        // Parse and populate configuration from JSON fields
        this.populateConfigurationFields(interfaceData);

        // Update dynamic fields based on current selections
        setTimeout(() => {
            if (typeof updateSourceFields === 'function') updateSourceFields();
            if (typeof updateTargetFields === 'function') updateTargetFields();
            if (typeof updateRoutingFields === 'function') updateRoutingFields();

            // Auto-correct target type if config suggests it should be different
            this.autoCorrectTargetType(interfaceData);
        }, 100);

        console.log('✅ Edit form populated with interface data:', interfaceData.name);
    }

    /**
     * Populate configuration fields from JSON data
     */
    populateConfigurationFields(interfaceData) {
        console.log('🔧 Populating configuration fields...');
        console.log('🔍 [DEBUG] Full interfaceData received:', interfaceData);

        try {
            // Source configuration - V30+ uses source_connectivity.config
            let sourceConfig = {};
            if (interfaceData.source_connectivity?.config) {
                // New V30+ format: {type, config}
                sourceConfig = interfaceData.source_connectivity.config;
            } else if (interfaceData.source_config) {
                // Legacy format fallback
                sourceConfig = typeof interfaceData.source_config === 'string'
                    ? JSON.parse(interfaceData.source_config)
                    : interfaceData.source_config;
            } else if (interfaceData.sourceConfig) {
                sourceConfig = interfaceData.sourceConfig;
            }

            console.log('📋 Source config:', sourceConfig);

            // Common fields for TCP/MLLP/HTTP source types
            this.setFieldIfExists('editSourceHost', sourceConfig.host);
            this.setFieldIfExists('editSourcePort', sourceConfig.port);

            // Database-specific fields - InterfaceConfigComponents uses {prefix}source{Field} pattern
            // So for edit modal with idPrefix='edit', fields are: editsourceHost, editsourceDatabase, etc.
            // Note: Component uses capital letters: Host, Port, Database, Username, Password, SslMode
            // Config uses snake_case: ssl_mode, table_name, polling_interval
            this.setFieldIfExists('editsourceHost', sourceConfig.host);
            this.setFieldIfExists('editsourcePort', sourceConfig.port);
            this.setFieldIfExists('editsourceDatabase', sourceConfig.database);
            this.setFieldIfExists('editsourceUsername', sourceConfig.username);
            this.setFieldIfExists('editsourcePassword', sourceConfig.password);
            this.setFieldIfExists('editsourceSslMode', sourceConfig.ssl_mode || sourceConfig.sslMode);
            this.setFieldIfExists('editsourceTableName', sourceConfig.table_name || sourceConfig.tableName);
            this.setFieldIfExists('editsourcePollingInterval', sourceConfig.polling_interval || sourceConfig.pollingInterval);
            this.setFieldIfExists('editsourceCustomQuery', sourceConfig.query);

            // TCP/MLLP specific fields
            this.setFieldIfExists('editSourceTLS', sourceConfig.useTLS);
            this.setFieldIfExists('editSourceAckMode', sourceConfig.ackMode);

            // HTTP specific fields
            this.setFieldIfExists('editSourcePath', sourceConfig.path);

            // DEBUG: Verify the values were actually set
            setTimeout(() => {
                const hostField = document.getElementById('editSourceHost');
                const portField = document.getElementById('editSourcePort');
                console.log('🔍 DEBUG: Source fields after population:', {
                    hostExists: !!hostField,
                    hostValue: hostField?.value,
                    hostVisible: hostField?.offsetParent !== null,
                    portExists: !!portField,
                    portValue: portField?.value,
                    portVisible: portField?.offsetParent !== null
                });
            }, 500);

            // Target configuration - V30+ uses target_connectivity.config
            let targetConfig = {};
            if (interfaceData.target_connectivity?.config) {
                // New V30+ format: {type, config}
                targetConfig = interfaceData.target_connectivity.config;
                console.log('📦 Using V30+ target_connectivity.config format');
            } else if (interfaceData.target_config) {
                // Legacy format fallback
                targetConfig = typeof interfaceData.target_config === 'string'
                    ? JSON.parse(interfaceData.target_config)
                    : interfaceData.target_config;
                console.log('📦 Using legacy target_config format');
            } else if (interfaceData.targetConfig) {
                targetConfig = interfaceData.targetConfig;
                console.log('📦 Using targetConfig (camelCase) format');
            }

            console.log('🎯 Target config extracted:', JSON.stringify(targetConfig, null, 2));
            console.log('🔑 Auth fields in targetConfig:', {
                authType: targetConfig.authType,
                username: targetConfig.username,
                password: targetConfig.password ? '***' : undefined,
                bearerToken: targetConfig.bearerToken ? '***' : undefined
            });

            // Use object-oriented approach for populating target fields
            const targetType = interfaceData.targetType;
            const targetHandler = window.InterfaceHandlerFactory?.createHandler(targetType, interfaceData);

            if (targetHandler) {
                console.log(`🔧 Using ${targetHandler.constructor.name} for field population`);
                targetHandler.populateTargetFields(targetConfig);
            } else {
                console.warn('⚠️ Interface handlers not loaded, using fallback population');
                // Fallback logic for basic field population
                this.setFieldIfExists('editTargetPort', targetConfig.port);
                if (targetConfig.path) {
                    this.setFieldIfExists('editResourceEndpoint',
                        targetConfig.path.replace(/^\/+/, '').replace(/^fhir\/?/i, '') || 'Patient');
                }
            }

            // Routing configuration
            const processingRules = typeof interfaceData.processing_rules === 'string'
                ? JSON.parse(interfaceData.processing_rules)
                : interfaceData.processing_rules || interfaceData.processingRules || {};

            console.log('🔄 Processing rules:', processingRules);

            this.setFieldIfExists('editRoutingMode', processingRules.routingMode);
            this.setFieldIfExists('editTargetFhirInterface', processingRules.targetFhirInterface);
            this.setFieldIfExists('editTransformationEngine', processingRules.transformationEngine);
            this.setFieldIfExists('editRetryPolicy', processingRules.retryPolicy);

            // Performance configuration
            if (interfaceData.use_dedicated_table !== undefined) {
                this.setFieldIfExists('editTableStrategy',
                    interfaceData.use_dedicated_table ? 'dedicated' : 'shared');
            }

            console.log('✅ Configuration fields populated');

        } catch (error) {
            console.error('❌ Error parsing interface configuration:', error);
        }
    }

    /**
     * Helper method to set field value if field exists
     */
    setFieldIfExists(fieldId, value) {
        const field = document.getElementById(fieldId);
        if (field && value !== null && value !== undefined) {
            field.value = value;
            console.log(`✅ Set ${fieldId}: ${value}`);
        } else if (!field) {
            console.warn(`⚠️ Field not found: ${fieldId}`);
        }
    }

    /**
     * Auto-correct target type based on configuration patterns
     */
    autoCorrectTargetType(interfaceData) {
        const currentTargetType = document.getElementById('editTargetType')?.value;
        const targetConfig = typeof interfaceData.target_config === 'string'
            ? JSON.parse(interfaceData.target_config)
            : interfaceData.target_config || {};

        console.log('🔍 Auto-correct check:', {
            currentTargetType,
            targetConfig,
            hasPath: !!targetConfig.path,
            pathValue: targetConfig.path
        });

        // Detect FHIR-like configuration
        if (currentTargetType === 'database' &&
            (targetConfig.path === '/fhir' || targetConfig.fhirServerUrl)) {

            console.log('🔧 Auto-correcting target type from database to fhir');
            this.setFieldIfExists('editTargetType', 'fhir');

            // Trigger update to show FHIR fields
            if (typeof updateTargetFields === 'function') {
                updateTargetFields();
            }

            // Re-populate with FHIR handler
            setTimeout(() => {
                const fhirHandler = window.InterfaceHandlerFactory?.createHandler('fhir', interfaceData);
                if (fhirHandler) {
                    console.log('🔧 Re-populating with FHIR handler');
                    fhirHandler.populateTargetFields(targetConfig);
                }
            }, 200);
        }
    }

    /**
     * Collect form data for saving
     * Uses InterfaceConfigComponents for unified data collection (same as Wizard)
     */
    collectFormData() {
        console.log('📋 Collecting form data via ConnectorConfigBuilder instances...');

        // Connector step IDs set by populateEditForm
        const stepIds = document.getElementById('editConnectorStepIds');
        const inboundStepId = stepIds?.dataset.inboundStepId || null;
        const outboundStepId = stepIds?.dataset.outboundStepId || null;

        // Full builder configs — these are the single source of truth sent to the backend
        const inboundBuilderConfig = window._editInboundBuilder?.getConfig() || {};
        const outboundBuilderConfig = window._editOutboundBuilder?.getConfig() || {};

        // Collect deployment settings from shared component (still uses InterfaceConfigComponents)
        const deploymentData = InterfaceConfigComponents.collectDeploymentSettings
            ? InterfaceConfigComponents.collectDeploymentSettings(document, 'edit')
            : {};

        const formData = {
            id: document.getElementById('editInterfaceId')?.value || '',
            name: document.getElementById('editInterfaceName')?.value || '',
            description: document.getElementById('editInterfaceDescription')?.value || '',
            status: document.getElementById('editStatus')?.value || 'inactive',

            // Connector step IDs — backend uses these for precise row targeting
            inboundStepId: inboundStepId || undefined,
            outboundStepId: outboundStepId || undefined,

            // Full connector configs — backend replaces the entire step config when connectorType is present
            sourceConnectorConfig: Object.keys(inboundBuilderConfig).length > 0 ? inboundBuilderConfig : undefined,
            targetConnectorConfig: Object.keys(outboundBuilderConfig).length > 0 ? outboundBuilderConfig : undefined,

            // Inner config — kept for backward compat with interfaces table source_config / target_config columns
            sourceConfig: inboundBuilderConfig.config || {},
            targetConfig: outboundBuilderConfig.config || {},

            // Connectivity strings for interfaces table columns
            sourceConnectivity: inboundBuilderConfig.connectorType || undefined,
            targetConnectivity: outboundBuilderConfig.connectorType || undefined,

            // Deployment settings
            deployment_mode: deploymentData.deployment_mode,
            auto_start: deploymentData.auto_start,
            deployment_delay_seconds: deploymentData.deployment_delay_seconds,

            // Logging settings
            log_level: getLogLevel(),
            debug_logging: getLogLevel() !== 'off',
            log_retention_days: parseInt(document.getElementById('editLogRetention')?.value) || 30,
            retain_error_logs_forever: document.getElementById('editRetainErrors')?.checked !== false,
        };

        // Derive sourceType / targetType for interfaces table (COALESCE keeps existing if null)
        const connectorToSourceType = {
            'tcp_mllp_inbound': 'hl7v2', 'http_fhir_inbound': 'fhir',
            'http_rest_inbound': 'json',  'file_listener': 'file',
            'postgresql_inbound': 'database', 'mysql_inbound': 'database',
            'sqlserver_inbound': 'database', 'oracle_inbound': 'database',
            'mongodb_inbound': 'database', 'kafka_inbound': 'kafka',
            'rabbitmq_inbound': 'rabbitmq', 'redis_inbound': 'redis',
            'aws_s3_inbound': 'file', 'sftp_inbound': 'sftp',
        };
        const connectorToTargetType = {
            'http_outbound': 'fhir', 'http_fhir_outbound': 'fhir',
            'tcp_mllp_outbound': 'hl7v2', 'file_writer': 'file',
            'postgresql_outbound': 'database', 'mysql_outbound': 'database',
            'sqlserver_outbound': 'database', 'oracle_outbound': 'database',
            'mongodb_outbound': 'database', 'sink_outbound': 'sink',
        };
        formData.sourceType = connectorToSourceType[inboundBuilderConfig.connectorType] || undefined;
        formData.targetType  = connectorToTargetType[outboundBuilderConfig.connectorType] || undefined;
        formData.format = formData.sourceType;
        formData.messageType = formData.sourceType;

        // Processing rules (fields may not exist in new layout)
        formData.processingRules = {
            routingMode: document.getElementById('editRoutingMode')?.value || 'direct',
            targetFhirInterface: document.getElementById('editTargetFhirInterface')?.value || '',
            transformationEngine: document.getElementById('editTransformationEngine')?.value || 'go-engine',
            retryPolicy: document.getElementById('editRetryPolicy')?.value || '3'
        };

        console.log('📦 Collected form data (inbound connectorType:', inboundBuilderConfig.connectorType,
                    ', outbound connectorType:', outboundBuilderConfig.connectorType, ')');
        return formData;
    }

    /**
     * Collect source configuration from shared components
     * @param {string} idPrefix - ID prefix for form fields
     */
    collectSourceConfig(idPrefix = '') {
        // Handle both casing patterns: editSourceConnectivity (our code) and editsourceConnectivity (components)
        const sourceConnectivity = document.getElementById(`${idPrefix}SourceConnectivity`)?.value ||
                                   document.getElementById(`${idPrefix}sourceConnectivity`)?.value ||
                                   document.getElementById(`${idPrefix}SourceType`)?.value ||  // fallback to source type
                                   'tcp';
        const config = {};

        console.log(`🔧 collectSourceConfig: idPrefix="${idPrefix}", sourceConnectivity="${sourceConnectivity}"`);

        // Common fields
        const port = document.getElementById(`${idPrefix}sourcePort`)?.value;
        const host = document.getElementById(`${idPrefix}sourceHost`)?.value;

        if (port) config.port = parseInt(port);
        if (host) config.host = host;

        // TCP/MLLP specific
        if (sourceConnectivity === 'tcp') {
            const protocol = document.getElementById(`${idPrefix}tcpProtocol`)?.value;
            const encoding = document.getElementById(`${idPrefix}tcpEncoding`)?.value;
            if (protocol) config.protocol = protocol;
            if (encoding) config.encoding = encoding;
        }

        // FHIR specific
        if (sourceConnectivity === 'http') {
            const basePath = document.getElementById(`${idPrefix}fhirBasePath`)?.value;
            const fhirVersion = document.getElementById(`${idPrefix}FhirVersion`)?.value;
            const authType = document.getElementById(`${idPrefix}HttpAuthType`)?.value;

            if (basePath) config.basePath = basePath;
            if (fhirVersion) config.fhirVersion = fhirVersion;
            if (authType) config.authType = authType;

            // Collect auth details based on type
            if (authType === 'basic') {
                config.authUsername = document.getElementById(`${idPrefix}HttpAuthUsername`)?.value;
                config.authPassword = document.getElementById(`${idPrefix}HttpAuthPassword`)?.value;
            } else if (authType === 'api_key') {
                config.authApiKey = document.getElementById(`${idPrefix}HttpAuthApiKey`)?.value;
                config.authHeaderName = document.getElementById(`${idPrefix}HttpAuthHeaderName`)?.value;
            } else if (authType === 'bearer') {
                config.authBearerToken = document.getElementById(`${idPrefix}HttpAuthBearerToken`)?.value;
            }
        }

        // File Listener specific
        if (sourceConnectivity === 'file') {
            const directoryPath = document.getElementById(`${idPrefix}sourceDirectoryPath`)?.value;
            const filePattern = document.getElementById(`${idPrefix}sourceFilePattern`)?.value;
            const pollingInterval = document.getElementById(`${idPrefix}sourcePollingInterval`)?.value;
            const afterProcessing = document.getElementById(`${idPrefix}sourceAfterProcessing`)?.value;
            const archivePath = document.getElementById(`${idPrefix}sourceArchivePath`)?.value;
            const encoding = document.getElementById(`${idPrefix}sourceEncoding`)?.value;
            const createDirs = document.getElementById(`${idPrefix}sourceCreateDirs`)?.checked;

            // Use snake_case for consistency with Go backend
            if (directoryPath) config.directory_path = directoryPath;
            if (filePattern) config.file_pattern = filePattern;
            if (pollingInterval) config.polling_interval = parseInt(pollingInterval);
            if (afterProcessing) config.after_processing = afterProcessing;
            if (archivePath) config.archive_path = archivePath;
            if (encoding) config.encoding = encoding;
            config.create_dirs = createDirs === true;

            console.log('📂 File source config collected:', config);
        }

        // Database specific - uses InterfaceConfigComponents field IDs
        if (sourceConnectivity === 'database') {
            // InterfaceConfigComponents uses prefix pattern: {idPrefix}source{Field}
            // e.g., editsourceHost, editsourcePort, editsourceDatabase
            // Note: Component uses capital letters: Host, Port, Database, Username, Password, SslMode
            const dbHost = document.getElementById(`${idPrefix}sourceHost`)?.value;
            const dbPort = document.getElementById(`${idPrefix}sourcePort`)?.value;
            const dbName = document.getElementById(`${idPrefix}sourceDatabase`)?.value;
            const username = document.getElementById(`${idPrefix}sourceUsername`)?.value;
            const password = document.getElementById(`${idPrefix}sourcePassword`)?.value;
            const sslMode = document.getElementById(`${idPrefix}sourceSslMode`)?.value; // SslMode not SSLMode
            const tableName = document.getElementById(`${idPrefix}sourceTableName`)?.value;
            const pollingInterval = document.getElementById(`${idPrefix}sourcePollingInterval`)?.value;
            const customQuery = document.getElementById(`${idPrefix}sourceCustomQuery`)?.value;

            if (dbHost) config.host = dbHost;
            if (dbPort) config.port = parseInt(dbPort);
            if (dbName) config.database = dbName;
            if (username) config.username = username;
            if (password) config.password = password;
            if (sslMode) config.ssl_mode = sslMode; // snake_case to match InterfaceConfigComponents
            if (tableName) config.table_name = tableName; // snake_case for consistency
            if (pollingInterval) config.polling_interval = parseInt(pollingInterval); // snake_case
            if (customQuery) config.query = customQuery;

            console.log('🗄️ Database source config collected:', config);
        }

        return config;
    }

    /**
     * Collect target configuration from shared components
     * @param {string} idPrefix - ID prefix for form fields
     * @param {string} connectivity - Target connectivity type
     */
    collectTargetConfig(idPrefix = '', connectivity = 'http') {
        const config = {};

        if (connectivity === 'sink') {
            // Sink target (store only, no routing)
            const enableLogging = document.getElementById(`${idPrefix}sinkEnableLogging`)?.checked;
            const enableValidation = document.getElementById(`${idPrefix}sinkEnableValidation`)?.checked;
            const retentionDays = document.getElementById(`${idPrefix}sinkRetentionDays`)?.value;
            const generateAck = document.getElementById(`${idPrefix}sinkGenerateAck`)?.checked;

            config.enableLogging = enableLogging !== false;
            config.enableValidation = enableValidation === true;
            config.retentionDays = retentionDays ? parseInt(retentionDays) : 30;
            config.generateAck = generateAck !== false;
            config.mode = 'sink'; // Explicit marker for backend
        } else if (connectivity === 'http') {
            // HTTP/FHIR target
            const endpoint = document.getElementById(`${idPrefix}targetEndpoint`)?.value;
            const deliveryMode = document.getElementById(`${idPrefix}fhirDeliveryMode`)?.value;
            const version = document.getElementById(`${idPrefix}targetVersion`)?.value;
            const format = document.getElementById(`${idPrefix}targetFormat`)?.value;
            // Auth fields use 'target' prefix (e.g., edittargetHttpAuthType)
            const authType = document.getElementById(`${idPrefix}targetHttpAuthType`)?.value;

            if (endpoint) config.endpoint = endpoint;
            if (deliveryMode) config.deliveryMode = deliveryMode;
            if (version) config.version = version;
            if (format) config.format = format;
            if (authType) config.authType = authType;

            // Collect auth details based on type
            if (authType === 'basic') {
                config.username = document.getElementById(`${idPrefix}targetAuthUsername`)?.value;
                config.password = document.getElementById(`${idPrefix}targetAuthPassword`)?.value;
            } else if (authType === 'api_key') {
                config.apiKey = document.getElementById(`${idPrefix}targetAuthApiKeyValue`)?.value;
                config.apiKeyHeader = document.getElementById(`${idPrefix}targetAuthApiKeyHeader`)?.value;
            } else if (authType === 'bearer') {
                config.bearerToken = document.getElementById(`${idPrefix}targetAuthBearerToken`)?.value;
            }
        } else if (connectivity === 'file') {
            // File Writer target
            const directoryPath = document.getElementById(`${idPrefix}targetDirectoryPath`)?.value;
            const filenamePattern = document.getElementById(`${idPrefix}targetFilenamePattern`)?.value;
            const appendTimestamp = document.getElementById(`${idPrefix}targetAppendTimestamp`)?.checked;
            const encoding = document.getElementById(`${idPrefix}targetEncoding`)?.value;

            // Use snake_case for consistency with Go backend
            if (directoryPath) config.directory_path = directoryPath;
            if (filenamePattern) config.filename_pattern = filenamePattern;
            if (appendTimestamp !== undefined) config.append_timestamp = appendTimestamp;
            if (encoding) config.encoding = encoding;

            console.log('📂 File target config collected:', config);
        } else if (connectivity === 'tcp') {
            // TCP/MLLP target
            const host = document.getElementById(`${idPrefix}targetTcpHost`)?.value;
            const port = document.getElementById(`${idPrefix}targetTcpPort`)?.value;
            const protocol = document.getElementById(`${idPrefix}targetTcpProtocol`)?.value;

            if (host) config.host = host;
            if (port) config.port = parseInt(port);
            if (protocol) config.protocol = protocol;
        } else if (connectivity === 'database') {
            // Database target
            const dbType = document.getElementById(`${idPrefix}targetDbType`)?.value;
            const dbHost = document.getElementById(`${idPrefix}targetDbHost`)?.value;
            const dbPort = document.getElementById(`${idPrefix}targetDbPort`)?.value;
            const dbName = document.getElementById(`${idPrefix}targetDbName`)?.value;

            if (dbType) config.dbType = dbType;
            if (dbHost) config.host = dbHost;
            if (dbPort) config.port = parseInt(dbPort);
            if (dbName) config.database = dbName;
        }

        return config;
    }

    /**
     * Validate configuration using object-oriented approach
     */
    validateConfiguration(formData) {
        const errors = [];

        // Basic validation
        if (!formData.name.trim()) {
            errors.push('Interface name is required');
        }

        // Source validation (generic for now)
        if (formData.sourceType === 'tcp' || formData.sourceType === 'mllp') {
            if (!formData.sourceConfig.port || formData.sourceConfig.port < 1 || formData.sourceConfig.port > 65535) {
                errors.push('Valid source port (1-65535) is required for TCP/MLLP');
            }
        }

        // Target validation using specialized handlers
        const targetHandler = window.InterfaceHandlerFactory?.createHandler(formData.targetType, formData);
        if (targetHandler) {
            console.log(`🔧 Using ${targetHandler.constructor.name} for validation`);
            const targetValidation = targetHandler.validateConfiguration();
            if (!targetValidation.isValid) {
                errors.push(...targetValidation.errors);
            }
        } else {
            // Fallback validation
            if (formData.targetType === 'fhir' || formData.targetType === 'http') {
                if (!formData.targetConfig.host?.trim()) {
                    errors.push('Target host/URL is required for FHIR/HTTP targets');
                }
            }
        }

        // Routing validation
        if (formData.processingRules.routingMode === 'hl7-to-fhir') {
            if (!formData.processingRules.targetFhirInterface) {
                errors.push('Target FHIR interface is required for HL7→FHIR routing');
            }
        }

        return errors;
    }
}

// Global functions for modal event handlers
function updateSourceFields() {
    const sourceType = document.getElementById('editSourceType').value;
    const configFields = document.getElementById('sourceConfigFields');

    if (!configFields) return;

    // PRESERVE existing values before regenerating HTML
    const existingHost = document.getElementById('editSourceHost')?.value || '';
    const existingPort = document.getElementById('editSourcePort')?.value || '';

    console.log('🔧 updateSourceFields: Preserving existing values:', {
        sourceType,
        existingHost,
        existingPort
    });

    let fieldsHTML = '';

    switch (sourceType) {
        case 'tcp':
        case 'mllp':
            fieldsHTML = `
                <div class="form-row">
                    <div class="form-group">
                        <label for="editSourceHost" class="field-label-required">Listen Host/IP</label>
                        <input type="text" id="editSourceHost" name="sourceHost" value="${existingHost || '0.0.0.0'}" required>
                        <small class="form-help">Use 0.0.0.0 to listen on all interfaces</small>
                    </div>
                    <div class="form-group">
                        <label for="editSourcePort" class="field-label-required">Listen Port</label>
                        <input type="number" id="editSourcePort" name="sourcePort" value="${existingPort}" placeholder="6661" min="1" max="65535" required>
                        <small class="form-help">Port for incoming HL7 messages</small>
                    </div>
                </div>
            `;
            break;
        case 'http':
            const existingPath = document.getElementById('editSourcePath')?.value || '';
            fieldsHTML = `
                <div class="form-row">
                    <div class="form-group">
                        <label for="editSourceHost">Listen Host/IP</label>
                        <input type="text" id="editSourceHost" name="sourceHost" value="${existingHost || '0.0.0.0'}">
                    </div>
                    <div class="form-group">
                        <label for="editSourcePort" class="field-label-required">Listen Port</label>
                        <input type="number" id="editSourcePort" name="sourcePort" value="${existingPort}" placeholder="8080" min="1" max="65535" required>
                    </div>
                </div>
                <div class="form-group">
                    <label for="editSourcePath">Endpoint Path</label>
                    <input type="text" id="editSourcePath" name="sourcePath" value="${existingPath || '/hl7'}" placeholder="/hl7">
                    <small class="form-help">HTTP path for receiving messages</small>
                </div>
            `;
            break;
        case 'file':
            fieldsHTML = `
                <div class="form-group">
                    <label for="editSourcePath" class="field-label-required">Directory Path</label>
                    <input type="text" id="editSourcePath" name="sourcePath" placeholder="/data/hl7/inbound" required>
                    <small class="form-help">Directory to monitor for incoming files</small>
                </div>
            `;
            break;
        // Note: 'database' case is handled by InterfaceConfigComponents.getSourceConfigPanel()
        // via modal-components.js updateEditSourceConfigPanel() - no duplicate code here
        default:
            fieldsHTML = '<p class="form-help">Configure source-specific settings above</p>';
    }

    configFields.innerHTML = fieldsHTML;
}

function updateTargetFields() {
    const targetType = document.getElementById('editTargetType').value;
    const configFields = document.getElementById('targetConfigFields');

    if (!configFields) return;

    // PRESERVE existing values before regenerating HTML
    const existingFhirUrl = document.getElementById('editFhirServerUrl')?.value || '';
    const existingTargetHost = document.getElementById('editTargetHost')?.value || '';
    const existingTargetPort = document.getElementById('editTargetPort')?.value || '';
    const existingTargetPath = document.getElementById('editTargetPath')?.value || '';
    const existingResourceEndpoint = document.getElementById('editResourceEndpoint')?.value || '';
    const existingDatabaseHost = document.getElementById('editDatabaseHost')?.value || '';
    const existingDatabasePort = document.getElementById('editDatabasePort')?.value || '';
    const existingDatabaseName = document.getElementById('editDatabaseName')?.value || '';
    const existingDatabaseUsername = document.getElementById('editDatabaseUsername')?.value || '';
    const existingDatabaseConnectionString = document.getElementById('editDatabaseConnectionString')?.value || '';
    const existingFileOutputDirectory = document.getElementById('editFileOutputDirectory')?.value || '';
    const existingFileNamePattern = document.getElementById('editFileNamePattern')?.value || '';
    const existingFileFormat = document.getElementById('editFileFormat')?.value || '';

    console.log('🔧 updateTargetFields: Preserving existing values:', {
        targetType,
        existingFhirUrl,
        existingTargetHost,
        existingTargetPort,
        existingTargetPath,
        existingResourceEndpoint
    });

    let fieldsHTML = '';

    switch (targetType) {
        case 'fhir':
            fieldsHTML = `
                <div class="form-row">
                    <div class="form-group">
                        <label for="editFhirServerUrl" class="field-label-required">FHIR Server URL</label>
                        <input type="text" id="editFhirServerUrl" name="fhirServerUrl" value="${existingFhirUrl}" placeholder="http://localhost:8080/fhir" required>
                        <small class="form-help">Full URL to FHIR server base</small>
                    </div>
                    <div class="form-group">
                        <label for="editTargetPort">Port (if different)</label>
                        <input type="number" id="editTargetPort" name="targetPort" value="${existingTargetPort}" placeholder="8080" min="1" max="65535">
                    </div>
                </div>
                <div class="form-group">
                    <label for="editResourceEndpoint">Resource Endpoint</label>
                    <input type="text" id="editResourceEndpoint" name="resourceEndpoint" value="${existingResourceEndpoint || 'Patient'}" placeholder="Patient">
                    <small class="form-help">FHIR resource endpoint (e.g., Patient, Observation)</small>
                </div>
            `;
            break;
        case 'tcp':
        case 'mllp':
            fieldsHTML = `
                <div class="form-row">
                    <div class="form-group">
                        <label for="editTargetHost" class="field-label-required">Target Host</label>
                        <input type="text" id="editTargetHost" name="targetHost" value="${existingTargetHost || 'localhost'}" placeholder="localhost" required>
                    </div>
                    <div class="form-group">
                        <label for="editTargetPort" class="field-label-required">Target Port</label>
                        <input type="number" id="editTargetPort" name="targetPort" value="${existingTargetPort || '6662'}" placeholder="6662" min="1" max="65535" required>
                    </div>
                </div>
            `;
            break;
        case 'http':
            fieldsHTML = `
                <div class="form-group">
                    <label for="editTargetHost" class="field-label-required">Target URL</label>
                    <input type="text" id="editTargetHost" name="targetHost" value="${existingTargetHost}" placeholder="http://target-server:8080/api/messages" required>
                    <small class="form-help">Complete URL for HTTP POST</small>
                </div>
            `;
            break;
        case 'file':
            fieldsHTML = `
                <div class="form-row">
                    <div class="form-group">
                        <label for="editFileOutputDirectory" class="field-label-required">Output Directory</label>
                        <input type="text" id="editFileOutputDirectory" name="outputDirectory" value="${existingFileOutputDirectory || '/data/hl7/outbound'}" placeholder="/data/hl7/outbound" required>
                        <small class="form-help">Directory for output files</small>
                    </div>
                    <div class="form-group">
                        <label for="editFileNamePattern">File Name Pattern</label>
                        <input type="text" id="editFileNamePattern" name="fileNamePattern" value="${existingFileNamePattern || '{timestamp}_{interface}.{ext}'}" placeholder="{timestamp}_{interface}.{ext}">
                        <small class="form-help">Pattern for output file names</small>
                    </div>
                </div>
                <div class="form-group">
                    <label for="editFileFormat">File Format</label>
                    <select id="editFileFormat" name="fileFormat">
                        <option value="json" ${existingFileFormat === 'json' ? 'selected' : ''}>JSON</option>
                        <option value="xml" ${existingFileFormat === 'xml' ? 'selected' : ''}>XML</option>
                        <option value="hl7" ${existingFileFormat === 'hl7' ? 'selected' : ''}>HL7</option>
                        <option value="csv" ${existingFileFormat === 'csv' ? 'selected' : ''}>CSV</option>
                    </select>
                </div>
            `;
            break;
        case 'database':
            fieldsHTML = `
                <div class="form-group">
                    <label for="editDatabaseConnectionString">Connection String (Optional)</label>
                    <input type="text" id="editDatabaseConnectionString" name="connectionString" value="${existingDatabaseConnectionString}" placeholder="postgresql://user:pass@localhost:5432/dbname">
                    <small class="form-help">Complete database connection string (leave empty to use individual fields)</small>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="editDatabaseHost" class="field-label-required">Database Host</label>
                        <input type="text" id="editDatabaseHost" name="databaseHost" value="${existingDatabaseHost || 'localhost'}" placeholder="localhost" required>
                    </div>
                    <div class="form-group">
                        <label for="editDatabasePort">Database Port</label>
                        <input type="number" id="editDatabasePort" name="databasePort" value="${existingDatabasePort || '5432'}" placeholder="5432" min="1" max="65535">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="editDatabaseName" class="field-label-required">Database Name</label>
                        <input type="text" id="editDatabaseName" name="databaseName" value="${existingDatabaseName}" placeholder="database_name" required>
                    </div>
                    <div class="form-group">
                        <label for="editDatabaseUsername">Username</label>
                        <input type="text" id="editDatabaseUsername" name="databaseUsername" value="${existingDatabaseUsername}" placeholder="username">
                    </div>
                </div>
            `;
            break;
        case 'sink':
            fieldsHTML = `
                <div class="form-group">
                    <div class="info-panel">
                        <div class="info-icon">💾</div>
                        <div class="info-content">
                            <h4>Sink Target Configuration</h4>
                            <p>This interface will receive and store all messages in the database with no forwarding to external systems.</p>
                            <ul>
                                <li>Messages are logged for audit purposes</li>
                                <li>No external network connections required</li>
                                <li>Ideal for monitoring and compliance tracking</li>
                            </ul>
                        </div>
                    </div>
                </div>
            `;
            break;
        default:
            fieldsHTML = '<p class="form-help">Configure target-specific settings above</p>';
    }

    configFields.innerHTML = fieldsHTML;
}

function updateRoutingFields() {
    const routingMode = document.getElementById('editRoutingMode').value;
    const hl7ToFhirConfig = document.getElementById('hl7ToFhirConfig');

    if (!hl7ToFhirConfig) return;

    if (routingMode === 'hl7-to-fhir') {
        hl7ToFhirConfig.style.display = 'block';
        // Populate FHIR interfaces if not already done
        if (window.interfaceConfigManager) {
            window.interfaceConfigManager.populateFhirInterfaceOptions();
        }
    } else {
        hl7ToFhirConfig.style.display = 'none';
    }
}

function testInterfaceConfiguration() {
    const formData = window.interfaceConfigManager ?
        window.interfaceConfigManager.collectFormData() :
        { name: document.getElementById('editInterfaceName').value };

    const errors = window.interfaceConfigManager ?
        window.interfaceConfigManager.validateConfiguration(formData) :
        [];

    if (errors.length > 0) {
        AppDialogs.toast('Configuration errors: ' + errors.join('; '), 'error');
        return;
    }

    AppDialogs.alert(
        `<b>Interface:</b> ${formData.name}<br>` +
        `<b>Source:</b> ${formData.sourceType} (${formData.sourceConnectivity})<br>` +
        `<b>Target:</b> ${formData.targetType} (${formData.targetConnectivity})<br>` +
        `<b>Routing:</b> ${formData.processingRules?.routingMode || 'direct'}<br><br>` +
        'In production, this would test connectivity and validate settings.',
        { title: '🧪 Configuration Test', type: 'info' });
}

// Initialize the configuration manager when DOM is ready
function initializeConfigManager() {
    console.log('🔧 Initializing InterfaceConfigManager...');
    window.basicInterfaceConfigManager = new BasicInterfaceConfigManager();
    window.basicInterfaceConfigManager.init();

    // CRITICAL FIX: Also expose as interfaceConfigManager for interfaces.js compatibility
    window.interfaceConfigManager = window.basicInterfaceConfigManager;

    console.log('✅ InterfaceConfigManager initialized and exposed as window.interfaceConfigManager');
}

// Make class available globally (after class definition)
window.BasicInterfaceConfigManager = BasicInterfaceConfigManager;
console.log('✅ InterfaceConfigManager class exposed globally');

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeConfigManager);
} else {
    // DOM already loaded
    initializeConfigManager();
}
// interface-config-manager.js
// Enhanced interface configuration management for comprehensive editing

class BasicInterfaceConfigManager {
    constructor() {
        this.availableInterfaces = [];
        this.currentInterface = null;
    }

    /**
     * Initialize the config manager
     */
    async init() {
        await this.loadAvailableInterfaces();
        this.setupDynamicFieldHandlers();
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

        // Source configuration
        setFieldValue('editSourceType', interfaceData.sourceType, 'tcp');
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

        try {
            // Source configuration
            const sourceConfig = typeof interfaceData.source_config === 'string'
                ? JSON.parse(interfaceData.source_config)
                : interfaceData.source_config || interfaceData.sourceConfig || {};

            console.log('📋 Source config:', sourceConfig);

            this.setFieldIfExists('editSourceHost', sourceConfig.host);
            this.setFieldIfExists('editSourcePort', sourceConfig.port);

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

            // Target configuration - handle multiple possible field names
            let targetConfig = interfaceData.target_config || interfaceData.targetConfig;
            if (typeof targetConfig === 'string') {
                targetConfig = JSON.parse(targetConfig);
            }
            targetConfig = targetConfig || {};

            console.log('🎯 Target config:', targetConfig);

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
     */
    collectFormData() {
        const formData = {
            id: document.getElementById('editInterfaceId').value,
            name: document.getElementById('editInterfaceName').value,
            description: document.getElementById('editInterfaceDescription').value,
            format: document.getElementById('editFormat').value,
            messageType: document.getElementById('editFormat').value, // Map format to messageType for backend compatibility
            status: document.getElementById('editStatus').value,
            sourceType: document.getElementById('editSourceType').value,
            sourceConnectivity: document.getElementById('editSourceConnectivity').value,
            targetType: document.getElementById('editTargetType').value,
            targetConnectivity: document.getElementById('editTargetConnectivity').value
        };

        // Collect source configuration
        formData.sourceConfig = {
            host: document.getElementById('editSourceHost')?.value || 'localhost',
            port: parseInt(document.getElementById('editSourcePort')?.value) || 6661
        };

        // Collect target configuration using object-oriented approach
        const targetType = formData.targetType;

        // Use specialized handler for target type
        const targetHandler = window.InterfaceHandlerFactory?.createHandler(targetType, formData);

        if (targetHandler) {
            console.log(`🔧 Using ${targetHandler.constructor.name} for target configuration`);
            formData.targetConfig = targetHandler.collectTargetConfig();
        } else {
            console.warn('⚠️ Interface handlers not loaded, using fallback logic');
            // Fallback for when handlers aren't loaded yet
            formData.targetConfig = {
                host: document.getElementById('editTargetHost')?.value || '',
                port: parseInt(document.getElementById('editTargetPort')?.value) || 8080,
                path: document.getElementById('editTargetPath')?.value || '/fhir'
            };
        }

        // Collect processing rules
        formData.processingRules = {
            routingMode: document.getElementById('editRoutingMode')?.value || 'direct',
            targetFhirInterface: document.getElementById('editTargetFhirInterface')?.value || '',
            transformationEngine: document.getElementById('editTransformationEngine')?.value || 'go-engine',
            retryPolicy: document.getElementById('editRetryPolicy')?.value || '3'
        };

        // Performance settings
        const tableStrategy = document.getElementById('editTableStrategy')?.value;
        formData.useDedicatedTable = tableStrategy === 'dedicated';
        formData.tableManagementStrategy = tableStrategy || 'shared';
        formData.expectedVolume = document.getElementById('editExpectedVolume')?.value || 'low';

        return formData;
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
        alert('Configuration errors:\n' + errors.join('\n'));
        return;
    }

    alert('🧪 Configuration test would be performed here:\n\n' +
          `Interface: ${formData.name}\n` +
          `Source: ${formData.sourceType} (${formData.sourceConnectivity})\n` +
          `Target: ${formData.targetType} (${formData.targetConnectivity})\n` +
          `Routing: ${formData.processingRules?.routingMode || 'direct'}\n\n` +
          'In production, this would test connectivity and validate settings.');
}

// Initialize the configuration manager when DOM is ready
function initializeConfigManager() {
    console.log('🔧 Initializing InterfaceConfigManager...');
    window.basicInterfaceConfigManager = new BasicInterfaceConfigManager();
    window.basicInterfaceConfigManager.init();
    console.log('✅ InterfaceConfigManager initialized');
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
// services/wizardConfigService.js
// Node.js service for handling wizard configuration with format + connectivity architecture
const axios = require('axios');
const config = require('../config');

class WizardConfigService {
    constructor() {
        this.goBackendUrl = config.GO_BACKEND_URL || 'http://localhost:8080';
        this.apiClient = axios.create({
            baseURL: this.goBackendUrl,
            timeout: 30000,
            headers: {
                'Content-Type': 'application/json'
            }
        });
    }

    /**
     * Save interface configuration after Step 4 completion
     * @param {Object} wizardData - Complete wizard data from steps 1-4 (already mapped by controller)
     * @param {string} userId - Current user ID
     * @returns {Promise<Object>} Interface creation result
     */
    async saveInterfaceConfiguration(wizardData, userId) {
        try {
            console.log('🔄 Saving interface configuration to Go backend...');
            console.log('🔧 Wizard data received:', {
                sourceType: wizardData.sourceType,
                targetType: wizardData.targetType,
                sourceConnectivity: wizardData.sourceConnectivity,
                targetConnectivity: wizardData.targetConnectivity,
                status: wizardData.status
            });
            
            // Prepare configuration payload for Go backend
            const configPayload = this.prepareConfigurationPayload(wizardData, userId);
            
            // Validate payload before sending
            this.validateConfigurationPayload(configPayload);
            
            console.log('📤 Sending to Go backend:', configPayload);
            
            // Send to Go backend API
            const response = await this.apiClient.post('/api/wizard/interfaces', configPayload);
            
            if (response.data.success) {
                console.log('✅ Interface configuration saved successfully:', response.data.data.id);
                return {
                    success: true,
                    interfaceId: response.data.data.id,
                    interface: response.data.data
                };
            } else {
                throw new Error(response.data.error || 'Unknown error from Go backend');
            }
            
        } catch (error) {
            console.error('❌ Failed to save interface configuration:', error.message);
            
            if (error.response) {
                // HTTP error from Go backend
                throw new Error(`Backend error (${error.response.status}): ${error.response.data || error.message}`);
            } else if (error.request) {
                // Network error
                throw new Error('Unable to connect to backend service');
            } else {
                // Other error
                throw error;
            }
        }
    }

    /**
     * Complete wizard (save and activate interface in one call)
     * @param {Object} wizardData - Complete wizard data from steps 1-4 (already mapped by controller)
     * @param {string} userId - Current user ID
     * @returns {Promise<Object>} Interface creation and activation result
     */
    async completeWizard(wizardData, userId) {
        try {
            console.log('🏁 Completing wizard (save + activate) for user:', userId);
            
            // First save the interface configuration
            const saveResult = await this.saveInterfaceConfiguration(wizardData, userId);
            
            if (!saveResult.success || !saveResult.interfaceId) {
                throw new Error('Failed to save interface configuration');
            }
            
            // Then activate the interface
            const activationResult = await this.activateInterface(saveResult.interfaceId, userId);
            
            if (!activationResult.success) {
                throw new Error('Interface saved but activation failed');
            }
            
            console.log('✅ Wizard completed successfully - interface created and activated');
            
            return {
                success: true,
                interfaceId: saveResult.interfaceId,
                interface: saveResult.interface,
                message: 'Interface created and activated successfully'
            };
            
        } catch (error) {
            console.error('❌ Failed to complete wizard:', error.message);
            throw error;
        }
    }

    /**
     * Prepare configuration payload for Go backend
     * @param {Object} wizardData - Mapped wizard data (controller already applied format + connectivity mapping)
     * @param {string} userId - User ID
     * @returns {Object} Formatted payload for Go API
     */
    prepareConfigurationPayload(wizardData, userId) {
        return {
            // Basic configuration (Step 1) - already mapped by controller
            userId: userId,
            name: wizardData.name,
            description: wizardData.description || '',
            
            // Format types (already mapped from UI types)
            sourceType: wizardData.sourceType,        // e.g., 'hl7', 'fhir', 'database'
            targetType: wizardData.targetType,        // e.g., 'fhir', 'database', 'flatfile'
            
            // Connectivity types (already mapped from UI types)
            sourceConnectivity: wizardData.sourceConnectivity,  // e.g., 'tcp', 'http', 'file'
            targetConnectivity: wizardData.targetConnectivity,  // e.g., 'http', 'database', 'file'
            
            // Source configuration (Step 2)
            sourceConfig: this.prepareSourceConfig(wizardData.sourceConnectivity, wizardData.sourceSettings),
            
            // Target configuration (Step 3)
            targetConfig: this.prepareTargetConfig(wizardData.targetConnectivity, wizardData.targetSettings),
            
            // Transformation configuration (Step 4)
            transformationMapping: this.prepareTransformationMapping(wizardData.mappingConfiguration),
            
            // Processing rules and options
            processingRules: this.prepareProcessingRules(wizardData),
            
            // Interface status
            status: wizardData.status || 'draft',
            
            // Timestamps
            createdAt: new Date().toISOString(),
            createdBy: userId
        };
    }

    /**
     * Prepare source configuration based on connectivity type
     * @param {string} sourceConnectivity - Connectivity type ('tcp', 'http', 'file', etc.)
     * @param {Object} sourceConfig - Raw source configuration from wizard
     * @returns {Object} Formatted source configuration
     */
    prepareSourceConfig(sourceConnectivity, sourceConfig = {}) {
        switch (sourceConnectivity) {
            case 'tcp':
                return {
                    type: 'tcp_listener',
                    tcp: {
                        host: sourceConfig.host || '0.0.0.0',
                        port: parseInt(sourceConfig.port) || 2575,
                        encoding: sourceConfig.encoding || 'utf-8',
                        timeout: parseInt(sourceConfig.timeout) || 30000
                    }
                };
                
            case 'http':
                return {
                    type: 'http_endpoint',
                    http: {
                        endpointUrl: sourceConfig.path || '/api/hl7',
                        method: sourceConfig.method || 'POST',
                        timeout: parseInt(sourceConfig.timeout) || 30000,
                        headers: {
                            'Content-Type': 'application/hl7-v2',
                            'Accept': 'application/json'
                        }
                    }
                };
                
            case 'file':
                return {
                    type: 'file_watcher',
                    file: {
                        inputPath: sourceConfig.path || '/app/input',
                        filePattern: sourceConfig.pattern || '*.hl7',
                        pollInterval: parseInt(sourceConfig.pollInterval) || 5000,
                        deleteAfterProcessing: sourceConfig.deleteAfterProcessing !== false
                    }
                };
                
            case 'database':
                return {
                    type: 'database_poller',
                    database: {
                        connectionString: sourceConfig.connectionString,
                        tableName: sourceConfig.table || 'hl7_messages',
                        pollInterval: parseInt(sourceConfig.pollInterval) || 30000,
                        query: sourceConfig.query || 'SELECT * FROM hl7_messages WHERE processed = 0'
                    }
                };
                
            default:
                console.warn(`Unknown source connectivity: ${sourceConnectivity}, defaulting to tcp`);
                return {
                    type: 'tcp_listener',
                    tcp: {
                        host: '0.0.0.0',
                        port: 2575,
                        encoding: 'utf-8',
                        timeout: 30000
                    }
                };
        }
    }

    /**
     * Prepare target configuration based on connectivity type
     * @param {string} targetConnectivity - Connectivity type ('http', 'database', 'file', etc.)
     * @param {Object} targetConfig - Raw target configuration from wizard
     * @returns {Object} Formatted target configuration
     */
    prepareTargetConfig(targetConnectivity, targetConfig = {}) {
        switch (targetConnectivity) {
            case 'http':
                return {
                    type: 'http_client',
                    http: {
                        endpointUrl: targetConfig.host ? 
                            `${targetConfig.ssl ? 'https' : 'http'}://${targetConfig.host}${targetConfig.port ? ':' + targetConfig.port : ''}${targetConfig.path || ''}` :
                            'https://hapi.fhir.org/baseR4',
                        method: 'POST',
                        timeout: parseInt(targetConfig.timeout) || 30000,
                        headers: {
                            'Content-Type': 'application/fhir+json',
                            'Accept': 'application/fhir+json'
                        },
                        auth: targetConfig.username && targetConfig.password ? {
                            username: targetConfig.username,
                            password: targetConfig.password,
                            method: targetConfig.authMethod || 'basic'
                        } : null
                    }
                };
                
            case 'database':
                return {
                    type: 'database_writer',
                    database: {
                        connectionString: targetConfig.connectionString || 
                            this.buildConnectionString(targetConfig),
                        tableName: targetConfig.table || 'fhir_resources',
                        batchSize: parseInt(targetConfig.batchSize) || 100,
                        timeout: parseInt(targetConfig.timeout) || 30000
                    }
                };
                
            case 'file':
                return {
                    type: 'file_writer',
                    file: {
                        outputPath: targetConfig.path || '/app/output',
                        fileNamePattern: targetConfig.pattern || 'fhir_{timestamp}.json',
                        createDirectories: targetConfig.createDirectories !== false,
                        encoding: targetConfig.encoding || 'utf-8'
                    }
                };
                
            case 'tcp':
                return {
                    type: 'tcp_client',
                    tcp: {
                        host: targetConfig.host || 'localhost',
                        port: parseInt(targetConfig.port) || 2576,
                        encoding: 'utf-8',
                        timeout: 30000
                    }
                };
            
            default:
                console.warn(`Unknown target connectivity: ${targetConnectivity}, defaulting to http`);
                return {
                    type: 'http_endpoint',
                    http: {
                        endpointUrl: 'https://hapi.fhir.org/baseR4',
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/fhir+json',
                            'Accept': 'application/fhir+json'
                        }
                    }
                };
        }
    }

    /**
     * Prepare transformation mapping configuration
     * @param {Object} mappingConfig - Mapping configuration from Step 4
     * @returns {Object} Formatted transformation mapping
     */
    prepareTransformationMapping(mappingConfig = {}) {
        return {
            version: '1.0',
            messageType: mappingConfig.messageType || '',
            hl7Version: mappingConfig.hl7Version || '2.5.1',
            fhirVersion: mappingConfig.fhirVersion || 'R4',
            fhirProfile: mappingConfig.fhirProfile || 'base',
            mappingRuleIds: mappingConfig.mappingRuleIds || [],
            resourceOverrides: mappingConfig.resourceOverrides || {},
            validationEnabled: mappingConfig.validationEnabled !== false,
            createBundle: mappingConfig.createBundle || false,
            createdAt: new Date().toISOString()
        };
    }

    /**
     * Prepare processing rules and options
     * @param {Object} wizardData - Complete wizard data
     * @returns {Object} Processing rules configuration
     */
    prepareProcessingRules(wizardData) {
        return {
            retryEnabled: wizardData.retryEnabled || false,
            maxRetries: parseInt(wizardData.maxRetries) || 3,
            retryDelay: parseInt(wizardData.retryDelay) || 1000,
            errorHandling: wizardData.errorHandling || 'log',
            validationLevel: wizardData.validationLevel || 'basic',
            asyncProcessing: wizardData.asyncProcessing || false,
            batchSize: parseInt(wizardData.batchSize) || 1,
            deadLetterQueue: wizardData.deadLetterQueue || false
        };
    }

    /**
     * Build database connection string from individual components
     * @param {Object} config - Database configuration
     * @returns {string} Connection string
     */
    buildConnectionString(config) {
        if (config.connectionString) {
            return config.connectionString;
        }
        
        // Build PostgreSQL connection string
        const parts = [];
        if (config.host) parts.push(`host=${config.host}`);
        if (config.port) parts.push(`port=${config.port}`);
        if (config.database) parts.push(`dbname=${config.database}`);
        if (config.username) parts.push(`user=${config.username}`);
        if (config.password) parts.push(`password=${config.password}`);
        if (config.ssl) parts.push(`sslmode=require`);
        
        return parts.join(' ');
    }

    /**
     * Validate configuration payload before sending to Go backend
     */
    validateConfigurationPayload(payload) {
        const required = ['userId', 'name', 'sourceType', 'targetType', 'sourceConnectivity', 'targetConnectivity'];
        
        for (const field of required) {
            if (!payload[field] || (typeof payload[field] === 'string' && payload[field].trim() === '')) {
                throw new Error(`Missing required field: ${field}`);
            }
        }
        
        // Validate format types
        const validFormats = ['hl7', 'fhir', 'database', 'flatfile', 'xml', 'json', 'edi', 'csv'];
        if (!validFormats.includes(payload.sourceType)) {
            throw new Error(`Invalid source format: ${payload.sourceType}. Must be one of: ${validFormats.join(', ')}`);
        }
        if (!validFormats.includes(payload.targetType)) {
            throw new Error(`Invalid target format: ${payload.targetType}. Must be one of: ${validFormats.join(', ')}`);
        }
        
        // Validate connectivity types
        const validConnectivity = ['tcp', 'http', 'file', 'database', 'sftp', 'mq', 'api'];
        if (!validConnectivity.includes(payload.sourceConnectivity)) {
            throw new Error(`Invalid source connectivity: ${payload.sourceConnectivity}. Must be one of: ${validConnectivity.join(', ')}`);
        }
        if (!validConnectivity.includes(payload.targetConnectivity)) {
            throw new Error(`Invalid target connectivity: ${payload.targetConnectivity}. Must be one of: ${validConnectivity.join(', ')}`);
        }
        
        // Validate source configuration
        if (!payload.sourceConfig || Object.keys(payload.sourceConfig).length === 0) {
            throw new Error('Source configuration is required');
        }
        
        // Validate target configuration
        if (!payload.targetConfig || Object.keys(payload.targetConfig).length === 0) {
            throw new Error('Target configuration is required');
        }
        
        console.log('✅ Configuration payload validation passed');
        console.log('🔧 Validated payload:', {
            formats: `${payload.sourceType} → ${payload.targetType}`,
            connectivity: `${payload.sourceConnectivity} → ${payload.targetConnectivity}`,
            status: payload.status
        });
    }

    /**
     * Activate interface after wizard completion
     * @param {string} interfaceId - Interface ID to activate
     * @param {string} userId - User performing activation
     * @returns {Promise<Object>} Activation result
     */
    async activateInterface(interfaceId, userId) {
        try {
            console.log(`🔄 Activating interface: ${interfaceId}`);
            
            const response = await this.apiClient.post(`/api/interfaces/${interfaceId}/activate`, {
                activatedBy: userId
            });
            
            if (response.data.success) {
                console.log('✅ Interface activated successfully');
                return { success: true, message: response.data.message };
            } else {
                throw new Error(response.data.message || 'Activation failed');
            }
            
        } catch (error) {
            console.error('❌ Failed to activate interface:', error.message);
            throw error;
        }
    }

    /**
     * Get interface details
     * @param {string} interfaceId - Interface ID
     * @returns {Promise<Object>} Interface details
     */
    async getInterface(interfaceId) {
        try {
            const response = await this.apiClient.get(`/api/interfaces/${interfaceId}`);
            
            if (response.data.success) {
                return response.data.data;
            } else {
                throw new Error(response.data.error || 'Failed to get interface');
            }
            
        } catch (error) {
            console.error('❌ Failed to get interface:', error.message);
            throw error;
        }
    }

    /**
     * List user interfaces
     * @param {string} userId - User ID
     * @returns {Promise<Array>} List of user interfaces
     */
    async listUserInterfaces(userId) {
        try {
            const response = await this.apiClient.get(`/api/interfaces?userId=${userId}`);
            
            if (response.data.success) {
                return response.data.data;
            } else {
                throw new Error(response.data.error || 'Failed to list interfaces');
            }
            
        } catch (error) {
            console.error('❌ Failed to list interfaces:', error.message);
            throw error;
        }
    }

    /**
     * Update interface status
     * @param {string} interfaceId - Interface ID
     * @param {string} status - New status ('draft', 'running', 'stopped', 'paused', 'error')
     * @param {string} userId - User performing update
     * @returns {Promise<Object>} Update result
     */
    async updateInterfaceStatus(interfaceId, status, userId) {
        try {
            console.log(`🔄 Updating interface ${interfaceId} status to ${status}`);
            
            const response = await this.apiClient.put(`/api/interfaces/${interfaceId}/status`, {
                status: status,
                updatedBy: userId
            });
            
            if (response.data.success) {
                console.log('✅ Interface status updated successfully');
                return { success: true, message: response.data.message };
            } else {
                throw new Error(response.data.message || 'Status update failed');
            }
            
        } catch (error) {
            console.error('❌ Failed to update interface status:', error.message);
            throw error;
        }
    }

    /**
     * Delete interface
     * @param {string} interfaceId - Interface ID to delete
     * @param {string} userId - User performing deletion
     * @returns {Promise<Object>} Deletion result
     */
    async deleteInterface(interfaceId, userId) {
        try {
            console.log(`🗑️ Deleting interface: ${interfaceId}`);
            
            const response = await this.apiClient.delete(`/api/interfaces/${interfaceId}`, {
                data: { deletedBy: userId }
            });
            
            if (response.data.success) {
                console.log('✅ Interface deleted successfully');
                return { success: true, message: response.data.message };
            } else {
                throw new Error(response.data.message || 'Deletion failed');
            }
            
        } catch (error) {
            console.error('❌ Failed to delete interface:', error.message);
            throw error;
        }
    }

    /**
     * Check if interface name is duplicate
     * @param {string} name - Interface name to check
     * @param {string} userId - User ID
     * @param {string} excludeId - Interface ID to exclude from check (for updates)
     * @returns {Promise<Object>} Duplicate check result
     */
    async checkDuplicateName(name, userId, excludeId = null) {
        try {
            const params = new URLSearchParams({ name, userId });
            if (excludeId) {
                params.append('excludeId', excludeId);
            }
            
            const response = await this.apiClient.get(`/api/interfaces/check-duplicate?${params}`);
            
            return {
                success: true,
                isDuplicate: response.data.isDuplicate,
                message: response.data.message
            };
            
        } catch (error) {
            console.error('❌ Failed to check duplicate name:', error.message);
            throw error;
        }
    }

    /**
     * Get interface statistics
     * @param {string} interfaceId - Interface ID
     * @returns {Promise<Object>} Interface statistics
     */
    async getInterfaceStats(interfaceId) {
        try {
            const response = await this.apiClient.get(`/api/interfaces/${interfaceId}/stats`);
            
            if (response.data.success) {
                return response.data.data;
            } else {
                throw new Error(response.data.error || 'Failed to get interface stats');
            }
            
        } catch (error) {
            console.error('❌ Failed to get interface stats:', error.message);
            throw error;
        }
    }
}

module.exports = WizardConfigService;
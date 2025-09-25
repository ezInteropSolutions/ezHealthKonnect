// interface-handlers.js - Object-Oriented Interface Type Handlers
// Provides specialized handling for different interface types (FHIR, HL7, etc.)

/**
 * Base Interface Handler - Abstract class for all interface types
 */
class BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        this.interfaceData = interfaceData;
        this.type = interfaceData.targetType || 'generic';
    }

    // Default implementations - can be overridden by subclasses
    collectTargetConfig() {
        console.warn(`⚠️ ${this.constructor.name}: Using default target config collection`);
        return {
            type: this.type,
            host: this.getFieldValue('editTargetHost', 'localhost'),
            port: parseInt(this.getFieldValue('editTargetPort', '8080')),
            path: this.getFieldValue('editTargetPath', '/api')
        };
    }

    populateTargetFields(targetConfig) {
        console.warn(`⚠️ ${this.constructor.name}: Using default target field population`);
        this.setFieldValue('editTargetHost', targetConfig.host);
        this.setFieldValue('editTargetPort', targetConfig.port);
        this.setFieldValue('editTargetPath', targetConfig.path);
    }

    validateConfiguration() {
        return { isValid: true, errors: [] };
    }

    // Common helper methods
    setFieldValue(fieldId, value) {
        const field = document.getElementById(fieldId);
        if (field && value !== null && value !== undefined) {
            field.value = value;
            console.log(`✅ ${this.constructor.name}: Set ${fieldId} = ${value}`);
            return true;
        }
        return false;
    }

    getFieldValue(fieldId, defaultValue = '') {
        const field = document.getElementById(fieldId);
        return field?.value || defaultValue;
    }
}

/**
 * FHIR Interface Handler - Specialized for FHIR server interactions
 */
class FhirInterfaceHandler extends BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        super(interfaceData);
        this.type = 'fhir';
    }

    /**
     * Collect FHIR-specific target configuration from form
     */
    collectTargetConfig() {
        const fhirServerUrl = this.getFieldValue('editFhirServerUrl');
        const resourceEndpoint = this.getFieldValue('editResourceEndpoint', 'Patient');
        const targetPort = this.getFieldValue('editTargetPort');

        console.log('🔍 FHIR Handler: Collecting config from form');
        console.log('  - FHIR Server URL:', fhirServerUrl);
        console.log('  - Resource Endpoint:', resourceEndpoint);

        // Parse FHIR Server URL to extract components
        let protocol = 'http', host = 'localhost', port = 8080;

        if (fhirServerUrl) {
            try {
                const url = new URL(fhirServerUrl);
                protocol = url.protocol.replace(':', '');
                host = url.hostname;
                port = parseInt(url.port) || (protocol === 'https' ? 443 : 80);
            } catch (e) {
                console.warn('🚨 FHIR Handler: Invalid server URL, using defaults');
                // Keep defaults
            }
        }

        // Override port if explicitly set
        if (targetPort) {
            port = parseInt(targetPort);
        }

        const config = {
            type: 'fhir',
            protocol,
            host,
            port,
            path: `/${resourceEndpoint.replace(/^\//, '')}`,
            fhirServerUrl, // Store original URL
            resourceEndpoint,

            // FHIR-specific properties
            acceptHeader: 'application/fhir+json',
            contentType: 'application/fhir+json',
            apiVersion: 'R4'
        };

        console.log('✅ FHIR Handler: Collected config:', config);
        return config;
    }

    /**
     * Populate FHIR-specific form fields from saved configuration
     */
    populateTargetFields(targetConfig) {
        console.log('🔍 FHIR Handler: Populating fields from config:', targetConfig);

        // Reconstruct FHIR Server URL if not stored directly
        let fhirServerUrl = targetConfig.fhirServerUrl;

        if (!fhirServerUrl && targetConfig.host) {
            const protocol = targetConfig.protocol || 'http';
            const host = targetConfig.host;
            const port = targetConfig.port || (protocol === 'https' ? 443 : 80);

            // Only include port if it's not the default
            const needsPort = (protocol === 'http' && port !== 80) ||
                              (protocol === 'https' && port !== 443);

            fhirServerUrl = needsPort ?
                `${protocol}://${host}:${port}/fhir` :
                `${protocol}://${host}/fhir`;
        }

        // Set form fields
        this.setFieldValue('editFhirServerUrl', fhirServerUrl);
        this.setFieldValue('editTargetPort', targetConfig.port);

        // Extract resource endpoint from path
        let resourceEndpoint = 'Patient';
        if (targetConfig.path) {
            resourceEndpoint = targetConfig.path
                .replace(/^\/+/, '')
                .replace(/^fhir\/?/i, '')
                || 'Patient';
        } else if (targetConfig.resourceEndpoint) {
            resourceEndpoint = targetConfig.resourceEndpoint;
        }

        this.setFieldValue('editResourceEndpoint', resourceEndpoint);

        console.log('✅ FHIR Handler: Fields populated successfully');
    }

    /**
     * Validate FHIR-specific configuration
     */
    validateConfiguration() {
        const errors = [];
        const fhirServerUrl = this.getFieldValue('editFhirServerUrl');

        if (!fhirServerUrl.trim()) {
            errors.push('FHIR Server URL is required');
        } else {
            try {
                new URL(fhirServerUrl);
            } catch (e) {
                errors.push('FHIR Server URL must be a valid URL');
            }
        }

        const resourceEndpoint = this.getFieldValue('editResourceEndpoint');
        if (!resourceEndpoint.trim()) {
            errors.push('Resource Endpoint is required');
        }

        return {
            isValid: errors.length === 0,
            errors
        };
    }
}

/**
 * HL7 Interface Handler - Specialized for HL7 v2.x interactions
 */
class Hl7InterfaceHandler extends BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        super(interfaceData);
        this.type = 'hl7';
    }

    collectTargetConfig() {
        const config = {
            type: 'hl7',
            host: this.getFieldValue('editTargetHost', 'localhost'),
            port: parseInt(this.getFieldValue('editTargetPort', '6662')),
            protocol: 'mllp', // HL7 typically uses MLLP

            // HL7-specific properties
            startOfBlock: '\x0b', // MLLP start
            endOfBlock: '\x1c\x0d', // MLLP end
            acknowledgmentMode: 'immediate'
        };

        console.log('✅ HL7 Handler: Collected config:', config);
        return config;
    }

    populateTargetFields(targetConfig) {
        console.log('🔍 HL7 Handler: Populating fields from config:', targetConfig);

        this.setFieldValue('editTargetHost', targetConfig.host);
        this.setFieldValue('editTargetPort', targetConfig.port);

        console.log('✅ HL7 Handler: Fields populated successfully');
    }

    validateConfiguration() {
        const errors = [];
        const host = this.getFieldValue('editTargetHost');
        const port = this.getFieldValue('editTargetPort');

        if (!host.trim()) {
            errors.push('Target host is required for HL7 interfaces');
        }

        if (!port || isNaN(parseInt(port)) || parseInt(port) < 1 || parseInt(port) > 65535) {
            errors.push('Valid port number (1-65535) is required for HL7 interfaces');
        }

        return {
            isValid: errors.length === 0,
            errors
        };
    }
}

/**
 * Database Interface Handler - Specialized for database targets
 */
class DatabaseInterfaceHandler extends BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        super(interfaceData);
        this.type = 'database';
    }

    collectTargetConfig() {
        const config = {
            type: 'database',
            connectionString: this.getFieldValue('editDatabaseConnectionString'),
            host: this.getFieldValue('editDatabaseHost', 'localhost'),
            port: parseInt(this.getFieldValue('editDatabasePort', '5432')),
            database: this.getFieldValue('editDatabaseName'),
            username: this.getFieldValue('editDatabaseUsername'),

            // Database-specific properties
            poolSize: 10,
            connectionTimeout: 30000,
            ssl: false
        };

        console.log('✅ Database Handler: Collected config:', config);
        return config;
    }

    populateTargetFields(targetConfig) {
        console.log('🔍 Database Handler: Populating fields from config:', targetConfig);

        this.setFieldValue('editDatabaseConnectionString', targetConfig.connectionString);
        this.setFieldValue('editDatabaseHost', targetConfig.host);
        this.setFieldValue('editDatabasePort', targetConfig.port);
        this.setFieldValue('editDatabaseName', targetConfig.database);
        this.setFieldValue('editDatabaseUsername', targetConfig.username);

        console.log('✅ Database Handler: Fields populated successfully');
    }

    validateConfiguration() {
        const errors = [];
        const connectionString = this.getFieldValue('editDatabaseConnectionString');
        const host = this.getFieldValue('editDatabaseHost');
        const database = this.getFieldValue('editDatabaseName');

        if (!connectionString.trim() && !host.trim()) {
            errors.push('Either connection string or host is required for database interfaces');
        }

        if (!database.trim()) {
            errors.push('Database name is required');
        }

        return {
            isValid: errors.length === 0,
            errors
        };
    }
}

/**
 * File Interface Handler - Specialized for file system targets
 */
class FileInterfaceHandler extends BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        super(interfaceData);
        this.type = 'file';
    }

    collectTargetConfig() {
        const config = {
            type: 'file',
            outputDirectory: this.getFieldValue('editFileOutputDirectory', '/data/output'),
            fileNamePattern: this.getFieldValue('editFileNamePattern', '{timestamp}_{interface}.{ext}'),
            fileFormat: this.getFieldValue('editFileFormat', 'json'),

            // File-specific properties
            createDirectories: true,
            overwriteExisting: false,
            archiveProcessed: true
        };

        console.log('✅ File Handler: Collected config:', config);
        return config;
    }

    populateTargetFields(targetConfig) {
        console.log('🔍 File Handler: Populating fields from config:', targetConfig);

        this.setFieldValue('editFileOutputDirectory', targetConfig.outputDirectory);
        this.setFieldValue('editFileNamePattern', targetConfig.fileNamePattern);
        this.setFieldValue('editFileFormat', targetConfig.fileFormat);

        console.log('✅ File Handler: Fields populated successfully');
    }

    validateConfiguration() {
        const errors = [];
        const outputDirectory = this.getFieldValue('editFileOutputDirectory');

        if (!outputDirectory.trim()) {
            errors.push('Output directory is required for file interfaces');
        }

        return {
            isValid: errors.length === 0,
            errors
        };
    }
}

/**
 * Sink Interface Handler - Specialized for logging with no forwarding
 */
class SinkInterfaceHandler extends BaseInterfaceHandler {
    constructor(interfaceData = {}) {
        super(interfaceData);
        this.type = 'sink';
    }

    collectTargetConfig() {
        const config = {
            type: 'sink',

            // Sink-specific properties
            loggingOnly: true,
            enableForwarding: false,
            auditLevel: 'full',
            retentionDays: 365
        };

        console.log('✅ Sink Handler: Collected config:', config);
        return config;
    }

    populateTargetFields(targetConfig) {
        console.log('🔍 Sink Handler: Populating fields from config:', targetConfig);
        // Sink target type has no configurable fields - it's a simple logging endpoint
        console.log('✅ Sink Handler: No fields to populate for sink target');
    }

    validateConfiguration() {
        // Sink interfaces are always valid as they have no external dependencies
        return {
            isValid: true,
            errors: []
        };
    }
}

/**
 * Interface Handler Factory - Creates appropriate handler based on type
 */
class InterfaceHandlerFactory {
    static createHandler(targetType, interfaceData = {}) {
        console.log(`🏭 Factory: Creating handler for type: ${targetType}`);

        switch (targetType?.toLowerCase()) {
            case 'fhir':
                return new FhirInterfaceHandler(interfaceData);
            case 'hl7':
            case 'mllp':
            case 'tcp':
                return new Hl7InterfaceHandler(interfaceData);
            case 'database':
            case 'db':
                return new DatabaseInterfaceHandler(interfaceData);
            case 'file':
            case 'filesystem':
                return new FileInterfaceHandler(interfaceData);
            case 'sink':
                return new SinkInterfaceHandler(interfaceData);
            case 'http':
            case 'https':
            case 'rest':
                // For HTTP targets, use base handler with HTTP-specific defaults
                const httpHandler = new BaseInterfaceHandler(interfaceData);
                httpHandler.type = 'http';
                return httpHandler;
            default:
                console.warn(`⚠️ Factory: Unknown type ${targetType}, using base handler`);
                const baseHandler = new BaseInterfaceHandler(interfaceData);
                baseHandler.type = targetType || 'generic';
                return baseHandler;
        }
    }

    static getSupportedTypes() {
        return ['fhir', 'hl7', 'database', 'file', 'http', 'sink'];
    }
}

// Export classes for global access
window.BaseInterfaceHandler = BaseInterfaceHandler;
window.FhirInterfaceHandler = FhirInterfaceHandler;
window.Hl7InterfaceHandler = Hl7InterfaceHandler;
window.DatabaseInterfaceHandler = DatabaseInterfaceHandler;
window.FileInterfaceHandler = FileInterfaceHandler;
window.SinkInterfaceHandler = SinkInterfaceHandler;
window.InterfaceHandlerFactory = InterfaceHandlerFactory;

console.log('✅ Interface Handlers loaded with object-oriented approach');
console.log('📋 Supported types:', InterfaceHandlerFactory.getSupportedTypes());
// services/connectors/ConnectorFactory.js
// Factory for creating appropriate input/output connectors

class ConnectorFactory {
    /**
     * Create input connector based on configuration
     * @param {string} sourceType - Type of source (tcp, database, file, etc.)
     * @param {Object} sourceConfig - Configuration for the connector
     * @returns {InputConnector} - Appropriate input connector instance
     */
    static createInputConnector(sourceType, sourceConfig) {
        console.log(`🔌 Creating input connector: ${sourceType}`);

        switch (sourceType.toLowerCase()) {
            case 'tcp':
            case 'hl7':
                const TCPConnector = require('./input/TCPInputConnector');
                return new TCPConnector(sourceConfig);

            case 'file':
            case 'filesystem':
                const FileConnector = require('./input/FileInputConnector');
                return new FileConnector(sourceConfig);

            case 'database':
            case 'db':
                const DatabaseConnector = require('./input/DatabaseInputConnector');
                return new DatabaseConnector(sourceConfig);

            case 'sftp':
                const SFTPConnector = require('./input/SFTPInputConnector');
                return new SFTPConnector(sourceConfig);

            case 's3':
            case 'aws-s3':
                const S3Connector = require('./input/S3InputConnector');
                return new S3Connector(sourceConfig);

            case 'azure-blob':
            case 'azure':
                const AzureConnector = require('./input/AzureBlobInputConnector');
                return new AzureConnector(sourceConfig);

            case 'gcp-storage':
            case 'gcp':
                const GCPConnector = require('./input/GCPStorageInputConnector');
                return new GCPConnector(sourceConfig);

            case 'rest':
            case 'http':
            case 'api':
                const RESTConnector = require('./input/RESTInputConnector');
                return new RESTConnector(sourceConfig);

            case 'soap':
                const SOAPConnector = require('./input/SOAPInputConnector');
                return new SOAPConnector(sourceConfig);

            default:
                throw new Error(`Unsupported input connector type: ${sourceType}`);
        }
    }

    /**
     * Create output connector based on configuration
     * @param {string} targetType - Type of target (tcp, database, file, etc.)
     * @param {Object} targetConfig - Configuration for the connector
     * @returns {OutputConnector} - Appropriate output connector instance
     */
    static createOutputConnector(targetType, targetConfig) {
        console.log(`🔌 Creating output connector: ${targetType}`);

        switch (targetType.toLowerCase()) {
            case 'tcp':
            case 'hl7':
                const TCPConnector = require('./output/TCPOutputConnector');
                return new TCPConnector(targetConfig);

            case 'file':
            case 'filesystem':
                const FileConnector = require('./output/FileOutputConnector');
                return new FileConnector(targetConfig);

            case 'database':
            case 'db':
                const DatabaseConnector = require('./output/DatabaseOutputConnector');
                return new DatabaseConnector(targetConfig);

            case 'sftp':
                const SFTPConnector = require('./output/SFTPOutputConnector');
                return new SFTPConnector(targetConfig);

            case 's3':
            case 'aws-s3':
                const S3Connector = require('./output/S3OutputConnector');
                return new S3Connector(targetConfig);

            case 'azure-blob':
            case 'azure':
                const AzureConnector = require('./output/AzureBlobOutputConnector');
                return new AzureConnector(targetConfig);

            case 'gcp-storage':
            case 'gcp':
                const GCPConnector = require('./output/GCPStorageOutputConnector');
                return new GCPConnector(targetConfig);

            case 'rest':
            case 'http':
            case 'api':
                const RESTConnector = require('./output/RESTOutputConnector');
                return new RESTConnector(targetConfig);

            case 'fhir':
            case 'fhir-server':
                const FHIRConnector = require('./output/FHIROutputConnector');
                return new FHIRConnector(targetConfig);

            case 'soap':
                const SOAPConnector = require('./output/SOAPOutputConnector');
                return new SOAPConnector(targetConfig);

            default:
                throw new Error(`Unsupported output connector type: ${targetType}`);
        }
    }

    /**
     * Get list of supported connector types
     */
    static getSupportedConnectors() {
        return {
            input: [
                'tcp', 'file', 'database', 'sftp', 's3', 'azure-blob',
                'gcp-storage', 'rest', 'soap'
            ],
            output: [
                'tcp', 'file', 'database', 'sftp', 's3', 'azure-blob',
                'gcp-storage', 'rest', 'fhir', 'soap'
            ]
        };
    }

    /**
     * Validate connector configuration
     */
    static validateConnectorConfig(connectorType, config, direction = 'input') {
        const validationRules = {
            tcp: ['host', 'port'],
            file: ['directory', 'pattern'],
            database: ['connectionString', 'query'],
            sftp: ['host', 'username', 'directory'],
            s3: ['bucket', 'region'],
            'azure-blob': ['connectionString', 'container'],
            'gcp-storage': ['bucket', 'keyFile'],
            rest: ['endpoint'],
            fhir: ['endpoint'],
            soap: ['endpoint', 'wsdl']
        };

        const requiredFields = validationRules[connectorType.toLowerCase()];

        if (!requiredFields) {
            throw new Error(`Unknown connector type: ${connectorType}`);
        }

        const missing = requiredFields.filter(field => !config[field]);

        if (missing.length > 0) {
            throw new Error(`Missing required fields for ${connectorType} connector: ${missing.join(', ')}`);
        }

        return true;
    }
}

module.exports = ConnectorFactory;
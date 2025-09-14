// services/MessageTypeMappingService.js
// Message-Type-Centric HL7-FHIR Mapping Service
// Replaces WizardMappingService with optimized architecture

class MessageTypeMappingService {
    constructor(pgPool = null) {
        this.pool = pgPool || this.getExistingDatabaseConnection();
        this.logger = this.getLogger();
    }

    /**
     * Get existing database connection from current setup
     */
    getExistingDatabaseConnection() {
        try {
            // Try to use existing interfaceService pool
            const interfaceService = require('./interfaceService');
            if (interfaceService && interfaceService.pool) {
                console.log('✅ Using interfaceService database pool');
                return interfaceService.pool;
            }
        } catch (error) {
            console.log('⚠️ interfaceService not available, trying alternatives...');
        }

        try {
            // Create new pool using existing pattern
            const { Pool } = require('pg');
            const pool = new Pool({
                host: process.env.DB_HOST || 'localhost',
                port: process.env.DB_PORT || 5432,
                database: process.env.DB_NAME || 'ezhealthkonnect',
                user: process.env.DB_USER || 'postgres',
                password: process.env.DB_PASSWORD || 'password',
                max: 20,
                idleTimeoutMillis: 30000,
                connectionTimeoutMillis: 2000,
            });
            console.log('✅ Created new database pool for MessageTypeMappingService');
            return pool;
        } catch (error) {
            console.error('❌ Failed to create database connection:', error);
            throw new Error('Cannot establish database connection for MessageTypeMappingService');
        }
    }

    /**
     * Get logger - use existing or fallback to console
     */
    getLogger() {
        try {
            return require('../utils/logger');
        } catch (error) {
            return {
                info: console.log.bind(console, '[INFO]'),
                warn: console.warn.bind(console, '[WARN]'),
                error: console.error.bind(console, '[ERROR]'),
                debug: console.log.bind(console, '[DEBUG]')
            };
        }
    }

    /**
     * Save wizard configuration using message-type-centric approach
     * @param {string} interfaceId - Interface UUID
     * @param {Object} wizardData - Complete wizard configuration data
     * @returns {Promise<Object>} - Save result with mapping details
     */
    async saveWizardConfiguration(interfaceId, wizardData) {
        const client = await this.pool.connect();

        try {
            await client.query('BEGIN');
            this.logger.info(`Starting message-type-centric mapping save for interface: ${interfaceId}`);

            const messageType = this.extractMessageType(wizardData);
            this.logger.info(`Processing mapping for message type: ${messageType}`);

            // 1. Check if we should use standard template or custom mapping
            const shouldUseStandard = await this.shouldUseStandardTemplate(wizardData, messageType);

            let mappingResult;
            if (shouldUseStandard) {
                mappingResult = await this.saveStandardMapping(client, interfaceId, messageType, wizardData);
            } else {
                mappingResult = await this.saveCustomMapping(client, interfaceId, messageType, wizardData);
            }

            await client.query('COMMIT');

            this.logger.info(`✅ Message-type mapping saved successfully:`, {
                interfaceId,
                messageType,
                usesStandard: shouldUseStandard,
                mappingId: mappingResult.id
            });

            return {
                success: true,
                interfaceId,
                messageType,
                mappingId: mappingResult.id,
                usesStandardTemplate: shouldUseStandard
            };

        } catch (error) {
            await client.query('ROLLBACK');
            this.logger.error('Failed to save message-type mapping:', error);
            throw error;
        } finally {
            client.release();
        }
    }

    /**
     * Save interface mapping using standard template
     */
    async saveStandardMapping(client, interfaceId, messageType, wizardData) {
        this.logger.info(`💾 Using standard template for ${messageType}`);

        // Get the standard template for this message type
        const templateQuery = `
            SELECT id, template_name, template_config
            FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
        `;
        const templateResult = await client.query(templateQuery, [messageType]);

        if (templateResult.rows.length === 0) {
            throw new Error(`No standard template found for message type: ${messageType}`);
        }

        const template = templateResult.rows[0];

        // Insert/Update interface message mapping
        const mappingQuery = `
            INSERT INTO interface_message_mappings (
                interface_id,
                message_type,
                uses_standard_template,
                standard_template_id,
                custom_mapping_config,
                created_by
            ) VALUES ($1, $2, true, $3, NULL, $4)
            ON CONFLICT (interface_id, message_type)
            DO UPDATE SET
                uses_standard_template = true,
                standard_template_id = EXCLUDED.standard_template_id,
                custom_mapping_config = NULL,
                updated_at = CURRENT_TIMESTAMP
            RETURNING id
        `;

        const userId = await this.getUserId(client, interfaceId);
        const mappingResult = await client.query(mappingQuery, [interfaceId, messageType, template.id, userId]);

        this.logger.info(`✅ Standard mapping applied using template: ${template.template_name}`);

        return {
            id: mappingResult.rows[0].id,
            templateId: template.id,
            templateName: template.template_name
        };
    }

    /**
     * Save interface mapping with custom configuration
     */
    async saveCustomMapping(client, interfaceId, messageType, wizardData) {
        this.logger.info(`🔧 Using custom mapping for ${messageType}`);

        const customConfig = this.buildCustomMappingConfig(wizardData);
        const customizedFields = this.extractCustomizedFields(wizardData);

        // Insert/Update interface message mapping
        const mappingQuery = `
            INSERT INTO interface_message_mappings (
                interface_id,
                message_type,
                uses_standard_template,
                standard_template_id,
                custom_mapping_config,
                customized_fields,
                customization_reason,
                created_by
            ) VALUES ($1, $2, false, NULL, $3, $4, $5, $6)
            ON CONFLICT (interface_id, message_type)
            DO UPDATE SET
                uses_standard_template = false,
                standard_template_id = NULL,
                custom_mapping_config = EXCLUDED.custom_mapping_config,
                customized_fields = EXCLUDED.customized_fields,
                customization_reason = EXCLUDED.customization_reason,
                updated_at = CURRENT_TIMESTAMP
            RETURNING id
        `;

        const userId = await this.getUserId(client, interfaceId);
        const customizationReason = 'Wizard-generated custom mapping';

        const mappingResult = await client.query(mappingQuery, [
            interfaceId,
            messageType,
            JSON.stringify(customConfig),
            customizedFields,
            customizationReason,
            userId
        ]);

        this.logger.info(`✅ Custom mapping saved with ${customizedFields.length} customized fields`);

        return {
            id: mappingResult.rows[0].id,
            customFieldCount: customizedFields.length
        };
    }

    /**
     * Determine if wizard data matches standard template
     */
    async shouldUseStandardTemplate(wizardData, messageType) {
        try {
            // Get standard template for comparison
            const templateQuery = `
                SELECT template_config
                FROM hl7_fhir_templates
                WHERE message_type = $1 AND is_default = true
            `;
            const result = await this.pool.query(templateQuery, [messageType]);

            if (result.rows.length === 0) {
                this.logger.warn(`No standard template found for ${messageType}, using custom mapping`);
                return false;
            }

            const standardConfig = result.rows[0].template_config;
            const wizardConfig = this.buildCustomMappingConfig(wizardData);

            // Simple comparison - in production, you might want more sophisticated comparison
            const differences = this.compareConfigurations(standardConfig, wizardConfig);
            const shouldUseStandard = differences.length < 3; // Threshold for "close enough"

            this.logger.info(`Template comparison result: ${differences.length} differences, using ${shouldUseStandard ? 'standard' : 'custom'}`);

            return shouldUseStandard;

        } catch (error) {
            this.logger.warn('Error comparing with standard template, defaulting to custom:', error.message);
            return false;
        }
    }

    /**
     * Get runtime mapping configuration for Go backend
     * This is the key method that the Go service will call
     */
    async getRuntimeMappingConfig(interfaceId, messageType) {
        try {
            const query = `
                SELECT
                    CASE
                        WHEN imm.uses_standard_template THEN t.template_config
                        ELSE imm.custom_mapping_config
                    END as effective_mapping,
                    imm.uses_standard_template,
                    t.template_name
                FROM interface_message_mappings imm
                LEFT JOIN hl7_fhir_templates t ON imm.standard_template_id = t.id
                WHERE imm.interface_id = $1 AND imm.message_type = $2
            `;

            const result = await this.pool.query(query, [interfaceId, messageType]);

            if (result.rows.length === 0) {
                throw new Error(`No mapping configuration found for interface ${interfaceId}, message type ${messageType}`);
            }

            const row = result.rows[0];

            this.logger.debug(`Retrieved ${row.uses_standard_template ? 'standard' : 'custom'} mapping for ${interfaceId}/${messageType}`);

            return {
                config: row.effective_mapping,
                usesStandard: row.uses_standard_template,
                templateName: row.template_name
            };

        } catch (error) {
            this.logger.error(`Failed to get runtime mapping config: ${error.message}`);
            throw error;
        }
    }

    /**
     * Get all mapping configurations for an interface
     */
    async getInterfaceMappings(interfaceId) {
        const query = `
            SELECT
                imm.message_type,
                imm.uses_standard_template,
                t.template_name,
                imm.customized_fields,
                imm.mapping_version,
                imm.updated_at
            FROM interface_message_mappings imm
            LEFT JOIN hl7_fhir_templates t ON imm.standard_template_id = t.id
            WHERE imm.interface_id = $1
            ORDER BY imm.message_type
        `;

        const result = await this.pool.query(query, [interfaceId]);
        return result.rows;
    }

    /**
     * Helper methods
     */
    extractMessageType(wizardData) {
        return wizardData.detectedMessageType ||
               wizardData.messageType ||
               wizardData.transformationMapping?.messageType ||
               'ADT^A01';
    }

    buildCustomMappingConfig(wizardData) {
        return {
            messageType: this.extractMessageType(wizardData),
            version: '2.0',
            createdAt: new Date().toISOString(),
            atomicMappings: wizardData.transformationMapping?.atomicMappings || [],
            enhancedSegments: wizardData.transformationMapping?.enhancedSegments || {},
            fhirTransformResult: wizardData.transformationMapping?.fhirTransformResult || null
        };
    }

    extractCustomizedFields(wizardData) {
        const atomicMappings = wizardData.transformationMapping?.atomicMappings || [];
        return atomicMappings.map(mapping => mapping.hl7Field).filter(Boolean);
    }

    async getUserId(client, interfaceId) {
        const result = await client.query('SELECT user_id FROM interfaces WHERE id = $1', [interfaceId]);
        return result.rows[0]?.user_id;
    }

    compareConfigurations(standardConfig, wizardConfig) {
        // Simplified comparison - count differences
        // In production, implement more sophisticated comparison logic
        const differences = [];

        // Compare atomic mappings count
        const standardMappings = standardConfig.mappings ? Object.keys(standardConfig.mappings).length : 0;
        const wizardMappings = wizardConfig.atomicMappings?.length || 0;

        if (Math.abs(standardMappings - wizardMappings) > 5) {
            differences.push('mapping_count_difference');
        }

        // Add more comparison logic as needed
        return differences;
    }
}

module.exports = MessageTypeMappingService;
// services/TransformationPipelineService.js
// Handles transformation pipeline creation and management for wizard
// CANONICAL FLOW: Wizard → Pipeline → Steps → Template

class TransformationPipelineService {
    constructor(pgPool = null) {
        this.pool = pgPool || this.getExistingDatabaseConnection();
    }

    /**
     * Get existing database connection
     */
    getExistingDatabaseConnection() {
        try {
            const interfaceService = require('./interfaceService');
            if (interfaceService && interfaceService.pool) {
                console.log('✅ Using interfaceService database pool');
                return interfaceService.pool;
            }
        } catch (error) {
            console.log('⚠️ interfaceService not available, trying alternatives...');
        }

        try {
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
            console.log('✅ Created new database pool for TransformationPipelineService');
            return pool;
        } catch (error) {
            console.error('❌ Failed to create database connection:', error);
            throw new Error('Cannot establish database connection');
        }
    }

    /**
     * Create complete pipeline for interface
     * This is the main entry point called by wizard
     */
    async createPipelineForInterface(interfaceId, messageType, interfaceName, wizardMappings, userId) {
        const client = await this.pool.connect();

        try {
            await client.query('BEGIN');

            console.log('\n📦 === CREATING TRANSFORMATION PIPELINE ===');
            console.log('Interface ID:', interfaceId);
            console.log('Message Type:', messageType);
            console.log('Mappings:', wizardMappings?.atomicMappings?.length || 0);

            // Step 1: Create pipeline record
            const pipelineId = await this.createPipeline(
                client,
                interfaceId,
                messageType,
                interfaceName,
                userId
            );

            // Step 2: Get or create template for mappings
            const templateId = await this.getOrCreateTemplate(
                client,
                messageType,
                wizardMappings,
                userId
            );

            // Step 3: Add HL7→FHIR mapping step (sequence 100)
            await this.addMappingStep(
                client,
                pipelineId,
                templateId,
                wizardMappings,
                userId
            );

            // Step 4: Add default pipeline steps
            await this.addDefaultPipelineSteps(
                client,
                pipelineId,
                userId
            );

            await client.query('COMMIT');

            console.log('✅ Pipeline created successfully:', {
                pipelineId,
                templateId,
                stepsCreated: 6 // 1 mapping + 5 default
            });

            return {
                success: true,
                pipelineId,
                templateId
            };

        } catch (error) {
            await client.query('ROLLBACK');
            console.error('❌ Failed to create pipeline:', error);
            throw error;
        } finally {
            client.release();
        }
    }

    /**
     * Create pipeline record
     */
    async createPipeline(client, interfaceId, messageType, pipelineName, userId) {
        console.log('📝 Creating pipeline record...');

        const query = `
            INSERT INTO transformation_pipelines (
                interface_id,
                message_type,
                pipeline_name,
                description,
                is_active,
                created_by
            ) VALUES ($1, $2, $3, $4, true, $5)
            RETURNING id
        `;

        const result = await client.query(query, [
            interfaceId,
            messageType,
            pipelineName,
            `Auto-generated pipeline for ${messageType}`,
            userId
        ]);

        const pipelineId = result.rows[0].id;
        console.log('✅ Pipeline created:', pipelineId);

        return pipelineId;
    }

    /**
     * Get or create template for message type
     * If standard template exists, use it
     * Otherwise, create custom template from wizard mappings
     */
    async getOrCreateTemplate(client, messageType, wizardMappings, userId) {
        console.log('🔍 Looking for standard template for', messageType);

        // Check if standard template exists (OOB template)
        const existingTemplate = await this.getStandardTemplate(client, messageType);

        if (existingTemplate) {
            console.log('✅ Using existing standard template:', existingTemplate.template_name);
            return existingTemplate.id;
        }

        // No standard template - create custom from wizard mappings
        if (!wizardMappings || !wizardMappings.atomicMappings || wizardMappings.atomicMappings.length === 0) {
            console.warn('⚠️ No mappings provided and no standard template exists');
            return null;
        }

        console.log('📝 Creating custom template from wizard mappings...');
        return await this.createCustomTemplate(client, messageType, wizardMappings, userId);
    }

    /**
     * Get standard template for message type
     */
    async getStandardTemplate(client, messageType) {
        const query = `
            SELECT id, template_name, template_description
            FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
            ORDER BY created_at DESC
            LIMIT 1
        `;

        const result = await client.query(query, [messageType]);
        return result.rows[0];
    }

    /**
     * Create custom template from wizard mappings
     */
    async createCustomTemplate(client, messageType, wizardMappings, userId) {
        const templateConfig = {
            version: '2.0',
            messageType: messageType,
            source: 'wizard',
            createdAt: new Date().toISOString(),
            resources: this.convertMappingsToResourceFormat(wizardMappings.atomicMappings),
            atomicMappings: wizardMappings.atomicMappings || [],
            metadata: wizardMappings.metadata || {}
        };

        const query = `
            INSERT INTO hl7_fhir_templates (
                message_type,
                hl7_version,
                template_name,
                template_description,
                template_config,
                is_default,
                created_by
            ) VALUES ($1, $2, $3, $4, $5, false, $6)
            RETURNING id
        `;

        const result = await client.query(query, [
            messageType,
            '2.5',
            `Wizard ${messageType} Mapping`,
            `Custom mapping created via wizard on ${new Date().toLocaleDateString()}`,
            JSON.stringify(templateConfig),
            userId
        ]);

        const templateId = result.rows[0].id;
        console.log('✅ Custom template created:', templateId);

        return templateId;
    }

    /**
     * Add HL7→FHIR mapping step to pipeline
     */
    async addMappingStep(client, pipelineId, templateId, wizardMappings, userId) {
        console.log('🔀 Adding HL7→FHIR mapping step...');

        const stepConfig = {
            fhir_version: 'R4',
            use_template: !!templateId,
            template_id: templateId
        };

        // If no template, embed mappings directly in step config
        if (!templateId && wizardMappings && wizardMappings.atomicMappings) {
            stepConfig.use_template = false;
            stepConfig.custom_mapping = wizardMappings;
        }

        const query = `
            INSERT INTO transformation_steps (
                pipeline_id,
                step_name,
                step_type,
                sequence,
                config,
                enabled,
                created_by
            ) VALUES ($1, $2, $3, $4, $5, true, $6)
            RETURNING id
        `;

        const result = await client.query(query, [
            pipelineId,
            'HL7→FHIR Transform',
            'core.mapping',
            100,  // Core mapping always at sequence 100
            JSON.stringify(stepConfig),
            userId
        ]);

        console.log('✅ HL7→FHIR step created:', result.rows[0].id);
    }

    /**
     * Add default pipeline steps
     */
    async addDefaultPipelineSteps(client, pipelineId, userId) {
        console.log('⚙️ Adding default pipeline steps...');

        const defaultSteps = [
            { sequence: 10, name: 'Field Validation', type: 'core.validation', config: { validations: [] } },
            { sequence: 20, name: 'API Enrichment', type: 'pre.enrichment.api', config: {} },
            { sequence: 30, name: 'Database Enrichment', type: 'pre.enrichment.database', config: {} },
            { sequence: 50, name: 'Field Mapping', type: 'core.transformation', config: { mappings: [] } },
            { sequence: 60, name: 'Script Enrichment', type: 'pre.enrichment.script', config: {} }
        ];

        const query = `
            INSERT INTO transformation_steps (
                pipeline_id,
                step_name,
                step_type,
                sequence,
                config,
                enabled,
                created_by
            ) VALUES ($1, $2, $3, $4, $5, true, $6)
        `;

        for (const step of defaultSteps) {
            await client.query(query, [
                pipelineId,
                step.name,
                step.type,
                step.sequence,
                JSON.stringify(step.config),
                userId
            ]);
        }

        console.log(`✅ Added ${defaultSteps.length} default steps`);
    }

    /**
     * Convert atomic mappings to resource-grouped format
     */
    convertMappingsToResourceFormat(atomicMappings) {
        const resources = {};

        for (const mapping of atomicMappings) {
            const resourceType = this.extractResourceType(mapping.fhirPath);

            if (!resources[resourceType]) {
                resources[resourceType] = { mappings: [] };
            }

            resources[resourceType].mappings.push(mapping);
        }

        return resources;
    }

    /**
     * Extract FHIR resource type from path
     * Example: "Patient.identifier[0].value" → "Patient"
     */
    extractResourceType(fhirPath) {
        const match = fhirPath.match(/^([A-Z][a-zA-Z]+)\./);
        return match ? match[1] : 'Unknown';
    }
}

module.exports = TransformationPipelineService;

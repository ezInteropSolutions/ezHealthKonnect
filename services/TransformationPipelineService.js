// services/TransformationPipelineService.js
// Handles transformation pipeline creation and management for wizard
// CANONICAL FLOW: Wizard → Pipeline → Steps → Template

class TransformationPipelineService {
    /**
     * Get sequelize instance (lazy - called at request time, not module load)
     */
    _getSequelize() {
        const database = require('../config/database');
        if (!database.sequelize) {
            throw new Error('Database not connected. Ensure database.connect() was called on startup.');
        }
        return database.sequelize;
    }

    /**
     * Create complete pipeline for interface
     * This is the main entry point called by wizard
     *
     * connectivityInfo shape:
     *   { sourceConnectivity, sourceConfig, targetConnectivity, targetConfig, sourceType, targetType }
     */
    async createPipelineForInterface(interfaceId, messageType, interfaceName, wizardMappings, userId, connectivityInfo = null) {
        const sequelize = this._getSequelize();

        const sourceType = connectivityInfo?.sourceType || 'hl7v2';
        const targetType = connectivityInfo?.targetType || 'fhir';

        console.log('\n📦 === CREATING TRANSFORMATION PIPELINE ===');
        console.log('Interface ID:', interfaceId);
        console.log('Message Type:', messageType);
        console.log('Flow:', `${sourceType} → ${targetType}`);
        console.log('Mappings:', wizardMappings?.atomicMappings?.length || 0);

        return await sequelize.transaction(async (t) => {
            // Step 1: Create pipeline record
            const pipelineId = await this.createPipeline(sequelize, t, interfaceId, messageType, interfaceName);

            // Step 2: Look up template (isolated with SAVEPOINT — failure does not abort the transaction)
            await sequelize.query('SAVEPOINT template_creation', { transaction: t });
            let templateId = null;
            try {
                templateId = await this.getOrCreateTemplate(sequelize, t, messageType, wizardMappings, userId);
                await sequelize.query('RELEASE SAVEPOINT template_creation', { transaction: t });
            } catch (templateErr) {
                await sequelize.query('ROLLBACK TO SAVEPOINT template_creation', { transaction: t });
                console.warn('⚠️ Template lookup/creation failed (pipeline will still be created):', templateErr.message);
            }

            // Step 3: Add all pipeline steps based on interface type
            const stepsCreated = await this.addDefaultPipelineSteps(
                sequelize, t, pipelineId,
                { sourceType, targetType, connectivityInfo, templateId, wizardMappings }
            );

            console.log('✅ Pipeline created successfully:', { pipelineId, templateId, stepsCreated });
            return { success: true, pipelineId, templateId };
        });
    }

    /**
     * Create pipeline record
     */
    async createPipeline(sequelize, t, interfaceId, messageType, pipelineName) {
        console.log('📝 Creating pipeline record...');

        const [rows] = await sequelize.query(`
            INSERT INTO transformation_pipelines (
                interface_id,
                message_type,
                pipeline_name,
                enabled
            ) VALUES ($1, $2, $3, true)
            RETURNING id::text
        `, {
            bind: [interfaceId, messageType, pipelineName],
            transaction: t
        });

        const pipelineId = rows[0].id;
        console.log('✅ Pipeline created:', pipelineId);
        return pipelineId;
    }

    /**
     * Get or create template for message type
     */
    async getOrCreateTemplate(sequelize, t, messageType, wizardMappings, userId) {
        console.log('🔍 Looking for standard template for', messageType);

        const existingTemplate = await this.getStandardTemplate(sequelize, t, messageType);

        if (existingTemplate) {
            console.log('✅ Using existing standard template:', existingTemplate.template_name);
            return existingTemplate.id;
        }

        if (!wizardMappings || !wizardMappings.atomicMappings || wizardMappings.atomicMappings.length === 0) {
            console.warn('⚠️ No mappings provided and no standard template exists');
            return null;
        }

        console.log('📝 Creating custom template from wizard mappings...');
        return await this.createCustomTemplate(sequelize, t, messageType, wizardMappings, userId);
    }

    /**
     * Get standard template for message type
     */
    async getStandardTemplate(sequelize, t, messageType) {
        const { QueryTypes } = require('sequelize');

        const rows = await sequelize.query(`
            SELECT id, template_name, template_description
            FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
            ORDER BY created_at DESC
            LIMIT 1
        `, {
            bind: [messageType],
            type: QueryTypes.SELECT,
            transaction: t
        });

        return rows[0] || null;
    }

    /**
     * Create custom template from wizard mappings
     */
    async createCustomTemplate(sequelize, t, messageType, wizardMappings, userId) {
        const templateConfig = {
            version: '2.0',
            messageType,
            source: 'wizard',
            createdAt: new Date().toISOString(),
            resources: this.convertMappingsToResourceFormat(wizardMappings.atomicMappings),
            atomicMappings: wizardMappings.atomicMappings || [],
            metadata: wizardMappings.metadata || {}
        };

        const [rows] = await sequelize.query(`
            INSERT INTO hl7_fhir_templates (
                message_type,
                hl7_version,
                template_name,
                template_description,
                template_config,
                is_default,
                created_by
            ) VALUES ($1, $2, $3, $4, $5, false, $6)
            RETURNING id::text
        `, {
            bind: [
                messageType,
                '2.5',
                `Wizard ${messageType} Mapping`,
                `Custom mapping created via wizard on ${new Date().toLocaleDateString()}`,
                JSON.stringify(templateConfig),
                userId || null
            ],
            transaction: t
        });

        const templateId = rows[0].id;
        console.log('✅ Custom template created:', templateId);
        return templateId;
    }

    /**
     * Add the correct pipeline steps for the given interface type.
     *
     * Step layout:
     *   seq   5 — source connector  (always, if connectivity provided)
     *   seq  60 — flow-specific transforms (e.g. HL7→FHIR)
     *   seq 295 — target connector  (always, if connectivity provided)
     *
     * Supported flows:
     *   hl7v2 → fhir  : HL7→FHIR Transform (seq 60)
     *   (others)       : connector → connector only, no transform in between
     *
     * @param {object} ctx  { sourceType, targetType, connectivityInfo, templateId, wizardMappings }
     * @returns {number}    total steps created
     */
    async addDefaultPipelineSteps(sequelize, t, pipelineId, ctx) {
        const { sourceType, targetType, connectivityInfo, templateId, wizardMappings } = ctx;
        let count = 0;

        // ── Source connector (always first) ──────────────────────────────────
        if (connectivityInfo?.sourceConnectivity) {
            await this.addConnectorStep(
                sequelize, t, pipelineId,
                'inbound', connectivityInfo.sourceConnectivity, connectivityInfo.sourceConfig
            );
            count++;
        }

        // ── Flow-specific transform steps ─────────────────────────────────────
        const flow = `${sourceType}→${targetType}`;
        console.log(`⚙️ Building transform steps for flow: ${flow}`);

        if (sourceType === 'hl7v2' && targetType === 'fhir') {
            // HL7 v2 → FHIR R4
            const stepConfig = {
                fhir_version: 'R4',
                use_template: !!templateId,
                template_id: templateId
            };
            if (!templateId && wizardMappings?.atomicMappings?.length) {
                stepConfig.use_template = false;
                stepConfig.custom_mapping = wizardMappings;
            }
            await sequelize.query(`
                INSERT INTO transformation_steps
                    (pipeline_id, step_name, step_type, sequence, config, enabled)
                VALUES ($1, $2, $3, $4, $5, true)
            `, {
                bind: [pipelineId, 'HL7→FHIR Transform', 'hl7_fhir_transform', 60, JSON.stringify(stepConfig)],
                transaction: t
            });
            count++;
            console.log('✅ Added HL7→FHIR Transform step');

        } else {
            // Future flows (HL7→HL7 passthrough, FHIR→FHIR, CSV→FHIR, etc.) go here
            console.log(`ℹ️ No transform steps defined for flow "${flow}" — source and target connectors only`);
        }

        // ── Target connector (always last) ────────────────────────────────────
        if (connectivityInfo?.targetConnectivity) {
            await this.addConnectorStep(
                sequelize, t, pipelineId,
                'outbound', connectivityInfo.targetConnectivity, connectivityInfo.targetConfig
            );
            count++;
        }

        console.log(`✅ Added ${count} step(s) for ${flow} pipeline`);
        return count;
    }

    /**
     * Add a connector step (inbound or outbound) to the pipeline
     */
    async addConnectorStep(sequelize, t, pipelineId, direction, connectivityType, wizardConfig = {}) {
        const SOURCE_TYPE_MAP = {
            'tcp':      { typeName: 'tcp_mllp',          name: 'TCP/MLLP Inbound' },   // matches connectivity_types.type_name in DB
            'http':     { typeName: 'http_rest',          name: 'HTTP REST Inbound' },
            'file':     { typeName: 'file_listener',      name: 'File Listener' },
            'database': { typeName: 'postgresql_inbound', name: 'Database Inbound' }
        };

        const TARGET_TYPE_MAP = {
            'http':     { typeName: 'http_outbound',      name: 'HTTP Outbound' },
            'fhir':     { typeName: 'http_outbound',      name: 'FHIR Outbound' },
            'tcp':      { typeName: 'tcp_mllp_outbound',  name: 'TCP/MLLP Outbound' },
            'file':     { typeName: 'file_writer',        name: 'File Writer' },
            'database': { typeName: 'postgresql_outbound', name: 'Database Outbound' }
        };

        const typeMap  = direction === 'inbound' ? SOURCE_TYPE_MAP : TARGET_TYPE_MAP;
        const mapping  = typeMap[connectivityType];

        if (!mapping) {
            console.log(`⚠️ No connector mapping for ${direction} type: ${connectivityType}, skipping`);
            return;
        }

        const stepType = `connector.${direction}`;
        const sequence = direction === 'inbound' ? 5 : 295;
        const normalizedConfig = this.normalizeWizardConfig(wizardConfig, mapping.typeName);

        const config = { connectorType: mapping.typeName, config: normalizedConfig };
        if (direction === 'outbound') {
            config.contentField = 'fhirBundle';
            config.contentType  = 'application/fhir+json';
        } else {
            config.timeoutMs = 30000;
        }

        await sequelize.query(`
            INSERT INTO transformation_steps (
                pipeline_id,
                step_name,
                step_type,
                sequence,
                config,
                enabled
            ) VALUES ($1, $2, $3, $4, $5, true)
        `, {
            bind: [pipelineId, mapping.name, stepType, sequence, JSON.stringify(config)],
            transaction: t
        });

        console.log(`🔌 Added ${direction} connector step: ${mapping.name} (seq ${sequence}, type: ${mapping.typeName})`);
    }

    /**
     * Sync wizard connectivity config into the matching connector step for an existing interface.
     * Called when the wizard is re-run to update an interface — keeps step and wizard in sync.
     */
    async syncConnectorStepFromWizard(interfaceId, direction, connectivityType, wizardConfig = {}) {
        const sequelize = this._getSequelize();

        const TARGET_TYPE_MAP = {
            'http':     'http_outbound',
            'fhir':     'http_outbound',
            'tcp':      'tcp_mllp_outbound',
            'file':     'file_writer',
            'database': 'postgresql_outbound'
        };
        const SOURCE_TYPE_MAP = {
            'tcp':      'tcp_mllp',
            'http':     'http_rest',
            'file':     'file_listener',
            'database': 'postgresql_inbound'
        };

        const typeMap      = direction === 'outbound' ? TARGET_TYPE_MAP : SOURCE_TYPE_MAP;
        const connTypeName = typeMap[connectivityType] || connectivityType;
        const stepType     = `connector.${direction}`;
        const normalizedConfig = this.normalizeWizardConfig(wizardConfig, connTypeName);

        try {
            // Find the pipeline for this interface
            const pipelines = await sequelize.query(
                `SELECT id FROM transformation_pipelines WHERE interface_id = $1 LIMIT 1`,
                { bind: [interfaceId], type: sequelize.QueryTypes.SELECT }
            );
            if (!pipelines.length) return;

            const pipelineId = pipelines[0].id;

            // Update the matching connector step's config sub-object (preserve connectorType, contentField etc.)
            await sequelize.query(`
                UPDATE transformation_steps
                SET config = jsonb_set(config, '{config}', config->'config' || $1::jsonb, true)
                WHERE pipeline_id = $2
                  AND step_type = $3
                  AND config->>'connectorType' = $4
            `, {
                bind: [JSON.stringify(normalizedConfig), pipelineId, stepType, connTypeName]
            });

            console.log(`🔄 Synced ${direction} connector step config for interface ${interfaceId}`);
        } catch (err) {
            console.warn('⚠️ syncConnectorStepFromWizard failed (non-fatal):', err.message);
        }
    }

    /**
     * Normalize wizard config keys to match connector config_schema field names
     */
    normalizeWizardConfig(wizardConfig, connectorTypeName) {
        if (!wizardConfig || Object.keys(wizardConfig).length === 0) return {};

        const normalized = {};

        if (connectorTypeName === 'http_outbound') {
            if (wizardConfig.endpoint) normalized.url = wizardConfig.endpoint;
            if (wizardConfig.url)      normalized.url = wizardConfig.url;

            // Normalize authType → authentication_type
            // Wizard may save 'basic'/'bearer' (form values) or already-normalized 'basic_auth'/'bearer_token'
            const authTypeMap = { basic: 'basic_auth', bearer: 'bearer_token', api_key: 'api_key', none: 'none' };
            const rawAuthType = wizardConfig.authType || wizardConfig.authentication_type || '';
            if (rawAuthType) {
                normalized.authentication_type = authTypeMap[rawAuthType] || rawAuthType;
            }

            // Credentials — accept both camelCase form keys (authUsername) AND direct keys (username)
            // Direct keys (username/password) are what `target_connectivity.config` stores
            normalized.username     = wizardConfig.username     || wizardConfig.authUsername  || undefined;
            normalized.password     = wizardConfig.password     || wizardConfig.authPassword  || undefined;
            normalized.bearer_token = wizardConfig.bearer_token || wizardConfig.bearerToken   || undefined;
            normalized.api_key      = wizardConfig.api_key      || wizardConfig.apiKey        || undefined;
            normalized.api_key_header = wizardConfig.api_key_header || wizardConfig.apiKeyHeader || undefined;

            // Strip undefined keys so they don't pollute the stored config
            Object.keys(normalized).forEach(k => { if (normalized[k] === undefined) delete normalized[k]; });

            if (wizardConfig.method)          normalized.method          = wizardConfig.method;
            if (wizardConfig.content_type)    normalized.content_type    = wizardConfig.content_type;
            if (wizardConfig.timeout_seconds) normalized.timeout_seconds = wizardConfig.timeout_seconds;
            return normalized;
        }

        if (connectorTypeName === 'tcp_mllp' || connectorTypeName === 'tcp_mllp_inbound' || connectorTypeName === 'tcp_mllp_outbound') {
            if (wizardConfig.host) normalized.host = wizardConfig.host;
            if (wizardConfig.port) normalized.port = parseInt(wizardConfig.port, 10) || wizardConfig.port;
            return normalized;
        }

        if (connectorTypeName === 'file_listener' || connectorTypeName === 'file_writer') {
            if (wizardConfig.directory_path) normalized.directory_path = wizardConfig.directory_path;
            if (wizardConfig.file_pattern)   normalized.file_pattern   = wizardConfig.file_pattern;
            return normalized;
        }

        if (connectorTypeName.includes('postgresql') || connectorTypeName.includes('mysql') ||
            connectorTypeName.includes('mongodb')    || connectorTypeName.includes('sqlserver')) {
            ['host', 'port', 'database', 'username', 'password', 'ssl_mode', 'query', 'table_name']
                .forEach(key => { if (wizardConfig[key] !== undefined) normalized[key] = wizardConfig[key]; });
            return normalized;
        }

        return { ...wizardConfig };
    }

    /**
     * Convert atomic mappings to resource-grouped format
     */
    convertMappingsToResourceFormat(atomicMappings) {
        const resources = {};
        for (const mapping of atomicMappings) {
            const resourceType = this.extractResourceType(mapping.fhirPath);
            if (!resources[resourceType]) resources[resourceType] = { mappings: [] };
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

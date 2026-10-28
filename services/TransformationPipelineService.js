// services/TransformationPipelineService.js
// Handles transformation pipeline creation and management for wizard
// CANONICAL FLOW: Wizard → Pipeline → Steps → Template

// What an outbound connector step DELIVERS, by wizard flow. A flow that builds a FHIR bundle delivers that bundle
// (the default). A passthrough delivers the ORIGINAL message — the parsed message's `raw` field — byte for byte, in
// its own format. Previously every outbound step was saved as contentField "fhirBundle" + "application/fhir+json",
// so a passthrough (which builds no bundle) delivered the whole parsed-message envelope labelled as FHIR: ~387 KB of
// JSON for a 4-segment HL7 message. A new flow is one row here.
const FLOW_OUTBOUND_CONTENT = {
    hl7_passthrough:  { contentField: 'raw', contentType: 'application/hl7-v2' },
    fhir_passthrough: { contentField: 'raw', contentType: 'application/fhir+json' },
};
// Flows whose pipeline builds a FHIR bundle (FLOW_STEP_TEMPLATES in addDefaultPipelineSteps): the outbound step delivers it.
const FHIR_BUNDLE_FLOWS = new Set(['hl7_to_fhir', 'hl7_to_fhir_r5', 'hl7_to_fhir_stu3', 'ccd_to_fhir']);
const FHIR_BUNDLE_CONTENT = { contentField: 'fhirBundle', contentType: 'application/fhir+json' };
// Every other flow adds no transform step, so its output IS its input: deliver the original content, whatever the
// format (EDI, NCPDP, CSV, DICOM text, ...). No contentType is forced, since the format is not known here.
const ORIGINAL_CONTENT = { contentField: 'raw' };

// The settings the wizard copies into a file connector's step: the connector's real, documented options (see its
// connectivity_types config schema). Previously only directory_path/file_pattern were kept, so a file writer's
// filename_pattern / create_subdirs / overwrite_existing / encoding and a file listener's polling_interval /
// after_processing / archive_path ... were silently discarded.
const FILE_CONNECTOR_KEYS = {
    file_listener: ['directory_path', 'file_pattern', 'polling_interval', 'after_processing', 'archive_path', 'recursive', 'create_dirs', 'encoding'],
    file_writer:   ['directory_path', 'file_pattern', 'filename_pattern', 'create_subdirs', 'overwrite_existing', 'encoding'],
};

// Wizard form keys that normalizeWizardConfig renames to the connector's own names; they must not also be copied
// across under the old name.
const HTTP_ALIAS_KEYS = ['endpoint', 'authType', 'authUsername', 'authPassword', 'bearerToken', 'apiKey', 'apiKeyHeader'];

/**
 * Copies every setting the user entered that normalization has not already handled. The wizard used to keep a short
 * whitelist per connector (TCP/MLLP: host and port only; HTTP: five fields; databases: eight), silently discarding
 * the rest of what the user typed: retries, keep-alive, TLS, the ACK block, headers, polling interval ...
 * Empty values are not copied (an untouched form field is not a setting).
 */
function keepRemainingSettings(wizardConfig, normalized, renamedKeys = []) {
    for (const [key, value] of Object.entries(wizardConfig)) {
        if (renamedKeys.includes(key) || key in normalized) continue;
        if (value === undefined || value === null || value === '') continue;
        normalized[key] = value;
    }
    return normalized;
}

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
                { sourceType, targetType, connectivityInfo, templateId, wizardMappings, interfaceId, messageType }
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
     *   seq  60 — flow-specific transform (keyed by transformationFlow)
     *   seq 295 — target connector  (always, if connectivity provided)
     *
     * Transform steps are driven by FLOW_STEP_TEMPLATES below (single source of truth).
     * Adding a new flow: add one entry to FLOW_STEP_TEMPLATES — no other changes needed here.
     *
     * @param {object} ctx  { sourceType, targetType, connectivityInfo, templateId, wizardMappings }
     * @returns {number}    total steps created
     */

    // Single source of truth: maps transformationFlow → pipeline step definition.
    // Mirror of FlowRegistry.js on the frontend (same keys).

    async addDefaultPipelineSteps(sequelize, t, pipelineId, ctx) {
        const FLOW_STEP_TEMPLATES = {
            hl7_to_fhir:    { stepType: 'hl7_fhir_transform', stepName: 'HL7→FHIR Transform',  sequence: 60, fhirVersion: 'R4' },
            hl7_to_fhir_r5: { stepType: 'hl7_fhir_transform', stepName: 'HL7→FHIR Transform',  sequence: 60, fhirVersion: 'R5' },
            hl7_to_fhir_stu3: { stepType: 'hl7_fhir_transform', stepName: 'HL7→FHIR Transform', sequence: 60, fhirVersion: 'STU3' },
            ccd_to_fhir:    { stepType: 'cda.to_fhir',         stepName: 'CDA→FHIR Transform',  sequence: 60, fhirVersion: 'R4' },
            // passthrough, fhir_receiver, file_processor, others → no transform step (not listed here)
        };

        const { sourceType, targetType, connectivityInfo, templateId, wizardMappings, interfaceId, messageType } = ctx;
        let count = 0;

        // ── Source connector (always first) ──────────────────────────────────
        if (connectivityInfo?.sourceConnectivity) {
            await this.addConnectorStep(
                sequelize, t, pipelineId,
                'inbound', connectivityInfo.sourceConnectivity, connectivityInfo.sourceConfig
            );
            count++;
        }

        // ── Flow-specific transform step ──────────────────────────────────────
        const transformationFlow = connectivityInfo?.transformationFlow;
        const srcConn = connectivityInfo?.sourceConnectivity || '';
        const isFHIRInboundConnector = srcConn.includes('fhir') || srcConn === 'http_rest_inbound';

        console.log(`⚙️ Building transform steps for flow: ${sourceType}→${targetType} (transformationFlow: ${transformationFlow})`);

        const template = FLOW_STEP_TEMPLATES[transformationFlow];
        if (template && !isFHIRInboundConnector) {
            const mappingMode = templateId ? 'oob' : 'custom';
            const stepConfig = {
                fhir_version: template.fhirVersion,
                interface_id: interfaceId || null,
                message_type: messageType || null,
                mapping_mode: mappingMode,
            };
            await sequelize.query(`
                INSERT INTO transformation_steps
                    (pipeline_id, step_name, step_type, sequence, config, enabled)
                VALUES ($1, $2, $3, $4, $5, true)
            `, {
                bind: [pipelineId, template.stepName, template.stepType, template.sequence, JSON.stringify(stepConfig)],
                transaction: t
            });
            count++;
            console.log(`✅ Added ${template.stepName} step (type: ${template.stepType}, interface_id: ${interfaceId}, mode: ${mappingMode})`);
        } else {
            console.log(`ℹ️ No transform steps defined for flow "${transformationFlow}" — source and target connectors only`);
        }

        // ── Target connector (always last) ────────────────────────────────────
        if (connectivityInfo?.targetConnectivity) {
            await this.addConnectorStep(
                sequelize, t, pipelineId,
                'outbound', connectivityInfo.targetConnectivity, connectivityInfo.targetConfig, transformationFlow
            );
            count++;
        }

        console.log(`✅ Added ${count} step(s) for ${sourceType}→${targetType} (${transformationFlow}) pipeline`);
        return count;
    }

    /**
     * Add a connector step (inbound or outbound) to the pipeline
     */
    async addConnectorStep(sequelize, t, pipelineId, direction, connectivityType, wizardConfig = {}, transformationFlow = null) {
        if (!connectivityType) return; // no source/target connector configured for this interface

        // Legacy short-name aliases predating the modern ConnectorConfigBuilder
        // UI, which now sends real connector type names directly. This is the
        // ONLY hardcoded mapping left — every real type name resolves
        // dynamically below instead of a second, parallel hardcoded allowlist,
        // which is what previously left as2_inbound/as2_outbound (and most
        // database/warehouse connector types — mongodb, oracle, snowflake,
        // databricks, azure_blob, kafka, redis, sftp, gcs...) silently
        // creating NO connector step at all when built via this generic
        // wizard flow (they only ever worked via a template's own
        // pre-built pipeline, which bypasses this function entirely).
        const LEGACY_ALIASES = {
            inbound:  { tcp: 'tcp_mllp', http: 'http_rest', fhir: 'http_fhir_inbound', file: 'file_listener', database: 'postgresql_inbound' },
            // 'sink' is what the wizard sends for store-only flows (Passthrough, FHIR Receiver); the
            // registered connector is sink_outbound. Without this alias the step was saved with an
            // unknown connector type and every message to such an interface failed at delivery.
            outbound: { http: 'http_outbound', fhir: 'http_outbound', tcp: 'tcp_mllp_outbound', file: 'file_writer', database: 'postgresql_outbound', sink: 'sink_outbound' },
        };
        const typeName = (LEGACY_ALIASES[direction] || {})[connectivityType] || connectivityType;

        // Best-effort human-readable step name from connectivity_types (the
        // real connector catalog) — never blocks step creation on this
        // lookup succeeding. A type with no catalog row (an older bare
        // alias like tcp_mllp_inbound alongside the catalog's own tcp_mllp,
        // or a brand-new type whose migration hasn't landed in THIS
        // database yet) still gets a working step, just with a generated
        // name instead of the curated one.
        let displayName = typeName.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
        try {
            const rows = await sequelize.query(
                `SELECT display_name FROM connectivity_types WHERE type_name = $1 AND category = $2 LIMIT 1`,
                { bind: [typeName, direction], type: sequelize.QueryTypes.SELECT, transaction: t }
            );
            if (rows.length && rows[0].display_name) displayName = rows[0].display_name;
        } catch (err) {
            console.warn(`⚠️ connectivity_types lookup failed for ${typeName} (non-fatal, using generated name):`, err.message);
        }

        const stepType = `connector.${direction}`;
        const sequence = direction === 'inbound' ? 5 : 295;
        const normalizedConfig = this.normalizeWizardConfig(wizardConfig, typeName);

        const config = { connectorType: typeName, config: normalizedConfig };
        if (direction === 'outbound') {
            // EDI/AS2 outbound connectors deliver a built X12 document, never
            // a FHIR bundle — the FHIR default below predates these types and
            // would otherwise silently point payload extraction at a field
            // that's never populated for an EDI-only interface.
            if (typeName === 'edi_x12_outbound' || typeName === 'as2_outbound') {
                config.contentField = 'ediX12';
                config.contentType  = 'application/edi-x12';
            } else {
                const content = FLOW_OUTBOUND_CONTENT[transformationFlow]
                    || (FHIR_BUNDLE_FLOWS.has(transformationFlow) ? FHIR_BUNDLE_CONTENT : ORIGINAL_CONTENT);
                config.contentField = content.contentField;
                if (content.contentType) config.contentType = content.contentType;
            }
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
            bind: [pipelineId, displayName, stepType, sequence, JSON.stringify(config)],
            transaction: t
        });

        console.log(`🔌 Added ${direction} connector step: ${displayName} (seq ${sequence}, type: ${typeName})`);
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
            'database': 'postgresql_outbound',
            'sink':     'sink_outbound'
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

            // Everything else the user entered (method, content_type, timeouts, retry_attempts, retry_delay_seconds,
            // headers, ...) is the connector's own setting and is kept as typed.
            return keepRemainingSettings(wizardConfig, normalized, HTTP_ALIAS_KEYS);
        }

        if (connectorTypeName === 'tcp_mllp' || connectorTypeName === 'tcp_mllp_inbound' || connectorTypeName === 'tcp_mllp_outbound') {
            if (wizardConfig.host) normalized.host = wizardConfig.host;
            if (wizardConfig.port) normalized.port = parseInt(wizardConfig.port, 10) || wizardConfig.port;
            // Keep-alive, TLS, retries, the ACK/NACK block and the rest are real connector settings: not dropped.
            return keepRemainingSettings(wizardConfig, normalized);
        }

        if (connectorTypeName === 'sink_outbound') {
            // The wizard stores camelCase options; the Go connector reads snake_case keys.
            const sinkKeys = { enableLogging: 'enable_logging', enableValidation: 'enable_validation', retentionDays: 'retention_days', generateAck: 'generate_ack' };
            for (const [wizardKey, connectorKey] of Object.entries(sinkKeys)) {
                const value = wizardConfig[connectorKey] !== undefined ? wizardConfig[connectorKey] : wizardConfig[wizardKey];
                if (value !== undefined) normalized[connectorKey] = value;
            }
            return normalized;
        }

        if (FILE_CONNECTOR_KEYS[connectorTypeName]) {
            for (const key of FILE_CONNECTOR_KEYS[connectorTypeName]) {
                if (wizardConfig[key] !== undefined && wizardConfig[key] !== '') normalized[key] = wizardConfig[key];
            }
            return normalized;
        }

        if (connectorTypeName.includes('postgresql') || connectorTypeName.includes('mysql') ||
            connectorTypeName.includes('mongodb')    || connectorTypeName.includes('sqlserver')) {
            // Connection string, collection, polling interval, batch size, mark-as-read query ... are all real settings.
            return keepRemainingSettings(wizardConfig, normalized);
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

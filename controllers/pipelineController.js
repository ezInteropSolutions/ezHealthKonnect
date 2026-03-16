// controllers/pipelineController_v2.js
// FIXED: Saves pipeline to V20 schema with embedded wizard mappings

const axios = require('axios');
const crypto = require('crypto');
const GO_BACKEND_URL = process.env.GO_BACKEND_URL || `http://127.0.0.1:${process.env.API_PORT || 8080}`;

// ── Credential encryption for step configs ─────────────────────────────────
// Uses the same APP_CREDENTIAL_KEY + AES-256-GCM algorithm as the Go
// CredentialStore so the Go executor can transparently decrypt on execution.
// Wire-format: nonce(12 bytes) || ciphertext(N bytes) || tag(16 bytes), base64-
// encoded, prefixed with 'ENC:v1:'.
//
// Sensitive keys are detected by substring (case-insensitive), matching the same
// logic as Go's isSensitiveKey(). This catches dbPassword, connectionString,
// secretAccessKey, apiKey, etc. without an exhaustive allowlist.

const SENSITIVE_KEY_SUBSTRINGS = [
    'password', 'passwd',
    'secret',
    'token',
    'passphrase',
    'apikey', 'api_key',
    'privatekey', 'private_key',
    'connectionstring', 'connection_string',
    'accesskey', 'access_key',
];

function isSensitiveConfigKey(key) {
    const lower = key.toLowerCase();
    return SENSITIVE_KEY_SUBSTRINGS.some(s => lower.includes(s));
}

/**
 * Encrypts sensitive field values in a step config object.
 * Returns a shallow copy — the original is not mutated.
 * Passthrough if APP_CREDENTIAL_KEY is not set (dev mode).
 * Idempotent: values already starting with 'ENC:v1:' are skipped.
 */
function encryptSensitiveConfigFields(config) {
    if (!config || typeof config !== 'object' || Array.isArray(config)) return config;

    const credKeyB64 = process.env.APP_CREDENTIAL_KEY;
    if (!credKeyB64) return config; // passthrough in dev mode

    let credKey;
    try {
        credKey = Buffer.from(credKeyB64, 'base64');
        if (credKey.length !== 32) return config; // wrong key length — passthrough
    } catch (e) {
        return config;
    }

    const result = Object.assign({}, config);
    for (const [k, v] of Object.entries(result)) {
        if (!isSensitiveConfigKey(k)) continue;
        if (typeof v !== 'string' || v === '' || v.startsWith('ENC:v1:')) continue;

        // AES-256-GCM: layout must match Go's gcm.Seal output:
        //   nonce(12) || ciphertext(N) || tag(16)  →  base64  →  'ENC:v1:' prefix
        const nonce = crypto.randomBytes(12);
        const cipher = crypto.createCipheriv('aes-256-gcm', credKey, nonce);
        const enc = Buffer.concat([cipher.update(v, 'utf8'), cipher.final()]);
        const tag = cipher.getAuthTag(); // 16 bytes
        result[k] = 'ENC:v1:' + Buffer.concat([nonce, enc, tag]).toString('base64');
    }
    return result;
}
// ──────────────────────────────────────────────────────────────────────────────

/**
 * Topological sort for steps based on parent_conditional_step_id relationships
 * Ensures parent steps are inserted before their children (FK constraint)
 * @param {Array} steps - Array of step objects
 * @returns {Array} - Sorted array where parents come before children
 */
function topologicalSortSteps(steps) {
    // Build a map of step ID to step
    const stepMap = new Map();
    steps.forEach(step => {
        stepMap.set(step.id, step);
    });

    // Build adjacency list (parent → children)
    const children = new Map(); // parent_id → [child steps]
    const inDegree = new Map(); // step_id → number of parents

    steps.forEach(step => {
        if (!children.has(step.id)) {
            children.set(step.id, []);
        }
        inDegree.set(step.id, 0);
    });

    steps.forEach(step => {
        const parentId = step.parent_conditional_step_id || step.parentConditionalStepId;
        if (parentId && stepMap.has(parentId)) {
            // This step has a parent that's in our list
            children.get(parentId).push(step);
            inDegree.set(step.id, (inDegree.get(step.id) || 0) + 1);
        }
    });

    // Kahn's algorithm for topological sort
    const queue = [];
    const sorted = [];

    // Start with steps that have no parents (in-degree 0)
    steps.forEach(step => {
        if (inDegree.get(step.id) === 0) {
            queue.push(step);
        }
    });

    while (queue.length > 0) {
        const step = queue.shift();
        sorted.push(step);

        // Process children of this step
        const childSteps = children.get(step.id) || [];
        childSteps.forEach(child => {
            inDegree.set(child.id, inDegree.get(child.id) - 1);
            if (inDegree.get(child.id) === 0) {
                queue.push(child);
            }
        });
    }

    // If there are steps not in sorted (cycle or orphan references), add them
    // This handles cases where parent_conditional_step_id references a step not in the pipeline
    steps.forEach(step => {
        if (!sorted.includes(step)) {
            console.warn(`⚠️ Step ${step.id} (${step.stepName || step.step_name}) has orphan parent reference - adding to end`);
            sorted.push(step);
        }
    });

    return sorted;
}

/**
 * Test pipeline with sample message
 * POST /api/pipelines/test
 * Proxies request to Go backend for execution
 */
exports.testPipeline = async (req, res) => {
    try {
        console.log('🧪 [Node.js] Proxying test request to Go backend:', GO_BACKEND_URL);

        // Forward request to Go backend with increased limits for large pipeline responses
        const response = await axios.post(
            `${GO_BACKEND_URL}/api/fhir/pipeline/test`,
            req.body,
            {
                timeout: 60000,
                maxContentLength: 50 * 1024 * 1024,  // 50MB
                maxBodyLength: 50 * 1024 * 1024,
                headers: {
                    'Content-Type': 'application/json'
                }
            }
        );

        // Debug: Log what Go returned
        const dataType = typeof response.data;
        const dataKeys = response.data && dataType === 'object' ? Object.keys(response.data) : 'NOT_OBJECT';
        console.log(`🧪 [Node.js] Go response: status=${response.status}, type=${dataType}, keys=${JSON.stringify(dataKeys)}`);

        // Guard against empty response from Go
        if (!response.data || dataType === 'string') {
            console.error('❌ [Node.js] Go returned empty or non-JSON response:', response.data);
            return res.status(500).json({
                success: false,
                error: 'Go backend returned empty response',
                _debug: { dataType, responseStatus: response.status, headers: response.headers }
            });
        }

        return res.status(200).json(response.data);
    } catch (error) {
        console.error('❌ [Node.js] Test pipeline proxy error:', error.message);

        // If Go returned an error response, pass it through
        if (error.response) {
            console.error('❌ [Node.js] Go error status:', error.response.status);
            console.error('❌ [Node.js] Go error data:', JSON.stringify(error.response.data).substring(0, 500));
            return res.status(error.response.status).json(
                error.response.data || { success: false, error: error.message }
            );
        }

        return res.status(500).json({
            success: false,
            error: error.message || 'Failed to test pipeline'
        });
    }
};

/**
 * Sprint 4 / P7 migration: rewrite enriched.* path references → steps.{ns}.step_output.*
 * Called on every pipeline save to transparently migrate configs written before P7.
 *
 * Mapping rules (default enriched keys → step type):
 *   enriched.api          → api_enrichment step
 *   enriched.database     → database_enrichment step
 *   enriched.script       → script_enrichment step
 *   enriched.field_mapping → field_mapping step
 *   enriched.file_parser  → file_parser step
 *
 * Namespace format (mirrors Go generateStepNamespace):
 *   {alias_or_stepName}_{first6charsOfStepId}  (lowercase, spaces → underscores)
 *
 * Ambiguous case (multiple steps of same type) → path left unchanged, warning logged.
 */
function migrateEnrichedPaths(steps) {
    const TYPE_TO_KEY = {
        'api_enrichment':      'api',
        'database_enrichment': 'database',
        'script_enrichment':   'script',
        'field_mapping':       'field_mapping',
        'file_parser':         'file_parser',
    };

    // Build {enrichedKey: namespace} map from the pipeline's steps
    const keyToNs = {};
    const ambiguous = new Set();
    for (const step of steps) {
        const stepType = step.stepType || step.step_type || step.type || '';
        const eKey = TYPE_TO_KEY[stepType];
        if (!eKey) continue;
        const rawName = step.alias || step.stepAlias || step.stepName || step.step_name || step.name || '';
        const baseName = rawName.replace(/\s+/g, '_').toLowerCase();
        const shortId  = (step.id || '').substring(0, 6);
        const ns       = `${baseName}_${shortId}`;
        if (keyToNs[eKey]) {
            ambiguous.add(eKey);
        } else {
            keyToNs[eKey] = ns;
        }
    }

    // Recursively rewrite enriched.* strings in any config value
    function rewrite(value) {
        if (typeof value === 'string') {
            return value.replace(
                /\benriched\.(api|database|script|field_mapping|file_parser)((?:\.[^"'\s,)}\]]+)*)/g,
                (match, eKey, rest) => {
                    if (ambiguous.has(eKey)) {
                        console.warn(`[P7 migration] Ambiguous enriched.${eKey} path — multiple steps of same type, skipping rewrite`);
                        return match;
                    }
                    const ns = keyToNs[eKey];
                    if (!ns) return match; // No step of this type in this pipeline
                    const stripped = rest.startsWith('.') ? rest.slice(1) : rest;
                    return stripped
                        ? `steps.${ns}.step_output.${stripped}`
                        : `steps.${ns}.step_output`;
                }
            );
        }
        if (Array.isArray(value)) return value.map(rewrite);
        if (value && typeof value === 'object') {
            const out = {};
            for (const [k, v] of Object.entries(value)) out[k] = rewrite(v);
            return out;
        }
        return value;
    }

    let migratedCount = 0;
    for (const step of steps) {
        if (step.config) {
            const rewritten = rewrite(step.config);
            if (JSON.stringify(rewritten) !== JSON.stringify(step.config)) {
                step.config = rewritten;
                migratedCount++;
            }
        }
    }
    if (migratedCount > 0) {
        console.log(`[P7 migration] Rewrote enriched.* paths in ${migratedCount} step config(s)`);
    }
}

/**
 * Save pipeline (create or update) - V20 Schema with Embedded Mappings
 * POST /api/pipelines
 *
 * FIXES:
 * 1. Saves to transformation_pipelines (metadata only)
 * 2. Saves each step to transformation_steps (proper V20 schema)
 * 3. Auto-embeds wizard mappings into HL7→FHIR step config (self-contained)
 */
exports.savePipeline = async (req, res) => {
    // DEBUG: Topological Sort Implementation v2.0
    console.log('');
    console.log('=================================================================');
    console.log('SAVE PIPELINE v2.0 - WITH TOPOLOGICAL SORT');
    console.log('Timestamp:', new Date().toISOString());
    console.log('=================================================================');
    console.log('');

    try {
        const pipelineData = req.body;

        // Validate required fields
        const interfaceId = pipelineData.interface_id || pipelineData.interfaceId;
        const messageType = pipelineData.message_type || pipelineData.messageType;

        if (!interfaceId || !messageType) {
            return res.status(400).json({
                success: false,
                error: 'interface_id and message_type are required'
            });
        }

        const { sequelize } = require('../config/database');
        const { v4: uuidv4 } = require('uuid');
        const { QueryTypes } = require('sequelize');

        const pipelineId = pipelineData.id || uuidv4();

        // Load wizard mappings for this interface (to embed in HL7→FHIR steps)
        // Try interface_message_mappings first (new V9+ architecture), fallback to transformation_mapping (legacy)
        const wizardMappings = await sequelize.query(`
            SELECT
                COALESCE(imm.custom_mapping_config, i.transformation_mapping) as mappings,
                imm.message_type as mapping_message_type
            FROM interfaces i
            LEFT JOIN interface_message_mappings imm
                ON imm.interface_id = i.id AND imm.message_type = $2
            WHERE i.id = $1
        `, {
            bind: [interfaceId, messageType],
            type: QueryTypes.SELECT
        });

        const embeddedMappings = wizardMappings[0]?.mappings || null;
        console.log('\n📋 === LOADING WIZARD MAPPINGS FOR EMBEDDING ===');
        console.log('Interface ID:', interfaceId);
        console.log('Message Type:', messageType);
        console.log('Query result:', wizardMappings);
        console.log('Embedded mappings exist:', embeddedMappings ? 'YES' : 'NO');
        if (embeddedMappings) {
            console.log('Mappings type:', typeof embeddedMappings);
            console.log('Mappings sample:', JSON.stringify(embeddedMappings).substring(0, 200));
        }

        // Start transaction
        await sequelize.query('BEGIN');

        try {
            // 1. Upsert pipeline metadata (including connections and pipeline_config)
            const connections = pipelineData.connections || [];
            const pipelineConfig = pipelineData.pipeline_config || {};
            console.log(`🔗 Saving ${connections.length} connections to pipeline`);
            console.log(`⚙️ Saving pipeline_config:`, JSON.stringify(pipelineConfig));

            const pipelineResult = await sequelize.query(`
                INSERT INTO transformation_pipelines
                    (id, interface_id, message_type, pipeline_name, enabled, version, connections, pipeline_config, created_at, updated_at)
                VALUES
                    ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
                ON CONFLICT (interface_id, message_type)
                DO UPDATE SET
                    pipeline_name = EXCLUDED.pipeline_name,
                    enabled = EXCLUDED.enabled,
                    connections = EXCLUDED.connections,
                    pipeline_config = EXCLUDED.pipeline_config,
                    version = transformation_pipelines.version + 1,
                    updated_at = NOW()
                RETURNING id::text, interface_id::text, message_type, pipeline_name, enabled, version, connections, pipeline_config;
            `, {
                bind: [
                    pipelineId,
                    interfaceId,
                    messageType,
                    pipelineData.name || `${messageType} Pipeline`,
                    pipelineData.status !== 'disabled',
                    1,
                    JSON.stringify(connections),
                    JSON.stringify(pipelineConfig)
                ],
                type: QueryTypes.SELECT
            });

            console.log(`✅ Pipeline saved: ${pipelineId}`);

            // 2. Delete old steps
            await sequelize.query(`
                DELETE FROM transformation_steps WHERE pipeline_id = $1
            `, {
                bind: [pipelineId],
                type: QueryTypes.DELETE
            });

            console.log(`🗑️  Cleared old steps for pipeline ${pipelineId}`);

            // 3. Collect ALL steps (supports both new flat format and legacy layers format)
            let stepsSaved = 0;
            const allSteps = [];

            // NEW FORMAT: flat execution_groups array
            if (pipelineData.execution_groups && pipelineData.execution_groups.length > 0) {
                pipelineData.execution_groups.forEach(group => {
                    if (group.steps && Array.isArray(group.steps)) {
                        group.steps.forEach(step => {
                            allSteps.push({ ...step, _layer: step.layer || 'core' });
                        });
                    }
                });
                console.log(`📊 Collected ${allSteps.length} steps from execution_groups`);
            }
            // LEGACY FORMAT: layers { pre, core, post }
            else if (pipelineData.layers) {
                for (const layer of ['pre', 'core', 'post']) {
                    const layerData = pipelineData.layers[layer];
                    let layerSteps = [];

                    if (layerData?.steps) {
                        layerSteps = layerData.steps;
                    } else if (layerData?.execution_groups) {
                        layerData.execution_groups.forEach(group => {
                            if (group.steps && Array.isArray(group.steps)) {
                                layerSteps.push(...group.steps);
                            }
                        });
                    }

                    console.log(`📊 Layer "${layer}": Found ${layerSteps.length} steps`);
                    layerSteps.forEach(step => {
                        allSteps.push({ ...step, _layer: 'core' });
                    });
                }
            }

            // P7 migration: rewrite enriched.* path references in step configs → steps.{ns}.step_output.*
            migrateEnrichedPaths(allSteps);

            // TOPOLOGICAL SORT: Parent steps must be inserted before child steps (FK constraint)
            // This handles multi-level dependencies: A -> B -> C
            console.log('');
            console.log('========== TOPOLOGICAL SORT DEBUG ==========');
            console.log('Pre-sort:', allSteps.length, 'steps collected from all layers');
            allSteps.forEach(s => {
                const parentId = s.parent_conditional_step_id || s.parentConditionalStepId;
                if (parentId) {
                    console.log('  Step', s.stepName || s.step_name, '(' + s.id + ') has parent:', parentId);
                }
            });

            const sortedSteps = topologicalSortSteps(allSteps);
            console.log('');
            console.log('Post-sort:', sortedSteps.length, 'steps in insertion order:');
            sortedSteps.forEach((s, i) => {
                const parentId = s.parent_conditional_step_id || s.parentConditionalStepId;
                const stepIdShort = s.id ? s.id.substring(0, 8) + '...' : 'no-id';
                const parentIdShort = parentId ? parentId.substring(0, 8) + '...' : 'none';
                console.log('  ' + (i + 1) + '. "' + (s.stepName || s.step_name) + '" (' + stepIdShort + ') parent: ' + parentIdShort);
            });
            console.log('========== END TOPOLOGICAL SORT DEBUG ==========');
            console.log('');

            for (const step of sortedSteps) {
                const layer = step._layer;
                const stepId = step.id || uuidv4();
                let stepConfig = step.config || {};

                // 🔥 CRITICAL: Embed wizard mappings into HL7→FHIR mapping steps
                // Check both camelCase and snake_case since frontend sends snake_case via toJSON()
                const stepType = step.stepType || step.step_type || step.type || '';
                const templateId = step.templateId || step.template_id || '';
                const stepName = step.stepName || step.step_name || step.name || '';

                console.log(`🔍 Step Detection: name="${stepName}" type="${stepType}" template="${templateId}"`);

                const isHL7FHIRStep = (
                    stepType === 'hl7_fhir_transform' ||
                    stepType === 'core.mapping' ||
                    templateId === 'hl7-fhir-mapping' ||
                    (stepName.includes('HL7') && stepName.includes('FHIR'))
                );

                if (isHL7FHIRStep && embeddedMappings) {
                    console.log('\n💾 === EMBEDDING WIZARD MAPPINGS ===');
                    console.log('Step name:', stepName);
                    console.log('Step type:', stepType);
                    console.log('Template ID:', templateId);
                    console.log('Embedding mappings of type:', typeof embeddedMappings);
                    stepConfig = {
                        ...stepConfig,
                        interface_id: interfaceId,
                        embedded_mappings: embeddedMappings,
                        _embedded_at: new Date().toISOString(),
                        _mapping_version: embeddedMappings.version || 1
                    };
                    console.log('✅ Step config after embedding:', JSON.stringify(stepConfig).substring(0, 300));
                } else if (isHL7FHIRStep && !embeddedMappings) {
                    console.warn('⚠️ HL7→FHIR step detected but NO mappings to embed!');
                }

                await sequelize.query(`
                    INSERT INTO transformation_steps
                        (id, pipeline_id, step_name, step_type, sequence, required, timeout_ms, enabled, config, script_type, script_content, on_error_strategy, position_x, position_y, parent_conditional_step_id, branch_type, case_value, created_at, updated_at)
                    VALUES
                        ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW())
                `, {
                    bind: [
                        stepId,
                        pipelineId,
                        step.step_name || step.stepName || step.name || 'Unnamed Step',
                        step.step_type || step.stepType || step.type || 'custom',
                        step.sequence || 100,
                        step.required !== false,
                        step.timeoutMs || step.timeout_ms || 5000,
                        step.enabled !== false,
                        JSON.stringify(encryptSensitiveConfigFields(stepConfig)),
                        step.scriptType || null,
                        step.scriptContent || null,
                        step.onErrorStrategy || step.on_error_strategy || 'fail',
                        step.position_x !== undefined ? step.position_x : null,
                        step.position_y !== undefined ? step.position_y : null,
                        // Support both snake_case (DB) and camelCase (JS) property names
                        step.parent_conditional_step_id || step.parentConditionalStepId || null,
                        step.branch_type || step.branchType || null,
                        step.case_value || step.caseValue || null
                    ],
                    type: QueryTypes.INSERT
                });

                stepsSaved++;
            }

            console.log(`✅ Saved ${stepsSaved} steps to pipeline ${pipelineId}`);

            // Commit transaction
            await sequelize.query('COMMIT');

            res.json({
                success: true,
                pipeline: pipelineResult[0],
                steps_saved: stepsSaved,
                mappings_embedded: embeddedMappings ? true : false,
                message: `Pipeline saved successfully with ${stepsSaved} steps${embeddedMappings ? ' (wizard mappings embedded)' : ''}`
            });

        } catch (error) {
            // Rollback on error
            await sequelize.query('ROLLBACK');
            throw error;
        }

    } catch (error) {
        console.error('❌ Save pipeline error:', error.message);
        console.error('Stack:', error.stack);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Get standard HL7-FHIR template mappings
 * Used by Pipeline Builder UI to display default mappings when no custom mappings exist
 */
exports.getStandardTemplateMappings = async (req, res) => {
    try {
        const { messageType } = req.params;

        if (!messageType) {
            return res.status(400).json({
                success: false,
                error: 'Message type is required'
            });
        }

        console.log(`📚 Fetching standard template mappings for message type: ${messageType}`);

        // Get database connection
        const database = require('../config/database');
        const sequelize = database.sequelize;
        const { QueryTypes } = require('sequelize');

        // Query the hl7_fhir_templates table for the default template
        const result = await sequelize.query(`
            SELECT
                id,
                template_name,
                template_description,
                message_type,
                hl7_version,
                template_config,
                is_default,
                created_at
            FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
            ORDER BY created_at DESC
            LIMIT 1
        `, {
            bind: [messageType],
            type: QueryTypes.SELECT
        });

        if (!result || result.length === 0) {
            console.log(`⚠️ No default template found for message type: ${messageType}`);
            return res.json({
                success: true,
                data: {
                    messageType,
                    template: null,
                    mappings: [],
                    message: 'No default template found for this message type'
                }
            });
        }

        const template = result[0];
        let mappings = [];

        // Extract mappings from template_config
        if (template.template_config) {
            const config = typeof template.template_config === 'string'
                ? JSON.parse(template.template_config)
                : template.template_config;

            // Handle different mapping formats
            if (Array.isArray(config)) {
                mappings = config;
            } else if (config.mappings && Array.isArray(config.mappings)) {
                mappings = config.mappings;
            } else if (config.atomicMappings && Array.isArray(config.atomicMappings)) {
                mappings = config.atomicMappings;
            } else if (config.fieldMappings && Array.isArray(config.fieldMappings)) {
                mappings = config.fieldMappings;
            } else if (config.resources && typeof config.resources === 'object') {
                // OOB template format: mappings are nested under resources.{ResourceType}.mappings
                // e.g., resources.Patient.mappings, resources.Encounter.mappings
                for (const resourceType of Object.keys(config.resources)) {
                    const resource = config.resources[resourceType];
                    if (resource && resource.mappings && Array.isArray(resource.mappings)) {
                        // Normalize mapping field names to match UI expectations
                        const resourceMappings = resource.mappings.map(m => ({
                            // Keep original fields
                            ...m,
                            // Add resourceType for context
                            fhirResource: resourceType,
                            // Normalize field names for UI compatibility
                            hl7Field: m.hl7Path || m.hl7Field || m.sourceField || m.sourcePath,
                            sourcePath: m.hl7Path || m.sourcePath || m.hl7Field,
                            targetPath: m.fhirPath || m.targetPath,
                            dataType: m.hl7DataType || m.dataType || ''
                        }));
                        mappings.push(...resourceMappings);
                    }
                }
            }
        }

        console.log(`✅ Found template "${template.template_name}" with ${mappings.length} mappings`);

        return res.json({
            success: true,
            data: {
                messageType,
                template: {
                    id: template.id,
                    name: template.template_name,
                    description: template.template_description,
                    hl7Version: template.hl7_version,
                    isDefault: template.is_default
                },
                mappings,
                mappingCount: mappings.length
            }
        });

    } catch (error) {
        console.error('❌ Error fetching standard template mappings:', error);
        return res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

// ── Pipeline read/manage operations ────────────────────────────────────────────
// These use direct Sequelize queries (same pattern as savePipeline above).
// Steps are selected WITHOUT the `layer` column (removed in V50 migration).

const STEPS_SELECT = `
    SELECT
        id::text, pipeline_id::text, step_name, step_type, sequence,
        required, timeout_ms, enabled, config, script_type, script_content,
        on_error_strategy, position_x, position_y,
        parent_conditional_step_id::text, branch_type, case_value, step_alias,
        created_at, updated_at
    FROM transformation_steps
    WHERE pipeline_id = $1
    ORDER BY sequence ASC
`;

exports.loadPipeline = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const { id } = req.params;

        const rows = await sequelize.query(
            `SELECT id::text, interface_id::text, message_type, pipeline_name, enabled, version,
                    connections, pipeline_config, created_at, updated_at
             FROM transformation_pipelines WHERE id = $1`,
            { bind: [id], type: QueryTypes.SELECT }
        );
        if (!rows.length) {
            return res.status(404).json({ success: false, error: 'Pipeline not found' });
        }
        const pipeline = rows[0];
        const steps = await sequelize.query(STEPS_SELECT, { bind: [id], type: QueryTypes.SELECT });
        pipeline.execution_groups = [{ steps }];
        res.json({ success: true, pipeline });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.loadPipelineByInterface = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const { interfaceId, messageType } = req.params;

        const rows = await sequelize.query(
            `SELECT id::text, interface_id::text, message_type, pipeline_name, enabled, version,
                    connections, pipeline_config, created_at, updated_at
             FROM transformation_pipelines WHERE interface_id = $1 AND message_type = $2`,
            { bind: [interfaceId, messageType], type: QueryTypes.SELECT }
        );
        if (!rows.length) {
            return res.json({ success: true, pipeline: null });
        }
        const pipeline = rows[0];
        const steps = await sequelize.query(STEPS_SELECT, { bind: [pipeline.id], type: QueryTypes.SELECT });
        pipeline.execution_groups = [{ steps }];
        res.json({ success: true, pipeline });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.listPipelines = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const { interfaceId } = req.params;

        const pipelines = await sequelize.query(
            `SELECT id::text, interface_id::text, message_type, pipeline_name, enabled, version,
                    connections, pipeline_config, created_at, updated_at
             FROM transformation_pipelines WHERE interface_id = $1 ORDER BY created_at DESC`,
            { bind: [interfaceId], type: QueryTypes.SELECT }
        );
        res.json({ success: true, pipelines, count: pipelines.length });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.deletePipeline = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { id } = req.params;
        await sequelize.query('DELETE FROM transformation_steps WHERE pipeline_id = $1', { bind: [id] });
        await sequelize.query('DELETE FROM transformation_pipelines WHERE id = $1', { bind: [id] });
        res.json({ success: true, message: 'Pipeline deleted' });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.clonePipeline = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const { v4: uuidv4 } = require('uuid');
        const { id } = req.params;
        const newName = req.body.new_name || req.body.newName;

        const rows = await sequelize.query(
            `SELECT * FROM transformation_pipelines WHERE id = $1`,
            { bind: [id], type: QueryTypes.SELECT }
        );
        if (!rows.length) {
            return res.status(404).json({ success: false, error: 'Pipeline not found' });
        }
        const src = rows[0];
        const newId = uuidv4();

        await sequelize.query(
            `INSERT INTO transformation_pipelines
                 (id, interface_id, message_type, pipeline_name, enabled, version, connections, pipeline_config, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, 1, $6, $7, NOW(), NOW())`,
            { bind: [newId, src.interface_id, src.message_type, newName || src.pipeline_name + ' (copy)',
                     src.enabled, JSON.stringify(src.connections || []), JSON.stringify(src.pipeline_config || {})] }
        );

        const steps = await sequelize.query(STEPS_SELECT, { bind: [id], type: QueryTypes.SELECT });
        for (const step of steps) {
            await sequelize.query(
                `INSERT INTO transformation_steps
                     (id, pipeline_id, step_name, step_type, sequence, required, timeout_ms, enabled,
                      config, script_type, script_content, on_error_strategy, position_x, position_y,
                      parent_conditional_step_id, branch_type, case_value, step_alias, created_at, updated_at)
                 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NOW(),NOW())`,
                { bind: [uuidv4(), newId, step.step_name, step.step_type, step.sequence,
                         step.required, step.timeout_ms, step.enabled,
                         JSON.stringify(step.config || {}), step.script_type, step.script_content,
                         step.on_error_strategy, step.position_x, step.position_y,
                         step.parent_conditional_step_id, step.branch_type, step.case_value, step.step_alias] }
            );
        }
        res.json({ success: true, pipeline_id: newId, message: 'Pipeline cloned' });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

// Proxy execution to Go backend
exports.executePipeline = async (req, res) => {
    try {
        const response = await axios.post(`${GO_BACKEND_URL}/api/fhir/pipeline/test`, req.body);
        res.json(response.data);
    } catch (error) {
        const status = error.response?.status || 500;
        res.status(status).json(error.response?.data || { success: false, error: error.message });
    }
};

exports.getPipelineStats = async (req, res) => {
    // Stub — execution stats tracking not yet implemented
    res.json({ success: true, stats: { executions: 0, avg_duration_ms: 0, success_rate: 1 } });
};

// Step template stubs (stored in transformation_templates, full implementation in Sprint later)
exports.listTemplates = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const rows = await sequelize.query(
            `SELECT id::text, template_name, step_type, description, config, created_at FROM transformation_templates ORDER BY step_type, template_name`,
            { type: QueryTypes.SELECT }
        );
        res.json({ success: true, templates: rows });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.getTemplate = async (req, res) => {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');
        const rows = await sequelize.query(
            `SELECT id::text, template_name, step_type, description, config, created_at FROM transformation_templates WHERE id = $1`,
            { bind: [req.params.id], type: QueryTypes.SELECT }
        );
        if (!rows.length) return res.status(404).json({ success: false, error: 'Template not found' });
        res.json({ success: true, template: rows[0] });
    } catch (error) {
        res.status(500).json({ success: false, error: error.message });
    }
};

exports.createTemplate = async (req, res) => {
    res.status(501).json({ success: false, error: 'createTemplate not yet implemented' });
};

// Step execution proxied to Go
const _proxyStepExec = async (req, res, stepType) => {
    try {
        const response = await axios.post(`${GO_BACKEND_URL}/api/fhir/pipeline/test`, {
            ...req.body, _step_type: stepType
        });
        res.json(response.data);
    } catch (error) {
        const status = error.response?.status || 500;
        res.status(status).json(error.response?.data || { success: false, error: error.message });
    }
};

exports.executeValidationStep  = (req, res) => _proxyStepExec(req, res, 'validation');
exports.executeEnrichmentStep  = (req, res) => _proxyStepExec(req, res, 'enrichment');
exports.executeMappingStep     = (req, res) => _proxyStepExec(req, res, 'mapping');
exports.executeCustomStep      = (req, res) => _proxyStepExec(req, res, 'custom');

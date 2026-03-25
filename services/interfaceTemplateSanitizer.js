'use strict';
// services/interfaceTemplateSanitizer.js
// Strips sensitive connection details from an interface configuration before
// saving it as a reusable template. Preserves all pipeline step logic.

// ── Field name patterns that are ALWAYS cleared ────────────────────────────
const SENSITIVE_PATTERNS = [
    // Credentials
    'password',
    'secret',
    'token',
    'api_key',
    'access_key',
    'client_secret',
    'private_key',
    'cert_content',
    'certificate_content',
    'bearer',
    // Network / host identifiers
    'host',
    'hostname',
    'ip_address',
    'port',             // environment-specific (e.g. 6610, 5432)
    'server',           // sql_server, server_host, server_address
    'endpoint',
    'url',
    'base_url',
    'fhir_base_url',
    'server_url',
    'connection_string',
    'dsn',
    // Auth identifiers
    'username',
    'client_id',        // OAuth client ID (env-specific per registration)
    'tenant',           // Azure tenant_id, tenant name
    // Database / storage identifiers
    'database',
    'db_name',
    'table_name',       // specific table — env-specific
    'collection',       // MongoDB collection name
    'schema_name',      // DB schema name
    'index_name',       // Elasticsearch index
    'account',          // Snowflake account
    'warehouse',        // Snowflake warehouse
    'workspace_id',     // Databricks workspace
    'cluster',          // Databricks / Kafka cluster
    'project_id',       // GCP project
    // Cloud storage
    'bucket',           // S3 / GCS bucket
    'container_name',   // Azure Blob container
    'region',           // AWS/Azure region (env-specific)
    // Messaging
    'queue_name',       // RabbitMQ / SQS queue
    'topic',            // Kafka topic
    'namespace',        // AMQP / Kafka namespace
    // File system
    'directory',        // local file path
    'folder',           // local file folder
    'file_path',        // explicit file path
];

// ── Field name patterns that are ALWAYS kept even if they match above ───────
// (Checked as a prefix/suffix match against the full field key)
const SAFE_OVERRIDES = [
    'method',         // HTTP method (POST, GET) — not sensitive
    'content_type',   // application/json — not sensitive
    'auth_type',      // 'bearer', 'basic' labels — not values
    'auth_method',
    'mode',           // 'immediate', 'persistent' — not sensitive
    'enable_tls',     // Boolean flag
    'tls',            // e.g. tls_verify (boolean)
    'ssl_mode',       // 'disable', 'require' — standard hints
    'encoding',       // 'UTF-8'
    'max_connections',
    'polling_interval',
    'batch_size',
    'timeout',
    'retry',
    'version',        // FHIR version, protocol version
    'scope',          // OAuth scope — not a secret itself
    'sending_app',    // MSH-3 override (app name, not credential)
    'sending_facility',
    'ack',            // ACK config objects
    'on_error',
    'text_success',
    'text_error',
];

/**
 * Returns true if a field key should be cleared.
 * Applies case-insensitive substring matching.
 */
function isSensitiveKey(key) {
    const lower = key.toLowerCase();

    // Check safe overrides first — if any safe pattern matches, keep it
    for (const safe of SAFE_OVERRIDES) {
        if (lower === safe || lower.startsWith(safe + '_') || lower.endsWith('_' + safe)) {
            return false;
        }
    }

    // Check sensitive patterns
    for (const pattern of SENSITIVE_PATTERNS) {
        if (lower.includes(pattern)) {
            return true;
        }
    }

    return false;
}

/**
 * Recursively sanitize a config object.
 * Returns { sanitized: object, clearedFields: string[] }
 */
function sanitizeConfig(config, parentKey = '') {
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
        return { sanitized: config, clearedFields: [] };
    }

    const sanitized = {};
    const clearedFields = [];

    for (const [key, value] of Object.entries(config)) {
        const fullKey = parentKey ? `${parentKey}.${key}` : key;

        if (isSensitiveKey(key)) {
            // Clear the value but keep the key so UI knows it exists
            sanitized[key] = '';
            clearedFields.push(fullKey);
        } else if (value && typeof value === 'object' && !Array.isArray(value)) {
            // Recurse into nested objects (e.g. ack config, oauth2 config)
            const nested = sanitizeConfig(value, fullKey);
            sanitized[key] = nested.sanitized;
            clearedFields.push(...nested.clearedFields);
        } else {
            sanitized[key] = value;
        }
    }

    return { sanitized, clearedFields };
}

/**
 * Build a user-facing manifest of fields that must be filled in.
 * @param {string[]} clearedFields - dot-notation paths that were cleared
 * @param {string} section - 'source' or 'target'
 * @returns {object[]} - [{section, field, label, type, hint}]
 */
function buildRequiredFieldsManifest(clearedFields, section) {
    // Friendly labels for common fields
    const LABELS = {
        port: { label: 'Port', type: 'number', hint: 'e.g. 2575 for MLLP, 5432 for PostgreSQL' },
        host: { label: 'Host / IP Address', type: 'string', hint: 'e.g. 192.168.1.10 or ehr.hospital.org' },
        hostname: { label: 'Hostname', type: 'string', hint: '' },
        endpoint: { label: 'Endpoint URL', type: 'url', hint: 'e.g. https://fhir.hospital.org/R4' },
        url: { label: 'URL', type: 'url', hint: '' },
        base_url: { label: 'Base URL', type: 'url', hint: '' },
        fhir_base_url: { label: 'FHIR Base URL', type: 'url', hint: 'e.g. https://fhir.hospital.org/api/FHIR/R4' },
        server_url: { label: 'Server URL', type: 'url', hint: '' },
        username: { label: 'Username', type: 'string', hint: '' },
        password: { label: 'Password', type: 'password', hint: '' },
        database: { label: 'Database Name', type: 'string', hint: '' },
        db_name: { label: 'Database Name', type: 'string', hint: '' },
        api_key: { label: 'API Key', type: 'password', hint: '' },
        access_key: { label: 'Access Key', type: 'password', hint: '' },
        secret: { label: 'Secret', type: 'password', hint: '' },
        token: { label: 'Token', type: 'password', hint: '' },
        bearer: { label: 'Bearer Token', type: 'password', hint: '' },
        client_secret: { label: 'Client Secret', type: 'password', hint: '' },
        private_key: { label: 'Private Key', type: 'textarea', hint: 'PEM-format private key' },
        cert_content: { label: 'Certificate Content', type: 'textarea', hint: 'PEM-format certificate' },
        connection_string: { label: 'Connection String', type: 'string', hint: '' },
        dsn: { label: 'DSN / Connection String', type: 'string', hint: '' },
        account: { label: 'Account', type: 'string', hint: '' },
        bucket: { label: 'Bucket Name', type: 'string', hint: '' },
        container_name: { label: 'Container Name', type: 'string', hint: '' },
        queue_name: { label: 'Queue Name', type: 'string', hint: '' },
        topic: { label: 'Kafka Topic', type: 'string', hint: '' },
        project_id: { label: 'GCP Project ID', type: 'string', hint: '' },
        ip_address: { label: 'IP Address', type: 'string', hint: '' },
    };

    return clearedFields.map(fullField => {
        // Get just the leaf key (last segment after last dot)
        const leafKey = fullField.split('.').pop();
        const meta = LABELS[leafKey] || { label: leafKey.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()), type: 'string', hint: '' };

        return {
            section,
            field: leafKey,
            full_path: fullField,
            label: meta.label,
            type: meta.type,
            hint: meta.hint,
            required: ['host', 'hostname', 'endpoint', 'url', 'base_url', 'fhir_base_url', 'username', 'password', 'database', 'connection_string', 'dsn'].includes(leafKey),
        };
    });
}

/**
 * Build a simplified preview_steps array from pipeline_config execution_groups.
 * Used for the gallery card display (no sensitive data in steps).
 */
function buildPreviewSteps(pipelineConfig) {
    const STEP_ICONS = {
        'enrichment.validation': '✅',
        'validation': '✅',
        'hl7_fhir_transform': '🔄',
        'field_mapping': '🗺️',
        'enrichment.script': '📝',
        'enrichment.api': '🌐',
        'enrichment.database': '💾',
        'data_masking': '🔒',
        'normalizer': '📊',
        'remove_duplicates': '🔁',
        'connector.inbound': '📥',
        'connector.outbound': '📤',
        'loop': '🔁',
        'conditional': '🔀',
    };

    const steps = [];
    try {
        const groups = pipelineConfig?.execution_groups || [];
        for (const group of groups) {
            for (const step of (group.steps || [])) {
                const type = step.step_type || step.type || '';
                steps.push({
                    name: step.step_name || step.name || type,
                    icon: STEP_ICONS[type] || '⚙️',
                    step_type: type,
                });
            }
        }
    } catch (_) {
        // Silently ignore malformed pipeline_config
    }
    return steps;
}

/**
 * Sanitize all step configs within a pipeline's execution_groups.
 * Returns { sanitizedPipeline, clearedFields }
 * clearedFields entries are prefixed: "pipeline.groupN.stepName.field"
 */
// Step types that contain environment-specific connection details.
// Their configs are sanitized: sensitive fields (host, port, password, endpoint, etc.)
// are cleared, but business-logic fields (query, method, content_type, script, etc.) are kept.
//
// NOT included (business logic only — no credentials):
//   hl7_fhir_transform, enrichment.validation, field_mapping, enrichment.script,
//   data_masking, remove_duplicates, normalizer, conditional, loop, etc.
const CONNECTOR_STEP_TYPES = new Set([
    // Inbound / outbound connector pipeline steps
    'connector.inbound',
    'connector.outbound',
    // Database enrichment steps (host, port, db, username, password — preserve query)
    'enrichment.database',
    // API enrichment steps (endpoint, token, api_key — preserve method, content_type, script)
    'enrichment.api',
]);

function sanitizePipelineConfig(pipeline) {
    if (!pipeline || !Array.isArray(pipeline.execution_groups)) {
        return { sanitizedPipeline: pipeline || {}, clearedFields: [] };
    }

    const clearedFields = [];
    const sanitizedGroups = pipeline.execution_groups.map((group, groupIdx) => {
        const sanitizedSteps = (group.steps || []).map(step => {
            const stepType = step.step_type || step.type || '';

            // Only sanitize connector steps — leave all other steps untouched
            if (!CONNECTOR_STEP_TYPES.has(stepType)) return step;
            if (!step.config || typeof step.config !== 'object') return step;

            const stepLabel = step.step_name || step.name || stepType || `step${groupIdx}`;
            const result = sanitizeConfig(step.config, `pipeline.group${groupIdx}.${stepLabel}`);
            result.clearedFields.forEach(f => clearedFields.push(f));

            return { ...step, config: result.sanitized };
        });
        return { ...group, steps: sanitizedSteps };
    });

    return {
        sanitizedPipeline: { ...pipeline, execution_groups: sanitizedGroups },
        clearedFields,
    };
}

/**
 * Main entry point: sanitize a full interface record for use as a template.
 *
 * @param {object} iface - The interface object with connectivity and pipeline data
 * @param {object} connectivity - {source_connector_type, source_config, target_connector_type, target_config}
 * @param {object} pipeline - {execution_groups: [...]}
 * @returns {object} - Ready-to-insert template record (without id/slug/name/category — caller fills those)
 */
/**
 * Extract connector types and cleared field manifests from pipeline steps.
 * Connectors are typically stored as connector.inbound / connector.outbound steps
 * rather than in interface_connectivity when using the pipeline-centric approach.
 */
function extractPipelineConnectorInfo(sanitizedPipeline) {
    let sourceConnectorType = null;
    let targetConnectorType = null;
    const clearedFieldsByStep = []; // [{stepLabel, section, clearedFields}]

    const groups = sanitizedPipeline?.execution_groups || [];
    for (const group of groups) {
        for (const step of (group.steps || [])) {
            const type = step.step_type || step.type || '';
            const cfg = step.config || {};

            if (type === 'connector.inbound' && cfg.connectorType) {
                sourceConnectorType = cfg.connectorType;
                // Collect cleared fields from this step's config
                const { clearedFields } = sanitizeConfig(cfg);
                if (clearedFields.length) {
                    clearedFieldsByStep.push({
                        stepLabel: step.step_name || 'Inbound Connector',
                        section: 'source',
                        clearedFields,
                    });
                }
            } else if (type === 'connector.outbound' && cfg.connectorType) {
                targetConnectorType = cfg.connectorType;
                const { clearedFields } = sanitizeConfig(cfg);
                if (clearedFields.length) {
                    clearedFieldsByStep.push({
                        stepLabel: step.step_name || 'Outbound Connector',
                        section: 'target',
                        clearedFields,
                    });
                }
            }
        }
    }

    return { sourceConnectorType, targetConnectorType, clearedFieldsByStep };
}

function sanitizeInterfaceForTemplate(iface, connectivity, pipeline) {
    const sourceResult = sanitizeConfig(connectivity?.source_config || {});
    const targetResult = sanitizeConfig(connectivity?.target_config || {});
    const pipelineResult = sanitizePipelineConfig(pipeline);

    // Extract connector types + cleared field info from pipeline steps
    // (used when connectors are stored as pipeline steps rather than interface_connectivity)
    const pipelineConnInfo = extractPipelineConnectorInfo(pipelineResult.sanitizedPipeline);

    const allSanitizedFields = [
        ...sourceResult.clearedFields.map(f => `source.${f}`),
        ...targetResult.clearedFields.map(f => `target.${f}`),
        ...pipelineResult.clearedFields,
    ];

    // Build required fields manifest: legacy connectivity fields + pipeline step fields
    const requiredFields = [
        ...buildRequiredFieldsManifest(sourceResult.clearedFields, 'source'),
        ...buildRequiredFieldsManifest(targetResult.clearedFields, 'target'),
        ...pipelineConnInfo.clearedFieldsByStep.flatMap(({ clearedFields, section }) =>
            buildRequiredFieldsManifest(clearedFields, section)
        ),
    ];

    // Deduplicate required fields by section+field
    const seen = new Set();
    const uniqueRequiredFields = requiredFields.filter(f => {
        const key = `${f.section}:${f.field}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
    });

    const previewSteps = buildPreviewSteps(pipeline);

    return {
        // Prefer explicit connectivity type; fall back to what we found in pipeline steps
        source_connector_type: connectivity?.source_connector_type || pipelineConnInfo.sourceConnectorType || null,
        source_config_template: sourceResult.sanitized,
        target_connector_type: connectivity?.target_connector_type || pipelineConnInfo.targetConnectorType || null,
        target_config_template: targetResult.sanitized,
        pipeline_config: pipelineResult.sanitizedPipeline,
        message_type: iface?.message_type || null,
        required_connection_fields: uniqueRequiredFields,
        sanitized_fields: allSanitizedFields,
        preview_steps: previewSteps,
    };
}

/**
 * Merge a template's scaffold config with user-provided connection details
 * to produce a ready-to-use interface connectivity config.
 *
 * @param {object} templateConfig - sanitized config from template
 * @param {object} userValues - user-provided {field: value} pairs
 * @returns {object} - merged config
 */
function mergeTemplateWithUserValues(templateConfig, userValues) {
    if (!userValues || typeof userValues !== 'object') return { ...templateConfig };
    const merged = { ...templateConfig };
    for (const [key, value] of Object.entries(userValues)) {
        if (value !== undefined && value !== null && value !== '') {
            merged[key] = value;
        }
    }
    return merged;
}

module.exports = {
    sanitizeConfig,
    sanitizePipelineConfig,
    buildRequiredFieldsManifest,
    buildPreviewSteps,
    sanitizeInterfaceForTemplate,
    mergeTemplateWithUserValues,
    // Expose patterns for testing / extension
    SENSITIVE_PATTERNS,
    SAFE_OVERRIDES,
    isSensitiveKey,
};

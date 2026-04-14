'use strict';
// controllers/interfaceTemplateController.js
// Interface-level template CRUD: browse, preview, use, and save-as-template.
// Distinct from templateController.js which handles pipeline *step* templates.

const { sequelize } = require('../config/database');
const { QueryTypes } = require('sequelize');
const { v4: uuidv4 } = require('uuid');
const sanitizer = require('../services/interfaceTemplateSanitizer');

// ── Helpers ─────────────────────────────────────────────────────────────────

function generateSlug(name) {
    return name
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '')
        .substring(0, 80);
}

async function ensureUniqueSlug(baseSlug) {
    let slug = baseSlug;
    let suffix = 0;
    while (true) {
        const existing = await sequelize.query(
            'SELECT id FROM interface_templates WHERE slug = $1',
            { bind: [slug], type: QueryTypes.SELECT }
        );
        if (existing.length === 0) return slug;
        suffix++;
        slug = `${baseSlug}-${suffix}`;
    }
}

// ── List templates ───────────────────────────────────────────────────────────

/**
 * GET /api/interface-templates
 * Query params: category, search, tags (comma-separated), include_categories
 */
exports.listTemplates = async (req, res) => {
    try {
        const { category, search, tags, include_categories } = req.query;

        const conditions = ['(is_public = true OR is_system = true'];
        const bindings = [];
        let bi = 1;

        if (req.user) {
            conditions[0] += ` OR created_by_user_id = $${bi++})`;
            bindings.push(req.user.id);
        } else {
            conditions[0] += ')';
        }

        if (category && category !== 'all') {
            conditions.push(`category = $${bi++}`);
            bindings.push(category);
        }

        if (search) {
            conditions.push(`(name ILIKE $${bi} OR description ILIKE $${bi} OR $${bi} = ANY(tags))`);
            bindings.push(`%${search}%`);
            bi++;
        }

        if (tags) {
            const tagList = tags.split(',').map(t => t.trim()).filter(Boolean);
            if (tagList.length > 0) {
                conditions.push(`tags && $${bi++}`);
                bindings.push(tagList);
            }
        }

        const where = conditions.length ? `WHERE ${conditions.join(' AND ')}` : '';

        const templates = await sequelize.query(`
            SELECT
                id::text,
                name,
                slug,
                description,
                category,
                subcategory,
                tags,
                icon,
                difficulty,
                source_connector_type,
                target_connector_type,
                message_type,
                preview_steps,
                estimated_setup_minutes,
                is_system,
                is_public,
                author,
                created_by_user_id::text,
                usage_count,
                (user_guide IS NOT NULL AND user_guide <> '') AS has_guide,
                created_at,
                updated_at
            FROM interface_templates
            ${where}
            ORDER BY is_system DESC, usage_count DESC, name ASC
        `, { bind: bindings, type: QueryTypes.SELECT });

        let categories = null;
        if (include_categories === 'true' || include_categories === '1') {
            const catRows = await sequelize.query(`
                SELECT category, COUNT(*)::int AS count
                FROM interface_templates
                WHERE is_public = true OR is_system = true
                GROUP BY category
                ORDER BY count DESC
            `, { type: QueryTypes.SELECT });
            categories = catRows;
        }

        res.json({
            success: true,
            count: templates.length,
            templates,
            ...(categories !== null && { categories }),
        });
    } catch (err) {
        console.error('[InterfaceTemplates] listTemplates error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Category list ────────────────────────────────────────────────────────────

/**
 * GET /api/interface-templates/categories
 */
exports.listCategories = async (req, res) => {
    try {
        const rows = await sequelize.query(`
            SELECT
                category,
                COUNT(*)::int AS count,
                array_agg(DISTINCT subcategory) FILTER (WHERE subcategory IS NOT NULL) AS subcategories
            FROM interface_templates
            WHERE is_public = true OR is_system = true
            GROUP BY category
            ORDER BY count DESC
        `, { type: QueryTypes.SELECT });

        res.json({ success: true, categories: rows });
    } catch (err) {
        console.error('[InterfaceTemplates] listCategories error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Get single template ──────────────────────────────────────────────────────

/**
 * GET /api/interface-templates/:id
 * Returns full detail including pipeline_config and required_connection_fields.
 */
exports.getTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        // Accept both UUID and slug
        const field = id.includes('-') && id.length === 36 ? 'id::text' : 'slug';
        const rows = await sequelize.query(`
            SELECT
                id::text,
                name,
                slug,
                description,
                category,
                subcategory,
                tags,
                icon,
                difficulty,
                source_connector_type,
                source_config_template,
                target_connector_type,
                target_config_template,
                pipeline_config,
                message_type,
                required_connection_fields,
                sanitized_fields,
                preview_steps,
                estimated_setup_minutes,
                is_system,
                is_public,
                author,
                created_by_user_id::text,
                usage_count,
                user_guide,
                created_at,
                updated_at
            FROM interface_templates
            WHERE ${field} = $1
        `, { bind: [id], type: QueryTypes.SELECT });

        if (rows.length === 0) {
            return res.status(404).json({ success: false, error: 'Template not found' });
        }

        res.json({ success: true, template: rows[0] });
    } catch (err) {
        console.error('[InterfaceTemplates] getTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Create template (from scratch) ──────────────────────────────────────────

/**
 * POST /api/interface-templates
 * Body: { name, description, category, subcategory, tags, icon,
 *         source_connector_type, source_config_template,
 *         target_connector_type, target_config_template,
 *         pipeline_config, message_type, is_public }
 */
exports.createTemplate = async (req, res) => {
    try {
        const {
            name, description, category = 'custom', subcategory, tags = [],
            icon = '📋', difficulty = 'intermediate',
            source_connector_type, source_config_template = {},
            target_connector_type, target_config_template = {},
            pipeline_config = {}, message_type, is_public = true,
        } = req.body;

        if (!name) {
            return res.status(400).json({ success: false, error: 'name is required' });
        }

        const id = uuidv4();
        const slug = await ensureUniqueSlug(generateSlug(name));
        const userId = req.user ? req.user.id : null;
        const previewSteps = sanitizer.buildPreviewSteps(pipeline_config);

        await sequelize.query(`
            INSERT INTO interface_templates
                (id, name, slug, description, category, subcategory, tags, icon, difficulty,
                 source_connector_type, source_config_template,
                 target_connector_type, target_config_template,
                 pipeline_config, message_type,
                 required_connection_fields, sanitized_fields, preview_steps,
                 is_system, is_public, author, created_by_user_id, usage_count)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
                    false,$19,$20,$21,0)
        `, {
            bind: [
                id, name, slug, description || null, category, subcategory || null,
                tags, icon, difficulty,
                source_connector_type || null, JSON.stringify(source_config_template),
                target_connector_type || null, JSON.stringify(target_config_template),
                JSON.stringify(pipeline_config), message_type || null,
                '{}', '{}', JSON.stringify(previewSteps),
                is_public, userId ? `${userId}` : null, userId,
            ],
            type: QueryTypes.INSERT,
        });

        console.log(`✅ [InterfaceTemplates] Created: ${id} - ${name}`);
        res.status(201).json({ success: true, template_id: id, slug });
    } catch (err) {
        console.error('[InterfaceTemplates] createTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Save existing interface as template ──────────────────────────────────────

/**
 * POST /api/interfaces/:interfaceId/save-as-template
 * Body: { name, description, category, subcategory, tags, is_public }
 * Reads current interface + connectivity + pipeline, sanitizes, stores as template.
 */
exports.saveInterfaceAsTemplate = async (req, res) => {
    try {
        const { interfaceId } = req.params;
        const {
            name, description, category = 'custom', subcategory, tags = [],
            icon = '📋', difficulty = 'intermediate', is_public = true,
        } = req.body;

        if (!name) {
            return res.status(400).json({ success: false, error: 'name is required' });
        }

        // Load interface
        const ifaceRows = await sequelize.query(`
            SELECT id::text, name, message_type FROM interfaces WHERE id = $1
        `, { bind: [interfaceId], type: QueryTypes.SELECT });

        if (ifaceRows.length === 0) {
            return res.status(404).json({ success: false, error: 'Interface not found' });
        }

        // Load connectivity (from Go backend via direct DB query)
        let connectivity = { source_config: {}, target_config: {} };
        try {
            const connRows = await sequelize.query(`
                SELECT
                    ct_src.type_name AS source_connector_type,
                    ic.source_config,
                    ct_tgt.type_name AS target_connector_type,
                    ic.target_config
                FROM interface_connectivity ic
                LEFT JOIN connectivity_types ct_src ON ic.source_connectivity_type_id = ct_src.id
                LEFT JOIN connectivity_types ct_tgt ON ic.target_connectivity_type_id = ct_tgt.id
                WHERE ic.interface_id = $1
            `, { bind: [interfaceId], type: QueryTypes.SELECT });

            if (connRows.length > 0) {
                connectivity = connRows[0];
            }
        } catch (_) {
            // connectivity_types table may not exist in all environments — continue
        }

        // Load pipeline
        let pipeline = {};
        try {
            // Capture ALL step fields so scripts, conditionals, positions are preserved.
            // Also captures script_content (for enrichment.script steps), parent_conditional_step_id
            // (for if-then-else/switch-case branch steps), and other fields needed for full fidelity.
            const pipelineRows = await sequelize.query(`
                SELECT tp.id::text,
                       json_agg(
                           json_build_object(
                               'sequence', ts.sequence,
                               'steps', (
                                   SELECT json_agg(
                                       json_build_object(
                                           'id', ts2.id::text,
                                           'step_name', ts2.step_name,
                                           'step_type', ts2.step_type,
                                           'sequence', ts2.sequence,
                                           'config', ts2.config,
                                           'enabled', ts2.enabled,
                                           'required', ts2.required,
                                           'script_type', ts2.script_type,
                                           'script_content', ts2.script_content,
                                           'on_error_strategy', ts2.on_error_strategy,
                                           'timeout_ms', ts2.timeout_ms,
                                           'position_x', ts2.position_x,
                                           'position_y', ts2.position_y,
                                           'parent_conditional_step_id', ts2.parent_conditional_step_id::text,
                                           'branch_type', ts2.branch_type,
                                           'case_value', ts2.case_value,
                                           'step_alias', ts2.step_alias
                                       )
                                   )
                                   FROM transformation_steps ts2
                                   WHERE ts2.pipeline_id = tp.id AND ts2.sequence = ts.sequence
                               )
                           )
                           ORDER BY ts.sequence
                       ) AS execution_groups
                FROM transformation_pipelines tp
                LEFT JOIN transformation_steps ts ON ts.pipeline_id = tp.id
                WHERE tp.interface_id = $1
                GROUP BY tp.id
                LIMIT 1
            `, { bind: [interfaceId], type: QueryTypes.SELECT });

            if (pipelineRows.length > 0) {
                pipeline = { execution_groups: pipelineRows[0].execution_groups || [] };
            }
        } catch (_) {
            // Pipeline may not exist yet — continue with empty
        }

        // Ensure every HL7→FHIR step in the pipeline has embedded_mappings before saving as template.
        // Source priority (highest to lowest):
        //   1. Step's own embedded_mappings (non-empty)
        //   2. interface_message_mappings (wizard store for this interface)
        //   3. transformation_templates.default_config (when step uses use_template + template_id)
        const HL7_FHIR_TYPES = new Set(['hl7_fhir_transform', 'core.mapping']);
        try {
            // Source 2: interface_message_mappings
            const wizzRows = await sequelize.query(`
                SELECT message_type, custom_mapping_config
                FROM interface_message_mappings
                WHERE interface_id = $1 AND custom_mapping_config IS NOT NULL
            `, { bind: [interfaceId], type: QueryTypes.SELECT });

            const wizzMap = {};
            for (const row of wizzRows) {
                if (row.custom_mapping_config) {
                    const cfg = typeof row.custom_mapping_config === 'string'
                        ? JSON.parse(row.custom_mapping_config)
                        : row.custom_mapping_config;
                    if (cfg.atomicMappings && cfg.atomicMappings.length > 0) {
                        wizzMap[row.message_type] = cfg;
                    }
                }
            }

            // Source 3: interfaces.transformation_mapping (legacy wizard store — same column savePipeline COALESCEs)
            let legacyMapping = null;
            try {
                const legacyRows = await sequelize.query(`
                    SELECT transformation_mapping FROM interfaces WHERE id = $1 AND transformation_mapping IS NOT NULL
                `, { bind: [interfaceId], type: QueryTypes.SELECT });
                if (legacyRows.length > 0 && legacyRows[0].transformation_mapping) {
                    const lm = typeof legacyRows[0].transformation_mapping === 'string'
                        ? JSON.parse(legacyRows[0].transformation_mapping)
                        : legacyRows[0].transformation_mapping;
                    const lmMappings = lm && (lm.atomicMappings || lm.mappings || (Array.isArray(lm) ? lm : null));
                    if (lmMappings && lmMappings.length > 0) {
                        legacyMapping = Array.isArray(lmMappings)
                            ? { atomicMappings: lmMappings }
                            : lm;
                        console.log(`[SaveTemplate] Found ${lmMappings.length} mappings in interfaces.transformation_mapping`);
                    }
                }
            } catch (e) { /* column may not exist in all envs */ }

            // Scan and log all HL7→FHIR steps
            const allStepsFlat = (pipeline.execution_groups || []).flatMap(g => g.steps || []);
            const hl7Steps = allStepsFlat.filter(s => HL7_FHIR_TYPES.has(s.step_type || s.type || ''));
            console.log(`[SaveTemplate] ${hl7Steps.length} HL7→FHIR steps | wizzMap keys: [${Object.keys(wizzMap)}] | legacyMapping: ${!!legacyMapping}`);

            if (pipeline.execution_groups) {
                pipeline.execution_groups = pipeline.execution_groups.map(group => ({
                    ...group,
                    steps: (group.steps || []).map(step => {
                        const sType = step.step_type || step.type || '';
                        if (!HL7_FHIR_TYPES.has(sType)) return step;

                        // Source 1: step already has real embedded_mappings
                        const emb = step.config && step.config.embedded_mappings;
                        const embHasData = emb && (
                            (Array.isArray(emb) && emb.length > 0) ||
                            (emb.atomicMappings && emb.atomicMappings.length > 0) ||
                            (emb.mappings && emb.mappings.length > 0)
                        );
                        if (embHasData) {
                            console.log(`[SaveTemplate] Step "${step.step_name}": keeping existing embedded_mappings (${(emb.atomicMappings || emb).length} mappings)`);
                            return step;
                        }

                        // Source 2: wizard store
                        const msgType = step.config && step.config.message_type;
                        const wizzMappings = (msgType && wizzMap[msgType]) || Object.values(wizzMap)[0];
                        if (wizzMappings) {
                            console.log(`[SaveTemplate] Step "${step.step_name}": injecting ${wizzMappings.atomicMappings.length} wizard mappings`);
                            return { ...step, config: { ...(step.config || {}), embedded_mappings: wizzMappings } };
                        }

                        // Source 3: legacy interfaces.transformation_mapping
                        if (legacyMapping) {
                            const count = (legacyMapping.atomicMappings || []).length;
                            console.log(`[SaveTemplate] Step "${step.step_name}": embedding ${count} mappings from interfaces.transformation_mapping`);
                            return { ...step, config: { ...(step.config || {}), embedded_mappings: legacyMapping } };
                        }

                        console.warn(`[SaveTemplate] Step "${step.step_name}": no mappings found from any source — template will show blank`);
                        return step;
                    }),
                }));
            }
        } catch (err) {
            console.error('[SaveTemplate] Mapping injection failed:', err.message);
        }

        // Sanitize
        const iface = ifaceRows[0];
        const sanitized = sanitizer.sanitizeInterfaceForTemplate(iface, connectivity, pipeline);

        // Create template record
        const id = uuidv4();
        const slug = await ensureUniqueSlug(generateSlug(name));
        const userId = req.user ? req.user.id : null;
        const author = req.user ? (req.user.name || req.user.email) : null;

        await sequelize.query(`
            INSERT INTO interface_templates
                (id, name, slug, description, category, subcategory, tags, icon, difficulty,
                 source_connector_type, source_config_template,
                 target_connector_type, target_config_template,
                 pipeline_config, message_type,
                 required_connection_fields, sanitized_fields, preview_steps,
                 estimated_setup_minutes, is_system, is_public, author,
                 created_by_user_id, usage_count)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
                    15, false,$19,$20,$21,0)
        `, {
            bind: [
                id, name, slug, description || null, category, subcategory || null,
                tags, icon, difficulty,
                sanitized.source_connector_type, JSON.stringify(sanitized.source_config_template),
                sanitized.target_connector_type, JSON.stringify(sanitized.target_config_template),
                JSON.stringify(sanitized.pipeline_config), sanitized.message_type,
                JSON.stringify(sanitized.required_connection_fields),
                Array.isArray(sanitized.sanitized_fields) ? sanitized.sanitized_fields : [],
                JSON.stringify(sanitized.preview_steps),
                is_public, author, userId,
            ],
            type: QueryTypes.INSERT,
        });

        console.log(`✅ [InterfaceTemplates] Saved interface ${interfaceId} as template ${id}`);
        res.status(201).json({
            success: true,
            template_id: id,
            slug,
            sanitized_fields: sanitized.sanitized_fields,
            required_connection_fields: sanitized.required_connection_fields,
        });
    } catch (err) {
        console.error('[InterfaceTemplates] saveInterfaceAsTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Preview sanitization (dry-run, no save) ──────────────────────────────────

/**
 * POST /api/interfaces/:interfaceId/template-preview
 * Returns what would be cleared if saved as template. No write operations.
 */
exports.previewSanitization = async (req, res) => {
    try {
        const { source_config = {}, target_config = {} } = req.body;

        const sourceResult = sanitizer.sanitizeConfig(source_config);
        const targetResult = sanitizer.sanitizeConfig(target_config);

        res.json({
            success: true,
            source: {
                sanitized: sourceResult.sanitized,
                cleared_fields: sourceResult.clearedFields,
            },
            target: {
                sanitized: targetResult.sanitized,
                cleared_fields: targetResult.clearedFields,
            },
            required_connection_fields: [
                ...sanitizer.buildRequiredFieldsManifest(sourceResult.clearedFields, 'source'),
                ...sanitizer.buildRequiredFieldsManifest(targetResult.clearedFields, 'target'),
            ],
        });
    } catch (err) {
        console.error('[InterfaceTemplates] previewSanitization error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Use a template (instantiate as a new interface config) ───────────────────

/**
 * POST /api/interface-templates/:id/use
 * Body: { source_values: {field: value}, target_values: {field: value},
 *         interface_name: "My New Interface" }
 * Returns a complete interface scaffold the frontend can use to create an interface.
 * Does NOT create the interface — that's the frontend's job.
 */
exports.useTemplate = async (req, res) => {
    try {
        const { id } = req.params;
        const { source_values = {}, target_values = {}, interface_name } = req.body;

        // Load template — cast id to text to avoid uuid vs varchar type mismatch in OR clause
        const rows = await sequelize.query(`
            SELECT * FROM interface_templates WHERE id::text = $1 OR slug = $1
        `, { bind: [id], type: QueryTypes.SELECT });

        if (rows.length === 0) {
            return res.status(404).json({ success: false, error: 'Template not found' });
        }

        const tmpl = rows[0];

        // Merge user-provided values into sanitized configs
        const sourceConfig = sanitizer.mergeTemplateWithUserValues(
            tmpl.source_config_template || {}, source_values
        );
        const targetConfig = sanitizer.mergeTemplateWithUserValues(
            tmpl.target_config_template || {}, target_values
        );

        // Increment usage count (fire-and-forget)
        sequelize.query(
            'UPDATE interface_templates SET usage_count = usage_count + 1 WHERE id = $1',
            { bind: [tmpl.id], type: QueryTypes.UPDATE }
        ).catch(() => {});

        res.json({
            success: true,
            interface_scaffold: {
                name: interface_name || `${tmpl.name} (from template)`,
                message_type: tmpl.message_type,
                source_connector_type: tmpl.source_connector_type,
                source_config: sourceConfig,
                target_connector_type: tmpl.target_connector_type,
                target_config: targetConfig,
                pipeline_config: tmpl.pipeline_config,
                template_id: tmpl.id,
                template_name: tmpl.name,
            },
        });
    } catch (err) {
        console.error('[InterfaceTemplates] useTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Update template ──────────────────────────────────────────────────────────

/**
 * PUT /api/interface-templates/:id
 */
exports.updateTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        const rows = await sequelize.query(
            'SELECT is_system, created_by_user_id::text FROM interface_templates WHERE id = $1',
            { bind: [id], type: QueryTypes.SELECT }
        );
        if (rows.length === 0) return res.status(404).json({ success: false, error: 'Template not found' });

        const tmpl = rows[0];
        if (tmpl.is_system) return res.status(403).json({ success: false, error: 'System templates cannot be modified' });

        const userId = req.user ? String(req.user.id) : null;
        if (userId && tmpl.created_by_user_id !== userId && !req.user?.isAdmin) {
            return res.status(403).json({ success: false, error: 'Permission denied' });
        }

        const allowed = ['name', 'description', 'category', 'subcategory', 'tags', 'icon',
            'difficulty', 'is_public', 'source_config_template', 'target_config_template',
            'pipeline_config', 'message_type', 'estimated_setup_minutes'];

        const sets = ['updated_at = NOW()'];
        const bindings = [];
        let bi = 1;

        for (const field of allowed) {
            if (req.body[field] !== undefined) {
                const val = typeof req.body[field] === 'object' ? JSON.stringify(req.body[field]) : req.body[field];
                sets.push(`${field} = $${bi++}`);
                bindings.push(val);
            }
        }

        if (sets.length === 1) return res.json({ success: true, message: 'No changes' });

        bindings.push(id);
        await sequelize.query(
            `UPDATE interface_templates SET ${sets.join(', ')} WHERE id = $${bi}`,
            { bind: bindings, type: QueryTypes.UPDATE }
        );

        res.json({ success: true, message: 'Template updated' });
    } catch (err) {
        console.error('[InterfaceTemplates] updateTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

// ── Delete template ──────────────────────────────────────────────────────────

/**
 * DELETE /api/interface-templates/:id
 */
exports.deleteTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        const rows = await sequelize.query(
            'SELECT is_system, created_by_user_id::text, name FROM interface_templates WHERE id = $1',
            { bind: [id], type: QueryTypes.SELECT }
        );
        if (rows.length === 0) return res.status(404).json({ success: false, error: 'Template not found' });

        const tmpl = rows[0];
        if (tmpl.is_system) return res.status(403).json({ success: false, error: 'System templates cannot be deleted' });

        const userId = req.user ? String(req.user.id) : null;
        if (userId && tmpl.created_by_user_id !== userId && !req.user?.isAdmin) {
            return res.status(403).json({ success: false, error: 'Permission denied' });
        }

        await sequelize.query('DELETE FROM interface_templates WHERE id = $1', { bind: [id], type: QueryTypes.DELETE });

        console.log(`✅ [InterfaceTemplates] Deleted: ${id} - ${tmpl.name}`);
        res.json({ success: true, message: `Template "${tmpl.name}" deleted` });
    } catch (err) {
        console.error('[InterfaceTemplates] deleteTemplate error:', err);
        res.status(500).json({ success: false, error: err.message });
    }
};

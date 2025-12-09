// controllers/templateController.js
// Template CRUD operations for reusable transformation steps

const { sequelize } = require('../config/database');
const { QueryTypes } = require('sequelize');
const { v4: uuidv4 } = require('uuid');

/**
 * List all templates (system + user templates)
 * GET /api/templates
 *
 * Query params:
 * - layer: Filter by layer (pre, core, post)
 * - is_public: Filter public/private
 * - user_id: Filter by creator
 */
exports.listTemplates = async (req, res) => {
    try {
        const { layer, is_public, user_id } = req.query;

        let whereConditions = [];
        let bindings = [];
        let bindIndex = 1;

        // Filter by layer
        if (layer) {
            whereConditions.push(`layer = $${bindIndex}`);
            bindings.push(layer);
            bindIndex++;
        }

        // Filter by visibility (show system + public + own private)
        if (is_public !== undefined) {
            whereConditions.push(`is_public = $${bindIndex}`);
            bindings.push(is_public === 'true');
            bindIndex++;
        } else if (req.user) {
            // Show: system templates + public templates + user's private templates
            whereConditions.push(`(is_system = true OR is_public = true OR created_by_user_id = $${bindIndex})`);
            bindings.push(req.user.id);
            bindIndex++;
        } else {
            // Not authenticated - show only system and public
            whereConditions.push(`(is_system = true OR is_public = true)`);
        }

        // Filter by creator
        if (user_id) {
            whereConditions.push(`created_by_user_id = $${bindIndex}`);
            bindings.push(user_id);
            bindIndex++;
        }

        const whereClause = whereConditions.length > 0 ? `WHERE ${whereConditions.join(' AND ')}` : '';

        const templates = await sequelize.query(`
            SELECT
                id::text,
                template_name,
                template_type,
                description,
                layer,
                default_config,
                script_template,
                is_system,
                is_public,
                usage_count,
                created_by_user_id::text,
                created_at,
                updated_at
            FROM transformation_templates
            ${whereClause}
            ORDER BY is_system DESC, usage_count DESC, template_name ASC
        `, {
            bind: bindings,
            type: QueryTypes.SELECT
        });

        res.json({
            success: true,
            count: templates.length,
            templates: templates
        });
    } catch (error) {
        console.error('[Templates] List error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Get single template by ID
 * GET /api/templates/:id
 */
exports.getTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        const templates = await sequelize.query(`
            SELECT
                id::text,
                template_name,
                template_type,
                description,
                layer,
                default_config,
                script_template,
                is_system,
                is_public,
                usage_count,
                created_by_user_id::text,
                created_at,
                updated_at
            FROM transformation_templates
            WHERE id = $1
        `, {
            bind: [id],
            type: QueryTypes.SELECT
        });

        if (templates.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Template not found'
            });
        }

        res.json({
            success: true,
            template: templates[0]
        });
    } catch (error) {
        console.error('[Templates] Get error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Create template from existing step
 * POST /api/templates/from-step
 *
 * Body:
 * {
 *   step_id: "uuid",
 *   template_name: "My Custom Validation",
 *   description: "...",
 *   is_public: true
 * }
 */
exports.createTemplateFromStep = async (req, res) => {
    try {
        const { step_id, template_name, description, is_public } = req.body;

        if (!step_id || !template_name) {
            return res.status(400).json({
                success: false,
                error: 'step_id and template_name are required'
            });
        }

        // Get the step
        const steps = await sequelize.query(`
            SELECT step_type, layer, config, script_type, script_content
            FROM transformation_steps
            WHERE id = $1
        `, {
            bind: [step_id],
            type: QueryTypes.SELECT
        });

        if (steps.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Step not found'
            });
        }

        const step = steps[0];

        // Create template
        const templateId = uuidv4();
        const userId = req.user ? req.user.id : null;

        await sequelize.query(`
            INSERT INTO transformation_templates
                (id, template_name, template_type, description, layer,
                 default_config, script_template, is_system, is_public,
                 created_by_user_id, usage_count)
            VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, $9, 0)
        `, {
            bind: [
                templateId,
                template_name,
                step.step_type,
                description || null,
                step.layer,
                step.config || {},
                step.script_content || null,
                is_public !== false, // Default to public
                userId
            ],
            type: QueryTypes.INSERT
        });

        console.log(`✅ Template created: ${templateId} - ${template_name}`);

        res.json({
            success: true,
            template_id: templateId,
            message: `Template "${template_name}" created successfully`
        });
    } catch (error) {
        console.error('[Templates] Create from step error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Create template directly
 * POST /api/templates
 *
 * Body:
 * {
 *   template_name: "...",
 *   template_type: "pre.validation",
 *   layer: "pre",
 *   description: "...",
 *   default_config: {},
 *   script_template: "...",
 *   is_public: true
 * }
 */
exports.createTemplate = async (req, res) => {
    try {
        const {
            template_name,
            template_type,
            layer,
            description,
            default_config,
            script_template,
            is_public
        } = req.body;

        if (!template_name || !template_type || !layer) {
            return res.status(400).json({
                success: false,
                error: 'template_name, template_type, and layer are required'
            });
        }

        const templateId = uuidv4();
        const userId = req.user ? req.user.id : null;

        await sequelize.query(`
            INSERT INTO transformation_templates
                (id, template_name, template_type, description, layer,
                 default_config, script_template, is_system, is_public,
                 created_by_user_id, usage_count)
            VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, $9, 0)
        `, {
            bind: [
                templateId,
                template_name,
                template_type,
                description || null,
                layer,
                default_config || {},
                script_template || null,
                is_public !== false,
                userId
            ],
            type: QueryTypes.INSERT
        });

        console.log(`✅ Template created: ${templateId} - ${template_name}`);

        res.json({
            success: true,
            template_id: templateId,
            message: `Template "${template_name}" created successfully`
        });
    } catch (error) {
        console.error('[Templates] Create error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Update template
 * PUT /api/templates/:id
 *
 * Body:
 * {
 *   template_name: "...",
 *   description: "...",
 *   default_config: {},
 *   script_template: "...",
 *   is_public: true,
 *   update_existing_steps: false  // If true, update all non-customized steps
 * }
 */
exports.updateTemplate = async (req, res) => {
    try {
        const { id } = req.params;
        const {
            template_name,
            description,
            default_config,
            script_template,
            is_public,
            update_existing_steps
        } = req.body;

        // Check if template exists and user has permission
        const templates = await sequelize.query(`
            SELECT is_system, created_by_user_id::text
            FROM transformation_templates
            WHERE id = $1
        `, {
            bind: [id],
            type: QueryTypes.SELECT
        });

        if (templates.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Template not found'
            });
        }

        const template = templates[0];

        // System templates can't be modified
        if (template.is_system) {
            return res.status(403).json({
                success: false,
                error: 'System templates cannot be modified'
            });
        }

        // User must own the template (unless admin)
        if (req.user && template.created_by_user_id !== req.user.id && !req.user.isAdmin) {
            return res.status(403).json({
                success: false,
                error: 'You do not have permission to modify this template'
            });
        }

        // Build update query
        const updates = [];
        const bindings = [];
        let bindIndex = 1;

        if (template_name !== undefined) {
            updates.push(`template_name = $${bindIndex++}`);
            bindings.push(template_name);
        }
        if (description !== undefined) {
            updates.push(`description = $${bindIndex++}`);
            bindings.push(description);
        }
        if (default_config !== undefined) {
            updates.push(`default_config = $${bindIndex++}`);
            bindings.push(default_config);
        }
        if (script_template !== undefined) {
            updates.push(`script_template = $${bindIndex++}`);
            bindings.push(script_template);
        }
        if (is_public !== undefined) {
            updates.push(`is_public = $${bindIndex++}`);
            bindings.push(is_public);
        }

        updates.push(`updated_at = CURRENT_TIMESTAMP`);
        bindings.push(id);

        // Update template
        await sequelize.query(`
            UPDATE transformation_templates
            SET ${updates.join(', ')}
            WHERE id = $${bindIndex}
        `, {
            bind: bindings,
            type: QueryTypes.UPDATE
        });

        console.log(`✅ Template updated: ${id}`);

        let stepsUpdated = 0;

        // Update existing steps if requested
        if (update_existing_steps && default_config !== undefined) {
            // Find all non-customized steps using this template
            const steps = await sequelize.query(`
                SELECT id::text, pipeline_id::text
                FROM transformation_steps
                WHERE template_id = $1 AND is_customized = false
            `, {
                bind: [id],
                type: QueryTypes.SELECT
            });

            // Update each step
            for (const step of steps) {
                await sequelize.query(`
                    UPDATE transformation_steps
                    SET config = $1, updated_at = CURRENT_TIMESTAMP
                    WHERE id = $2
                `, {
                    bind: [default_config, step.id],
                    type: QueryTypes.UPDATE
                });
            }

            stepsUpdated = steps.length;
            console.log(`✅ Updated ${stepsUpdated} steps using template ${id}`);
        }

        res.json({
            success: true,
            message: `Template updated successfully`,
            steps_updated: stepsUpdated
        });
    } catch (error) {
        console.error('[Templates] Update error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Delete template
 * DELETE /api/templates/:id
 */
exports.deleteTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        // Check if template exists
        const templates = await sequelize.query(`
            SELECT is_system, created_by_user_id::text, template_name
            FROM transformation_templates
            WHERE id = $1
        `, {
            bind: [id],
            type: QueryTypes.SELECT
        });

        if (templates.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Template not found'
            });
        }

        const template = templates[0];

        // System templates can't be deleted
        if (template.is_system) {
            return res.status(403).json({
                success: false,
                error: 'System templates cannot be deleted'
            });
        }

        // User must own the template (unless admin)
        if (req.user && template.created_by_user_id !== req.user.id && !req.user.isAdmin) {
            return res.status(403).json({
                success: false,
                error: 'You do not have permission to delete this template'
            });
        }

        // Delete template (steps will have template_id set to NULL due to ON DELETE SET NULL)
        await sequelize.query(`
            DELETE FROM transformation_templates WHERE id = $1
        `, {
            bind: [id],
            type: QueryTypes.DELETE
        });

        console.log(`✅ Template deleted: ${id} - ${template.template_name}`);

        res.json({
            success: true,
            message: `Template "${template.template_name}" deleted successfully`
        });
    } catch (error) {
        console.error('[Templates] Delete error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Apply template to pipeline (create step from template)
 * POST /api/templates/:id/apply
 *
 * Body:
 * {
 *   pipeline_id: "uuid",
 *   sequence: 100,
 *   step_name: "Custom step name" (optional, uses template name if not provided)
 * }
 */
exports.applyTemplate = async (req, res) => {
    try {
        const { id: template_id } = req.params;
        const { pipeline_id, sequence, step_name } = req.body;

        if (!pipeline_id || !sequence) {
            return res.status(400).json({
                success: false,
                error: 'pipeline_id and sequence are required'
            });
        }

        // Get template
        const templates = await sequelize.query(`
            SELECT
                template_name,
                template_type,
                layer,
                default_config,
                script_template
            FROM transformation_templates
            WHERE id = $1
        `, {
            bind: [template_id],
            type: QueryTypes.SELECT
        });

        if (templates.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Template not found'
            });
        }

        const template = templates[0];

        // Create new step from template
        const stepId = uuidv4();

        await sequelize.query(`
            INSERT INTO transformation_steps
                (id, pipeline_id, step_name, step_type, layer, sequence,
                 config, script_content, template_id, is_customized,
                 required, enabled)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, true, true)
        `, {
            bind: [
                stepId,
                pipeline_id,
                step_name || template.template_name,
                template.template_type,
                template.layer,
                sequence,
                template.default_config || {},
                template.script_template || null,
                template_id
            ],
            type: QueryTypes.INSERT
        });

        console.log(`✅ Template applied: ${template_id} → Step ${stepId}`);

        // Usage count is auto-incremented by trigger

        res.json({
            success: true,
            step_id: stepId,
            message: `Template "${template.template_name}" applied successfully`
        });
    } catch (error) {
        console.error('[Templates] Apply error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

module.exports = exports;

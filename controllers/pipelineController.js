// controllers/pipelineController_v2.js
// FIXED: Saves pipeline to V20 schema with embedded wizard mappings

const axios = require('axios');
const GO_BACKEND_URL = process.env.GO_BACKEND_URL || `http://127.0.0.1:${process.env.API_PORT || 8080}`;

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
            // 1. Upsert pipeline metadata
            const pipelineResult = await sequelize.query(`
                INSERT INTO transformation_pipelines
                    (id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
                VALUES
                    ($1, $2, $3, $4, $5, $6, NOW(), NOW())
                ON CONFLICT (interface_id, message_type)
                DO UPDATE SET
                    pipeline_name = EXCLUDED.pipeline_name,
                    enabled = EXCLUDED.enabled,
                    version = transformation_pipelines.version + 1,
                    updated_at = NOW()
                RETURNING id::text, interface_id::text, message_type, pipeline_name, enabled, version;
            `, {
                bind: [
                    pipelineId,
                    interfaceId,
                    messageType,
                    pipelineData.name || `${messageType} Pipeline`,
                    pipelineData.status !== 'disabled',
                    1
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

            // 3. Save each step from all layers
            let stepsSaved = 0;
            const layers = ['pre', 'core', 'post'];

            for (const layer of layers) {
                // Extract steps from execution groups (frontend uses visual pipeline model)
                const layerData = pipelineData.layers?.[layer];
                let layerSteps = [];

                if (layerData?.steps) {
                    // Direct steps array (legacy format)
                    layerSteps = layerData.steps;
                } else if (layerData?.execution_groups) {
                    // Visual pipeline format - extract steps from execution groups
                    layerData.execution_groups.forEach(group => {
                        if (group.steps && Array.isArray(group.steps)) {
                            layerSteps.push(...group.steps);
                        }
                    });
                }

                console.log(`📊 Layer "${layer}": Found ${layerSteps.length} steps`);

                for (const step of layerSteps) {
                    console.log(`📦 RAW STEP OBJECT:`, JSON.stringify(step, null, 2));
                    console.log(`📦 Processing step - ALL KEYS:`, Object.keys(step));
                    console.log(`📦 step_name:`, step.step_name);
                    console.log(`📦 stepName:`, step.stepName);
                    const stepId = step.id || uuidv4();
                    let stepConfig = step.config || {};

                    // 🔥 CRITICAL: Embed wizard mappings into HL7→FHIR mapping steps
                    const isHL7FHIRStep = (
                        step.stepType === 'core.mapping' ||
                        step.type === 'core.mapping' ||
                        step.templateId === 'hl7-fhir-mapping' ||
                        (step.stepName && step.stepName.includes('HL7') && step.stepName.includes('FHIR'))
                    );

                    if (isHL7FHIRStep && embeddedMappings) {
                        console.log('\n💾 === EMBEDDING WIZARD MAPPINGS ===');
                        console.log('Step name:', step.stepName || step.name);
                        console.log('Step type:', step.stepType || step.type);
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
                            (id, pipeline_id, step_name, step_type, sequence, layer, required, timeout_ms, enabled, config, script_type, script_content, on_error_strategy, created_at, updated_at)
                        VALUES
                            ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
                    `, {
                        bind: [
                            stepId,
                            pipelineId,
                            step.step_name || step.stepName || step.name || 'Unnamed Step',
                            step.step_type || step.stepType || step.type || 'custom',
                            step.sequence || 100,
                            layer,
                            step.required !== false,
                            step.timeoutMs || step.timeout_ms || 5000,
                            step.enabled !== false,
                            JSON.stringify(stepConfig),
                            step.scriptType || null,
                            step.scriptContent || null,
                            step.onErrorStrategy || step.on_error_strategy || 'fail'
                        ],
                        type: QueryTypes.INSERT
                    });

                    stepsSaved++;
                }
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

// Export all other functions from original controller (the OLD one, renamed)
const originalController = require('./pipelineController_old');
Object.keys(originalController).forEach(key => {
    if (key !== 'savePipeline') {
        exports[key] = originalController[key];
    }
});

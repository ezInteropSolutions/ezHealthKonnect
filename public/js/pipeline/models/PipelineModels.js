/**
 * Data Models for Pipeline Builder
 * Pure JavaScript data structures matching backend models
 */

class VisualPipeline {
    constructor(data = {}) {
        this.id = data.id || this.generateUUID();
        this.interfaceId = data.interfaceId || '';
        this.messageType = data.messageType || '';
        this.name = data.name || 'Untitled Pipeline';
        this.description = data.description || '';
        this.version = data.version || 1;
        this.status = data.status || 'draft'; // draft, active, paused
        // Flat execution groups (no layers)
        this.executionGroups = data.executionGroups || [];
        // Connections between steps (saved to database for persistence)
        this.connections = data.connections || [];
        // Pipeline-level config (default error handling, retry, etc.)
        this.pipelineConfig = data.pipelineConfig || {};
        this.createdAt = data.createdAt || new Date().toISOString();
        this.updatedAt = data.updatedAt || new Date().toISOString();
    }

    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }

    addExecutionGroup(group) {
        this.executionGroups.push(group);
    }

    removeExecutionGroup(groupId) {
        this.executionGroups = this.executionGroups.filter(g => g.id !== groupId);
    }

    getExecutionGroup(groupId) {
        return this.executionGroups.find(g => g.id === groupId);
    }

    getAllSteps() {
        const steps = [];
        for (const group of this.executionGroups) {
            if (group.steps && Array.isArray(group.steps)) {
                steps.push(...group.steps);
            }
        }
        steps.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));
        return steps;
    }

    toJSON() {
        return {
            id: this.id,
            interface_id: this.interfaceId,
            message_type: this.messageType,
            name: this.name,
            description: this.description,
            version: this.version,
            status: this.status,
            execution_groups: this.executionGroups.map(g => g.toJSON()),
            connections: this.connections || [],
            pipeline_config: this.pipelineConfig || {},
            created_at: this.createdAt,
            updated_at: this.updatedAt
        };
    }

    static fromJSON(json) {
        let executionGroups = [];
        if (json.execution_groups && json.execution_groups.length > 0) {
            executionGroups = json.execution_groups.map(g => VisualExecutionGroup.fromJSON(g));
        }

        return new VisualPipeline({
            id: json.id,
            interfaceId: json.interface_id,
            messageType: json.message_type,
            name: json.name,
            description: json.description,
            version: json.version,
            status: json.status,
            executionGroups: executionGroups,
            connections: json.connections || [],
            pipelineConfig: json.pipeline_config || {},
            createdAt: json.created_at,
            updatedAt: json.updated_at
        });
    }
}

// VisualLayer removed — layer concept eliminated. All groups are flat in VisualPipeline.executionGroups.

class VisualExecutionGroup {
    constructor(data = {}) {
        this.id = data.id || this.generateUUID();
        this.groupId = data.groupId || `group_${Date.now()}`;
        this.groupType = data.groupType || 'inline'; // parallel or inline
        this.sequence = data.sequence || 100;
        this.mergeStrategy = data.mergeStrategy || 'deep_merge';
        this.steps = data.steps || [];
        this.dependsOn = data.dependsOn || [];
        this.position = data.position || { x: 0, y: 0 };
    }

    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }

    addStep(step) {
        this.steps.push(step);
    }

    removeStep(stepId) {
        this.steps = this.steps.filter(s => s.id !== stepId);
    }

    getStep(stepId) {
        return this.steps.find(s => s.id === stepId);
    }

    toJSON() {
        return {
            id: this.id,
            group_id: this.groupId,
            group_type: this.groupType,
            sequence: this.sequence,
            merge_strategy: this.mergeStrategy,
            steps: this.steps.map(s => {
                // Safety: convert plain objects to VisualStep before serializing
                if (typeof s.toJSON === 'function') return s.toJSON();
                return new VisualStep(s).toJSON();
            }),
            depends_on: this.dependsOn,
            position: this.position
        };
    }

    static fromJSON(json) {
        const steps = (json.steps || []).map(s => VisualStep.fromJSON(s));
        return new VisualExecutionGroup({
            id: json.id,
            groupId: json.group_id || json.groupId,
            groupType: json.group_type || json.groupType,
            sequence: json.sequence,
            mergeStrategy: json.merge_strategy || json.mergeStrategy,
            steps: steps,
            dependsOn: json.depends_on || json.dependsOn || [],
            position: json.position || { x: 0, y: 0 }
        });
    }
}

class VisualStep {
    constructor(data = {}) {
        this.id = data.id || this.generateUUID();
        this.stepName = data.stepName || 'Untitled Step';
        this.stepType = data.stepType || 'custom';
        this.templateId = data.templateId || null;
        this.sequence = data.sequence || 100;
        this.required = data.required !== undefined ? data.required : true;
        this.timeoutMs = data.timeoutMs || 5000;
        this.enabled = data.enabled !== undefined ? data.enabled : true;
        // Deep clone config to prevent reference sharing between steps
        // Use structuredClone if available (modern browsers), fallback to JSON method
        this.config = data.config
            ? (typeof structuredClone === 'function'
                ? structuredClone(data.config)
                : JSON.parse(JSON.stringify(data.config)))
            : {};
        this.scriptType = data.scriptType || null;
        this.scriptContent = data.scriptContent || null;
        this.onErrorStrategy = data.onErrorStrategy || 'fail';
        this.executionMode = data.executionMode || 'sequential';
        this.description = data.description || '';
        // Auto-assign icon based on step type if not provided
        this.icon = data.icon || this.getIconForType(this.stepType);
        // Canvas position persistence (V39)
        this.position_x = data.position_x !== undefined ? data.position_x : null;
        this.position_y = data.position_y !== undefined ? data.position_y : null;
        // Conditional branch tracking - which conditional step this belongs to and which branch
        // branchType: 'true' | 'false' | null (null = not connected to a conditional branch) - for IfThenElse
        // caseValue: string | null (e.g., "M", "F", "default") - for Switch/Case
        this.parentConditionalStepId = data.parentConditionalStepId || null;
        this.branchType = data.branchType || null;
        this.caseValue = data.caseValue || null;
        // Container system: which container step this step belongs to and which zone
        this.parentStepId = data.parentStepId || null;
        this.containerZone = data.containerZone || null;

        // Error handling: per-step try-catch (base property inherited by all steps)
        // Stored in config.errorHandling but exposed as first-class accessor
        if (!this.config.errorHandling) {
            this.config.errorHandling = null; // null = not enabled (no overhead)
        }
    }

    /**
     * Check if this step has error handling enabled
     * @returns {boolean}
     */
    get hasErrorHandling() {
        return this.config?.errorHandling?.enabled === true;
    }

    /**
     * Get error handling configuration (or null if disabled)
     * @returns {Object|null}
     */
    get errorHandlingConfig() {
        return this.hasErrorHandling ? this.config.errorHandling : null;
    }

    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }

    toJSON() {
        console.log('[VisualStep.toJSON] Serializing step:', {
            id: this.id,
            stepName: this.stepName,
            stepType: this.stepType,
            instanceOf: this instanceof VisualStep,
            config: this.config,
            position_x: this.position_x,
            position_y: this.position_y,
            parentConditionalStepId: this.parentConditionalStepId,
            branchType: this.branchType,
            caseValue: this.caseValue
        });
        return {
            id: this.id,
            step_name: this.stepName,
            step_type: this.stepType,
            template_id: this.templateId,
            sequence: this.sequence,
            required: this.required,
            timeout_ms: this.timeoutMs,
            enabled: this.enabled,
            config: this.config,
            script_type: this.scriptType,
            script_content: this.scriptContent,
            on_error_strategy: this.onErrorStrategy,
            execution_mode: this.executionMode,
            description: this.description,
            icon: this.icon,
            position_x: this.position_x,
            position_y: this.position_y,
            // WORKAROUND: Don't send parent_conditional_step_id to avoid FK constraint issues
            // The backend topological sort should handle this, but there's a Docker volume issue
            // Branch membership can be determined from connections graph during execution
            parent_conditional_step_id: null,
            branch_type: this.branchType,
            case_value: this.caseValue,
            parent_step_id: this.parentStepId,
            container_zone: this.containerZone
        };
    }

    static fromJSON(json) {
        return new VisualStep({
            id: json.id,
            stepName: json.step_name,
            stepType: json.step_type,
            templateId: json.template_id,
            sequence: json.sequence,
            required: json.required,
            timeoutMs: json.timeout_ms,
            enabled: json.enabled,
            config: json.config,
            scriptType: json.script_type,
            scriptContent: json.script_content,
            onErrorStrategy: json.on_error_strategy,
            executionMode: json.execution_mode,
            description: json.description,
            icon: json.icon,
            position_x: json.position_x,
            position_y: json.position_y,
            parentConditionalStepId: json.parent_conditional_step_id,
            branchType: json.branch_type,
            caseValue: json.case_value,
            parentStepId: json.parent_step_id,
            containerZone: json.container_zone
        });
    }

    /**
     * Get icon for step type (automatic mapping)
     * @param {string} stepType - Step type (e.g., 'pre.validation', 'pre.enrichment.api')
     * @returns {string} Font Awesome icon class
     */
    getIconForType(stepType) {
        const iconMap = {
            // New type names
            'field_validation': 'fas fa-check-circle',
            'fhir_validation': 'fas fa-shield-alt',
            'enrichment.api': 'fas fa-cloud',
            'enrichment.database': 'fas fa-database',
            'enrichment.script': 'fas fa-code',
            'if_then_else': 'fas fa-sitemap',
            'switch_case': 'fas fa-code-branch',
            'hl7_fhir_transform': 'fas fa-exchange-alt',
            'field_mapping': 'fas fa-arrows-alt-h',
            'data_masking': 'fas fa-user-secret',
            'remove_duplicates': 'fas fa-filter',
            'normalizer': 'fas fa-random',
            'deidentify': 'fas fa-shield-alt',
            'control.loop': 'fas fa-redo-alt',
            'connector.inbound': 'fas fa-download',
            'connector.outbound': 'fas fa-upload',
            'payload.builder': 'fas fa-file-export',
            'payload_builder': 'fas fa-file-export',
            // Legacy type names (backward compat)
            'pre.validation': 'fas fa-check-circle',
            'pre.enrichment.api': 'fas fa-cloud',
            'pre.enrichment.database': 'fas fa-database',
            'pre.enrichment.script': 'fas fa-code',
            'pre.enrichment': 'fas fa-plus-circle',
            'core.transformation': 'fas fa-arrows-alt-h',
            'core.mapping': 'fas fa-project-diagram',
            'post.validation': 'fas fa-shield-alt',
            'post.error_handling': 'fas fa-exclamation-triangle',
            'post.quality': 'fas fa-check-double',
            'pre.logic': 'fas fa-sitemap',
            'custom': 'fas fa-cog',
            'default': 'fas fa-cog'
        };

        // Try exact match first
        if (iconMap[stepType]) {
            return iconMap[stepType];
        }

        // Try category match (e.g., 'pre.enrichment.xyz' → 'pre.enrichment')
        const parts = stepType.split('.');
        if (parts.length >= 2) {
            const category = parts.slice(0, 2).join('.');
            if (iconMap[category]) {
                return iconMap[category];
            }
        }

        // Default fallback
        return iconMap['default'];
    }

    clone() {
        return new VisualStep({
            stepName: this.stepName,
            stepType: this.stepType,
            templateId: this.templateId,
            sequence: this.sequence,
            required: this.required,
            timeoutMs: this.timeoutMs,
            enabled: this.enabled,
            // Note: config will be deep cloned again by VisualStep constructor
            config: this.config,
            scriptType: this.scriptType,
            scriptContent: this.scriptContent,
            onErrorStrategy: this.onErrorStrategy,
            executionMode: this.executionMode,
            description: this.description,
            icon: this.icon
        });
    }

    // ========================================================================
    // STATIC UTILITY METHODS - Centralized step type detection
    // Use these instead of scattering type checks throughout the codebase
    // ========================================================================

    /**
     * Check if a step is an HL7-FHIR transform step
     * Handles both old (core.mapping) and new (hl7_fhir_transform) type names
     * @param {VisualStep|Object} step - Step object with stepType and/or templateId
     * @returns {boolean}
     */
    static isHL7FHIRTransform(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        const templateId = step.templateId || step.template_id || '';
        return type === 'hl7_fhir_transform' ||
               type === 'core.mapping' ||
               templateId === 'hl7-fhir-mapping';
    }

    /**
     * Check if a step is a Script Enrichment step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isScriptEnrichment(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'enrichment.script' || type === 'pre.enrichment.script';
    }

    /**
     * Check if a step is an API Enrichment step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isAPIEnrichment(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'enrichment.api' || type === 'pre.enrichment.api';
    }

    /**
     * Check if a step is a Database Enrichment step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isDatabaseEnrichment(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'enrichment.database' || type === 'pre.enrichment.database';
    }

    /**
     * Check if a step is a Field Validation step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isFieldValidation(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'field_validation' || type === 'pre.validation';
    }

    /**
     * Check if a step is a FHIR Validation step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isFHIRValidation(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'fhir_validation' || type === 'post.validation';
    }

    /**
     * Check if a step is a Field Mapping (transformation) step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isFieldMapping(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'field_mapping' || type === 'core.transformation';
    }

    /**
     * Check if a step is a File Parser step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isFileParser(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'file_parser';
    }

    /**
     * Check if a step is a Remove Duplicates step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isRemoveDuplicates(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'remove_duplicates';
    }

    /**
     * Checks if a step is a Data Masking/Anonymization step.
     * Supports both the canonical 'data_masking' type and the legacy
     * 'post.data_masking' backward-compat alias registered in executor_registry.go.
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isDataMasking(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'data_masking' || type === 'post.data_masking';
    }

    /**
     * Check if a step is a Normalizer / Pivot / Transpose step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isNormalizer(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'normalizer' || type === 'post.normalizer';
    }

    /**
     * Check if a step is a De-identify (HIPAA Safe Harbor) step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isDeidentify(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'deidentify' || type === 'pre.deidentify' || type === 'post.deidentify';
    }

    /**
     * Check if a step is a Payload Builder step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isPayloadBuilder(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'payload.builder' || type === 'payload_builder';
    }

    /**
     * Check if a step is an If-Then-Else conditional step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isIfThenElse(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'if_then_else' || type === 'pre.logic';
    }

    /**
     * Check if a step is a Switch/Case conditional step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isSwitchCase(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'switch_case' || type === 'pre.logic.switch';
    }

    /**
     * Check if a step is a CDA parse step (cda.parse)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isCdaParseStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'cda.parse';
    }

    /**
     * Check if a step is a CDA-to-FHIR transform step (cda.to_fhir)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isCdaToFhirStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'cda.to_fhir';
    }

    /**
     * Check if a step is any CDA-family step (parse, transform, normalize, or fhir.to_cda)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isCdaStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'cda.parse' || type === 'cda.to_fhir' ||
               type === 'cda.normalize' || type === 'fhir.to_cda';
    }

    /**
     * Get the canonical (new) type name for any step type
     * @param {string} stepType - Old or new type name
     * @returns {string} Canonical type name
     */
    static getCanonicalType(stepType) {
        const typeMap = {
            // Old → New mappings
            'pre.validation': 'field_validation',
            'core.validation': 'field_validation',
            'pre.enrichment.api': 'enrichment.api',
            'pre.enrichment.database': 'enrichment.database',
            'pre.enrichment.script': 'enrichment.script',
            'core.mapping': 'hl7_fhir_transform',
            'core.transformation': 'field_mapping',
            'post.validation': 'fhir_validation',
            'pre.logic': 'if_then_else',
            'pre.logic.switch': 'switch_case',
            'post.data_masking': 'data_masking',
            'post.remove_duplicates': 'remove_duplicates',
            'post.normalizer': 'normalizer'
        };
        return typeMap[stepType] || stepType;
    }

    /**
     * Check if a step is ANY conditional/logic step (if-then-else OR switch-case)
     * Use this for layout engines and flowchart rendering
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isConditionalStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        const templateId = step.templateId || step.template_id || '';
        return type === 'if_then_else' ||
               type === 'switch_case' ||
               type === 'pre.logic' ||
               type === 'pre.logic.switch' ||
               type === 'core.logic' ||
               type === 'post.logic' ||
               type === 'control' ||
               templateId === 'if-then-else' ||
               templateId === 'switch-case';
    }

    /**
     * Check if a step is a Loop step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isLoopStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        const templateId = step.templateId || step.template_id || '';
        return type === 'control.loop' ||
               templateId === 'loop-container';
    }

    /**
     * Check if a step is a container step (Loop only)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isContainerStep(step) {
        return VisualStep.isLoopStep(step);
    }

    /**
     * Get the container zones for a container step
     * @param {VisualStep|Object} step
     * @returns {string[]} Zone names
     */
    static getContainerZones(step) {
        return ['body'];
    }

    /**
     * Check if a step is a Custom (user-defined script) step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isCustomStep(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'custom' || !!step.scriptContent;
    }

    /**
     * Check if a step is an Outbound Connector step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isOutboundConnector(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'connector.outbound';
    }

    /**
     * Check if a step is an Inbound Connector step
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isInboundConnector(step) {
        if (!step) return false;
        const type = step.stepType || step.step_type || '';
        return type === 'connector.inbound';
    }

    /**
     * Check if a step is any type of Connector step (Inbound or Outbound)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isConnectorStep(step) {
        return VisualStep.isInboundConnector(step) || VisualStep.isOutboundConnector(step);
    }

    /**
     * Check if a step is any type of Enrichment step (API, Database, or Script)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isEnrichmentStep(step) {
        return VisualStep.isAPIEnrichment(step) ||
               VisualStep.isDatabaseEnrichment(step) ||
               VisualStep.isScriptEnrichment(step);
    }

    /**
     * Check if a step is any type of Validation step (Field or FHIR)
     * @param {VisualStep|Object} step
     * @returns {boolean}
     */
    static isValidationStep(step) {
        return VisualStep.isFieldValidation(step) ||
               VisualStep.isFHIRValidation(step);
    }
}

class StepTemplate {
    constructor(data = {}) {
        this.id = data.id || '';
        this.name = data.name || '';
        this.type = data.type || '';
        this.description = data.description || '';
        this.icon = data.icon || 'fas fa-cog';
        // CRITICAL: Deep clone defaultConfig to prevent mutations to the template object
        // Templates are singletons in ToolboxManager, so we must protect defaultConfig from being modified
        if (data.defaultConfig) {
            // Force JSON method to ensure deep clone works
            const cloneMethod = typeof structuredClone === 'function' ? 'structuredClone' : 'JSON';
            this.defaultConfig = JSON.parse(JSON.stringify(data.defaultConfig));

            // Debug: Add mutation detection
            if (data.name === 'Script Enrichment' && this.defaultConfig.script) {
                console.log(`🔧 Template "${data.name}" constructor - cloned defaultConfig using ${cloneMethod}`);
                console.log('   Script preview:', this.defaultConfig.script.substring(0, 60) + '...');

                // Freeze the config to prevent mutations
                Object.freeze(this.defaultConfig);
                if (this.defaultConfig.script) {
                    console.log('   ⚠️ Froze defaultConfig to prevent mutations');
                }
            }
        } else {
            this.defaultConfig = {};
        }
        this.scriptTemplate = data.scriptTemplate || null;
        this.isSystem = data.isSystem !== undefined ? data.isSystem : false;
        // Template-level defaults for required and onErrorStrategy (override VisualStep defaults)
        this.required = data.required;  // undefined means use VisualStep default
        this.onErrorStrategy = data.onErrorStrategy;  // undefined means use VisualStep default
    }

    createStep() {
        // Deep clone defaultConfig to ensure complete isolation between step instances
        // Use structuredClone if available (modern browsers), fallback to JSON method
        const clonedConfig = typeof structuredClone === 'function'
            ? structuredClone(this.defaultConfig)
            : JSON.parse(JSON.stringify(this.defaultConfig || {}));

        console.log('🏭 Template.createStep() called for:', this.name);
        console.log('   Template ID:', this.id);
        console.log('   Template defaultConfig.script (first 100 chars):',
            this.defaultConfig.script ? this.defaultConfig.script.substring(0, 100) + '...' : 'N/A');
        console.log('   Cloned config.script (first 100 chars):',
            clonedConfig.script ? clonedConfig.script.substring(0, 100) + '...' : 'N/A');
        console.log('   Are they different objects?', this.defaultConfig !== clonedConfig);
        console.log('   Are script values equal?',
            this.defaultConfig.script === clonedConfig.script ? 'YES (expected)' : 'NO (unexpected)');

        return new VisualStep({
            stepName: this.name,
            stepType: this.type,
            templateId: this.id,
            config: clonedConfig,
            scriptContent: this.scriptTemplate,
            description: this.description,
            icon: this.icon,
            // Pass template-level defaults if specified
            required: this.required,
            onErrorStrategy: this.onErrorStrategy
        });
    }

    static fromJSON(json) {
        return new StepTemplate({
            id: json.id,
            name: json.name,
            type: json.type,
            description: json.description,
            icon: json.icon,
            defaultConfig: json.default_config || json.defaultConfig,
            scriptTemplate: json.script_template || json.scriptTemplate,
            isSystem: json.is_system !== undefined ? json.is_system : json.isSystem
        });
    }
}

// Export for browser (window object)
if (typeof window !== 'undefined') {
    window.VisualPipeline = VisualPipeline;
    window.VisualExecutionGroup = VisualExecutionGroup;
    window.VisualStep = VisualStep;
    window.StepTemplate = StepTemplate;
}

// Export for Node.js (if needed)
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        VisualPipeline,
        VisualExecutionGroup,
        VisualStep,
        StepTemplate
    };
}

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
        this.layers = data.layers || {
            pre: new VisualLayer('pre'),
            core: new VisualLayer('core'),
            post: new VisualLayer('post')
        };
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

    toJSON() {
        return {
            id: this.id,
            interface_id: this.interfaceId,
            message_type: this.messageType,
            name: this.name,
            description: this.description,
            version: this.version,
            status: this.status,
            layers: {
                pre: this.layers.pre.toJSON(),
                core: this.layers.core.toJSON(),
                post: this.layers.post.toJSON()
            },
            created_at: this.createdAt,
            updated_at: this.updatedAt
        };
    }

    static fromJSON(json) {
        return new VisualPipeline({
            id: json.id,
            interfaceId: json.interface_id,
            messageType: json.message_type,
            name: json.name,
            description: json.description,
            version: json.version,
            status: json.status,
            layers: {
                pre: VisualLayer.fromJSON(json.layers?.pre || {}),
                core: VisualLayer.fromJSON(json.layers?.core || {}),
                post: VisualLayer.fromJSON(json.layers?.post || {})
            },
            createdAt: json.created_at,
            updatedAt: json.updated_at
        });
    }
}

class VisualLayer {
    constructor(name, data = {}) {
        this.name = name;
        this.executionGroups = data.executionGroups || [];
    }

    addGroup(group) {
        this.executionGroups.push(group);
    }

    removeGroup(groupId) {
        this.executionGroups = this.executionGroups.filter(g => g.id !== groupId);
    }

    getGroup(groupId) {
        return this.executionGroups.find(g => g.id === groupId);
    }

    toJSON() {
        return {
            name: this.name,
            execution_groups: this.executionGroups.map(g => g.toJSON())
        };
    }

    static fromJSON(json) {
        return new VisualLayer(json.name, {
            executionGroups: (json.execution_groups || []).map(g => VisualExecutionGroup.fromJSON(g))
        });
    }
}

class VisualExecutionGroup {
    constructor(data = {}) {
        this.id = data.id || this.generateUUID();
        this.groupId = data.groupId || `group_${Date.now()}`;
        this.groupType = data.groupType || 'inline'; // parallel or inline
        this.layer = data.layer || 'core';
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
            layer: this.layer,
            sequence: this.sequence,
            merge_strategy: this.mergeStrategy,
            steps: this.steps.map(s => s.toJSON()),
            depends_on: this.dependsOn,
            position: this.position
        };
    }

    static fromJSON(json) {
        return new VisualExecutionGroup({
            id: json.id,
            groupId: json.group_id,
            groupType: json.group_type,
            layer: json.layer,
            sequence: json.sequence,
            mergeStrategy: json.merge_strategy,
            steps: (json.steps || []).map(s => VisualStep.fromJSON(s)),
            dependsOn: json.depends_on || [],
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
        this.layer = data.layer || 'core';
        this.sequence = data.sequence || 100;
        this.required = data.required !== undefined ? data.required : true;
        this.timeoutMs = data.timeoutMs || 5000;
        this.enabled = data.enabled !== undefined ? data.enabled : true;
        this.config = data.config || {};
        this.scriptType = data.scriptType || null;
        this.scriptContent = data.scriptContent || null;
        this.onErrorStrategy = data.onErrorStrategy || 'fail';
        this.executionMode = data.executionMode || 'sequential';
        this.description = data.description || '';
        this.icon = data.icon || 'fas fa-cog';
    }

    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }

    toJSON() {
        return {
            id: this.id,
            step_name: this.stepName,
            step_type: this.stepType,
            template_id: this.templateId,
            layer: this.layer,
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
            icon: this.icon
        };
    }

    static fromJSON(json) {
        return new VisualStep({
            id: json.id,
            stepName: json.step_name,
            stepType: json.step_type,
            templateId: json.template_id,
            layer: json.layer,
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
            icon: json.icon
        });
    }

    clone() {
        return new VisualStep({
            stepName: this.stepName,
            stepType: this.stepType,
            templateId: this.templateId,
            layer: this.layer,
            sequence: this.sequence,
            required: this.required,
            timeoutMs: this.timeoutMs,
            enabled: this.enabled,
            config: JSON.parse(JSON.stringify(this.config)),
            scriptType: this.scriptType,
            scriptContent: this.scriptContent,
            onErrorStrategy: this.onErrorStrategy,
            executionMode: this.executionMode,
            description: this.description,
            icon: this.icon
        });
    }
}

class StepTemplate {
    constructor(data = {}) {
        this.id = data.id || '';
        this.name = data.name || '';
        this.type = data.type || '';
        this.description = data.description || '';
        this.layer = data.layer || 'core';
        this.icon = data.icon || 'fas fa-cog';
        this.defaultConfig = data.defaultConfig || {};
        this.scriptTemplate = data.scriptTemplate || null;
        this.isSystem = data.isSystem !== undefined ? data.isSystem : false;
    }

    createStep() {
        return new VisualStep({
            stepName: this.name,
            stepType: this.type,
            templateId: this.id,
            layer: this.layer,
            config: JSON.parse(JSON.stringify(this.defaultConfig)),
            scriptContent: this.scriptTemplate,
            description: this.description,
            icon: this.icon
        });
    }

    static fromJSON(json) {
        return new StepTemplate({
            id: json.id,
            name: json.name,
            type: json.type,
            description: json.description,
            layer: json.layer,
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
    window.VisualLayer = VisualLayer;
    window.VisualExecutionGroup = VisualExecutionGroup;
    window.VisualStep = VisualStep;
    window.StepTemplate = StepTemplate;
}

// Export for Node.js (if needed)
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        VisualPipeline,
        VisualLayer,
        VisualExecutionGroup,
        VisualStep,
        StepTemplate
    };
}

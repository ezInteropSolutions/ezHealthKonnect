/**
 * Layer Container Manager
 * Manages the single pipeline canvas (unified - no pre/core/post layers)
 */

class LayerContainer {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.canvas = document.getElementById('canvasDropZone');
        this.executionMode = 'inline'; // Default mode
    }

    /**
     * Render the entire canvas
     */
    renderCanvas() {
        const container = this.canvas;
        if (!container) return;

        // Clear existing content (except placeholder)
        const placeholder = container.querySelector('.drop-zone-placeholder');
        container.innerHTML = '';
        if (placeholder) {
            container.appendChild(placeholder);
        }

        // Sort execution groups by sequence
        const groups = [...this.builder.pipeline.executionGroups].sort((a, b) => a.sequence - b.sequence);

        // Render execution groups
        groups.forEach(group => {
            this.renderExecutionGroup(group, container);
        });

        // Hide placeholder if has content
        if (groups.length > 0 && placeholder) {
            placeholder.style.display = 'none';
        } else if (groups.length === 0 && placeholder) {
            placeholder.style.display = 'flex';
        }
    }

    /**
     * Render execution group
     */
    renderExecutionGroup(group, container) {
        const groupElement = document.createElement('div');
        groupElement.className = `execution-group ${group.groupType}`;
        groupElement.dataset.groupId = group.id;

        // Group header
        const header = document.createElement('div');
        header.className = 'execution-group-header';
        header.innerHTML = `
            <i class="fas ${group.groupType === 'parallel' ? 'fa-stream' : 'fa-arrow-right'}"></i>
            ${group.groupType === 'parallel' ? 'Parallel Execution' : 'Sequential Execution'}
            <button class="group-delete-btn" style="margin-left: auto; background: none; border: none; cursor: pointer; color: #ef4444;">
                <i class="fas fa-times"></i>
            </button>
        `;

        groupElement.appendChild(header);

        // Render steps
        group.steps.forEach(step => {
            const stepNode = this.builder.stepNodeManager.createStepNode(step, group.id);
            groupElement.appendChild(stepNode);
        });

        // Delete group button
        const deleteBtn = header.querySelector('.group-delete-btn');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', () => {
                this.deleteGroup(group.id);
            });
        }

        container.appendChild(groupElement);
    }

    /**
     * Add step to canvas
     */
    addStep(step) {
        const pipeline = this.builder.pipeline;

        // Check if we should create new group or add to existing
        let targetGroup;

        if (pipeline.executionGroups.length === 0) {
            // Create first group
            targetGroup = new VisualExecutionGroup({
                groupType: this.executionMode,
                layer: 'core',
                sequence: this.getNextSequence()
            });
            pipeline.addExecutionGroup(targetGroup);
        } else {
            // Add to last group of same type, or create new group
            const lastGroup = pipeline.executionGroups[pipeline.executionGroups.length - 1];

            if (lastGroup.groupType === this.executionMode) {
                targetGroup = lastGroup;
            } else {
                targetGroup = new VisualExecutionGroup({
                    groupType: this.executionMode,
                    layer: 'core',
                    sequence: this.getNextSequence()
                });
                pipeline.addExecutionGroup(targetGroup);
            }
        }

        // Add step to group - ensure it's a VisualStep instance
        step.layer = 'core';
        step.sequence = this.getNextStepSequence(targetGroup);

        // Convert plain object to VisualStep if needed
        let visualStep = step;
        if (!(step instanceof VisualStep)) {
            visualStep = new VisualStep({
                id: step.id,
                stepName: step.stepName || step.step_name,
                stepType: step.stepType || step.step_type,
                templateId: step.templateId || step.template_id,
                layer: step.layer,
                sequence: step.sequence,
                required: step.required,
                timeoutMs: step.timeoutMs || step.timeout_ms,
                enabled: step.enabled,
                config: step.config || {},
                scriptType: step.scriptType || step.script_type,
                scriptContent: step.scriptContent || step.script_content,
                onErrorStrategy: step.onErrorStrategy || step.on_error_strategy,
                executionMode: step.executionMode || step.execution_mode,
                description: step.description,
                icon: step.icon,
                position_x: step.position_x,
                position_y: step.position_y,
                parentConditionalStepId: step.parentConditionalStepId || step.parent_conditional_step_id,
                branchType: step.branchType || step.branch_type,
                caseValue: step.caseValue || step.case_value
            });
        }
        targetGroup.addStep(visualStep);

        // Re-render canvas
        this.renderCanvas();

        // Redraw connections
        this.builder.canvasRenderer.redrawAllConnections();
    }

    /**
     * Remove step from group
     */
    removeStepFromGroup(stepId, groupId) {
        const pipeline = this.builder.pipeline;

        for (const group of pipeline.executionGroups) {
            if (group.id === groupId || group.groupId === groupId) {
                group.removeStep(stepId);

                // If group is empty, remove it
                if (group.steps.length === 0) {
                    pipeline.removeExecutionGroup(group.id || group.groupId);
                }

                // Re-render
                this.renderCanvas();
                this.builder.canvasRenderer.redrawAllConnections();
                return;
            }
        }
    }

    /**
     * Delete execution group
     */
    async deleteGroup(groupId) {
        const confirmed = await this.builder.dragDropManager.showConfirmDialog(
            'Delete this execution group and all its steps?',
            {
                title: 'Delete Group',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (!confirmed) return;

        this.builder.pipeline.removeExecutionGroup(groupId);
        this.renderCanvas();
        this.builder.canvasRenderer.redrawAllConnections();
        this.builder.dragDropManager.showNotification('Group deleted', 'info');
    }

    /**
     * Set execution mode
     */
    setExecutionMode(mode) {
        this.executionMode = mode;
    }

    /**
     * Get next sequence number
     */
    getNextSequence() {
        const groups = this.builder.pipeline.executionGroups;
        if (groups.length === 0) {
            return 10;
        }

        const maxSequence = Math.max(...groups.map(g => g.sequence));
        return maxSequence + 10;
    }

    /**
     * Get next step sequence in group
     */
    getNextStepSequence(group) {
        if (group.steps.length === 0) {
            return group.sequence;
        }

        const maxSequence = Math.max(...group.steps.map(s => s.sequence));
        return maxSequence + 10;
    }

    /**
     * Clear canvas
     */
    clearCanvas() {
        this.builder.pipeline.executionGroups = [];
        this.renderCanvas();
    }

    // ============================================================
    // BACKWARD COMPATIBILITY ALIASES
    // These methods delegate to the new names so old code still works
    // ============================================================

    /**
     * @deprecated Use addStep() instead
     */
    addStepToLayer(step, _layerName) {
        this.addStep(step);
    }

    /**
     * @deprecated Use renderCanvas() instead
     */
    renderAllLayers() {
        this.renderCanvas();
    }

    /**
     * @deprecated Use clearCanvas() instead
     */
    clearAllLayers() {
        this.clearCanvas();
    }

    /**
     * @deprecated Use renderCanvas() instead
     */
    renderLayer(_layerName, _visualLayer) {
        this.renderCanvas();
    }

    /**
     * @deprecated No longer needed (single canvas)
     */
    moveStepToLayer(_stepId, _sourceGroupId, _sourceLayer, _targetLayer) {
        // No-op: layers no longer exist
    }

    /**
     * @deprecated No longer needed
     */
    updateLayerBadge(_layerName, _visualLayer) {
        // No-op: badges removed
    }
}

// Export
if (typeof window !== 'undefined') {
    window.LayerContainer = LayerContainer;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = LayerContainer;
}

/**
 * Layer Container Manager
 * Manages the three-layer canvas (pre, core, post)
 */

class LayerContainer {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.layers = {
            pre: document.getElementById('preLayer'),
            core: document.getElementById('coreLayer'),
            post: document.getElementById('postLayer')
        };
        this.badges = {
            pre: document.getElementById('preLayerBadge'),
            core: document.getElementById('coreLayerBadge'),
            post: document.getElementById('postLayerBadge')
        };
        this.executionMode = 'inline'; // Default mode
    }

    /**
     * Render entire layer
     */
    renderLayer(layerName, visualLayer) {
        const container = this.layers[layerName];
        if (!container) return;

        // Clear existing content (except placeholder)
        const placeholder = container.querySelector('.drop-zone-placeholder');
        container.innerHTML = '';
        if (placeholder) {
            container.appendChild(placeholder);
        }

        // Render execution groups
        visualLayer.executionGroups.forEach(group => {
            this.renderExecutionGroup(group, container);
        });

        // Update badge
        this.updateLayerBadge(layerName, visualLayer);

        // Hide placeholder if has content
        if (visualLayer.executionGroups.length > 0 && placeholder) {
            placeholder.style.display = 'none';
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
                this.deleteGroup(group.id, group.layer);
            });
        }

        container.appendChild(groupElement);
    }

    /**
     * Add step to layer
     */
    addStepToLayer(step, layerName) {
        const visualLayer = this.builder.pipeline.layers[layerName];

        // Check if we should create new group or add to existing
        let targetGroup;

        if (visualLayer.executionGroups.length === 0) {
            // Create first group
            targetGroup = new VisualExecutionGroup({
                groupType: this.executionMode,
                layer: layerName,
                sequence: this.getNextSequence(layerName)
            });
            visualLayer.addGroup(targetGroup);
        } else {
            // Add to last group of same type, or create new group
            const lastGroup = visualLayer.executionGroups[visualLayer.executionGroups.length - 1];

            if (lastGroup.groupType === this.executionMode) {
                targetGroup = lastGroup;
            } else {
                targetGroup = new VisualExecutionGroup({
                    groupType: this.executionMode,
                    layer: layerName,
                    sequence: this.getNextSequence(layerName)
                });
                visualLayer.addGroup(targetGroup);
            }
        }

        // Add step to group
        step.layer = layerName;
        step.sequence = this.getNextStepSequence(targetGroup);
        targetGroup.addStep(step);

        // Re-render layer
        this.renderLayer(layerName, visualLayer);

        // Redraw connections
        this.builder.canvasRenderer.redrawAllConnections();
    }

    /**
     * Remove step from layer
     */
    removeStepFromGroup(stepId, groupId) {
        // Find the group
        for (const [layerName, visualLayer] of Object.entries(this.builder.pipeline.layers)) {
            const group = visualLayer.getGroup(groupId);
            if (group) {
                group.removeStep(stepId);

                // If group is empty, remove it
                if (group.steps.length === 0) {
                    visualLayer.removeGroup(groupId);
                }

                // Re-render
                this.renderLayer(layerName, visualLayer);
                this.builder.canvasRenderer.redrawAllConnections();
                return;
            }
        }
    }

    /**
     * Move step to different layer
     */
    moveStepToLayer(stepId, sourceGroupId, sourceLayer, targetLayer) {
        // Find and remove from source
        const sourceVisualLayer = this.builder.pipeline.layers[sourceLayer];
        const sourceGroup = sourceVisualLayer.getGroup(sourceGroupId);

        if (!sourceGroup) return;

        const step = sourceGroup.getStep(stepId);
        if (!step) return;

        sourceGroup.removeStep(stepId);

        // Remove empty group
        if (sourceGroup.steps.length === 0) {
            sourceVisualLayer.removeGroup(sourceGroupId);
        }

        // Add to target layer
        this.addStepToLayer(step, targetLayer);

        // Re-render both layers
        this.renderLayer(sourceLayer, sourceVisualLayer);
    }

    /**
     * Delete execution group
     */
    deleteGroup(groupId, layerName) {
        if (!confirm('Delete this execution group and all its steps?')) return;

        const visualLayer = this.builder.pipeline.layers[layerName];
        visualLayer.removeGroup(groupId);

        this.renderLayer(layerName, visualLayer);
        this.builder.canvasRenderer.redrawAllConnections();
        this.builder.dragDropManager.showNotification('Group deleted', 'info');
    }

    /**
     * Update layer badge
     */
    updateLayerBadge(layerName, visualLayer) {
        const badge = this.badges[layerName];
        if (!badge) return;

        const stepCount = visualLayer.executionGroups.reduce((sum, group) => sum + group.steps.length, 0);
        badge.textContent = `${stepCount} step${stepCount !== 1 ? 's' : ''}`;
    }

    /**
     * Set execution mode
     */
    setExecutionMode(mode) {
        this.executionMode = mode;
    }

    /**
     * Get next sequence number for layer
     */
    getNextSequence(layerName) {
        const baseSequences = {
            pre: 0,
            core: 100,
            post: 200
        };

        const visualLayer = this.builder.pipeline.layers[layerName];
        if (visualLayer.executionGroups.length === 0) {
            return baseSequences[layerName] + 10;
        }

        const maxSequence = Math.max(...visualLayer.executionGroups.map(g => g.sequence));
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
     * Clear all layers
     */
    clearAllLayers() {
        ['pre', 'core', 'post'].forEach(layerName => {
            const visualLayer = this.builder.pipeline.layers[layerName];
            visualLayer.executionGroups = [];
            this.renderLayer(layerName, visualLayer);

            // Show placeholder
            const placeholder = this.layers[layerName]?.querySelector('.drop-zone-placeholder');
            if (placeholder) {
                placeholder.style.display = 'flex';
            }
        });
    }

    /**
     * Render all layers
     */
    renderAllLayers() {
        ['pre', 'core', 'post'].forEach(layerName => {
            const visualLayer = this.builder.pipeline.layers[layerName];
            this.renderLayer(layerName, visualLayer);
        });
    }
}

// Export
if (typeof window !== 'undefined') {
    window.LayerContainer = LayerContainer;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = LayerContainer;
}

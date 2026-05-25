/**
 * Step Node Manager
 * Manages step node creation, rendering, and interaction
 */

class StepNodeManager {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.selectedNode = null;
    }

    /**
     * Create step node element
     */
    createStepNode(step, groupId) {
        const node = document.createElement('div');
        node.className = 'step-node';
        node.dataset.stepId = step.id;
        node.dataset.groupId = groupId;

        node.innerHTML = `
            <div class="step-node-header">
                <div class="step-reorder-controls">
                    <button class="step-node-btn btn-move-up" title="Move Up">
                        <i class="fas fa-arrow-up"></i>
                    </button>
                    <button class="step-node-btn btn-move-down" title="Move Down">
                        <i class="fas fa-arrow-down"></i>
                    </button>
                </div>
                <div class="step-node-title">
                    <i class="${step.icon}"></i>
                    <span>${step.stepName}</span>
                </div>
                <div class="step-node-actions">
                    <button class="step-node-btn btn-config" title="Configure">
                        <i class="fas fa-cog"></i>
                    </button>
                    <button class="step-node-btn btn-duplicate" title="Duplicate">
                        <i class="fas fa-copy"></i>
                    </button>
                    <button class="step-node-btn btn-delete" title="Delete">
                        <i class="fas fa-trash"></i>
                    </button>
                </div>
            </div>
            <div class="step-node-meta">
                <span class="step-type-badge">${this.formatStepType(step.stepType)}</span>
                ${step.required ? '<span class="required-badge">Required</span>' : ''}
                ${!step.enabled ? '<span class="disabled-badge">Disabled</span>' : ''}
            </div>
        `;

        // Add event listeners
        this.attachNodeEvents(node, step, groupId);

        // Make draggable
        this.builder.dragDropManager.makeStepNodeDraggable(node, step, groupId);

        return node;
    }

    /**
     * Attach event listeners to node
     */
    attachNodeEvents(node, step, groupId) {
        // Select node on click
        node.addEventListener('click', (e) => {
            if (!e.target.closest('.step-node-btn')) {
                this.selectNode(node, step);
            }
        });

        // Configure button
        const configBtn = node.querySelector('.btn-config');
        if (configBtn) {
            configBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.selectNode(node, step);
            });
        }

        // Duplicate button
        const duplicateBtn = node.querySelector('.btn-duplicate');
        if (duplicateBtn) {
            duplicateBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.duplicateStep(step, groupId);
            });
        }

        // Delete button
        const deleteBtn = node.querySelector('.btn-delete');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.deleteStep(step.id, groupId);
            });
        }

        // Move Up button
        const moveUpBtn = node.querySelector('.btn-move-up');
        if (moveUpBtn) {
            moveUpBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.moveStepUp(step, groupId);
            });
        }

        // Move Down button
        const moveDownBtn = node.querySelector('.btn-move-down');
        if (moveDownBtn) {
            moveDownBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.moveStepDown(step, groupId);
            });
        }
    }

    /**
     * Select node and show properties
     */
    selectNode(node, step) {
        // Deselect previous
        if (this.selectedNode) {
            this.selectedNode.classList.remove('selected');
        }

        // Select new
        this.selectedNode = node;
        node.classList.add('selected');

        // Show properties
        this.builder.propertiesPanel.showStepProperties(step);
    }

    /**
     * Deselect current node
     */
    deselectNode() {
        // Prevent infinite loop with PropertiesPanel.hideProperties()
        if (this.isDeselecting) return;
        this.isDeselecting = true;

        try {
            if (this.selectedNode) {
                this.selectedNode.classList.remove('selected');
                this.selectedNode = null;
            }
            this.builder.propertiesPanel.hideProperties();
        } finally {
            this.isDeselecting = false;
        }
    }

    /**
     * Duplicate step
     */
    duplicateStep(step, groupId) {
        const newStep = step.clone();
        newStep.stepName = `${step.stepName} (Copy)`;

        this.builder.addStepToGroup(newStep, groupId);
        this.builder.dragDropManager.showNotification(`Duplicated ${step.stepName}`, 'success');
    }

    /**
     * Delete step
     */
    async deleteStep(stepId, groupId) {
        const confirmed = await this.builder.dragDropManager.showConfirmDialog(
            'Are you sure you want to delete this step?',
            {
                title: 'Delete Step',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (confirmed) {
            this.builder.removeStepFromGroup(stepId, groupId);
            this.deselectNode();
            this.builder.dragDropManager.showNotification('Step deleted', 'info');
        }
    }

    /**
     * Update node after step modification
     */
    updateNode(stepId, updatedStep) {
        const node = document.querySelector(`[data-step-id="${stepId}"]`);
        if (!node) return;

        // Update title
        const titleSpan = node.querySelector('.step-node-title span');
        if (titleSpan) {
            titleSpan.textContent = updatedStep.stepName;
        }

        // Update icon
        const icon = node.querySelector('.step-node-title i');
        if (icon) {
            icon.className = updatedStep.icon;
        }

        // Update meta
        const meta = node.querySelector('.step-node-meta');
        if (meta) {
            meta.innerHTML = `
                <span class="step-type-badge">${this.formatStepType(updatedStep.stepType)}</span>
                ${updatedStep.required ? '<span class="required-badge">Required</span>' : ''}
                ${!updatedStep.enabled ? '<span class="disabled-badge">Disabled</span>' : ''}
            `;
        }
    }

    /**
     * Format step type for display
     */
    formatStepType(stepType) {
        return stepType
            .split('.')
            .map(word => word.charAt(0).toUpperCase() + word.slice(1))
            .join(' ');
    }

    /**
     * Get selected step
     */
    getSelectedStep() {
        if (!this.selectedNode) return null;

        const stepId = this.selectedNode.dataset.stepId;
        const groupId = this.selectedNode.dataset.groupId;

        return this.builder.findStep(stepId, groupId);
    }

    /**
     * Move step up in the execution order
     */
    moveStepUp(step, groupId) {
        const layer = this.builder.pipeline.layers[step.layer];
        if (!layer) return;

        // Find the execution group
        const group = layer.executionGroups.find(g => g.id === groupId);
        if (!group) return;

        // Find step index
        const stepIndex = group.steps.findIndex(s => s.id === step.id);
        if (stepIndex <= 0) {
            // Already at the top
            this.builder.dragDropManager.showNotification('Step is already at the top', 'info');
            return;
        }

        // Swap with previous step
        const temp = group.steps[stepIndex - 1];
        group.steps[stepIndex - 1] = group.steps[stepIndex];
        group.steps[stepIndex] = temp;

        // Update sequence numbers
        group.steps.forEach((s, i) => {
            s.sequence = (i + 1) * 10;
        });

        // Re-render the layer
        this.builder.layerContainer.renderLayer(step.layer, layer);
        this.builder.markAsUnsaved();
        this.builder.dragDropManager.showNotification(`Moved ${step.stepName} up`, 'success');
    }

    /**
     * Move step down in the execution order
     */
    moveStepDown(step, groupId) {
        const layer = this.builder.pipeline.layers[step.layer];
        if (!layer) return;

        // Find the execution group
        const group = layer.executionGroups.find(g => g.id === groupId);
        if (!group) return;

        // Find step index
        const stepIndex = group.steps.findIndex(s => s.id === step.id);
        if (stepIndex < 0 || stepIndex >= group.steps.length - 1) {
            // Already at the bottom
            this.builder.dragDropManager.showNotification('Step is already at the bottom', 'info');
            return;
        }

        // Swap with next step
        const temp = group.steps[stepIndex + 1];
        group.steps[stepIndex + 1] = group.steps[stepIndex];
        group.steps[stepIndex] = temp;

        // Update sequence numbers
        group.steps.forEach((s, i) => {
            s.sequence = (i + 1) * 10;
        });

        // Re-render the layer
        this.builder.layerContainer.renderLayer(step.layer, layer);
        this.builder.markAsUnsaved();
        this.builder.dragDropManager.showNotification(`Moved ${step.stepName} down`, 'success');
    }
}

// Export
if (typeof window !== 'undefined') {
    window.StepNodeManager = StepNodeManager;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = StepNodeManager;
}
